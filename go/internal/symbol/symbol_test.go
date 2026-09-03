package symbol_test

import (
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// TestParseFormat_RoundTrip pins the v1 grammar (DR-04): every supported
// top-level, method and init form formats, parses and formats again to the
// identical bytes, with no pointer marker and no type arguments anywhere.
func TestParseFormat_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"package-level func", "example.com/store.Read"},
		{"package-level var", "example.com/store.DefaultLimit"},
		{"package-level const", "example.com/store.MaxSize"},
		{"named type", "example.com/store.Store"},
		{"alias type", "example.com/store.DB"},
		{"value-receiver method", "(example.com/store.Store).Get"},
		{"pointer-receiver method canonical form", "(example.com/store.Store).Put"},
		{"init", "example.com/store.init"},
		{"stdlib package", "os.ReadFile"},
		{"stdlib method", "(os.File).Read"},
		{"package path containing init element", "example.com/init.F"},
		{"package path containing dotted init part", "example.com/sub.init.F"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := symbol.Parse(tt.id)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.id, err)
			}
			if string(id) != tt.id {
				t.Fatalf("Parse(%q) = %q; want identical", tt.id, id)
			}
			formatted := id.Format()
			if formatted != tt.id {
				t.Fatalf("Format() = %q; want %q", formatted, tt.id)
			}
			if strings.ContainsAny(string(id), "*[]") {
				t.Fatalf("canonical ID %q contains pointer marker or type arguments", id)
			}
		})
	}
}

// TestSymbolID_EqualityAndOrder pins the comparison semantics that sorted
// artifacts (surfaces, maps, reports) rely on (N4).
func TestSymbolID_EqualityAndOrder(t *testing.T) {
	a, err := symbol.Parse("(example.com/store.Store).Get")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	b, err := symbol.Parse("(example.com/store.Store).Get")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if a != b {
		t.Fatalf("equal spellings compare unequal: %q vs %q", a, b)
	}
	if symbol.Compare(a, b) != 0 {
		t.Fatalf("Compare(equal) = %d; want 0", symbol.Compare(a, b))
	}
	c := symbol.SymbolID("example.com/store.Read")
	// Ordering is byte-wise: '(' (0x28) sorts before 'e' (0x65). The contract
	// is that Compare is the order sort.Slice needs, not any particular
	// human-friendly collation.
	if symbol.Compare(c, a) <= 0 {
		t.Fatalf("Compare(%q, %q) = %d; want positive", c, a, symbol.Compare(c, a))
	}
	if symbol.Compare(a, c) >= 0 {
		t.Fatalf("Compare(%q, %q) = %d; want negative", a, c, symbol.Compare(a, c))
	}
}

// TestParse_Malformed rejects malformed spellings and spellings outside the
// v1 grammar with an actionable error (DR-04: one versioned encoding; the
// design table persists bare keys, so pointers and type arguments never enter
// an artifact as an ID).
func TestParse_Malformed(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"bare identifier", "Read"},
		{"package path only", "example.com/store."},
		{"leading dot", ".Read"},
		{"empty name segment", "pkg..Read"},
		{"pointer marker", "(*example.com/store.Store).Get"},
		{"pointer marker without parens", "*example.com/store.Store.Get"},
		{"type arguments", "example.com/store.Box[int]"},
		{"type arguments in method receiver", "(example.com/store.Box[int]).Get"},
		{"generic alias arguments", "example.com/store.Set[string]"},
		{"method on init", "(example.com/store.init).Get"},
		{"missing closing paren", "(example.com/store.Store.Get"},
		{"empty receiver", "().Get"},
		{"method without dot", "(example.com/store.Store)Get"},
		{"version prefix", "v1:pkg.Read"},
		{"whitespace", "pkg. Read"},
		{"invalid identifier name", "pkg.1Read"},
		{"invalid receiver identifier", "(pkg.1Store).Get"},
		{"hyphen in name", "pkg.Re-ad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := symbol.Parse(tt.in)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error; want rejection", tt.in)
			}
			if strings.TrimSpace(err.Error()) == "" {
				t.Fatalf("Parse(%q) error must be actionable, got empty", tt.in)
			}
		})
	}
}

func TestGrammarVersion(t *testing.T) {
	if symbol.GrammarVersion != 1 {
		t.Fatalf("GrammarVersion = %d; want 1 (DR-04 pins one v1 grammar)", symbol.GrammarVersion)
	}
}
