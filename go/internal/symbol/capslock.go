package symbol

import (
	"fmt"
	"go/scanner"
	"go/token"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// ParseCapslock normalizes a Capslock classifier name into the canonical v1
// SymbolID of the symbol it declares (DR-04, task req 3). It accepts the
// spellings Capslock/SSA produce for declared symbols — top-level names,
// pointer- and value-receiver method names, and generic instantiations with
// their type-argument brackets — and maps them all to the one canonical ID of
// the declaring symbol:
//
//	(*os.File).Read              -> (os.File).Read
//	(os.File).Read               -> (os.File).Read
//	(*store.Box[int]).Get        -> (store.Box).Get
//	store.Load[example.com/x.T]  -> store.Load
//
// It is built on Parse, so there is exactly one canonicalization rule: the
// package path of the result is canonicalized through the host-policy hook,
// exactly as the go/types conversion does, so both producers produce
// identical IDs in any host namespace. Type arguments are validated against
// Go token syntax with a scanner-based single-type grammar (go/scanner; the
// parser package is unusable here because its entry points statically reach
// os.ReadFile, which the component's guaranteed-pure self-check rejects)
// after a documented translation of full import paths through the canonical
// package grammar (see validateTypeArguments), so acceptance tracks actual
// formatter output over the supported subset.
// Names that cannot be mapped safely to a declared symbol — unbalanced,
// misplaced, empty or non-type bracket contents, dotted method spellings,
// compiler-synthesized names — are rejected with an error instead of
// guessed, so classifier text can never enter an artifact as an ID.
//
// Unparenthesized spellings whose package path contains a dot (such as
// "gopkg.in/yaml.v2.Unmarshal") are ambiguous without outside knowledge: a
// dot could equally be a dotted method separator, and Capslock's text carries
// no object-kind information. ParseCapslock rejects them; callers that know
// the package inventory use ParseCapslockWithPackages to disambiguate.
func ParseCapslock(name string) (SymbolID, error) {
	return parseCapslock(name, nil, nil, nil)
}

// CapslockFunction mirrors the structured identity Capslock keeps for a
// function — the Name display string and the Package import path of
// proto.Function — so callers can supply the fields the text alone cannot
// carry instead of recovering all identity from one ambiguous display string.
type CapslockFunction struct {
	// Name is the Capslock display spelling (proto.Function.Name).
	Name string
	// Package is the structured package field (proto.Function.Package):
	// the import path of the package declaring the function. Empty when
	// unknown; it participates in known-package confirmation but the
	// inventory remains the declaration authority.
	Package string
}

// CapslockInventory is the independently inventoried declaration and package
// context (the typed declaration inventory used by the map/surface
// producers) against which structured Capslock normalization is confirmed.
type CapslockInventory interface {
	// KnownPackage reports whether importPath names a package in the
	// caller's world.
	KnownPackage(importPath string) bool
	// Declares reports whether pkg declares the top-level symbol or
	// receiver type name. Normalization fails closed unless the inventory
	// confirms exactly the one canonical declaration the name maps to.
	Declares(pkg, name string) bool
	// DeclaresMethod reports whether the inventory contains exactly the
	// canonical method declaration (pkg.TypeName).Method. The receiver type
	// alone is not sufficient: an inventoried receiver does not vouch for
	// arbitrary methods spelled against it.
	DeclaresMethod(pkg, typeName, method string) bool
}

// ParseCapslockFunction normalizes a Capslock function given its structured
// identity and the caller's declaration inventory. It is the inventory-backed
// form of ParseCapslock (task req 1): the structured package field and the
// inventory together confirm the one canonical declaration the display
// spelling maps to, and normalization fails closed — with an actionable
// error and no SymbolID — whenever either is missing, malformed or does not
// confirm the declaration. A nil inventory is always rejected.
func ParseCapslockFunction(fn CapslockFunction, inv CapslockInventory) (SymbolID, error) {
	if inv == nil {
		return "", fmt.Errorf("capslock name %q: normalization is inventory-backed; supply the declaration inventory", fn.Name)
	}
	if fn.Package != "" && !validPackagePath(fn.Package) {
		return "", fmt.Errorf("capslock name %q: structured package %q is not a valid import path", fn.Name, fn.Package)
	}
	knownPackage := func(pkg string) bool {
		return (fn.Package != "" && pkg == fn.Package) || inv.KnownPackage(hostpolicy.CanonicalizePath(pkg))
	}
	// resolve maps the package prefix recovered from the display text to its
	// canonical import path using the structured field: the text prefix is
	// either the full path or a trailing shorthand of it ("store" for
	// "example.com/store"); anything else contradicts the structured field
	// and fails closed.
	resolve := func(pkg string) (string, bool) {
		if fn.Package == "" || pkg == fn.Package {
			return pkg, true
		}
		if strings.HasSuffix(fn.Package, "/"+pkg) {
			return fn.Package, true
		}
		return "", false
	}
	return parseCapslock(fn.Name, knownPackage, inv, resolve)
}

// ParseCapslockWithPackages is ParseCapslock with one piece of structured
// context: knownPackage reports whether an import path names a package in the
// caller's world (typed declarations, a manifest, a module inventory). An
// unparenthesized spelling whose package path contains a dot is accepted as a
// top-level symbol only when knownPackage confirms that package; otherwise
// the spelling could be an unprinted dotted method name and is rejected
// instead of guessed. Dotless package paths ("os.ReadFile") are unambiguous
// and need no resolver.
func ParseCapslockWithPackages(name string, knownPackage func(importPath string) bool) (SymbolID, error) {
	if knownPackage == nil {
		return "", fmt.Errorf("capslock name %q: disambiguating a dotful unparenthesized spelling requires a known-package resolver", name)
	}
	return parseCapslock(name, knownPackage, nil, nil)
}

func parseCapslock(name string, knownPackage func(string) bool, inv CapslockInventory, resolve func(string) (string, bool)) (SymbolID, error) {
	if recv, method, ok := splitMethod(name); ok {
		recv, err := stripReceiverBrackets(recv)
		if err != nil {
			return "", fmt.Errorf("capslock name %q: %w", name, err)
		}
		// SSA prints pointer receivers as "*pkg.Type"; the v1 grammar stores
		// the receiver base name only.
		recv = strings.TrimPrefix(recv, "*")
		if strings.ContainsAny(recv, "*[]()") {
			return "", fmt.Errorf("capslock name %q: receiver %q is not a declared type spelling", name, recv)
		}
		if strings.ContainsAny(method, "*[]()") {
			return "", fmt.Errorf("capslock name %q: method name %q contains brackets or a pointer marker", name, method)
		}
		dot := strings.LastIndexByte(recv, '.')
		if dot < 0 {
			return "", fmt.Errorf("capslock name %q: receiver %q is not \"pkg.Type\"", name, recv)
		}
		pkg := recv[:dot]
		if resolve != nil {
			canonical, ok := resolve(pkg)
			if !ok {
				return "", fmt.Errorf("capslock name %q: structured package field does not confirm receiver package %q", name, pkg)
			}
			pkg = canonical
		}
		// One canonicalization: confirm and emit the canonical package.
		canon := hostpolicy.CanonicalizePath(pkg)
		if err := confirmDeclaration(inv, name, canon, recv[dot+1:]); err != nil {
			return "", err
		}
		if inv != nil && !inv.DeclaresMethod(canon, recv[dot+1:], method) {
			return "", fmt.Errorf("capslock name %q: inventory does not confirm a declaration of %q in %q", name, "("+recv[dot+1:]+")."+method, canon)
		}
		return Parse("(" + canon + "." + recv[dot+1:] + ")." + method)
	}
	// Top-level spelling: type-argument brackets are allowed only as a
	// trailing group (store.Load[example.com/x.T]); anywhere else they would
	// be part of a dotted method spelling, which Capslock does not produce
	// for declared symbols and which cannot be mapped safely.
	rest, err := stripTrailingBrackets(name)
	if err != nil {
		return "", fmt.Errorf("capslock name %q: %w", name, err)
	}
	dot := strings.LastIndexByte(rest, '.')
	if dot < 0 {
		return "", fmt.Errorf("capslock name %q: %q is not a Capslock spelling of a declared top-level symbol", name, rest)
	}
	pkg := rest[:dot]
	structured := false
	if resolve != nil {
		canonical, ok := resolve(pkg)
		if !ok {
			return "", fmt.Errorf("capslock name %q: structured package field does not confirm package %q", name, pkg)
		}
		if canonical != pkg {
			structured = true
		}
		pkg = canonical
	}
	if strings.Contains(pkg, ".") && !structured {
		// A dot in the package path makes the spelling ambiguous: the same
		// text could be a dotted method spelling ("pkg.Type.Method"), which
		// Capslock never prints unparenthesized, or a top-level symbol in a
		// dotful package. Without a known-package confirmation there is no
		// safe mapping.
		if knownPackage == nil {
			return "", fmt.Errorf("capslock name %q: %q is ambiguous — a dotful unparenthesized spelling could equally be a dotted method spelling; confirm the package with ParseCapslockWithPackages", name, rest)
		}
		if !knownPackage(pkg) {
			return "", fmt.Errorf("capslock name %q: package %q is not known, so %q cannot be mapped safely to a top-level symbol", name, pkg, rest)
		}
	}
	canon := hostpolicy.CanonicalizePath(pkg)
	if err := confirmDeclaration(inv, name, canon, rest[dot+1:]); err != nil {
		return "", err
	}
	return Parse(canon + "." + rest[dot+1:])
}

// confirmDeclaration enforces the inventory-backed contract: when an
// inventory is supplied, it must confirm exactly the one canonical
// declaration the spelling maps to (top-level name or receiver type) in the
// canonical namespace; text-only callers pass a nil inventory.
func confirmDeclaration(inv CapslockInventory, name, pkg, decl string) error {
	if inv == nil {
		return nil
	}
	if !inv.Declares(pkg, decl) {
		return fmt.Errorf("capslock name %q: inventory does not confirm a declaration of %q in %q", name, decl, pkg)
	}
	return nil
}

// stripReceiverBrackets removes the single trailing type-argument bracket
// group from a "(pkg.Type[args])" receiver spelling. Any other bracket —
// misplaced or a second group — is rejected, so a glued type name like
// "pkg.A[T]B" cannot masquerade as a declared receiver.
func stripReceiverBrackets(recv string) (string, error) {
	rest, err := stripTrailingBrackets(recv)
	if err != nil {
		return "", fmt.Errorf("receiver %q: %w", recv, err)
	}
	return rest, nil
}

// stripTrailingBrackets removes one trailing balanced bracket group from s
// and validates the group's contents as Capslock type arguments. It fails
// when s contains brackets that are not exactly one trailing balanced group,
// or when the group's contents are not valid type arguments.
func stripTrailingBrackets(s string) (string, error) {
	if !strings.ContainsAny(s, "[]") {
		return s, nil
	}
	if !strings.HasSuffix(s, "]") {
		return "", fmt.Errorf("misplaced or unbalanced type-argument brackets")
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ']':
			depth++
		case '[':
			depth--
			if depth == 0 {
				if err := validateTypeArguments(s[i+1 : len(s)-1]); err != nil {
					return "", err
				}
				prefix := s[:i]
				// The prefix must be bracket-free in its raw text: a second
				// top-level group ("Load[T][U]") or a misplaced bracket is
				// not a Capslock spelling.
				if strings.ContainsAny(prefix, "[]") {
					return "", fmt.Errorf("misplaced or repeated type-argument brackets")
				}
				return prefix, nil
			}
		}
	}
	return "", fmt.Errorf("unbalanced type-argument brackets")
}

// validateTypeArguments reports whether inner — the text between a Capslock
// type-argument bracket group — is a non-empty, comma-separated list in which
// every element is exactly one Go type spelled the way the Capslock/SSA
// formatter prints them (task req 3/4/5).
//
// The check is context-aware in two steps instead of a handwritten Go-type
// grammar:
//
//  1. translateImportPaths rewrites every package-qualified name's full
//     import path ("example.com/2x.T") to a placeholder identifier,
//     validating the path against the canonical SymbolID package grammar
//     (validPackagePath — the same authority SymbolID.Parse applies), so
//     there is no second, stricter path grammar.
//  2. checkSingleType validates each translated element as exactly one type
//     with a recursive-descent grammar over real Go tokens (go/scanner). A
//     parenthesized group in a type position must contain a single type, so
//     tuples ("(int, string)") and named lists ("(x int)") that are only
//     legal in function signatures are rejected, while real formatter
//     output — qualified unnamed and named variadic parameters, qualified embedded struct fields, interface-method
//     parameters, and constant-expression array lengths including &^ — is
//     accepted by Go's own grammar.
func validateTypeArguments(inner string) error {
	args, err := splitTypeArgumentList(inner)
	if err != nil {
		return err
	}
	for i, arg := range args {
		translated, err := translateImportPaths(arg)
		if err != nil {
			return fmt.Errorf("type argument %d (%q): %w", i+1, arg, err)
		}
		if err := checkSingleType(translated); err != nil {
			return fmt.Errorf("type argument %d (%q): %w", i+1, arg, err)
		}
	}
	return nil
}

// splitTypeArgumentList splits inner at top-level commas (commas not nested
// in (), [] or {}) and rejects empty elements, so ",,", a trailing comma or
// an empty bracket group fails with an actionable error.
func splitTypeArgumentList(inner string) ([]string, error) {
	depth := 0
	var args []string
	start := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced %q", string(inner[i]))
			}
		case ',':
			if depth == 0 {
				args = append(args, inner[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced brackets")
	}
	args = append(args, inner[start:])
	for i, arg := range args {
		if strings.TrimSpace(arg) == "" {
			return nil, fmt.Errorf("type argument %d is empty", i+1)
		}
	}
	return args, nil
}

// translateImportPaths rewrites every package-qualified name's full import
// path to a placeholder identifier ("example.com/2x.T" -> "_capslockP0.T").
// A name-ish token (identifiers, digits, dots, slashes and hyphens) is
// translated only when its last dot splits it into a package path accepted
// by the canonical SymbolID grammar (validPackagePath) and a plain Go
// identifier (the qualified type name); anything else is left untouched, so
// array-length arithmetic ("N/2", "N-1") stays an expression and malformed
// tokens fail in the standard parse instead. The placeholder names never
// occur in formatter output.
func translateImportPaths(s string) (string, error) {
	var b strings.Builder
	i := 0
	n := 0
	for i < len(s) {
		if !isIdentStart(s[i]) {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i
		for j < len(s) && (isIdentStart(s[j]) || s[j] >= '0' && s[j] <= '9' || s[j] == '.' || s[j] == '/' || s[j] == '-') {
			j++
		}
		tok := s[i:j]
		if path, ok := qualifiedNameParts(tok); ok {
			b.WriteString(pathPlaceholder(n))
			b.WriteString(".")
			b.WriteString(path.name)
			n++
		} else {
			b.WriteString(tok)
		}
		i = j
	}
	return b.String(), nil
}

// qualifiedNameParts splits a name-ish token into its import path and
// qualified type name when the token is a package-qualified type spelling.
func qualifiedNameParts(tok string) (qualifiedName, bool) {
	dot := strings.LastIndexByte(tok, '.')
	if dot <= 0 || dot == len(tok)-1 {
		return qualifiedName{}, false
	}
	path, name := tok[:dot], tok[dot+1:]
	if strings.ContainsAny(name, "./-") || !validGoIdent(name) {
		return qualifiedName{}, false
	}
	// One authority: the canonical SymbolID package grammar, exactly the
	// check Parse applies to the persisted path (digit-leading elements such
	// as "example.com/2x" are valid).
	if !validPackagePath(path) {
		return qualifiedName{}, false
	}
	return qualifiedName{path: path, name: name}, true
}

type qualifiedName struct {
	path string
	name string
}

func pathPlaceholder(n int) string { return fmt.Sprintf("_capslockP%d", n) }

// isIdentStart reports whether c can start a Go identifier (Unicode letters
// are matched byte-wise for ASCII; the multi-byte range is treated as a
// start, as the formatter never emits one inside a type argument keyword).
func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// validGoIdent reports whether s is a plain Go identifier.
func validGoIdent(s string) bool {
	if s == "" || !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentStart(s[i]) && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	return true
}

// checkSingleType validates that arg is exactly one Go type, using a
// recursive-descent grammar over real Go tokens (go/scanner). Tokenizing
// with the standard scanner (rather than a character-level grammar) keeps
// the type check grounded in Go's own lexical rules — "&^" is one AND_NOT
// operator, "..." one variadic marker, "2x.T" two tokens — while
// package-qualified names arrive pre-translated by translateImportPaths.
//
// go/parser itself cannot be used here: every parser entry point statically
// reaches os.ReadFile through readSource, which the guaranteed-pure symbol
// component's self-check (correctly) rejects as undeclared ambient FILES
// authority, even though the probe never passes a filename. The grammar
// below covers exactly the single-type-position semantics the probe needs:
// in a type position a parenthesized group must contain a single type, so a
// tuple ("(int, string)") or a named list ("(x int)") — both legal only in
// function signatures — is rejected with an actionable error, and the whole
// argument must be consumed, so trailing tokens or statements cannot hide
// behind an accepted prefix.
func checkSingleType(arg string) error {
	var p typeParser
	if err := p.init(strings.TrimSpace(arg)); err != nil {
		return err
	}
	if err := p.typ(); err != nil {
		return err
	}
	if !p.atEnd() {
		return p.errorf("unexpected %s after type", p.tok)
	}
	return nil
}

// typeParser is a recursive-descent parser over go/scanner tokens that
// validates exactly one Go type. Only syntax is checked; package-qualified
// names have already been translated and their paths validated.
type typeParser struct {
	src  string
	fset *token.FileSet
	file *token.File
	scan scanner.Scanner
	pos  token.Pos
	tok  token.Token
	lit  string
}

func (p *typeParser) init(s string) error {
	if s == "" {
		return fmt.Errorf("empty type argument")
	}
	p.src = s
	p.fset = token.NewFileSet()
	p.file = p.fset.AddFile("", p.fset.Base(), len(s))
	p.scan.Init(p.file, []byte(s), nil, 0)
	p.next()
	return nil
}

func (p *typeParser) next() {
	p.pos, p.tok, p.lit = p.scan.Scan()
}

// atEnd reports whether the current token terminates the input. The scanner
// inserts an automatic semicolon (lit "\n") at end of input after a token
// that can end a line, but it also inserts one at every real newline, so the
// position must be confirmed to be the end of the argument before an
// inserted semicolon is treated as end-of-input; anywhere else it is a
// trailing token and rejected.
func (p *typeParser) atEnd() bool {
	if p.tok == token.EOF {
		return true
	}
	return p.tok == token.SEMICOLON && p.lit == "\n" && p.file.Offset(p.pos) >= len(p.src)
}

func (p *typeParser) errorf(format string, args ...any) error {
	return fmt.Errorf("%s: %s", p.fset.Position(p.pos), fmt.Sprintf(format, args...))
}

func (p *typeParser) expect(tok token.Token, what string) error {
	if p.tok != tok {
		return p.errorf("expected %q (%s), found %q", tok, what, p.lit)
	}
	p.next()
	return nil
}

// peekIsQualified reports whether the IDENT at the current position is
// followed by a selector dot, making it the start of a qualified type
// ("pkg.T") rather than another name in an identifier run. go/scanner has no
// pushback, so the lookahead re-scans the remaining input on a throwaway
// scanner (a pure, in-memory operation).
func (p *typeParser) peekIsQualified() bool {
	off := p.file.Offset(p.pos) + len(p.lit)
	fset := token.NewFileSet()
	file := fset.AddFile("", 1, len(p.src)-off)
	var s scanner.Scanner
	s.Init(file, []byte(p.src[off:]), nil, 0)
	_, t, _ := s.Scan()
	return t == token.PERIOD
}

// startsType reports whether the current token can begin a type.
func (p *typeParser) startsType() bool {
	switch p.tok {
	case token.MUL, token.LBRACK, token.LPAREN, token.CHAN, token.FUNC,
		token.MAP, token.STRUCT, token.INTERFACE, token.ARROW, token.IDENT:
		return true
	}
	return false
}

// typ parses one type.
func (p *typeParser) typ() error {
	switch p.tok {
	case token.MUL:
		p.next()
		return p.typ()
	case token.LBRACK:
		p.next()
		if p.tok == token.RBRACK {
			p.next()
			return p.typ()
		}
		if err := p.expr(); err != nil {
			return err
		}
		if err := p.expect(token.RBRACK, "to close the array length"); err != nil {
			return err
		}
		return p.typ()
	case token.MAP:
		p.next()
		if err := p.expect(token.LBRACK, "after map"); err != nil {
			return err
		}
		if err := p.typ(); err != nil {
			return err
		}
		if err := p.expect(token.RBRACK, "to close the map key type"); err != nil {
			return err
		}
		return p.typ()
	case token.CHAN:
		p.next()
		if p.tok == token.ARROW {
			p.next()
		}
		return p.typ()
	case token.ARROW:
		p.next()
		if err := p.expect(token.CHAN, "after receive direction"); err != nil {
			return err
		}
		return p.typ()
	case token.FUNC:
		p.next()
		return p.signature()
	case token.STRUCT:
		p.next()
		return p.structBody()
	case token.INTERFACE:
		p.next()
		return p.ifaceBody()
	case token.LPAREN:
		// A parenthesized type group: exactly one type inside.
		p.next()
		if err := p.typ(); err != nil {
			return err
		}
		return p.expect(token.RPAREN, "to close the parenthesized type")
	case token.IDENT:
		return p.namedType()
	default:
		return p.errorf("unexpected %q: want a type", p.lit)
	}
}

// namedType parses a bare or package-qualified type name, optionally
// instantiated ("_p.T[int]").
func (p *typeParser) namedType() error {
	if p.tok != token.IDENT {
		return p.errorf("want a type name, found %q", p.lit)
	}
	p.next()
	if p.tok == token.PERIOD {
		p.next()
		if err := p.expect(token.IDENT, "after the package qualifier"); err != nil {
			return err
		}
	}
	if p.tok == token.LBRACK {
		p.next()
		if err := p.typ(); err != nil {
			return err
		}
		for p.tok == token.COMMA {
			p.next()
			if err := p.typ(); err != nil {
				return err
			}
		}
		if err := p.expect(token.RBRACK, "to close the type arguments"); err != nil {
			return err
		}
	}
	return nil
}

// signature parses a function type "(params)" with optional results.
func (p *typeParser) signature() error {
	if err := p.expect(token.LPAREN, "after func"); err != nil {
		return err
	}
	if err := p.paramList(true); err != nil {
		return err
	}
	switch {
	case p.tok == token.LPAREN:
		p.next()
		return p.paramList(false)
	case p.startsType():
		return p.typ()
	}
	return nil
}

// identRun parses a comma-separated run of plain identifiers (parameter,
// result or field names) and returns their count.
func (p *typeParser) identRun() (int, error) {
	n := 0
	for {
		if p.tok != token.IDENT {
			return n, p.errorf("want an identifier, found %q", p.lit)
		}
		n++
		p.next()
		if p.tok == token.COMMA {
			p.next()
			continue
		}
		return n, nil
	}
}

// paramList parses "( ... )" parameter or result declarations. Each
// declaration is either a comma-separated identifier list naming a following
// type ("x, y int"), an unnamed type ("int", "*T", "pkg.T"), or — in a
// parameter list only — a variadic parameter ("...T", "x ...T") which must
// be last and carry exactly one name, as Go requires. Mixing named and
// unnamed declarations is invalid Go and rejected.
func (p *typeParser) paramList(allowVariadic bool) error {
	if p.atEnd() {
		return p.errorf("unexpected end: want %q", token.RPAREN)
	}
	if p.tok == token.RPAREN {
		p.next()
		return nil
	}
	named, unnamed := 0, 0
	for {
		switch {
		case p.tok == token.ELLIPSIS:
			if !allowVariadic {
				return p.errorf("variadic results are not valid Go")
			}
			p.next()
			if err := p.typ(); err != nil {
				return err
			}
			unnamed++
			// A variadic parameter must be the last one.
			if err := p.closeParamList(); err != nil {
				return err
			}
			if named > 0 && unnamed > 0 {
				return p.errorf("named and unnamed parameters cannot be mixed")
			}
			return nil
		case p.startsTypeKeyword():
			if err := p.typ(); err != nil {
				return err
			}
			unnamed++
		case p.tok == token.IDENT && p.peekIsQualified():
			// An unambiguous qualified type start ("pkg.T").
			if err := p.namedType(); err != nil {
				return err
			}
			unnamed++
		default:
			count, err := p.identRun()
			if err != nil {
				return err
			}
			if p.tok == token.ELLIPSIS {
				if !allowVariadic {
					return p.errorf("variadic results are not valid Go")
				}
				if count != 1 {
					return p.errorf("variadic parameter must have exactly one name")
				}
				p.next()
				if err := p.typ(); err != nil {
					return err
				}
				named++
				if err := p.closeParamList(); err != nil {
					return err
				}
				if named > 0 && unnamed > 0 {
					return p.errorf("named and unnamed parameters cannot be mixed")
				}
				return nil
			}
			if p.startsType() {
				if err := p.typ(); err != nil {
					return err
				}
				named += count
			} else {
				unnamed += count
			}
		}
		if p.tok == token.COMMA {
			p.next()
			continue
		}
		if err := p.closeParamList(); err != nil {
			return err
		}
		if named > 0 && unnamed > 0 {
			return p.errorf("named and unnamed parameters cannot be mixed")
		}
		return nil
	}
}

// closeParamList consumes the ')' closing a parameter list. A variadic
// parameter must be last, so its caller closes directly; all other callers
// must not have a comma pending (handled above).
func (p *typeParser) closeParamList() error {
	return p.expect(token.RPAREN, "to close the parameter list")
}

// startsTypeKeyword reports whether the current token is an unambiguous
// type keyword or operator (pointer, slice/array, paren, channel direction,
// func, map, struct, interface).
func (p *typeParser) startsTypeKeyword() bool {
	switch p.tok {
	case token.MUL, token.LBRACK, token.LPAREN, token.CHAN, token.FUNC,
		token.MAP, token.STRUCT, token.INTERFACE, token.ARROW:
		return true
	}
	return false
}

// structBody parses a struct literal type argument: "struct{ field-list }",
// where a field is a comma-separated identifier list followed by a type, an
// optional string tag, or a single embedded type (qualified and non-identifier
// embedded types such as pointers included). Fields are separated by ';' with
// an optional trailing ';'.
func (p *typeParser) structBody() error {
	if err := p.expect(token.LBRACE, "after struct"); err != nil {
		return err
	}
	if p.tok == token.RBRACE {
		p.next()
		return nil
	}
	for {
		if !p.startsTypeKeyword() && !(p.tok == token.IDENT && p.peekIsQualified()) {
			// A named field: a comma-separated identifier list plus its type.
			if _, err := p.identRun(); err != nil {
				return err
			}
			if !p.startsType() {
				return p.errorf("struct field %q must be a single embedded type or name a field", p.lit)
			}
			if err := p.typ(); err != nil {
				return err
			}
			if p.tok == token.STRING {
				// A field tag, part of the formatter's Go output.
				p.next()
			}
		} else if err := p.typ(); err != nil {
			// A single embedded type (qualified, pointer or other).
			return err
		}
		if p.tok == token.SEMICOLON {
			p.next()
			if p.tok == token.RBRACE {
				p.next()
				return nil
			}
			continue
		}
		break
	}
	return p.expect(token.RBRACE, "to close the struct body")
}

// ifaceBody parses an interface literal type argument: "interface{ method-set
// }", where a spec is a method declaration ("M(params) results") or a single
// embedded type. Specs are separated by ';' with an optional trailing ';'.
func (p *typeParser) ifaceBody() error {
	if err := p.expect(token.LBRACE, "after interface"); err != nil {
		return err
	}
	if p.tok == token.RBRACE {
		p.next()
		return nil
	}
	for {
		if p.tok == token.IDENT && p.peekIsQualified() {
			// A qualified embedded type.
			if err := p.typ(); err != nil {
				return err
			}
		} else {
			count, err := p.identRun()
			if err != nil {
				return err
			}
			switch {
			case p.tok == token.LPAREN:
				// A method declaration: Name(params) results.
				if err := p.methodParamsResults(); err != nil {
					return err
				}
			case count == 1:
				// A bare embedded type name.
			default:
				return p.errorf("interface spec must be a method declaration or a single embedded type")
			}
		}
		if p.tok == token.SEMICOLON {
			p.next()
			if p.tok == token.RBRACE {
				p.next()
				return nil
			}
			continue
		}
		break
	}
	return p.expect(token.RBRACE, "to close the interface body")
}

// methodParamsResults parses "(params) results" after an interface method
// name consumed by identRun.
func (p *typeParser) methodParamsResults() error {
	if err := p.expect(token.LPAREN, "after the interface method name"); err != nil {
		return err
	}
	if err := p.paramList(true); err != nil {
		return err
	}
	switch {
	case p.tok == token.LPAREN:
		p.next()
		return p.paramList(false)
	case p.startsType():
		return p.typ()
	}
	return nil
}

// expr parses a constant-expression array length: unary ops (+ - ^ !), then
// atoms (numbers, dotted identifiers, parenthesized expressions) joined by
// binary operators — the full set the formatter can emit, including &^.
func (p *typeParser) expr() error {
	if err := p.unary(); err != nil {
		return err
	}
	for {
		switch p.tok {
		case token.ADD, token.SUB, token.MUL, token.QUO, token.REM,
			token.AND, token.OR, token.XOR, token.SHL, token.SHR, token.AND_NOT,
			token.LAND, token.LOR, token.EQL, token.NEQ, token.LSS, token.LEQ,
			token.GTR, token.GEQ:
			p.next()
			if err := p.unary(); err != nil {
				return err
			}
		default:
			return nil
		}
	}
}

func (p *typeParser) unary() error {
	switch p.tok {
	case token.ADD, token.SUB, token.XOR, token.NOT:
		p.next()
		return p.unary()
	}
	return p.atom()
}

func (p *typeParser) atom() error {
	switch p.tok {
	case token.INT, token.FLOAT, token.IMAG:
		p.next()
		return nil
	case token.LPAREN:
		p.next()
		if err := p.expr(); err != nil {
			return err
		}
		return p.expect(token.RPAREN, "to close the parenthesized expression")
	case token.IDENT:
		// A dotted identifier (e.g. "pkg.Const"); no slashes — in an array
		// length a slash is the division operator and a hyphen subtraction.
		p.next()
		for p.tok == token.PERIOD {
			p.next()
			if err := p.expect(token.IDENT, "after '.' in an array length"); err != nil {
				return err
			}
		}
		return nil
	default:
		return p.errorf("unexpected %q in array length", p.lit)
	}
}
