package goanalysis

import (
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// ScanAnalysisDefeats scans member packages for the analysis-defeating
// constructs of DR-11 — `//go:linkname` directives, assembly source files and
// cgo use — which create edges typed reference analysis cannot see and must
// never silently pass.
//
// Only declared member package/file metadata is consulted: the loaded
// packages are filtered to members, assembly is read from the package's
// declared non-Go files, and directives are read from the declared Go source
// files (loaded syntax where available, a fresh comment-preserving parse of
// the declared file otherwise — including files the current build
// configuration ignores, which is where a cgo file lands under a cgo-off
// target). No undeclared directory is walked and no non-member closure file
// is ever scanned.
//
// The result is sorted and duplicate-free, so identical inputs produce
// identical output in any enumeration order (N4). Malformed input — a
// declared member file that cannot be read or parsed, or a position outside
// the component root — is an error: the scan returns no partial facts.
func ScanAnalysisDefeats(
	pkgs []*packages.Package,
	members facts.MemberSet,
	root string,
) ([]facts.BypassObservation, error) {
	obs := make(map[facts.BypassKey]struct{})
	observer := scanObserver
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		packagePath := hostpolicy.CanonicalizePath(p.PkgPath)
		member := members.Contains(packagePath)
		notifyScanObserver(observer, scanObserverEvent{
			Scanner:   scanObserverAnalysisDefeats,
			Operation: scanObserverPackageConsidered,
			Package:   packagePath,
			Member:    member,
			Count:     1,
		})
		if !member {
			continue
		}
		if err := scanBypassPackage(p, root, obs, member, observer); err != nil {
			return nil, err
		}
	}
	out := make([]facts.BypassObservation, 0, len(obs))
	for key := range obs {
		out = append(out, facts.BypassObservation{Kind: key.Kind, Site: key.Site})
	}
	out = facts.DedupBypassObservations(facts.SortBypassObservations(out))
	return out, nil
}

// scanBypassPackage routes every declared file of one member package to its
// construct class: assembly files (selected or ignored by the target
// configuration) become assembly observations, and Go source files are
// scanned for directives and cgo use.
func scanBypassPackage(
	p *packages.Package,
	root string,
	obs map[facts.BypassKey]struct{},
	member bool,
	observer scanObservationObserver,
) error {
	packagePath := hostpolicy.CanonicalizePath(p.PkgPath)
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverPackageEntered,
		Package:   packagePath,
		Member:    member,
		Count:     1,
	})

	for _, file := range allDeclaredFiles(p) {
		notifyScanObserver(observer, scanObserverEvent{
			Scanner:   scanObserverAnalysisDefeats,
			Operation: scanObserverDeclaredFiles,
			Package:   packagePath,
			Member:    member,
			Count:     1,
		})
		switch {
		case isAssemblyFile(file):
			notifyScanObserver(observer, scanObserverEvent{
				Scanner:   scanObserverAnalysisDefeats,
				Operation: scanObserverAssemblyFiles,
				Package:   packagePath,
				Member:    member,
				Count:     1,
			})
			if err := addBypassSite(p, root, file, 0, facts.BypassAssembly, obs); err != nil {
				return err
			}
			continue
		case !isGoSourceFile(file):
			continue
		}
		notifyScanObserver(observer, scanObserverEvent{
			Scanner:   scanObserverAnalysisDefeats,
			Operation: scanObserverGoSourceFiles,
			Package:   packagePath,
			Member:    member,
			Count:     1,
		})
		syntax := syntaxForFile(p, file)
		if syntax.file == nil {
			if err := scanBypassSourceFile(p, root, file, obs, member, observer); err != nil {
				return err
			}
			continue
		}
		fset := p.Fset
		if syntax.fset != nil {
			fset = syntax.fset
		}
		notifyScanObserver(observer, scanObserverEvent{
			Scanner:   scanObserverAnalysisDefeats,
			Operation: scanObserverParsedSyntaxFiles,
			Package:   packagePath,
			Member:    member,
			Count:     1,
		})
		if err := scanBypassSyntax(fset, p, root, file, syntax.file, obs, member, observer); err != nil {
			return err
		}
	}
	return nil
}

// allDeclaredFiles returns the package's declared sources, both selected and
// ignored by the current build configuration. Ignored files are scanned
// fail-closed — a construct hidden behind a build tag or a cgo-off target
// must never silently pass (DR-11) — and only Go sources reach the parser;
// assembly is routed to its own construct class and any other declared
// non-Go file (a cgo package's C headers and sources, for example, whose
// cgo use is already observed at the member's `import "C"` line) is inert
// without the Go constructs around it.
func allDeclaredFiles(p *packages.Package) []string {
	return concatFiles(p.GoFiles, p.IgnoredFiles, p.OtherFiles)
}

func concatFiles(lists ...[]string) []string {
	n := 0
	for _, l := range lists {
		n += len(l)
	}
	out := make([]string, 0, n)
	for _, l := range lists {
		out = append(out, l...)
	}
	sort.Strings(out)
	return out
}

func isAssemblyFile(file string) bool {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".s":
		return true
	default:
		return false
	}
}

// isGoSourceFile reports whether a declared file is Go source at all: only
// such files are handed to the Go parser; every other declared file is
// routed by its construct class or skipped as inert.
func isGoSourceFile(file string) bool {
	return filepath.Ext(file) == ".go"
}

// parsedFile pairs an AST with the file set its positions resolve against:
// loaded syntax uses the load's shared file set, a fresh parse uses a local
// one.
type parsedFile struct {
	file *ast.File
	fset *token.FileSet
}

// syntaxForFile returns the loaded syntax AST whose position filename matches
// the declared file, or an empty result when the loader did not parse it.
func syntaxForFile(p *packages.Package, file string) parsedFile {
	if p.Fset == nil || p.Syntax == nil {
		return parsedFile{}
	}
	want := filepath.Clean(file)
	for _, f := range p.Syntax {
		if f == nil {
			continue
		}
		if filepath.Clean(p.Fset.Position(f.Pos()).Filename) == want {
			return parsedFile{file: f}
		}
	}
	return parsedFile{}
}

// scanBypassSourceFile lexes one declared member file not covered by the load's
// syntax. Ignored files need only two constructs for this scan — directives
// and import "C" — so a comment-preserving lexical pass avoids a second Go
// parser dependency while still failing closed on scanner errors or unbalanced
// delimiters. Selected files continue through the typed AST path above.
func scanBypassSourceFile(
	p *packages.Package,
	root string,
	file string,
	obs map[facts.BypassKey]struct{},
	member bool,
	observer scanObservationObserver,
) error {
	src, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("scan analysis defeats: read member file %q: %w", file, err)
	}
	packagePath := hostpolicy.CanonicalizePath(p.PkgPath)
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverLexedSourceFiles,
		Package:   packagePath,
		Member:    member,
		Count:     1,
	})
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverLexedSourceBytes,
		Package:   packagePath,
		Member:    member,
		Count:     len(src),
	})
	fset := token.NewFileSet()
	tokenFile := fset.AddFile(file, -1, len(src))

	var scanErr error
	var sourceScanner scanner.Scanner
	sourceScanner.Init(tokenFile, src, func(pos token.Position, message string) {
		if scanErr == nil {
			scanErr = fmt.Errorf("%s: %s", pos, message)
		}
	}, scanner.ScanComments)

	var delimiters []token.Token
	importDepth := 0
	inImport := false
	sawPackage := false
	expectingPackageName := false
	sourceTokens := 0
	for {
		pos, tok, literal := sourceScanner.Scan()
		if tok == token.EOF {
			break
		}
		sourceTokens++
		if scanErr != nil {
			return fmt.Errorf("scan analysis defeats: member file %q does not lex as Go source: %w", file, scanErr)
		}

		switch tok {
		case token.COMMENT:
			if isLinknameDirective(literal) {
				if err := addBypassSite(p, root, file, fset.Position(pos).Line, facts.BypassLinkname, obs); err != nil {
					return err
				}
			}
		case token.PACKAGE:
			expectingPackageName = true
		case token.IDENT:
			if expectingPackageName {
				sawPackage = true
				expectingPackageName = false
			}
		case token.IMPORT:
			inImport = true
		case token.STRING:
			if inImport && literal == `"C"` {
				if err := addBypassSite(p, root, file, fset.Position(pos).Line, facts.BypassCgo, obs); err != nil {
					return err
				}
			}
		case token.LPAREN:
			delimiters = append(delimiters, tok)
			if inImport {
				importDepth++
			}
		case token.LBRACE, token.LBRACK:
			delimiters = append(delimiters, tok)
		case token.RPAREN, token.RBRACE, token.RBRACK:
			if len(delimiters) == 0 || !matchingDelimiter(delimiters[len(delimiters)-1], tok) {
				return fmt.Errorf("scan analysis defeats: member file %q has an unmatched delimiter at line %d", file, fset.Position(pos).Line)
			}
			delimiters = delimiters[:len(delimiters)-1]
			if tok == token.RPAREN && inImport && importDepth > 0 {
				importDepth--
				if importDepth == 0 {
					inImport = false
				}
			}
		case token.SEMICOLON:
			if inImport && importDepth == 0 {
				inImport = false
			}
		}
	}
	if scanErr != nil {
		return fmt.Errorf("scan analysis defeats: member file %q does not lex as Go source: %w", file, scanErr)
	}
	if expectingPackageName || !sawPackage {
		return fmt.Errorf("scan analysis defeats: member file %q has no valid package declaration", file)
	}
	if len(delimiters) > 0 {
		return fmt.Errorf("scan analysis defeats: member file %q has an unclosed delimiter", file)
	}
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverSourceTokens,
		Package:   packagePath,
		Member:    member,
		Count:     sourceTokens,
	})
	return nil
}

func matchingDelimiter(open, close token.Token) bool {
	switch close {
	case token.RPAREN:
		return open == token.LPAREN
	case token.RBRACE:
		return open == token.LBRACE
	case token.RBRACK:
		return open == token.LBRACK
	default:
		return false
	}
}

// scanBypassSyntax records the analysis-defeating constructs of one parsed
// member file: every `//go:linkname` directive comment and every
// `import "C"` declaration, at their exact source lines.
func scanBypassSyntax(
	fset *token.FileSet,
	p *packages.Package,
	root string,
	file string,
	f *ast.File,
	obs map[facts.BypassKey]struct{},
	member bool,
	observer scanObservationObserver,
) error {
	if fset == nil {
		fset = p.Fset
	}
	packagePath := hostpolicy.CanonicalizePath(p.PkgPath)
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverASTFiles,
		Package:   packagePath,
		Member:    member,
		Count:     1,
	})
	commentGroups := 0
	comments := 0
	for _, cg := range f.Comments {
		commentGroups++
		comments += len(cg.List)
		for _, c := range cg.List {
			if !isLinknameDirective(c.Text) {
				continue
			}
			if err := addBypassSite(p, root, file, fset.Position(c.Pos()).Line, facts.BypassLinkname, obs); err != nil {
				return err
			}
		}
	}
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverASTCommentGroups,
		Package:   packagePath,
		Member:    member,
		Count:     commentGroups,
	})
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverASTComments,
		Package:   packagePath,
		Member:    member,
		Count:     comments,
	})
	astDeclarations := 0
	importDeclarations := 0
	importSpecs := 0
	for _, decl := range f.Decls {
		astDeclarations++
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		importDeclarations++
		for _, spec := range genDecl.Specs {
			importSpecs++
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok || importSpec.Path == nil || importSpec.Path.Value != `"C"` {
				continue
			}
			if err := addBypassSite(p, root, file, fset.Position(importSpec.Pos()).Line, facts.BypassCgo, obs); err != nil {
				return err
			}
		}
	}
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverASTDeclarations,
		Package:   packagePath,
		Member:    member,
		Count:     astDeclarations,
	})
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverImportDeclarations,
		Package:   packagePath,
		Member:    member,
		Count:     importDeclarations,
	})
	notifyScanObserver(observer, scanObserverEvent{
		Scanner:   scanObserverAnalysisDefeats,
		Operation: scanObserverImportSpecs,
		Package:   packagePath,
		Member:    member,
		Count:     importSpecs,
	})
	return nil
}

// isLinknameDirective reports whether a comment text is a `//go:linkname`
// directive: the directive line itself, not an occurrence inside another
// comment.
func isLinknameDirective(text string) bool {
	if !strings.HasPrefix(text, "//go:linkname") {
		return false
	}
	return len(text) == len("//go:linkname") || text[len("//go:linkname")] == ' ' || text[len("//go:linkname")] == '\t'
}

// addBypassSite converts a position to a validated component-relative site
// and records the observation.
func addBypassSite(
	p *packages.Package,
	root string,
	file string,
	line int,
	kind facts.BypassKind,
	obs map[facts.BypassKey]struct{},
) error {
	site, err := bypassSite(p, root, file, line)
	if err != nil {
		return fmt.Errorf("scan analysis defeats: %w", err)
	}
	obs[facts.BypassKey{Kind: kind, Site: site}] = struct{}{}
	return nil
}

// bypassSite makes a component-relative site for a declared member file,
// computed against the file's site root (memberSiteRoot). An explicit line
// (a directive or import) is used directly; a file-level construct such as
// assembly uses line 1, the first line of the file.
func bypassSite(p *packages.Package, root string, file string, line int) (facts.SourceSite, error) {
	siteRoot, err := memberSiteRoot(p, root, file)
	if err != nil {
		return facts.SourceSite{}, err
	}
	if line < 1 {
		line = 1
	}
	rel, err := filepath.Rel(siteRoot, file)
	if err != nil {
		return facts.SourceSite{}, fmt.Errorf("resolving %q relative to site root %q: %w", file, siteRoot, err)
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	site := facts.SourceSite{File: rel, Line: line}
	if err := site.Validate(); err != nil {
		return facts.SourceSite{}, err
	}
	return site, nil
}
