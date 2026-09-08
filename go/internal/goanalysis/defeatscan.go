package goanalysis

import (
	"fmt"
	"go/ast"
	"go/parser"
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
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		if !members.Contains(hostpolicy.CanonicalizePath(p.PkgPath)) {
			continue
		}
		if err := scanBypassPackage(p, root, obs); err != nil {
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
) error {
	for _, file := range allDeclaredFiles(p) {
		switch {
		case isAssemblyFile(file):
			if err := addBypassSite(p.Fset, root, file, 0, facts.BypassAssembly, obs); err != nil {
				return err
			}
			continue
		case !isGoSourceFile(file):
			continue
		}
		syntax := syntaxForFile(p, file)
		if syntax.file == nil {
			var err error
			syntax, err = parseDeclaredFile(file)
			if err != nil {
				return err
			}
		}
		fset := p.Fset
		if syntax.fset != nil {
			fset = syntax.fset
		}
		if err := scanBypassSyntax(fset, root, file, syntax.file, obs); err != nil {
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

// parseDeclaredFile reads and parses one declared member file not covered by
// the load's syntax, preserving comments so directives are visible. The parse
// is fail-closed: a read or parse error — even one accompanied by a partial
// AST — aborts the scan, because a directive outside the recoverable portion
// of a malformed file must never be silently missed.
func parseDeclaredFile(file string) (parsedFile, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return parsedFile{}, fmt.Errorf("scan analysis defeats: read member file %q: %w", file, err)
	}
	fset := token.NewFileSet()
	f, parseErr := parser.ParseFile(fset, file, src, parser.ParseComments)
	if parseErr != nil {
		return parsedFile{}, fmt.Errorf("scan analysis defeats: member file %q does not parse as Go source: %w", file, parseErr)
	}
	if f == nil {
		return parsedFile{}, fmt.Errorf("scan analysis defeats: member file %q does not parse as Go source", file)
	}
	return parsedFile{file: f, fset: fset}, nil
}

// scanBypassSyntax records the analysis-defeating constructs of one parsed
// member file: every `//go:linkname` directive comment and every
// `import "C"` declaration, at their exact source lines.
func scanBypassSyntax(
	fset *token.FileSet,
	root string,
	file string,
	f *ast.File,
	obs map[facts.BypassKey]struct{},
) error {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if !isLinknameDirective(c.Text) {
				continue
			}
			if err := addBypassSite(fset, root, file, fset.Position(c.Pos()).Line, facts.BypassLinkname, obs); err != nil {
				return err
			}
		}
	}
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		for _, spec := range genDecl.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok || importSpec.Path == nil || importSpec.Path.Value != `"C"` {
				continue
			}
			if err := addBypassSite(fset, root, file, fset.Position(importSpec.Pos()).Line, facts.BypassCgo, obs); err != nil {
				return err
			}
		}
	}
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
	fset *token.FileSet,
	root string,
	file string,
	line int,
	kind facts.BypassKind,
	obs map[facts.BypassKey]struct{},
) error {
	site, err := bypassSite(fset, root, file, line)
	if err != nil {
		return fmt.Errorf("scan analysis defeats: %w", err)
	}
	obs[facts.BypassKey{Kind: kind, Site: site}] = struct{}{}
	return nil
}

// bypassSite makes a component-relative site for a declared member file. An
// explicit line (a directive or import) is used directly; a file-level
// construct such as assembly uses line 1, the first line of the file.
func bypassSite(fset *token.FileSet, root string, file string, line int) (facts.SourceSite, error) {
	if fset == nil {
		fset = token.NewFileSet()
	}
	if line < 1 {
		line = 1
	}
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return facts.SourceSite{}, fmt.Errorf("resolving %q relative to component root: %w", file, err)
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	site := facts.SourceSite{File: rel, Line: line}
	if err := site.Validate(); err != nil {
		return facts.SourceSite{}, err
	}
	return site, nil
}
