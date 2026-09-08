// Package surface derives persisted surface manifests (R7) from a
// component's own manifest facts: parsed manifest inputs, package
// layout/member facts, the exact Step 3 SymbolID extraction, target SDK
// identity, namespace, and source bytes.
//
// The package is pure data-model work: Derive consumes already-loaded values
// and performs no filesystem, network, or process access. Filesystem reads
// are isolated in the artifactio shell adapter (ReadSources), and the wiring
// into `arcc check` lands with Step 5 task 3.
//
// Deliberate Step 5-only disagreement: the emitted surface is the exact
// declared interface (symbol.ExtractSurface over the surviving interface
// files, no implements-closure injection), while the check keeps resolving
// dependency interfaces through goanalysis.ResolveDependencyInterface's
// implements closure until Step 6. A concrete method admitted only by that
// workaround is therefore checked as callable but absent from the emitted
// surface for this one step; see the comment on ResolveDependencyInterface.
//
// Component Contract (FR10):
//   - What it does: Derives the persisted SurfaceManifest from fully explicit
//     inputs — component identity, interface style, structural authority,
//     namespace, complete target SDK key, producer version, canonical member
//     packages, the exact declared symbols (symbol.ExtractSurface over the
//     surviving interface files), and digest inputs — failing closed on
//     incomplete SDK identity or non-canonical input, and computes the DR-03
//     digest over member source bytes, manifest bytes, format version,
//     namespace, SDK key and producer version.
//   - What it requires: Already-loaded values (parsed manifest facts, typed
//     interface files, member source bytes); nothing is read or resolved here.
//   - What it provides: Derive, the Input/SourceFile emission boundary, and a
//     deterministic, canonical result (sorted, duplicate-free collections,
//     populated digest). Nothing here performs I/O or touches the check path;
//     filesystem reads live in the artifactio shell adapter (ReadSources).
//   - Ambient Authority: This component is guaranteed-pure and holds no
//     ambient authority (no filesystem I/O, network, process execution or
//     reflection).
package surface

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// SourceFile is one member source file's bytes keyed by canonical path. Only
// member sources are ever supplied, so the digest cannot observe dependency,
// test, or unrelated files (task req 5).
type SourceFile struct {
	Path  string
	Bytes []byte
}

// Input is the complete, explicit emission boundary: everything Derive needs,
// nothing more. All collections may arrive in any enumeration order; output
// ordering is canonical.
type Input struct {
	// Component is the manifest's logical component name.
	Component string
	// Style selects declared-interface (InterfaceStyleUnspecified) or
	// PACKAGE_SURFACE emission.
	Style manifest.InterfaceStyle
	// Authority is the manifest's structural authority declaration (R10/R11).
	Authority manifest.AuthorityDeclaration
	// Namespace is the hostpolicy.NamespaceID of the emitter.
	Namespace string
	// Key is the complete target SDK identity; incomplete keys fail closed.
	Key *stdlibauthority.SDKKey
	// ProducerVersion identifies the producing tool; audit information.
	ProducerVersion string
	// MemberPackages are the component's owned concrete member packages in
	// the emitter's namespace — never patterns, never the dependency closure.
	MemberPackages []string
	// Manifest is the component manifest's bytes; a digest input.
	Manifest []byte
	// InterfaceFiles are the ASTs of the surviving interface files
	// (build-constraint exclusions already applied). Declared-interface
	// style only; must be nil for PACKAGE_SURFACE.
	InterfaceFiles []*ast.File
	// InterfaceInfo is the full package type info covering the
	// InterfaceFiles declarations. Declared-interface style only.
	InterfaceInfo *types.Info
	// Sources are the member source bytes; digest inputs only.
	Sources []SourceFile
}

// Derive assembles and returns the schema SurfaceManifest for the inputs.
// The result is canonical: packages and symbols sorted and duplicate-free,
// with the digest populated per DR-03. Emission is deterministic: identical
// complete inputs yield identical manifests in any enumeration order.
func Derive(in Input) (*gen.SurfaceManifest, error) {
	if in.Component == "" {
		return nil, fmt.Errorf("surface derivation: component name is empty")
	}
	if in.Namespace == "" {
		return nil, fmt.Errorf("surface derivation: namespace is empty")
	}
	if in.ProducerVersion == "" {
		return nil, fmt.Errorf("surface derivation: producer version is empty")
	}
	if err := validateAuthority(in.Authority); err != nil {
		return nil, err
	}
	sdkKey, err := sdkKeyProto(in.Key)
	if err != nil {
		return nil, err
	}

	var style gen.InterfaceStyle
	switch in.Style {
	case manifest.InterfaceStyleUnspecified:
		style = gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED
	case manifest.InterfaceStylePackageSurface:
		style = gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE
		if in.InterfaceFiles != nil || in.InterfaceInfo != nil {
			return nil, fmt.Errorf("surface derivation: PACKAGE_SURFACE components carry no interface files")
		}
	default:
		return nil, fmt.Errorf("surface derivation: unknown interface style: %d", int(in.Style))
	}

	var symbols []string
	if in.Style == manifest.InterfaceStyleUnspecified {
		if in.InterfaceInfo == nil {
			return nil, fmt.Errorf("surface derivation: declared-interface component requires type-checked interface files")
		}
		symbols = symbolIDs(symbol.ExtractSurface(in.InterfaceFiles, in.InterfaceInfo))
	}

	m := &gen.SurfaceManifest{
		FormatVersion:   surfaceFormatVersion,
		Component:       in.Component,
		InterfaceStyle:  style,
		Authority:       authorityProto(in.Authority),
		Packages:        canonicalPaths(in.MemberPackages),
		Symbols:         symbols,
		Namespace:       in.Namespace,
		SdkKey:          sdkKey,
		ProducerVersion: in.ProducerVersion,
	}
	digest, err := computeDigest(in.Sources, in.Manifest, m)
	if err != nil {
		return nil, err
	}
	m.Digest = digest
	return m, nil
}

// surfaceFormatVersion mirrors artifactio.SurfaceFormatVersion without
// importing the shell package from the pure core.
const surfaceFormatVersion = 1

// sdkKeyProto validates the target key's completeness and converts it to the
// persisted form. A consumer of the emitted surface fails closed on key
// mismatch (DR-09, N3), so an incomplete key must never be emitted.
func sdkKeyProto(k *stdlibauthority.SDKKey) (*gen.SDKKey, error) {
	if k == nil {
		return nil, fmt.Errorf("surface derivation: target SDK key is missing")
	}
	var missing []string
	if k.ToolchainVersion == "" {
		missing = append(missing, "toolchain_version")
	}
	if k.GOOS == "" {
		missing = append(missing, "goos")
	}
	if k.GOARCH == "" {
		missing = append(missing, "goarch")
	}
	if k.ClassifierHash == "" {
		missing = append(missing, "classifier_hash")
	}
	if k.MapFormatVersion == 0 {
		missing = append(missing, "map_format_version")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("surface derivation: target SDK key is incomplete, missing: %s", strings.Join(missing, ", "))
	}
	// Canonicalize build tags regardless of input enumeration order; a
	// duplicate tag is non-canonical input and fails closed here rather than
	// surfacing later at the encoder boundary.
	tags := slices.Clone(k.BuildTags)
	slices.Sort(tags)
	if i, seen := firstDuplicate(tags); seen {
		return nil, fmt.Errorf("surface derivation: target SDK key has duplicate build tag %q", tags[i])
	}
	return &gen.SDKKey{
		ToolchainVersion: k.ToolchainVersion,
		Goos:             k.GOOS,
		Goarch:           k.GOARCH,
		CgoEnabled:       k.CgoEnabled,
		BuildTags:        tags,
		Goexperiment:     k.GOEXPERIMENT,
		ClassifierHash:   k.ClassifierHash,
		MapFormatVersion: k.MapFormatVersion,
	}, nil
}

// firstDuplicate returns the index of the first duplicate entry in a sorted
// slice, or (-1, false) when all entries are distinct.
func firstDuplicate(sorted []string) (int, bool) {
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			return i, true
		}
	}
	return -1, false
}

// authorityProto converts the manifest's structural declaration to the
// persisted form, preserving the UNKNOWN axis structurally (R11).
func authorityProto(d manifest.AuthorityDeclaration) *gen.AuthorityDeclaration {
	if !d.Known {
		return &gen.AuthorityDeclaration{Authority: gen.Authority_UNKNOWN}
	}
	return &gen.AuthorityDeclaration{
		Authority:         gen.Authority_DECLARED,
		DeclaredAuthority: slices.Clone(d.Set),
	}
}

// validateAuthority rejects the forbidden spellings before conversion.
func validateAuthority(d manifest.AuthorityDeclaration) error {
	if !d.Known && len(d.Set) > 0 {
		return fmt.Errorf("surface derivation: UNKNOWN authority carries %d declared capabilities; declared_authority must be empty when authority is UNKNOWN", len(d.Set))
	}
	return nil
}

// canonicalPaths sorts and de-duplicates concrete member package paths.
// Enumeration order must not reach the artifact (N4).
func canonicalPaths(paths []string) []string {
	out := slices.Clone(paths)
	slices.Sort(out)
	return slices.Compact(out)
}

// symbolIDs converts the extractor's output to the persisted string form.
// ExtractSurface already returns the IDs sorted and duplicate-free.
func symbolIDs(ids []symbol.SymbolID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
