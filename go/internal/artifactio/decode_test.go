package artifactio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
)

// TestDecodeIgnoresUnknownFields pins AC3's forward-compatibility half: an
// unknown JSON field is ignored, everything else decodes normally.
func TestDecodeIgnoresUnknownFields(t *testing.T) {
	encoded, err := MarshalSurface(validSurface())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decorated := strings.Replace(string(encoded),
		`"component": "widget",`,
		`"component": "widget", "zzFutureField": {"nested": true},`, 1)
	if decorated == string(encoded) {
		t.Fatal("test setup: anchor field not found in canonical bytes")
	}
	got, err := DecodeSurface(strings.NewReader(decorated))
	if err != nil {
		t.Fatalf("DecodeSurface with unknown field: %v", err)
	}
	if got.Component != "widget" {
		t.Errorf("decoded component = %q, want %q", got.Component, "widget")
	}
}

// TestDecodeRejectsVersionAndMalformed pins AC3's fail-closed half for
// unsupported major versions and malformed content.
func TestDecodeRejectsVersionAndMalformed(t *testing.T) {
	s := validSurface()
	s.FormatVersion = 2
	if _, err := DecodeSurface(strings.NewReader(`{"formatVersion": 2}`)); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("DecodeSurface(format_version=2) error = %v, want ErrUnsupportedVersion", err)
	}
	if _, err := DecodeSurface(strings.NewReader(`{not json`)); err == nil {
		t.Error("DecodeSurface accepted malformed JSON")
	}
	if _, err := DecodeMap(strings.NewReader(`{"formatVersion": 99}`)); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("DecodeMap(format_version=99) error = %v, want ErrUnsupportedVersion", err)
	}
	if _, err := DecodeMap(strings.NewReader(`[]`)); err == nil {
		t.Error("DecodeMap accepted malformed content")
	}
}

// TestDecodeRejectsSemanticErrors pins AC3's semantic-validation half via
// table-driven cases for both artifact kinds.
func TestDecodeRejectsSemanticErrors(t *testing.T) {
	cases := []struct {
		name string
		json string
		kind string // "surface" or "map"
		want string
	}{
		{
			name: "surface duplicate package",
			json: `{"formatVersion":1,"packages":["a","a"]}`,
			kind: "surface",
			want: "duplicate",
		},
		{
			name: "surface unknown authority with caps",
			json: `{"formatVersion":1,"authority":{"authority":"UNKNOWN","declaredAuthority":["FILES"]}}`,
			kind: "surface",
			want: "UNKNOWN",
		},
		{
			name: "surface invalid symbol ID",
			json: `{"formatVersion":1,"symbols":["not a symbol!"]}`,
			kind: "surface",
			want: "symbol",
		},
		{
			name: "surface unknown capability",
			json: `{"formatVersion":1,"authority":{"authority":"DECLARED","declaredAuthority":["NOT_A_CAPABILITY"]}}`,
			kind: "surface",
			want: "capability",
		},
		{
			name: "map unspecified classification",
			json: `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":1},"symbols":[{"package":"os","id":"os.Exit"}]}`,
			kind: "map",
			want: "CLASSIFICATION_UNSPECIFIED",
		},
		{
			name: "map capabilities with empty list",
			json: `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":1},"packages":[{"path":"os","importable":true}],"symbols":[{"package":"os","id":"(os.File).Read","classification":"CAPABILITIES"}]}`,
			kind: "map",
			want: "empty capability list",
		},
		{
			name: "map duplicate package",
			json: `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":1},"packages":[{"path":"os"},{"path":"os"}]}`,
			kind: "map",
			want: "duplicate",
		},
		{
			name: "map symbol outside inventory",
			json: `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":1},"packages":[{"path":"bytes"}],"symbols":[{"package":"os","id":"os.Exit","classification":"SAFE"}]}`,
			kind: "map",
			want: "outside the total package inventory",
		},
		{
			name: "map key format version mismatch",
			json: `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":2}}`,
			kind: "map",
			want: "map_format_version",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind == "surface" {
				_, err := DecodeSurface(strings.NewReader(tc.json))
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("DecodeSurface error = %v, want containing %q", err, tc.want)
				}
				return
			}
			_, err := DecodeMap(strings.NewReader(tc.json))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("DecodeMap error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// TestDecodeSizeCaps pins AC4: inputs at the cap decode (or fail on content,
// never on size), inputs above the cap fail with an actionable size error.
func TestDecodeSizeCaps(t *testing.T) {
	t.Run("surface", func(t *testing.T) {
		// A valid surface whose producer_version is padded so the canonical
		// encoding is exactly MaxSurfaceBytes, then MaxSurfaceBytes+1.
		s := validSurface()
		s.ProducerVersion = "p"
		encoded, err := MarshalSurface(s)
		if err != nil {
			t.Fatalf("marshal seed surface: %v", err)
		}
		if delta := int(MaxSurfaceBytes) - len(encoded); delta > 0 {
			s.ProducerVersion += strings.Repeat("p", delta)
			encoded, err = MarshalSurface(s)
			if err != nil {
				t.Fatalf("marshal padded surface: %v", err)
			}
		}
		if int64(len(encoded)) != MaxSurfaceBytes {
			t.Fatalf("test setup: padded surface is %d bytes, want exactly %d", len(encoded), MaxSurfaceBytes)
		}
		if _, err := DecodeSurface(bytes.NewReader(encoded)); err != nil {
			t.Fatalf("DecodeSurface at %d bytes (limit %d): %v", len(encoded), MaxSurfaceBytes, err)
		}

		s.ProducerVersion += "p"
		oversize, err := MarshalSurface(s)
		if err != nil {
			t.Fatalf("marshal oversize surface: %v", err)
		}
		if int64(len(oversize)) != MaxSurfaceBytes+1 {
			t.Fatalf("test setup: oversize surface is %d bytes, want exactly %d", len(oversize), MaxSurfaceBytes+1)
		}
		_, err = DecodeSurface(bytes.NewReader(oversize))
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Errorf("DecodeSurface(oversize) error = %v, want an actionable size error", err)
		}
	})

	t.Run("map", func(t *testing.T) {
		// Pad via the SDK key's classifier hash, keeping the fixture
		// semantically valid, to reach exactly the cap and cap+1.
		m := validMap()
		m.Key.ClassifierHash = "h"
		encoded, err := MarshalMap(m)
		if err != nil {
			t.Fatalf("marshal seed map: %v", err)
		}
		if delta := int(MaxMapBytes) - len(encoded); delta > 0 {
			m.Key.ClassifierHash += strings.Repeat("h", delta)
			encoded, err = MarshalMap(m)
			if err != nil {
				t.Fatalf("marshal padded map: %v", err)
			}
		}
		if int64(len(encoded)) != MaxMapBytes {
			t.Fatalf("test setup: padded map is %d bytes, want exactly %d", len(encoded), MaxMapBytes)
		}
		if _, err := DecodeMap(bytes.NewReader(encoded)); err != nil {
			t.Fatalf("DecodeMap at %d bytes (limit %d): %v", len(encoded), MaxMapBytes, err)
		}

		m.Key.ClassifierHash += "h"
		oversize, err := MarshalMap(m)
		if err != nil {
			t.Fatalf("marshal oversize map: %v", err)
		}
		_, err = DecodeMap(bytes.NewReader(oversize))
		if err == nil || !strings.Contains(err.Error(), "limit") {
			t.Errorf("DecodeMap(oversize) error = %v, want an actionable size error", err)
		}
	})
}

// TestDigestCanonicalEquivalence pins AC2: equivalent artifacts encoded with
// different source whitespace or entry order produce the same lowercase
// SHA-256 digest, and a semantic change changes it.
func TestDigestCanonicalEquivalence(t *testing.T) {
	a := validSurface()
	b := validSurface()
	b.Packages = []string{"b.example/pkg", "a.example/pkg"}
	b.Symbols = []string{"(a.example/pkg.T).M", "a.example/pkg.F"}

	da, err := SurfaceDigest(a)
	if err != nil {
		t.Fatalf("SurfaceDigest(a): %v", err)
	}
	db, err := SurfaceDigest(b)
	if err != nil {
		t.Fatalf("SurfaceDigest(b): %v", err)
	}
	if da != db {
		t.Errorf("digests differ for equivalent surfaces: %s vs %s", da, db)
	}
	if len(da) != 64 {
		t.Fatalf("digest %q is not a 64-char hex SHA-256", da)
	}
	if da != strings.ToLower(da) {
		t.Errorf("digest %q is not lowercase", da)
	}

	changed := validSurface()
	changed.Component = "gadget"
	dc, err := SurfaceDigest(changed)
	if err != nil {
		t.Fatalf("SurfaceDigest(changed): %v", err)
	}
	if dc == da {
		t.Error("a semantic field change produced the same digest")
	}
}

// TestDigestOnDecodedArtifact pins that hashing a decoded artifact operates on
// its re-canonicalized representation: raw non-canonical source bytes never
// influence the digest.
func TestDigestOnDecodedArtifact(t *testing.T) {
	raw := `{"formatVersion":1,"component":"widget",   "packages":["b.example/pkg","a.example/pkg"], "symbols":["(a.example/pkg.T).M","a.example/pkg.F"]}`
	decoded, err := DecodeSurface(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("decode non-canonical source: %v", err)
	}
	dDecoded, err := SurfaceDigest(decoded)
	if err != nil {
		t.Fatalf("digest decoded: %v", err)
	}
	dFixture, err := SurfaceDigest(validSurface())
	if err != nil {
		t.Fatalf("digest fixture: %v", err)
	}
	if dDecoded != dFixture {
		t.Errorf("decoded non-canonical source digest %s != canonical fixture digest %s", dDecoded, dFixture)
	}

	// Cross-check the helper's spelling against crypto/sha256 directly.
	canonical, err := MarshalSurface(validSurface())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sum := sha256.Sum256(canonical)
	if want := hex.EncodeToString(sum[:]); dFixture != want {
		t.Errorf("digest %s != direct sha256 %s", dFixture, want)
	}
}

// TestMapDigestEquivalence pins AC2 for maps.
func TestMapDigestEquivalence(t *testing.T) {
	a := validMap()
	b := validMap()
	b.Symbols = []*gen.SymbolRecord{
		{Package: "os", Id: "(os.File).Read", Classification: gen.Classification_CAPABILITIES, Capabilities: []string{"READ_SYSTEM_STATE", "FILES"}},
		{Package: "bytes", Id: "bytes.Buffer", Classification: gen.Classification_SAFE},
	}
	da, err := MapDigest(a)
	if err != nil {
		t.Fatalf("MapDigest(a): %v", err)
	}
	db, err := MapDigest(b)
	if err != nil {
		t.Fatalf("MapDigest(b): %v", err)
	}
	if da != db {
		t.Errorf("digests differ for equivalent maps: %s vs %s", da, db)
	}
}
