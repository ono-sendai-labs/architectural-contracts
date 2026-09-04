package hostpolicy_test

import (
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// requireUpstreamDefaults asserts that all package-global seams hold their
// upstream default values, so tests can verify restoration and defaulting.
func requireUpstreamDefaults(t *testing.T) {
	t.Helper()
	if hostpolicy.NamespaceID != "upstream" {
		t.Errorf("NamespaceID = %q, want %q", hostpolicy.NamespaceID, "upstream")
	}
	for _, p := range []string{"", "fmt", "example.com/aspect/core", "orig/foo", "canon/foo"} {
		if got := hostpolicy.CanonicalizePath(p); got != p {
			t.Errorf("CanonicalizePath(%q) = %q, want identity", p, got)
		}
		if !hostpolicy.IsCanonicalPath(p) {
			t.Errorf("IsCanonicalPath(%q) = false, want true under identity defaults", p)
		}
	}
}

// restoreSeams saves the current values of the mutable seams and registers a
// t.Cleanup restoring them; tests that override seams must call it first and
// must not run in parallel.
func restoreSeams(t *testing.T) {
	t.Helper()
	ns, canon, isCanon := hostpolicy.NamespaceID, hostpolicy.CanonicalizePath, hostpolicy.IsCanonicalPath
	t.Cleanup(func() {
		hostpolicy.NamespaceID = ns
		hostpolicy.CanonicalizePath = canon
		hostpolicy.IsCanonicalPath = isCanon
	})
}

func TestUpstreamDefaultsAreStable(t *testing.T) {
	requireUpstreamDefaults(t)
}

func TestIsCanonicalPathDefaultAgreesWithIdentity(t *testing.T) {
	for _, p := range []string{"", "fmt", "example.com/aspect/core"} {
		want := hostpolicy.CanonicalizePath(p) == p
		if got := hostpolicy.IsCanonicalPath(p); got != want {
			t.Errorf("IsCanonicalPath(%q) = %v, want %v (agreement with CanonicalizePath)", p, got, want)
		}
	}
}

func TestHostOverrideIsIdempotent(t *testing.T) {
	restoreSeams(t)
	requireUpstreamDefaults(t)

	rewrite := func(p string) string {
		if rest, ok := strings.CutPrefix(p, "orig/"); ok {
			return "canon/" + rest
		}
		return p
	}
	hostpolicy.CanonicalizePath = rewrite
	hostpolicy.IsCanonicalPath = func(p string) bool { return rewrite(p) == p }
	hostpolicy.NamespaceID = "acme-rewritten"

	if hostpolicy.IsCanonicalPath("orig/foo") {
		t.Errorf(`IsCanonicalPath("orig/foo") = true, want false for a rewritten spelling`)
	}
	if !hostpolicy.IsCanonicalPath("canon/foo") {
		t.Errorf(`IsCanonicalPath("canon/foo") = false, want true for a canonical spelling`)
	}
	const p = "orig/foo"
	once := hostpolicy.CanonicalizePath(p)
	if twice := hostpolicy.CanonicalizePath(once); twice != once {
		t.Errorf("CanonicalizePath applied twice: %q, want idempotent %q", twice, once)
	}
}

func TestNamespaceIDOverrideAndRestore(t *testing.T) {
	restoreSeams(t)
	requireUpstreamDefaults(t)

	hostpolicy.NamespaceID = "acme-rewritten"
	if hostpolicy.NamespaceID != "acme-rewritten" {
		t.Errorf("NamespaceID = %q, want override to hold within the test", hostpolicy.NamespaceID)
	}
}

func TestValidateCanonicalPath(t *testing.T) {
	restoreSeams(t)

	tests := []struct {
		name    string
		path    string
		rewrite func(string) string
		isCanon func(string) bool
		wantErr string
	}{
		{
			name:    "canonical under identity defaults",
			path:    "example.com/aspect/core",
			wantErr: "",
		},
		{
			name:    "canonical under consistent host override",
			path:    "canon/foo",
			rewrite: func(p string) string { return strings.Replace(p, "orig/", "canon/", 1) },
			isCanon: func(p string) bool { return strings.HasPrefix(p, "canon/") },
			wantErr: "",
		},
		{
			name:    "loader output the canonicalizer would rewrite",
			path:    "orig/foo",
			rewrite: func(p string) string { return strings.Replace(p, "orig/", "canon/", 1) },
			isCanon: func(p string) bool { return strings.HasPrefix(p, "canon/") },
			wantErr: `not canonical: CanonicalizePath rewrites it to "canon/foo"`,
		},
		{
			name:    "predicate and canonicalizer disagree",
			path:    "weird/foo",
			rewrite: func(p string) string { return p },
			isCanon: func(string) bool { return false },
			wantErr: `reported non-canonical by IsCanonicalPath`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rewrite != nil {
				hostpolicy.CanonicalizePath = tt.rewrite
			}
			if tt.isCanon != nil {
				hostpolicy.IsCanonicalPath = tt.isCanon
			}
			err := hostpolicy.ValidateCanonicalPath(tt.path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateCanonicalPath(%q) = %v, want nil", tt.path, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateCanonicalPath(%q) = nil, want error containing %q", tt.path, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateStdlibPaths(t *testing.T) {
	restoreSeams(t)

	t.Run("identity policy accepts stdlib paths", func(t *testing.T) {
		requireUpstreamDefaults(t)
		if err := hostpolicy.ValidateStdlibPaths([]string{"fmt", "net/http", "os/exec"}); err != nil {
			t.Errorf("ValidateStdlibPaths under identity = %v, want nil", err)
		}
	})

	t.Run("empty list passes", func(t *testing.T) {
		if err := hostpolicy.ValidateStdlibPaths(nil); err != nil {
			t.Errorf("ValidateStdlibPaths(nil) = %v, want nil", err)
		}
	})

	t.Run("host rewriting a supplied stdlib path fails clearly", func(t *testing.T) {
		hostpolicy.CanonicalizePath = func(p string) string {
			if p == "fmt" {
				return "canon/fmt"
			}
			return p
		}
		hostpolicy.IsCanonicalPath = func(p string) bool { return hostpolicy.CanonicalizePath(p) == p }
		err := hostpolicy.ValidateStdlibPaths([]string{"net/http", "fmt"})
		if err == nil {
			t.Fatal("ValidateStdlibPaths with a rewriting policy = nil, want error")
		}
		if !strings.Contains(err.Error(), `"fmt"`) || !strings.Contains(err.Error(), "standard-library") {
			t.Errorf("error %q, want it to name the rewritten stdlib path %q and mention standard-library", err, `"fmt"`)
		}
	})
}
