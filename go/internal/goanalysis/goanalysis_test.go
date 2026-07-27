//go:build integration

package goanalysis_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func loadPackageFacts(root string) (facts.PackageFacts, error) {
	return goanalysis.LoadPackageFacts(goanalysis.LoadRequest{ComponentRoot: root})
}

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

	// Check IsStdlib (should be false for component packages)
	if pkgA.IsStdlib {
		t.Errorf("expected package a IsStdlib to be false")
	}
	if pkgB.IsStdlib {
		t.Errorf("expected package b IsStdlib to be false")
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

	// Expected inter-package call edges sorted alphabetically by Caller then Callee
	expectedCallEdges := []facts.CallEdge{
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.CallWithFunc",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.callback",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.Hello",
			Callee:          "fmt.Println",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.ProcessString",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.stringCallback",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.init",
			Callee:          "fmt.init",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.init",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts.init",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.init#1",
			Callee:          "fmt.Println",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.CallGreet",
			Callee:          "(github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.GreeterImpl).Greet",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.Greet",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.Hello",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.TriggerGenericFunc",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.Identity",
			PassesFuncValue: true,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.TriggerGenericMethods",
			Callee:          "(*github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.Box).Get",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.TriggerGenericMethods",
			Callee:          "(github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.Box).GetVal",
			PassesFuncValue: false,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.TriggerHigherOrder",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.CallWithFunc",
			PassesFuncValue: true,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.TriggerHigherOrderNamed",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.ProcessString",
			PassesFuncValue: true,
		},
		{
			Caller:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a/b.init",
			Callee:          "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a.init",
			PassesFuncValue: false,
		},
	}

	if !reflect.DeepEqual(factsResult.CallEdges, expectedCallEdges) {
		t.Errorf("expected CallEdges to match.\nExpected (%d):\n%+v\nGot (%d):\n%+v", len(expectedCallEdges), expectedCallEdges, len(factsResult.CallEdges), factsResult.CallEdges)
	}

	// Verify repeat-load determinism and duplicate-edge coverage (AC5)
	factsResult2, err := loadPackageFacts(root)
	if err != nil {
		t.Fatalf("unexpected error on repeated load: %v", err)
	}
	if !reflect.DeepEqual(factsResult.CallEdges, factsResult2.CallEdges) {
		t.Errorf("expected CallEdges to be identical on repeated load.\nFirst load:\n%+v\nSecond load:\n%+v", factsResult.CallEdges, factsResult2.CallEdges)
	}
}

func TestLoadPackageFacts_Errors(t *testing.T) {
	// 1. Invalid component root (Scenario 3)
	invalidFacts, err := loadPackageFacts("/nonexistent/directory")
	if err == nil {
		t.Errorf("expected error on nonexistent component root, got nil")
	}
	if len(invalidFacts.Packages) != 0 || len(invalidFacts.CallEdges) != 0 {
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
	if len(brokenFacts.Packages) != 0 || len(brokenFacts.CallEdges) != 0 {
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
			err := goanalysis.ValidateInterfaceFiles(root, tt.files, loaded)
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

	err = goanalysis.ValidateInterfaceFiles(root, []string{"a/a.go"}, loaded)
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
	err = goanalysis.ValidateInterfaceFiles(root, interfaceFiles, loadedFacts)
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
		Caps:      nil,
	}
	conformanceReport := checker.Check(inputs)

	// 5. Assert the rendered Pillar-1 report contains the expected undeclared dependency
	if len(conformanceReport.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(conformanceReport.Violations))
	}
	v := conformanceReport.Violations[0]
	if v.Kind != report.UndeclaredDependency {
		t.Errorf("expected violation kind %s, got %s", report.UndeclaredDependency, v.Kind)
	}
	expectedImport := "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	if !strings.Contains(v.Message, expectedImport) {
		t.Errorf("expected violation to contain %q, got: %q", expectedImport, v.Message)
	}

	renderedReport := report.RenderText(conformanceReport)
	expectedRendered := `Component: success-component

Violations:
- [UNDECLARED_DEPENDENCY] package "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a" imports undeclared dependency "github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
`
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
	if err := goanalysis.ValidateInterfaceFiles(tmp, []string{"extra.go"}, loaded); err != nil {
		t.Errorf("expected extra.go to be accepted, got: %v", err)
	}

	// Prove that main.go is rejected because it does not exist on disk
	err = goanalysis.ValidateInterfaceFiles(tmp, []string{"main.go"}, loaded)
	if err == nil {
		t.Errorf("expected main.go to be rejected (does not exist), got nil")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error containing 'does not exist', got: %v", err)
	}

	// Prove that other.go is rejected because it was not in the original loaded membership
	err = goanalysis.ValidateInterfaceFiles(tmp, []string{"other.go"}, loaded)
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
	if err := goanalysis.ValidateInterfaceFiles(tmp, interfaceFiles, loadedFacts); err != nil {
		t.Fatalf("failed to validate interface files: %v", err)
	}

	inputs := checker.Inputs{
		Manifest: manifest.Manifest{
			Name:           "generic-component",
			InterfaceFiles: interfaceFiles,
		},
		Facts: loadedFacts,
	}
	conformanceReport := checker.Check(inputs)

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
