package artifactio

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

// validSurface returns a canonical-quality surface manifest fixture.
func validSurface() *gen.SurfaceManifest {
	return &gen.SurfaceManifest{
		FormatVersion: 1,
		Component:     "widget",
		Packages:      []string{"a.example/pkg", "b.example/pkg"},
		Symbols:       []string{"a.example/pkg.F", "(a.example/pkg.T).M"},
	}
}

// validMap returns a canonical-quality stdlib map fixture.
func validMap() *gen.StdlibMap {
	return &gen.StdlibMap{
		FormatVersion: 1,
		Key: &gen.SDKKey{
			ToolchainVersion: "go1.26.4",
			Goos:             "linux",
			Goarch:           "amd64",
			MapFormatVersion: 1,
		},
		Packages: []*gen.PackageInventory{
			{Path: "bytes", Importable: true},
			{Path: "internal/secret", Importable: false},
			{Path: "os", Importable: true},
		},
		Symbols: []*gen.SymbolRecord{
			{Package: "bytes", Id: "bytes.Buffer", Classification: gen.Classification_SAFE},
			{Package: "os", Id: "(os.File).Read", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"FILES", "READ_SYSTEM_STATE"}},
		},
		Inits: []*gen.InitRecord{
			{Package: "bytes", Classification: gen.Classification_SAFE},
			{Package: "os", Classification: gen.Classification_SAFE},
		},
		Evidence: []*gen.Evidence{
			{
				SymbolId:   "(os.File).Read",
				Capability: "FILES",
				Frames: []*gen.Frame{
					{Function: "(os.File).Read", File: "file_io.go", Line: 1},
					{Function: "os.Open", File: "file.go", Line: 2},
				},
			},
			{
				SymbolId:   "(os.File).Read",
				Capability: "READ_SYSTEM_STATE",
				Frames: []*gen.Frame{
					{Function: "(os.File).Read", File: "file_io.go", Line: 1},
					{Function: "os.Getenv", File: "env.go", Line: 3},
				},
			},
		},
	}
}

// TestMarshalSurfaceCanonicalBytes pins AC1 for surfaces: byte-identical output
// for reordered inputs, format_version first, stable across repeated marshals,
// two-space indent, and a trailing newline.
func TestMarshalSurfaceCanonicalBytes(t *testing.T) {
	a := validSurface()
	b := validSurface()
	// Reorder every repeated collection.
	b.Packages = []string{"b.example/pkg", "a.example/pkg"}
	b.Symbols = []string{"(a.example/pkg.T).M", "a.example/pkg.F"}

	gotA, err := MarshalSurface(a)
	if err != nil {
		t.Fatalf("MarshalSurface(a): %v", err)
	}
	gotB, err := MarshalSurface(b)
	if err != nil {
		t.Fatalf("MarshalSurface(b): %v", err)
	}

	if string(gotA) != string(gotB) {
		t.Errorf("reordered inputs produced different bytes:\nA: %s\nB: %s", gotA, gotB)
	}
	if !strings.HasPrefix(string(gotA), "{\n  \"formatVersion\": 1,") {
		t.Errorf("format_version is not the first field in:\n%s", gotA)
	}
	again, err := MarshalSurface(a)
	if err != nil {
		t.Fatalf("MarshalSurface again: %v", err)
	}
	if string(gotA) != string(again) {
		t.Error("repeated marshals of the same message are not byte-identical")
	}
	if !strings.HasSuffix(string(gotA), "\n}\n") || strings.HasSuffix(string(gotA), "\n\n}") {
		t.Errorf("expected two-space indent and exactly one trailing newline, got:\n%q", gotA)
	}
}

// TestMarshalSurfaceInputImmutable pins the defensive-copy contract: the
// caller's message keeps its original ordering after marshaling.
func TestMarshalSurfaceInputImmutable(t *testing.T) {
	a := validSurface()
	before := proto.Clone(a)
	if _, err := MarshalSurface(a); err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	if !proto.Equal(before, a) {
		t.Error("MarshalSurface mutated its input")
	}
}

// TestMarshalMapCanonicalBytes pins AC1 for stdlib maps: reordered repeated
// entries (including nested capabilities) canonicalize to identical bytes.
func TestMarshalMapCanonicalBytes(t *testing.T) {
	a := validMap()
	b := validMap()
	b.Packages = []*gen.PackageInventory{{Path: "internal/secret", Importable: false}, {Path: "bytes", Importable: true}, {Path: "os", Importable: true}}
	b.Symbols = []*gen.SymbolRecord{
		{Package: "os", Id: "(os.File).Read", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"READ_SYSTEM_STATE", "FILES"}},
		{Package: "bytes", Id: "bytes.Buffer", Classification: gen.Classification_SAFE},
	}

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
	if !strings.HasPrefix(string(gotA), "{\n  \"formatVersion\": 1,") {
		t.Errorf("format_version is not the first field in:\n%s", gotA)
	}
}

// TestMarshalRejectsDuplicateKeys pins req 2: duplicate keys and contradictory
// states are rejected, never silently deduplicated or reordered away.
func TestMarshalRejectsDuplicateKeys(t *testing.T) {
	s := validSurface()
	s.Packages = []string{"a.example/pkg", "a.example/pkg"}
	if _, err := MarshalSurface(s); err == nil {
		t.Error("MarshalSurface accepted a duplicate package entry")
	}

	m := validMap()
	m.Symbols = append(m.Symbols, &gen.SymbolRecord{
		Package: "os", Id: "(os.File).Read",
		Classification: gen.Classification_SAFE, // contradictory with the CAPABILITIES twin
	})
	if _, err := MarshalMap(m); err == nil {
		t.Error("MarshalMap accepted a contradictory duplicate symbol entry")
	}

	m2 := validMap()
	m2.Inits = append(m2.Inits, &gen.InitRecord{Package: "bytes", Classification: gen.Classification_UNANALYZED})
	if _, err := MarshalMap(m2); err == nil {
		t.Error("MarshalMap accepted a contradictory duplicate init entry")
	}
}

// TestMarshalRejectsNonCanonicalAuthorityAndClass pins req 2/req 4: UNKNOWN
// authority with declared capabilities, UNSPECIFIED classifications, and
// CAPABILITIES/empty-capability mismatches are rejected.
func TestMarshalRejectsNonCanonicalAuthorityAndClass(t *testing.T) {
	s := validSurface()
	s.Authority = &gen.AuthorityDeclaration{
		Authority:         gen.Authority_UNKNOWN,
		DeclaredAuthority: []string{"FILES"},
	}
	if _, err := MarshalSurface(s); err == nil {
		t.Error("MarshalSurface accepted UNKNOWN authority with declared capabilities")
	}

	m := validMap()
	m.Symbols[0].Classification = gen.Classification_CLASSIFICATION_UNSPECIFIED
	if _, err := MarshalMap(m); err == nil {
		t.Error("MarshalMap accepted a CLASSIFICATION_UNSPECIFIED symbol")
	}

	m = validMap()
	m.Symbols[1].Capabilities = nil
	if _, err := MarshalMap(m); err == nil {
		t.Error("MarshalMap accepted CAPABILITIES with an empty capability list")
	}

	m = validMap()
	m.Symbols[0].Capabilities = []string{"FILES"}
	if _, err := MarshalMap(m); err == nil {
		t.Error("MarshalMap accepted SAFE with capabilities")
	}
}

// TestMarshalRoundTrip pins that decode(marshal(x)) == x round trips.
func TestMarshalRoundTrip(t *testing.T) {
	t.Run("surface", func(t *testing.T) {
		in := validSurface()
		encoded, err := MarshalSurface(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out, err := DecodeSurface(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		// The decoder returns the message as written; canonical bytes must
		// reproduce byte-for-byte after re-marshal.
		reencoded, err := MarshalSurface(out)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		if string(encoded) != string(reencoded) {
			t.Errorf("round trip bytes differ:\nwant %s\ngot  %s", encoded, reencoded)
		}
	})
	t.Run("map", func(t *testing.T) {
		in := validMap()
		encoded, err := MarshalMap(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out, err := DecodeMap(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		reencoded, err := MarshalMap(out)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		if string(encoded) != string(reencoded) {
			t.Errorf("round trip bytes differ:\nwant %s\ngot  %s", encoded, reencoded)
		}
	})
}
