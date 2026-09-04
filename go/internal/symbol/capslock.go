package symbol

import (
	"fmt"
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
// identical IDs in any host namespace. Names that cannot be mapped safely to
// a declared symbol — unbalanced, misplaced, empty or non-type bracket
// contents, dotted method spellings, compiler-synthesized names — are
// rejected with an error instead of guessed, so classifier text can never
// enter an artifact as an ID.
//
// Unparenthesized spellings whose package path contains a dot (such as
// "gopkg.in/yaml.v2.Unmarshal") are ambiguous without outside knowledge: a
// dot could equally be a dotted method separator, and Capslock's text carries
// no object-kind information. ParseCapslock rejects them; callers that know
// the package inventory use ParseCapslockWithPackages to disambiguate.
func ParseCapslock(name string) (SymbolID, error) {
	return parseCapslock(name, nil)
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
	return parseCapslock(name, knownPackage)
}

func parseCapslock(name string, knownPackage func(string) bool) (SymbolID, error) {
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
		return Parse("(" + hostpolicy.CanonicalizePath(recv[:dot]) + "." + recv[dot+1:] + ")." + method)
	}
	// Top-level spelling: type-argument brackets are allowed only as a
	// trailing group (store.Load[example.com/x.T]); anywhere else they would
	// be part of a dotted method spelling, which Capslock does not produce
	// for declared symbols and which cannot be mapped safely.
	rest, ok := stripTrailingBrackets(name)
	if !ok {
		return "", fmt.Errorf("capslock name %q: type-argument brackets are only supported as a trailing group or inside a method receiver", name)
	}
	dot := strings.LastIndexByte(rest, '.')
	if dot < 0 {
		return "", fmt.Errorf("capslock name %q: %q is not a Capslock spelling of a declared top-level symbol", name, rest)
	}
	pkg := rest[:dot]
	if strings.Contains(pkg, ".") {
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
	return Parse(hostpolicy.CanonicalizePath(pkg) + "." + rest[dot+1:])
}

// stripReceiverBrackets removes balanced type-argument bracket groups from a
// "(pkg.Type)" receiver spelling and validates the result contains none.
func stripReceiverBrackets(recv string) (string, error) {
	rest, depth, err := stripBalancedBrackets(recv)
	if err != nil {
		return "", err
	}
	if depth != 0 {
		return "", fmt.Errorf("receiver %q has unbalanced type-argument brackets", recv)
	}
	if strings.ContainsAny(rest, "[]") {
		return "", fmt.Errorf("receiver %q has misplaced type-argument brackets", recv)
	}
	return rest, nil
}

// stripTrailingBrackets removes one trailing balanced bracket group from s.
// It reports false when s contains brackets that are not a single trailing
// balanced group.
func stripTrailingBrackets(s string) (string, bool) {
	if !strings.ContainsAny(s, "[]") {
		return s, true
	}
	if !strings.HasSuffix(s, "]") {
		return "", false
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ']':
			depth++
		case '[':
			depth--
			if depth == 0 {
				inner := s[i+1 : len(s)-1]
				if inner == "" || !validTypeArgument(inner) {
					return "", false
				}
				prefix, depth2, err := stripBalancedBrackets(s[:i])
				if err != nil || depth2 != 0 || strings.ContainsAny(prefix, "[]") {
					return "", false
				}
				return prefix, true
			}
		}
	}
	return "", false
}

// stripBalancedBrackets removes balanced bracket groups from s, returning the
// remaining text and the final nesting depth (0 when every group was closed).
// The contents of each group must look like type arguments — identifiers,
// dots, pointer markers, commas and nested brackets — otherwise the spelling
// is not a Capslock instantiation and is rejected instead of guessed.
func stripBalancedBrackets(s string) (string, int, error) {
	var b strings.Builder
	depth := 0
	groupStart := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			if depth == 0 {
				groupStart = i
			}
			depth++
		case ']':
			depth--
			if depth < 0 {
				return "", 0, fmt.Errorf("unbalanced type-argument brackets")
			}
			if depth == 0 {
				// A type-argument group always carries at least one type
				// argument, and its text must be plausible type-argument
				// syntax; an empty or free-text group is not a Capslock
				// spelling.
				inner := s[groupStart+1 : i]
				if inner == "" || !validTypeArgument(inner) {
					return "", 0, fmt.Errorf("invalid type-argument text %q", inner)
				}
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	if depth > 0 {
		return "", 0, fmt.Errorf("unbalanced type-argument brackets")
	}
	return b.String(), depth, nil
}

// validTypeArgument reports whether s is a type argument from the explicit
// subset of Go type syntax that Capslock instantiations print: pointers,
// slices and arrays (including constant-expression lengths), maps, channels,
// functions (including variadic and multi-result lists), parenthesized
// groups, instantiated named types, and package-qualified named types (import
// paths may contain dots, slashes and hyphens). Struct and interface literal
// bodies are never printed by Capslock and are rejected. The check is a real
// grammar, not a character whitelist: malformed bodies such as ",," or "."
// fail, and valid bodies such as "chan int", "func(...int) (int, string)" or
// "[N+1]byte" pass.
func validTypeArgument(s string) bool {
	p := &typeArgParser{s: s}
	return p.top() == nil
}

// typeArgParser is a recursive-descent parser for the supported type-argument
// subset. Only syntax is checked; no packages are resolved, so any
// well-formed type argument is accepted and any malformed or unsupported
// body is rejected.
type typeArgParser struct {
	s string
	i int
}

func (p *typeArgParser) top() error {
	if err := p.typ(); err != nil {
		return err
	}
	for {
		p.space()
		if p.i >= len(p.s) {
			return nil
		}
		if p.s[p.i] != ',' {
			return fmt.Errorf("type argument: unexpected %q", p.s[p.i:])
		}
		p.i++
		if err := p.typ(); err != nil {
			return err
		}
	}
}

func (p *typeArgParser) space() {
	for p.i < len(p.s) && p.s[p.i] == ' ' {
		p.i++
	}
}

// kw consumes exactly the keyword w followed by a non-word byte.
func (p *typeArgParser) kw(w string) bool {
	p.space()
	if !strings.HasPrefix(p.s[p.i:], w) {
		return false
	}
	end := p.i + len(w)
	if end < len(p.s) && isWordByte(p.s[end]) {
		return false
	}
	p.i = end
	return true
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '-'
}

func (p *typeArgParser) startsType() bool {
	p.space()
	if p.i >= len(p.s) {
		return false
	}
	c := p.s[p.i]
	return isIdentStart(c) || c == '*' || c == '[' || c == '(' || c == '<'
}

func (p *typeArgParser) typ() error {
	p.space()
	if p.i >= len(p.s) {
		return fmt.Errorf("type argument: unexpected end")
	}
	switch c := p.s[p.i]; {
	case c == '*':
		p.i++
		return p.typ()
	case c == '[':
		p.i++
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return p.typ()
		}
		if err := p.expr(); err != nil {
			return err
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != ']' {
			return fmt.Errorf("type argument: missing ]")
		}
		p.i++
		return p.typ()
	case c == '<':
		if !p.kw("<-") {
			return fmt.Errorf("type argument: unexpected %q", p.s[p.i:])
		}
		return p.typ()
	case c == 'c' && p.kw("chan"):
		p.space()
		p.kw("<-")
		return p.typ()
	case c == 'm' && p.kw("map"):
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != '[' {
			return fmt.Errorf("type argument: map is missing [")
		}
		p.i++
		if err := p.typ(); err != nil {
			return err
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != ']' {
			return fmt.Errorf("type argument: map is missing ]")
		}
		p.i++
		return p.typ()
	case c == 'f' && p.kw("func"):
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != '(' {
			return fmt.Errorf("type argument: func is missing (")
		}
		p.i++
		if err := p.typeList(')', true); err != nil {
			return err
		}
		p.space()
		if p.startsType() {
			return p.typ()
		}
		return nil
	case c == '(':
		// A parenthesized group: "(T)" or a parenthesized type list such as
		// a function's multi-result "(int, string)".
		p.i++
		return p.typeList(')', false)
	case c == 's' && p.kw("struct"), c == 'i' && p.kw("interface"):
		return fmt.Errorf("type argument: struct and interface literals are not part of the supported subset")
	case isIdentStart(c):
		return p.name()
	default:
		return fmt.Errorf("type argument: unexpected %q", p.s[p.i:])
	}
}

// typeList parses a comma-separated type list closed by close, which is
// consumed. When variadic is set, the list may end with "...T" (function
// parameter lists); generic argument lists never accept an ellipsis.
func (p *typeArgParser) typeList(close byte, variadic bool) error {
	p.space()
	if p.i < len(p.s) && p.s[p.i] == close {
		p.i++
		return nil
	}
	if variadic && strings.HasPrefix(p.s[p.i:], "...") {
		p.i += 3
		if err := p.typ(); err != nil {
			return err
		}
	} else if err := p.typ(); err != nil {
		return err
	}
	for {
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != ',' {
			break
		}
		p.i++
		if err := p.typ(); err != nil {
			return err
		}
	}
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != close {
		return fmt.Errorf("type argument: missing %q", string(close))
	}
	p.i++
	return nil
}

// name parses a package-qualified or bare name: segments of identifier
// characters (hyphens included, as in import paths) joined by dots, with
// slash-separated path elements ("example.com/x-y.T"), optionally
// instantiated ("example.com/x.Pair[int]").
func (p *typeArgParser) name() error {
	if err := p.unit(); err != nil {
		return err
	}
	for p.i < len(p.s) && (p.s[p.i] == '.' || p.s[p.i] == '/') {
		p.i++
		if err := p.unit(); err != nil {
			return err
		}
	}
	if p.i < len(p.s) && p.s[p.i] == '[' {
		// A generic named type used as an argument ("Pair[int]").
		p.i++
		return p.typeList(']', false)
	}
	return nil
}

func (p *typeArgParser) unit() error {
	start := p.i
	for p.i < len(p.s) && isIdentCont(p.s[p.i]) {
		p.i++
	}
	if p.i == start {
		return fmt.Errorf("type argument: missing name after separator")
	}
	if p.s[start] == '-' || p.s[start] >= '0' && p.s[start] <= '9' {
		return fmt.Errorf("type argument: invalid name %q", p.s[start:min(p.i+1, len(p.s))])
	}
	return nil
}

// expr parses a constant-expression array length: atoms joined by +, -, *,
// / and %, where an atom is a number, a dotted identifier (no slashes — a
// slash in an array length is the division operator) or a parenthesized
// expression.
func (p *typeArgParser) expr() error {
	if err := p.atom(); err != nil {
		return err
	}
	for {
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != '+' && p.s[p.i] != '-' && p.s[p.i] != '*' && p.s[p.i] != '/' && p.s[p.i] != '%' {
			return nil
		}
		p.i++
		if err := p.atom(); err != nil {
			return err
		}
	}
}

func (p *typeArgParser) atom() error {
	p.space()
	if p.i >= len(p.s) {
		return fmt.Errorf("type argument: unexpected end in array length")
	}
	if p.s[p.i] == '(' {
		p.i++
		if err := p.expr(); err != nil {
			return err
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != ')' {
			return fmt.Errorf("type argument: missing )")
		}
		p.i++
		return nil
	}
	if p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
		return nil
	}
	if isIdentStart(p.s[p.i]) {
		// A dotted identifier, but no slash: in an array length a slash is
		// the division operator, not a path separator.
		if err := p.unit(); err != nil {
			return err
		}
		for p.i < len(p.s) && p.s[p.i] == '.' {
			p.i++
			if err := p.unit(); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("type argument: unexpected %q in array length", p.s[p.i:])
}
