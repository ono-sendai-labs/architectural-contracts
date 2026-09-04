package manifestparity_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifestparity"
)

// installCanonicalizer overrides the package-global host-policy canonicalization
// seam for the duration of the test. Cleanup runs the restore, so the seam
// cannot leak to other tests.
func installCanonicalizer(t *testing.T, fn func(string) string) {
	t.Helper()
	original := hostpolicy.CanonicalizePath
	t.Cleanup(func() { hostpolicy.CanonicalizePath = original })
	hostpolicy.CanonicalizePath = fn
}

// assertSeamRestored registers a cleanup that runs after installCanonicalizer's
// restore (cleanups run LIFO) and fails the test if the seam was not restored
// to the identity default.
func assertSeamRestored(t *testing.T) {
	t.Helper()
	const marker = "seam-isolation-marker"
	t.Cleanup(func() {
		if got := hostpolicy.CanonicalizePath(marker); got != marker {
			t.Errorf("canonicalizer seam was not restored: CanonicalizePath(%q) = %q", marker, got)
		}
	})
}

type spyTB struct {
	testing.TB
	errors []string
}

func (s *spyTB) Errorf(format string, args ...any) {
	s.errors = append(s.errors, fmt.Sprintf(format, args...))
}

func (s *spyTB) Helper() {}

func TestRun_NoArgs_Skips(t *testing.T) {
	manifestparity.Run(t, nil)
}

// TestCompareManifests_AuthorityDeclarationCompared threads the structural
// authority declaration through parity: both-default pairs agree, and a
// checked-in UNKNOWN declaration cannot be normalized to the generated
// known-default without a mismatch.
func TestCompareManifests_AuthorityDeclarationCompared(t *testing.T) {
	assertSeamRestored(t)
	writeManifest := func(dir, name, extra string) string {
		compDir := filepath.Join(dir, name)
		if err := os.MkdirAll(compDir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		chkPath := filepath.Join(compDir, "component.textproto")
		content := []byte("name: \"" + name + "\"\ninterface_files: \"" + name + ".go\"\n" + extra)
		if err := os.WriteFile(chkPath, content, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		genPath := filepath.Join(compDir, name+"_component.component.textproto")
		if err := os.WriteFile(genPath, []byte("name: \""+name+"_component\"\ninterface_files: \""+name+".go\"\n"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		return chkPath
	}

	t.Run("both default declared agree", func(t *testing.T) {
		tmpDir := t.TempDir()
		chk := writeManifest(tmpDir, "defaults", "")
		spy := &spyTB{TB: t}
		manifestparity.Run(spy, []string{chk, filepath.Join(tmpDir, "defaults", "defaults_component.component.textproto")})
		if len(spy.errors) > 0 {
			t.Errorf("unexpected errors for both-default pair: %v", spy.errors)
		}
	})

	t.Run("checked-in unknown versus generated default mismatches", func(t *testing.T) {
		tmpDir := t.TempDir()
		chk := writeManifest(tmpDir, "unowned", "authority: UNKNOWN\n")
		spy := &spyTB{TB: t}
		manifestparity.Run(spy, []string{chk, filepath.Join(tmpDir, "unowned", "unowned_component.component.textproto")})
		found := false
		for _, e := range spy.errors {
			if strings.Contains(e, "authority") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected an authority mismatch error, got %v", spy.errors)
		}
	})
}

func TestRun_MissingGeneratedCounterpart(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "mycomp")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chkPath := filepath.Join(compDir, "component.textproto")
	if err := os.WriteFile(chkPath, []byte("name: \"mycomp\"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spy := &spyTB{TB: t}
	manifestparity.Run(spy, []string{chkPath})

	if len(spy.errors) == 0 {
		t.Fatalf("expected Run to report error when generated counterpart is missing, got none")
	}
	found := false
	for _, e := range spy.errors {
		if strings.Contains(e, "checked-in manifest has no generated counterpart") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error containing 'checked-in manifest has no generated counterpart', got %v", spy.errors)
	}
}

func TestRun_MissingCheckedInCounterpart(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "mycomp")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	genPath := filepath.Join(compDir, "mycomp_component.component.textproto")
	if err := os.WriteFile(genPath, []byte("name: \"mycomp_component\"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spy := &spyTB{TB: t}
	manifestparity.Run(spy, []string{genPath})

	if len(spy.errors) == 0 {
		t.Fatalf("expected Run to report error when checked-in counterpart is missing, got none")
	}
	found := false
	for _, e := range spy.errors {
		if strings.Contains(e, "generated manifest has no checked-in counterpart") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error containing 'generated manifest has no checked-in counterpart', got %v", spy.errors)
	}
}

func TestRun_MatchingPair(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "mycomp")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chkPath := filepath.Join(compDir, "component.textproto")
	manifestContent := []byte("name: \"mycomp\"\ninterface_files: \"mycomp.go\"\n")
	if err := os.WriteFile(chkPath, manifestContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	genContent := []byte("name: \"mycomp_component\"\ninterface_files: \"mycomp.go\"\n")
	genPath := filepath.Join(compDir, "mycomp_component.component.textproto")
	if err := os.WriteFile(genPath, genContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spy := &spyTB{TB: t}
	manifestparity.Run(spy, []string{chkPath, genPath})

	if len(spy.errors) > 0 {
		t.Errorf("unexpected errors for matching pair: %v", spy.errors)
	}
}

func TestCompareManifests_RewrittenNamespacesEqual(t *testing.T) {
	assertSeamRestored(t)
	// The host rewrites the upstream prefix "canonical.example/" into
	// "rewritten.invalid/X/"; both spellings map to the canonical namespace.
	installCanonicalizer(t, func(p string) string {
		return strings.Replace(p, "rewritten.invalid/X/", "canonical.example/", 1)
	})

	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"rewritten.invalid/X/mycomp", "rewritten.invalid/X/mycomp/sub"},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"canonical.example/mycomp/sub"},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) > 0 {
		t.Errorf("expected no parity errors for semantically equal manifests in rewritten vs canonical namespaces, got %v", spy.errors)
	}
}

func TestCompareManifests_RealMismatchesStillFailAfterCanonicalization(t *testing.T) {
	assertSeamRestored(t)
	installCanonicalizer(t, func(p string) string {
		return strings.Replace(p, "rewritten.invalid/X/", "canonical.example/", 1)
	})

	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"rewritten.invalid/X/alpha"},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"canonical.example/beta"},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) == 0 {
		t.Fatalf("expected parity errors for genuinely different import paths, got none")
	}
	joined := strings.Join(spy.errors, "\n")
	for _, want := range []string{"members", "canonical.example/alpha", "canonical.example/beta"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected error containing %q, got %v", want, spy.errors)
		}
	}
}

func TestCompareManifests_ImplicitInterfaceMemberAllowed(t *testing.T) {
	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp", "github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp/sub"},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp/sub"},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) > 0 {
		t.Errorf("unexpected errors when generated manifest includes implicit interface member: %v", spy.errors)
	}
}

func TestCompareManifests_MissingNonInterfaceMemberFails(t *testing.T) {
	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp", "github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp/sub1", "github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp/sub2"},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		Members:        []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/mycomp/sub1"},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) == 0 {
		t.Fatalf("expected error when a non-interface member is missing, got none")
	}
	found := false
	for _, e := range spy.errors {
		if strings.Contains(e, "members") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error containing 'members', got %v", spy.errors)
	}
}

func TestCompareManifests_DependencyPathMismatchFails(t *testing.T) {
	// Same dependency names, but the generated manifest points its "dep"
	// dependency at a different component's manifest file than the checked-in
	// declaration. Name-only comparison would pass; identity comparison must
	// fail so a wrong same-named dependency cannot hide behind parity.
	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep", Manifest: "../dep/dep_component.component.textproto"},
		},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep", Manifest: "../other/dep.component.textproto"},
		},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) == 0 {
		t.Fatalf("expected parity error for dependency manifest path mismatch, got none")
	}
	joined := strings.Join(spy.errors, "\n")
	for _, want := range []string{"component-dependency identities", "dep.component.textproto"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected error containing %q, got %v", want, spy.errors)
		}
	}
}

func TestCompareManifests_DependencyAutoAttachedMismatchFails(t *testing.T) {
	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep", Manifest: "../dep/dep_component.component.textproto", AutoAttached: true},
		},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep", Manifest: "../dep/dep.component.textproto"},
		},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) == 0 {
		t.Fatalf("expected parity error for auto-attached mismatch, got none")
	}
	if !strings.Contains(strings.Join(spy.errors, "\n"), "auto_attached=true") {
		t.Errorf("expected error showing auto_attached=true, got %v", spy.errors)
	}
}

func TestCompareManifests_DependencyParityAgrees(t *testing.T) {
	// The positive case: Bazel's generated dependency filename carries the
	// `_component` suffix and lives in the dep's own directory; the checked-in
	// declaration names the same component and file modulo those conventions.
	gen := manifest.Manifest{
		Name:           "mycomp_component",
		InterfaceFiles: []string{"mycomp.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep", Manifest: "../dep/dep_component.component.textproto"},
		},
	}
	chk := manifest.Manifest{
		Name:           "mycomp",
		InterfaceFiles: []string{"mycomp.go"},
		ComponentDependencies: []manifest.ComponentDependency{
			{Name: "dep", Manifest: "../dep/dep.component.textproto"},
		},
	}

	spy := &spyTB{TB: t}
	manifestparity.CompareManifests(spy, "mycomp", gen, chk)

	if len(spy.errors) > 0 {
		t.Errorf("unexpected errors for equivalent dependency declarations: %v", spy.errors)
	}
}
