//go:build integration

package goanalysis_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
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
		_, err := goanalysis.LoadPackageFacts(goanalysis.LoadRequest{
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
		loaded, err = goanalysis.LoadPackageFacts(goanalysis.LoadRequest{ComponentRoot: workspace, Members: []string{root.PkgPath}})
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

func TestLoadPackageFacts_RefscanMatrixWithDependencySourcesAbsent(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, refscanRoot, workspace)
	modulePath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan"
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatalf("writing refscan go.mod: %v", err)
	}
	kindsSourcePath := filepath.Join(workspace, "member", "kinds", "kinds.go")
	kindsSource, err := os.ReadFile(kindsSourcePath)
	if err != nil {
		t.Fatalf("reading kinds source: %v", err)
	}
	kindsSource = bytes.Replace(kindsSource, []byte("import _ \""+modulePath+"/dep/initpkg\"\n"), []byte("import _ \"image/png\"\n"), 1)
	if err := os.WriteFile(kindsSourcePath, kindsSource, 0o644); err != nil {
		t.Fatalf("adding blank stdlib import: %v", err)
	}

	type listedPackage struct {
		ID         string   `json:"ImportPath"`
		Name       string   `json:"Name"`
		Dir        string   `json:"Dir"`
		GoFiles    []string `json:"GoFiles"`
		Compiled   []string `json:"CompiledGoFiles"`
		Imports    []string `json:"Imports"`
		ExportFile string   `json:"Export"`
	}
	cmd := exec.Command("go", "list", "-json", "-deps", "-export", "./member/...")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -export refscan fixture: %v", err)
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
			t.Fatalf("decoding refscan package list: %v", err)
		}
		listed = append(listed, pkg)
	}

	rootPaths := []string{
		modulePath + "/member/builtins",
		modulePath + "/member/concrete",
		modulePath + "/member/dispatch",
		modulePath + "/member/kinds",
	}
	rootSet := make(map[string]bool, len(rootPaths))
	for _, path := range rootPaths {
		rootSet[path] = true
	}
	packagesByPath := make(map[string]*packages.Package, len(listed))
	for _, pkg := range listed {
		packagesByPath[pkg.ID] = &packages.Package{
			ID: pkg.ID, Name: pkg.Name, PkgPath: pkg.ID,
			Imports: make(map[string]*packages.Package),
		}
	}
	for _, pkg := range listed {
		current := packagesByPath[pkg.ID]
		if current == nil {
			continue
		}
		for _, importPath := range pkg.Imports {
			if imported := packagesByPath[importPath]; imported != nil {
				current.Imports[importPath] = imported
			}
		}
		if rootSet[pkg.ID] {
			current.GoFiles = relativeFiles(workspace, pkg.Dir, pkg.GoFiles)
			compiled := pkg.Compiled
			if len(compiled) == 0 {
				compiled = pkg.GoFiles
			}
			current.CompiledGoFiles = relativeFiles(workspace, pkg.Dir, compiled)
			continue
		}
		if pkg.ExportFile == "" {
			continue
		}
		archiveName := filepath.ToSlash(filepath.Join("exports", strings.NewReplacer("/", "_", ".", "_").Replace(pkg.ID)+".a"))
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
		current.GoFiles = []string{"removed/" + strings.ReplaceAll(pkg.ID, "/", "_") + ".go"}
		current.CompiledGoFiles = append([]string(nil), current.GoFiles...)
	}

	layout := &packagelayout.Layout{Roots: rootPaths, Packages: make([]*packages.Package, 0, len(packagesByPath))}
	for _, pkg := range packagesByPath {
		layout.Packages = append(layout.Packages, pkg)
	}
	layoutData, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshalling refscan layout: %v", err)
	}
	layoutPath := filepath.Join(workspace, "refscan.package-layout.json")
	if err := os.WriteFile(layoutPath, layoutData, 0o644); err != nil {
		t.Fatalf("writing refscan layout: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(workspace, "dep")); err != nil {
		t.Fatalf("removing refscan dependency sources: %v", err)
	}

	var loaded facts.PackageFacts
	err = packagelayout.WithDriverEnv(layoutPath, workspace, func() error {
		var err error
		loaded, err = goanalysis.LoadPackageFacts(goanalysis.LoadRequest{ComponentRoot: workspace, Members: rootPaths})
		return err
	})
	if err != nil {
		t.Fatalf("member-only refscan load: %v", err)
	}
	members, err := facts.NewMemberSet(rootPaths...)
	if err != nil {
		t.Fatalf("member set: %v", err)
	}
	classified, err := checker.ClassifyEdges(members, nil, loaded.References, loaded.Imports, newFixtureAuthority(t), testSDKKey())
	if err != nil {
		t.Fatalf("classifying member-only refscan edges: %v", err)
	}
	var imageImport facts.ImportEdge
	for _, edge := range loaded.Imports {
		if edge.ImportPath == "image/png" {
			imageImport = edge
			break
		}
	}
	if imageImport.ImportPath == "" {
		t.Fatal("member-only refscan did not retain the blank image/png import")
	}
	var imageInitObservation *checker.AuthorityObservation
	for i := range classified.Authority {
		observation := &classified.Authority[i]
		if observation.Referent == facts.SymbolID("image/png.init") {
			imageInitObservation = observation
			break
		}
	}
	if imageInitObservation == nil || imageInitObservation.Capability != "FILES" || imageInitObservation.Site != imageImport.Site {
		t.Fatalf("member-only blank import classification = %+v, want FILES at %+v", imageInitObservation, imageImport.Site)
	}

	refs := make(map[facts.ReferenceKey]int, len(loaded.References))
	for _, edge := range loaded.References {
		refs[edge.Key()]++
	}
	wantRef := func(kind facts.ReferenceKind, from, referentPackage, referent, file string, line int) {
		t.Helper()
		key := facts.ReferenceKey{Kind: kind, FromPackage: from, ReferentPackage: referentPackage, Referent: facts.SymbolID(referent), Site: facts.SourceSite{File: file, Line: line}}
		if refs[key] != 1 {
			t.Errorf("member-only reference %+v count = %d, want 1", key, refs[key])
		}
	}
	kinds := refscanMember + "/kinds"
	kindsFile := "member/kinds/kinds.go"
	sub := refscanDep + "/sub"
	for _, want := range []struct {
		kind facts.ReferenceKind
		pkg  string
		id   string
		line int
	}{
		{facts.RefType, refscanDep, refscanDep + ".Base", 11},
		{facts.RefField, refscanDep, refscanDep + ".Base", 12},
		{facts.RefVar, refscanDep, refscanDep + ".ExportedVar", 13},
		{facts.RefConst, refscanDep, refscanDep + ".ExportedConst", 14},
		{facts.RefType, refscanDep, refscanDep + ".Plain", 15},
		{facts.RefType, refscanDep, refscanDep + ".Alias", 16},
		{facts.RefField, refscanDep, refscanDep + ".Base", 17},
		{facts.RefType, refscanDep, refscanDep + ".Outer", 18},
		{facts.RefField, refscanDep, refscanDep + ".Base", 19},
		{facts.RefType, refscanDep, refscanDep + ".Box", 20},
		{facts.RefType, refscanDep, refscanDep + ".Box", 21},
		{facts.RefMethod, refscanDep, "(" + refscanDep + ".Box).Get", 22},
		{facts.RefType, refscanDep, refscanDep + ".Ptr", 23},
		{facts.RefMethod, refscanDep, "(" + refscanDep + ".Ptr).Touch", 24},
		{facts.RefFunc, refscanDep, refscanDep + ".Free", 25},
		{facts.RefFunc, sub, sub + ".Sub", 28},
	} {
		wantRef(want.kind, kinds, want.pkg, want.id, kindsFile, want.line)
	}
	wantRef(facts.RefFunc, refscanMember+"/dispatch", refscanDep, refscanDep+".NewGreeter", "member/dispatch/dispatch.go", 8)
	wantRef(facts.RefMethod, refscanMember+"/dispatch", refscanDep, refscanDep+".Greeter", "member/dispatch/dispatch.go", 9)
	wantRef(facts.RefType, refscanMember+"/concrete", refscanDep, refscanDep+".Impl", "member/concrete/concrete.go", 8)
	wantRef(facts.RefMethod, refscanMember+"/concrete", refscanDep, "("+refscanDep+".Impl).Greet", "member/concrete/concrete.go", 9)
	wantRef(facts.RefFunc, refscanMember+"/builtins", "fmt", "fmt.Println", "member/builtins/builtins.go", 14)
	wantRef(facts.RefFunc, refscanMember+"/builtins", refscanDep, refscanDep+".Hello", "member/builtins/builtins.go", 14)

	imports := make(map[facts.ImportKey]int, len(loaded.Imports))
	for _, edge := range loaded.Imports {
		imports[edge.Key()]++
	}
	wantImport := func(from, path string, line int) {
		t.Helper()
		key := facts.ImportKey{ImportingPackage: from, ImportPath: path, Resolution: facts.ImportResolved, Site: facts.SourceSite{File: importFile(from), Line: line}}
		if imports[key] != 1 {
			t.Errorf("member-only import %+v count = %d, want 1", key, imports[key])
		}
	}
	wantImport(kinds, refscanDep, 4)
	wantImport(kinds, sub, 5)
	wantImport(kinds, "image/png", 8)
	wantImport(refscanMember+"/dispatch", refscanDep, 4)
	wantImport(refscanMember+"/concrete", refscanDep, 4)
	wantImport(refscanMember+"/builtins", "fmt", 4)
	wantImport(refscanMember+"/builtins", refscanDep, 6)
}

func importFile(pkg string) string {
	switch {
	case strings.HasSuffix(pkg, "/kinds"):
		return "member/kinds/kinds.go"
	case strings.HasSuffix(pkg, "/dispatch"):
		return "member/dispatch/dispatch.go"
	case strings.HasSuffix(pkg, "/concrete"):
		return "member/concrete/concrete.go"
	default:
		return "member/builtins/builtins.go"
	}
}

func relativeFiles(root, dir string, names []string) []string {
	files := make([]string, 0, len(names))
	for _, name := range names {
		path, err := filepath.Rel(root, filepath.Join(dir, name))
		if err != nil {
			panic(err)
		}
		files = append(files, filepath.ToSlash(path))
	}
	return files
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying fixture tree: %v", err)
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
