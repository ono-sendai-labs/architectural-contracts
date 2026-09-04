// Package stdlibmap is the generation-side shell for the standard-library
// authority map (design §Components "New: stdlibmap"): deterministic SDK
// package discovery, the independent go/types inventory oracle the map is
// reconciled against, and the target-specific SDK key with its classifier
// fingerprint (design DR-05, DR-09, I3). Full Capslock classification and map
// emission are the follow-on generation tasks; this package owns the
// discovery, inventory, and key-derivation substrate they consume.
//
// Component Contract (FR10):
//   - What it does: Discovers the SDK's standard-library package list behind an
//     injectable oracle (native `go list std` or an explicit Bazel-supplied
//     toolchain package list), normalizes it into a total, deduplicated,
//     canonically sorted PackageEntry list with importable flags (any internal
//     path segment ⇒ non-importable), inventories every externally referencable
//     exported declaration of every importable package via go/types under the
//     declaring-object rule (including one aggregate pkg.init per importable
//     package), and derives the target-configuration SDKKey with a
//     deterministic classifier_hash.
//   - What it requires: A package oracle, a batch package Loader for the target
//     configuration, the target toolchain version and build environment, and
//     the generation classifier text plus an explicit rule-version string.
//     Every inventoried identifier must be grammar-valid under the canonical
//     SymbolID constructors.
//   - What it provides: PackageEntry, PackageOracle, NormalizePackageList,
//     ExplicitPackageList, NativeStdPackageList (target-environment aware),
//     IsInternalPath, Loader, BuildInventory with its total Inventory
//     (Packages/Symbols/Inits, sorted and duplicate-free; the Inventory is
//     also the inventory-backed Capslock normalization context —
//     symbol.CapslockInventory — for the generator's reconciliation),
//     TargetConfig, GenerationDescriptor, ClassifierRule,
//     CanonicalClassifierText, ClassifierHash, DeriveSDKKey,
//     NativeToolchainVersion, TargetEnv and NativeLoader.
//   - Ambient Authority: This is a shell generation component. It holds FILES
//     (reads SDK sources and export data through the loader), EXEC and
//     READ_SYSTEM_STATE (runs the toolchain: `go list std`, `go env
//     GOVERSION`), OPERATING_SYSTEM and MODIFY_SYSTEM_STATE/ENV (build
//     environment discovery and the target build environment it constructs),
//     REFLECT and RUNTIME (go/packages and go/types type loading), and it is
//     the substrate on which Capslock map generation (a follow-on
//     responsibility of this component) runs. No global logging or network
//     access; all discovery, version, and loading seams are injectable.
package stdlibmap

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// PackageEntry is one package of the SDK enumeration oracle's total list
// (design DR-05): its import path and whether it is importable. A package is
// non-importable exactly when any import-path segment is `internal`; it stays
// in the total package inventory but has no symbol or init inventory.
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

// IsInternalPath reports whether any import-path segment of path is
// `internal` (task req 3). Such packages are enumerated but never loaded or
// inventoried.
func IsInternalPath(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == "internal" {
			return true
		}
	}
	return false
}

// ExplicitPackageList returns a PackageOracle backed by a Bazel-supplied
// toolchain package list (task req 2): the host passes the pinned toolchain's
// package paths, in any order. Normalization happens at build-inventory time,
// so malformed entries fail generation with an actionable error rather than
// being silently dropped here.
func ExplicitPackageList(paths []string) PackageOracle {
	return PackageOracleFunc(func() ([]PackageEntry, error) {
		entries := make([]PackageEntry, len(paths))
		for i, p := range paths {
			entries[i] = PackageEntry{Path: p, Importable: !IsInternalPath(p)}
		}
		return entries, nil
	})
}

// NormalizePackageList canonicalizes an oracle's raw enumeration into the
// total, deterministic package list (task req 2): every unique path is
// emitted exactly once in lexicographic byte order, internal-segment packages
// are retained as non-importable, and malformed paths fail with an actionable
// error. Empty strings, path escapes, separators-as-segments, whitespace, and
// non-canonical path spellings are rejected so a malformed oracle output can
// never enter an artifact as a package path.
func NormalizePackageList(paths []string) ([]PackageEntry, error) {
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
		entries = append(entries, PackageEntry{Path: p, Importable: !IsInternalPath(p)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
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
	cmd := exec.CommandContext(ctx, "go", "list", "std")
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("discovering the standard library with `go list std`: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return NormalizePackageList(strings.FieldsFunc(string(out), func(r rune) bool { return r == '\n' || r == ' ' }))
}

// NativeToolchainVersion reports the exact toolchain version of the Go
// toolchain on PATH via `go env GOVERSION` (task req 7's toolchain_version
// input; task req 9's injectable seam — callers with a pinned toolchain pass
// the version directly in TargetConfig instead).
func NativeToolchainVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "GOVERSION")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("discovering the toolchain version with `go env GOVERSION`: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", fmt.Errorf("`go env GOVERSION` returned no version")
	}
	return version, nil
}
