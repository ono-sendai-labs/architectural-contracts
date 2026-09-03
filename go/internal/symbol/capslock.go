package symbol

import (
	"fmt"
	"strings"
	"unicode"
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
// It is built on Parse, so there is exactly one canonicalization rule. Names
// that cannot be mapped safely to a declared symbol — unbalanced or
// misplaced brackets, dotted method spellings, compiler-synthesized names —
// are rejected with an error instead of guessed, so classifier text can never
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
		return Parse("(" + recv + ")." + method)
	}
	// Top-level spelling: type-argument brackets are allowed only as a
	// trailing group (store.Load[example.com/x.T]); anywhere else they would
	// be part of a dotted method spelling, which Capslock does not produce
	// for declared symbols and which cannot be mapped safely.
	rest, ok := stripTrailingBrackets(name)
	if !ok {
		return "", fmt.Errorf("capslock name %q: type-argument brackets are only supported as a trailing group or inside a method receiver", name)
	}
	if !capslockTopLevelPackagePlausible(rest) {
		return "", fmt.Errorf("capslock name %q: %q is not a Capslock spelling of a declared top-level symbol; a dotted method spelling cannot be mapped safely", name, rest)
	}
	return Parse(rest)
}

// capslockTopLevelPackagePlausible rejects top-level spellings whose package
// path contains an uppercase-starting dot-separated part. Capslock/SSA never
// prints a method in dotted form, so such a spelling cannot be mapped safely
// to a declared symbol; rejecting it (rather than reading it as a symbol in
// an implausible package) keeps classifier text from entering an artifact as
// an ID. Real import paths keep dot-separated parts lowercase
// ("example.com", "gopkg.in/yaml.v2").
func capslockTopLevelPackagePlausible(text string) bool {
	dot := strings.LastIndexByte(text, '.')
	if dot < 0 {
		return false
	}
	for _, elem := range strings.Split(text[:dot], "/") {
		for _, part := range strings.Split(elem, ".") {
			if part == "" {
				return false
			}
			r := []rune(part)[0]
			if unicode.IsUpper(r) {
				return false
			}
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
				if i == len(s)-2 {
					return "", false // empty type-argument group
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
				// argument; an empty group is not a Capslock spelling.
				if i == groupStart+1 {
					return "", 0, fmt.Errorf("empty type-argument brackets")
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
