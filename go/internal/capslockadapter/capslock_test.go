//go:build integration

package capslockadapter

import (
	"strings"
	"testing"

	"github.com/google/capslock/interesting"
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

	cl, err := buildClassifier(nil, nil)
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
	cl, err := buildClassifier(nil, nil)
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
	cl, err := buildClassifier(nil, nil)
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
	cl, err := buildClassifier(nil, nil)
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
	_, err := buildClassifier([]capanalyzer.InterfaceSymbol{"invalidKey\nfunc invalidFn INVALID_CAPABILITY_NAME"}, nil)
	if err == nil {
		t.Errorf("expected buildClassifier to fail with malformed custom classifier input")
	}
}

// TestClassifier_PruneAtPackages_NilUnchanged verifies that empty/nil PruneAtPackages produces bit-for-bit identical classifier text (AC6).
func TestClassifier_PruneAtPackages_NilUnchanged(t *testing.T) {
	txtNil, err1 := buildClassifierText(nil, nil)
	if err1 != nil {
		t.Fatalf("buildClassifierText(nil, nil) failed: %v", err1)
	}
	txtEmpty, err2 := buildClassifierText(nil, []string{})
	if err2 != nil {
		t.Fatalf("buildClassifierText(nil, []) failed: %v", err2)
	}

	if txtNil != txtEmpty {
		t.Errorf("expected buildClassifierText with nil and empty []string to be bit-for-bit identical:\nnil:\n%s\nempty:\n%s", txtNil, txtEmpty)
	}

	cl1, err1 := buildClassifier(nil, nil)
	if err1 != nil {
		t.Fatalf("buildClassifier(nil, nil) failed: %v", err1)
	}
	cl2, err2 := buildClassifier(nil, []string{})
	if err2 != nil {
		t.Fatalf("buildClassifier(nil, []) failed: %v", err2)
	}
	if cl1 == nil || cl2 == nil {
		t.Fatalf("expected non-nil classifiers")
	}
}

// TestClassifierText_Invariants verifies bit-level classifier text output contracts: deduplication, sorting, namespace separation, and determinism (AC5).
func TestClassifierText_Invariants(t *testing.T) {
	// 1. Deduplication and Sorting
	inputPkgs := []string{"example.com/pkgB", "example.com/pkgA", "example.com/pkgA", "example.com/pkgC"}
	txt, err := buildClassifierText(nil, inputPkgs)
	if err != nil {
		t.Fatalf("buildClassifierText failed: %v", err)
	}

	expectedTail := "package example.com/pkgA CAPABILITY_SAFE\npackage example.com/pkgB CAPABILITY_SAFE\npackage example.com/pkgC CAPABILITY_SAFE\n"
	if !strings.HasSuffix(txt, expectedTail) {
		t.Errorf("expected classifier text to end with sorted, deduplicated package lines:\n%q\ngot:\n%q", expectedTail, txt)
	}

	// 2. Namespace Separation (equal function and package strings emit both keyword forms)
	syms := []capanalyzer.InterfaceSymbol{"example.com/foo"}
	pkgs := []string{"example.com/foo"}
	txtBoth, err := buildClassifierText(syms, pkgs)
	if err != nil {
		t.Fatalf("buildClassifierText failed: %v", err)
	}

	expectedFuncLine := "func example.com/foo CAPABILITY_SAFE\n"
	expectedPkgLine := "package example.com/foo CAPABILITY_SAFE\n"
	if !strings.Contains(txtBoth, expectedFuncLine) {
		t.Errorf("expected text to contain function rule line %q, got:\n%s", expectedFuncLine, txtBoth)
	}
	if !strings.Contains(txtBoth, expectedPkgLine) {
		t.Errorf("expected text to contain package rule line %q, got:\n%s", expectedPkgLine, txtBoth)
	}

	// 3. Determinism across repeated invocations
	for i := 0; i < 10; i++ {
		txtIter, err := buildClassifierText(syms, inputPkgs)
		if err != nil {
			t.Fatalf("iteration %d failed: %v", i, err)
		}
		if i == 0 {
			txt = txtIter
		} else if txtIter != txt {
			t.Fatalf("iteration %d output diverged from iteration 0:\niter 0:\n%s\niter %d:\n%s", i, txt, i, txtIter)
		}
	}
}

// TestClassifier_FunctionOverridesPackagePrecedence verifies that a per-function key overrides a package key in Capslock lookup order (AC2).
func TestClassifier_FunctionOverridesPackagePrecedence(t *testing.T) {
	// Directly test Capslock classifier resolution order when a package rule and a function rule with a distinct category coexist.
	customRules := "package example.com/pkgA CAPABILITY_SAFE\nfunc example.com/pkgA.SpecificFunc CAPABILITY_FILES\n"
	cl, err := interesting.LoadClassifier("test-precedence", strings.NewReader(customRules), false /* excludeBuiltin */)
	if err != nil {
		t.Fatalf("LoadClassifier failed: %v", err)
	}

	// Function with explicit rule returns function category "FILES", overriding package "SAFE"
	if cat := cl.FunctionCategory("example.com/pkgA", "example.com/pkgA.SpecificFunc"); cat != "FILES" {
		t.Errorf("expected FunctionCategory for example.com/pkgA.SpecificFunc to be FILES (function override), got %q", cat)
	}

	// Other function in same package falls back to package category "SAFE"
	if cat := cl.FunctionCategory("example.com/pkgA", "example.com/pkgA.OtherFunc"); cat != "SAFE" {
		t.Errorf("expected FunctionCategory for example.com/pkgA.OtherFunc to be SAFE (package fallback), got %q", cat)
	}
}

// TestClassifier_PruneAtPackages_Malformed verifies that malformed package entries fail closed.
func TestClassifier_PruneAtPackages_Malformed(t *testing.T) {
	testCases := []struct {
		name    string
		pkgs    []string
		wantErr string
	}{
		{
			name:    "empty package entry",
			pkgs:    []string{""},
			wantErr: "empty prune package key",
		},
		{
			name:    "package entry with newline",
			pkgs:    []string{"foo/bar\nforged"},
			wantErr: "prune package key \"foo/bar\\nforged\" contains newline characters",
		},
		{
			name:    "package entry with carriage return",
			pkgs:    []string{"foo/bar\rforged"},
			wantErr: "prune package key \"foo/bar\\rforged\" contains newline characters",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildClassifier(nil, tc.pkgs)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestClassifier_PruneAtPackages_PackageResolution verifies that package entries in buildClassifier result in SAFE categories for functions in those packages.
func TestClassifier_PruneAtPackages_PackageResolution(t *testing.T) {
	pkgs := []string{"example.com/pkgB", "example.com/pkgA", "example.com/pkgA"}
	cl, err := buildClassifier(nil, pkgs)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	// Verify function inside pruned package resolves to SAFE
	if cat := cl.FunctionCategory("example.com/pkgA", "example.com/pkgA.SomeFunc"); cat != "SAFE" {
		t.Errorf("expected FunctionCategory for example.com/pkgA.SomeFunc to be SAFE, got %q", cat)
	}
	if cat := cl.FunctionCategory("example.com/pkgB", "example.com/pkgB.AnotherFunc"); cat != "SAFE" {
		t.Errorf("expected FunctionCategory for example.com/pkgB.AnotherFunc to be SAFE, got %q", cat)
	}
}

// TestClassifier_PruneAtPackages_NamespaceSeparation verifies that identical string keys across function and package namespaces do not suppress each other.
func TestClassifier_PruneAtPackages_NamespaceSeparation(t *testing.T) {
	// A package import path "example.com/foo" and a function key "example.com/foo"
	syms := []capanalyzer.InterfaceSymbol{"example.com/foo"}
	pkgs := []string{"example.com/foo"}

	cl, err := buildClassifier(syms, pkgs)
	if err != nil {
		t.Fatalf("failed to build classifier: %v", err)
	}

	if cat := cl.FunctionCategory("example.com/foo", "Bar"); cat != "SAFE" {
		t.Errorf("expected package prune to classify example.com/foo.Bar as SAFE, got %q", cat)
	}
}

// TestAdapter_Analyze_PruneAtPackages_Success verifies that package granularity pruning suppresses findings end-to-end.
func TestAdapter_Analyze_PruneAtPackages_Success(t *testing.T) {
	adapter := NewAdapter()
	pkgPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/packageprune"
	depPath := "github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/filereader"

	// 1. Without package pruning: reports FILES
	reqUnpruned := capanalyzer.AnalyzeRequest{
		Packages: []string{pkgPath},
	}
	findingsUnpruned, err := adapter.Analyze(reqUnpruned)
	if err != nil {
		t.Fatalf("failed to analyze unpruned: %v", err)
	}
	if len(findingsUnpruned) == 0 {
		t.Fatalf("expected findings when unpruned, got none")
	}

	// 2. With package pruning: 0 findings
	reqPruned := capanalyzer.AnalyzeRequest{
		Packages:        []string{pkgPath},
		PruneAtPackages: []string{depPath},
	}
	findingsPruned, err := adapter.Analyze(reqPruned)
	if err != nil {
		t.Fatalf("failed to analyze pruned: %v", err)
	}
	if len(findingsPruned) != 0 {
		t.Errorf("expected 0 findings when dependency package %q is pruned, got %d findings: %+v", depPath, len(findingsPruned), findingsPruned)
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
	} else {
		t.Log("ambient-authority-free (no findings)")
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
			if f.Class != capanalyzer.TrueAuthority {
				t.Errorf("expected FILES finding class to be TrueAuthority, got %q", f.Class)
			}
			if len(f.CallPath) == 0 {
				t.Errorf("expected non-empty CallPath for FILES finding")
			}

			// Validate and log call path frames
			t.Logf("FILES finding in package %q (class %s):", f.Package, f.Class)
			hasPositiveEvidence := false
			hasReadSomeFileFunc := false
			hasFileReaderFile := false
			for idx, frame := range f.CallPath {
				t.Logf("  [%d] %s at %s:%d", idx, frame.Func, frame.File, frame.Line)
				if frame.Func == "" {
					t.Errorf("expected non-empty Func in call path frame [%d]", idx)
				}
				if frame.File != "" && frame.Line > 0 {
					hasPositiveEvidence = true
				}
				// Semantic checks
				if strings.Contains(frame.Func, "ReadSomeFile") {
					hasReadSomeFileFunc = true
				}
				if strings.HasSuffix(frame.File, "filereader.go") {
					hasFileReaderFile = true
				}
			}
			if !hasPositiveEvidence {
				t.Errorf("expected at least one frame with positive source-line evidence (non-empty File and Line > 0)")
			}
			if !hasReadSomeFileFunc {
				t.Errorf("expected FILES finding call path to contain a frame with function name ReadSomeFile")
			}
			if !hasFileReaderFile {
				t.Errorf("expected FILES finding call path to contain a frame with filename filereader.go")
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

// TestAdapter_Analyze_EmptyPackages verifies that an empty package request is rejected with a descriptive error.
func TestAdapter_Analyze_EmptyPackages(t *testing.T) {
	adapter := NewAdapter()
	req := capanalyzer.AnalyzeRequest{
		Packages: []string{},
	}
	_, err := adapter.Analyze(req)
	if err == nil {
		t.Errorf("expected Analyze to fail on empty package request, but it succeeded")
	} else if !strings.Contains(err.Error(), "package request is empty") {
		t.Errorf("expected error message to contain %q, got: %q", "package request is empty", err.Error())
	}
}

// TestAdapter_Analyze_PruneAt_Success verifies that non-empty PruneAt is supported and prunes findings.
func TestAdapter_Analyze_PruneAt_Success(t *testing.T) {
	adapter := NewAdapter()
	req := capanalyzer.AnalyzeRequest{
		Packages: []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/filereader"},
		PruneAt:  []capanalyzer.InterfaceSymbol{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/filereader.ReadSomeFile"},
	}
	findings, err := adapter.Analyze(req)
	if err != nil {
		t.Fatalf("failed to analyze: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings because ReadSomeFile was pruned, got %d findings: %+v", len(findings), findings)
	}
}

// TestAdapter_Analyze_BrokenPackage verifies that a broken or non-existent package pattern
// returns useful loader diagnostics and no partial findings are returned.
func TestAdapter_Analyze_BrokenPackage(t *testing.T) {
	adapter := NewAdapter()
	req := capanalyzer.AnalyzeRequest{
		Packages: []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/nonexistent_package_xyz"},
	}
	findings, err := adapter.Analyze(req)
	if err == nil {
		t.Errorf("expected Analyze to fail on non-existent package pattern, but it succeeded")
	} else if !strings.Contains(err.Error(), "package load errors") {
		t.Errorf("expected error message to contain %q, got: %q", "package load errors", err.Error())
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings on package load failure, got %d findings", len(findings))
	}
}

// TestMapClass verifies that capability taxonomy mapping is handled safely and rejects unknown ones.
func TestMapClass(t *testing.T) {
	trueCaps := []string{
		"FILES", "NETWORK", "READ_SYSTEM_STATE", "MODIFY_SYSTEM_STATE",
		"OPERATING_SYSTEM", "SYSTEM_CALLS", "EXEC", "RUNTIME",
		"MODIFY_SYSTEM_STATE/ENV", "SYSTEM_CALLS/INDIRECT",
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

	unknownCaps := []string{"UNSPECIFIED", "SAFE", "SOMETHING_NEW", "", "UNSPECIFIED/foo", "SAFE/bar"}
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
