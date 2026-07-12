//go:build integration

package goanalysis_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestLoadPackageFacts_Success(t *testing.T) {
	// Find absolute path to testdata/success
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute path to testdata: %v", err)
	}

	loadRes, err := goanalysis.LoadPackageFacts(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	factsResult := loadRes.Facts

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
		{Name: expectedA + ".Identity", File: "a/api.go", Kind: "func"},
		{Name: "(*" + expectedA + ".GreeterImpl).SayHello", File: "a/api.go", Kind: "method", Receiver: "(*" + expectedA + ".GreeterImpl)"},
		// init.go
		{Name: expectedA + ".init", File: "a/init.go", Kind: "init"},
		// types.go
		{Name: expectedA + ".ExportedConst1", File: "a/types.go", Kind: "const"},
		{Name: expectedA + ".ExportedConst2", File: "a/types.go", Kind: "const"},
		{Name: "(*" + expectedA + ".Base).GetValue", File: "a/types.go", Kind: "method", Receiver: "(*" + expectedA + ".Base)"},
		{Name: "(*" + expectedA + ".Box[T]).Get", File: "a/types.go", Kind: "method", Receiver: "(*" + expectedA + ".Box[T])"},
		{Name: "(" + expectedA + ".Box[T]).GetVal", File: "a/types.go", Kind: "method", Receiver: "(" + expectedA + ".Box[T])"},
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
		{Name: expectedB + ".Greet", File: "a/b/b.go", Kind: "func"},
	}

	if !reflect.DeepEqual(pkgB.ExportedSymbols, expectedSymbolsB) {
		t.Errorf("expected package b exported symbols to match. Expected:\n%+v\nGot:\n%+v", expectedSymbolsB, pkgB.ExportedSymbols)
	}

	if len(factsResult.CallEdges) != 0 {
		t.Errorf("expected CallEdges to be empty")
	}
}

func TestLoadPackageFacts_Errors(t *testing.T) {
	// 1. Invalid component root (Scenario 3)
	invalidLoadRes, err := goanalysis.LoadPackageFacts("/nonexistent/directory")
	if err == nil {
		t.Errorf("expected error on nonexistent component root, got nil")
	}
	invalidFacts := invalidLoadRes.Facts
	if len(invalidFacts.Packages) != 0 || len(invalidFacts.CallEdges) != 0 {
		t.Errorf("expected empty facts on invalid root error, got %+v", invalidFacts)
	}

	// 2. Broken syntax package (Scenario 2)
	root, err := filepath.Abs("testdata/broken_syntax")
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}
	brokenLoadRes, err := goanalysis.LoadPackageFacts(root)
	if err == nil {
		t.Fatalf("expected error on package load with broken syntax, got nil")
	}
	brokenFacts := brokenLoadRes.Facts
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

	loaded, err := goanalysis.LoadPackageFacts(root)
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
			wantErr: "is a directory",
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

func TestVerticalSliceVerdict(t *testing.T) {
	root, err := filepath.Abs("testdata/success")
	if err != nil {
		t.Fatalf("failed to get absolute path to testdata: %v", err)
	}

	// 1. Call LoadPackageFacts
	loadedFacts, err := goanalysis.LoadPackageFacts(root)
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
		Facts:     loadedFacts.Facts,
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
	for _, p := range loadedFacts.Facts.Packages {
		t.Logf("  Package %q imports: %v", p.ImportPath, p.Imports)
	}

	t.Log("\nExported Symbol-to-File Mappings:")
	for _, p := range loadedFacts.Facts.Packages {
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
	loaded, err := goanalysis.LoadPackageFacts(tmp)
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
