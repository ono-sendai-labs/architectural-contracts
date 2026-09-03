package symbol

import (
	"fmt"
	"strings"
	"unicode"

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
func ParseCapslock(name string) (SymbolID, error) {
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
	if dot < 0 || !capslockTopLevelPackagePlausible(rest[:dot]) {
		return "", fmt.Errorf("capslock name %q: %q is not a Capslock spelling of a declared top-level symbol; a dotted method spelling cannot be mapped safely", name, rest)
	}
	return Parse(hostpolicy.CanonicalizePath(rest[:dot]) + "." + rest[dot+1:])
}

// capslockTopLevelPackagePlausible rejects top-level spellings whose package
// path cannot be a Capslock/SSA package prefix. Capslock/SSA never prints a
// method in dotted form, so such a spelling cannot be mapped safely to a
// declared symbol; rejecting it (rather than reading it as a symbol in an
// implausible package) keeps classifier text from entering an artifact as an
// ID. Two structural rules apply:
//   - dot-separated parts must start lowercase (real import paths keep them
//     lowercase: "example.com", "gopkg.in", "yaml.v2");
//   - dots may appear only in the first slash-separated element (the module
//     host) — so "example.com/p.private.Read" cannot be read as a top-level
//     symbol in package "example.com/p.private". A dotful no-slash host
//     spelling ("gopkg.in", "yaml.v2") remains accepted; a false rejection
//     there would break real Capslock names.
func capslockTopLevelPackagePlausible(pkg string) bool {
	elems := strings.Split(pkg, "/")
	for _, part := range strings.Split(elems[0], ".") {
		if part == "" {
			return false
		}
		if unicode.IsUpper([]rune(part)[0]) {
			return false
		}
	}
	for _, elem := range elems[1:] {
		if strings.Contains(elem, ".") {
			return false
		}
	}
	return true
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
				if inner == "" || !validTypeArgumentText(inner) {
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
				if inner == "" || !validTypeArgumentText(inner) {
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

// validTypeArgumentText reports whether s is plausible type-argument syntax:
// identifiers, package dots, pointer markers, commas and nested brackets,
// with no spaces or other free text.
func validTypeArgumentText(s string) bool {
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
		case r == '_' || r == '.' || r == '*' || r == ',' || r == '[' || r == ']' || r == '/':
		default:
			return false
		}
	}
	return true
}
