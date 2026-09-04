package artifactio

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

// TestMapPreservesEvidenceFrameOrder pins F2's order half: Evidence.frames is an
// ordered caller-to-capability path (DR-17), so marshal/decode/marshal keeps the
// frame sequence exactly, and reversing the input frames changes both the
// canonical bytes and the digest.
func TestMapPreservesEvidenceFrameOrder(t *testing.T) {
	m := mapWithFullInventory()
	frameSeq := func(m *gen.StdlibMap) string {
		var parts []string
		for _, e := range m.Evidence {
			for _, f := range e.Frames {
				parts = append(parts, f.Function)
			}
		}
		return strings.Join(parts, ",")
	}
	want := frameSeq(m)

	encoded, err := MarshalMap(m)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	out, err := DecodeMap(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("DecodeMap: %v", err)
	}
	if got := frameSeq(out); got != want {
		t.Errorf("frame order after round trip = %q, want %q", got, want)
	}
	reencoded, err := MarshalMap(out)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(reencoded) != string(encoded) {
		t.Errorf("round trip bytes differ:\nwant %s\ngot  %s", encoded, reencoded)
	}

	reversed := mapWithFullInventory()
	ev := reversed.Evidence[0]
	for i, j := 0, len(ev.Frames)-1; i < j; i, j = i+1, j-1 {
		ev.Frames[i], ev.Frames[j] = ev.Frames[j], ev.Frames[i]
	}
	reversedBytes, err := MarshalMap(reversed)
	if err != nil {
		t.Fatalf("MarshalMap(reversed): %v", err)
	}
	if string(reversedBytes) == string(encoded) {
		t.Error("reversed evidence frames produced identical canonical bytes")
	}
	dPlain, err := MapDigest(mapWithFullInventory())
	if err != nil {
		t.Fatalf("MapDigest(plain): %v", err)
	}
	dReversed, err := MapDigest(reversed)
	if err != nil {
		t.Fatalf("MapDigest(reversed): %v", err)
	}
	if dPlain == dReversed {
		t.Error("reversed evidence frames produced an identical digest")
	}
}

// TestMapRequiresCompleteInitInventory pins F2's init-inventory half (I3): every
// importable package has exactly one terminal init record and non-importable
// packages have none.
func TestMapRequiresCompleteInitInventory(t *testing.T) {
	m := mapWithFullInventory()
	m.Inits = m.Inits[:1] // drop os's init; bytes keeps one
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "init") {
		t.Errorf("importable package without init: err = %v, want init-inventory error", err)
	}

	m = mapWithFullInventory()
	m.Inits = append(m.Inits, &gen.InitRecord{Package: "internal/secret", Classification: gen.Classification_SAFE})
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "init") {
		t.Errorf("non-importable package with init: err = %v, want init-inventory error", err)
	}

	raw := `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":1},"packages":[{"path":"os","importable":true}],"symbols":[{"package":"os","id":"os.Open","classification":"SAFE"}],"inits":[]}`
	if _, err := DecodeMap(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "init") {
		t.Errorf("DecodeMap without required init: err = %v, want init-inventory error", err)
	}
}

// TestMapRequiresCompleteEvidenceInventory pins F2's evidence-inventory half
// (DR-17): each capability of a CAPABILITIES symbol has exactly one evidence
// entry whose path is non-empty, and no evidence is unmatched.
func TestMapRequiresCompleteEvidenceInventory(t *testing.T) {
	m := mapWithFullInventory()
	m.Evidence = append(m.Evidence, &gen.Evidence{SymbolId: "(os.File).Read", Capability: "READ_SYSTEM_STATE"})
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Errorf("capability without evidence: err = %v, want evidence-inventory error", err)
	}

	m = mapWithFullInventory()
	m.Evidence[0].Frames = nil
	if _, err := MarshalMap(m); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("evidence with an empty path: err = %v, want empty-path error", err)
	}

	raw := `{"formatVersion":1,"key":{"toolchainVersion":"go1.26.4","goos":"linux","goarch":"amd64","mapFormatVersion":1},"packages":[{"path":"os","importable":true}],"symbols":[{"package":"os","id":"(os.File).Read","classification":"CAPABILITIES","capabilities":["FILES"]}],"inits":[{"package":"os","classification":"SAFE"}],"evidence":[]}`
	if _, err := DecodeMap(strings.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Errorf("DecodeMap without required evidence: err = %v, want evidence-inventory error", err)
	}
}
