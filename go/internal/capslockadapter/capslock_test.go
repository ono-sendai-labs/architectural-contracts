package capslockadapter

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
)

// TestClassifier_FileHandleUse_Safe protects the complete 22-method set.
func TestClassifier_FileHandleUse_Safe(t *testing.T) {
	expectedFileHandleUseMethods := []string{
		"(*os.File).Chmod", "(*os.File).Chown", "(*os.File).Close", "(*os.File).Fd",
		"(*os.File).Name", "(*os.File).Read", "(*os.File).ReadAt", "(*os.File).ReadDir",
		"(*os.File).ReadFrom", "(*os.File).Readdir", "(*os.File).Readdirnames",
		"(*os.File).Seek", "(*os.File).SetDeadline", "(*os.File).SetReadDeadline",
		"(*os.File).SetWriteDeadline", "(*os.File).Stat", "(*os.File).Sync",
		"(*os.File).SyscallConn", "(*os.File).Truncate", "(*os.File).Write",
		"(*os.File).WriteAt", "(*os.File).WriteString",
	}

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

	// Verify exact match between fileHandleUseMethods (production) and expectedFileHandleUseMethods
	expectedMap := make(map[string]bool)
	for _, m := range expectedFileHandleUseMethods {
		expectedMap[m] = true
	}

	prodMap := make(map[string]bool)
	for _, m := range fileHandleUseMethods {
		prodMap[m] = true
		if !expectedMap[m] {
			t.Errorf("unexpected method found in production fileHandleUseMethods: %q", m)
		}
	}

	for _, m := range expectedFileHandleUseMethods {
		if !prodMap[m] {
			t.Errorf("missing expected method in production fileHandleUseMethods: %q", m)
		}
	}

	cl, err := buildClassifier(nil)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	// Verify every key is classified as SAFE
	for _, m := range expectedFileHandleUseMethods {
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
	if cat != "" {
		t.Errorf("expected empty category (allowing traversal) for io.ReadAll, got %q", cat)
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

// TestAdapter_Analyze_EmptyPackages verifies that an empty package request is rejected with an error.
func TestAdapter_Analyze_EmptyPackages(t *testing.T) {
	adapter := NewAdapter()
	req := capanalyzer.AnalyzeRequest{
		Packages: []string{},
	}
	_, err := adapter.Analyze(req)
	if err == nil {
		t.Errorf("expected Analyze to fail on empty package request, but it succeeded")
	}
}

// TestAdapter_Analyze_PruneAtError verifies that non-empty PruneAt is rejected with an error.
func TestAdapter_Analyze_PruneAtError(t *testing.T) {
	adapter := NewAdapter()
	req := capanalyzer.AnalyzeRequest{
		Packages: []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/pure"},
		PruneAt:  []capanalyzer.InterfaceSymbol{"some.Symbol"},
	}
	_, err := adapter.Analyze(req)
	if err == nil {
		t.Errorf("expected Analyze to fail when PruneAt is non-empty, but it succeeded")
	}
}

// TestMapClass verifies that capability taxonomy mapping is handled safely and rejects unknown ones.
func TestMapClass(t *testing.T) {
	trueCaps := []string{
		"FILES", "NETWORK", "READ_SYSTEM_STATE", "MODIFY_SYSTEM_STATE",
		"OPERATING_SYSTEM", "SYSTEM_CALLS", "EXEC", "RUNTIME",
	}
	defeatingCaps := []string{
		"ARBITRARY_EXECUTION", "CGO", "UNSAFE_POINTER", "REFLECT", "UNANALYZED",
	}

	for _, tc := range trueCaps {
		class, err := mapClass(tc)
		if err != nil {
			t.Errorf("expected %q to map without error, got err: %v", tc, err)
		}
		if class != capanalyzer.TrueAuthority {
			t.Errorf("expected %q to map to TrueAuthority, got %q", tc, class)
		}
	}

	for _, dc := range defeatingCaps {
		class, err := mapClass(dc)
		if err != nil {
			t.Errorf("expected %q to map without error, got err: %v", dc, err)
		}
		if class != capanalyzer.AnalysisDefeating {
			t.Errorf("expected %q to map to AnalysisDefeating, got %q", dc, class)
		}
	}

	unknownCaps := []string{"UNSPECIFIED", "SAFE", "SOMETHING_NEW", ""}
	for _, uc := range unknownCaps {
		_, err := mapClass(uc)
		if err == nil {
			t.Errorf("expected %q to be rejected with error, but it succeeded", uc)
		}
	}
}

// TestDeterministicSorting verifies that findings are sorted stably and deterministically.
func TestDeterministicSorting(t *testing.T) {
	findings := []capanalyzer.CapabilityFinding{
		{
			Package:    "pkgB",
			Capability: "FILES",
			Class:      capanalyzer.TrueAuthority,
		},
		{
			Package:    "pkgA",
			Capability: "NETWORK",
			Class:      capanalyzer.TrueAuthority,
		},
		{
			Package:    "pkgA",
			Capability: "FILES",
			Class:      capanalyzer.TrueAuthority,
			CallPath: []capanalyzer.Frame{
				{Func: "funcB", File: "b.go", Line: 10},
			},
		},
		{
			Package:    "pkgA",
			Capability: "FILES",
			Class:      capanalyzer.TrueAuthority,
			CallPath: []capanalyzer.Frame{
				{Func: "funcA", File: "a.go", Line: 5},
			},
		},
	}

	sortFindings(findings)

	expected := []capanalyzer.CapabilityFinding{
		{
			Package:    "pkgA",
			Capability: "FILES",
			Class:      capanalyzer.TrueAuthority,
			CallPath: []capanalyzer.Frame{
				{Func: "funcA", File: "a.go", Line: 5},
			},
		},
		{
			Package:    "pkgA",
			Capability: "FILES",
			Class:      capanalyzer.TrueAuthority,
			CallPath: []capanalyzer.Frame{
				{Func: "funcB", File: "b.go", Line: 10},
			},
		},
		{
			Package:    "pkgA",
			Capability: "NETWORK",
			Class:      capanalyzer.TrueAuthority,
		},
		{
			Package:    "pkgB",
			Capability: "FILES",
			Class:      capanalyzer.TrueAuthority,
		},
	}

	if len(findings) != len(expected) {
		t.Fatalf("expected %d findings, got %d", len(expected), len(findings))
	}

	for i, f := range findings {
		exp := expected[i]
		if f.Package != exp.Package || f.Capability != exp.Capability {
			t.Errorf("at index %d: expected Package/Capability %s/%s, got %s/%s", i, exp.Package, exp.Capability, f.Package, f.Capability)
		}
		if len(f.CallPath) != len(exp.CallPath) {
			t.Errorf("at index %d: expected CallPath length %d, got %d", i, len(exp.CallPath), len(f.CallPath))
			continue
		}
		for j, fr := range f.CallPath {
			expFr := exp.CallPath[j]
			if fr.Func != expFr.Func || fr.File != expFr.File || fr.Line != expFr.Line {
				t.Errorf("at index %d, frame %d: expected Frame %+v, got %+v", i, j, expFr, fr)
			}
		}
	}
}
