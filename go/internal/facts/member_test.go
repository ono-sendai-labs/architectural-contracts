package facts_test

import (
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// T4 / AC4: member identity is exact — only exact canonical packages match,
// and deterministic construction rejects invalid or duplicate members.
func TestMemberSetExactMembership(t *testing.T) {
	m, err := facts.NewMemberSet(
		"example.com/app/runner",
		"example.com/app/store",
	)
	if err != nil {
		t.Fatalf("NewMemberSet() = %v, want nil", err)
	}

	if !m.Contains("example.com/app/runner") {
		t.Fatal("exact member must be contained")
	}
	if !m.Contains("example.com/app/store") {
		t.Fatal("exact member must be contained")
	}
	for _, p := range []string{
		"example.com/app/runne",        // similar prefix, not a member
		"example.com/app",              // parent path
		"example.com/app/runner/extra", // child path
		"",                             // empty
	} {
		if m.Contains(p) {
			t.Fatalf("Contains(%q) = true, want false", p)
		}
	}

	pkgs := m.Packages()
	if !reflect.DeepEqual(pkgs, []string{"example.com/app/runner", "example.com/app/store"}) {
		t.Fatalf("Packages() = %v, want sorted canonical members", pkgs)
	}

	// Deterministic construction: input order does not matter.
	m2, err := facts.NewMemberSet("example.com/app/store", "example.com/app/runner")
	if err != nil {
		t.Fatalf("NewMemberSet() = %v, want nil", err)
	}
	if !reflect.DeepEqual(m.Packages(), m2.Packages()) {
		t.Fatalf("construction depends on input order: %v vs %v", m.Packages(), m2.Packages())
	}

	// Empty set is valid and matches nothing.
	empty, err := facts.NewMemberSet()
	if err != nil {
		t.Fatalf("NewMemberSet() = %v, want nil", err)
	}
	if empty.Contains("example.com/app/runner") || len(empty.Packages()) != 0 {
		t.Fatal("empty member set must match nothing")
	}
}

func TestMemberSetConstructionRejectsInvalid(t *testing.T) {
	invalid := []struct {
		name  string
		paths []string
	}{
		{"duplicate", []string{"example.com/app/runner", "example.com/app/runner"}},
		{"empty path", []string{""}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := facts.NewMemberSet(tc.paths...); err == nil {
				t.Fatalf("NewMemberSet(%v) = nil error, want error", tc.paths)
			}
		})
	}

	// Noncanonical rejection is policy-dependent; exercise it under a
	// rewriting host policy. Not parallel: it swaps the host hooks.
	t.Run("noncanonical path", func(t *testing.T) {
		origCanonical, origIsCanonical := hostpolicy.CanonicalizePath, hostpolicy.IsCanonicalPath
		hostpolicy.CanonicalizePath = func(p string) string {
			if p == "example.com/app/runner" {
				return "host/runner"
			}
			return p
		}
		hostpolicy.IsCanonicalPath = func(p string) bool { return hostpolicy.CanonicalizePath(p) == p }
		defer func() {
			hostpolicy.CanonicalizePath, hostpolicy.IsCanonicalPath = origCanonical, origIsCanonical
		}()
		if _, err := facts.NewMemberSet("example.com/app/runner"); err == nil {
			t.Fatal("NewMemberSet() = nil error, want error for noncanonical path")
		}
		// A fixed point of the host canonicalizer is accepted.
		if _, err := facts.NewMemberSet("host/runner"); err != nil {
			t.Fatalf("NewMemberSet() = %v, want nil", err)
		}
	})
}
