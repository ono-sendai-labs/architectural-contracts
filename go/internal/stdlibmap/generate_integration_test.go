//go:build integration && stdlibmap_full

package stdlibmap_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// fullGeneration runs Generate over the local toolchain's complete standard
// library: native discovery, native loading, and one isolated Capslock run per
// importable package at GranularityFunction under the generation classifier
// (approximately 146 seconds on the pinned host; the full lane pays this cost).
func fullGeneration(t *testing.T) *stdlibmap.GeneratedMap {
	t.Helper()
	entries, err := stdlibmap.NativeStdPackageList(context.Background(), nil)
	if err != nil {
		t.Fatalf("NativeStdPackageList: %v", err)
	}
	version, err := stdlibmap.NativeToolchainVersion(context.Background())
	if err != nil {
		t.Fatalf("NativeToolchainVersion: %v", err)
	}
	out, err := stdlibmap.Generate(stdlibmap.GenerationInput{
		Target:      stdlibmap.TargetConfig{ToolchainVersion: version, GOOS: "linux", GOARCH: "amd64"},
		RuleVersion: "test-rules-v1",
		Oracle:      stdlibmap.PackageOracleFunc(func() ([]stdlibmap.PackageEntry, error) { return entries, nil }),
		Loader:      &stdlibmap.NativeLoader{},
		// Findings left nil: Generate binds the native Capslock runner to the
		// target environment (TargetEnv), proving the analysis describes the
		// same configuration the loader and oracle describe.
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

// TestGenerateFullStdlibMap is the integration leg (task AC 1, 5, 7, 8): the
// real local stdlib generates a total map whose pinned semantic cases hold,
// every oracle package is represented, and every inventoried symbol and init
// carries a terminal entry. The second generation pins byte-identical
// determinism.
func TestGenerateFullStdlibMap(t *testing.T) {
	out := fullGeneration(t)
	m := out.Map

	// Totality: every enumerated package is represented; every importable
	// package has exactly one init; every record is terminal; no init#N
	// anywhere.
	pkgSet := map[string]bool{}
	for _, p := range m.Packages {
		if pkgSet[p.Path] {
			t.Fatalf("duplicate package inventory entry %q", p.Path)
		}
		pkgSet[p.Path] = true
	}
	if len(pkgSet) == 0 {
		t.Fatalf("empty package inventory")
	}
	var importable int
	for _, p := range m.Packages {
		if p.Importable {
			importable++
		}
	}
	if len(m.Inits) != importable {
		t.Fatalf("init records = %d; importable packages = %d", len(m.Inits), importable)
	}
	for _, i := range m.Inits {
		if i.Classification == gen.Classification_CLASSIFICATION_UNSPECIFIED {
			t.Fatalf("init of %s is not terminal", i.Package)
		}
	}
	symSet := map[string]bool{}
	for _, s := range m.Symbols {
		if symSet[s.Package+"\x00"+s.Id] {
			t.Fatalf("duplicate symbol record %s in %s", s.Id, s.Package)
		}
		symSet[s.Package+"\x00"+s.Id] = true
		if s.Classification == gen.Classification_CLASSIFICATION_UNSPECIFIED {
			t.Fatalf("symbol %s in %s is not terminal", s.Id, s.Package)
		}
		if strings.Contains(s.Id, ".init#") {
			t.Fatalf("init#N leaked into the symbol inventory: %+v", s)
		}
	}

	// AC 1: pinned semantic examples.
	lookup := func(pkg, id string) *gen.SymbolRecord {
		t.Helper()
		for _, s := range m.Symbols {
			if s.Package == pkg && s.Id == id {
				return s
			}
		}
		t.Fatalf("no record for %s in %s", id, pkg)
		return nil
	}
	hasCap := func(s *gen.SymbolRecord, capability string) bool {
		for _, c := range s.Capabilities {
			if c == capability {
				return true
			}
		}
		return false
	}
	// os.ReadFile is FILES with non-empty evidence.
	readFile := lookup("os", "os.ReadFile")
	if !hasCap(readFile, "FILES") {
		t.Fatalf("os.ReadFile = %+v; want FILES", readFile)
	}
	var readFileEvidence *gen.Evidence
	for _, e := range m.Evidence {
		if e.SymbolId == "os.ReadFile" && e.Capability == "FILES" {
			readFileEvidence = e
		}
	}
	if readFileEvidence == nil || len(readFileEvidence.Frames) == 0 {
		t.Fatalf("os.ReadFile FILES has no evidence frames")
	}
	// strings.TrimSpace is SAFE.
	if r := lookup("strings", "strings.TrimSpace"); r.Classification != gen.Classification_SAFE {
		t.Fatalf("strings.TrimSpace = %+v; want SAFE", r)
	}
	// sort.Slice is UNANALYZED (the spike's laundering case).
	if r := lookup("sort", "sort.Slice"); r.Classification != gen.Classification_UNANALYZED {
		t.Fatalf("sort.Slice = %+v; want UNANALYZED (the classifier-exclusion laundering case)", r)
	}
	// os.Stdin includes FILES.
	if r := lookup("os", "os.Stdin"); !hasCap(r, "FILES") {
		t.Fatalf("os.Stdin = %+v; want FILES among capabilities", r)
	}
	// io.EOF is SAFE.
	if r := lookup("io", "io.EOF"); r.Classification != gen.Classification_SAFE {
		t.Fatalf("io.EOF = %+v; want SAFE", r)
	}
	// http.DefaultClient includes NETWORK.
	if r := lookup("net/http", "net/http.DefaultClient"); !hasCap(r, "NETWORK") {
		t.Fatalf("net/http.DefaultClient = %+v; want NETWORK among capabilities", r)
	}
	// unsafe.Pointer is UNANALYZED (hardcoded compiler builtin).
	if r := lookup("unsafe", "unsafe.Pointer"); r.Classification != gen.Classification_UNANALYZED {
		t.Fatalf("unsafe.Pointer = %+v; want UNANALYZED", r)
	}

	// AC 6: the three SAFE provenance kinds are visibly distinct on real
	// entries. Curated-safe roots produce no findings, so their provenance
	// must come from the classifier (round-1 finding).
	if r := lookup("os", "os.Exit"); r.Classification != gen.Classification_SAFE || r.Provenance != "capslock-curated" {
		t.Fatalf("os.Exit = %+v; want SAFE capslock-curated", r)
	}
	if r := lookup("runtime", "runtime.GC"); r.Classification != gen.Classification_SAFE || r.Provenance != "capslock-curated" {
		t.Fatalf("runtime.GC = %+v; want SAFE capslock-curated", r)
	}
	if r := lookup("os", "(os.File).Read"); r.Classification != gen.Classification_SAFE || r.Provenance != "project-override" {
		t.Fatalf("(os.File).Read = %+v; want SAFE project-override", r)
	}
	if r := lookup("math", "math.Abs"); r.Classification != gen.Classification_SAFE || r.Provenance != "proved-pure" {
		t.Fatalf("math.Abs = %+v; want SAFE proved-pure", r)
	}
	// A curated VALUE-receiver method (the round-2 spelling fix): Capslock
	// curates "func (net.Flags).String CAPABILITY_SAFE".
	if r := lookup("net", "(net.Flags).String"); r.Classification != gen.Classification_SAFE || r.Provenance != "capslock-curated" {
		t.Fatalf("(net.Flags).String = %+v; want SAFE capslock-curated", r)
	}
	// Capability-bearing init records carry evidence (round-1 finding): at
	// least one importable package's init is classified with capabilities
	// somewhere in the stdlib, and its evidence exists.
	var initCaps, initEvidence int
	for _, i := range m.Inits {
		if i.Classification == gen.Classification_CAPABILITIES {
			initCaps++
		}
	}
	for _, e := range m.Evidence {
		if strings.HasSuffix(e.SymbolId, ".init") {
			initEvidence++
		}
	}
	if initCaps == 0 || initEvidence == 0 {
		t.Fatalf("init capability records = %d, init evidence = %d; capability-bearing inits must carry evidence", initCaps, initEvidence)
	}

	// The canonical bytes decode.
	if _, err := artifactio.DecodeMap(bytes.NewReader(out.Bytes)); err != nil {
		t.Fatalf("DecodeMap: %v", err)
	}

	// AC 7: a second generation over identical inputs is byte-identical.
	out2 := fullGeneration(t)
	if !bytes.Equal(out.Bytes, out2.Bytes) {
		t.Fatalf("two full generations produced different bytes")
	}
	if out.Digest != out2.Digest {
		t.Fatalf("digest differs between identical generations")
	}
}

// TestGenerateCrossTarget proves the Capslock analysis and the typed inventory
// describe the same target configuration before the map key is derived
// (round-1 finding): generation for darwin/arm64 must classify the
// darwin-only syscall surface and omit linux-only declarations, and vice
// versa, with distinct SDK keys.
func TestGenerateCrossTarget(t *testing.T) {
	generateFor := func(goos, goarch string) *stdlibmap.GeneratedMap {
		t.Helper()
		target := stdlibmap.TargetConfig{ToolchainVersion: "go-test", GOOS: goos, GOARCH: goarch}
		env := (&stdlibmap.NativeLoader{Env: stdlibmap.TargetEnv(target)}).Environment()
		loader := &stdlibmap.NativeLoader{Env: stdlibmap.TargetEnv(target)}
		entries, err := stdlibmap.NormalizePackageList([]string{"syscall"})
		if err != nil {
			t.Fatalf("NormalizePackageList: %v", err)
		}
		out, err := stdlibmap.Generate(stdlibmap.GenerationInput{
			Target:      target,
			RuleVersion: "test-rules-v1",
			Oracle:      stdlibmap.PackageOracleFunc(func() ([]stdlibmap.PackageEntry, error) { return entries, nil }),
			Loader:      loader,
			Findings: stdlibmap.FindingSourceFunc(func(paths []string) ([]capslockadapter.GenerationFinding, error) {
				return capslockadapter.GenerationFindingsForEnv(env, paths)
			}),
		})
		if err != nil {
			t.Fatalf("Generate(%s/%s): %v", goos, goarch, err)
		}
		return out
	}
	linux := generateFor("linux", "amd64")
	if _, ok := recordByID(linux.Map, "syscall.EpollCreate"); !ok {
		t.Fatalf("linux/amd64 syscall map lacks syscall.EpollCreate; the target did not reach the analysis")
	}
	darwin := generateFor("darwin", "arm64")
	if _, ok := recordByID(darwin.Map, "syscall.EpollCreate"); ok {
		t.Fatalf("darwin/arm64 syscall map has linux-only syscall.EpollCreate; the analysis used the host environment")
	}
	if _, ok := recordByID(darwin.Map, "syscall.Sysctl"); !ok {
		t.Fatalf("darwin/arm64 syscall map lacks syscall.Sysctl")
	}
	if bytes.Equal(linux.Bytes, darwin.Bytes) {
		t.Fatalf("linux and darwin maps are byte-identical; the SDK key does not distinguish targets")
	}
}

// recordByID finds a symbol record by its full ID.
func recordByID(m *gen.StdlibMap, id string) (*gen.SymbolRecord, bool) {
	for _, s := range m.Symbols {
		if s.Id == id {
			return s, true
		}
	}
	return nil, false
}
