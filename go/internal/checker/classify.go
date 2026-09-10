// Classification dispatch for the Step 6 typed reference vocabulary (R1–R6,
// DR-10, DR-14): given the scanned reference and import edges of a component,
// decide every cross-boundary edge against the exact declaring-object boundary
// (boundary.go) and the StdlibAuthority port, and turn stdlib terminal
// classifications into policy-ready authority observations.
//
// Imports and object references are classified by separate rules (DR-10).
// Standard-library membership is map enumeration (R6): no path predicate, no
// reachability, and no source inspection ever influences a decision here.
//
// Component Contract (FR10):
//   - What it does: Classifies every ReferenceEdge and ImportEdge of a
//     component into sorted boundary observations
//     (UNDECLARED_DEPENDENCY / CALLS_UNDECLARED_INTERFACE), per-capability
//     TrueAuthority observations and AnalysisDefeating observations carrying
//     the site, SymbolID (or aggregate pkg.init identity), the map's SDKKey
//     and the map's canned evidence, plus the sorted list of dependencies
//     whose surface was used by an import. Fails closed as tool errors on
//     SDK-key mismatch, inventory gaps, invalid terminal classifications, and
//     missing type/layout data for a written non-member import.
//   - What it requires: A validated MemberSet, fully resolved
//     DependencyInterface values, edges in canonical form, a
//     stdlibauthority.StdlibAuthority, and the target SDKKey to check the
//     map's key against.
//   - What it provides: ClassifyEdges, Classified, AuthorityObservation,
//     BoundaryObservation. Pure and deterministic: identical inputs produce
//     identical results or identical errors; emitted collections are sorted.
//   - Ambient Authority: This component is guaranteed-pure and holds no
//     ambient authority (no filesystem I/O, network, process execution or
//     reflection).
package checker

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// AuthorityObservation is one policy-ready capability observation produced
// from a stdlib classification. TrueAuthority observations carry exactly one
// capability with the map's canned evidence; AnalysisDefeating observations
// carry an empty Capability and no evidence. Referent is the referenced
// SymbolID for object references and the aggregate `pkg.init` identity for
// imports (R3, DR-04).
type AuthorityObservation struct {
	Class      capanalyzer.Class
	Capability string
	Referent   facts.SymbolID
	Site       facts.SourceSite
	// SDKKey is the map's own target-configuration key (N3), carried on
	// every observation so reports can pin the configuration that decided.
	SDKKey stdlibauthority.SDKKey
	// Evidence is the map's canned path for (Referent, Capability); nil for
	// AnalysisDefeating observations. Callers must not mutate it.
	Evidence []stdlibauthority.Frame
}

// BoundaryObservation is one reportable boundary violation at an exact site.
// Referent is empty for import edges (the violation names ReferentPackage).
type BoundaryObservation struct {
	Kind            report.Kind
	FromPackage     string
	ReferentPackage string
	Referent        facts.SymbolID
	Site            facts.SourceSite
	// Dependency is the owning dependency component for interface
	// violations; empty for undeclared dependencies.
	Dependency string
}

// Classified is the classification result: reportable observations ready for
// the report's policy application (FR6), and the dependencies an import used
// (including auto-attached infra dependencies, R13). Every collection is
// sorted and duplicate-free.
type Classified struct {
	Boundary         []BoundaryObservation
	Authority        []AuthorityObservation
	UsedDependencies []string
}

// ClassifyEdges classifies the component's scanned references and imports
// against its membership, its resolved direct dependencies, and the stdlib
// authority map. It fails closed: the first tool error (SDK-key mismatch,
// inventory gap, invalid classification, or missing type data for a written
// non-member import) aborts the whole operation and no partial verdict is
// produced (DR-14).
func ClassifyEdges(
	members facts.MemberSet,
	deps []facts.DependencyInterface,
	refs []facts.ReferenceEdge,
	imports []facts.ImportEdge,
	authority stdlibauthority.StdlibAuthority,
	expected stdlibauthority.SDKKey,
) (Classified, error) {
	if authority == nil {
		return Classified{}, fmt.Errorf("stdlib authority is nil")
	}
	if fields := stdlibauthority.EqualKeys(authority.Key(), expected); len(fields) > 0 {
		return Classified{}, &stdlibauthority.KeyMismatchError{Fields: fields}
	}
	index, err := NewBoundaryIndex(members, deps)
	if err != nil {
		return Classified{}, err
	}

	var out Classified
	used := map[string]bool{}

	for _, e := range facts.SortReferenceEdges(refs) {
		if err := e.Validate(); err != nil {
			return Classified{}, err
		}
		if members.Contains(e.ReferentPackage) {
			continue
		}
		if authority.IsStdlibPackage(e.ReferentPackage) {
			observations, err := classifySymbol(authority, e.Referent, e.Site)
			if err != nil {
				return Classified{}, fmt.Errorf("reference %s at %s:%d: %w", e.Referent, e.Site.File, e.Site.Line, err)
			}
			out.Authority = append(out.Authority, observations...)
			continue
		}
		decision, err := index.Classify(e)
		if err != nil {
			return Classified{}, err
		}
		if kind := decision.ViolationKind(); kind != "" {
			out.Boundary = append(out.Boundary, BoundaryObservation{
				Kind:            report.Kind(kind),
				FromPackage:     e.FromPackage,
				ReferentPackage: e.ReferentPackage,
				Referent:        e.Referent,
				Site:            e.Site,
				Dependency:      decision.Dependency,
			})
		}
	}

	for _, e := range facts.SortImportEdges(imports) {
		if err := e.Validate(); err != nil {
			return Classified{}, err
		}
		if members.Contains(e.ImportPath) {
			continue
		}
		if e.Resolution == facts.ImportMissingTypeData {
			return Classified{}, missingImportDataError(e)
		}
		if e.Resolution == facts.ImportUnresolved &&
			(authority.IsStdlibPackage(e.ImportPath) || index.ownsPackage(e.ImportPath)) {
			// A map-known stdlib path or a declared dependency path is
			// structurally expected to have a package node. If the loader
			// cannot resolve it, fail closed instead of letting the map or
			// boundary lookup turn missing data into a pass (DR-14).
			return Classified{}, missingImportDataError(e)
		}
		if authority.IsStdlibPackage(e.ImportPath) {
			observations, err := classifyPackageInit(authority, e.ImportPath, e.Site)
			if err != nil {
				return Classified{}, fmt.Errorf("import of %q at %s:%d: %w", e.ImportPath, e.Site.File, e.Site.Line, err)
			}
			out.Authority = append(out.Authority, observations...)
			continue
		}
		if dep, owns := index.owningDependency(e.ImportPath); owns {
			// Declared and auto-attached dependencies alike are accepted
			// behind the component boundary; the dependency counts as used
			// (R13).
			used[dep] = true
			continue
		}
		out.Boundary = append(out.Boundary, BoundaryObservation{
			Kind:            report.UndeclaredDependency,
			FromPackage:     e.ImportingPackage,
			ReferentPackage: e.ImportPath,
			Site:            e.Site,
		})
	}

	for dep := range used {
		out.UsedDependencies = append(out.UsedDependencies, dep)
	}
	sort.Strings(out.UsedDependencies)
	out.Authority = sortAuthorityObservations(out.Authority)
	out.Boundary = sortBoundaryObservations(out.Boundary)
	return out, nil
}

func missingImportDataError(e facts.ImportEdge) error {
	return fmt.Errorf("import of %q at %s:%d has no type or layout data; the export-data/layout contract is broken", e.ImportPath, e.Site.File, e.Site.Line)
}

// classifySymbol resolves one stdlib object reference through the port's
// exact total-map lookup (R6) and converts its terminal classification into
// observations (DR-05): one TrueAuthority observation per capability, one
// AnalysisDefeating observation for UNANALYZED, nothing for SAFE.
func classifySymbol(authority stdlibauthority.StdlibAuthority, id facts.SymbolID, site facts.SourceSite) ([]AuthorityObservation, error) {
	class, err := authority.SymbolAuthority(id)
	if err != nil {
		return nil, err
	}
	if err := class.Validate(); err != nil {
		return nil, fmt.Errorf("stdlib record for %s is not a valid terminal classification: %w", id, err)
	}
	key := authority.Key()
	switch {
	case class.Safe:
		return nil, nil
	case class.Unanalyzed:
		return []AuthorityObservation{{
			Class:    capanalyzer.AnalysisDefeating,
			Referent: id,
			Site:     site,
			SDKKey:   key,
		}}, nil
	default:
		obs := make([]AuthorityObservation, 0, len(class.Capabilities))
		for _, cap := range class.Capabilities {
			obs = append(obs, AuthorityObservation{
				Class:      capanalyzer.TrueAuthority,
				Capability: cap,
				Referent:   id,
				Site:       site,
				SDKKey:     key,
				Evidence:   authority.Evidence(id, cap),
			})
		}
		return obs, nil
	}
}

// classifyPackageInit resolves one import of a map-enumerated package
// through the port's package-init lookup (R3) and converts the aggregate
// init's terminal classification into observations under the `pkg.init`
// identity at the import site.
func classifyPackageInit(authority stdlibauthority.StdlibAuthority, pkgPath string, site facts.SourceSite) ([]AuthorityObservation, error) {
	class, err := authority.PackageInitAuthority(pkgPath)
	if err != nil {
		return nil, err
	}
	if err := class.Validate(); err != nil {
		return nil, fmt.Errorf("stdlib init record for %q is not a valid terminal classification: %w", pkgPath, err)
	}
	id, err := symbol.Parse(pkgPath + ".init")
	if err != nil {
		return nil, fmt.Errorf("stdlib package %q: %w", pkgPath, err)
	}
	key := authority.Key()
	switch {
	case class.Safe:
		return nil, nil
	case class.Unanalyzed:
		return []AuthorityObservation{{
			Class:    capanalyzer.AnalysisDefeating,
			Referent: id,
			Site:     site,
			SDKKey:   key,
		}}, nil
	default:
		obs := make([]AuthorityObservation, 0, len(class.Capabilities))
		for _, cap := range class.Capabilities {
			obs = append(obs, AuthorityObservation{
				Class:      capanalyzer.TrueAuthority,
				Capability: cap,
				Referent:   id,
				Site:       site,
				SDKKey:     key,
				Evidence:   authority.Evidence(id, cap),
			})
		}
		return obs, nil
	}
}

// owningDependency returns the component owning pkg, or false when no direct
// dependency claims it.
func (b *BoundaryIndex) owningDependency(pkg string) (string, bool) {
	dep, owns := b.pkgToDep[pkg]
	if !owns {
		return "", false
	}
	return dep.Component, true
}

// sortAuthorityObservations orders observations deterministically: by site,
// then referent, then class, then capability.
func sortAuthorityObservations(obs []AuthorityObservation) []AuthorityObservation {
	slices.SortStableFunc(obs, func(a, b AuthorityObservation) int {
		if c := facts.CompareSourceSite(a.Site, b.Site); c != 0 {
			return c
		}
		if c := symbol.Compare(a.Referent, b.Referent); c != 0 {
			return c
		}
		if a.Class != b.Class {
			return strings.Compare(string(a.Class), string(b.Class))
		}
		return strings.Compare(a.Capability, b.Capability)
	})
	return obs
}

// sortBoundaryObservations orders observations deterministically: by kind,
// referent package, referent, site, then dependency.
func sortBoundaryObservations(obs []BoundaryObservation) []BoundaryObservation {
	slices.SortStableFunc(obs, func(a, b BoundaryObservation) int {
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}
		if c := strings.Compare(a.ReferentPackage, b.ReferentPackage); c != 0 {
			return c
		}
		if c := symbol.Compare(a.Referent, b.Referent); c != 0 {
			return c
		}
		if c := facts.CompareSourceSite(a.Site, b.Site); c != 0 {
			return c
		}
		return strings.Compare(a.Dependency, b.Dependency)
	})
	return obs
}
