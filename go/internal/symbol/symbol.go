// Package symbol defines the versioned textual grammar for SymbolID, the
// stable identifier shared by surface manifests, the standard-library
// authority map and reports (DR-04).
//
// Component Contract (FR10):
//   - What it does: Defines the SymbolID value type, its single v1 textual
//     encoding with strict parse/format, a normalizer from Capslock spellings,
//     go/types object and selection conversion under the declaring-object rule,
//     and a deterministic exact-declared-surface extractor.
//   - What it requires: Parse/Format operate on v1 grammar text; go/types
//     conversion and extraction operate on already type-checked ASTs and their
//     types.Info; extraction canonicalizes package paths through
//     hostpolicy.CanonicalizePath.
//   - What it provides: Parse, Format, Compare, ParseCapslock, FromObject,
//     FromSelection and ExtractSurface. All pure and deterministic; results are
//     sorted and duplicate-free. Nothing here alters the live call-graph check
//     path (capanalyzer.InterfaceSymbol stays until the Step 6 cutover).
//   - Ambient Authority: This component is guaranteed-pure and holds no ambient
//     authority (no filesystem I/O, network, process execution or reflection).
package symbol

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// GrammarVersion is the version of the textual grammar Parse accepts and
// Format produces. The design (DR-04) fixes one v1 grammar whose persisted
// keys are the bare spellings of the design's grammar table; the version of a
// persisted artifact as a whole is carried by that artifact's format_version
// field (DR-15). Parse rejects anything outside the v1 grammar — including
// spellings only a later version could introduce (pointer markers, type
// arguments, dotted method names) — rather than guessing.
const GrammarVersion = 1

// SymbolID identifies one declared Go symbol — a package-level function,
// variable, constant or named/alias type, a declared method, or a package's
// aggregate initialization — under the single v1 grammar:
//
//	pkg.Name            top-level func, var, const, named or alias type
//	(pkg.Type).Method   declared method; receiver base name, no * , no type args
//	pkg.init            package initialization (the aggregate init)
//
// pkg is always a canonical package import path. Interface method specs and
// struct fields have no SymbolID of their own: references to them are
// authorised by the declaring interface or struct type's ID (the
// declaring-object rule, DR-04); FromSelection and FromObject return that ID.
//
// A SymbolID is a plain string value: two IDs are equal when their bytes are,
// and ordering is the byte-wise string order, which is the order used by
// every sorted artifact (N4).
//
// Grammar boundary note: "pkg.Name" is parsed by splitting at the LAST dot,
// because real import paths contain dots (module hosts) and slashes. A
// dotted-method spelling such as "pkg.T.M" is therefore grammatically a
// top-level symbol in a package with a dotted path element and is accepted
// as such by Parse; producers that know Capslock's spellings must reject
// that form instead of guessing (see ParseCapslock).
type SymbolID string

// Compare orders two SymbolIDs byte-wise, suitable for sort.Slice and for
// deterministic, byte-stable artifacts.
func Compare(a, b SymbolID) int {
	return strings.Compare(string(a), string(b))
}

// Format returns the canonical v1 text of the ID. SymbolID values are only
// ever produced through Parse or the conversion constructors, so Format is an
// identity over the canonical bytes and a round trip (format → parse →
// format) is byte-stable.
func (id SymbolID) Format() string {
	return string(id)
}

// String implements fmt.Stringer, returning the canonical v1 text.
func (id SymbolID) String() string {
	return string(id)
}

// Parse strictly parses a v1 grammar spelling into a SymbolID. It rejects
// empty and malformed input, pointer markers, type arguments, and any other
// spelling outside the v1 grammar, with an error that names the offending
// text so a bad artifact can be traced to its source.
func Parse(text string) (SymbolID, error) {
	if recv, name, ok := splitMethod(text); ok {
		if err := validateReceiver(recv); err != nil {
			return "", fmt.Errorf("invalid symbol %q: %w", text, err)
		}
		if !isIdentifier(name) {
			return "", fmt.Errorf("invalid symbol %q: %q is not an identifier", text, name)
		}
		if name == "init" {
			return "", fmt.Errorf("invalid symbol %q: init cannot be declared as a method", text)
		}
		return SymbolID(text), nil
	}
	dot := strings.LastIndexByte(text, '.')
	if dot < 0 {
		return "", fmt.Errorf("invalid symbol %q: want \"pkg.Name\", \"(pkg.Type).Method\" or \"pkg.init\"", text)
	}
	pkg, name := text[:dot], text[dot+1:]
	if pkg == "" {
		return "", fmt.Errorf("invalid symbol %q: empty package path", text)
	}
	if name == "" {
		return "", fmt.Errorf("invalid symbol %q: empty name after package path", text)
	}
	if strings.Contains(name, ".") {
		return "", fmt.Errorf("invalid symbol %q: %q is not a single identifier; methods use the \"(pkg.Type).Method\" form", text, name)
	}
	if !isIdentifier(name) {
		return "", fmt.Errorf("invalid symbol %q: %q is not an identifier", text, name)
	}
	// Type arguments and pointer markers never appear in the v1 grammar.
	if strings.ContainsAny(pkg, "*[]()") || strings.ContainsAny(name, "*[]()") {
		return "", fmt.Errorf("invalid symbol %q: pointer markers and type arguments are not part of the v1 grammar", text)
	}
	if !validPackagePath(pkg) {
		return "", fmt.Errorf("invalid symbol %q: %q is not a valid package import path", text, pkg)
	}
	return SymbolID(text), nil
}

// validPackagePath checks that a package path consists of path elements made
// of Go identifier characters (plus '-' and '.', which appear in real import
// paths such as module hosts). It is a grammar-level sanity check, not a
// path-policy decision: canonicalization of the path itself is the host's
// job (hostpolicy.CanonicalizePath).
func validPackagePath(pkg string) bool {
	if strings.HasPrefix(pkg, "/") || strings.HasSuffix(pkg, "/") || strings.Contains(pkg, "//") {
		return false
	}
	for _, elem := range strings.Split(pkg, "/") {
		if elem == "" {
			return false
		}
		for _, part := range strings.Split(elem, ".") {
			if part == "" {
				return false
			}
			for _, r := range part {
				if r == '_' || r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r) {
					continue
				}
				return false
			}
		}
	}
	return true
}

// splitMethod reports whether text is a "(recv).Name" method spelling, and
// splits it. This is purely syntactic; validation of the parts is the
// caller's job.
func splitMethod(text string) (recv, name string, ok bool) {
	if !strings.HasPrefix(text, "(") {
		return "", "", false
	}
	inner := text[1:]
	idx := strings.Index(inner, ").")
	if idx < 0 {
		return "", "", false
	}
	return inner[:idx], inner[idx+2:], true
}

// validateReceiver checks a "(recv)" method receiver: a non-empty canonical
// package path and a single receiver type identifier, with no pointer marker
// and no type arguments.
func validateReceiver(recv string) error {
	dot := strings.LastIndexByte(recv, '.')
	if dot < 0 {
		return fmt.Errorf("receiver %q is not \"pkg.Type\"", recv)
	}
	pkg, typ := recv[:dot], recv[dot+1:]
	if pkg == "" {
		return fmt.Errorf("receiver %q has an empty package path", recv)
	}
	if typ == "" {
		return fmt.Errorf("receiver %q has an empty type name", recv)
	}
	if strings.Contains(typ, ".") {
		return fmt.Errorf("receiver %q is not a single type identifier", recv)
	}
	if strings.ContainsAny(recv, "*[]()") {
		return fmt.Errorf("receiver %q contains a pointer marker or type arguments; the v1 grammar stores the receiver base name only", recv)
	}
	if !validPackagePath(pkg) {
		return fmt.Errorf("receiver %q has an invalid package path", recv)
	}
	if !isIdentifier(typ) {
		return fmt.Errorf("receiver %q is not an identifier", typ)
	}
	if typ == "init" {
		return fmt.Errorf("receiver %q names init, which is never a type", recv)
	}
	return nil
}

// isIdentifier reports whether s is a valid Go identifier (unicode letters,
// digits and underscores, not starting with a digit).
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || unicode.IsLetter(r) {
			continue
		}
		if unicode.IsDigit(r) && i > 0 {
			continue
		}
		return false
	}
	return utf8.ValidString(s)
}
