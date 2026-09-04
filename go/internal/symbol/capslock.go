package symbol

import (
	"fmt"
	"go/parser"
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
// standard Go syntax (go/parser) after a documented translation of full
// import paths through the canonical package grammar (see
// validateTypeArguments), so acceptance tracks actual formatter output.
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
	rest, err := stripTrailingBrackets(name)
	if err != nil {
		return "", fmt.Errorf("capslock name %q: %w", name, err)
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
//     by parsing a synthetic "type _a <element>" declaration with the
//     standard go/parser. A parenthesized group in a type position must
//     contain a single type, so tuples ("(int, string)") and named lists
//     ("(x int)") that are only legal in function signatures are rejected,
//     while real formatter output — qualified unnamed and named variadic
//     parameters, qualified embedded struct fields, interface-method
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

// checkSingleType validates that arg is exactly one Go type by parsing a
// synthetic type declaration with the standard go/parser. In a type
// position a parenthesized group must contain a single type, so a tuple
// ("(int, string)") or a named list ("(x int)") — both legal only in
// function signatures — is rejected with the parser's actionable error.
func checkSingleType(arg string) error {
	src := "package _capslock\n\ntype _a " + arg + "\n"
	if _, err := parser.ParseFile(token.NewFileSet(), "_capslock.go", src, 0); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(err.Error()), err)
	}
	return nil
}
