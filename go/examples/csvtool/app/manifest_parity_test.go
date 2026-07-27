package app_test

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// TestManifestParity checks that the manifests the go_component rule generates
// say the same thing as the hand-written colocated manifests they mirror. It is
// driven by Bazel, which passes every relevant manifest as a positional
// argument (see //go/examples/csvtool/app:manifest_parity_test). Under plain
// `go test` it gets none and skips.
//
// The two forms are not byte-identical by construction, so this compares
// meaning, not text. Normalized away:
//   - the `_component` suffix Bazel target names carry (`toprow_component` vs
//     the manifest name `toprow`);
//   - absorbed-dependency `reason` text, which the rule never emits (§7.3);
//   - source-path frames — interface files are compared by basename.
//
// Interface files are compared as checked-in ⊆ generated, not for equality:
// the Bazel model's interface granularity is the whole interface library, so a
// second file in the same package that the colocated manifest marked
// architecture-private (csvfile's private.go) legitimately appears in the
// generated interface set.
func TestManifestParity(t *testing.T) {
	files := flag.Args()
	if len(files) == 0 {
		t.Skip("no manifests passed; this parity test is driven by Bazel (see BUILD.bazel)")
	}

	// Bazel expands $(rootpath ...) relative to the runfiles root, which is
	// $TEST_SRCDIR/$TEST_WORKSPACE, not the test's working directory.
	runfilesRoot := filepath.Join(os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"))

	// Both a component's generated manifest and its checked-in manifest sit in
	// the same source directory, so the directory name keys them together.
	generated := map[string]string{}
	checkedIn := map[string]string{}
	for _, f := range files {
		base := filepath.Base(f)
		dir := filepath.Base(filepath.Dir(f))
		resolved := filepath.Join(runfilesRoot, f)
		switch {
		case strings.HasSuffix(base, ".package-layout.json"):
			// The layout travels with the manifest but is not compared here.
			continue
		case base == "component.textproto":
			checkedIn[dir] = resolved
		case strings.HasSuffix(base, "_component.component.textproto"):
			generated[dir] = resolved
		default:
			t.Errorf("unexpected manifest argument %q", f)
		}
	}

	if len(generated) == 0 {
		t.Fatal("no generated manifests among the arguments")
	}

	for dir, genPath := range generated {
		chkPath, ok := checkedIn[dir]
		if !ok {
			t.Errorf("%s: generated manifest has no checked-in counterpart", dir)
			continue
		}
		gen := parse(t, genPath)
		chk := parse(t, chkPath)
		compare(t, dir, gen, chk)
	}
}

func parse(t *testing.T, path string) manifest.Manifest {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	m, err := manifest.Parse(f)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return m
}

func compare(t *testing.T, dir string, gen, chk manifest.Manifest) {
	t.Helper()

	// Name: the generated name is the Bazel target, which suffixes `_component`.
	if got := strings.TrimSuffix(gen.Name, "_component"); got != chk.Name {
		t.Errorf("%s: name = %q (normalized %q), want %q", dir, gen.Name, got, chk.Name)
	}

	// Interface files: every file the checked-in manifest calls interface must
	// appear in the generated interface set (compared by basename).
	genIface := basenames(gen.InterfaceFiles)
	for _, want := range basenames(chk.InterfaceFiles) {
		if !slices.Contains(genIface, want) {
			t.Errorf("%s: checked-in interface file %q missing from generated interface files %v", dir, want, genIface)
		}
	}

	// Members: same set.
	if got, want := sorted(gen.Members), sorted(chk.Members); !slices.Equal(got, want) {
		t.Errorf("%s: members = %v, want %v", dir, got, want)
	}

	// Interface style: same style.
	if gen.InterfaceStyle != chk.InterfaceStyle {
		t.Errorf("%s: interface style = %v, want %v", dir, gen.InterfaceStyle, chk.InterfaceStyle)
	}

	// Absorbed dependencies: same import paths, reasons ignored.
	if got, want := importPaths(gen.AbsorbedDependencies), importPaths(chk.AbsorbedDependencies); !slices.Equal(got, want) {
		t.Errorf("%s: absorbed import paths = %v, want %v", dir, got, want)
	}

	// Declared authority: same set.
	if got, want := sorted(gen.DeclaredAuthority), sorted(chk.DeclaredAuthority); !slices.Equal(got, want) {
		t.Errorf("%s: declared authority = %v, want %v", dir, got, want)
	}

	// Component dependencies: same set of names, `_component` suffix normalized.
	if got, want := depNames(gen.ComponentDependencies), depNames(chk.ComponentDependencies); !slices.Equal(got, want) {
		t.Errorf("%s: component-dependency names = %v, want %v", dir, got, want)
	}
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	sort.Strings(out)
	return out
}

func importPaths(deps []manifest.AbsorbedDependency) []string {
	out := make([]string, len(deps))
	for i, d := range deps {
		out[i] = d.ImportPath
	}
	sort.Strings(out)
	return out
}

func depNames(deps []manifest.ComponentDependency) []string {
	out := make([]string, len(deps))
	for i, d := range deps {
		out[i] = strings.TrimSuffix(d.Name, "_component")
	}
	sort.Strings(out)
	return out
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
