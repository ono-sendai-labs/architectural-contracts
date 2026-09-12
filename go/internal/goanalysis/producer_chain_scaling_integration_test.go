//go:build integration

package goanalysis

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/packages"
)

const (
	producerChainSampleCount = 3
	producerChainPackage     = "bazel_rules/go/tests/testdata/producerchain"
	producerChainImportRoot  = "example.com/arcc-producer-chain"
)

var producerChainActionMnemonics = []string{"ArccImportGraph", "ArccLayout", "ArccCheck"}

type producerChainVariant struct {
	Name                    string
	Depth                   int
	Width                   int
	TypeSurface             int
	Component               string
	Dependency              string
	MemberPackage           string
	DependencyMemberPackage string
}

type producerChainLoadTarget struct {
	Label         string
	Name          string
	MemberPackage string
}

type producerChainAction struct {
	CommandArgs   []string                   `json:"commandArgs"`
	Environment   []producerChainEnvironment `json:"environmentVariables"`
	Inputs        []producerChainArtifact    `json:"inputs"`
	ListedOutputs []string                   `json:"listedOutputs"`
	ActualOutputs []producerChainArtifact    `json:"actualOutputs"`
	Mnemonic      string                     `json:"mnemonic"`
	TargetLabel   string                     `json:"targetLabel"`
	CacheHit      bool                       `json:"cacheHit"`
	Metrics       producerChainActionMetrics `json:"metrics"`
}

type producerChainEnvironment struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type producerChainArtifact struct {
	Path        string              `json:"path"`
	Digest      producerChainDigest `json:"digest"`
	IsTool      bool                `json:"isTool"`
	SymlinkPath string              `json:"symlinkTargetPath"`
}

type producerChainDigest struct {
	SizeBytes string `json:"sizeBytes"`
}

type producerChainActionMetrics struct {
	ExecutionWallTime string `json:"executionWallTime"`
	TotalTime         string `json:"totalTime"`
}

type producerChainProfile struct {
	TraceEvents []producerChainProfileEvent `json:"traceEvents"`
}

type producerChainProfileEvent struct {
	Category string  `json:"cat"`
	Name     string  `json:"name"`
	Phase    string  `json:"ph"`
	Duration float64 `json:"dur"`
	Args     struct {
		Mnemonic string `json:"mnemonic"`
	} `json:"args"`
}

type producerChainWorkload struct {
	MemberPackages            int
	MemberSourceFiles         int
	MemberSyntaxFiles         int
	MemberTypeInfoPackages    int
	MemberUses                int
	MemberSelections          int
	MemberImportEdges         int
	MemberReferenceEdges      int
	NonMemberPackages         int
	NonMemberSourceFiles      int
	NonMemberSyntaxFiles      int
	NonMemberTypeInfoPackages int
	NonMemberIncompleteTypes  int
}

type producerChainLoaderObservation struct {
	Workload      producerChainWorkload
	Diagnostics   facts.ExportDataDiagnostics
	LoadDurations []time.Duration
	ScanDurations []time.Duration
}

type producerChainRow struct {
	Variant                         string
	Depth                           int
	Width                           int
	TypeSurface                     int
	ImportGraphActions              int
	LayoutActions                   int
	CheckActions                    int
	ProjectionSourceFiles           uint64
	ProjectionSourceBytes           uint64
	DependencyProjectionSourceFiles uint64
	DependencyProjectionSourceBytes uint64
	ProjectionElapsed               time.Duration
	LayoutElapsed                   time.Duration
	CheckLoaderElapsed              time.Duration
	MemberScanElapsed               time.Duration
	DependencyCheckLoaderElapsed    time.Duration
	DependencyMemberScanElapsed     time.Duration
	CheckTotalElapsed               time.Duration
	ProducerChainElapsed            time.Duration
	ExportArtifactCount             uint64
	ExportArtifactBytes             uint64
	DependencyExportArtifactCount   uint64
	DependencyExportArtifactBytes   uint64
	MemberWorkload                  producerChainWorkload
	DependencyMemberWorkload        producerChainWorkload
}

func TestBazelProducerChainScaling_Integration(t *testing.T) {
	variants := producerChainScalingVariants()
	build := runProducerChainBazelBuild(t, variants)

	assertProducerChainProfile(t, build.ProfilePath, variants)
	actions := readProducerChainExecutionLog(t, build.ExecutionLogPath)
	byTargetAndMnemonic := indexProducerChainActions(actions)

	stdlibMapActions := actionsWithMnemonic(actions, "ArccStdlibMap")
	if len(stdlibMapActions) != 1 {
		t.Fatalf("ArccStdlibMap actions = %d, want exactly one default-configuration action", len(stdlibMapActions))
	}
	if stdlibMapActions[0].TargetLabel != "//:arcc_stdlib_map" {
		t.Fatalf("stdlib-map action target = %q, want //:arcc_stdlib_map", stdlibMapActions[0].TargetLabel)
	}
	assertProducerActionHermetic(t, stdlibMapActions[0])

	execroot := filepath.Join(build.OutputBase, "execroot", "_main")
	rows := make([]producerChainRow, 0, len(variants))
	for _, variant := range variants {
		rootLoader := measureProducerChainLoader(t, execroot, byTargetAndMnemonic, producerChainLoadTarget{
			Label:         variant.Component,
			Name:          variant.Name + "_component",
			MemberPackage: variant.MemberPackage,
		})
		dependencyLoader := measureProducerChainLoader(t, execroot, byTargetAndMnemonic, producerChainLoadTarget{
			Label:         variant.Dependency,
			Name:          variant.Name + "_dependency_component",
			MemberPackage: variant.DependencyMemberPackage,
		})
		row := producerChainRowForVariant(t, byTargetAndMnemonic, variant, rootLoader, dependencyLoader)
		rows = append(rows, row)
	}

	assertProducerChainScaling(t, rows)
	assertProducerChainArtifactsDeterministic(t, build, variants)
	printProducerChainTable(rows, len(stdlibMapActions))
}

type producerChainBazelBuild struct {
	OutputBase       string
	ProfilePath      string
	ExecutionLogPath string
}

func runProducerChainBazelBuild(t *testing.T, variants []producerChainVariant) producerChainBazelBuild {
	t.Helper()
	if _, err := exec.LookPath("bazel"); err != nil {
		t.Fatalf("locating bazel for producer-chain integration: %v", err)
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed while locating the repository")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../.."))
	runDir := t.TempDir()
	outputBase := filepath.Join(runDir, "output-base")
	profilePath := filepath.Join(runDir, "producer-chain.profile.json.gz")
	executionLogPath := filepath.Join(runDir, "producer-chain.execution.json")

	// The server is tied to the temporary output base. Shutting it down before
	// the test directory is reclaimed prevents an isolated run from leaving a
	// live Bazel process or state outside the test's declared directory.
	t.Cleanup(func() {
		command := exec.Command("bazel", "--output_base="+outputBase, "shutdown")
		command.Dir = repoRoot
		_ = command.Run()
		makeProducerChainTreeWritable(runDir)
	})

	args := producerChainBazelBuildArgs(outputBase, variants)
	args = append(args,
		"--profile="+profilePath,
		"--execution_log_json_file="+executionLogPath,
		"--execution_log_sort",
		"--noslim_profile",
		"--experimental_profile_additional_tasks=action",
	)
	output := runProducerChainBazelCommand(t, repoRoot, args...)
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("Bazel profile was not produced: %v\n%s", err, output)
	}
	if _, err := os.Stat(executionLogPath); err != nil {
		t.Fatalf("Bazel execution log was not produced: %v\n%s", err, output)
	}

	return producerChainBazelBuild{
		OutputBase:       outputBase,
		ProfilePath:      profilePath,
		ExecutionLogPath: executionLogPath,
	}
}

func producerChainBazelBuildArgs(outputBase string, variants []producerChainVariant) []string {
	args := []string{
		"--output_base=" + outputBase,
		"build",
		"--output_groups=+arcc",
		"--noshow_progress",
	}
	labels := make([]string, 0, len(variants))
	for _, variant := range variants {
		labels = append(labels, variant.Component)
	}
	sort.Strings(labels)
	return append(args, labels...)
}

func runProducerChainBazelCommand(t *testing.T, repoRoot string, args ...string) []byte {
	t.Helper()
	command := exec.Command("bazel", args...)
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Bazel producer-chain command %q: %v\n%s", strings.Join(args, " "), err, output)
	}
	return output
}

func readProducerChainExecutionLog(t *testing.T, path string) []producerChainAction {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening Bazel execution log %q: %v", path, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(bufio.NewReader(file))
	var actions []producerChainAction
	for {
		var action producerChainAction
		err := decoder.Decode(&action)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decoding Bazel execution log %q: %v", path, err)
		}
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		t.Fatalf("Bazel execution log %q contains no actions", path)
	}
	return actions
}

func indexProducerChainActions(actions []producerChainAction) map[string][]producerChainAction {
	indexed := make(map[string][]producerChainAction)
	for _, action := range actions {
		key := action.TargetLabel + "\x00" + action.Mnemonic
		indexed[key] = append(indexed[key], action)
	}
	return indexed
}

func actionsWithMnemonic(actions []producerChainAction, mnemonic string) []producerChainAction {
	var matching []producerChainAction
	for _, action := range actions {
		if action.Mnemonic == mnemonic {
			matching = append(matching, action)
		}
	}
	return matching
}

func producerChainScalingVariants() []producerChainVariant {
	var variants []producerChainVariant
	for _, depth := range []int{1, 4, 16} {
		for _, width := range []int{1, 2, 4} {
			variants = append(variants, newProducerChainVariant(depth, width, 0, false))
		}
	}
	for _, typeSurface := range []int{0, 8, 32, 128} {
		variants = append(variants, newProducerChainVariant(1, 1, typeSurface, true))
	}
	return variants
}

func newProducerChainVariant(depth, width, typeSurface int, directSurface bool) producerChainVariant {
	name := fmt.Sprintf("d%02dw%02d", depth, width)
	if directSurface {
		name = fmt.Sprintf("s%04d", typeSurface)
	}
	return producerChainVariant{
		Name:                    name,
		Depth:                   depth,
		Width:                   width,
		TypeSurface:             typeSurface,
		Component:               "//" + producerChainPackage + ":" + name + "_component",
		Dependency:              "//" + producerChainPackage + ":" + name + "_dependency_component",
		MemberPackage:           producerChainImportRoot + "/" + name + "/member",
		DependencyMemberPackage: producerChainImportRoot + "/" + name + "/entry",
	}
}

func producerChainRowForVariant(t *testing.T, indexed map[string][]producerChainAction, variant producerChainVariant, rootLoader, dependencyLoader producerChainLoaderObservation) producerChainRow {
	t.Helper()
	rootActions := make(map[string]producerChainAction, len(producerChainActionMnemonics))
	for _, mnemonic := range producerChainActionMnemonics {
		key := variant.Component + "\x00" + mnemonic
		matches := indexed[key]
		if len(matches) != 1 {
			t.Fatalf("%s %s actions = %d, want exactly one", variant.Name, mnemonic, len(matches))
		}
		if matches[0].CacheHit {
			t.Fatalf("%s %s unexpectedly came from the action cache in an isolated output root", variant.Name, mnemonic)
		}
		rootActions[mnemonic] = matches[0]
	}

	graph := rootActions["ArccImportGraph"]
	layout := rootActions["ArccLayout"]
	check := rootActions["ArccCheck"]
	assertProducerActionHermetic(t, graph)
	assertProducerActionHermetic(t, layout)
	assertProducerActionHermetic(t, check)
	assertProducerChainInputRoles(t, variant.Name+"_component", variant.Name+"_member.go", graph, layout, check, true)

	projectionFiles, projectionBytes := producerChainInputVolume(t, graph, func(path string) bool {
		return strings.HasSuffix(path, ".go")
	})
	exportFiles, exportBytes := producerChainInputVolume(t, check, isExportArtifactPath)
	wantRootArtifacts := uint64(1 + variant.Depth*variant.Width)
	if exportFiles != wantRootArtifacts {
		t.Fatalf("%s ArccCheck export artifact count = %d, want %d", variant.Name, exportFiles, wantRootArtifacts)
	}
	if rootLoader.Diagnostics.NonMemberExportArtifactCount != exportFiles || rootLoader.Diagnostics.NonMemberExportBytes != exportBytes {
		t.Fatalf("%s root export metrics disagree: action=%d/%d loader=%d/%d", variant.Name, exportFiles, exportBytes, rootLoader.Diagnostics.NonMemberExportArtifactCount, rootLoader.Diagnostics.NonMemberExportBytes)
	}

	projectionElapsed := producerChainActionElapsed(t, graph)
	layoutElapsed := producerChainActionElapsed(t, layout)
	checkTotalElapsed := producerChainActionElapsed(t, check)
	checkLoaderElapsed := medianProducerChainDuration(rootLoader.LoadDurations)
	memberScanElapsed := medianProducerChainDuration(rootLoader.ScanDurations)
	dependencyGraph := producerChainActionForTarget(t, indexed, variant.Dependency, "ArccImportGraph", variant.Name)
	dependencyLayout := producerChainActionForTarget(t, indexed, variant.Dependency, "ArccLayout", variant.Name)
	dependencyCheck := producerChainActionForTarget(t, indexed, variant.Dependency, "ArccCheck", variant.Name)
	assertProducerActionHermetic(t, dependencyGraph)
	assertProducerActionHermetic(t, dependencyLayout)
	assertProducerActionHermetic(t, dependencyCheck)
	assertProducerChainInputRoles(t, variant.Name+"_dependency_component", variant.Name+"_entry.go", dependencyGraph, dependencyLayout, dependencyCheck, false)
	dependencyProjectionFiles, dependencyProjectionBytes := producerChainInputVolume(t, dependencyGraph, func(path string) bool {
		return strings.HasSuffix(path, ".go")
	})
	dependencyExportFiles, dependencyExportBytes := producerChainInputVolume(t, dependencyCheck, isExportArtifactPath)
	wantDependencyArtifacts := uint64(variant.Depth * variant.Width)
	if dependencyProjectionFiles != wantDependencyArtifacts || dependencyExportFiles != wantDependencyArtifacts {
		t.Fatalf("%s dependency source/export counts = %d/%d, want %d", variant.Name, dependencyProjectionFiles, dependencyExportFiles, wantDependencyArtifacts)
	}
	if dependencyLoader.Diagnostics.NonMemberExportArtifactCount != dependencyExportFiles || dependencyLoader.Diagnostics.NonMemberExportBytes != dependencyExportBytes {
		t.Fatalf("%s dependency export metrics disagree: action=%d/%d loader=%d/%d", variant.Name, dependencyExportFiles, dependencyExportBytes, dependencyLoader.Diagnostics.NonMemberExportArtifactCount, dependencyLoader.Diagnostics.NonMemberExportBytes)
	}
	chainElapsed := projectionElapsed + layoutElapsed + checkTotalElapsed
	for _, action := range []producerChainAction{dependencyGraph, dependencyLayout, dependencyCheck} {
		chainElapsed += producerChainActionElapsed(t, action)
	}

	return producerChainRow{
		Variant:                         variant.Name,
		Depth:                           variant.Depth,
		Width:                           variant.Width,
		TypeSurface:                     variant.TypeSurface,
		ImportGraphActions:              1,
		LayoutActions:                   1,
		CheckActions:                    1,
		ProjectionSourceFiles:           projectionFiles,
		ProjectionSourceBytes:           projectionBytes,
		DependencyProjectionSourceFiles: dependencyProjectionFiles,
		DependencyProjectionSourceBytes: dependencyProjectionBytes,
		ProjectionElapsed:               projectionElapsed,
		LayoutElapsed:                   layoutElapsed,
		CheckLoaderElapsed:              checkLoaderElapsed,
		MemberScanElapsed:               memberScanElapsed,
		DependencyCheckLoaderElapsed:    medianProducerChainDuration(dependencyLoader.LoadDurations),
		DependencyMemberScanElapsed:     medianProducerChainDuration(dependencyLoader.ScanDurations),
		CheckTotalElapsed:               checkTotalElapsed,
		ProducerChainElapsed:            chainElapsed,
		ExportArtifactCount:             exportFiles,
		ExportArtifactBytes:             exportBytes,
		DependencyExportArtifactCount:   dependencyExportFiles,
		DependencyExportArtifactBytes:   dependencyExportBytes,
		MemberWorkload:                  rootLoader.Workload,
		DependencyMemberWorkload:        dependencyLoader.Workload,
	}
}

func producerChainActionForTarget(t *testing.T, indexed map[string][]producerChainAction, target, mnemonic, variant string) producerChainAction {
	t.Helper()
	matches := indexed[target+"\x00"+mnemonic]
	if len(matches) != 1 {
		t.Fatalf("%s dependency %s actions = %d, want exactly one", variant, mnemonic, len(matches))
	}
	if matches[0].CacheHit {
		t.Fatalf("%s dependency %s unexpectedly came from the action cache", variant, mnemonic)
	}
	return matches[0]
}

func assertProducerActionHermetic(t *testing.T, action producerChainAction) {
	t.Helper()
	if len(action.Environment) != 0 {
		t.Errorf("%s %s environment = %#v, want empty", action.TargetLabel, action.Mnemonic, action.Environment)
	}
	for _, input := range action.Inputs {
		path := filepath.ToSlash(input.Path)
		if strings.Contains(path, "/.cache/") || strings.Contains(path, "/gomodcache/") ||
			strings.Contains(path, "/gopath/") ||
			(strings.Contains(path, "/pkg/tool/") && !strings.Contains(path, "/stdlib_/")) ||
			strings.HasSuffix(path, "/bin/go") {
			t.Errorf("%s %s has forbidden cache/toolchain input %q", action.TargetLabel, action.Mnemonic, input.Path)
		}
	}
}

func assertProducerChainInputRoles(t *testing.T, targetName, memberSource string, graph, layout, check producerChainAction, requireDependencyReport bool) {
	t.Helper()
	graphPaths := producerChainInputPaths(graph)
	layoutPaths := producerChainInputPaths(layout)
	checkPaths := producerChainInputPaths(check)

	if !producerChainHasSuffix(graphPaths, ".package-imports.request.json") || !producerChainHasSuffix(graphPaths, ".go") {
		t.Errorf("%s ArccImportGraph inputs = %v, want request and ordinary Go source", targetName, graphPaths)
	}
	if producerChainHasSuffix(graphPaths, ".package-layout.base.json") || producerChainHasSuffix(graphPaths, ".x") ||
		producerChainHasSuffix(graphPaths, ".surface.json") || producerChainHasSuffix(graphPaths, ".report.json") ||
		producerChainHasSuffix(graphPaths, ".stdlib-map.json") {
		t.Errorf("%s ArccImportGraph received a non-projection input: %v", targetName, graphPaths)
	}
	for _, input := range graph.Inputs {
		if input.IsTool {
			continue
		}
		path := filepath.ToSlash(input.Path)
		if !strings.HasSuffix(path, ".package-imports.request.json") && !strings.HasSuffix(path, ".go") {
			t.Errorf("%s ArccImportGraph has an unclassified input %q", targetName, input.Path)
		}
	}
	for _, path := range graphPaths {
		if strings.HasSuffix(path, ".go") && filepath.Base(path) == memberSource {
			t.Errorf("%s member source leaked into ArccImportGraph: %q", targetName, path)
		}
	}

	if !producerChainHasSuffix(layoutPaths, ".package-imports.json") || !producerChainHasSuffix(layoutPaths, ".package-layout.base.json") {
		t.Errorf("%s ArccLayout inputs = %v, want projection descriptor and base layout", targetName, layoutPaths)
	}
	if producerChainHasSuffix(layoutPaths, ".package-imports.request.json") || producerChainHasSuffix(layoutPaths, ".go") ||
		producerChainHasSuffix(layoutPaths, ".x") || producerChainHasSuffix(layoutPaths, ".surface.json") ||
		producerChainHasSuffix(layoutPaths, ".report.json") || producerChainHasSuffix(layoutPaths, ".stdlib-map.json") {
		t.Errorf("%s ArccLayout received a non-merge input: %v", targetName, layoutPaths)
	}
	for _, input := range layout.Inputs {
		if input.IsTool {
			continue
		}
		path := filepath.ToSlash(input.Path)
		if !strings.HasSuffix(path, ".package-imports.json") && !strings.HasSuffix(path, ".package-layout.base.json") {
			t.Errorf("%s ArccLayout has an unclassified input %q", targetName, input.Path)
		}
	}

	required := []string{".package-layout.json", ".package-imports.json", ".stdlib-map.json", ".x", ".surface.json", memberSource}
	if requireDependencyReport {
		required = append(required, ".report.json")
	}
	for _, required := range required {
		if !producerChainHasSuffix(checkPaths, required) {
			t.Errorf("%s ArccCheck is missing %s input: %v", targetName, required, checkPaths)
		}
	}
	if producerChainHasSuffix(checkPaths, ".package-layout.base.json") || producerChainHasSuffix(checkPaths, ".package-imports.request.json") {
		t.Errorf("%s ArccCheck received private auxiliary input: %v", targetName, checkPaths)
	}
	for _, path := range checkPaths {
		if strings.HasSuffix(path, ".go") && filepath.Base(path) != memberSource {
			t.Errorf("%s non-member source leaked into ArccCheck: %q", targetName, path)
		}
	}
	for _, input := range check.Inputs {
		if input.IsTool {
			continue
		}
		path := filepath.ToSlash(input.Path)
		base := filepath.Base(path)
		if strings.Contains(path, "/stdlib_/") || strings.HasSuffix(base, ".component.textproto") ||
			strings.HasSuffix(base, ".package-layout.json") || strings.HasSuffix(base, ".package-imports.json") ||
			strings.HasSuffix(base, ".stdlib-map.json") || strings.HasSuffix(base, ".x") ||
			strings.HasSuffix(base, ".surface.json") || strings.HasSuffix(base, ".report.json") ||
			base == memberSource {
			continue
		}
		t.Errorf("%s ArccCheck has an unclassified input %q", targetName, input.Path)
	}
}

type producerChainLayoutWire struct {
	GoSDKRoot                  string                     `json:"go_sdk_root"`
	Packages                   []producerChainPackageWire `json:"packages"`
	OrdinaryImportData         producerChainOrdinaryWire  `json:"ordinary_import_data"`
	StdlibExportData           producerChainStdlibWire    `json:"stdlib_export_data"`
	DependencyArtifactBindings []producerChainBindingWire `json:"dependency_artifact_bindings"`
}

type producerChainPackageWire struct {
	ID              string   `json:"ID"`
	GoFiles         []string `json:"GoFiles"`
	CompiledGoFiles []string `json:"CompiledGoFiles"`
	ExportFile      string   `json:"ExportFile"`
}

type producerChainOrdinaryWire struct {
	Metadata string `json:"metadata"`
}

type producerChainStdlibWire struct {
	Metadata    string                        `json:"metadata"`
	ExportRoots []producerChainExportRootWire `json:"export_roots"`
}

type producerChainExportRootWire struct {
	RunfilesPath string `json:"runfiles_path"`
}

type producerChainBindingWire struct {
	Surface string `json:"surface"`
	Report  string `json:"report"`
}

func producerChainLoaderFrame(t *testing.T, execroot string, indexed map[string][]producerChainAction, target producerChainLoadTarget) string {
	t.Helper()
	lookup := func(mnemonic string) producerChainAction {
		matches := indexed[target.Label+"\x00"+mnemonic]
		if len(matches) != 1 {
			t.Fatalf("%s %s action count = %d while preparing loader frame", target.Name, mnemonic, len(matches))
		}
		return matches[0]
	}
	graph := lookup("ArccImportGraph")
	layout := lookup("ArccLayout")
	check := lookup("ArccCheck")
	layoutOutput := producerChainOutputPath(t, execroot, layout, target.Name+".package-layout.json")
	graphOutput := producerChainOutputPath(t, execroot, graph, target.Name+".package-imports.json")
	layoutBytes, err := os.ReadFile(layoutOutput)
	if err != nil {
		t.Fatalf("reading %s final layout %q: %v", target.Name, layoutOutput, err)
	}
	var wire producerChainLayoutWire
	if err := json.Unmarshal(layoutBytes, &wire); err != nil {
		t.Fatalf("decoding %s final layout %q: %v", target.Name, layoutOutput, err)
	}

	frameDir := t.TempDir()
	link := func(logical, physical string) {
		if logical == "" {
			return
		}
		if filepath.IsAbs(logical) {
			t.Fatalf("%s loader frame logical path %q is absolute", target.Name, logical)
		}
		linkPath := filepath.Join(frameDir, filepath.FromSlash(logical))
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			t.Fatalf("creating loader frame for %s: %v", target.Name, err)
		}
		if err := os.Symlink(physical, linkPath); err != nil {
			t.Fatalf("linking loader frame path %q to %q: %v", linkPath, physical, err)
		}
	}
	link(filepath.Base(layoutOutput), layoutOutput)
	link(filepath.Base(graphOutput), graphOutput)

	linkByBase := func(logical string, inputs []producerChainArtifact) {
		base := filepath.Base(filepath.FromSlash(logical))
		for _, input := range inputs {
			if filepath.Base(input.Path) != base || input.IsTool {
				continue
			}
			physical := input.Path
			if !filepath.IsAbs(physical) {
				physical = filepath.Join(execroot, filepath.FromSlash(physical))
			}
			link(logical, physical)
			return
		}
		t.Fatalf("%s loader frame has no physical input for logical path %q", target.Name, logical)
	}
	linkByTree := func(logical string, inputs []producerChainArtifact) {
		linkProducerChainTree(t, target, execroot, logical, inputs, link)
	}

	for _, pkg := range wire.Packages {
		if pkg.ID == target.MemberPackage {
			seen := make(map[string]bool)
			for _, file := range append(append([]string{}, pkg.GoFiles...), pkg.CompiledGoFiles...) {
				if seen[file] {
					continue
				}
				seen[file] = true
				linkByBase(file, check.Inputs)
			}
		}
		if pkg.ExportFile != "" {
			linkByBase(pkg.ExportFile, check.Inputs)
		}
	}
	linkByBase(wire.StdlibExportData.Metadata, check.Inputs)
	for _, root := range wire.StdlibExportData.ExportRoots {
		linkByTree(root.RunfilesPath, check.Inputs)
	}
	for _, binding := range wire.DependencyArtifactBindings {
		linkByBase(binding.Surface, check.Inputs)
		if binding.Report != "" {
			linkByBase(binding.Report, check.Inputs)
		}
	}

	if wire.GoSDKRoot != "" {
		repository := strings.Split(filepath.ToSlash(wire.GoSDKRoot), "/")[0]
		physical := filepath.Join(execroot, "external", filepath.FromSlash(repository))
		if _, err := os.Stat(physical); err != nil {
			t.Fatalf("%s loader frame SDK root %q: %v", target.Name, physical, err)
		}
		link(repository, physical)
	}
	return frameDir
}

func linkProducerChainTree(t *testing.T, target producerChainLoadTarget, execroot, logical string, inputs []producerChainArtifact, link func(string, string)) {
	t.Helper()
	base := filepath.Base(filepath.FromSlash(logical))
	marker := "/" + base + "/"
	for _, input := range inputs {
		if input.IsTool {
			continue
		}
		path := filepath.ToSlash(input.Path)
		index := strings.Index(path, marker)
		if index == -1 {
			continue
		}
		root := path[:index+len(marker)-1]
		physical := root
		if !filepath.IsAbs(physical) {
			physical = filepath.Join(execroot, filepath.FromSlash(physical))
		}
		link(logical, physical)
		return
	}
	t.Fatalf("%s loader frame has no physical tree input for logical path %q", target.Name, logical)
}

func producerChainOutputPath(t *testing.T, execroot string, action producerChainAction, suffix string) string {
	t.Helper()
	for _, output := range action.ActualOutputs {
		if suffix == "" || strings.HasSuffix(output.Path, suffix) {
			if filepath.IsAbs(output.Path) {
				return output.Path
			}
			return filepath.Join(execroot, filepath.FromSlash(output.Path))
		}
	}
	t.Fatalf("%s %s action has no output ending in %q", action.TargetLabel, action.Mnemonic, suffix)
	return ""
}

func producerChainInputPaths(action producerChainAction) []string {
	paths := make([]string, 0, len(action.Inputs))
	for _, input := range action.Inputs {
		paths = append(paths, filepath.ToSlash(input.Path))
	}
	return paths
}

func producerChainHasSuffix(paths []string, suffix string) bool {
	for _, path := range paths {
		if strings.HasSuffix(filepath.Base(path), suffix) {
			return true
		}
	}
	return false
}

func producerChainInputVolume(t *testing.T, action producerChainAction, include func(string) bool) (uint64, uint64) {
	t.Helper()
	var count, total uint64
	for _, input := range action.Inputs {
		if input.IsTool || !include(filepath.ToSlash(input.Path)) {
			continue
		}
		size, err := producerChainArtifactSize(input)
		if err != nil {
			t.Fatalf("%s %s input %q: %v", action.TargetLabel, action.Mnemonic, input.Path, err)
		}
		if ^uint64(0)-total < size {
			t.Fatalf("%s %s input byte total overflows uint64", action.TargetLabel, action.Mnemonic)
		}
		count++
		total += size
	}
	return count, total
}

func producerChainArtifactSize(input producerChainArtifact) (uint64, error) {
	if input.Digest.SizeBytes == "" {
		return 0, fmt.Errorf("input digest has no size")
	}
	size, err := strconv.ParseUint(input.Digest.SizeBytes, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid digest size %q: %w", input.Digest.SizeBytes, err)
	}
	return size, nil
}

func isExportArtifactPath(path string) bool {
	// rules_go's ordinary archive provider is the `.x` file staged into the
	// component action. The generated stdlib export tree also expands to many
	// `.a` files, but those are the target-configured stdlib input and must not
	// be counted as ordinary non-member closure artifacts.
	return strings.HasSuffix(path, ".x")
}

func producerChainActionElapsed(t *testing.T, action producerChainAction) time.Duration {
	t.Helper()
	value := action.Metrics.ExecutionWallTime
	if value == "" {
		value = action.Metrics.TotalTime
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		t.Fatalf("%s %s elapsed time %q: %v", action.TargetLabel, action.Mnemonic, value, err)
	}
	return duration
}

func measureProducerChainLoader(t *testing.T, execroot string, indexed map[string][]producerChainAction, target producerChainLoadTarget) producerChainLoaderObservation {
	t.Helper()
	previousObserver := phaseObserver
	previousLoader := loadPackages
	defer func() {
		phaseObserver = previousObserver
		loadPackages = previousLoader
	}()

	type capture struct {
		Packages []*packages.Package
		Phases   map[analysisPhase]time.Duration
	}
	var current *capture
	phaseObserver = func(phase analysisPhase, duration time.Duration) {
		if current != nil {
			current.Phases[phase] = duration
		}
	}
	loadPackages = func(config *packages.Config, patterns ...string) ([]*packages.Package, error) {
		loaded, err := previousLoader(config, patterns...)
		if current != nil {
			current.Packages = loaded
		}
		return loaded, err
	}

	var measured producerChainLoaderObservation
	frameDir := producerChainLoaderFrame(t, execroot, indexed, target)
	layoutPath := filepath.Join(frameDir, target.Name+".package-layout.json")
	for iteration := 0; iteration < producerChainSampleCount+1; iteration++ {
		current = &capture{Phases: make(map[analysisPhase]time.Duration)}
		var loaded facts.PackageFacts
		err := packagelayout.WithDriverEnv(layoutPath, frameDir, func() error {
			var err error
			loaded, err = LoadPackageFacts(LoadRequest{
				ComponentName: target.Name,
				ComponentRoot: frameDir,
				Members:       []string{target.MemberPackage},
			})
			return err
		})
		if err != nil {
			t.Fatalf("%s member-only loader observation %d: %v", target.Name, iteration, err)
		}
		if iteration == 0 {
			continue
		}
		loadDuration, ok := current.Phases[analysisPhaseLoadPackages]
		if !ok {
			t.Fatalf("%s loader observation %d has no packages.Load duration", target.Name, iteration)
		}
		scanDuration, ok := current.Phases[analysisPhaseScanReferences]
		if !ok {
			t.Fatalf("%s loader observation %d has no ScanReferences duration", target.Name, iteration)
		}
		workload := snapshotProducerChainWorkload(current.Packages, target.MemberPackage, loaded)
		if iteration == 1 {
			measured.Workload = workload
			measured.Diagnostics = loaded.ExportDataDiagnostics
		} else if workload != measured.Workload {
			t.Fatalf("%s member workload changed across observations: got %#v, want %#v", target.Name, workload, measured.Workload)
		} else if loaded.ExportDataDiagnostics != measured.Diagnostics {
			t.Fatalf("%s export diagnostics changed across observations: got %#v, want %#v", target.Name, loaded.ExportDataDiagnostics, measured.Diagnostics)
		}
		measured.LoadDurations = append(measured.LoadDurations, loadDuration)
		measured.ScanDurations = append(measured.ScanDurations, scanDuration)
	}
	if len(measured.LoadDurations) != producerChainSampleCount || len(measured.ScanDurations) != producerChainSampleCount {
		t.Fatalf("%s retained loader observations = %d/%d, want %d/%d", target.Name, len(measured.LoadDurations), len(measured.ScanDurations), producerChainSampleCount, producerChainSampleCount)
	}
	return measured
}

func snapshotProducerChainWorkload(pkgs []*packages.Package, memberPath string, loaded facts.PackageFacts) producerChainWorkload {
	var workload producerChainWorkload
	workload.MemberImportEdges = len(loaded.Imports)
	workload.MemberReferenceEdges = len(loaded.References)
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		if pkg.PkgPath == memberPath {
			workload.MemberPackages++
			workload.MemberSourceFiles = len(pkg.GoFiles)
			workload.MemberSyntaxFiles = len(pkg.Syntax)
			if pkg.TypesInfo != nil {
				workload.MemberTypeInfoPackages++
				workload.MemberUses = len(pkg.TypesInfo.Uses)
				workload.MemberSelections = len(pkg.TypesInfo.Selections)
			}
			return
		}
		workload.NonMemberPackages++
		if len(pkg.GoFiles) > 0 || len(pkg.CompiledGoFiles) > 0 {
			workload.NonMemberSourceFiles++
		}
		workload.NonMemberSyntaxFiles += len(pkg.Syntax)
		if pkg.TypesInfo != nil {
			workload.NonMemberTypeInfoPackages++
		}
		if pkg.Types == nil || !pkg.Types.Complete() {
			workload.NonMemberIncompleteTypes++
		}
	})
	return workload
}

func assertProducerChainScaling(t *testing.T, rows []producerChainRow) {
	t.Helper()
	if len(rows) != 13 {
		t.Fatalf("producer-chain rows = %d, want nine primary plus four direct-type-surface rows", len(rows))
	}
	baseline := rows[0]
	for _, row := range rows {
		if row.ImportGraphActions != 1 || row.LayoutActions != 1 || row.CheckActions != 1 {
			t.Errorf("%s action counts = %d/%d/%d, want one of each", row.Variant, row.ImportGraphActions, row.LayoutActions, row.CheckActions)
		}
		assertProducerChainWorkload(t, row.Variant+" root", row.MemberWorkload)
		assertProducerChainWorkload(t, row.Variant+" dependency", row.DependencyMemberWorkload)
		if !sameProducerChainMemberWorkload(row.MemberWorkload, baseline.MemberWorkload) {
			t.Errorf("%s member workload = %#v, want fixed workload %#v", row.Variant, row.MemberWorkload, baseline.MemberWorkload)
		}
		if !sameProducerChainMemberWorkload(row.DependencyMemberWorkload, baseline.DependencyMemberWorkload) {
			t.Errorf("%s dependency member workload = %#v, want fixed workload %#v", row.Variant, row.DependencyMemberWorkload, baseline.DependencyMemberWorkload)
		}
		wantRootArtifacts := uint64(1 + row.Depth*row.Width)
		wantDependencyArtifacts := uint64(row.Depth * row.Width)
		if row.ProjectionSourceFiles != wantRootArtifacts || row.ExportArtifactCount != wantRootArtifacts {
			t.Errorf("%s root source/export counts = %d/%d, want %d", row.Variant, row.ProjectionSourceFiles, row.ExportArtifactCount, wantRootArtifacts)
		}
		if row.DependencyProjectionSourceFiles != wantDependencyArtifacts || row.DependencyExportArtifactCount != wantDependencyArtifacts {
			t.Errorf("%s dependency source/export counts = %d/%d, want %d", row.Variant, row.DependencyProjectionSourceFiles, row.DependencyExportArtifactCount, wantDependencyArtifacts)
		}
	}

	primary := rows[:9]
	for _, depth := range []int{1, 4, 16} {
		previous := primaryRow(t, primary, depth, 1)
		for _, width := range []int{2, 4} {
			current := primaryRow(t, primary, depth, width)
			assertProducerChainVolumeGrowth(t, "width", depth, width, previous, current)
			previous = current
		}
	}
	for _, width := range []int{1, 2, 4} {
		previous := primaryRow(t, primary, 1, width)
		for _, depth := range []int{4, 16} {
			current := primaryRow(t, primary, depth, width)
			assertProducerChainVolumeGrowth(t, "depth", width, depth, previous, current)
			previous = current
		}
	}

	direct := rows[9:]
	previous := direct[0]
	for _, current := range direct[1:] {
		if current.ExportArtifactCount != previous.ExportArtifactCount {
			t.Errorf("direct type surface %d artifact count = %d, want constant %d", current.TypeSurface, current.ExportArtifactCount, previous.ExportArtifactCount)
		}
		if current.ExportArtifactBytes <= previous.ExportArtifactBytes {
			t.Errorf("direct type surface %d export bytes = %d, want growth beyond %d", current.TypeSurface, current.ExportArtifactBytes, previous.ExportArtifactBytes)
		}
		if !sameProducerChainMemberWorkload(current.MemberWorkload, previous.MemberWorkload) {
			t.Errorf("direct type surface %d member workload = %#v, want %#v", current.TypeSurface, current.MemberWorkload, previous.MemberWorkload)
		}
		if !sameProducerChainMemberWorkload(current.DependencyMemberWorkload, previous.DependencyMemberWorkload) {
			t.Errorf("direct type surface %d dependency member workload = %#v, want %#v", current.TypeSurface, current.DependencyMemberWorkload, previous.DependencyMemberWorkload)
		}
		previous = current
	}

	minimumScan := baseline.MemberScanElapsed
	minimumDependencyScan := baseline.DependencyMemberScanElapsed
	for _, row := range rows {
		if row.MemberScanElapsed > minimumScan*20+20*time.Millisecond {
			t.Errorf("%s member scan median = %s, exceeds flatness bound %s", row.Variant, row.MemberScanElapsed, minimumScan*20+20*time.Millisecond)
		}
		if row.DependencyMemberScanElapsed > minimumDependencyScan*20+20*time.Millisecond {
			t.Errorf("%s dependency member scan median = %s, exceeds flatness bound %s", row.Variant, row.DependencyMemberScanElapsed, minimumDependencyScan*20+20*time.Millisecond)
		}
	}
}

func assertProducerChainWorkload(t *testing.T, label string, workload producerChainWorkload) {
	t.Helper()
	if workload.MemberPackages != 1 || workload.MemberSourceFiles != 1 ||
		workload.MemberSyntaxFiles != 1 || workload.MemberTypeInfoPackages != 1 ||
		workload.NonMemberSourceFiles != 0 || workload.NonMemberSyntaxFiles != 0 ||
		workload.NonMemberTypeInfoPackages != 0 || workload.NonMemberIncompleteTypes != 0 {
		t.Errorf("%s workload = %#v, want one typed member and source-free complete non-members", label, workload)
	}
}

func sameProducerChainMemberWorkload(left, right producerChainWorkload) bool {
	return left.MemberPackages == right.MemberPackages &&
		left.MemberSourceFiles == right.MemberSourceFiles &&
		left.MemberSyntaxFiles == right.MemberSyntaxFiles &&
		left.MemberTypeInfoPackages == right.MemberTypeInfoPackages &&
		left.MemberUses == right.MemberUses &&
		left.MemberSelections == right.MemberSelections &&
		left.MemberImportEdges == right.MemberImportEdges &&
		left.MemberReferenceEdges == right.MemberReferenceEdges
}

func primaryRow(t *testing.T, rows []producerChainRow, depth, width int) producerChainRow {
	t.Helper()
	for _, row := range rows {
		if row.Depth == depth && row.Width == width && row.TypeSurface == 0 {
			return row
		}
	}
	t.Fatalf("missing primary producer-chain row depth=%d width=%d", depth, width)
	return producerChainRow{}
}

func assertProducerChainVolumeGrowth(t *testing.T, axis string, fixed, value int, previous, current producerChainRow) {
	t.Helper()
	if current.ProjectionSourceFiles <= previous.ProjectionSourceFiles || current.ProjectionSourceBytes <= previous.ProjectionSourceBytes {
		t.Errorf("%s fixed=%d value=%d projection volume = %d/%d, want growth beyond %d/%d", axis, fixed, value, current.ProjectionSourceFiles, current.ProjectionSourceBytes, previous.ProjectionSourceFiles, previous.ProjectionSourceBytes)
	}
	if current.ExportArtifactCount <= previous.ExportArtifactCount || current.ExportArtifactBytes <= previous.ExportArtifactBytes {
		t.Errorf("%s fixed=%d value=%d export volume = %d/%d, want growth beyond %d/%d", axis, fixed, value, current.ExportArtifactCount, current.ExportArtifactBytes, previous.ExportArtifactCount, previous.ExportArtifactBytes)
	}
	if current.DependencyProjectionSourceFiles <= previous.DependencyProjectionSourceFiles || current.DependencyProjectionSourceBytes <= previous.DependencyProjectionSourceBytes {
		t.Errorf("%s fixed=%d value=%d dependency projection volume = %d/%d, want growth beyond %d/%d", axis, fixed, value, current.DependencyProjectionSourceFiles, current.DependencyProjectionSourceBytes, previous.DependencyProjectionSourceFiles, previous.DependencyProjectionSourceBytes)
	}
	if current.DependencyExportArtifactCount <= previous.DependencyExportArtifactCount || current.DependencyExportArtifactBytes <= previous.DependencyExportArtifactBytes {
		t.Errorf("%s fixed=%d value=%d dependency export volume = %d/%d, want growth beyond %d/%d", axis, fixed, value, current.DependencyExportArtifactCount, current.DependencyExportArtifactBytes, previous.DependencyExportArtifactCount, previous.DependencyExportArtifactBytes)
	}
}

func medianProducerChainDuration(values []time.Duration) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	if len(ordered) == 0 {
		return 0
	}
	if len(ordered)%2 == 1 {
		return ordered[len(ordered)/2]
	}
	return (ordered[len(ordered)/2-1] + ordered[len(ordered)/2]) / 2
}

func assertProducerChainProfile(t *testing.T, path string, variants []producerChainVariant) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening Bazel profile %q: %v", path, err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("opening compressed Bazel profile %q: %v", path, err)
	}
	defer compressed.Close()
	var profile producerChainProfile
	if err := json.NewDecoder(compressed).Decode(&profile); err != nil {
		t.Fatalf("decoding Bazel profile %q: %v", path, err)
	}

	for _, variant := range variants {
		for _, mnemonic := range producerChainActionMnemonics {
			matches := 0
			for _, event := range profile.TraceEvents {
				if event.Category == "action processing" && event.Phase == "X" && event.Args.Mnemonic == mnemonic && strings.Contains(event.Name, variant.Component) {
					if event.Duration <= 0 {
						t.Errorf("profile action event for %s %s has non-positive duration %f", variant.Name, mnemonic, event.Duration)
					}
					matches++
				}
			}
			if matches != 1 {
				t.Errorf("profile action events for %s %s = %d, want exactly one", variant.Name, mnemonic, matches)
			}
		}
	}
}

func assertProducerChainArtifactsDeterministic(t *testing.T, build producerChainBazelBuild, variants []producerChainVariant) {
	t.Helper()
	first := producerChainOutputSnapshot(t, build, variants)
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed while locating the repository")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../.."))
	args := producerChainBazelBuildArgs(build.OutputBase, variants)
	runProducerChainBazelCommand(t, repoRoot, args...)
	second := producerChainOutputSnapshot(t, build, variants)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated Bazel producer-chain build changed map/surface/report bytes")
	}
}

func producerChainOutputSnapshot(t *testing.T, build producerChainBazelBuild, variants []producerChainVariant) map[string][]byte {
	t.Helper()
	actions := readProducerChainExecutionLog(t, build.ExecutionLogPath)
	selected := make(map[string]bool)
	for _, action := range actions {
		if action.Mnemonic == "ArccStdlibMap" && action.TargetLabel == "//:arcc_stdlib_map" {
			selected[action.TargetLabel+"\x00"+action.Mnemonic] = true
		}
		if action.Mnemonic == "ArccCheck" {
			for _, variant := range variants {
				if action.TargetLabel == variant.Component {
					selected[action.TargetLabel+"\x00"+action.Mnemonic] = true
				}
			}
		}
	}
	if len(selected) != len(variants)+1 {
		t.Fatalf("determinism snapshot selected %d actions, want %d", len(selected), len(variants)+1)
	}

	execroot := filepath.Join(build.OutputBase, "execroot", "_main")
	snapshot := make(map[string][]byte)
	for _, action := range actions {
		if !selected[action.TargetLabel+"\x00"+action.Mnemonic] {
			continue
		}
		for _, output := range action.ActualOutputs {
			if !strings.HasSuffix(output.Path, ".stdlib-map.json") && !strings.HasSuffix(output.Path, ".surface.json") && !strings.HasSuffix(output.Path, ".report.json") {
				continue
			}
			path := output.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(execroot, filepath.FromSlash(path))
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading deterministic producer output %q: %v", path, err)
			}
			if len(bytes) == 0 {
				t.Fatalf("deterministic producer output %q is empty", path)
			}
			snapshot[output.Path] = bytes
		}
	}
	return snapshot
}

func makeProducerChainTreeWritable(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			_ = os.Chmod(path, info.Mode().Perm()|0o700)
		} else if info.Mode().IsRegular() {
			_ = os.Chmod(path, info.Mode().Perm()|0o600)
		}
		return nil
	})
}

func printProducerChainTable(rows []producerChainRow, stdlibMapActions int) {
	// This driver invokes only Bazel; native stdlib-map generation is therefore
	// absent by construction rather than a synthetic execution-log counter.
	fmt.Printf("producer-chain-scaling-v1 stdlib_map_actions=%d native_whole_sdk_generation=none(driver-only-bazel)\n", stdlibMapActions)
	fmt.Println("variant depth width type_surface import_graph_actions layout_actions check_actions projection_source_files projection_source_bytes dependency_projection_source_files dependency_projection_source_bytes projection_us layout_us check_loader_us member_scan_us dependency_check_loader_us dependency_member_scan_us check_total_us producer_chain_us export_artifacts export_bytes dependency_export_artifacts dependency_export_bytes root_member_workload dependency_member_workload")
	for _, row := range rows {
		values := []string{
			row.Variant,
			strconv.Itoa(row.Depth),
			strconv.Itoa(row.Width),
			strconv.Itoa(row.TypeSurface),
			strconv.Itoa(row.ImportGraphActions),
			strconv.Itoa(row.LayoutActions),
			strconv.Itoa(row.CheckActions),
			strconv.FormatUint(row.ProjectionSourceFiles, 10),
			strconv.FormatUint(row.ProjectionSourceBytes, 10),
			strconv.FormatUint(row.DependencyProjectionSourceFiles, 10),
			strconv.FormatUint(row.DependencyProjectionSourceBytes, 10),
			strconv.FormatInt(row.ProjectionElapsed.Microseconds(), 10),
			strconv.FormatInt(row.LayoutElapsed.Microseconds(), 10),
			strconv.FormatInt(row.CheckLoaderElapsed.Microseconds(), 10),
			strconv.FormatInt(row.MemberScanElapsed.Microseconds(), 10),
			strconv.FormatInt(row.DependencyCheckLoaderElapsed.Microseconds(), 10),
			strconv.FormatInt(row.DependencyMemberScanElapsed.Microseconds(), 10),
			strconv.FormatInt(row.CheckTotalElapsed.Microseconds(), 10),
			strconv.FormatInt(row.ProducerChainElapsed.Microseconds(), 10),
			strconv.FormatUint(row.ExportArtifactCount, 10),
			strconv.FormatUint(row.ExportArtifactBytes, 10),
			strconv.FormatUint(row.DependencyExportArtifactCount, 10),
			strconv.FormatUint(row.DependencyExportArtifactBytes, 10),
			producerChainMemberWorkloadToken(row.MemberWorkload),
			producerChainMemberWorkloadToken(row.DependencyMemberWorkload),
		}
		fmt.Println(strings.Join(values, " "))
	}
}

func producerChainMemberWorkloadToken(workload producerChainWorkload) string {
	values := []string{
		strconv.Itoa(workload.MemberPackages),
		strconv.Itoa(workload.MemberSourceFiles),
		strconv.Itoa(workload.MemberSyntaxFiles),
		strconv.Itoa(workload.MemberTypeInfoPackages),
		strconv.Itoa(workload.MemberUses),
		strconv.Itoa(workload.MemberSelections),
		strconv.Itoa(workload.MemberImportEdges),
		strconv.Itoa(workload.MemberReferenceEdges),
	}
	return strings.Join(values, ":")
}
