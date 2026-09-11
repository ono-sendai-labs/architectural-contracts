//go:build integration

package goanalysis

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/packages"
)

func TestLoadPackageFacts_RejectsIncompatibleExportData(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "member.go"), []byte("package member\n\nimport _ \"example.com/dep\"\n"), 0o644); err != nil {
		t.Fatalf("writing member source: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "exports"), 0o755); err != nil {
		t.Fatalf("creating export directory: %v", err)
	}
	// This is a readable artifact with no supported gc export stream. It
	// exercises the same fail-closed path as a compiler-version mismatch while
	// keeping the fixture independent of a second Go toolchain installation.
	const incompatible = "!<arch>\n__.PKGDEF\nversion go1.99\n"
	if err := os.WriteFile(filepath.Join(workspace, "exports", "incompatible.a"), []byte(incompatible), 0o644); err != nil {
		t.Fatalf("writing incompatible export artifact: %v", err)
	}
	layoutPath := filepath.Join(workspace, "component.package-layout.json")
	layout := map[string]any{
		"roots": []string{"example.com/member"},
		"packages": []map[string]any{
			{
				"id": "example.com/member", "name": "member", "pkgPath": "example.com/member",
				"goFiles": []string{"member.go"}, "compiledGoFiles": []string{"member.go"},
				"imports": map[string]string{"example.com/dep": "example.com/dep"},
			},
			{
				"id": "example.com/dep", "name": "dep", "pkgPath": "example.com/dep",
				"exportFile": "exports/incompatible.a", "imports": map[string]string{},
			},
		},
	}
	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshalling package layout: %v", err)
	}
	if err := os.WriteFile(layoutPath, data, 0o644); err != nil {
		t.Fatalf("writing package layout: %v", err)
	}

	err = packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		_, err := LoadPackageFacts(LoadRequest{
			ComponentRoot: workspace,
			Members:       []string{"example.com/member"},
		})
		return err
	})
	if err == nil {
		t.Fatal("LoadPackageFacts() succeeded with incompatible export data")
	}
	for _, want := range []string{"example.com/dep", "incompatible.a", "export"} {
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
			t.Errorf("error = %q, want substring %q", err, want)
		}
	}
}

func TestLoadPackageFacts_DeepExportClosureWithoutDependencySources(t *testing.T) {
	workspace := t.TempDir()
	write := func(path, contents string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(workspace, path)), 0o755); err != nil {
			t.Fatalf("creating directory for %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(workspace, path), []byte(contents), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
	}
	write("go.mod", "module example.com/hermetic\n\ngo 1.26\n")
	write("member/member.go", `package member

import "example.com/hermetic/dep"

func Name() string { return dep.Make().Name }
`)
	write("dep/dep.go", `package dep

import "example.com/hermetic/deep"

func Make() deep.Value { return deep.Value{Name: "deep"} }
`)
	write("deep/deep.go", `package deep

type Value struct { Name string }
`)

	type listedPackage struct {
		ID              string   `json:"ImportPath"`
		Name            string   `json:"Name"`
		Dir             string   `json:"Dir"`
		GoFiles         []string `json:"GoFiles"`
		CompiledGoFiles []string `json:"CompiledGoFiles"`
		Imports         []string `json:"Imports"`
		ExportFile      string   `json:"Export"`
	}
	cmd := exec.Command("go", "list", "-json", "-deps", "-export", "./member")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -export: %v", err)
	}
	var listed []listedPackage
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	for {
		var pkg listedPackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decoding go list package: %v", err)
		}
		listed = append(listed, pkg)
	}
	if len(listed) < 3 {
		t.Fatalf("go list returned %d packages, want the member and deep closure", len(listed))
	}

	byPath := make(map[string]*packages.Package, len(listed))
	for _, pkg := range listed {
		if pkg.ID == "" || pkg.Name == "" {
			continue
		}
		byPath[pkg.ID] = &packages.Package{ID: pkg.ID, Name: pkg.Name, PkgPath: pkg.ID, Imports: make(map[string]*packages.Package)}
	}
	if byPath["example.com/hermetic/member"] == nil || byPath["example.com/hermetic/dep"] == nil || byPath["example.com/hermetic/deep"] == nil {
		t.Fatalf("go list closure paths = %v, want member, dep, and deep", sortedPackagePaths(byPath))
	}
	for _, pkg := range listed {
		current := byPath[pkg.ID]
		if current == nil {
			continue
		}
		for _, importPath := range pkg.Imports {
			if imported := byPath[importPath]; imported != nil {
				current.Imports[importPath] = imported
			}
		}
		if pkg.ID == "example.com/hermetic/member" {
			current.GoFiles = []string{"member/member.go"}
			current.CompiledGoFiles = []string{"member/member.go"}
			continue
		}
		if pkg.ExportFile == "" {
			continue
		}
		archiveName := filepath.ToSlash(filepath.Join("exports", fmt.Sprintf("%s.a", strings.NewReplacer("/", "_", ".", "_").Replace(pkg.ID))))
		archivePath := filepath.Join(workspace, filepath.FromSlash(archiveName))
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			t.Fatalf("creating export directory: %v", err)
		}
		archive, err := os.ReadFile(pkg.ExportFile)
		if err != nil {
			t.Fatalf("reading export data for %s: %v", pkg.ID, err)
		}
		if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
			t.Fatalf("staging export data for %s: %v", pkg.ID, err)
		}
		current.ExportFile = archiveName
		// These paths deliberately do not exist after the source closure is
		// removed; the member-only driver must discard them before loading.
		current.GoFiles = []string{"removed/" + strings.ReplaceAll(pkg.ID, "/", "_") + ".go"}
		current.CompiledGoFiles = append([]string(nil), current.GoFiles...)
	}

	root := byPath["example.com/hermetic/member"]
	layout := &packagelayout.Layout{Roots: []string{root.ID}, Packages: make([]*packages.Package, 0, len(byPath))}
	for _, pkg := range byPath {
		layout.Packages = append(layout.Packages, pkg)
	}
	data, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshalling deep layout: %v", err)
	}
	layoutPath := filepath.Join(workspace, "member.package-layout.json")
	if err := os.WriteFile(layoutPath, data, 0o644); err != nil {
		t.Fatalf("writing deep layout: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(workspace, "dep")); err != nil {
		t.Fatalf("removing dependency source: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(workspace, "deep")); err != nil {
		t.Fatalf("removing deep dependency source: %v", err)
	}

	var loaded facts.PackageFacts
	err = packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		var err error
		loaded, err = LoadPackageFacts(LoadRequest{ComponentRoot: workspace, Members: []string{root.PkgPath}})
		return err
	})
	if err != nil {
		t.Fatalf("LoadPackageFacts() with dependency sources absent: %v", err)
	}
	if len(loaded.Packages) != 1 || loaded.Packages[0].ImportPath != root.PkgPath {
		t.Fatalf("loaded package facts = %+v, want only the member root", loaded.Packages)
	}
	if len(loaded.Imports) == 0 || loaded.Imports[0].ImportPath != "example.com/hermetic/dep" {
		t.Fatalf("import facts = %+v, want the dep edge", loaded.Imports)
	}
	var sawDep, sawDeep bool
	for _, reference := range loaded.References {
		if reference.ReferentPackage == "example.com/hermetic/dep" {
			sawDep = true
		}
		if reference.ReferentPackage == "example.com/hermetic/deep" {
			sawDeep = true
		}
	}
	if !sawDep || !sawDeep {
		t.Fatalf("typed references = %+v, want dep and deep declaring objects", loaded.References)
	}
}

func sortedPackagePaths(packages map[string]*packages.Package) []string {
	paths := make([]string, 0, len(packages))
	for path := range packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
