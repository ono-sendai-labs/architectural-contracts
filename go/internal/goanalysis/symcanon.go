package goanalysis

import (
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// stripAllBrackets recursively removes all brackets and their contents from a string,
// such as generic type parameters/arguments like "[T]" or "[K, V]".
func stripAllBrackets(s string) string {
	var sb strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '[' {
			depth++
		} else if ch == ']' {
			if depth > 0 {
				depth--
			}
		} else if depth == 0 {
			sb.WriteByte(ch)
		}
	}
	return sb.String()
}

// canonicalizeSymbol rewrites package-qualified components in supported
// formatted function and method symbols. It parses the outer receiver/method
// shape first, then scans type expressions token by token so package paths in
// nested generic arguments are handled without replacing type or method names.
// Unsupported or malformed shapes are returned byte-for-byte unchanged.
func canonicalizeSymbol(sym string) string {
	if strings.HasPrefix(sym, "(*") || strings.HasPrefix(sym, "(") {
		close := matchingSymbolParen(sym)
		if close < 0 {
			return sym
		}
		innerStart := 1
		if strings.HasPrefix(sym, "(*") {
			innerStart = 2
		}
		if close <= innerStart {
			return sym
		}
		if close != len(sym)-1 && (close+2 >= len(sym) || sym[close+1] != '.' || !isIdentifier(sym[close+2:])) {
			return sym
		}
		rewritten, ok := rewriteSymbolTypeExpression(sym[innerStart:close])
		if !ok {
			return sym
		}
		return sym[:innerStart] + rewritten + sym[close:]
	}

	dot := topLevelSymbolDot(sym)
	if dot <= 0 || dot+1 >= len(sym) || !isPackagePath(sym[:dot]) {
		return sym
	}
	name, arguments, ok := splitSymbolTail(sym[dot+1:])
	if !ok {
		return sym
	}
	if arguments != "" {
		arguments, ok = rewriteSymbolTypeArguments(arguments)
		if !ok {
			return sym
		}
	}
	return hostpolicy.CanonicalizePath(sym[:dot]) + "." + name + arguments
}

type symbolPathReplacement struct {
	start int
	end   int
	path  string
}

// rewriteSymbolTypeExpression validates the supported type-expression grammar
// before rewriting qualified names. Unsupported or malformed expressions are
// returned unchanged instead of being partially rewritten.
func rewriteSymbolTypeExpression(expr string) (string, bool) {
	parser := symbolTypeParser{expr: expr}
	if !parser.parseType() || !parser.atEnd() {
		return expr, false
	}
	return parser.rewrite(), true
}

func rewriteSymbolTypeArguments(expr string) (string, bool) {
	parser := symbolTypeParser{expr: expr}
	if !parser.parseTypeArguments() || !parser.atEnd() {
		return expr, false
	}
	return parser.rewrite(), true
}

type symbolTypeParser struct {
	expr         string
	pos          int
	replacements []symbolPathReplacement
}

func (p *symbolTypeParser) parseType() bool {
	p.skipSpace()
	if p.pos >= len(p.expr) {
		return false
	}
	if p.consume("*") {
		return p.parseType()
	}
	if p.consume("<-") {
		if !p.consumeName("chan") {
			return false
		}
		return p.parseType()
	}
	if p.expr[p.pos] == '[' {
		return p.parseArrayOrSlice()
	}
	if p.expr[p.pos] == '(' {
		p.pos++
		if !p.parseType() {
			return false
		}
		p.skipSpace()
		return p.consume(")")
	}

	start := p.pos
	for p.pos < len(p.expr) && isPackagePathChar(p.expr[p.pos]) {
		p.pos++
	}
	if start == p.pos {
		return false
	}
	token := p.expr[start:p.pos]
	if token == "map" {
		return p.parseTypeArguments() && p.parseType()
	}
	if token == "chan" {
		p.skipSpace()
		p.consume("<-")
		return p.parseType()
	}
	if dot := strings.LastIndexByte(token, '.'); dot >= 0 {
		if dot == 0 || dot+1 == len(token) || !isPackagePath(token[:dot]) || !isIdentifier(token[dot+1:]) {
			return false
		}
		p.replacements = append(p.replacements, symbolPathReplacement{
			start: start,
			end:   start + dot,
			path:  token[:dot],
		})
	} else if !isIdentifier(token) {
		return false
	}
	if p.peekNonSpace() == '[' {
		return p.parseTypeArguments()
	}
	return true
}

func (p *symbolTypeParser) parseTypeArguments() bool {
	p.skipSpace()
	if !p.consume("[") || !p.parseType() {
		return false
	}
	for {
		p.skipSpace()
		if p.consume("]") {
			return true
		}
		if !p.consume(",") || !p.parseType() {
			return false
		}
	}
}

func (p *symbolTypeParser) parseArrayOrSlice() bool {
	if !p.consume("[") {
		return false
	}
	p.skipSpace()
	if p.consume("]") {
		return p.parseType()
	}
	start := p.pos
	for p.pos < len(p.expr) && p.expr[p.pos] != ']' {
		if !isArrayLengthChar(p.expr[p.pos]) {
			return false
		}
		p.pos++
	}
	if start == p.pos || !p.consume("]") {
		return false
	}
	return p.parseType()
}

func isArrayLengthChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || strings.ContainsRune("_ +-*/%().", rune(ch))
}

func (p *symbolTypeParser) consumeName(name string) bool {
	p.skipSpace()
	if !strings.HasPrefix(p.expr[p.pos:], name) {
		return false
	}
	p.pos += len(name)
	return true
}

func (p *symbolTypeParser) consume(s string) bool {
	if !strings.HasPrefix(p.expr[p.pos:], s) {
		return false
	}
	p.pos += len(s)
	return true
}

func (p *symbolTypeParser) peekNonSpace() byte {
	pos := p.pos
	for pos < len(p.expr) && (p.expr[pos] == ' ' || p.expr[pos] == '\t' || p.expr[pos] == '\n') {
		pos++
	}
	if pos == len(p.expr) {
		return 0
	}
	return p.expr[pos]
}

func (p *symbolTypeParser) skipSpace() {
	for p.pos < len(p.expr) && (p.expr[p.pos] == ' ' || p.expr[p.pos] == '\t' || p.expr[p.pos] == '\n') {
		p.pos++
	}
}

func (p *symbolTypeParser) atEnd() bool {
	p.skipSpace()
	return p.pos == len(p.expr)
}

func (p *symbolTypeParser) rewrite() string {
	result := p.expr
	for i := len(p.replacements) - 1; i >= 0; i-- {
		r := p.replacements[i]
		result = result[:r.start] + hostpolicy.CanonicalizePath(r.path) + result[r.end:]
	}
	return result
}

func matchingSymbolParen(sym string) int {
	parenDepth := 0
	bracketDepth := 0
	for i := 0; i < len(sym); i++ {
		switch sym[i] {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth == 0 {
				return -1
			}
			bracketDepth--
		case '(':
			if bracketDepth == 0 {
				parenDepth++
			}
		case ')':
			if bracketDepth != 0 {
				continue
			}
			parenDepth--
			if parenDepth == 0 {
				return i
			}
			if parenDepth < 0 {
				return -1
			}
		}
	}
	return -1
}

func topLevelSymbolDot(sym string) int {
	bracketDepth := 0
	last := -1
	for i := 0; i < len(sym); i++ {
		switch sym[i] {
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth == 0 {
				return -1
			}
			bracketDepth--
		case '.':
			if bracketDepth == 0 {
				last = i
			}
		}
	}
	if bracketDepth != 0 {
		return -1
	}
	return last
}

func isPackagePath(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isPackagePathChar(s[i]) {
			return false
		}
	}
	return true
}

func isPackagePathChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.' || ch == '/'
}

func isIdentifier(s string) bool {
	if s == "" || !((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z') || s[0] == '_') {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') ||
			(s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
			return false
		}
	}
	return true
}

func splitSymbolTail(s string) (name, arguments string, ok bool) {
	for i := 0; i < len(s); i++ {
		if !((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') ||
			(s[i] >= '0' && s[i] <= '9') || s[i] == '_') {
			if i == 0 || s[i] != '[' {
				return "", "", false
			}
			name = s[:i]
			arguments = s[i:]
			return name, arguments, balancedBrackets(arguments)
		}
	}
	return s, "", isIdentifier(s)
}

func balancedBrackets(s string) bool {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return false
			}
			depth--
		}
	}
	return depth == 0
}
