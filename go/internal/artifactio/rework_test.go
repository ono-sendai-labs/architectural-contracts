package artifactio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

// mapWithFullInventory extends validMap with build tags, a second init, and
// evidence with frames, so every normalized collection is exercised.
func mapWithFullInventory() *gen.StdlibMap {
	m := validMap()
	m.Key.BuildTags = []string{"a", "b"}
	m.Inits = []*gen.InitRecord{
		{Package: "bytes", Classification: gen.Classification_SAFE},
		{Package: "os", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"FILES"}},
	}
	m.Evidence = []*gen.Evidence{
		{
			SymbolId:   "(os.File).Read",
			Capability: "FILES",
			Frames: []*gen.Frame{
				{Function: "h", File: "c.go", Line: 3},
				{Function: "f", File: "a.go", Line: 1},
				{Function: "g", File: "b.go", Line: 2},
			},
		},
		{
			SymbolId:   "(os.File).Read",
			Capability: "READ_SYSTEM_STATE",
			Frames: []*gen.Frame{
				{Function: "h", File: "c.go", Line: 3},
				{Function: "f", File: "a.go", Line: 1},
				{Function: "g", File: "b.go", Line: 2},
			},
		},
	}
	return m
}

// TestMapCanonicalizesEveryRepeatedField pins rework finding "tests omit
// frames and map immutability": reordered inits, build tags, evidence entries,
// and capabilities canonicalize to identical bytes (frame paths are ordered
// sequences, not keyed sets, so entry order within one evidence list is
// preserved — see TestMapPreservesEvidenceFrameOrder), and the input map is
// not mutated.
func TestMapCanonicalizesEveryRepeatedField(t *testing.T) {
	a := mapWithFullInventory()
	b := mapWithFullInventory()
	b.Key.BuildTags = []string{"b", "a"}
	b.Symbols = []*gen.SymbolRecord{
		{Package: "os", Id: "(os.File).Read", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"READ_SYSTEM_STATE", "FILES"}},
		{Package: "bytes", Id: "bytes.Buffer", Classification: gen.Classification_SAFE},
	}
	b.Packages = []*gen.PackageInventory{{Path: "os", Importable: true}, {Path: "internal/secret", Importable: false}, {Path: "bytes", Importable: true}}
	b.Inits = []*gen.InitRecord{
		{Package: "os", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"FILES"}},
		{Package: "bytes", Classification: gen.Classification_SAFE},
	}
	b.Evidence = []*gen.Evidence{
		{
			SymbolId:   "(os.File).Read",
			Capability: "READ_SYSTEM_STATE",
			Frames: []*gen.Frame{
				{Function: "h", File: "c.go", Line: 3},
				{Function: "f", File: "a.go", Line: 1},
				{Function: "g", File: "b.go", Line: 2},
			},
		},
		{
			SymbolId:   "(os.File).Read",
			Capability: "FILES",
			Frames: []*gen.Frame{
				{Function: "h", File: "c.go", Line: 3},
				{Function: "f", File: "a.go", Line: 1},
				{Function: "g", File: "b.go", Line: 2},
			},
		},
	}

	before := proto.Clone(a)
	gotA, err := MarshalMap(a)
	if err != nil {
		t.Fatalf("MarshalMap(a): %v", err)
	}
	gotB, err := MarshalMap(b)
	if err != nil {
		t.Fatalf("MarshalMap(b): %v", err)
	}
	if string(gotA) != string(gotB) {
		t.Errorf("reordered map inputs produced different bytes:\nA: %s\nB: %s", gotA, gotB)
	}
	if !proto.Equal(before, a) {
		t.Error("MarshalMap mutated its input map")
	}
}

// TestSurfaceCanonicalizesSDKBuildTags pins rework finding "surface SDK build
// tags are not canonicalized".
func TestSurfaceCanonicalizesSDKBuildTags(t *testing.T) {
	a := validSurface()
	a.SdkKey = &gen.SDKKey{ToolchainVersion: "go1.26.4", Goos: "linux", Goarch: "amd64", BuildTags: []string{"a", "b"}, MapFormatVersion: 1}
	b := validSurface()
	b.SdkKey = &gen.SDKKey{ToolchainVersion: "go1.26.4", Goos: "linux", Goarch: "amd64", BuildTags: []string{"b", "a"}, MapFormatVersion: 1}

	gotA, err := MarshalSurface(a)
	if err != nil {
		t.Fatalf("MarshalSurface(a): %v", err)
	}
	gotB, err := MarshalSurface(b)
	if err != nil {
		t.Fatalf("MarshalSurface(b): %v", err)
	}
	if string(gotA) != string(gotB) {
		t.Errorf("reordered SDK build tags produced different bytes:\nA: %s\nB: %s", gotA, gotB)
	}
	da, err := SurfaceDigest(a)
	if err != nil {
		t.Fatalf("SurfaceDigest(a): %v", err)
	}
	db, err := SurfaceDigest(b)
	if err != nil {
		t.Fatalf("SurfaceDigest(b): %v", err)
	}
	if da != db {
		t.Errorf("digests differ for tag-reordered equivalents: %s vs %s", da, db)
	}
}

// TestMarshalRejectsNilRepeatedEntries pins rework finding "malformed repeated
// map entries can panic": nil repeated-message elements produce errors, never
// panics.
func TestMarshalRejectsNilRepeatedEntries(t *testing.T) {
	m := validMap()
	m.Packages = append(m.Packages, nil)
	assertMarshalError(t, m, "nil package inventory entry")

	m = validMap()
	m.Symbols = append(m.Symbols, nil)
	assertMarshalError(t, m, "nil symbol record")

	m = validMap()
	m.Inits = append(m.Inits, nil)
	assertMarshalError(t, m, "nil init record")

	m = validMap()
	m.Evidence = append(m.Evidence, nil)
	assertMarshalError(t, m, "nil evidence entry")

	m = validMap()
	m.Evidence = []*gen.Evidence{{
		SymbolId:   "bytes.Buffer",
		Capability: "FILES",
		Frames:     []*gen.Frame{nil},
	}}
	assertMarshalError(t, m, "nil frame")

	s := validSurface()
	s.SdkKey = nil // key is optional on surfaces; must not panic
	if _, err := MarshalSurface(s); err != nil {
		t.Errorf("MarshalSurface(nil SDK key): %v", err)
	}
}

func assertMarshalError(t *testing.T, m *gen.StdlibMap, want string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MarshalMap panicked on %q: %v", want, r)
		}
	}()
	_, err := MarshalMap(m)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("MarshalMap error = %v, want containing %q", err, want)
	}
}

// TestMapRejectsNonImportableAndMismatchedRecords pins rework findings "map
// validation permits records for non-importable packages" and "package/ID
// disagreement".
func TestMapRejectsNonImportableAndMismatchedRecords(t *testing.T) {
	m := validMap()
	m.Symbols = []*gen.SymbolRecord{{Package: "internal/secret", Id: "internal/secret.F", Classification: gen.Classification_SAFE}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "non-importable") {
		t.Errorf("symbol under non-importable package: err = %v, want non-importable rejection", err)
	}

	m = validMap()
	m.Inits = []*gen.InitRecord{{Package: "internal/secret", Classification: gen.Classification_SAFE}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "non-importable") {
		t.Errorf("init under non-importable package: err = %v, want non-importable rejection", err)
	}

	m = validMap()
	m.Symbols = []*gen.SymbolRecord{{Package: "bytes", Id: "os.Exit", Classification: gen.Classification_SAFE}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "package \"bytes\"") {
		t.Errorf("symbol with mismatched declaring package: err = %v, want package mismatch rejection", err)
	}
}

// evidenceFrame returns one concrete frame so evidence fixtures carry a
// non-empty path, as required of persisted evidence.
func evidenceFrame() *gen.Frame {
	return &gen.Frame{Function: "f", File: "a.go", Line: 1}
}

// TestMapEvidenceValidation pins rework finding "evidence symbol IDs are not
// validated": evidence IDs must parse, name the recorded package, and attach
// to a CAPABILITIES record listing the evidence capability.
func TestMapEvidenceValidation(t *testing.T) {
	m := validMap()
	m.Evidence = []*gen.Evidence{{SymbolId: "not a symbol", Capability: "FILES", Frames: []*gen.Frame{evidenceFrame()}}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "symbol") {
		t.Errorf("malformed evidence symbol ID: err = %v, want symbol error", err)
	}

	m = validMap()
	m.Evidence = []*gen.Evidence{{SymbolId: "bytes.Buffer", Capability: "FILES", Frames: []*gen.Frame{evidenceFrame()}}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "CAPABILITIES") {
		t.Errorf("evidence against a non-CAPABILITIES symbol: err = %v, want CAPABILITIES rejection", err)
	}

	m = validMap()
	m.Evidence = []*gen.Evidence{{SymbolId: "(os.File).Read", Capability: "EXEC", Frames: []*gen.Frame{evidenceFrame()}}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "capability") {
		t.Errorf("evidence capability absent from the record: err = %v, want capability rejection", err)
	}

	m = validMap()
	m.Evidence = []*gen.Evidence{{SymbolId: "(bytes.Buffer).Read", Capability: "FILES", Frames: []*gen.Frame{evidenceFrame()}}}
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "absent from the inventory") {
		t.Errorf("evidence symbol from a foreign package: err = %v, want package rejection", err)
	}
}

// TestSurfaceRejectsNonCanonicalShape pins rework finding "surface validation
// accepts non-canonical shapes": unknown interface styles, PACKAGE_SURFACE
// surfaces with symbols, and non-concrete package paths are rejected.
func TestSurfaceRejectsNonCanonicalShape(t *testing.T) {
	s := validSurface()
	s.InterfaceStyle = gen.InterfaceStyle(99)
	if _, err := MarshalSurface(s); err == nil || !strings.Contains(err.Error(), "interface_style") {
		t.Errorf("unknown interface style: err = %v, want interface_style error", err)
	}

	s = validSurface()
	s.InterfaceStyle = gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE
	if _, err := MarshalSurface(s); err == nil || !strings.Contains(err.Error(), "symbols") {
		t.Errorf("PACKAGE_SURFACE with symbols: err = %v, want symbols rejection", err)
	}

	s = validSurface()
	s.Packages = []string{"a.example/*"}
	if _, err := MarshalSurface(s); err == nil || !strings.Contains(err.Error(), "package") {
		t.Errorf("wildcard package path: err = %v, want package rejection", err)
	}

	s = validSurface()
	s.Packages = []string{"a.example//pkg"}
	if _, err := MarshalSurface(s); err == nil || !strings.Contains(err.Error(), "package") {
		t.Errorf("empty path element: err = %v, want package rejection", err)
	}

	s = validSurface()
	s.SdkKey = &gen.SDKKey{BuildTags: []string{"a", "a"}}
	if _, err := MarshalSurface(s); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("duplicate SDK build tags: err = %v, want duplicate rejection", err)
	}
}

// TestWriteFileAtomicAppliesRequestedMode pins rework finding "atomic writer
// ignores the requested file mode": both a new and an existing target end up
// with the requested permission bits.
func TestWriteFileAtomicAppliesRequestedMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact.json")
	next := []byte("{\"next\":true}")
	if err := WriteFileAtomic(target, next, 0o640, nil); err != nil {
		t.Fatalf("WriteFileAtomic (new target): %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("new target mode = %v, want 0640", got)
	}

	if err := WriteFileAtomic(target, next, 0o600, nil); err != nil {
		t.Fatalf("WriteFileAtomic (existing target): %v", err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("existing target mode = %v, want 0600", got)
	}
}

// TestAtomicWriteSurfacesCleanupFailure pins rework finding "atomic cleanup
// failures are silently discarded": a failing removal is surfaced alongside the
// original error, without losing either.
func TestAtomicWriteSurfacesCleanupFailure(t *testing.T) {
	_, target, previous := writeHarness(t)
	seams := &WriteSeams{
		Rename: func(string, string) error { return errSeam },
		Remove: func(string) error { return errCleanup },
	}
	err := WriteFileAtomic(target, []byte("{\"next\":true}"), 0o644, seams)
	if err == nil {
		t.Fatal("WriteFileAtomic succeeded despite injected failures")
	}
	if !strings.Contains(err.Error(), errSeam.Error()) {
		t.Errorf("error %v lost the original failure", err)
	}
	if !strings.Contains(err.Error(), errCleanup.Error()) {
		t.Errorf("error %v lost the cleanup failure", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != previous {
		t.Errorf("target = %q (%v), want %q", got, err, previous)
	}
}

var errCleanup = os.ErrPermission
