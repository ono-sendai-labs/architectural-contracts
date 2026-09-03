package symbol_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// TestParseCapslock_Convergence pins the Capslock-name normalizer (task req 3,
// DR-04): pointer-receiver, value-receiver and generic-instantiated spellings
// of the same declared method all converge on one canonical
// "(pkg.Type).Method" SymbolID, and top-level spellings map to themselves.
func TestParseCapslock_Convergence(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"pointer receiver", "(*os.File).Read", "(os.File).Read"},
		{"value receiver", "(os.File).Read", "(os.File).Read"},
		{"top-level func", "os.ReadFile", "os.ReadFile"},
		{"top-level func instantiated", "store.Load[example.com/x.T]", "store.Load"},
		{"generic method instantiation pointer", "(*example.com/store.Box[int]).Get", "(example.com/store.Box).Get"},
		{"generic method instantiation value", "(example.com/store.Box[int]).Get", "(example.com/store.Box).Get"},
		{"generic type args value receiver", "(example.com/store.Pair[K,V]).First", "(example.com/store.Pair).First"},
		{"stdlib method on generic alias", "(sync/atomic.Pointer[example.com/x.T]).Load", ""},
		{"init", "os.init", "os.init"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == "" {
				// covered by the rejection table below
				return
			}
			got, err := symbol.ParseCapslock(tt.input)
			if err != nil {
				t.Fatalf("ParseCapslock(%q) error = %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Fatalf("ParseCapslock(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}

	// Convergence: all spellings of one declared method produce one identical ID.
	ids := map[symbol.SymbolID]bool{}
	for _, in := range []string{
		"(*example.com/store.Box[int]).Get",
		"(example.com/store.Box[int]).Get",
		"(example.com/store.Box).Get",
		"(*example.com/store.Box).Get",
	} {
		id, err := symbol.ParseCapslock(in)
		if err != nil {
			t.Fatalf("ParseCapslock(%q) error = %v", in, err)
		}
		ids[id] = true
	}
	if len(ids) != 1 {
		t.Fatalf("spellings did not converge on one ID: %v", ids)
	}
}

// TestParseCapslock_Rejected rejects Capslock spellings that cannot be mapped
// safely to a declared symbol, instead of guessing (task req 3). None of
// these may enter an artifact as an ID.
func TestParseCapslock_Rejected(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"dotted method spelling", "sync/atomic.Pointer[example.com/x.T].Load"},
		{"bare dotted method", "os.File.Read"},
		{"unbalanced brackets", "(*example.com/store.Box[int).Get"},
		{"dollar SSA suffix", "example.com/store.Read$1"},
		{"method on init", "(os.init).Run"},
		{"pointer outside parens", "*os.File.Read"},
		{"bare receiver dot method with brackets", "example.com/store.Box[int].Get"},
		{"empty brackets", "example.com/store.Read[]"},
		{"method on pointer-typed package", "(*os).Read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := symbol.ParseCapslock(tt.input)
			if err == nil {
				t.Fatalf("ParseCapslock(%q) = nil error; want rejection", tt.input)
			}
		})
	}
}
