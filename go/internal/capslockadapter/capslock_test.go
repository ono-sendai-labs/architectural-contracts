package capslockadapter

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
)

// TestClassifier_FileHandleUse_Safe protects the complete 22-method set.
func TestClassifier_FileHandleUse_Safe(t *testing.T) {
	// Verify exact count is 22
	if len(fileHandleUseMethods) != 22 {
		t.Errorf("expected exactly 22 methods in fileHandleUseMethods, got %d", len(fileHandleUseMethods))
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, m := range fileHandleUseMethods {
		if seen[m] {
			t.Errorf("duplicate method found in fileHandleUseMethods: %q", m)
		}
		seen[m] = true
	}

	cl, err := buildClassifier(nil)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	// Verify every key is classified as SAFE
	for _, m := range fileHandleUseMethods {
		cat := cl.FunctionCategory("", m)
		if cat != "SAFE" {
			t.Errorf("expected method %q to be classified as SAFE, got %q", m, cat)
		}
	}
}

// TestClassifier_UNANALYZED_Excluded verifies that UNANALYZED helpers are descended through.
func TestClassifier_UNANALYZED_Excluded(t *testing.T) {
	cl, err := buildClassifier(nil)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	// io.ReadAll is a built-in helper normally classified as UNANALYZED.
	// Since UNANALYZED is excluded, it should return "" (allowing traversal).
	cat := cl.FunctionCategory("", "io.ReadAll")
	if cat == "UNANALYZED" {
		t.Errorf("expected UNANALYZED to be excluded from io.ReadAll, but got %q", cat)
	}
}

// TestClassifier_AmbientMinting_FILES verifies that ambient minting functions remain visible.
func TestClassifier_AmbientMinting_FILES(t *testing.T) {
	cl, err := buildClassifier(nil)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	mintingFuncs := []string{"os.Open", "os.ReadFile"}
	for _, f := range mintingFuncs {
		cat := cl.FunctionCategory("", f)
		if cat != "FILES" {
			t.Errorf("expected minting function %q to remain FILES, got %q", f, cat)
		}
	}
}

// TestClassifier_Chdir_Unsafe verifies that (*os.File).Chdir is not safe and retains its system-state-modifying classification.
func TestClassifier_Chdir_Unsafe(t *testing.T) {
	cl, err := buildClassifier(nil)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	cat := cl.FunctionCategory("", "(*os.File).Chdir")
	if cat == "SAFE" {
		t.Errorf("(*os.File).Chdir should not be reclassified as SAFE")
	}
	if cat != "MODIFY_SYSTEM_STATE/CHDIR" {
		t.Errorf("expected (*os.File).Chdir to remain MODIFY_SYSTEM_STATE/CHDIR, got %q", cat)
	}
}

// TestClassifier_FailuresExplicit verifies that malformed input returns an error instead of falling back.
func TestClassifier_FailuresExplicit(t *testing.T) {
	// Passing an invalid capability category via newline injection to trigger parse errors in LoadClassifier
	_, err := buildClassifier([]capanalyzer.InterfaceSymbol{"invalidKey\nfunc invalidFn INVALID_CAPABILITY_NAME"})
	if err == nil {
		t.Errorf("expected buildClassifier to fail with malformed custom classifier input")
	}
}

// TestAdapter_Analyze runs integration tests on pure, filereader, and scope packages.
func TestAdapter_Analyze(t *testing.T) {
	adapter := NewAdapter()

	// 1. Analyze Pure Package
	reqPure := capanalyzer.AnalyzeRequest{
		Packages: []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/pure"},
	}
	findingsPure, err := adapter.Analyze(reqPure)
	if err != nil {
		t.Fatalf("failed to analyze pure package: %v", err)
	}
	if len(findingsPure) != 0 {
		t.Errorf("expected pure package to have 0 findings (ambient-authority-free), got %d findings: %+v", len(findingsPure), findingsPure)
	}

	// 2. Analyze FileReader Package
	reqReader := capanalyzer.AnalyzeRequest{
		Packages: []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/filereader"},
	}
	findingsReader, err := adapter.Analyze(reqReader)
	if err != nil {
		t.Fatalf("failed to analyze filereader package: %v", err)
	}
	if len(findingsReader) == 0 {
		t.Errorf("expected filereader package to have findings, got none")
	}

	hasFiles := false
	for _, f := range findingsReader {
		if f.Capability == "FILES" {
			hasFiles = true
			expectedPkg := "github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/filereader"
			if f.Package != expectedPkg {
				t.Errorf("expected package of FILES capability to be %q, got %q", expectedPkg, f.Package)
			}
			if len(f.CallPath) == 0 {
				t.Errorf("expected non-empty CallPath for FILES finding")
			}
		}
	}
	if !hasFiles {
		t.Errorf("expected filereader package to report FILES capability")
	}

	// 3. Analyze Scope Package (Scope & Test Exclusion tests)
	reqScope := capanalyzer.AnalyzeRequest{
		Packages: []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/scope"},
	}
	findingsScope, err := adapter.Analyze(reqScope)
	if err != nil {
		t.Fatalf("failed to analyze scope package: %v", err)
	}

	hasUnreachedFiles := false
	hasTestAuthority := false

	for _, f := range findingsScope {
		if f.Capability == "FILES" {
			hasUnreachedFiles = true
		}
		if f.Capability == "READ_SYSTEM_STATE" {
			hasTestAuthority = true
		}
	}

	// Verify whole-package scope: Unreached helper was reported
	if !hasUnreachedFiles {
		t.Errorf("expected scope package to report FILES capability from private unreachable helper")
	}

	// Verify _test.go files are excluded: TestSomething from scope_test.go is NOT reported
	if hasTestAuthority {
		t.Errorf("expected scope package NOT to report READ_SYSTEM_STATE capability from scope_test.go")
	}
}
