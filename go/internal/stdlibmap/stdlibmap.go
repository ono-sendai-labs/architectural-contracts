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
//     ExplicitPackageList, NativeStdPackageList, IsInternalPath, Loader,
//     BuildInventory with its total Inventory (Packages/Symbols/Inits, sorted
//     and duplicate-free), TargetConfig, GenerationDescriptor, ClassifierRule,
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
	return entries, nil
}

// validatePackagePath rejects import paths outside the grammar the persisted
// package inventory admits: non-empty, slash-separated, no empty, dot, dotdot
// or whitespace-containing segments, and no leading, trailing or repeated
// separators. Non-canonical spellings (backslashes) are rejected with the
// same authority.
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
		switch seg {
		case ".", "..":
			return fmt.Errorf("package path %q is not canonical: %q path segment", path, seg)
		}
	}
	return nil
}

// NativeStdPackageList discovers the standard library of the Go toolchain on
// PATH by running `go list std` (task req 2, native mode). Output is
// whitespace-split per line, so any unsorted or repeated `go list` output is
// normalized by NormalizePackageList downstream.
func NativeStdPackageList(ctx context.Context) ([]PackageEntry, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "std")
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
