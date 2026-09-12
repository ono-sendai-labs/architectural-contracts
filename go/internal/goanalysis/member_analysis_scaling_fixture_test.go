//go:build integration

package goanalysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/teststdlibmap"
	"golang.org/x/tools/go/packages"
)

const scalingSampleCount = 3

type scalingFixtureParameters struct {
	Depth       int
	Width       int
	TypeSurface int
}

type scalingFixture struct {
	Workspace         string
	LayoutPath        string
	MemberPath        string
	ExpectedArtifacts uint64
}

type scalingRunCapture struct {
	Packages []*packages.Package
	Phases   map[analysisPhase][]time.Duration
}

type scalingSample struct {
	Facts        facts.PackageFacts
	Graph        scalingGraphSnapshot
	Workload     scalingMemberWorkload
	Phases       map[analysisPhase][]time.Duration
	ReportBytes  []byte
	SurfaceBytes []byte
}

type scalingGraphSnapshot struct {
	MemberPackages                  int
	NonMemberPackages               int
	MemberSyntaxFiles               int
	MemberTypeInfoPackages          int
	MemberUses                      int
	MemberSelections                int
	NonMemberSyntaxFiles            int
	NonMemberTypeInfoPackages       int
	NonMemberSourceFilePackages     int
	NonMemberExportBackedPackages   int
	NonMemberIncompleteTypePackages int
}

type scalingMemberWorkload struct {
	PackagePaths      []string
	GoFiles           []string
	CompiledGoFiles   []string
	SyntaxFiles       []string
	SyntaxFileCount   int
	TypeInfoPackages  int
	Uses              int
	Selections        int
	SourceFileContent []scalingSourceFile
}

type scalingSourceFile struct {
	Path    string
	Content []byte
}

type scalingListedPackage struct {
	ID              string   `json:"ImportPath"`
	Name            string   `json:"Name"`
	Dir             string   `json:"Dir"`
	GoFiles         []string `json:"GoFiles"`
	CompiledGoFiles []string `json:"CompiledGoFiles"`
	Imports         []string `json:"Imports"`
	ExportFile      string   `json:"Export"`
}

func buildScalingFixture(t *testing.T, params scalingFixtureParameters) scalingFixture {
	t.Helper()
	if params.Depth < 1 || params.Width < 1 || params.TypeSurface < 0 {
		t.Fatalf("invalid scaling fixture parameters: %#v", params)
	}

	workspace := t.TempDir()
	const modulePath = "example.com/member-analysis-scaling"
	memberPath := modulePath + "/member"
	entryPath := modulePath + "/entry"
	writeScalingFile(t, workspace, "go.mod", "module "+modulePath+"\n\ngo 1.26\n")
	writeScalingFile(t, workspace, "member/member.go", "package member\n\nimport \""+entryPath+"\"\n\nfunc Use() entry.Value {\n\treturn entry.Make()\n}\n")
	writeScalingFile(t, workspace, "entry/entry.go", scalingEntrySource(modulePath, params.TypeSurface, params.Depth, params.Width))

	for level := 0; level < params.Depth; level++ {
		for index := 0; index < params.Width; index++ {
			importPaths := make([]string, 0)
			if level+1 < params.Depth {
				for child := 0; child < params.Width; child++ {
					importPaths = append(importPaths, scalingNodePath(modulePath, level+1, child))
				}
			}
			writeScalingFile(t, workspace, filepath.ToSlash(filepath.Join("nodes", fmt.Sprintf("level-%02d-node-%02d", level, index), "node.go")), scalingNodeSource(importPaths))
		}
	}

	listed := listScalingPackages(t, workspace)
	packagesByPath := make(map[string]*packages.Package, len(listed))
	for _, listedPackage := range listed {
		if listedPackage.ID == "" || listedPackage.Name == "" {
			t.Fatalf("go list returned an incomplete package record: %#v", listedPackage)
		}
		packagesByPath[listedPackage.ID] = &packages.Package{
			ID:      listedPackage.ID,
			Name:    listedPackage.Name,
			PkgPath: listedPackage.ID,
			Imports: make(map[string]*packages.Package),
		}
	}
	wantPackages := 2 + params.Depth*params.Width
	if len(packagesByPath) != wantPackages {
		t.Fatalf("scaling fixture depth=%d width=%d package count = %d, want member plus exact dependency graph count %d", params.Depth, params.Width, len(packagesByPath), wantPackages)
	}

	dependencyDirs := make(map[string]bool)
	var exportArtifacts uint64
	for _, listedPackage := range listed {
		current := packagesByPath[listedPackage.ID]
		for _, importPath := range listedPackage.Imports {
			imported := packagesByPath[importPath]
			if imported == nil {
				t.Fatalf("package %q imports package %q absent from go list closure", listedPackage.ID, importPath)
			}
			current.Imports[importPath] = imported
		}
		if listedPackage.ID == memberPath {
			current.GoFiles = scalingRelativeFiles(t, workspace, listedPackage.Dir, listedPackage.GoFiles)
			compiledFiles := listedPackage.CompiledGoFiles
			if len(compiledFiles) == 0 {
				compiledFiles = listedPackage.GoFiles
			}
			current.CompiledGoFiles = scalingRelativeFiles(t, workspace, listedPackage.Dir, compiledFiles)
			continue
		}
		if listedPackage.ExportFile == "" {
			if listedPackage.ID == "unsafe" {
				t.Fatalf("scaling fixture unexpectedly includes unsafe in the synthetic no-import closure")
			}
			t.Fatalf("non-member package %q has no compiler export artifact", listedPackage.ID)
		}
		archiveName := filepath.ToSlash(filepath.Join("exports", scalingArtifactName(listedPackage.ID)+".a"))
		archivePath := filepath.Join(workspace, filepath.FromSlash(archiveName))
		archive, err := os.ReadFile(listedPackage.ExportFile)
		if err != nil {
			t.Fatalf("reading export artifact for %q: %v", listedPackage.ID, err)
		}
		if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
			t.Fatalf("creating export directory for %q: %v", listedPackage.ID, err)
		}
		if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
			t.Fatalf("staging export artifact for %q: %v", listedPackage.ID, err)
		}
		current.ExportFile = archiveName
		removedSource := filepath.ToSlash(filepath.Join("removed", scalingArtifactName(listedPackage.ID)+".go"))
		current.GoFiles = []string{removedSource}
		current.CompiledGoFiles = []string{removedSource}
		dependencyDirs[listedPackage.Dir] = true
		exportArtifacts++
	}
	if exportArtifacts != uint64(1+params.Depth*params.Width) {
		t.Fatalf("scaling fixture export artifacts = %d, want exact count %d", exportArtifacts, 1+params.Depth*params.Width)
	}

	packagesList := make([]*packages.Package, 0, len(packagesByPath))
	for _, pkg := range packagesByPath {
		packagesList = append(packagesList, pkg)
	}
	sort.Slice(packagesList, func(i, j int) bool { return packagesList[i].ID < packagesList[j].ID })
	layout := &packagelayout.Layout{
		Roots:    []string{memberPath},
		Packages: packagesList,
	}
	layoutBytes, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("marshalling scaling package layout: %v", err)
	}
	layoutPath := filepath.Join(workspace, "member-analysis-scaling.package-layout.json")
	if err := os.WriteFile(layoutPath, layoutBytes, 0o644); err != nil {
		t.Fatalf("writing scaling package layout: %v", err)
	}
	for dependencyDir := range dependencyDirs {
		if err := os.RemoveAll(dependencyDir); err != nil {
			t.Fatalf("removing dependency source directory %q: %v", dependencyDir, err)
		}
	}

	return scalingFixture{
		Workspace:         workspace,
		LayoutPath:        layoutPath,
		MemberPath:        memberPath,
		ExpectedArtifacts: 1 + uint64(params.Depth*params.Width),
	}
}

func measureScalingSample(t *testing.T, fixture scalingFixture) scalingSample {
	t.Helper()
	previousObserver := phaseObserver
	previousLoader := loadPackages
	defer func() {
		phaseObserver = previousObserver
		loadPackages = previousLoader
	}()

	var current *scalingRunCapture
	phaseObserver = func(phase analysisPhase, duration time.Duration) {
		if current != nil {
			current.Phases[phase] = append(current.Phases[phase], duration)
		}
	}
	loadPackages = func(config *packages.Config, patterns ...string) ([]*packages.Package, error) {
		loaded, err := previousLoader(config, patterns...)
		if current != nil && current.Packages == nil {
			current.Packages = loaded
		}
		return loaded, err
	}

	var measured scalingSample
	var factsBytes []byte
	for iteration := 0; iteration < scalingSampleCount+1; iteration++ {
		current = &scalingRunCapture{Phases: make(map[analysisPhase][]time.Duration)}
		var loaded facts.PackageFacts
		var reportBytes, surfaceBytes []byte
		err := packagelayout.WithDriverEnv(fixture.LayoutPath, fixture.Workspace, func() error {
			var err error
			loaded, err = LoadPackageFacts(LoadRequest{
				ComponentRoot: fixture.Workspace,
				Members:       []string{fixture.MemberPath},
			})
			if err != nil {
				return err
			}
			reportBytes, surfaceBytes = emitScalingArtifacts(t, fixture, loaded)
			return nil
		})
		if err != nil {
			t.Fatalf("member-only scaling load iteration %d: %v", iteration, err)
		}
		if iteration == 0 {
			continue
		}
		if measured.Phases == nil {
			measured = scalingSample{
				Facts:        loaded,
				Graph:        snapshotScalingGraph(current.Packages, fixture),
				Workload:     snapshotScalingWorkload(t, current.Packages, fixture),
				Phases:       make(map[analysisPhase][]time.Duration),
				ReportBytes:  append([]byte(nil), reportBytes...),
				SurfaceBytes: append([]byte(nil), surfaceBytes...),
			}
		}
		if !bytes.Equal(reportBytes, measured.ReportBytes) {
			t.Fatalf("sample iteration %d changed canonical report bytes", iteration)
		}
		if !bytes.Equal(surfaceBytes, measured.SurfaceBytes) {
			t.Fatalf("sample iteration %d changed canonical surface bytes", iteration)
		}
		encodedFacts, err := json.Marshal(loaded)
		if err != nil {
			t.Fatalf("marshalling facts from sample iteration %d: %v", iteration, err)
		}
		if factsBytes == nil {
			factsBytes = encodedFacts
		} else if !bytes.Equal(encodedFacts, factsBytes) {
			t.Fatalf("sample iteration %d changed deterministic fact bytes", iteration)
		}
		for _, phase := range []analysisPhase{analysisPhaseLoadPackageFacts, analysisPhaseLoadPackages, analysisPhaseScanReferences} {
			durations := current.Phases[phase]
			if len(durations) != 1 {
				t.Fatalf("phase %q recorded %d observations in sample iteration %d, want one", phase, len(durations), iteration)
			}
			measured.Phases[phase] = append(measured.Phases[phase], durations[0])
		}
		if !reflect.DeepEqual(loaded.References, measured.Facts.References) || !reflect.DeepEqual(loaded.Imports, measured.Facts.Imports) {
			t.Fatalf("repeated scaling sample %d changed typed workload", iteration)
		}
		if got := snapshotScalingWorkload(t, current.Packages, fixture); !reflect.DeepEqual(got, measured.Workload) {
			t.Fatalf("repeated scaling sample %d changed member load workload: got %#v, want %#v", iteration, got, measured.Workload)
		}
	}
	if measured.Phases == nil {
		t.Fatal("scaling measurement produced no non-warm-up samples")
	}
	return measured
}

// emitScalingArtifacts runs the production surface derivation and canonical
// artifact encoders against the facts produced by the member-only load. The
// phase observer is intentionally not an input to either encoder; retained
// samples therefore prove that observed duration variation cannot contaminate
// report or surface bytes.
func emitScalingArtifacts(t *testing.T, fixture scalingFixture, loaded facts.PackageFacts) ([]byte, []byte) {
	t.Helper()
	inputs, err := LoadSurfaceInputs(LoadRequest{
		ComponentName: fixture.MemberPath,
		ComponentRoot: fixture.Workspace,
		Members:       []string{fixture.MemberPath},
	})
	if err != nil {
		t.Fatalf("loading surface inputs for artifact determinism: %v", err)
	}
	sources := make([]surface.SourceFile, 0, len(inputs.SourcePaths))
	for _, sourcePath := range inputs.SourcePaths {
		data, err := os.ReadFile(filepath.Join(fixture.Workspace, filepath.FromSlash(sourcePath)))
		if err != nil {
			t.Fatalf("reading surface source %q for artifact determinism: %v", sourcePath, err)
		}
		sources = append(sources, surface.SourceFile{Path: sourcePath, Bytes: data})
	}
	key, err := teststdlibmap.ExpectedKey()
	if err != nil {
		t.Fatalf("deriving scaling surface SDK key: %v", err)
	}
	derived, err := surface.Derive(surface.Input{
		Component:       fixture.MemberPath,
		Style:           manifest.InterfaceStylePackageSurface,
		Authority:       manifest.AuthorityDeclaration{Known: true},
		Namespace:       hostpolicy.NamespaceID,
		Key:             &key,
		ProducerVersion: "member-analysis-scaling-test",
		MemberPackages:  inputs.MemberPackages,
		Manifest:        []byte("name: \"member-analysis-scaling\"\n"),
		Sources:         sources,
	})
	if err != nil {
		t.Fatalf("deriving scaling surface artifact: %v", err)
	}
	surfaceBytes, err := artifactio.MarshalSurface(derived)
	if err != nil {
		t.Fatalf("encoding scaling surface artifact: %v", err)
	}
	reportBytes, err := artifactio.MarshalReport(report.ConformanceReport{
		Component: fixture.MemberPath,
		Diagnostics: report.Diagnostics{
			NonMemberExportArtifactCount: loaded.ExportDataDiagnostics.NonMemberExportArtifactCount,
			NonMemberExportBytes:         loaded.ExportDataDiagnostics.NonMemberExportBytes,
		},
	})
	if err != nil {
		t.Fatalf("encoding scaling report artifact: %v", err)
	}
	return reportBytes, surfaceBytes
}

func scalingPhasesHaveDistinctDurations(phases map[analysisPhase][]time.Duration) bool {
	seen := make(map[time.Duration]struct{})
	for _, durations := range phases {
		for _, duration := range durations {
			seen[duration] = struct{}{}
		}
	}
	return len(seen) > 1
}

func snapshotScalingGraph(pkgs []*packages.Package, fixture scalingFixture) scalingGraphSnapshot {
	memberSet := map[string]bool{fixture.MemberPath: true}
	var snapshot scalingGraphSnapshot
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		if memberSet[pkg.PkgPath] {
			snapshot.MemberPackages++
			snapshot.MemberSyntaxFiles += len(pkg.Syntax)
			if pkg.TypesInfo != nil {
				snapshot.MemberTypeInfoPackages++
				snapshot.MemberUses += len(pkg.TypesInfo.Uses)
				snapshot.MemberSelections += len(pkg.TypesInfo.Selections)
			}
			return
		}
		snapshot.NonMemberPackages++
		snapshot.NonMemberSyntaxFiles += len(pkg.Syntax)
		if pkg.TypesInfo != nil {
			snapshot.NonMemberTypeInfoPackages++
		}
		if len(pkg.GoFiles) > 0 || len(pkg.CompiledGoFiles) > 0 {
			snapshot.NonMemberSourceFilePackages++
		}
		if pkg.ExportFile != "" {
			snapshot.NonMemberExportBackedPackages++
		}
		if pkg.Types == nil || !pkg.Types.Complete() {
			snapshot.NonMemberIncompleteTypePackages++
		}
	})
	return snapshot
}

func assertScalingGraph(t *testing.T, graph scalingGraphSnapshot, fixture scalingFixture) {
	t.Helper()
	wantNonMembers := int(fixture.ExpectedArtifacts)
	if graph.MemberPackages != 1 || graph.NonMemberPackages != wantNonMembers {
		t.Errorf("package roles = member:%d non-member:%d, want member:1 non-member:%d", graph.MemberPackages, graph.NonMemberPackages, wantNonMembers)
	}
	if graph.MemberSyntaxFiles == 0 || graph.MemberTypeInfoPackages != 1 {
		t.Errorf("member syntax/type-info counters = files:%d type-info:%d, want a non-empty member and one typed member", graph.MemberSyntaxFiles, graph.MemberTypeInfoPackages)
	}
	if graph.NonMemberSyntaxFiles != 0 || graph.NonMemberTypeInfoPackages != 0 || graph.NonMemberSourceFilePackages != 0 {
		t.Errorf("non-member source eligibility counters = syntax:%d type-info:%d source-files:%d, want all zero", graph.NonMemberSyntaxFiles, graph.NonMemberTypeInfoPackages, graph.NonMemberSourceFilePackages)
	}
	if graph.NonMemberExportBackedPackages != wantNonMembers || graph.NonMemberIncompleteTypePackages != 0 {
		t.Errorf("non-member export/type counters = export-backed:%d incomplete:%d, want export-backed:%d incomplete:0", graph.NonMemberExportBackedPackages, graph.NonMemberIncompleteTypePackages, wantNonMembers)
	}
}

func snapshotScalingWorkload(t *testing.T, pkgs []*packages.Package, fixture scalingFixture) scalingMemberWorkload {
	var workload scalingMemberWorkload
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil || pkg.PkgPath != fixture.MemberPath {
			return
		}
		workload.PackagePaths = []string{pkg.PkgPath}
		workload.GoFiles = scalingRelativePackageFiles(fixture.Workspace, pkg.GoFiles)
		workload.CompiledGoFiles = scalingRelativePackageFiles(fixture.Workspace, pkg.CompiledGoFiles)
		for _, syntax := range pkg.Syntax {
			if syntax == nil {
				continue
			}
			path := filepath.ToSlash(scalingRelativeFile(t, fixture.Workspace, pkg.Fset.Position(syntax.Pos()).Filename))
			workload.SyntaxFiles = append(workload.SyntaxFiles, path)
		}
		workload.SyntaxFileCount = len(pkg.Syntax)
		if pkg.TypesInfo != nil {
			workload.TypeInfoPackages = 1
			workload.Uses = len(pkg.TypesInfo.Uses)
			workload.Selections = len(pkg.TypesInfo.Selections)
		}
		for _, file := range workload.GoFiles {
			content, err := os.ReadFile(filepath.Join(fixture.Workspace, filepath.FromSlash(file)))
			if err != nil {
				panic(fmt.Sprintf("reading captured member source %q: %v", file, err))
			}
			workload.SourceFileContent = append(workload.SourceFileContent, scalingSourceFile{Path: file, Content: content})
		}
	})
	sort.Strings(workload.SyntaxFiles)
	sort.Slice(workload.SourceFileContent, func(i, j int) bool { return workload.SourceFileContent[i].Path < workload.SourceFileContent[j].Path })
	return workload
}

func listScalingPackages(t *testing.T, workspace string) []scalingListedPackage {
	t.Helper()
	command := exec.Command("go", "list", "-json", "-deps", "-export", "./member")
	command.Dir = workspace
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -export scaling fixture: %v\n%s", err, output)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var listed []scalingListedPackage
	for {
		var pkg scalingListedPackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decoding scaling package list: %v", err)
		}
		listed = append(listed, pkg)
	}
	return listed
}

func scalingEntrySource(modulePath string, typeSurface, depth, width int) string {
	var builder strings.Builder
	builder.WriteString("package entry\n")
	if depth > 0 {
		builder.WriteString("\nimport (\n")
		for index := 0; index < width; index++ {
			fmt.Fprintf(&builder, "\t_ %q\n", scalingNodePath(modulePath, 0, index))
		}
		builder.WriteString(")\n")
	}
	builder.WriteString("\n")
	builder.WriteString("type Value struct { Name string }\n\n")
	builder.WriteString("func Make() Value { return Value{Name: \"fixed\"} }\n")
	for index := 0; index < typeSurface; index++ {
		fmt.Fprintf(&builder, "\ntype PublicType%03d struct { Field string }\n\nfunc PublicFunc%03d(value PublicType%03d) PublicType%03d { return value }\n", index, index, index, index)
	}
	return builder.String()
}

func scalingNodeSource(importPaths []string) string {
	var builder strings.Builder
	builder.WriteString("package node\n")
	if len(importPaths) > 0 {
		builder.WriteString("\nimport (\n")
		for _, importPath := range importPaths {
			fmt.Fprintf(&builder, "\t_ %q\n", importPath)
		}
		builder.WriteString(")\n")
	}
	return builder.String()
}

func scalingNodePath(modulePath string, level, index int) string {
	return modulePath + "/nodes/level-" + fmt.Sprintf("%02d-node-%02d", level, index)
}

func scalingArtifactName(importPath string) string {
	return strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(importPath)
}

func writeScalingFile(t *testing.T, workspace, relativePath, content string) {
	t.Helper()
	path := filepath.Join(workspace, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating scaling fixture directory for %q: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing scaling fixture file %q: %v", relativePath, err)
	}
}

func scalingRelativeFiles(t *testing.T, workspace, packageDir string, names []string) []string {
	t.Helper()
	files := make([]string, 0, len(names))
	for _, name := range names {
		files = append(files, scalingRelativeFile(t, workspace, filepath.Join(packageDir, name)))
	}
	sort.Strings(files)
	return files
}

func scalingRelativePackageFiles(workspace string, names []string) []string {
	files := make([]string, 0, len(names))
	for _, name := range names {
		relative, err := filepath.Rel(workspace, name)
		if err != nil {
			panic(err)
		}
		files = append(files, filepath.ToSlash(relative))
	}
	sort.Strings(files)
	return files
}

func scalingRelativeFile(t *testing.T, workspace, path string) string {
	t.Helper()
	relative, err := filepath.Rel(workspace, path)
	if err != nil {
		t.Fatalf("making scaling file %q relative to %q: %v", path, workspace, err)
	}
	return filepath.ToSlash(relative)
}

func medianPhase(phases map[analysisPhase][]time.Duration, phase analysisPhase) time.Duration {
	values := append([]time.Duration(nil), phases[phase]...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if len(values) == 0 {
		return 0
	}
	if len(values)%2 == 1 {
		return values[len(values)/2]
	}
	return (values[len(values)/2-1] + values[len(values)/2]) / 2
}

func formatScalingSamples(phases map[analysisPhase][]time.Duration) string {
	return fmt.Sprintf("load=%v scan=%v total=%v", phases[analysisPhaseLoadPackages], phases[analysisPhaseScanReferences], phases[analysisPhaseLoadPackageFacts])
}

func formatScalingPhases(phases map[analysisPhase][]time.Duration) string {
	return fmt.Sprintf("load=%s scan=%s total=%s", medianPhase(phases, analysisPhaseLoadPackages), medianPhase(phases, analysisPhaseScanReferences), medianPhase(phases, analysisPhaseLoadPackageFacts))
}
