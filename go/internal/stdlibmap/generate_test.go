package stdlibmap

import (
	"bytes"
	"errors"
	"slices"

	"google.golang.org/protobuf/proto"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// --- hermetic generation fixtures ---------------------------------------------
//
// The "os" fixture mirrors the real package's shape: a File handle type whose
// Read/Write/Chmod are minting-reclassified use methods and whose Chdir is
// genuine ambient authority, pre-minted standard-stream vars, and minting
// funcs. The genpkg fixture carries the remaining object kinds and the
// analyzer-root drop cases. Findings are canned; no Capslock run happens here
// (that is the integration leg's job).

const genOsSrc = `package os

type File struct{ Fd int }

func (f *File) Read(p []byte) (int, error) { return 0, nil }
func (f *File) Write(p []byte) (int, error) { return len(p), nil }
func (f *File) Chmod(mode uint32) error { return nil }
func (f *File) Chdir() error { return nil }

func Open(name string) (*File, error) { return nil, nil }
func ReadFile(name string) ([]byte, error) { return nil, nil }
func Getenv(key string) string { return "" }
func Exit(code int) {}

var Stdin = &File{}
var Stdout = &File{}
`

const genGenSrc = `package genpkg

import "os"

const Max = 3

type Plain struct{ X int }

type Reader interface{ Read() }

var Count int
var Client Reader

func Load(name string) ([]byte, error) { return os.ReadFile(name) }
func Trim(s string) string { return s }
func init() { _ = os.Getenv("X") }
`

func genInventory(t *testing.T) *Inventory {
	t.Helper()
	loader := typeCheckLoader(t, map[string]string{
		"os":                        genOsSrc,
		"example.com/genpkg":        genGenSrc,
		"example.com/internal/seed": "package seed\n",
	})
	inv, err := BuildInventory([]PackageEntry{
		{Path: "os", Importable: true},
		{Path: "example.com/genpkg", Importable: true},
		{Path: "example.com/internal/seed", Importable: false},
	}, loader)
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	return inv
}

func genFindings() []capslockadapter.GenerationFinding {
	frame := func(name string) stdlibauthority.Frame {
		return stdlibauthority.Frame{Function: name, File: "/src/x.go", Line: 3}
	}
	return []capslockadapter.GenerationFinding{
		{RootName: "example.com/genpkg.Load", RootPackage: "example.com/genpkg", Capability: "FILES",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.Load"), frame("os.ReadFile")}},
		{RootName: "example.com/genpkg.init", RootPackage: "example.com/genpkg", Capability: "READ_SYSTEM_STATE",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.init"), frame("os.Getenv")}},
		{RootName: "example.com/genpkg.init#1", RootPackage: "example.com/genpkg", Capability: "FILES",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.init#1")}},
		{RootName: "example.com/genpkg.Load$1", RootPackage: "example.com/genpkg", Capability: "NETWORK",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.Load$1")}},
		{RootName: "example.com/genpkg.helper", RootPackage: "example.com/genpkg", Capability: "EXEC",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.helper")}},
		{RootName: "(*os.File).Read", RootPackage: "os", Capability: "CAPABILITY_SAFE"},
		{RootName: "(*os.File).Write", RootPackage: "os", Capability: "CAPABILITY_SAFE"},
		{RootName: "(*os.File).Chmod", RootPackage: "os", Capability: "CAPABILITY_SAFE"},
		{RootName: "(*os.File).Chdir", RootPackage: "os", Capability: "MODIFY_SYSTEM_STATE/CHDIR",
			Path: []stdlibauthority.Frame{frame("(*os.File).Chdir")}},
		{RootName: "os.Open", RootPackage: "os", Capability: "CAPABILITY_SAFE"},
		{RootName: "os.ReadFile", RootPackage: "os", Capability: "FILES",
			Path: []stdlibauthority.Frame{frame("os.ReadFile")}},
		{RootName: "os.Getenv", RootPackage: "os", Capability: "CAPABILITY_UNANALYZED"},
		{RootName: "os.Exit", RootPackage: "os", Capability: "CAPABILITY_SAFE"},
	}
}

func buildFixtureMap(t *testing.T) (*gen.StdlibMap, error) {
	t.Helper()
	return buildFixtureMapWith(t, genFindings())
}

func buildFixtureMapWith(t *testing.T, findings []capslockadapter.GenerationFinding) (*gen.StdlibMap, error) {
	t.Helper()
	m, err := BuildAuthorityMap(genInventory(t), findings, nil)
	if err != nil {
		return nil, err
	}
	m.FormatVersion = artifactio.MapFormatVersion
	m.Key = &gen.SDKKey{ToolchainVersion: "fixture", MapFormatVersion: artifactio.MapFormatVersion}
	return m, nil
}

func recordOf(t *testing.T, m *gen.StdlibMap, pkg, id string) *gen.SymbolRecord {
	t.Helper()
	for _, s := range m.Symbols {
		if s.Package == pkg && s.Id == id {
			return s
		}
	}
	t.Fatalf("no symbol record for %s in %s; have %d records", id, pkg, len(m.Symbols))
	return nil
}

func initRecordOf(t *testing.T, m *gen.StdlibMap, pkg string) *gen.InitRecord {
	t.Helper()
	for _, i := range m.Inits {
		if i.Package == pkg {
			return i
		}
	}
	t.Fatalf("no init record for %s", pkg)
	return nil
}

// --- req 3/4: root normalization and func/method classification ----------------

func TestBuildAuthorityMapClassifiesFuncs(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	// Minting func with evidence.
	load := recordOf(t, m, "example.com/genpkg", "example.com/genpkg.Load")
	if load.Classification != gen.Classification_CAPABILITIES || !slices.Equal(load.Capabilities, []string{"FILES"}) {
		t.Fatalf("Load = %+v; want CAPABILITIES [FILES]", load)
	}
	// Zero-finding root is explicitly SAFE.
	trim := recordOf(t, m, "example.com/genpkg", "example.com/genpkg.Trim")
	if trim.Classification != gen.Classification_SAFE || trim.Provenance != "proved-pure" {
		t.Fatalf("Trim = %+v; want SAFE proved-pure", trim)
	}
	// Curated SAFE (os.Exit) is distinct from the minting override (os.Open is
	// CAPABILITY_SAFE but not a reclassified handle-use method).
	if got := recordOf(t, m, "os", "os.Open"); got.Classification != gen.Classification_SAFE || got.Provenance != "capslock-curated" {
		t.Fatalf("os.Open = %+v; want SAFE capslock-curated", got)
	}
	// Minting-reclassified method is a project override.
	if got := recordOf(t, m, "os", "(os.File).Read"); got.Classification != gen.Classification_SAFE || got.Provenance != "project-override" {
		t.Fatalf("(os.File).Read = %+v; want SAFE project-override", got)
	}
	// UNANALYZED is terminal.
	if got := recordOf(t, m, "os", "os.Getenv"); got.Classification != gen.Classification_UNANALYZED || len(got.Capabilities) != 0 {
		t.Fatalf("os.Getenv = %+v; want UNANALYZED", got)
	}
}

func TestBuildAuthorityMapClassifiesMethods(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	if got := recordOf(t, m, "os", "(os.File).Chdir"); got.Classification != gen.Classification_CAPABILITIES ||
		!slices.Equal(got.Capabilities, []string{"MODIFY_SYSTEM_STATE"}) {
		t.Fatalf("(os.File).Chdir = %+v; want CAPABILITIES [MODIFY_SYSTEM_STATE]", got)
	}
}

// --- req 5: consts, plain types, vars ------------------------------------------

func TestBuildAuthorityMapClassifiesVarsAndStructural(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	for _, want := range []struct{ pkg, id string }{
		{"example.com/genpkg", "example.com/genpkg.Max"},
		{"example.com/genpkg", "example.com/genpkg.Plain"},
		{"example.com/genpkg", "example.com/genpkg.Reader"},
		{"os", "os.File"},
	} {
		if got := recordOf(t, m, want.pkg, want.id); got.Classification != gen.Classification_SAFE {
			t.Fatalf("%s = %+v; want SAFE", want.id, got)
		}
	}
	// Methodless vars are SAFE.
	if got := recordOf(t, m, "example.com/genpkg", "example.com/genpkg.Count"); got.Classification != gen.Classification_SAFE {
		t.Fatalf("Count = %+v; want SAFE", got)
	}
	if got := recordOf(t, m, "example.com/genpkg", "example.com/genpkg.Client"); got.Classification != gen.Classification_SAFE {
		t.Fatalf("Client (interface-typed) = %+v; want SAFE", got)
	}
	// Handle vars combine method authority with the minting clause.
	for _, id := range []string{"os.Stdin", "os.Stdout"} {
		got := recordOf(t, m, "os", id)
		if got.Classification != gen.Classification_CAPABILITIES ||
			!slices.Equal(got.Capabilities, []string{"FILES", "MODIFY_SYSTEM_STATE"}) {
			t.Fatalf("%s = %+v; want CAPABILITIES [FILES MODIFY_SYSTEM_STATE]", id, got)
		}
	}
}

// --- req 6/3: aggregate init ----------------------------------------------------

func TestBuildAuthorityMapClassifiesAggregateInit(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	init := initRecordOf(t, m, "example.com/genpkg")
	if init.Classification != gen.Classification_CAPABILITIES || !slices.Equal(init.Capabilities, []string{"READ_SYSTEM_STATE"}) {
		t.Fatalf("genpkg.init = %+v; want CAPABILITIES [READ_SYSTEM_STATE]", init)
	}
	for _, s := range m.Symbols {
		if strings.Contains(s.Id, ".init#") || strings.HasSuffix(s.Id, ".init") {
			t.Fatalf("init leaked into the symbol inventory: %+v", s)
		}
	}
	// Zero-finding package init is explicit SAFE.
	if got := initRecordOf(t, m, "os"); got.Classification != gen.Classification_SAFE {
		t.Fatalf("os.init = %+v; want explicit SAFE", got)
	}
}

// --- req 3: drop rules and unaccounted roots -----------------------------------

func TestBuildAuthorityMapDropsNonPersistedRoots(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	for _, s := range m.Symbols {
		if strings.Contains(s.Id, "$") || strings.Contains(s.Id, ".helper") {
			t.Fatalf("dropped root leaked into the map: %+v", s)
		}
	}
	// The closure's NETWORK and the helper's EXEC must not appear anywhere.
	for _, i := range m.Inits {
		if slices.Contains(i.Capabilities, "NETWORK") || slices.Contains(i.Capabilities, "EXEC") {
			t.Fatalf("dropped root's capability leaked: %+v", i)
		}
	}
}

func TestBuildAuthorityMapFailsOnUnresolvableExportedRoot(t *testing.T) {
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "os.Undef", RootPackage: "os", Capability: "FILES",
	})
	if _, err := BuildAuthorityMap(genInventory(t), findings, nil); err == nil {
		t.Fatalf("BuildAuthorityMap: want an error for the unresolvable exported root os.Undef")
	} else if !strings.Contains(err.Error(), "os.Undef") {
		t.Fatalf("error %v: want it to name the unresolvable root", err)
	}
}

func TestBuildAuthorityMapFailsOnUnknownRootPackage(t *testing.T) {
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "example.com/other.Thing", RootPackage: "example.com/other", Capability: "FILES",
	})
	if _, err := BuildAuthorityMap(genInventory(t), findings, nil); err == nil {
		t.Fatalf("BuildAuthorityMap: want an error for a root in an unqueried package")
	}
}

// --- req 7: unsafe builtins -----------------------------------------------------

// Hermetic stand-in: the builtin rule is exercised directly; the integration
// leg pins the real unsafe.Pointer.
func TestBuiltinIsHardcodedUnanalyzed(t *testing.T) {
	class, provenance := classifyBuiltin()
	if class != gen.Classification_UNANALYZED || provenance != "project-override" {
		t.Fatalf("classifyBuiltin = (%v, %q); want UNANALYZED project-override", class, provenance)
	}
}

// --- req 8: evidence ------------------------------------------------------------

func TestEvidenceIsOrderedAndDeterministic(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	var loadEvidence *gen.Evidence
	for _, e := range m.Evidence {
		if e.SymbolId == "example.com/genpkg.Load" && e.Capability == "FILES" {
			loadEvidence = e
		}
	}
	if loadEvidence == nil {
		t.Fatalf("no evidence for (Load, FILES)")
	}
	if len(loadEvidence.Frames) == 0 || loadEvidence.Frames[0].Function != "example.com/genpkg.Load" {
		t.Fatalf("evidence frames = %+v; want the ordered root-to-use path", loadEvidence.Frames)
	}

	// Two generations with findings in different orders produce identical bytes.
	m2, err := buildFixtureMapWith(t, reversedFindings())
	if err != nil {
		t.Fatalf("BuildAuthorityMap(reversed): %v", err)
	}
	b1, err := artifactio.MarshalMap(m)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	b2, err := artifactio.MarshalMap(m2)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("map bytes depend on analyzer iteration order")
	}
}

func reversedFindings() []capslockadapter.GenerationFinding {
	out := slices.Clone(genFindings())
	slices.Reverse(out)
	return out
}

// --- req 9: reconciliation fails closed -----------------------------------------

func TestDamagedMapFailsClosedOnLookup(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	// The complete map passes the canonical validator and covers every
	// inventoried symbol exactly once.
	if _, err := artifactio.MarshalMap(m); err != nil {
		t.Fatalf("MarshalMap(complete): %v", err)
	}
	if len(m.Symbols) != len(genInventory(t).Symbols) {
		t.Fatalf("records = %d; want exact inventory equality", len(m.Symbols))
	}
	// A deliberately damaged map (one symbol record deleted) is structurally
	// valid but fails closed at lookup with an inventory-gap error: a gap can
	// never be read as pure.
	damaged := gen.StdlibMap{FormatVersion: artifactio.MapFormatVersion, Key: m.Key, Packages: m.Packages, Inits: m.Inits, Evidence: m.Evidence}
	for _, s := range m.Symbols {
		if s.Id == "example.com/genpkg.Trim" {
			continue
		}
		damaged.Symbols = append(damaged.Symbols, s)
	}
	if _, err := artifactio.MarshalMap(&damaged); err != nil {
		t.Fatalf("MarshalMap(damaged): %v", err)
	}
	authority, err := artifactio.NewStdlibMapReader(&damaged, nil)
	if err != nil {
		t.Fatalf("NewStdlibMapReader(damaged): %v", err)
	}
	if _, err := authority.SymbolAuthority("example.com/genpkg.Trim"); !errors.Is(err, stdlibauthority.ErrInventoryGap) {
		t.Fatalf("SymbolAuthority(damaged Trim) = %v; want ErrInventoryGap", err)
	}
}

// --- req 2/10: classifier rules and the SDK key ---------------------------------

func TestGenerationClassifierRulesMatchSourceMaterial(t *testing.T) {
	rules := GenerationClassifierRules()
	text, err := CanonicalClassifierText(rules)
	if err != nil {
		t.Fatalf("CanonicalClassifierText: %v", err)
	}
	methods := capslockadapter.ReclassifiedHandleUseMethods()
	if len(methods) != 22 {
		t.Fatalf("reclassified methods = %d; want 22", len(methods))
	}
	var reclassRule string
	for _, r := range rules {
		if r.Name == "minting.reclassification" {
			reclassRule = r.Effect
		}
	}
	for _, m := range methods {
		if !strings.Contains(reclassRule, m) {
			t.Fatalf("minting.reclassification effect %q misses reclassified method %q", reclassRule, m)
		}
	}
	// The generation classifier's source text is exactly the reclassification
	// material the rule descriptor names (one line per method).
	genText, err := capslockadapter.GenerationClassifierText()
	if err != nil {
		t.Fatalf("GenerationClassifierText: %v", err)
	}
	for _, m := range methods {
		if !strings.Contains(genText, "func "+m+" CAPABILITY_SAFE") {
			t.Fatalf("generation classifier text %q misses %q", genText, m)
		}
	}
	if text == "" {
		t.Fatalf("canonical classifier text is empty")
	}
}

func TestGenerateRoundTripsAndKeysTheMap(t *testing.T) {
	in := GenerationInput{
		Target:      TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64", CgoEnabled: false},
		RuleVersion: "test-rules-v1",
		Oracle:      ExplicitPackageList([]string{"os", "example.com/genpkg", "example.com/internal/seed"}),
		Loader:      typeCheckLoader(t, map[string]string{"os": genOsSrc, "example.com/genpkg": genGenSrc, "example.com/internal/seed": "package seed\n"}),
		Findings:    FindingSourceFunc(func([]string) ([]capslockadapter.GenerationFinding, error) { return genFindings(), nil }),
	}
	out, err := Generate(in)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out.Map.Key == nil || out.Map.Key.ToolchainVersion != "go1.26.4" || out.Map.Key.Goos != "linux" {
		t.Fatalf("map key = %+v; want the target's key", out.Map.Key)
	}
	if out.Map.Key.MapFormatVersion != artifactio.MapFormatVersion {
		t.Fatalf("map key format version = %d; want %d", out.Map.Key.MapFormatVersion, artifactio.MapFormatVersion)
	}
	if out.Map.FormatVersion != artifactio.MapFormatVersion {
		t.Fatalf("map format version = %d; want %d", out.Map.FormatVersion, artifactio.MapFormatVersion)
	}
	// Canonical I/O round-trip: the emitted bytes decode and carry the same key.
	decoded, err := artifactio.DecodeMap(bytes.NewReader(out.Bytes))
	if err != nil {
		t.Fatalf("DecodeMap(Generate bytes): %v", err)
	}
	if !proto.Equal(decoded.Key, out.Map.Key) {
		t.Fatalf("round-tripped key %+v != emitted %+v", decoded.Key, out.Map.Key)
	}
	digest, err := artifactio.MapDigest(out.Map)
	if err != nil {
		t.Fatalf("MapDigest: %v", err)
	}
	if digest == "" || digest != out.Digest {
		t.Fatalf("digest = %q, %v; want a stable digest on the result", out.Digest, digest)
	}
	// Repeated generation over identical inputs is byte-identical (req 10).
	out2, err := Generate(in)
	if err != nil {
		t.Fatalf("Generate (repeat): %v", err)
	}
	if !bytes.Equal(out.Bytes, out2.Bytes) {
		t.Fatalf("two generations over identical inputs produced different bytes")
	}
}

// --- the batched-run requirement (req 1) is structural: Generate calls the
// findings source exactly once with the complete importable batch. ---------------

func TestGenerateRunsCapslockOnceOverCompleteBatch(t *testing.T) {
	calls := 0
	var gotPaths []string
	loader := typeCheckLoader(t, map[string]string{"os": genOsSrc, "example.com/genpkg": genGenSrc})
	in := GenerationInput{
		Target:      TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64"},
		RuleVersion: "test-rules-v1",
		Oracle:      ExplicitPackageList([]string{"os", "example.com/genpkg"}),
		Loader:      loader,
		Findings: FindingSourceFunc(func(paths []string) ([]capslockadapter.GenerationFinding, error) {
			calls++
			gotPaths = slices.Clone(paths)
			return genFindings(), nil
		}),
	}
	if _, err := Generate(in); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("findings source called %d times; want exactly one batched run", calls)
	}
	slices.Sort(gotPaths)
	if !slices.Equal(gotPaths, []string{"example.com/genpkg", "os"}) {
		t.Fatalf("batch = %v; want the complete importable set", gotPaths)
	}
}
