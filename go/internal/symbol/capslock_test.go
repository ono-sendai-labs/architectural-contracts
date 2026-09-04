package symbol_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
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
		{"variadic function type argument", "example.com/store.Load[func(...int) int]", "example.com/store.Load"},
		{"multi-result function type argument", "example.com/store.Load[func() (int, string)]", "example.com/store.Load"},
		{"parenthesized type argument", "example.com/store.Load[(*example.com/x.T)]", "example.com/store.Load"},
		{"array-division length type argument", "example.com/store.Load[[N/2]byte]", "example.com/store.Load"},
		{"nested generic type argument", "example.com/store.Load[example.com/x.Pair[int]]", "example.com/store.Load"},
		{"channel type argument", "example.com/store.Load[chan int]", "example.com/store.Load"},
		{"function type argument", "example.com/store.Load[func(int) string]", "example.com/store.Load"},
		{"array-constant type argument", "example.com/store.Load[[N+1]byte]", "example.com/store.Load"},
		{"hyphenated package type argument", "example.com/store.Load[example.com/x-y.T]", "example.com/store.Load"},
		{"named function parameter type argument", "example.com/store.Load[func(x int) string]", "example.com/store.Load"},
		{"named grouped function parameter type argument", "example.com/store.Load[func(a, b int) string]", "example.com/store.Load"},
		{"shift constant-expression length type argument", "example.com/store.Load[[N<<1]byte]", "example.com/store.Load"},
		{"struct literal type argument", "example.com/store.Load[struct{A int; B string}]", "example.com/store.Load"},
		{"interface literal type argument", "example.com/store.Load[interface{M(); N(x int) string}]", "example.com/store.Load"},
		{"pointer receiver with map type argument", "(*example.com/store.Box[map[string]int]).Get", "(example.com/store.Box).Get"},
		{"digit-leading path element type argument", "store.Load[example.com/2x.T]", "store.Load"},
		{"qualified unnamed parameter type argument", "store.Load[func(example.com/x.T)]", "store.Load"},
		{"named variadic parameter type argument", "store.Load[func(x ...int)]", "store.Load"},
		{"qualified embedded struct field type argument", "store.Load[struct{example.com/x.T}]", "store.Load"},
		{"bare embedded struct type argument", "store.Load[struct{Local}]", "store.Load"},
		{"bare embedded struct field after a named field", "store.Load[struct{A int; Local}]", "store.Load"},
		{"qualified pointer embedded struct field type argument", "store.Load[struct{*example.com/x.T}]", "store.Load"},
		{"tagged struct field type argument", `store.Load[struct{A int "json:\"a\""}]`, "store.Load"},
		{"tagged struct field with closing brace in tag", `store.Load[struct{A int "json:\"}\""}]`, "store.Load"},
		{"tagged struct field with opening bracket in tag", `store.Load[struct{A int "json:\"[\""}]`, "store.Load"},
		{"tagged struct field with comma in tag", `store.Load[struct{A int "a,b"}]`, "store.Load"},
		{"qualified interface method parameter type argument", "store.Load[interface{M(example.com/x.T) string}]", "store.Load"},
		{"and-not constant-expression length type argument", "store.Load[[N &^ 3]byte]", "store.Load"},
		{"parenthesized single type argument", "store.Load[(int)]", "store.Load"},
		{"stdlib method on generic alias", "(sync/atomic.Pointer[example.com/x.T]).Load", ""},
		{"init", "os.init", "os.init"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want == "" {
				// covered by the rejection table below
				return
			}
			// Top-level spellings with a dotful package path are ambiguous
			// without package knowledge (round-3 finding 1); the table's
			// resolver confirms their packages.
			got, err := symbol.ParseCapslockWithPackages(tt.input, func(pkg string) bool {
				return pkg == "example.com/store" || pkg == "store"
			})
			if err != nil {
				t.Fatalf("ParseCapslockWithPackages(%q) error = %v", tt.input, err)
			}
			if string(got) != tt.want {
				t.Fatalf("ParseCapslockWithPackages(%q) = %q; want %q", tt.input, got, tt.want)
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
		{"dotful package path without host slash", "example.com.private.Read"},
		{"dotted path without package knowledge", "example.com/p.private.Read"},
		{"dotted module path without package knowledge", "gopkg.in/yaml.v2.Unmarshal"},
		{"invalid type-argument text", "(*example.com/store.Box[not a type]).Get"},
		{"invalid type-argument colon", "(example.com/store.Map[string]int]).Get"},
		{"punctuation-only type argument", "example.com/store.Load[,,]"},
		{"dot-only type argument", "example.com/store.Load[.]"},
		{"elementless channel type argument", "example.com/store.Load[chan]"},
		{"hyphenated bare type name", "store.Load[foo-bar]"},
		{"variadic parameter not last", "store.Load[func(...int, string)]"},
		{"repeated trailing bracket groups", "store.Load[example.com/x.T][int]"},
		{"bracketed receiver type glued to name", "(example.com/store.A[example.com/x.T]B).Get"},
		{"tuple type argument", "store.Load[(int, string)]"},
		{"named-list type argument", "store.Load[(x int)]"},
		{"variadic parameter not final in func type argument", "store.Load[func(x ...int, string)]"},
		{"package path without type name", "store.Load[example.com/x]"},
		{"bare digit-leading identifier", "store.Load[2x.T]"},
		{"trailing statement after type", "store.Load[int; var y int]"},
		{"two top-level types", "store.Load[int string]"},
		{"variadic result", "store.Load[func() (...int)]"},
		{"grouped variadic parameters", "store.Load[func(x, y ...int)]"},
		{"trailing tokens after newline", "store.Load[int\nvar y int]"},
		{"invalid string escape in tag", `store.Load[struct{A int "json:\q"}]`},
		{"malformed numeric literal in array length", `store.Load[[1_]byte]`},
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

// TestParseCapslockWithPackages pins review-round-3 finding 1: an
// unparenthesized spelling whose package path contains a dot is ambiguous
// (it could be a dotted method spelling, which Capslock never prints, or a
// top-level symbol in a dotful package), so plain ParseCapslock rejects it
// and ParseCapslockWithPackages accepts it only when the caller's
// known-package resolver confirms the top-level package.
func TestParseCapslockWithPackages(t *testing.T) {
	if _, err := symbol.ParseCapslockWithPackages("gopkg.in/yaml.v2.Unmarshal", nil); err == nil {
		t.Fatalf("ParseCapslockWithPackages with a nil resolver = nil error; want rejection")
	}

	known := map[string]bool{"gopkg.in/yaml.v2": true, "example.com/p.private": true}
	resolver := func(pkg string) bool { return known[pkg] }

	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"dotted versioned module path", "gopkg.in/yaml.v2.Unmarshal", "gopkg.in/yaml.v2.Unmarshal"},
		{"dotted subdirectory package", "example.com/p.private.Read", "example.com/p.private.Read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := symbol.ParseCapslockWithPackages(tc.input, resolver)
			if err != nil {
				t.Fatalf("ParseCapslockWithPackages(%q) error = %v", tc.input, err)
			}
			if string(got) != tc.want {
				t.Fatalf("ParseCapslockWithPackages(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}

	// Without the package being known, the ambiguous spellings stay rejected.
	for _, in := range []string{"gopkg.in/yaml.v2.Unmarshal", "example.com/p.private.Read"} {
		if _, err := symbol.ParseCapslockWithPackages(in, func(string) bool { return false }); err == nil {
			t.Fatalf("ParseCapslockWithPackages(%q, unknown-package resolver) = nil error; want rejection", in)
		}
	}
}

// TestParseCapslock_CanonicalizesNamespace pins review-round-1 finding 1:
// Capslock spellings are canonicalized through the same host-policy hook as
// the go/types conversion, so the two producers produce identical IDs in a
// host-rewritten namespace.
func TestParseCapslock_CanonicalizesNamespace(t *testing.T) {
	orig := hostpolicy.CanonicalizePath
	defer func() { hostpolicy.CanonicalizePath = orig }()
	hostpolicy.CanonicalizePath = func(p string) string {
		return "canonical.example/" + p
	}

	// A small typed fixture: the same declaration converted from go/types.
	src := `package raw

type T struct{}

func (t T) M() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "raw.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{}
	if _, err := conf.Check("raw", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("check: %v", err)
	}
	var method types.Object
	for _, obj := range info.Defs {
		if obj != nil && obj.Name() == "M" {
			method = obj
		}
	}
	if method == nil {
		t.Fatalf("fixture method M not found")
	}
	fromTypes, err := symbol.FromObject(method)
	if err != nil {
		t.Fatalf("FromObject(method) error = %v", err)
	}

	for _, tc := range []struct {
		name  string
		input string
		want  symbol.SymbolID
	}{
		{"method, pointer spelling", "(*raw.T).M", fromTypes},
		{"method, value spelling", "(raw.T).M", fromTypes},
		{"top-level", "raw.T", "canonical.example/raw.T"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := symbol.ParseCapslock(tc.input)
			if err != nil {
				t.Fatalf("ParseCapslock(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseCapslock(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}
