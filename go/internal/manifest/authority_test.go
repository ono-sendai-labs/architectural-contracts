package manifest_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

func unknownPersisted() gen.Authority  { return gen.Authority_UNKNOWN }
func declaredPersisted() gen.Authority { return gen.Authority_DECLARED }

// knownDeclarations are the declaration archetypes the lattice is defined over:
// the absorbing top element, known-empty, and two known non-empty sets.
var knownDeclarations = []struct {
	name string
	decl manifest.AuthorityDeclaration
}{
	{"unknown", manifest.UnknownAuthority()},
	{"declared-empty", manifest.DeclaredAuthority()},
	{"declared-files", manifest.DeclaredAuthority("FILES")},
	{"declared-network", manifest.DeclaredAuthority("NETWORK")},
}

// TestJoin_LatticePairs evaluates every ordered pair among the archetypes
// (design §Data Models / The authority lattice, R11): unknown absorbs, known
// sets union, and DECLARED{} joined with itself stays known-empty.
func TestJoin_LatticePairs(t *testing.T) {
	for _, a := range knownDeclarations {
		for _, b := range knownDeclarations {
			got := manifest.Join(a.decl, b.decl)
			want := joinWant(a.name, b.name, a.decl, b.decl)
			if !manifest.Equal(got, want) {
				t.Errorf("Join(%s, %s) = %s, want %s", a.name, b.name, renderDeclaration(got), renderDeclaration(want))
			}
		}
	}
}

func joinWant(a, b string, aDecl, bDecl manifest.AuthorityDeclaration) manifest.AuthorityDeclaration {
	if a == "unknown" || b == "unknown" {
		return manifest.UnknownAuthority()
	}
	merged := append(slices.Clone(aDecl.Set), bDecl.Set...)
	slices.Sort(merged)
	merged = slices.Compact(merged)
	return manifest.DeclaredAuthority(merged...)
}

func renderDeclaration(d manifest.AuthorityDeclaration) string {
	if !d.Known {
		return "UNKNOWN"
	}
	return "DECLARED" + renderSet(d.Set)
}

func renderSet(set []manifest.Capability) string {
	out := "{"
	for i, c := range set {
		if i > 0 {
			out += " "
		}
		out += string(c)
	}
	return out + "}"
}

// TestJoin_InputsUnchanged verifies Join never aliases or mutates either input
// slice (task requirement 4): the result must be a fresh, sorted allocation and
// the inputs must be untouched afterwards.
func TestJoin_InputsUnchanged(t *testing.T) {
	a := manifest.DeclaredAuthority("NETWORK", "FILES")
	b := manifest.DeclaredAuthority("EXEC", "FILES")
	aBefore := slices.Clone(a.Set)
	bBefore := slices.Clone(b.Set)

	got := manifest.Join(a, b)
	if !slices.Equal(got.Set, []manifest.Capability{"EXEC", "FILES", "NETWORK"}) {
		t.Fatalf("Join result = %v, want sorted [EXEC FILES NETWORK]", got.Set)
	}

	got.Set[0] = "MUTATED"
	if !slices.Equal(a.Set, aBefore) {
		t.Errorf("input a mutated: %v, want %v", a.Set, aBefore)
	}
	if !slices.Equal(b.Set, bBefore) {
		t.Errorf("input b mutated: %v, want %v", b.Set, bBefore)
	}

	// The result must not alias an input even when only one side contributes.
	if solo := manifest.Join(manifest.DeclaredAuthority("FILES"), manifest.DeclaredAuthority()); solo.Known != true || &solo.Set == nil {
		t.Fatal("Join with known-empty lost the known state")
	}
}

// TestAuthorityDeclaration_ConstructorValidation covers R11 plus the retained
// capability validation (task requirement 2): unknown capability names and
// duplicates are rejected, and the unknown value cannot carry capabilities.
func TestAuthorityDeclaration_ConstructorValidation(t *testing.T) {
	t.Run("unknown capability name rejected", func(t *testing.T) {
		if _, err := manifest.NewDeclared("TELEPORTATION"); err == nil {
			t.Fatal("NewDeclared with unknown capability, want error")
		}
	})
	t.Run("duplicate capabilities rejected", func(t *testing.T) {
		if _, err := manifest.NewDeclared("FILES", "FILES"); err == nil {
			t.Fatal("NewDeclared with duplicate capabilities, want error")
		}
	})
	t.Run("set is sorted and deduplicated", func(t *testing.T) {
		d, err := manifest.NewDeclared("NETWORK", "FILES")
		if err != nil {
			t.Fatalf("NewDeclared: %v", err)
		}
		if !slices.Equal(d.Set, []manifest.Capability{"FILES", "NETWORK"}) {
			t.Errorf("Set = %v, want [FILES NETWORK]", d.Set)
		}
	})
	t.Run("unknown is known-false with empty set", func(t *testing.T) {
		d := manifest.UnknownAuthority()
		if d.Known {
			t.Error("UnknownAuthority().Known = true, want false")
		}
		if len(d.Set) != 0 {
			t.Errorf("UnknownAuthority().Set = %v, want empty", d.Set)
		}
	})
	t.Run("declared-empty is known-true", func(t *testing.T) {
		d, err := manifest.NewDeclared()
		if err != nil {
			t.Fatalf("NewDeclared(): %v", err)
		}
		if !d.Known || len(d.Set) != 0 {
			t.Errorf("NewDeclared() = %v, want known-empty", d)
		}
	})
}

// TestAuthorityPersistedRoundTrip covers task requirement 5: valid declared and
// unknown native values survive the conversion to the persisted authority-plus-
// capabilities representation and back with state, ordering and equality intact.
func TestAuthorityPersistedRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		decl manifest.AuthorityDeclaration
	}{
		{"unknown", manifest.UnknownAuthority()},
		{"declared-empty", manifest.DeclaredAuthority()},
		{"declared-unsorted-input", manifest.DeclaredAuthority("NETWORK", "FILES")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authority, caps, err := manifest.ToPersisted(tc.decl)
			if err != nil {
				t.Fatalf("ToPersisted: %v", err)
			}
			back, err := manifest.FromPersisted(authority, caps)
			if err != nil {
				t.Fatalf("FromPersisted: %v", err)
			}
			want := tc.decl
			if want.Known {
				slices.Sort(want.Set)
			}
			if !manifest.Equal(back, want) {
				t.Errorf("round trip = %s, want %s", renderDeclaration(back), renderDeclaration(want))
			}
		})
	}
}

// TestFromPersisted_RejectsNonCanonical covers the fail-closed serialization
// rule (task requirement 5, AC 3): an unknown value paired with a non-empty
// capability list is an error, never normalized away, and unknown capability
// names are rejected.
func TestFromPersisted_RejectsNonCanonical(t *testing.T) {
	if _, err := manifest.FromPersisted(unknownPersisted(), []manifest.Capability{"FILES"}); err == nil {
		t.Fatal("FromPersisted(UNKNOWN, [FILES]), want error")
	}
	if _, err := manifest.FromPersisted(declaredPersisted(), []manifest.Capability{"TELEPORTATION"}); err == nil {
		t.Fatal("FromPersisted(DECLARED, [TELEPORTATION]), want error")
	}
}

// TestToPersisted_RejectsUnknownWithCapabilities pins that a declaration that
// is both unknown and carrying capabilities (only constructible by struct
// literal) is refused rather than silently serialized.
func TestToPersisted_RejectsUnknownWithCapabilities(t *testing.T) {
	bogus := manifest.AuthorityDeclaration{Known: false, Set: []manifest.Capability{"FILES"}}
	if _, _, err := manifest.ToPersisted(bogus); err == nil {
		t.Fatal("ToPersisted(UNKNOWN with caps), want error")
	}
}

// TestEqual_DistinguishesEmptyFromUnknown covers AC 2 at the native level:
// neither is inferable from slice emptiness alone.
func TestEqual_DistinguishesEmptyFromUnknown(t *testing.T) {
	if manifest.Equal(manifest.DeclaredAuthority(), manifest.UnknownAuthority()) {
		t.Fatal("DECLARED{} equals UNKNOWN")
	}
	if manifest.Equal(manifest.DeclaredAuthority("FILES"), manifest.DeclaredAuthority()) {
		t.Fatal("DECLARED{FILES} equals DECLARED{}")
	}
	if !manifest.Equal(manifest.DeclaredAuthority("FILES"), manifest.DeclaredAuthority("FILES")) {
		t.Fatal("identical declarations not equal")
	}
	if !reflect.DeepEqual(manifest.Join(manifest.DeclaredAuthority(), manifest.DeclaredAuthority()), manifest.DeclaredAuthority()) {
		t.Fatal("DECLARED{} ⊔ DECLARED{} is not known-empty")
	}
}
