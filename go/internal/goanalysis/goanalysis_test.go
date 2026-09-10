//go:build integration

package goanalysis_test

import (
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

func loadPackageFacts(root string) (facts.PackageFacts, error) {
	return goanalysis.LoadPackageFacts(goanalysis.LoadRequest{ComponentRoot: root})
}

// verticalSliceAuthority is the fixture authority extended with the fmt
// closure the success fixture legitimately imports: the stdlib enumeration
// decides membership, not the path.
type verticalSliceAuthority struct {
	stdlibauthority.StdlibAuthority
}

func newVerticalSliceAuthority(t *testing.T) stdlibauthority.StdlibAuthority {
	t.Helper()
	base := &extendedAuthority{fixtureAuthority: newFixtureAuthority(t), packages: map[string]bool{}}
	base.packages["fmt"] = true
	return base
}

type extendedAuthority struct {
	fixtureAuthority *fixtureAuthority
	packages         map[string]bool
}

func (e *extendedAuthority) IsStdlibPackage(pkgPath string) bool {
	return e.packages[pkgPath] || e.fixtureAuthority.IsStdlibPackage(pkgPath)
}

func (e *extendedAuthority) SymbolAuthority(id symbol.SymbolID) (stdlibauthority.Classification, error) {
	if facts.SymbolIDPackage(id) == "fmt" {
		return stdlibauthority.Classification{Safe: true}, nil
	}
	return e.fixtureAuthority.SymbolAuthority(id)
}

func (e *extendedAuthority) PackageInitAuthority(pkgPath string) (stdlibauthority.Classification, error) {
	if pkgPath == "fmt" {
		return stdlibauthority.Classification{Safe: true}, nil
	}
	return e.fixtureAuthority.PackageInitAuthority(pkgPath)
}

func (e *extendedAuthority) Evidence(id symbol.SymbolID, cap stdlibauthority.Capability) []stdlibauthority.Frame {
	return e.fixtureAuthority.Evidence(id, cap)
}

func (e *extendedAuthority) Key() stdlibauthority.SDKKey { return e.fixtureAuthority.Key() }

func TestLoadPackageFacts_Success(t *testing.T) {
	// Find absolute path to testdata/success
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute path to testdata: %v", err)
	}

	factsResult, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We expect 2 packages under the root:
	// 1. github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a
	// 2. github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b
	if len(factsResult.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(factsResult.Packages))
	}

	// Packages should be sorted alphabetically by ImportPath
	pkgA := factsResult.Packages[0]
	pkgB := factsResult.Packages[1]

	expectedA := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a"
	expectedB := "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b"

	if pkgA.ImportPath != expectedA {
		t.Errorf("expected package 0 path to be %q, got %q", expectedA, pkgA.ImportPath)
	}
	if pkgB.ImportPath != expectedB {
		t.Errorf("expected package 1 path to be %q, got %q", expectedB, pkgB.ImportPath)
	}

	// Check sorted direct imports for package A: "fmt" and "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	expectedImportsA := []string{
		"fmt",
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts",
	}
	if !reflect.DeepEqual(pkgA.Imports, expectedImportsA) {
		t.Errorf("expected package a imports %v, got %v", expectedImportsA, pkgA.Imports)
	}

	// Check sorted direct imports for package B: "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a"
	expectedImportsB := []string{
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a",
	}
	if !reflect.DeepEqual(pkgB.Imports, expectedImportsB) {
		t.Errorf("expected package b imports %v, got %v", expectedImportsB, pkgB.Imports)
	}

	// Assert exported symbols for Package A
	expectedSymbolsA := []facts.ExportedSymbol{
		// a.go
		{Name: expectedA + ".Hello", File: "a/a.go", Kind: "func"},
		// api.go
		{Name: expectedA + ".CallWithFunc", File: "a/api.go", Kind: "func"},
		{Name: expectedA + ".Identity", File: "a/api.go", Kind: "func"},
		{Name: expectedA + ".ProcessString", File: "a/api.go", Kind: "func"},
		{Name: "(*" + expectedA + ".GreeterImpl).SayHello", File: "a/api.go", Kind: "method", Receiver: "(*" + expectedA + ".GreeterImpl)"},
		{Name: expectedA + ".StringProcessor", File: "a/api.go", Kind: "type"},
		// init.go
		{Name: expectedA + ".init", File: "a/init.go", Kind: "init"},
		// types.go
		{Name: expectedA + ".ExportedConst1", File: "a/types.go", Kind: "const"},
		{Name: expectedA + ".ExportedConst2", File: "a/types.go", Kind: "const"},
		{Name: "(*" + expectedA + ".Base).GetValue", File: "a/types.go", Kind: "method", Receiver: "(*" + expectedA + ".Base)"},
		{Name: "(*" + expectedA + ".Box).Get", File: "a/types.go", Kind: "method", Receiver: "(*" + expectedA + ".Box)"},
		{Name: "(" + expectedA + ".Box).GetVal", File: "a/types.go", Kind: "method", Receiver: "(" + expectedA + ".Box)"},
		{Name: "(" + expectedA + ".GreeterImpl).Greet", File: "a/types.go", Kind: "method", Receiver: "(" + expectedA + ".GreeterImpl)"},
		{Name: expectedA + ".Base", File: "a/types.go", Kind: "type"},
		{Name: expectedA + ".Box", File: "a/types.go", Kind: "type"},
		{Name: expectedA + ".Greeter", File: "a/types.go", Kind: "type"},
		{Name: expectedA + ".GreeterImpl", File: "a/types.go", Kind: "type"},
		{Name: expectedA + ".Wrapper", File: "a/types.go", Kind: "type"},
		{Name: expectedA + ".ExportedVar1", File: "a/types.go", Kind: "var"},
		{Name: expectedA + ".ExportedVar2", File: "a/types.go", Kind: "var"},
	}

	if !reflect.DeepEqual(pkgA.ExportedSymbols, expectedSymbolsA) {
		t.Errorf("expected package a exported symbols to match. Expected:\n%+v\nGot:\n%+v", expectedSymbolsA, pkgA.ExportedSymbols)
	}

	// Assert exported symbols for Package B
	expectedSymbolsB := []facts.ExportedSymbol{
		{Name: expectedB + ".CallGreet", File: "a/b/b.go", Kind: "func"},
		{Name: expectedB + ".Greet", File: "a/b/b.go", Kind: "func"},
		{Name: expectedB + ".TriggerDynamicDispatch", File: "a/b/b.go", Kind: "func"},
		{Name: expectedB + ".TriggerGenericFunc", File: "a/b/b.go", Kind: "func"},
		{Name: expectedB + ".TriggerGenericMethods", File: "a/b/b.go", Kind: "func"},
		{Name: expectedB + ".TriggerHigherOrder", File: "a/b/b.go", Kind: "func"},
		{Name: expectedB + ".TriggerHigherOrderNamed", File: "a/b/b.go", Kind: "func"},
	}

	if !reflect.DeepEqual(pkgB.ExportedSymbols, expectedSymbolsB) {
		t.Errorf("expected package b exported symbols to match. Expected:\n%+v\nGot:\n%+v", expectedSymbolsB, pkgB.ExportedSymbols)
	}

	// The typed reference vocabulary records the inter-package edges of this
	// fixture as import and reference edges (Uses/Selections), sorted and
	// duplicate-free.
	if len(factsResult.Imports) == 0 {
		t.Errorf("expected import edges, got none")
	}
	for _, e := range factsResult.Imports {
		if e.ImportingPackage == expectedA {
			continue
		}
		if e.ImportingPackage != expectedB {
			t.Errorf("import edge from non-member %q leaked into the facts: %+v", e.ImportingPackage, e)
		}
	}
	for _, e := range factsResult.References {
		if e.FromPackage != expectedA && e.FromPackage != expectedB {
			t.Errorf("reference edge from non-member %q leaked into the facts: %+v", e.FromPackage, e)
		}
	}

	// Verify repeat-load determinism (AC5)
	factsResult2, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("unexpected error on repeated load: %v", err)
	}
	if !reflect.DeepEqual(factsResult.References, factsResult2.References) {
		t.Errorf("expected References to be identical on repeated load.\nFirst:\n%+v\nSecond:\n%+v", factsResult.References, factsResult2.References)
	}
	if !reflect.DeepEqual(factsResult.Imports, factsResult2.Imports) {
		t.Errorf("expected Imports to be identical on repeated load.\nFirst:\n%+v\nSecond:\n%+v", factsResult.Imports, factsResult2.Imports)
	}
}

func TestLoadPackageFacts_Errors(t *testing.T) {
	// 1. Invalid component root (Scenario 3)
	invalidFacts, err := loadPackageFacts("/nonexistent/directory")
	if err == nil {
		t.Errorf("expected error on nonexistent component root, got nil")
	}
	if len(invalidFacts.Packages) != 0 || len(invalidFacts.References) != 0 {
		t.Errorf("expected empty facts on invalid root error, got %+v", invalidFacts)
	}

	// 2. Broken syntax package (Scenario 2)
	root, err := filepath.Abs("testdata/broken_syntax")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}
	brokenFacts, err := loadPackageFacts(root)
	if err == nil {
		t.Fatalf("expected error on package load with broken syntax, got nil")
	}
	if len(brokenFacts.Packages) != 0 || len(brokenFacts.References) != 0 {
		t.Errorf("expected empty facts on broken-package load error, got %+v", brokenFacts)
	}

	// Verify the error contains loader diagnostics/syntax error text
	errMsg := err.Error()
	if !strings.Contains(errMsg, "package load errors:") {
		t.Errorf("expected error to contain %q, got: %q", "package load errors:", errMsg)
	}
	if !strings.Contains(errMsg, "cannot use") && !strings.Contains(errMsg, "declared and not used") {
		t.Errorf("expected error to contain compiler diagnostics, got: %q", errMsg)
	}
}

func TestValidateInterfaceFiles(t *testing.T) {
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute path to testdata: %v", err)
	}

	loaded, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("failed to load package facts: %v", err)
	}

	tests := []struct {
		name    string
		files   []string
		wantErr string
	}{
		{
			name:  "valid relative files",
			files: []string{"a/a.go", "a/api.go", "a/b/b.go"},
		},
		{
			name:    "absolute path rejected",
			files:   []string{filepath.Join(root, "a/a.go")},
			wantErr: "is absolute",
		},
		{
			name:    "escaping path rejected",
			files:   []string{"../success/a/a.go"},
			wantErr: "escapes the component root",
		},
		{
			name:    "nonexistent file rejected",
			files:   []string{"a/nonexistent.go"},
			wantErr: "does not exist",
		},
		{
			name:    "directory path rejected",
			files:   []string{"a/b"},
			wantErr: "is not a regular file",
		},
		{
			name:    "non-Go file outside package set rejected",
			files:   []string{"a/non_go_file.txt"},
			wantErr: "does not belong to any loaded Go package",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := goanalysis.ValidateInterfaceFiles(root, tt.files, loaded)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestValidateInterfaceFiles_ReturnsDeterministicExclusions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/exclusions\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gatedExpression := "!" + runtime.GOOS
	filenameGated := "a_windows.go"
	if runtime.GOOS == "windows" {
		filenameGated = "a_linux.go"
	}
	mixedConstraint := "mixed_windows.go"
	mixedSuffix := "_windows.go"
	if runtime.GOOS == "windows" {
		mixedConstraint = "mixed_linux.go"
		mixedSuffix = "_linux.go"
	}
	files := map[string]string{
		"api.go":          "package exclusions\n",
		"z_gated.go":      "//go:build " + gatedExpression + "\n\npackage exclusions\n",
		"legacy_gated.go": "// +build " + gatedExpression + "\n\npackage exclusions\n",
		filenameGated:     "package exclusions\n",
		mixedConstraint:   "//go:build " + runtime.GOOS + "\n\npackage exclusions\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("loadPackageFacts() error = %v", err)
	}
	exclusions, err := goanalysis.ValidateInterfaceFiles(root, []string{"z_gated.go", "legacy_gated.go", filenameGated, mixedConstraint, "api.go"}, loaded)
	if err != nil {
		t.Fatalf("ValidateInterfaceFiles() error = %v", err)
	}
	if len(exclusions) != 4 {
		t.Fatalf("exclusions = %#v, want four exclusions", exclusions)
	}
	if exclusions[0].File != filenameGated || exclusions[1].File != "legacy_gated.go" || exclusions[2].File != mixedConstraint || exclusions[3].File != "z_gated.go" {
		t.Fatalf("exclusions = %#v, want stable file order", exclusions)
	}
	for _, exclusion := range exclusions {
		if exclusion.Constraint == "" {
			t.Fatalf("exclusions = %#v, want specific constraints", exclusions)
		}
	}
	if exclusions[1].Constraint != "// +build "+gatedExpression {
		t.Errorf("legacy constraint = %q, want %q", exclusions[1].Constraint, "// +build "+gatedExpression)
	}
	if exclusions[3].Constraint != "//go:build "+gatedExpression {
		t.Errorf("gated constraint = %q, want %q", exclusions[3].Constraint, "//go:build "+gatedExpression)
	}
	if exclusions[2].Constraint != `filename suffix "`+mixedSuffix+`"` {
		t.Errorf("mixed constraint = %q, want filename suffix %q", exclusions[2].Constraint, mixedSuffix)
	}

	if matched, err := build.Default.MatchFile(root, "z_gated.go"); err != nil || matched {
		t.Fatalf("build.Default.MatchFile() = (%v, %v), want (false, nil)", matched, err)
	}
}

func TestValidateInterfaceFiles_AllExcludedFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/all-excluded\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := "api_" + runtime.GOOS + ".go"
	if err := os.WriteFile(filepath.Join(root, file), []byte("//go:build never\n\npackage excluded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep.go"), []byte("package excluded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("loadPackageFacts() error = %v", err)
	}

	exclusions, err := goanalysis.ValidateInterfaceFiles(root, []string{file}, loaded)
	if err == nil {
		t.Fatalf("ValidateInterfaceFiles() error = nil, exclusions = %#v", exclusions)
	}
	if !strings.Contains(err.Error(), "no interface file survives") || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), "never") {
		t.Fatalf("ValidateInterfaceFiles() error = %q, want no-survivor file/constraint context", err)
	}
}

func TestValidateInterfaceFiles_RejectsNonRegularCachedPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture.test/nonregular\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture go.mod: %v", err)
	}
	pkgDir := filepath.Join(root, "a")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create package dir: %v", err)
	}
	goFile := filepath.Join(pkgDir, "a.go")
	if err := os.WriteFile(goFile, []byte("package a\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}

	loaded, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("failed to load package facts: %v", err)
	}

	// Replace the cached member source path with a non-regular filesystem entry
	// (a FIFO) after loading, to prove validation rejects it rather than accepting
	// it because it is merely "not a directory".
	if err := os.Remove(goFile); err != nil {
		t.Fatalf("failed to remove fixture file: %v", err)
	}
	if err := syscall.Mkfifo(goFile, 0o644); err != nil {
		t.Skipf("mkfifo not supported on this platform: %v", err)
	}

	_, err = goanalysis.ValidateInterfaceFiles(root, []string{"a/a.go"}, loaded)
	if err == nil {
		t.Fatalf("expected error for non-regular cached path, got nil")
	}
	if !strings.Contains(err.Error(), "is not a regular file") {
		t.Errorf("expected error to contain %q, got: %q", "is not a regular file", err.Error())
	}
}

func TestVerticalSliceVerdict(t *testing.T) {
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute path to testdata: %v", err)
	}

	// 1. Call LoadPackageFacts
	loadedFacts, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("failed to load package facts: %v", err)
	}

	// 2. Validate fixture interface files
	interfaceFiles := []string{"a/a.go", "a/api.go", "a/types.go", "a/init.go"}
	_, err = goanalysis.ValidateInterfaceFiles(root, interfaceFiles, loadedFacts)
	if err != nil {
		t.Fatalf("failed to validate interface files: %v", err)
	}

	// 3. Construct manifest
	testManifest := manifest.Manifest{
		Name:           "success-component",
		InterfaceFiles: interfaceFiles,
	}

	// 4. Pass real facts to checker.Check with empty capabilities / dependency interfaces
	inputs := checker.Inputs{
		Manifest:  testManifest,
		Facts:     loadedFacts,
		DepIfaces: nil,
		Authority: newVerticalSliceAuthority(t),
		SDKKey:    testSDKKey(),
	}
	conformanceReport, checkErr := checker.Check(inputs)
	if checkErr != nil {
		t.Fatalf("unexpected tool error: %v", checkErr)
	}

	// 5. Assert the rendered Pillar-1 report contains the expected undeclared
	// dependency. The typed scan also reports every unowned reference at its
	// exact site, so the fixture's references into the facts package are
	// additional UNDECLARED_DEPENDENCY violations; the pinned edge is the
	// import of the facts package.
	expectedImport := "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	var sawFactsImport bool
	for _, v := range conformanceReport.Violations {
		if v.Kind != report.UndeclaredDependency {
			t.Errorf("expected violation kind %s, got %s: %s", report.UndeclaredDependency, v.Kind, v.Message)
		}
		if strings.Contains(v.Message, `imports undeclared dependency "`+expectedImport+`"`) {
			sawFactsImport = true
		}
	}
	if !sawFactsImport {
		t.Fatalf("expected the facts-package import violation, got %+v", conformanceReport.Violations)
	}

	renderedReport := report.RenderText(conformanceReport)
	expectedRendered := `- [UNDECLARED_DEPENDENCY] package "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a" imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts" at a/a.go:5`
	if !strings.Contains(renderedReport, expectedRendered) {
		t.Errorf("rendered report does not match expected pattern. Got:\n%s\nExpected to contain:\n%s", renderedReport, expectedRendered)
	}

	// 6. Print concise logged demo of imports, symbols, and report (runs when test is verbose)
	t.Log("======================================== DEMO START ========================================")
	t.Logf("Component Root: %s", root)
	t.Log("Loaded Package Imports:")
	for _, p := range loadedFacts.Packages {
		t.Logf("  Package %q imports: %v", p.ImportPath, p.Imports)
	}

	t.Log("\nExported Symbol-to-File Mappings:")
	for _, p := range loadedFacts.Packages {
		for _, sym := range p.ExportedSymbols {
			t.Logf("  Symbol %q (Kind: %q) in File: %q", sym.Name, sym.Kind, sym.File)
		}
	}

	t.Log("\nRendered Pillar-1 Conformance Report:")
	t.Log(renderedReport)
	t.Log("========================================  DEMO END  ========================================")
}

func TestValidateInterfaceFiles_UsesCachedMembership(t *testing.T) {
	tmp := t.TempDir()

	// Write simple go.mod and two Go files
	goMod := "module tempcomp\n\ngo 1.20\n"
	mainGo := "package tempcomp\n\nfunc Hello() {}\n"
	extraGo := "package tempcomp\n\nfunc Extra() {}\n"

	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "extra.go"), []byte(extraGo), 0644); err != nil {
		t.Fatalf("failed to write extra.go: %v", err)
	}

	// Load facts (this populates cached membership with main.go and extra.go)
	loaded, err := loadPackageFacts(tmp)
	if err != nil {
		t.Fatalf("failed to load package facts: %v", err)
	}

	// Now modify the directory on disk:
	// 1. Delete main.go (which is in the cache). Validation should reject it with "does not exist".
	if err := os.Remove(filepath.Join(tmp, "main.go")); err != nil {
		t.Fatalf("failed to remove main.go: %v", err)
	}

	// 2. Create other.go (which is NOT in the cache). Validation should reject it with "does not belong".
	otherGo := "package tempcomp\n\nfunc Other() {}\n"
	if err := os.WriteFile(filepath.Join(tmp, "other.go"), []byte(otherGo), 0644); err != nil {
		t.Fatalf("failed to write other.go: %v", err)
	}

	// Prove that extra.go is STILL accepted because it exists on disk AND is in the cache
	if _, err := goanalysis.ValidateInterfaceFiles(tmp, []string{"extra.go"}, loaded); err != nil {
		t.Errorf("expected extra.go to be accepted, got: %v", err)
	}

	// Prove that main.go is rejected because it does not exist on disk
	_, err = goanalysis.ValidateInterfaceFiles(tmp, []string{"main.go"}, loaded)
	if err == nil {
		t.Errorf("expected main.go to be rejected (does not exist), got nil")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error containing 'does not exist', got: %v", err)
	}

	// Prove that other.go is rejected because it was not in the original loaded membership
	_, err = goanalysis.ValidateInterfaceFiles(tmp, []string{"other.go"}, loaded)
	if err == nil {
		t.Errorf("expected other.go to be rejected (not in loaded membership), got nil")
	} else if !strings.Contains(err.Error(), "does not belong to any loaded Go package") {
		t.Errorf("expected error containing 'does not belong to any loaded Go package', got: %v", err)
	}
}

func TestGenericReceiverMethodOutsideInterfaceIsDetected(t *testing.T) {
	tmp := t.TempDir()

	goMod := "module generictest\n\ngo 1.21\n"
	// Box[T] is declared in types.go, which will be an interface file.
	typesGo := "package generictest\n\ntype Box[T any] struct {\n\tVal T\n}\n"
	// Get is an exported method on the generic receiver, declared in a
	// non-interface file. Without bracket-free receiver-key normalization,
	// this method's receiver key "(*generictest.Box[T])" would not match
	// the type declaration key "generictest.Box", so FR4 would silently
	// miss the violation.
	methodGo := "package generictest\n\nfunc (b *Box[T]) Get() T {\n\treturn b.Val\n}\n"

	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "types.go"), []byte(typesGo), 0o644); err != nil {
		t.Fatalf("failed to write types.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "method.go"), []byte(methodGo), 0o644); err != nil {
		t.Fatalf("failed to write method.go: %v", err)
	}

	loadedFacts, err := loadPackageFacts(tmp)
	if err != nil {
		t.Fatalf("failed to load package facts: %v", err)
	}

	interfaceFiles := []string{"types.go"}
	if _, err := goanalysis.ValidateInterfaceFiles(tmp, interfaceFiles, loadedFacts); err != nil {
		t.Fatalf("failed to validate interface files: %v", err)
	}

	inputs := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "generic-component",
			InterfaceFiles: interfaceFiles,
		},
		Facts:     loadedFacts,
		Authority: newVerticalSliceAuthority(t),
		SDKKey:    testSDKKey(),
	}
	conformanceReport, checkErr := checker.Check(inputs)
	if checkErr != nil {
		t.Fatalf("unexpected tool error: %v", checkErr)
	}

	var found bool
	for _, v := range conformanceReport.Violations {
		if v.Kind == report.MethodOutsideInterface && v.Location.File == "method.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a MethodOutsideInterface violation for method.go, got violations: %+v", conformanceReport.Violations)
	}
}

func TestLoadPackageFacts_Signature(t *testing.T) {
	var _ func(goanalysis.LoadRequest) (facts.PackageFacts, error) = goanalysis.LoadPackageFacts
}

func TestLoadPackageFacts_DeclaredMemberOutsideComponentRoot(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/workspace\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	componentRoot := filepath.Join(workspace, "component")
	outsideRoot := filepath.Join(workspace, "outside")
	unrelatedRoot := filepath.Join(componentRoot, "unrelated")
	for _, dir := range []string{componentRoot, outsideRoot, unrelatedRoot} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(componentRoot, "api.go"):       "package component\n\nfunc API() {}\n",
		filepath.Join(outsideRoot, "outside.go"):     "package outside\n\nimport \"os\"\n\nfunc Outside() { _, _ = os.Getwd() }\n",
		filepath.Join(unrelatedRoot, "unrelated.go"): "package unrelated\n\nfunc Unrelated() {}\n",
	}
	for file, content := range files {
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := goanalysis.LoadPackageFacts(goanalysis.LoadRequest{
		ComponentRoot:  componentRoot,
		Members:        []string{"example.com/workspace/outside"},
		InterfaceFiles: []string{"api.go"},
	})
	if err != nil {
		t.Fatalf("LoadPackageFacts() error = %v", err)
	}

	var paths []string
	for _, pkg := range loaded.Packages {
		paths = append(paths, pkg.ImportPath)
	}
	want := []string{"example.com/workspace/component", "example.com/workspace/outside"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("loaded package paths = %v, want %v", paths, want)
	}
	if len(loaded.Packages[1].ExportedSymbols) == 0 ||
		loaded.Packages[1].ExportedSymbols[0].Name != "example.com/workspace/outside.Outside" {
		t.Fatalf("outside package symbols = %+v, want Outside", loaded.Packages[1].ExportedSymbols)
	}
}

func TestLoadPackageFacts_DeclaredMemberMustResolve(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/workspace\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "api.go"), []byte("package workspace\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := goanalysis.LoadPackageFacts(goanalysis.LoadRequest{
		ComponentRoot:  root,
		Members:        []string{"example.com/workspace/missing"},
		InterfaceFiles: []string{"api.go"},
	})
	if err == nil || !strings.Contains(err.Error(), "example.com/workspace/missing") {
		t.Fatalf("LoadPackageFacts() error = %v, want offending member", err)
	}
}

func TestLoadPackageFacts_DeclaredMemberWithNoSourceFails(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/workspace\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "api.go"), []byte("package workspace\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "empty", "empty.go"), []byte("//go:build never\n\npackage empty\n"), 0644); err != nil {
		t.Fatal(err)
	}

	member := "example.com/workspace/empty"
	_, err := goanalysis.LoadPackageFacts(goanalysis.LoadRequest{
		ComponentRoot:  root,
		Members:        []string{member},
		InterfaceFiles: []string{"api.go"},
	})
	if err == nil || !strings.Contains(err.Error(), member) || !strings.Contains(err.Error(), "no source package") {
		t.Fatalf("LoadPackageFacts() error = %v, want %q and no-source context", err, member)
	}
}

func TestLoadPackageFacts_BodilessMemberIsHardError(t *testing.T) {
	tmpDir := t.TempDir()
	sdkRoot := filepath.Join(tmpDir, "mock_sdk")
	if err := os.MkdirAll(filepath.Join(sdkRoot, "fmt"), 0755); err != nil {
		t.Fatalf("failed to create mock SDK dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sdkRoot, "fmt", "fmt.go"), []byte("package fmt\n"), 0644); err != nil {
		t.Fatalf("failed to write mock SDK source: %v", err)
	}

	layoutM10Path := filepath.Join(tmpDir, "package-layout-m10.json")
	layoutM10JSON := fmt.Sprintf(`{
		"go_sdk_root": %q,
		"roots": ["example.com/bodiless_member"],
		"packages": [
			{
				"id": "example.com/bodiless_member",
				"name": "bodiless_member",
				"pkgPath": "example.com/bodiless_member",
				"goFiles": [],
				"compiledGoFiles": [],
				"imports": {},
				"is_stdlib": false
			}
		]
	}`, sdkRoot)

	if err := os.WriteFile(layoutM10Path, []byte(layoutM10JSON), 0644); err != nil {
		t.Fatalf("failed to write M10 layout file: %v", err)
	}

	err := packagelayout.WithDriverEnv(layoutM10Path, tmpDir, func() error {
		_, err := goanalysis.LoadPackageFacts(goanalysis.LoadRequest{
			ComponentRoot: tmpDir,
			Members:       []string{"example.com/bodiless_member"},
		})
		return err
	})
	if err == nil {
		t.Fatalf("expected error for bodiless member (M10), got nil")
	}
	if !strings.Contains(err.Error(), "has no source files") && !strings.Contains(err.Error(), "has no source package") {
		t.Errorf("expected M10 error message about missing source files, got %v", err)
	}
}
