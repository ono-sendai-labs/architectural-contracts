// Package stdlibmap is the generation-side shell for the standard-library
// authority map (design §Components "New: stdlibmap"): deterministic SDK
// package discovery, the independent go/types inventory oracle the map is
// reconciled against, the target-specific SDK key with its classifier
// fingerprint, total-map Capslock classification and emission, the
// native SDK-keyed on-demand cache (design DR-05, DR-09, I3), and the
// explicit-input mode's layout-backed loading through the packagelayout
// driver (design I5).
//
// Component Contract (FR10):
//   - What it does: Discovers the SDK's standard-library package list behind an
//     injectable oracle (native `go list std` or an explicit Bazel-supplied
//     toolchain package list), normalizes it into a total, deduplicated,
//     canonically sorted PackageEntry list with importable flags (internal path
//     segments and target-excluded/no-source packages are non-importable),
//     reconciles the explicit oracle against
//     the layout's discovery, inventories every
//     externally referencable exported declaration of every importable
//     package via go/types under the declaring-object rule (including one
//     aggregate pkg.init per importable package), derives the
//     target-configuration SDKKey with a deterministic classifier_hash,
//     generates the total authority map (Generate; generateViaLayout serves
//     every load through the packagelayout layout driver; generateExplicit is
//     the CLI-facing explicit-input entry point), parses and renders the
//     deterministic key=value target-configuration file
//     and toolchain package-list file, and opens it from the native cache on
//     demand (OpenCachedMap): a digest of the complete SDKKey names the cache
//     entry, every hit is revalidated against the requested key, corrupt or
//     mismatched content is regenerated, and fresh bytes are written
//     atomically so a failed generation leaves the previous artifact intact.
//   - What it requires: A package oracle, a batch package Loader for the
//     target configuration (native: the toolchain via go/packages; explicit:
//     the self-exec layout driver), the target toolchain
//     version and build environment, and the generation classifier text plus
//     an explicit rule-version string. Every inventoried identifier must be
//     grammar-valid under the canonical SymbolID constructors. Cache
//     operations take injectable seams (cache root, filesystem reads, atomic
//     writes, generation) so tests neither mutate the real user cache nor run
//     a full SDK analysis.
//   - What it provides: PackageEntry, PackageOracle, PackageOracleFunc,
//     NativeStdPackageList (target-environment aware), RenderTargetConfig,
//     Loader, LoaderFunc, TargetConfig, GenerationDescriptor, ClassifierRule,
//     CanonicalClassifierText, DeriveSDKKey, GenerationClassifierRules,
//     RuleVersion, FindingSource, FindingSourceFunc, GeneratedMap, Generate,
//     NativeTargetConfig, TargetEnv, NativeEnvironment, NativeLoader,
//     CachedMapInput, OpenCachedMap, WriteArtifactAtomic, DecodeMapArtifact,
//     RunGenerateCommand, and RunProjectExportCommand (declared in stdlibmap.go
//     and sdkkey.go).
//   - Ambient Authority: None (`declared_authority` is empty). Host toolchain
//     invocation (`go list std`, `go env`), user cache directory and
//     environment queries, and host filesystem probes/writes are confined to
//     the `stdlibmap-hosttoolchain` and `atomicfile` sub-components.
package stdlibmap

import (
	"context"
	"fmt"
	"go/types"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap/internal/hosttoolchain"
)

// PackageEntry is one package of the SDK enumeration oracle's total list
// (design DR-05): its import path and whether it is importable. A package is
// non-importable when any import-path segment is `internal`, or when the
// target's build constraints exclude all of its Go sources (including a
// directory with no Go sources); it stays in the total package inventory but
// has no symbol or init inventory.
type PackageEntry struct {
	// Path is the canonical import path.
	Path string
	// Importable reports whether the package may be loaded and inventoried.
	Importable bool
}

// PackageOracle is the injectable package-discovery seam behind map generation
// (task req 2): native discovery runs `go list std` for the target toolchain;
// Bazel supplies the pinned toolchain's package list explicitly. The
// returned enumeration is the total package list; implementations must not
// filter, and the caller normalizes it.
type PackageOracle interface {
	// Packages returns the SDK's standard-library package paths, in any
	// order, possibly with duplicates. Errors fail generation.
	Packages() ([]PackageEntry, error)
}

// PackageOracleFunc adapts a function to the PackageOracle seam.
type PackageOracleFunc func() ([]PackageEntry, error)

// Packages implements PackageOracle.
func (f PackageOracleFunc) Packages() ([]PackageEntry, error) { return f() }

// Loader is the injectable batch package-loading seam (task req 9): it loads
// every requested importable package for the target configuration described by
// the generation environment. Implementations must not consult global
// process state beyond the configuration they were constructed with.
type Loader interface {
	// Load type-loads every path and returns the loaded *types.Package per
	// path. A load or type error for any path fails the whole batch with an
	// actionable error; a path absent from the result is likewise an error,
	// so a partial result can never masquerade as a complete one.
	Load(paths []string) (map[string]*types.Package, error)
}

// LoaderFunc adapts a function to the Loader seam.
type LoaderFunc func(paths []string) (map[string]*types.Package, error)

// Load implements Loader.
func (f LoaderFunc) Load(paths []string) (map[string]*types.Package, error) { return f(paths) }

// FindingSource is the injectable per-package Capslock seam behind Generate:
// Generate invokes it once for each importable package so each result is
// scoped to that package's own analysis closure. The
// capslockadapter.GenerationFindings function is the native implementation.
type FindingSource interface {
	Findings(paths []string) ([]capslockadapter.GenerationFinding, error)
}

// FindingSourceFunc adapts a function to the FindingSource seam.
type FindingSourceFunc func(paths []string) ([]capslockadapter.GenerationFinding, error)

// Findings implements FindingSource.
func (f FindingSourceFunc) Findings(paths []string) ([]capslockadapter.GenerationFinding, error) {
	return f(paths)
}

// GeneratedMap is one completed generation: the canonical map message, its
// canonical bytes, and the digest over those bytes (DR-15).
type GeneratedMap struct {
	Map    *gen.StdlibMap
	Bytes  []byte
	Digest string
}

// GenerationInput carries every seam and configuration Generate needs: the
// target configuration and rule version for the SDK key, the package oracle,
// the batch loader, and the Capslock findings source.
type GenerationInput struct {
	// Target describes the target build configuration (DR-09).
	Target TargetConfig
	// RuleVersion is the generation classification rules' explicit version.
	RuleVersion string
	// Oracle enumerates the SDK's packages (total, any order).
	Oracle PackageOracle
	// Loader type-loads the importable packages for the target.
	Loader Loader
	// Findings runs Capslock for one importable package at a time. When nil,
	// the native Capslock runner is used, bound to the same target environment
	// as the loader and oracle (round-1 finding: the analysis must describe the
	// target, not the host).
	Findings FindingSource
}

// Generate produces the total stdlib authority map (task reqs 1–10): it
// inventories the SDK independently, runs one Capslock batch per importable
// package, groups the findings by root, completes every inventoried symbol and init with a
// terminal classification under the design's object-kind rules, reconciles
// the exact inventory equality, and emits canonical bytes through the
// artifact I/O boundary. It fails closed: any inventory gap, unaccounted
// analyzer root, or validation error means no map.
func Generate(in GenerationInput) (*GeneratedMap, error) {
	return generate(in)
}

// CachedMapInput is one native-cache open request: the target SDK key the
// map must match (N3) and, for a cache miss, the full generation
// configuration. Root selects an explicit cache root (tests, Bazel-style
// pinned locations); the empty value resolves the platform user cache dir's
// arcc subtree.
type CachedMapInput struct {
	// Key is the requested target SDK configuration. The opened authority is
	// guaranteed to agree with it on every field, classifier_hash and
	// format version included.
	Key stdlibauthority.SDKKey
	// Root is an explicit cache root; empty means defaultCacheRoot.
	Root string
	// Generation is the generation configuration used on a cache miss; the
	// freshly generated bytes are validated against Key before they are
	// written, so a generation producing a different key fails closed.
	Generation GenerationInput
	// Seams inject the cache's environment and filesystem operations.
	Seams *cacheSeams
}

// OpenCachedMap returns the stdlib authority map for the requested SDK key,
// generating and caching it on demand (task reqs 1–3, design "native
// on-demand generator with cache").
func OpenCachedMap(in CachedMapInput) (stdlibauthority.StdlibAuthority, error) {
	return openCachedMap(in)
}

// WriteArtifactAtomic replaces path with persisted stdlib-map artifact bytes
// using the artifact I/O boundary's atomic-write semantics (Step 3).
func WriteArtifactAtomic(path string, data []byte, mode fs.FileMode) error {
	return writeArtifactAtomic(path, data, mode)
}

// DecodeMapArtifact reads and validates a persisted stdlib map from r: the
// bounded decode, the semantic invariants, and the format-version check, in
// one call.
func DecodeMapArtifact(r io.Reader) (*gen.StdlibMap, error) {
	return decodeMapArtifact(r)
}

// NativeTargetConfig discovers the host toolchain's current target
// configuration for native map generation (task req 5's default native
// discovery).
func NativeTargetConfig() (TargetConfig, error) {
	return nativeTargetConfig()
}

// RunGenerateCommand implements the shared `stdlibmap generate` command used
// by the full CLI and the thin Bazel generator.
func RunGenerateCommand(args []string, stdout, stderr io.Writer) int {
	return runGenerateCommand(args, stdout, stderr)
}

// RunProjectExportCommand implements the explicit-input stdlib export
// projection used by the single shared Bazel producer.
func RunProjectExportCommand(args []string, stdout, stderr io.Writer) int {
	return runProjectExportCommand(args, stdout, stderr)
}

// RenderTargetConfig renders the deterministic configuration-parameter file
// the explicit-input mode passes around: six fixed key=value lines in a fixed
// order, so analysis-time bytes are reproducible (task req 7).
func RenderTargetConfig(t TargetConfig) (string, error) {
	return renderTargetConfig(t)
}

// TargetEnv renders the target environment overrides that make the Go
// toolchain build for the target configuration (task reqs 8–9).
func TargetEnv(t TargetConfig) []string {
	return targetEnv(t)
}

// NativeEnvironment returns the complete environment every toolchain
// invocation of a native generation must run in: the host environment with
// the target overrides merged.
func NativeEnvironment(target TargetConfig) []string {
	return (&NativeLoader{Env: TargetEnv(target)}).Environment()
}

// NativeLoader loads importable packages from the target toolchain's SDK
// sources with x/tools go/packages (task req 4).
type NativeLoader struct {
	// Env holds the target environment overrides (TargetEnv).
	Env []string
}

// Environment returns the complete environment the loader runs the toolchain
// in: the host environment with the target overrides merged.
func (l *NativeLoader) Environment() []string {
	return nativeLoaderEnvironment(l)
}

// Load implements the batch Loader seam: every path is loaded for the target
// configuration; any load or type error, or any path missing from the
// result, fails the batch with an actionable error.
func (l *NativeLoader) Load(paths []string) (map[string]*types.Package, error) {
	return nativeLoaderLoad(l, paths)
}

// isInternalPath reports whether any import-path segment of path is
// `internal` (task req 3). Such packages are enumerated but never loaded or
// inventoried.
func isInternalPath(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == "internal" {
			return true
		}
	}
	return false
}

// explicitPackageList returns a PackageOracle backed by a Bazel-supplied
// toolchain package list (task req 2): the host passes the pinned toolchain's
// package paths, in any order. Normalization happens at build-inventory time,
// so malformed entries fail generation with an actionable error rather than
// being silently dropped here.
func explicitPackageList(paths []string) PackageOracle {
	return PackageOracleFunc(func() ([]PackageEntry, error) {
		entries := make([]PackageEntry, len(paths))
		for i, p := range paths {
			entries[i] = PackageEntry{Path: p, Importable: !isInternalPath(p)}
		}
		return entries, nil
	})
}

// normalizePackageList canonicalizes an oracle's raw enumeration into the
// total, deterministic package list (task req 2): every unique path is
// emitted exactly once in lexicographic byte order, internal-segment packages
// are retained as non-importable, and malformed paths fail with an actionable
// error. Empty strings, path escapes, separators-as-segments, whitespace, and
// non-canonical path spellings are rejected so a malformed oracle output can
// never enter an artifact as a package path.
func normalizePackageList(paths []string) ([]PackageEntry, error) {
	seen := make(map[string]bool, len(paths))
	entries := make([]PackageEntry, 0, len(paths))
	for _, p := range paths {
		if err := validatePackagePath(p); err != nil {
			return nil, err
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		entries = append(entries, PackageEntry{Path: p, Importable: !isInternalPath(p)})
	}
	slices.SortFunc(entries, func(a, b PackageEntry) int { return strings.Compare(a.Path, b.Path) })
	if err := hostpolicy.ValidateStdlibPaths(pathsOf(entries)); err != nil {
		return nil, fmt.Errorf("normalizing the package list: %w", err)
	}
	return entries, nil
}

func pathsOf(entries []PackageEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths
}

// validatePackagePath rejects import paths outside the grammar the persisted
// package inventory admits: non-empty, slash-separated, no empty, dot,
// dotdot or whitespace-containing segments, only Go import-path characters
// (letters, digits, '-', '.', '_'), and no leading, trailing or repeated
// separators. The host-policy canonicalization fixed-point invariant is then
// enforced with hostpolicy.ValidateCanonicalPath, so a path the canonical
// SymbolID grammar or a host rewriter would reject fails here, before any
// inventory work.
func validatePackagePath(path string) error {
	if path == "" {
		return fmt.Errorf("package path %q is not a canonical import path: empty path", path)
	}
	if strings.ContainsAny(path, " \t\n\r\\") {
		return fmt.Errorf("package path %q is not a canonical import path (whitespace or backslash)", path)
	}
	if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.Contains(path, "//") {
		return fmt.Errorf("package path %q is not canonical: empty path segment", path)
	}
	for _, seg := range strings.Split(path, "/") {
		switch {
		case seg == ".", seg == "..", strings.Contains(seg, ".."):
			return fmt.Errorf("package path %q is not canonical: %q path segment", path, seg)
		}
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
				c == '-' || c == '.' || c == '_' {
				continue
			}
			return fmt.Errorf("package path %q is not canonical: %q segment contains invalid import-path character %q", path, seg, string(rune(c)))
		}
	}
	if err := hostpolicy.ValidateCanonicalPath(path); err != nil {
		return fmt.Errorf("package path %q: %w", path, err)
	}
	return nil
}

// NativeStdPackageList discovers the standard library of the Go toolchain by
// running `go list std` (task req 2, native mode). env is the COMPLETE
// environment the toolchain runs in — NativeLoader.Environment() carries the
// target overrides, so a cross-compilation enumerates the target's package
// list, not the host's; nil inherits the host environment. Output is
// whitespace-split per line, so any unsorted or repeated `go list` output is
// normalized by NormalizePackageList downstream.
func NativeStdPackageList(ctx context.Context, env []string) ([]PackageEntry, error) {
	out, err := hosttoolchain.GoListStd(ctx, env)
	if err != nil {
		return nil, err
	}
	return parseGoListOutput(out)
}

// parseGoListOutput normalizes `go list` output into the total package list.
// strings.Fields splits on every whitespace run — newlines, carriage returns
// and tabs included — so CRLF or tab-separated tool output never leaves a
// stray separator attached to a package path.
func parseGoListOutput(out string) ([]PackageEntry, error) {
	return normalizePackageList(strings.Fields(out))
}

// nativeToolchainVersion reports the exact toolchain version of the Go
// toolchain on PATH via `go env GOVERSION` (task req 7's toolchain_version
// input; task req 9's injectable seam — callers with a pinned toolchain pass
// the version directly in TargetConfig instead).
func nativeToolchainVersion(ctx context.Context) (string, error) {
	return hosttoolchain.GoVersion(ctx)
}
