package checker_test

import (
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

func depInterface(name string, style manifest.InterfaceStyle, pkgs []string, syms []facts.SymbolID) facts.DependencyInterface {
	return facts.DependencyInterface{
		Component:      name,
		InterfaceStyle: style,
		Packages:       pkgs,
		Symbols:        syms,
	}
}

func refEdge(kind facts.ReferenceKind, from, referentPkg, referent string, line int) facts.ReferenceEdge {
	return facts.ReferenceEdge{
		Kind:            kind,
		FromPackage:     from,
		ReferentPackage: referentPkg,
		Referent:        facts.SymbolID(referent),
		Site:            facts.SourceSite{File: "member/api.go", Line: line},
	}
}

func TestBoundaryIndex_Classify(t *testing.T) {
	const (
		member = "example.com/comp/member"
		depPkg = "example.com/dep"
	)

	ms, err := facts.NewMemberSet(member)
	if err != nil {
		t.Fatalf("member set: %v", err)
	}

	declared := depInterface("greeter", manifest.InterfaceStyleUnspecified, []string{depPkg}, []facts.SymbolID{
		facts.SymbolID(depPkg + ".Greeter"),
		facts.SymbolID("(" + depPkg + ".Greeter).Greet"),
	})
	surface := depInterface("infra", manifest.InterfaceStylePackageSurface, []string{"example.com/infra"}, nil)

	index, err := checker.NewBoundaryIndex(ms, []facts.DependencyInterface{declared, surface})
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}

	cases := []struct {
		name     string
		edge     facts.ReferenceEdge
		want     checker.ReferenceStatus
		wantDep  string
		wantText string
	}{
		{
			name: "intra-component reference is ignored",
			edge: refEdge(facts.RefFunc, member, member, member+".Local", 3),
			want: checker.ReferenceIntraComponent,
		},
		{
			name:    "declared interface method by declaring-object id",
			edge:    refEdge(facts.RefMethod, member, depPkg, "("+depPkg+".Greeter).Greet", 7),
			want:    checker.ReferenceDeclaredDependency,
			wantDep: "greeter",
		},
		{
			name:    "declared interface type reference",
			edge:    refEdge(facts.RefType, member, depPkg, depPkg+".Greeter", 8),
			want:    checker.ReferenceDeclaredDependency,
			wantDep: "greeter",
		},
		{
			name:     "concrete implementor method is rejected",
			edge:     refEdge(facts.RefMethod, member, depPkg, "("+depPkg+".Impl).Greet", 9),
			want:     checker.ReferenceCallsUndeclaredInterface,
			wantDep:  "greeter",
			wantText: "CALLS_UNDECLARED_INTERFACE",
		},
		{
			name:     "undeclared external package",
			edge:     refEdge(facts.RefFunc, member, "example.com/other", "example.com/other.F", 11),
			want:     checker.ReferenceUndeclaredDependency,
			wantText: "UNDECLARED_DEPENDENCY",
		},
		{
			name:    "package-surface reference is authorized with provenance",
			edge:    refEdge(facts.RefFunc, member, "example.com/infra", "example.com/infra.Do", 13),
			want:    checker.ReferenceDeclaredDependency,
			wantDep: "infra",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := index.Classify(tc.edge)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision.Status != tc.want {
				t.Errorf("status = %q, want %q", decision.Status, tc.want)
			}
			if decision.Dependency != tc.wantDep {
				t.Errorf("dependency provenance = %q, want %q", decision.Dependency, tc.wantDep)
			}
			if tc.wantText != "" && !strings.Contains(decision.ViolationKind(), tc.wantText) {
				t.Errorf("violation kind = %q, want it to contain %q", decision.ViolationKind(), tc.wantText)
			}
		})
	}
}

func TestBoundaryIndex_DependencyOverlap(t *testing.T) {
	ms, err := facts.NewMemberSet("example.com/comp/member")
	if err != nil {
		t.Fatalf("member set: %v", err)
	}
	first := depInterface("alpha", manifest.InterfaceStyleUnspecified, []string{"example.com/shared"}, nil)
	second := depInterface("beta", manifest.InterfaceStyleUnspecified, []string{"example.com/shared"}, nil)

	_, err = checker.NewBoundaryIndex(ms, []facts.DependencyInterface{first, second})
	if err == nil {
		t.Fatalf("expected a DEPENDENCY_OVERLAP error, got nil")
	}
	if !strings.Contains(err.Error(), "DEPENDENCY_OVERLAP") || !strings.Contains(err.Error(), "example.com/shared") ||
		!strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "beta") {
		t.Errorf("overlap error must name the tool kind, package and both components, got: %v", err)
	}

	// The error is deterministic and independent of dependency order.
	_, err2 := checker.NewBoundaryIndex(ms, []facts.DependencyInterface{second, first})
	if err2 == nil || err2.Error() != err.Error() {
		t.Errorf("overlap error must be order-independent:\n%v\n%v", err, err2)
	}

	// A dependency repeating its own package is not an overlap.
	dup := depInterface("alpha", manifest.InterfaceStyleUnspecified, []string{"example.com/shared", "example.com/shared"}, nil)
	if _, err := checker.NewBoundaryIndex(ms, []facts.DependencyInterface{dup}); err != nil {
		t.Errorf("repeated package within one dependency must not be an overlap: %v", err)
	}
}

func TestBoundaryIndex_CountsUsesWithProvenance(t *testing.T) {
	const (
		member = "example.com/comp/member"
		depPkg = "example.com/dep"
		infra  = "example.com/infra"
	)
	ms, err := facts.NewMemberSet(member)
	if err != nil {
		t.Fatalf("member set: %v", err)
	}
	declared := depInterface("greeter", manifest.InterfaceStyleUnspecified, []string{depPkg}, []facts.SymbolID{
		facts.SymbolID(depPkg + ".Greeter"),
	})
	// Auto-attached infra dependency: authorized exactly like a declared one
	// for reference purposes; the provenance keeps the components distinct.
	attached := depInterface("infra", manifest.InterfaceStylePackageSurface, []string{infra}, nil)

	index, err := checker.NewBoundaryIndex(ms, []facts.DependencyInterface{declared, attached})
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}

	edges := []facts.ReferenceEdge{
		refEdge(facts.RefType, member, depPkg, depPkg+".Greeter", 3),
		refEdge(facts.RefType, member, depPkg, depPkg+".Greeter", 8),
		refEdge(facts.RefFunc, member, infra, infra+".Run", 5),
	}
	uses := map[string]int{}
	for _, e := range edges {
		decision, err := index.Classify(e)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if decision.Status == checker.ReferenceDeclaredDependency {
			uses[decision.Dependency]++
		}
	}
	if uses["greeter"] != 2 || uses["infra"] != 1 {
		t.Errorf("edge uses with provenance = %v, want greeter:2 infra:1", uses)
	}
}

func TestBoundaryIndex_MalformedEdge(t *testing.T) {
	ms, err := facts.NewMemberSet("example.com/comp/member")
	if err != nil {
		t.Fatalf("member set: %v", err)
	}
	index, err := checker.NewBoundaryIndex(ms, nil)
	if err != nil {
		t.Fatalf("boundary index: %v", err)
	}
	bad := facts.ReferenceEdge{Kind: facts.ReferenceKind("bogus")}
	if _, err := index.Classify(bad); err == nil {
		t.Errorf("expected an error for a malformed edge")
	}
}
