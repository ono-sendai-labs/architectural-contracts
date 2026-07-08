package spike

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// spikeDir is this module's root (where the probe packages live).
func spikeDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

// repoGoDir locates the product go/ module (holding the csvtool example) by
// walking up from this file until a go/go.mod is found.
func repoGoDir(t *testing.T) string {
	t.Helper()
	dir := spikeDir(t)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go", "go.mod")); err == nil {
			return filepath.Join(dir, "go")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo go/ module")
		}
		dir = parent
	}
}

func logFindings(t *testing.T, title string, fs []Finding) {
	t.Helper()
	t.Logf("--- %s (%d findings) ---", title, len(fs))
	if len(fs) == 0 {
		t.Logf("    <none> — ambient-authority-free")
		return
	}
	for _, f := range fs {
		t.Logf("    %s", f)
		for _, p := range f.Path {
			t.Logf("        -> %s", p)
		}
	}
}

const (
	spikeMod   = "github.com/ono-sendai-labs/architectural-contracts/spike"
	csvtoolMod = "github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool"
)

// Assumption 1: strict-safe stdlib envelope.
//
// Finding: under the recommended strict classifier (UNANALYZED excluded), every
// stdlib entry point the pure core and examples need comes out clean — including
// io.ReadAll (needed by manifest.Parse), errors.Is/As, fmt, strconv, and BOTH
// sort.Sort AND sort.Slice. This revises design review B8: sort.Slice is fine —
// Capslock rewrites sort.Sort/sort.Slice call sites to invoke the comparator
// directly (see TestSortCallSiteRewrite), so neither reaches reflect. Under the
// builtin default (UNANALYZED visible), io.ReadAll and errors.Is surface as
// spurious UNANALYZED findings — the reason the adapter must exclude UNANALYZED.
func TestStdlibEnvelope(t *testing.T) {
	pkgs, err := loadPackages(spikeDir(t), "./probes/stdlibenv")
	if err != nil {
		t.Fatal(err)
	}

	strict := analyze(pkgs, strictClassifier())
	logFindings(t, "stdlibenv (strict — UNANALYZED excluded) [RECOMMENDED]", strict)
	if len(strict) != 0 {
		t.Errorf("strict envelope expected fully clean, got %d findings", len(strict))
	}

	visible := analyze(pkgs, unanalyzedVisibleClassifier())
	logFindings(t, "stdlibenv (builtin default — UNANALYZED visible) [CONTRAST]", visible)
	if !anyFunctionContains(visible, "UseIoReadAll") || !anyFunctionContains(visible, "UseErrors") {
		t.Errorf("expected io.ReadAll and errors.Is to surface as UNANALYZED under the builtin default")
	}
	// The sort rewrite means sort.Sort/sort.Slice stay clean even here.
	if anyFunctionContains(visible, "UseSortSort") || anyFunctionContains(visible, "UseSortSlice") {
		t.Errorf("sort.Sort/sort.Slice unexpectedly surfaced; expected Capslock's sort rewrite to keep them clean")
	}
}

// Evidence for the sort rewrite: the raw VTA graph contains the edge
// UseSortSort -> sort.Sort (category UNANALYZED), yet Capslock reports nothing,
// because buildGraph rewrites sort call sites before building SSA.
func TestSortCallSiteRewrite(t *testing.T) {
	pkgs, err := loadPackages(spikeDir(t), "./probes/stdlibenv")
	if err != nil {
		t.Fatal(err)
	}
	rawEdge := rawGraphHasEdge(pkgs, "UseSortSort", "sort.Sort")
	t.Logf("raw VTA graph has UseSortSort -> sort.Sort edge: %v", rawEdge)
	if !rawEdge {
		t.Skip("raw edge not present; environment-dependent, skipping evidence assertion")
	}
	reported := analyze(pkgs, unanalyzedVisibleClassifier())
	if anyFunctionContains(reported, "UseSortSort") {
		t.Errorf("expected the sort rewrite to suppress the sort.Sort finding")
	}
}

// Assumption 3 & partial 4: key formats & normalization (review A4).
func TestKeyForms(t *testing.T) {
	pkgs, err := loadPackages(spikeDir(t), "./probes/keyforms")
	if err != nil {
		t.Fatal(err)
	}
	keys := ssaKeys(pkgs)
	t.Logf("--- keyforms ssa.Function.String() keys ---")
	for _, k := range keys {
		if strings.Contains(k, "keyforms") {
			t.Logf("    %s", k)
		}
	}
	base := spikeMod + "/probes/keyforms"
	want := []string{
		"(" + base + ".Widget).Name",     // value receiver
		"(*" + base + ".Widget).SetName", // pointer receiver
		"(" + base + ".Circle).Area",     // interface impl (value receiver)
		"(" + base + ".Boxed).Name",      // promoted method wrapper
		base + ".MapInts",                // generic origin — bracket-free
		base + ".init",                   // explicit/synthetic init
	}
	joined := strings.Join(keys, "\n")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("expected ssa key %q not found", w)
		}
	}
	// The generic instantiation appears as a separate SSA name with brackets, but
	// getNodeCapabilities categorizes it via origin.String() (bracket-free), so the
	// classifier key is the bracket-free form above.
	if strings.Contains(joined, base+".MapInts[") {
		t.Logf("NOTE: instantiation key present (categorized via bracket-free origin): %s.MapInts[...]", base)
	}
}

// Assumption 4 (init attribution, review A2) & CAPABILITY_SAFE init pruning.
func TestInitAttributionAndPruning(t *testing.T) {
	pkgs, err := loadPackages(spikeDir(t), "./probes/initimporter")
	if err != nil {
		t.Fatal(err)
	}
	fs := analyze(pkgs, strictClassifier())
	logFindings(t, "initimporter (unpruned)", fs)
	if !hasCapability(fs, "READ_SYSTEM_STATE") {
		t.Errorf("expected initimporter to inherit READ_SYSTEM_STATE via dependency init")
	}
	if !anyFunctionContains(fs, ".init") {
		t.Errorf("expected the attributed function to be initimporter.init")
	}

	initKey := spikeMod + "/probes/initauth.init"
	cl, err := pruneClassifier(initKey)
	if err != nil {
		t.Fatal(err)
	}
	fp := analyze(pkgs, cl)
	logFindings(t, "initimporter (dependency init pruned via `func <pkg>.init CAPABILITY_SAFE`)", fp)
	if hasCapability(fp, "READ_SYSTEM_STATE") {
		t.Errorf("pruning %q should have removed READ_SYSTEM_STATE", initKey)
	}
}

// Assumption 6: io.Reader attribution — clean (bytes.Reader) vs dirty (*os.File).
// Under the strict classifier Capslock descends through io.ReadAll, so VTA flows
// the concrete reader type into Parse: *os.File → FILES, bytes.Reader → clean.
// This validates the shell-wraps-in-bytes.Reader rule for manifest.Parse (§11).
func TestReaderAttribution(t *testing.T) {
	clean, err := loadPackages(spikeDir(t), "./probes/readerattr", "./probes/readercleanshell")
	if err != nil {
		t.Fatal(err)
	}
	cf := analyze(clean, strictClassifier())
	logFindings(t, "reader clean (only bytes.Reader flows into Parse)", cf)
	if hasCapability(cf, "FILES") {
		t.Errorf("Parse must be FILES-free when only a bytes.Reader flows in")
	}

	dirty, err := loadPackages(spikeDir(t), "./probes/readerattr", "./probes/readerdirtyshell")
	if err != nil {
		t.Fatal(err)
	}
	df := analyze(dirty, strictClassifier())
	logFindings(t, "reader dirty (*os.File flows into Parse via VTA)", df)
	if !functionHasCapability(df, "readerattr.Parse", "FILES") {
		t.Errorf("Parse should be attributed FILES when an *os.File flows in via VTA")
	}

	// Contrast: with UNANALYZED visible, io.ReadAll is a leaf and the flow is
	// masked — Parse comes out only UNANALYZED, never FILES. Documents *why* the
	// adapter must exclude UNANALYZED for §11 attribution to work.
	masked := analyze(dirty, unanalyzedVisibleClassifier())
	logFindings(t, "reader dirty (builtin default — FILES flow MASKED by io.ReadAll leaf) [CONTRAST]", masked)
	if functionHasCapability(masked, "readerattr.Parse", "FILES") {
		t.Errorf("did not expect FILES on Parse under the masking default classifier")
	}
}

// Assumption 5: whole-package scope + _test.go exclusion (§5.4).
func TestScopeAndTestExclusion(t *testing.T) {
	pkgs, err := loadPackages(spikeDir(t), "./probes/scope")
	if err != nil {
		t.Fatal(err)
	}
	fs := analyze(pkgs, strictClassifier())
	logFindings(t, "scope", fs)
	// Whole-package scope: the unreachable exported helper is still reported.
	if !functionHasCapability(fs, "scope.Unreached", "FILES") {
		t.Errorf("whole-package scope: Unreached should be reported with FILES")
	}
	// _test.go exclusion: test-only authority must not appear.
	if anyFunctionContains(fs, "inTestAuthority") || anyFunctionContains(fs, "externalTestAuthority") {
		t.Errorf("_test.go authority should be excluded from the analyzed build")
	}
}

// Model refinement: "ambient authority = capability minting, not capability use."
//
// Capslock's builtin map classifies the (*os.File) *use* methods (Read/Write/...)
// as FILES alongside the *minting* functions (os.Open/os.ReadFile/...). That
// conflates object-capability *use* with ambient authority and makes a
// deprivileged consumer's attributed authority depend on what its *caller* passes
// in — breaking modular reasoning. Reclassifying the handle-use methods SAFE
// attributes filesystem authority to the minting site instead. This test proves:
//   - the SAME dirty-reader scenario now attributes FILES to Run (calls os.Open),
//     NOT to Parse (only consumes the io.Reader) — so manifest.Parse is
//     authority-free by construction, no bytes.Reader wrapping needed;
//   - direct handle use (f.Read) is likewise authority-free;
//   - minting (os.Open) and path-based reads (os.ReadFile in csvfile) still flag.
func TestOcapMintingNotUse(t *testing.T) {
	ocap, err := ocapClassifier()
	if err != nil {
		t.Fatal(err)
	}

	dirty, err := loadPackages(spikeDir(t), "./probes/readerattr", "./probes/readerdirtyshell")
	if err != nil {
		t.Fatal(err)
	}
	df := analyze(dirty, ocap)
	logFindings(t, "ocap: reader dirty (*os.File flows into Parse)", df)
	if functionHasCapability(df, "readerattr.Parse", "FILES") {
		t.Errorf("ocap: Parse only consumes the granted io.Reader; it must be authority-free")
	}
	if !functionHasCapability(df, "readerdirtyshell.Run", "FILES") {
		t.Errorf("ocap: Run mints the capability via os.Open; it must retain FILES")
	}

	fh, err := loadPackages(spikeDir(t), "./probes/filehandle")
	if err != nil {
		t.Fatal(err)
	}
	fhf := analyze(fh, ocap)
	logFindings(t, "ocap: filehandle (Mint vs Consume)", fhf)
	if functionHasCapability(fhf, "filehandle.Consume", "FILES") {
		t.Errorf("ocap: Consume only uses a granted handle (f.Read); it must be authority-free")
	}
	if !functionHasCapability(fhf, "filehandle.Mint", "FILES") {
		t.Errorf("ocap: Mint calls os.Open; it must retain FILES")
	}

	csv, err := loadPackages(repoGoDir(t), "./examples/csvtool/csvfile")
	if err != nil {
		t.Fatal(err)
	}
	cf := analyze(csv, ocap)
	logFindings(t, "ocap: csvfile (os.ReadFile — path-based mint+read)", cf)
	if !functionHasCapability(cf, "csvfile.Read", "FILES") {
		t.Errorf("ocap: csvfile.Read uses ambient authority (os.ReadFile takes a path); it must retain FILES")
	}
}

// Assumption 2: CAPABILITY_SAFE pruning end-to-end on the csvtool app (FR5b).
func TestAppPruningEndToEnd(t *testing.T) {
	goDir := repoGoDir(t)
	pkgs, err := loadPackages(goDir, "./examples/csvtool/app")
	if err != nil {
		t.Fatal(err)
	}
	fs := analyze(pkgs, strictClassifier())
	logFindings(t, "csvtool app (unpruned — FILES absorbed through csvfile.Read)", fs)
	if !functionHasCapability(fs, "app.Run", "FILES") {
		t.Errorf("unpruned app should absorb FILES transitively through csvfile.Read")
	}

	readKey := csvtoolMod + "/csvfile.Read"
	cl, err := pruneClassifier(readKey)
	if err != nil {
		t.Fatal(err)
	}
	fp := analyze(pkgs, cl)
	logFindings(t, "csvtool app (pruned at csvfile.Read — component-dependency boundary, FR5b)", fp)
	if len(fp) != 0 {
		t.Errorf("pruning at csvfile.Read should make app ambient-authority-free, got %d findings", len(fp))
	}
}
