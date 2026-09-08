package stdlibmap

import (
	"bytes"
	"errors"
	"go/types"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
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

type Box[T any] struct{ V T }

func (b *Box[T]) Get() T { var z T; return z }

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
		{RootName: "example.com/genpkg.init", RootPackage: "example.com/genpkg", Capability: "FILES",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.init"), frame("os.ReadFile")}},
		{RootName: "example.com/genpkg.init#1", RootPackage: "example.com/genpkg", Capability: "FILES",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.init#1")}},
		{RootName: "example.com/genpkg.Load$1", RootPackage: "example.com/genpkg", Capability: "NETWORK",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.Load$1")}},
		{RootName: "example.com/genpkg.helper", RootPackage: "example.com/genpkg", Capability: "EXEC",
			Path: []stdlibauthority.Frame{frame("example.com/genpkg.helper")}},
		{RootName: "(*os.File).Chdir", RootPackage: "os", Capability: "MODIFY_SYSTEM_STATE/CHDIR",
			Path: []stdlibauthority.Frame{frame("(*os.File).Chdir")}},
		{RootName: "os.ReadFile", RootPackage: "os", Capability: "FILES",
			Path: []stdlibauthority.Frame{frame("os.ReadFile")}},
		{RootName: "os.Getenv", RootPackage: "os", Capability: "UNANALYZED"},
	}
}

// classifierCuratedSafe simulates capslockadapter.CuratedSafeKeys over the
// fixture's classifier spellings: curated-safe and minting-reclassified roots
// produce NO findings, so their SAFE classification and provenance come from
// the classifier (the production path; round-1 finding).
func classifierCuratedSafe() map[string]bool {
	return map[string]bool{
		"os.Exit":          true,
		"os.Open":          true,
		"(*os.File).Read":  true,
		"(*os.File).Write": true,
		"(*os.File).Chmod": true,
		"(os.File).Close":  true,
	}
}

func buildFixtureMap(t *testing.T) (*gen.StdlibMap, error) {
	t.Helper()
	return buildFixtureMapWith(t, genFindings())
}

func buildFixtureMapWith(t *testing.T, findings []capslockadapter.GenerationFinding) (*gen.StdlibMap, error) {
	t.Helper()
	return buildFixtureMapCurated(t, findings, classifierCuratedSafe())
}

func buildFixtureMapCurated(t *testing.T, findings []capslockadapter.GenerationFinding, curated map[string]bool) (*gen.StdlibMap, error) {
	t.Helper()
	m, err := BuildAuthorityMap(genInventory(t), findings, curated)
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
	if init.Classification != gen.Classification_CAPABILITIES || !slices.Equal(init.Capabilities, []string{"FILES", "READ_SYSTEM_STATE"}) {
		t.Fatalf("genpkg.init = %+v; want CAPABILITIES [FILES READ_SYSTEM_STATE]", init)
	}
	// Each init capability carries its own ordered evidence path (round-1
	// finding: init capabilities were emitted without evidence).
	var initEvidence int
	for _, e := range m.Evidence {
		if e.SymbolId == "example.com/genpkg.init" {
			initEvidence++
			if len(e.Frames) == 0 || e.Frames[0].Function != "example.com/genpkg.init" {
				t.Fatalf("init evidence for %s = %+v; want the ordered aggregate-root path", e.Capability, e.Frames)
			}
		}
	}
	if initEvidence != 2 {
		t.Fatalf("init evidence entries = %d; want one per init capability", initEvidence)
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
	class, provenance, err := classifyBuiltin(projectCompletionRules().Unsafe)
	if err != nil {
		t.Fatalf("classifyBuiltin: %v", err)
	}
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
	rules, err := GenerationClassifierRules()
	if err != nil {
		t.Fatalf("GenerationClassifierRules: %v", err)
	}
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

// TestGenerationFingerprintPinsProductionClassifierInputs ties the production
// fingerprint assembly to the production classifier inputs (review F1, task
// reqs 1/3): the builtins rule's effect is Capslock's exact builtin content
// pin, and the minting-reclassification rule's effect is the exact overlay
// text NewGenerationClassifier loads — no prose surrogate.
func TestGenerationFingerprintPinsProductionClassifierInputs(t *testing.T) {
	rules, err := GenerationClassifierRules()
	if err != nil {
		t.Fatalf("GenerationClassifierRules: %v", err)
	}
	pin, err := capslockadapter.BuiltinClassifierPin()
	if err != nil {
		t.Fatalf("BuiltinClassifierPin: %v", err)
	}
	overlay, err := capslockadapter.GenerationClassifierText()
	if err != nil {
		t.Fatalf("GenerationClassifierText: %v", err)
	}
	byName := map[string]string{}
	for _, r := range rules {
		if _, dup := byName[r.Name]; dup {
			t.Fatalf("rule %q appears twice", r.Name)
		}
		byName[r.Name] = r.Effect
	}
	if got := byName["capslock.builtins.pin"]; got != pin {
		t.Fatalf("capslock.builtins.pin effect %q != the builtin content pin %q", got, pin)
	}
	if got := byName["minting.reclassification"]; got != overlay {
		t.Fatalf("minting.reclassification effect %q != the executed overlay text %q", got, overlay)
	}
	// The project completion rules' effects ARE the executed rules' canonical
	// renderings, not prose (review: assembly-to-execution drift).
	rulesCanonical := projectCompletionRules()
	if got, want := byName["unsafe.builtins"], renderUnsafeRule(rulesCanonical.Unsafe); got != want {
		t.Fatalf("unsafe.builtins effect %q != the executed rule rendering %q", got, want)
	}
	if got, want := byName["var.completion"], renderVarRule(rulesCanonical.Var); got != want {
		t.Fatalf("var.completion effect %q != the executed rule rendering %q", got, want)
	}
	if !strings.Contains(byName["var.completion"], "os.File → FILES") {
		t.Fatalf("var.completion effect %q misses the executed minting entry", byName["var.completion"])
	}
	if !strings.Contains(byName["unsafe.builtins"], "builtin-classification=UNANALYZED") ||
		!strings.Contains(byName["unsafe.builtins"], "provenance=project-override") {
		t.Fatalf("unsafe.builtins effect %q misses the executed unsafe classification", byName["unsafe.builtins"])
	}
	for _, name := range []string{"capslock.builtins.pin", "minting.reclassification", "var.completion", "unsafe.builtins"} {
		if byName[name] == "" {
			t.Fatalf("fingerprint rule %q is missing", name)
		}
	}
}

// TestClassifierFingerprintSemanticDrift proves every semantic classifier
// input moves the hash while non-semantic ordering does not (task reqs 2/4):
// the builtin pin, the overlay, and every canonical completion-rule field —
// the sources the execution paths consume — each change the fingerprint, and
// unordered minting-table iteration does not.
func TestClassifierFingerprintSemanticDrift(t *testing.T) {
	pinA, pinB := "aaa1", "aaa2"
	overlayA := "func (*os.File).Read CAPABILITY_SAFE\nfunc (*os.File).Write CAPABILITY_SAFE\n"
	overlayB := "func (*os.File).Read CAPABILITY_SAFE\nfunc (*os.File).Seek CAPABILITY_SAFE\n"
	rulesA := projectCompletionRules()

	fingerprint := func(pin, overlay string, rules completionRules) string {
		ruleset, err := classifierFingerprintRules(pin, overlay, rules)
		if err != nil {
			t.Fatalf("classifierFingerprintRules: %v", err)
		}
		text, err := CanonicalClassifierText(ruleset)
		if err != nil {
			t.Fatalf("CanonicalClassifierText: %v", err)
		}
		return ClassifierHash(GenerationDescriptor{ClassifierText: text, RuleVersion: "v1"})
	}
	base := fingerprint(pinA, overlayA, rulesA)

	if fingerprint(pinB, overlayA, rulesA) == base {
		t.Fatalf("changing the builtin pin did not change the fingerprint")
	}
	if fingerprint(pinA, overlayB, rulesA) == base {
		t.Fatalf("changing the project overlay did not change the fingerprint")
	}

	// Each executed completion-rule semantic moves the fingerprint when it
	// changes: the unsafe builtin/type classifications, the provenance, and
	// every var-policy field plus the minting table.
	ruleCases := map[string]func(*completionRules){
		"unsafe builtin classification": func(r *completionRules) { r.Unsafe.BuiltinClassification = "SAFE" },
		"unsafe type classification":    func(r *completionRules) { r.Unsafe.TypeClassification = "SAFE" },
		"unsafe provenance":             func(r *completionRules) { r.Unsafe.Provenance = ProvenanceCapslockCurated },
		"var pointer dereference":       func(r *completionRules) { r.Var.PointerDereference = false },
		"var method set":                func(r *completionRules) { r.Var.MethodSet = "value" },
		"var exported-only":             func(r *completionRules) { r.Var.ExportedOnly = false },
		"var interface-safe":            func(r *completionRules) { r.Var.InterfaceSafe = false },
		"var named-safe":                func(r *completionRules) { r.Var.NamedSafe = false },
		"var minting table entry":       func(r *completionRules) { r.Var.Minting = map[string]string{"os.File": "NETWORK"} },
		"var minting table added member": func(r *completionRules) {
			r.Var.Minting = map[string]string{"os.File": "FILES", "os.Process": "OPERATING_SYSTEM"}
		},
	}
	for name, mutate := range ruleCases {
		mutated := projectCompletionRules()
		mutate(&mutated)
		if fingerprint(pinA, overlayA, mutated) == base {
			t.Fatalf("changing the %s did not change the fingerprint", name)
		}
	}

	// The explicit rule version changes the hash (independent of the text).
	ruleset, err := classifierFingerprintRules(pinA, overlayA, rulesA)
	if err != nil {
		t.Fatalf("classifierFingerprintRules: %v", err)
	}
	text, err := CanonicalClassifierText(ruleset)
	if err != nil {
		t.Fatalf("CanonicalClassifierText: %v", err)
	}
	if ClassifierHash(GenerationDescriptor{ClassifierText: text, RuleVersion: "v2"}) == base {
		t.Fatalf("changing the rule version did not change the fingerprint")
	}

	// Minting-table iteration order is non-semantic: repeated assembly over
	// the same inputs is hash-stable.
	for i := 0; i < 50; i++ {
		if again := fingerprint(pinA, overlayA, rulesA); again != base {
			t.Fatalf("reassembly iteration %d changed the fingerprint", i)
		}
	}
}

// TestCompletionRulesDriveExecution ties the canonical completion rules to
// the execution paths (review: assembly-to-execution drift): classifyBuiltin
// and classifyVar apply exactly the canonical instance's semantics, so a
// changed rule source changes executed classifications — and, through
// classifierFingerprintRules, the SDK key.
func TestCompletionRulesDriveExecution(t *testing.T) {
	rules := projectCompletionRules()
	cls, provenance, err := classifyBuiltin(rules.Unsafe)
	if err != nil {
		t.Fatalf("classifyBuiltin: %v", err)
	}
	if cls != gen.Classification_UNANALYZED || provenance != ProvenanceProjectOverride {
		t.Fatalf("classifyBuiltin = %v, %q; want UNANALYZED, project-override", cls, provenance)
	}
	// A changed unsafe rule changes the executed classification.
	mutated := rules
	mutated.Unsafe.BuiltinClassification = "SAFE"
	cls, _, err = classifyBuiltin(mutated.Unsafe)
	if err != nil {
		t.Fatalf("classifyBuiltin(mutated): %v", err)
	}
	if cls != gen.Classification_SAFE {
		t.Fatalf("classifyBuiltin with a changed rule = %v; want the rule's SAFE", cls)
	}

	// The var rule: the fixture's os.Stdin inherits FILES from the executed
	// minting table; clearing the table's entry removes the capability.
	inv := genInventory(t)
	obj, ok := inv.loaded["os"].Scope().Lookup("Stdin").(*types.Var)
	if !ok || obj == nil {
		t.Fatalf("fixture os.Stdin did not resolve to a types.Var")
	}
	stdin := symbol.SymbolID("os.Stdin")
	result, err := classifyVar(inv, stdin, obj, map[symbol.SymbolID]*symbolResult{}, rules.Var)
	if err != nil {
		t.Fatalf("classifyVar(os.Stdin): %v", err)
	}
	if !slices.Contains(result.caps, "FILES") {
		t.Fatalf("os.Stdin = %+v; want the executed minting capability FILES", result)
	}
	mutated = rules
	mutated.Var.Minting = map[string]string{}
	result, err = classifyVar(inv, stdin, obj, map[symbol.SymbolID]*symbolResult{}, mutated.Var)
	if err != nil {
		t.Fatalf("classifyVar(os.Stdin, cleared minting): %v", err)
	}
	if slices.Contains(result.caps, "FILES") {
		t.Fatalf("os.Stdin with a cleared minting table = %+v; want no inherited FILES", result)
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

// --- round-1: promoted and alias-exposed variable authority ---------------------

const varPkgSrc = `package varpkg

type Base struct{}

func (b Base) Ping() error { return nil }

type Wrapper struct {
	Base
}

type Mixed struct{}

func (m Mixed) Proved() {}

func (m Mixed) Curated() {}

type hidden struct{}

func (h *hidden) Touch() {}

type Alias = hidden

var Promoted Wrapper
var Aliased Alias
var MixedVar Mixed
`

// varFindings carries the canned findings for the varpkg fixture: the
// promoted Base method reaches NETWORK, the alias-exposed unexported receiver
// method reaches READ_SYSTEM_STATE, and Mixed.Curated is classifier-curated.
func varFindings() []capslockadapter.GenerationFinding {
	return []capslockadapter.GenerationFinding{
		{RootName: "(example.com/varpkg.Base).Ping", RootPackage: "example.com/varpkg", Capability: "NETWORK",
			Path: []stdlibauthority.Frame{{Function: "(example.com/varpkg.Base).Ping", File: "p.go", Line: 4}}},
		{RootName: "(example.com/varpkg.hidden).Touch", RootPackage: "example.com/varpkg", Capability: "READ_SYSTEM_STATE",
			Path: []stdlibauthority.Frame{{Function: "(example.com/varpkg.hidden).Touch", File: "h.go", Line: 11}}},
	}
}

func varCuratedSafe() map[string]bool {
	return map[string]bool{"(*example.com/varpkg.Mixed).Curated": true}
}

func TestVarRuleCoversPromotedAndAliasExposedMethods(t *testing.T) {
	loader := typeCheckLoader(t, map[string]string{"example.com/varpkg": varPkgSrc})
	inv, err := BuildInventory([]PackageEntry{{Path: "example.com/varpkg", Importable: true}}, loader)
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}
	m, err := BuildAuthorityMap(inv, varFindings(), varCuratedSafe())
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	promoted := recordOf(t, m, "example.com/varpkg", "example.com/varpkg.Promoted")
	if promoted.Classification != gen.Classification_CAPABILITIES || !slices.Equal(promoted.Capabilities, []string{"NETWORK"}) {
		t.Fatalf("Promoted = %+v; want the promoted Base method's NETWORK", promoted)
	}
	aliased := recordOf(t, m, "example.com/varpkg", "example.com/varpkg.Aliased")
	if aliased.Classification != gen.Classification_CAPABILITIES || !slices.Equal(aliased.Capabilities, []string{"READ_SYSTEM_STATE"}) {
		t.Fatalf("Aliased = %+v; want the alias-exposed receiver method's READ_SYSTEM_STATE", aliased)
	}
	// The curated method contributes a SAFE trust decision without faking a
	// capability record; MixedVar stays terminal-SAFE with inherited provenance.
	mixed := recordOf(t, m, "example.com/varpkg", "example.com/varpkg.MixedVar")
	if mixed.Classification != gen.Classification_SAFE || mixed.Provenance != "capslock-curated" {
		t.Fatalf("MixedVar = %+v; want SAFE capslock-curated", mixed)
	}
}

// --- round-1: SAFE provenance precedence is order-independent -------------------

func TestVarProvenancePrecedenceIndependentOfOrder(t *testing.T) {
	override := &symbolResult{classification: gen.Classification_SAFE, provenance: ProvenanceProjectOverride}
	curated := &symbolResult{classification: gen.Classification_SAFE, provenance: ProvenanceCapslockCurated}
	proved := &symbolResult{classification: gen.Classification_SAFE, provenance: ProvenanceProvedPure}
	forward := &symbolResult{}
	forward.mergeFrom(proved)
	forward.mergeFrom(curated)
	forward.mergeFrom(override)
	reverse := &symbolResult{}
	reverse.mergeFrom(override)
	reverse.mergeFrom(curated)
	reverse.mergeFrom(proved)
	for name, r := range map[string]*symbolResult{"forward": forward, "reverse": reverse} {
		r.finish()
		if r.provenance != ProvenanceProjectOverride {
			t.Fatalf("%s merge = %q; want project-override regardless of visit order", name, r.provenance)
		}
	}
	curatedOnly := &symbolResult{}
	curatedOnly.mergeFrom(proved)
	curatedOnly.mergeFrom(curated)
	curatedOnly.finish()
	if curatedOnly.provenance != ProvenanceCapslockCurated {
		t.Fatalf("proved-then-curated merge = %q; want capslock-curated", curatedOnly.provenance)
	}
}

// --- round-1: unknown-package roots fail even for droppable shapes --------------

func TestUnknownPackageClosureFailsGeneration(t *testing.T) {
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "example.com/other.Load$1", RootPackage: "example.com/other", Capability: "NETWORK",
	})
	if _, err := BuildAuthorityMap(genInventory(t), findings, classifierCuratedSafe()); err == nil {
		t.Fatalf("BuildAuthorityMap: want an error for an unknown-package closure root")
	} else if !strings.Contains(err.Error(), "example.com/other") {
		t.Fatalf("error %v: want it to name the unknown package", err)
	}
}

// --- round-1: the minting frame names the handle var in full --------------------

func TestMintingEvidenceFrameIsFullyQualified(t *testing.T) {
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	for _, e := range m.Evidence {
		if e.SymbolId == "os.Stdin" && e.Capability == "FILES" {
			if len(e.Frames) != 1 || e.Frames[0].Function != "os.Stdin" {
				t.Fatalf("minting evidence frames = %+v; want the fully qualified os.Stdin frame", e.Frames)
			}
			return
		}
	}
	t.Fatalf("no minting evidence for (os.Stdin, FILES)")
}

// --- round-1: curated-safe aggregate inits --------------------------------------

func TestCuratedInitProvenance(t *testing.T) {
	inv := genInventory(t)
	curatedOnly, err := BuildAuthorityMap(inv, []capslockadapter.GenerationFinding{
		{RootName: "example.com/genpkg.Load", RootPackage: "example.com/genpkg", Capability: "FILES",
			Path: []stdlibauthority.Frame{{Function: "example.com/genpkg.Load"}}},
	}, map[string]bool{"os.init": true})
	if err != nil {
		t.Fatalf("BuildAuthorityMap(curated init): %v", err)
	}
	// The InitRecord provenance field (round-2 finding) keeps a curated SAFE
	// init visibly distinct from a proved-pure one in the persisted artifact.
	for _, i := range curatedOnly.Inits {
		if i.Package == "os" && (i.Classification != gen.Classification_SAFE || i.Provenance != "capslock-curated") {
			t.Fatalf("curated os.init = %+v; want SAFE capslock-curated", i)
		}
	}
	// The persisted form carries the annotation through canonical I/O.
	curatedOnly.FormatVersion = artifactio.MapFormatVersion
	curatedOnly.Key = &gen.SDKKey{ToolchainVersion: "fixture", MapFormatVersion: artifactio.MapFormatVersion}
	if _, err := artifactio.MarshalMap(curatedOnly); err != nil {
		t.Fatalf("MarshalMap(curated init): %v", err)
	}
}

// TestClassifierSpellings pins the Capslock display spellings the provenance
// pass queries: the pointer form and the canonical value form, with the
// receiver type name intact (round-2 finding: the value form dropped it).
func TestClassifierSpellings(t *testing.T) {
	id, err := symbol.Parse("(net.Flags).String")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := classifierSpellings(id)
	want := []string{"(*net.Flags).String", "(net.Flags).String"}
	if !slices.Equal(got, want) {
		t.Fatalf("classifierSpellings = %v; want %v", got, want)
	}
	top, err := symbol.Parse("os.ReadFile")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := classifierSpellings(top); !slices.Equal(got, []string{"os.ReadFile"}) {
		t.Fatalf("classifierSpellings(top-level) = %v; want the bare spelling", got)
	}
}

// --- round-3: generic-instantiation roots are discarded, not merged -------------

func TestInstantiationRootIsDroppedNotMerged(t *testing.T) {
	// A capability-bearing instantiated root must not attach its capability
	// to the uninstantiated inventory symbol: Load stays [FILES] from its own
	// findings only.
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "example.com/genpkg.Load[int]", RootPackage: "example.com/genpkg", Capability: "NETWORK",
		Path: []stdlibauthority.Frame{{Function: "example.com/genpkg.Load[int]", File: "x.go", Line: 7}},
	})
	m, err := buildFixtureMapCurated(t, findings, classifierCuratedSafe())
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	load := recordOf(t, m, "example.com/genpkg", "example.com/genpkg.Load")
	if !slices.Equal(load.Capabilities, []string{"FILES"}) {
		t.Fatalf("Load = %+v; want [FILES] with the instantiation root discarded", load)
	}
	for _, e := range m.Evidence {
		if e.SymbolId == "example.com/genpkg.Load" && e.Capability == "NETWORK" {
			t.Fatalf("the discarded instantiation's NETWORK capability leaked into the origin record")
		}
	}
}

func TestMalformedBracketedRootFailsClosed(t *testing.T) {
	for _, name := range []string{"example.com/genpkg.Load[in[t", "example.com/genpkg.Load]x", "example.com/genpkg.Load[int][bool]"} {
		findings := append(genFindings(), capslockadapter.GenerationFinding{
			RootName: name, RootPackage: "example.com/genpkg", Capability: "NETWORK",
		})
		if _, err := BuildAuthorityMap(genInventory(t), findings, classifierCuratedSafe()); err == nil {
			t.Fatalf("BuildAuthorityMap(%q): want a fail-closed error for the malformed bracketed root", name)
		}
	}
}

// --- round-4: instantiation roots validate through the Step 3 parser ------------

// TestGenericMethodInstantiationRootIsDroppedNotMerged covers a receiver
// instantiation spelling: the Step 3 parser validates the type arguments and
// confirms the uninstantiated declaration through the inventory, and the
// validated instantiation is discarded without merging its findings — the
// generic method Get stays SAFE from its own (empty) findings alone.
func TestGenericMethodInstantiationRootIsDroppedNotMerged(t *testing.T) {
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "(*example.com/genpkg.Box[int]).Get", RootPackage: "example.com/genpkg", Capability: "NETWORK",
		Path: []stdlibauthority.Frame{{Function: "(*example.com/genpkg.Box[int]).Get", File: "x.go", Line: 9}},
	})
	m, err := buildFixtureMapCurated(t, findings, classifierCuratedSafe())
	if err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
	get := recordOf(t, m, "example.com/genpkg", "(example.com/genpkg.Box).Get")
	if get.Classification != gen.Classification_SAFE {
		t.Fatalf("(genpkg.Box).Get = %+v; want SAFE with the instantiation root discarded", get)
	}
	for _, e := range m.Evidence {
		if e.SymbolId == "(example.com/genpkg.Box).Get" && e.Capability == "NETWORK" {
			t.Fatalf("the discarded instantiation's NETWORK capability leaked into the origin record")
		}
	}
}

// TestInvalidTypeArgumentRootFailsClosed pins the Step 3 type-argument
// grammar: empty and non-type bracket contents fail generation instead of
// being silently dropped as instantiations.
func TestInvalidTypeArgumentRootFailsClosed(t *testing.T) {
	for _, name := range []string{"example.com/genpkg.Load[]", "example.com/genpkg.Load[not a type]"} {
		findings := append(genFindings(), capslockadapter.GenerationFinding{
			RootName: name, RootPackage: "example.com/genpkg", Capability: "NETWORK",
		})
		if _, err := BuildAuthorityMap(genInventory(t), findings, classifierCuratedSafe()); err == nil {
			t.Fatalf("BuildAuthorityMap(%q): want a fail-closed error for the invalid type argument", name)
		}
	}
}

// TestUnknownExportedGenericRootFailsClosed pins the inventory confirmation:
// an exported generic origin the inventory cannot confirm fails generation
// instead of being dropped as an instantiation.
func TestUnknownExportedGenericRootFailsClosed(t *testing.T) {
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "example.com/genpkg.Missing[int]", RootPackage: "example.com/genpkg", Capability: "NETWORK",
	})
	_, err := BuildAuthorityMap(genInventory(t), findings, classifierCuratedSafe())
	if err == nil {
		t.Fatalf("BuildAuthorityMap: want a fail-closed error for the unknown exported generic root")
	}
	if !strings.Contains(err.Error(), "example.com/genpkg.Missing") {
		t.Fatalf("error %v: want it to name the unconfirmable origin", err)
	}
}

// TestUnexportedGenericOriginRootIsDropped pins the one sanctioned drop for a
// bracketed spelling: an unexported generic origin is a helper folded into
// its exported callers.
func TestUnexportedGenericOriginRootIsDropped(t *testing.T) {
	findings := append(genFindings(), capslockadapter.GenerationFinding{
		RootName: "example.com/genpkg.load[int]", RootPackage: "example.com/genpkg", Capability: "NETWORK",
	})
	if _, err := BuildAuthorityMap(genInventory(t), findings, classifierCuratedSafe()); err != nil {
		t.Fatalf("BuildAuthorityMap: %v", err)
	}
}
