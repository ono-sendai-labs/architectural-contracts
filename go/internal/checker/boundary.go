// Boundary classification for the Step 6 typed reference vocabulary (R1,
// R4, R14; design Edge classification). This is the pure half of the FR5
// decision: given a ReferenceEdge produced by the typed scan, decide whether
// the member source may name the declaring object it names.
//
// The stdlib dispatch is deliberately absent here: standard-library
// membership and authority are the StdlibAuthority port's decision (Task 03),
// so this index answers only the dependency-boundary rows of the design's
// reference table.
//
// Component Contract (FR10):
//   - What it does: Builds a package-to-dependency lookup over resolved
//     direct dependencies (rejecting package overlap deterministically with a
//     DEPENDENCY_OVERLAP tool error before any lookup exists), and classifies
//     a ReferenceEdge as intra-component, authorized by a declared
//     dependency's exact declaring-object symbols, authorized by a
//     PACKAGE_SURFACE dependency's owned packages, or a boundary violation
//     (CALLS_UNDECLARED_INTERFACE / UNDECLARED_DEPENDENCY) at the exact site.
//   - What it requires: A validated MemberSet and fully resolved
//     DependencyInterface values; ReferenceEdge values in canonical form.
//   - What it provides: NewBoundaryIndex, Classify (with dependency
//     provenance so declared and auto-attached dependency edges count as
//     uses), ReferenceStatus, ReferenceDecision. Pure and deterministic;
//     identical inputs produce identical decisions and identical errors.
//   - Ambient Authority: This component is guaranteed-pure and holds no
//     ambient authority (no filesystem I/O, network, process execution or
//     reflection).
package checker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// ReferenceStatus is the outcome of classifying one reference edge against
// the component boundary.
//
// Exported in a non-interface file only for goanalysis fixture tests
// (pending test rewrite follow-up).
type ReferenceStatus string

const (
	// referenceIntraComponent: the referent is a member object; not a
	// boundary edge.
	referenceIntraComponent ReferenceStatus = "intra_component"
	// ReferenceDeclaredDependency: the referent is authorized by a direct
	// dependency — either any object of a PACKAGE_SURFACE dependency's owned
	// packages, or an exact declaring-object SymbolID of a declared
	// interface.
	//
	// Exported in a non-interface file only for goanalysis fixture tests
	// (pending test rewrite follow-up).
	ReferenceDeclaredDependency ReferenceStatus = "declared_dependency"
	// ReferenceCallsUndeclaredInterface: the owning dependency is a declared
	// interface and the declaring object is not in its symbol set.
	//
	// Exported in a non-interface file only for goanalysis fixture tests
	// (pending test rewrite follow-up).
	ReferenceCallsUndeclaredInterface ReferenceStatus = "calls_undeclared_interface"
	// ReferenceUndeclaredDependency: no direct dependency owns the referent
	// package.
	//
	// Exported in a non-interface file only for goanalysis fixture tests
	// (pending test rewrite follow-up).
	ReferenceUndeclaredDependency ReferenceStatus = "undeclared_dependency"
)

// referenceDecision is the classification of one reference edge. Dependency
// is the owning component's name whenever the edge reaches a dependency,
// including violating edges, so edge provenance (and per-dependency use
// counting) survives the decision.
type referenceDecision struct {
	Status     ReferenceStatus
	Dependency string
}

// ViolationKind returns the report.Kind spelling for a violating decision,
// or "" when the edge is not a violation.
//
// Exported in a non-interface file only for goanalysis fixture tests
// (pending test rewrite follow-up).
func (d referenceDecision) ViolationKind() string {
	switch d.Status {
	case ReferenceCallsUndeclaredInterface:
		return "CALLS_UNDECLARED_INTERFACE"
	case ReferenceUndeclaredDependency:
		return "UNDECLARED_DEPENDENCY"
	default:
		return ""
	}
}

// boundaryIndex is the precomputed package-to-dependency lookup over the
// resolved direct dependencies. Construct it once per check; construction
// fails closed on overlapping ownership (R14).
type boundaryIndex struct {
	members  facts.MemberSet
	pkgToDep map[string]*facts.DependencyInterface
}

// NewBoundaryIndex builds the index. If two resolved direct dependencies
// claim the same package, construction fails with a deterministic
// DEPENDENCY_OVERLAP tool error naming the package and both components —
// lexicographically first by package, so the reported collision (and the
// error text) is byte-identical in any dependency order (R14). The legacy
// last-wins behavior must not return.
//
// Exported in a non-interface file only for goanalysis fixture tests
// (pending test rewrite follow-up).
func NewBoundaryIndex(members facts.MemberSet, deps []facts.DependencyInterface) (*boundaryIndex, error) {
	owners := make(map[string]string)
	var collisions []overlap
	for i := range deps {
		di := &deps[i]
		for _, pkg := range di.Packages {
			prev, exists := owners[pkg]
			if exists && prev != di.Component {
				first, second := sortedPair(prev, di.Component)
				collisions = append(collisions, overlap{pkg: pkg, first: first, second: second})
				continue
			}
			owners[pkg] = di.Component
		}
	}
	if len(collisions) > 0 {
		slices.SortFunc(collisions, func(a, b overlap) int {
			if c := strings.Compare(a.pkg, b.pkg); c != 0 {
				return c
			}
			if c := strings.Compare(a.first, b.first); c != 0 {
				return c
			}
			return strings.Compare(a.second, b.second)
		})
		c := collisions[0]
		return nil, fmt.Errorf("DEPENDENCY_OVERLAP: package %q is claimed by dependencies %q and %q", c.pkg, c.first, c.second)
	}
	pkgToDep := make(map[string]*facts.DependencyInterface)
	for i := range deps {
		di := &deps[i]
		for _, pkg := range di.Packages {
			pkgToDep[pkg] = di
		}
	}
	return &boundaryIndex{members: members, pkgToDep: pkgToDep}, nil
}

// overlap is one cross-component package collision, normalized so the pair
// order cannot depend on the input order.
type overlap struct {
	pkg    string
	first  string
	second string
}

// sortedPair returns its two inputs in byte order, so an overlap error is
// byte-identical regardless of which dependency was seen first.
func sortedPair(a, b string) (string, string) {
	if strings.Compare(a, b) > 0 {
		return b, a
	}
	return a, b
}

// Classify decides one reference edge. A malformed edge is an error, never a
// silent pass.
//
// Exported in a non-interface file only for goanalysis fixture tests
// (pending test rewrite follow-up).
func (b *boundaryIndex) Classify(edge facts.ReferenceEdge) (referenceDecision, error) {
	if err := edge.Validate(); err != nil {
		return referenceDecision{}, err
	}
	if b.members.Contains(edge.ReferentPackage) {
		return referenceDecision{Status: referenceIntraComponent}, nil
	}
	dep, owns := b.pkgToDep[edge.ReferentPackage]
	if !owns {
		return referenceDecision{Status: ReferenceUndeclaredDependency}, nil
	}
	if dep.InterfaceStyle == manifest.InterfaceStylePackageSurface {
		return referenceDecision{Status: ReferenceDeclaredDependency, Dependency: dep.Component}, nil
	}
	symbols := make(map[facts.SymbolID]bool, len(dep.Symbols))
	for _, sym := range dep.Symbols {
		symbols[sym] = true
	}
	if !symbols[edge.Referent] {
		return referenceDecision{
			Status:     ReferenceCallsUndeclaredInterface,
			Dependency: dep.Component,
		}, nil
	}
	return referenceDecision{Status: ReferenceDeclaredDependency, Dependency: dep.Component}, nil
}

// ownsPackage reports whether a direct dependency claims pkg. The distinction
// between an unowned unresolved import and a missing dependency package is made
// by the import classifier, after this index has already rejected overlap.
func (b *boundaryIndex) ownsPackage(pkg string) bool {
	_, ok := b.pkgToDep[pkg]
	return ok
}
