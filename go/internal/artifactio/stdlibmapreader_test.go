package artifactio

import (
	"errors"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// expectedKey is the SDK key of validMap's fixture, expressed core-side.

func expectedKeyPtr() *stdlibauthority.SDKKey {
	k := expectedKey()
	return &k
}
func expectedKey() stdlibauthority.SDKKey {
	return stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		BuildTags:        []string{"a", "b"},
		MapFormatVersion: MapFormatVersion,
	}
}

// TestStdlibMapReaderMembership pins AC1: membership comes only from the
// map's enumeration — importable packages, non-importable internal packages,
// and nothing else (no path heuristic).
func TestStdlibMapReaderMembership(t *testing.T) {
	reader, err := NewStdlibMapReader(mapWithFullInventory(), expectedKeyPtr())
	if err != nil {
		t.Fatalf("NewStdlibMapReader: %v", err)
	}
	if !reader.IsStdlibPackage("os") {
		t.Error("IsStdlibPackage(os) = false, want true (importable member)")
	}
	if !reader.IsStdlibPackage("internal/secret") {
		t.Error("IsStdlibPackage(internal/secret) = false, want true (non-importable member)")
	}
	for _, absent := range []string{"example.com/notstdlib", "os/", "strings"} {
		if reader.IsStdlibPackage(absent) {
			t.Errorf("IsStdlibPackage(%q) = true, want false", absent)
		}
	}
}

// TestStdlibMapReaderLookups pins AC2: known symbols and inits return their
// exact terminal classifications; absent entries fail closed with
// ErrInventoryGap naming the absent package and symbol, never SAFE.
func TestStdlibMapReaderLookups(t *testing.T) {
	m := mapWithFullInventory()
	m.Packages = append(m.Packages, &gen.PackageInventory{Path: "unsafe", Importable: true})
	m.Inits = append(m.Inits, &gen.InitRecord{Package: "unsafe", Classification: gen.Classification_SAFE})
	m.Symbols = append(m.Symbols,
		&gen.SymbolRecord{Package: "os", Id: "os.Open", Classification: gen.Classification_SAFE},
		&gen.SymbolRecord{Package: "unsafe", Id: "unsafe.Pointer", Classification: gen.Classification_UNANALYZED},
	)
	reader, err := NewStdlibMapReader(m, expectedKeyPtr())
	if err != nil {
		t.Fatalf("NewStdlibMapReader: %v", err)
	}

	for _, tc := range []struct {
		name string
		id   string
		want stdlibauthority.Classification
	}{
		{"capabilities symbol", "(os.File).Read", stdlibauthority.Classification{Capabilities: []stdlibauthority.Capability{"FILES", "READ_SYSTEM_STATE"}}},
		{"safe symbol", "os.Open", stdlibauthority.Classification{Safe: true}},
		{"safe type symbol", "bytes.Buffer", stdlibauthority.Classification{Safe: true}},
		{"unanalyzed symbol", "unsafe.Pointer", stdlibauthority.Classification{Unanalyzed: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reader.SymbolAuthority(symbol.SymbolID(tc.id))
			if err != nil {
				t.Fatalf("SymbolAuthority(%s): %v", tc.id, err)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("returned classification is not terminal: %v", err)
			}
			if got.Safe != tc.want.Safe || got.Unanalyzed != tc.want.Unanalyzed || strings.Join(got.Capabilities, ",") != strings.Join(tc.want.Capabilities, ",") {
				t.Errorf("SymbolAuthority(%s) = %+v, want %+v", tc.id, got, tc.want)
			}
		})
	}

	// Inits: exact classification, and absence fails closed.
	if got, err := reader.PackageInitAuthority("os"); err != nil || got.Safe || got.Unanalyzed || strings.Join(got.Capabilities, ",") != "FILES" {
		t.Errorf("PackageInitAuthority(os) = %+v (%v), want CAPABILITIES{FILES}", got, err)
	}
	if got, err := reader.PackageInitAuthority("bytes"); err != nil || !got.Safe {
		t.Errorf("PackageInitAuthority(bytes) = %+v (%v), want SAFE", got, err)
	}

	// Absent entries: inventory gaps, never SAFE.
	gapChecks := []struct {
		name    string
		symErr  error
		initErr error
		pkg     string
		sym     string
	}{
		{"absent symbol in enumerated package", mustSymErr(t, reader, "os.Nope"), nil, "os", "os.Nope"},
		{"symbol in non-importable package", mustSymErr(t, reader, "internal/secret.F"), nil, "internal/secret", "internal/secret.F"},
		{"symbol in unlisted package", mustSymErr(t, reader, "example.com/x.Y"), nil, "example.com/x", "example.com/x.Y"},
	}
	for _, g := range gapChecks {
		t.Run(g.name, func(t *testing.T) {
			if !errors.Is(g.symErr, stdlibauthority.ErrInventoryGap) {
				t.Fatalf("err = %v, want ErrInventoryGap", g.symErr)
			}
			var ig *stdlibauthority.InventoryGapError
			if !errors.As(g.symErr, &ig) || ig.Package != g.pkg || ig.Symbol != g.sym {
				t.Errorf("gap error = %v, want Package=%q Symbol=%q", g.symErr, g.pkg, g.sym)
			}
		})
	}
	t.Run("absent init", func(t *testing.T) {
		got, err := reader.PackageInitAuthority("example.com/x")
		if got.Validate() == nil {
			t.Errorf("PackageInitAuthority(example.com/x) gap classification %+v must be invalid (never SAFE)", got)
		}
		if !errors.Is(err, stdlibauthority.ErrInventoryGap) {
			t.Fatalf("err = %v, want ErrInventoryGap", err)
		}
		var ig *stdlibauthority.InventoryGapError
		if !errors.As(err, &ig) || ig.Package != "example.com/x" || ig.Symbol != "" {
			t.Errorf("gap error = %v, want Package=example.com/x", err)
		}
	})
}

// TestStdlibMapReaderCanonicalizesCapabilities pins review finding
// "reader does not enforce sorted capability sets": a valid in-memory record
// whose capability list is unsorted is indexed canonically — lookups return
// the sorted set — and the classification passes the core terminal
// validation.
func TestStdlibMapReaderCanonicalizesCapabilities(t *testing.T) {
	m := mapWithFullInventory()
	m.Symbols[1].Capabilities = []string{"READ_SYSTEM_STATE", "FILES"}
	reader, err := NewStdlibMapReader(m, expectedKeyPtr())
	if err != nil {
		t.Fatalf("NewStdlibMapReader: %v", err)
	}
	got, err := reader.SymbolAuthority(symbol.SymbolID("(os.File).Read"))
	if err != nil {
		t.Fatalf("SymbolAuthority: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("returned classification not canonical: %v", err)
	}
	if strings.Join(got.Capabilities, ",") != "FILES,READ_SYSTEM_STATE" {
		t.Errorf("capabilities = %v, want sorted [FILES READ_SYSTEM_STATE]", got.Capabilities)
	}
}

// TestStdlibMapReaderKeyIsDefensiveCopy pins review finding "Key exposes
// mutable build-tag storage": mutating a returned key's tags must not change
// subsequent Key() observations.
func TestStdlibMapReaderKeyIsDefensiveCopy(t *testing.T) {
	reader, err := NewStdlibMapReader(mapWithFullInventory(), expectedKeyPtr())
	if err != nil {
		t.Fatalf("NewStdlibMapReader: %v", err)
	}
	key := reader.Key()
	key.BuildTags[0] = "mutated"
	key.BuildTags = append(key.BuildTags, "extra")
	again := reader.Key()
	if strings.Join(again.BuildTags, ",") != "a,b" {
		t.Errorf("mutated returned key leaked into adapter state: %v", again.BuildTags)
	}
}

// TestStdlibMapReaderEvidenceOrderAndCopies pins AC4's evidence half: frame
// order is preserved, and returned slices are defensive copies that callers
// cannot use to mutate adapter state.
func TestStdlibMapReaderEvidenceOrderAndCopies(t *testing.T) {
	reader, err := NewStdlibMapReader(mapWithFullInventory(), expectedKeyPtr())
	if err != nil {
		t.Fatalf("NewStdlibMapReader: %v", err)
	}
	frames := reader.Evidence(symbol.SymbolID("(os.File).Read"), "FILES")
	if len(frames) != 3 || frames[0].Function != "h" || frames[1].Function != "f" || frames[2].Function != "g" {
		t.Fatalf("Evidence frames = %+v, want ordered [h f g]", frames)
	}
	frames[0].Function = "mutated"
	frames = append(frames, stdlibauthority.Frame{Function: "extra"})
	again := reader.Evidence(symbol.SymbolID("(os.File).Read"), "FILES")
	if len(again) != 3 || again[0].Function != "h" {
		t.Errorf("mutated returned slice leaked into adapter state: %+v", again)
	}

	// Capability slices are defensive copies too.
	classification, err := reader.SymbolAuthority(symbol.SymbolID("(os.File).Read"))
	if err != nil {
		t.Fatalf("SymbolAuthority: %v", err)
	}
	classification.Capabilities[0] = "MUTATED"
	classification.Capabilities = append(classification.Capabilities, "EXTRA")
	againClass, err := reader.SymbolAuthority(symbol.SymbolID("(os.File).Read"))
	if err != nil {
		t.Fatalf("SymbolAuthority: %v", err)
	}
	if strings.Join(againClass.Capabilities, ",") != "FILES,READ_SYSTEM_STATE" {
		t.Errorf("mutated capability slice leaked into adapter state: %v", againClass.Capabilities)
	}
}

// TestStdlibMapReaderRejectsInvalidInMemoryStates pins AC3 + technical
// requirement 5: terminal states handed to the constructor as an in-memory
// message, outside the decoder, are still validated — empty, overlapping,
// and duplicate states are rejected.
func TestStdlibMapReaderRejectsInvalidInMemoryStates(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*gen.StdlibMap)
		wantErr string
	}{
		{
			name:    "SAFE record carrying capabilities",
			mutate:  func(m *gen.StdlibMap) { m.Symbols[1].Classification = gen.Classification_SAFE },
			wantErr: "carries capabilities",
		},
		{
			name: "UNSPECIFIED classification",
			mutate: func(m *gen.StdlibMap) {
				m.Symbols[1].Classification = gen.Classification_CLASSIFICATION_UNSPECIFIED
				m.Symbols[1].Capabilities = nil
			},
			wantErr: "terminal",
		},
		{
			name: "duplicate symbol record",
			mutate: func(m *gen.StdlibMap) {
				m.Symbols = append(m.Symbols, &gen.SymbolRecord{Package: "bytes", Id: "bytes.Buffer", Classification: gen.Classification_SAFE})
			},
			wantErr: "duplicate symbol",
		},
		{
			name: "contradictory reclassification of an existing record",
			mutate: func(m *gen.StdlibMap) {
				m.Symbols[1] = &gen.SymbolRecord{Package: "bytes", Id: "bytes.Buffer", Classification: gen.Classification_UNANALYZED}
			},
			wantErr: "duplicate symbol",
		},
		{
			name:    "nil key",
			mutate:  func(m *gen.StdlibMap) { m.Key = nil },
			wantErr: "no SDK key",
		},
		{
			name:    "format version mismatch",
			mutate:  func(m *gen.StdlibMap) { m.FormatVersion = 2 },
			wantErr: "unsupported artifact format version",
		},
		{
			name: "incomplete target identity",
			mutate: func(m *gen.StdlibMap) {
				m.Key.ToolchainVersion = ""
				m.Key.Goos = ""
				m.Key.Goarch = ""
			},
			wantErr: "no concrete target",
		},
		{
			name:    "missing toolchain version",
			mutate:  func(m *gen.StdlibMap) { m.Key.ToolchainVersion = "" },
			wantErr: "no concrete target",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mapWithFullInventory()
			tc.mutate(m)
			_, err := NewStdlibMapReader(m, nil)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
				t.Fatalf("NewStdlibMapReader error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestStdlibMapReaderKeyMismatch pins AC4's fail-closed half: a required-key
// mismatch fails closed with ErrKeyMismatch listing the mismatched fields;
// a matching key succeeds.
func TestStdlibMapReaderKeyMismatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mut    func(*stdlibauthority.SDKKey)
		fields []string
	}{
		{"goarch", func(k *stdlibauthority.SDKKey) { k.GOARCH = "arm64" }, []string{"goarch"}},
		{"cgo", func(k *stdlibauthority.SDKKey) { k.CgoEnabled = true }, []string{"cgo_enabled"}},
		{"multiple fields", func(k *stdlibauthority.SDKKey) { k.GOARCH = "arm64"; k.CgoEnabled = true }, []string{"cgo_enabled", "goarch"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := expectedKey()
			tc.mut(&want)
			_, err := NewStdlibMapReader(mapWithFullInventory(), &want)
			if !errors.Is(err, stdlibauthority.ErrKeyMismatch) {
				t.Fatalf("err = %v, want ErrKeyMismatch", err)
			}
			var km *stdlibauthority.KeyMismatchError
			if !errors.As(err, &km) || strings.Join(km.Fields, ",") != strings.Join(tc.fields, ",") {
				t.Errorf("mismatch fields = %v, want %v", km, tc.fields)
			}
		})
	}
	t.Run("matching key succeeds", func(t *testing.T) {
		reader, err := NewStdlibMapReader(mapWithFullInventory(), expectedKeyPtr())
		if err != nil {
			t.Fatalf("NewStdlibMapReader with matching key: %v", err)
		}
		if got := reader.Key(); len(stdlibauthority.EqualKeys(got, expectedKey())) != 0 {
			t.Errorf("Key() = %v, want equal to expected", got)
		}
	})
	t.Run("OpenStdlibMap round trip", func(t *testing.T) {
		encoded, err := MarshalMap(mapWithFullInventory())
		if err != nil {
			t.Fatalf("MarshalMap: %v", err)
		}
		reader, err := OpenStdlibMap(strings.NewReader(string(encoded)), expectedKeyPtr())
		if err != nil {
			t.Fatalf("OpenStdlibMap: %v", err)
		}
		if !reader.IsStdlibPackage("os") {
			t.Error("opened reader lost package membership")
		}
	})
}

// mustSymErr returns the error from SymbolAuthority for one symbol text,
// after asserting the returned classification is the invalid zero value —
// never SAFE (AC2's fail-closed property).
func mustSymErr(t *testing.T, r stdlibauthority.StdlibAuthority, text string) error {
	t.Helper()
	got, err := r.SymbolAuthority(symbol.SymbolID(text))
	if err == nil && got.Validate() == nil {
		t.Errorf("SymbolAuthority(%s) returned terminal classification %+v alongside no error; a gap must never read as SAFE", text, got)
	}
	if got.Validate() == nil {
		t.Errorf("SymbolAuthority(%s) gap classification %+v must be invalid (never SAFE)", text, got)
	}
	return err
}
