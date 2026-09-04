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

// stripReceiverBrackets removes the single trailing type-argument bracket
// group from a "(pkg.Type[args])" receiver spelling. Any other bracket —
// misplaced or a second group — is rejected, so a glued type name like
// "pkg.A[T]B" cannot masquerade as a declared receiver.
func stripReceiverBrackets(recv string) (string, error) {
	rest, ok := stripTrailingBrackets(recv)
	if !ok {
		return "", fmt.Errorf("receiver %q has misplaced, unbalanced or invalid type-argument brackets", recv)
	}
	return rest, nil
}

// stripTrailingBrackets removes one trailing balanced bracket group from s.
// It reports false when s contains brackets that are not exactly one trailing
// balanced group, or when the group's contents are not valid type arguments.
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
				prefix := s[:i]
				// The prefix must be bracket-free in its raw text: a second
				// top-level group ("Load[T][U]") or a misplaced bracket is
				// not a Capslock spelling.
				if strings.ContainsAny(prefix, "[]") {
					return "", false
				}
				return prefix, true
			}
		}
	}
	return "", false
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
		if err := p.paramList(')', true); err != nil {
			return err
		}
		p.space()
		if p.startsType() {
			return p.typ()
		}
		return nil
	case c == '(':
		// A parenthesized type list: "(T)", "(int, string)" or a named
		// result list "(x int)".
		p.i++
		return p.paramList(')', false)
	case c == 's' && p.kw("struct"):
		return p.structBody()
	case c == 'i' && p.kw("interface"):
		return p.ifaceBody()
	case isIdentStart(c):
		return p.name()
	default:
		return fmt.Errorf("type argument: unexpected %q", p.s[p.i:])
	}
}

// structBody parses a struct literal type argument: "struct{ field-list }",
// where a field is a comma-separated identifier list followed by a type, or a
// single embedded type. Fields are separated by ';' with an optional
// trailing ';'.
func (p *typeArgParser) structBody() error {
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != '{' {
		return fmt.Errorf("type argument: struct is missing {")
	}
	p.i++
	p.space()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return nil
	}
	for {
		names, err := p.identRun()
		if err != nil {
			return err
		}
		if p.startsType() {
			if err := p.typ(); err != nil {
				return err
			}
		} else if len(names) != 1 {
			return fmt.Errorf("type argument: struct field %q must be a single embedded type or name a field", strings.Join(names, ", "))
		}
		p.space()
		if p.i < len(p.s) && p.s[p.i] == ';' {
			p.i++
			p.space()
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return nil
			}
			continue
		}
		break
	}
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != '}' {
		return fmt.Errorf("type argument: struct is missing }")
	}
	p.i++
	return nil
}

// ifaceBody parses an interface literal type argument: "interface{ method-set
// }", where a spec is a method declaration ("M(params) results") or a single
// embedded type. Specs are separated by ';' with an optional trailing ';'.
func (p *typeArgParser) ifaceBody() error {
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != '{' {
		return fmt.Errorf("type argument: interface is missing {")
	}
	p.i++
	p.space()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return nil
	}
	for {
		if p.identThenParen() {
			// A method declaration: name (params) results?
			if _, err := p.goIdent(); err != nil {
				return err
			}
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != '(' {
				return fmt.Errorf("type argument: interface method is missing (")
			}
			p.i++
			if err := p.paramList(')', true); err != nil {
				return err
			}
			p.space()
			if p.startsType() {
				if err := p.typ(); err != nil {
					return err
				}
			}
		} else if err := p.typ(); err != nil {
			return err
		}
		p.space()
		if p.i < len(p.s) && p.s[p.i] == ';' {
			p.i++
			p.space()
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return nil
			}
			continue
		}
		break
	}
	p.space()
	if p.i >= len(p.s) || p.s[p.i] != '}' {
		return fmt.Errorf("type argument: interface is missing }")
	}
	p.i++
	return nil
}

// identThenParen reports whether the text at the parser position is an
// identifier followed (after optional space) by '(', without consuming.
func (p *typeArgParser) identThenParen() bool {
	j := p.i
	for j < len(p.s) && (isIdentStart(p.s[j]) || p.s[j] >= '0' && p.s[j] <= '9') {
		j++
	}
	for j < len(p.s) && p.s[j] == ' ' {
		j++
	}
	return j < len(p.s) && p.s[j] == '('
}

// identRun parses a comma-separated list of plain identifiers (no dots,
// hyphens or path separators), as used for parameter, result and field names.
func (p *typeArgParser) identRun() ([]string, error) {
	var names []string
	for {
		name, err := p.goIdent()
		if err != nil {
			return nil, err
		}
		names = append(names, name)
		p.space()
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
			continue
		}
		return names, nil
	}
}

// typeList parses a comma-separated list of pure types closed by close, which
// is consumed (generic argument lists and bare parenthesized type groups).
func (p *typeArgParser) typeList(close byte) error {
	p.space()
	if p.i < len(p.s) && p.s[p.i] == close {
		p.i++
		return nil
	}
	if err := p.typ(); err != nil {
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

// paramList parses a function parameter or result list closed by close, which
// is consumed. Each declaration is either a comma-separated identifier list
// naming a following type ("x, y int") or a single unnamed type ("int",
// "*T"). Mixing named and unnamed declarations is invalid Go and rejected.
// When variadic is set, the final declaration may be "...T"; a variadic
// parameter must be the last one.
func (p *typeArgParser) paramList(close byte, variadic bool) error {
	p.space()
	if p.i < len(p.s) && p.s[p.i] == close {
		p.i++
		return nil
	}
	named := 0
	unnamed := 0
	for {
		p.space()
		if p.i >= len(p.s) {
			return fmt.Errorf("type argument: missing %q", string(close))
		}
		if variadic && strings.HasPrefix(p.s[p.i:], "...") {
			p.i += 3
			if err := p.typ(); err != nil {
				return err
			}
			unnamed++
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != close {
				return fmt.Errorf("type argument: variadic parameter must be the last one")
			}
			p.i++
			break
		}
		if !isIdentStart(p.s[p.i]) || p.startsTypeKeyword() {
			// An unambiguous type start: pointer, slice, map, chan, func,
			// struct, interface, paren or send direction.
			if err := p.typ(); err != nil {
				return err
			}
			unnamed++
		} else {
			names, err := p.identRun()
			if err != nil {
				return err
			}
			if p.startsType() {
				if err := p.typ(); err != nil {
					return err
				}
				named++
			} else {
				// The identifiers were themselves unnamed types.
				unnamed += len(names)
			}
		}
		p.space()
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
			continue
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != close {
			return fmt.Errorf("type argument: missing %q", string(close))
		}
		p.i++
		break
	}
	if named > 0 && unnamed > 0 {
		return fmt.Errorf("type argument: named and unnamed parameters cannot be mixed")
	}
	return nil
}

// startsTypeKeyword reports whether a type-keyword (func, chan, map, struct,
// interface, <-) begins at the parser position, without consuming.
func (p *typeArgParser) startsTypeKeyword() bool {
	j := p.i
	for _, w := range []string{"func", "chan", "map", "struct", "interface", "<-"} {
		if strings.HasPrefix(p.s[j:], w) {
			end := j + len(w)
			if w == "<-" || end >= len(p.s) || !isWordByte(p.s[end]) {
				return true
			}
		}
	}
	return false
}

// name parses a package-qualified or bare name: a dotted module host of
// plain identifiers, optionally followed by slash-separated path elements
// whose segments may contain hyphens and dots ("gopkg.in/yaml.v2",
// "example.com/x-y"), then the qualified type identifier, optionally
// instantiated ("example.com/x.Pair[int]").
func (p *typeArgParser) name() error {
	if err := p.hostPart(); err != nil {
		return err
	}
	if p.i < len(p.s) && p.s[p.i] == '/' {
		// Slash-separated path. Dots in path elements ("yaml.v2") are
		// ambiguous with the qualified type name, so the tail is scanned
		// as one token and split at its last dot: the remainder is the
		// path, the final segment must be a plain type identifier.
		start := p.i
		for p.i < len(p.s) && (isIdentCont(p.s[p.i]) || p.s[p.i] == '.' || p.s[p.i] == '/') {
			p.i++
		}
		tok := p.s[start+1 : p.i] // skip the leading path separator
		lastDot := strings.LastIndexByte(tok, '.')
		if lastDot < 0 {
			return fmt.Errorf("type argument: missing type name after package path %q", tok)
		}
		if !validGoIdent(tok[lastDot+1:]) {
			return fmt.Errorf("type argument: invalid type name %q", tok[lastDot+1:])
		}
		if err := validatePathTail(tok[:lastDot]); err != nil {
			return err
		}
	} else if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		if _, err := p.goIdent(); err != nil {
			return err
		}
	}
	if p.i < len(p.s) && p.s[p.i] == '[' {
		// A generic named type used as an argument ("Pair[int]").
		p.i++
		return p.typeList(']')
	}
	return nil
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

// validatePathTail validates the slash-separated path elements of a
// package-path tail (everything after the module host and before the type
// name). Elements are non-empty; their dot-separated segments start with a
// letter or underscore and continue with identifier characters or hyphens.
func validatePathTail(tail string) error {
	for _, elem := range strings.Split(tail, "/") {
		if elem == "" {
			return fmt.Errorf("type argument: empty path element in %q", tail)
		}
		for _, seg := range strings.Split(elem, ".") {
			if seg == "" || !isIdentStart(seg[0]) {
				return fmt.Errorf("type argument: invalid path segment %q", seg)
			}
			for i := 1; i < len(seg); i++ {
				if !isIdentCont(seg[i]) {
					return fmt.Errorf("type argument: invalid path segment %q", seg)
				}
			}
		}
	}
	return nil
}

// hostPart parses the leading module host: plain identifier segments joined
// by dots ("example.com", "gopkg.in"). Hyphens are not allowed here — they
// are only valid inside slash-separated path elements — so a malformed bare
// name like "foo-bar" is rejected.
func (p *typeArgParser) hostPart() error {
	if _, err := p.goIdent(); err != nil {
		return err
	}
	for p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		if _, err := p.goIdent(); err != nil {
			return err
		}
	}
	return nil
}

// goIdent parses a plain Go identifier: a letter or underscore followed by
// letters, digits or underscores. No hyphens and no leading digit.
func (p *typeArgParser) goIdent() (string, error) {
	p.space()
	start := p.i
	if p.i < len(p.s) && isIdentStart(p.s[p.i]) {
		p.i++
		for p.i < len(p.s) && (isIdentStart(p.s[p.i]) || p.s[p.i] >= '0' && p.s[p.i] <= '9') {
			p.i++
		}
	}
	if p.i == start {
		return "", fmt.Errorf("type argument: missing identifier at %q", p.s[p.i:])
	}
	if p.s[start] >= '0' && p.s[start] <= '9' {
		return "", fmt.Errorf("type argument: invalid identifier %q", p.s[start:p.i])
	}
	return p.s[start:p.i], nil
}

// expr parses a constant-expression array length: atoms joined by +, -, *,
// /, %, <<, >>, &, | and ^, where an atom is a number, a dotted identifier
// (no slashes — a slash in an array length is the division operator) or a
// parenthesized expression.
func (p *typeArgParser) expr() error {
	if err := p.atom(); err != nil {
		return err
	}
	for {
		p.space()
		if p.i >= len(p.s) {
			return nil
		}
		switch c := p.s[p.i]; {
		case c == '+' || c == '-' || c == '*' || c == '/' || c == '%' || c == '&' || c == '|' || c == '^':
			p.i++
		case c == '<' || c == '>':
			if !strings.HasPrefix(p.s[p.i:], "<<") && !strings.HasPrefix(p.s[p.i:], ">>") {
				return fmt.Errorf("type argument: unexpected %q in array length", p.s[p.i:])
			}
			p.i += 2
		default:
			return nil
		}
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
		// A dotted identifier, but no slash and no hyphen: in an array
		// length a slash is the division operator and a hyphen is
		// subtraction.
		if _, err := p.goIdent(); err != nil {
			return err
		}
		for p.i < len(p.s) && p.s[p.i] == '.' {
			p.i++
			if _, err := p.goIdent(); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("type argument: unexpected %q in array length", p.s[p.i:])
}
