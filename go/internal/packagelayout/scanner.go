package packagelayout

import (
	"fmt"
	"go/scanner"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// noGoPackageError identifies a directory whose Go sources are all excluded
// by the target context. It replaces go/build.NoGoError so package discovery
// does not need to call go/build's source-loading methods.
type noGoPackageError struct {
	dir string
}

func (e *noGoPackageError) Error() string {
	return fmt.Sprintf("no Go files for target in %q", e.dir)
}

// scanSDKPackageDir selects one SDK package's source files and extracts the
// package clause/import paths needed to construct the layout graph. The SDK
// walk does not need a typed AST: go/scanner is sufficient for the compiler's
// file-selection metadata and avoids making package discovery itself an
// analysis-defeating parser edge.
func scanSDKPackageDir(sdk fs.FS, dir string, tags BuildTags) (string, []string, []string, error) {
	entries, err := fs.ReadDir(sdk, dir)
	if err != nil {
		return "", nil, nil, err
	}

	var packageName string
	var goFiles []string
	imports := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		filePath := path.Join(dir, name)
		if !fileMatchesBuildConstraintsWithTags(sdk, filePath, tags) {
			continue
		}
		source, err := fs.ReadFile(sdk, filePath)
		if err != nil {
			return "", nil, nil, fmt.Errorf("reading source file %q: %w", filePath, err)
		}
		name, fileImports, usesCgo, err := scanGoSource(filePath, source)
		if err != nil {
			return "", nil, nil, err
		}
		if usesCgo && !tags.CgoEnabled {
			continue
		}
		if packageName == "" {
			packageName = name
		} else if packageName != name {
			return "", nil, nil, fmt.Errorf("package directory %q contains package names %q and %q", dir, packageName, name)
		}
		goFiles = append(goFiles, entry.Name())
		for _, importPath := range fileImports {
			imports[importPath] = true
		}
	}
	if len(goFiles) == 0 {
		return "", nil, nil, &noGoPackageError{dir: dir}
	}

	sort.Strings(goFiles)
	sortedImports := make([]string, 0, len(imports))
	for importPath := range imports {
		sortedImports = append(sortedImports, importPath)
	}
	sort.Strings(sortedImports)
	return packageName, goFiles, sortedImports, nil
}

// scanGoSource extracts the package clause and imports from Go source using the
// standard lexical scanner. In addition to lexical errors it validates the
// package/import declaration grammar and delimiter balance. This is the small
// syntax subset needed by layout construction, and keeps malformed source from
// being accepted merely because its import strings were recoverable.
func scanGoSource(path string, source []byte) (string, []string, bool, error) {
	fset := token.NewFileSet()
	tokenFile := fset.AddFile(path, -1, len(source))
	var scanErr error
	var sourceScanner scanner.Scanner
	sourceScanner.Init(tokenFile, source, func(pos token.Position, message string) {
		if scanErr == nil {
			scanErr = fmt.Errorf("%s: %s", pos, message)
		}
	}, scanner.ScanComments)

	packageName := ""
	packageSeen := false
	expectingPackageName := false
	packageClausePending := false
	seenNonCommentToken := false
	inImport := false
	importGroup := false
	importState := importNeedsPathOrAlias
	imports := make(map[string]bool)
	usesCgo := false
	var delimiters []token.Token
	for {
		_, tok, literal := sourceScanner.Scan()
		if tok == token.EOF {
			break
		}
		if scanErr != nil {
			return "", nil, false, fmt.Errorf("parsing source file %q: %w", path, scanErr)
		}
		if tok == token.COMMENT {
			continue
		}
		if !seenNonCommentToken {
			seenNonCommentToken = true
			if tok != token.PACKAGE {
				return "", nil, false, fmt.Errorf("parsing source file %q: package clause must be first", path)
			}
		}
		if packageClausePending && tok != token.SEMICOLON {
			return "", nil, false, fmt.Errorf("parsing source file %q: package clause is not terminated before %s", path, tok)
		}
		if tok == token.PACKAGE {
			if packageSeen {
				return "", nil, false, fmt.Errorf("parsing source file %q: duplicate package clause", path)
			}
			packageSeen = true
			expectingPackageName = true
			continue
		}
		if tok == token.IDENT && expectingPackageName {
			packageName = literal
			expectingPackageName = false
			packageClausePending = true
			continue
		}
		if tok == token.SEMICOLON {
			if packageClausePending {
				packageClausePending = false
				continue
			}
			if inImport {
				if !importGroup {
					if importState != importAfterPath {
						return "", nil, false, fmt.Errorf("parsing source file %q: import declaration has no path", path)
					}
					inImport = false
					continue
				}
				switch importState {
				case importNeedsPathOrAlias:
					// Blank lines and explicit semicolons between grouped imports
					// are harmless.
				case importAfterPath:
					importState = importNeedsPathOrAlias
				default:
					return "", nil, false, fmt.Errorf("parsing source file %q: import specifier has no path", path)
				}
				continue
			}
			continue
		}
		if inImport {
			switch {
			case !importGroup && importState == importNeedsPathOrAlias && tok == token.LPAREN:
				importGroup = true
				importState = importNeedsPathOrAlias
				delimiters = append(delimiters, tok)
				continue
			case importGroup && tok == token.RPAREN:
				if importState != importNeedsPathOrAlias && importState != importAfterPath {
					return "", nil, false, fmt.Errorf("parsing source file %q: import specifier has no path", path)
				}
				if len(delimiters) == 0 || delimiters[len(delimiters)-1] != token.LPAREN {
					return "", nil, false, fmt.Errorf("parsing source file %q: import group has an unmatched close delimiter", path)
				}
				delimiters = delimiters[:len(delimiters)-1]
				inImport = false
				importGroup = false
				continue
			case importState == importNeedsPathOrAlias && (tok == token.IDENT || tok == token.PERIOD):
				importState = importNeedsPath
				continue
			case importState == importNeedsPath && tok == token.STRING:
				importPath, err := strconv.Unquote(literal)
				if err != nil {
					return "", nil, false, fmt.Errorf("parsing import path in source file %q: %w", path, err)
				}
				if importPath == "C" {
					usesCgo = true
				} else {
					imports[importPath] = true
				}
				importState = importAfterPath
				continue
			case importState == importNeedsPathOrAlias && tok == token.STRING:
				importPath, err := strconv.Unquote(literal)
				if err != nil {
					return "", nil, false, fmt.Errorf("parsing import path in source file %q: %w", path, err)
				}
				if importPath == "C" {
					usesCgo = true
				} else {
					imports[importPath] = true
				}
				importState = importAfterPath
				continue
			case importState == importAfterPath:
				return "", nil, false, fmt.Errorf("parsing source file %q: unexpected %s after import path", path, tok)
			default:
				return "", nil, false, fmt.Errorf("parsing source file %q: unexpected %s in import declaration", path, tok)
			}
		}
		if tok == token.IMPORT {
			inImport = true
			importGroup = false
			importState = importNeedsPathOrAlias
			continue
		}
		switch tok {
		case token.LPAREN, token.LBRACE, token.LBRACK:
			delimiters = append(delimiters, tok)
		case token.RPAREN, token.RBRACE, token.RBRACK:
			if len(delimiters) == 0 || !matchingSourceDelimiter(delimiters[len(delimiters)-1], tok) {
				return "", nil, false, fmt.Errorf("parsing source file %q: unmatched delimiter %s", path, tok)
			}
			delimiters = delimiters[:len(delimiters)-1]
		}
	}
	if scanErr != nil || sourceScanner.ErrorCount > 0 {
		if scanErr == nil {
			scanErr = fmt.Errorf("scanner reported %d source error(s)", sourceScanner.ErrorCount)
		}
		return "", nil, false, fmt.Errorf("parsing source file %q: %w", path, scanErr)
	}
	if expectingPackageName || packageName == "" {
		return "", nil, false, fmt.Errorf("parsing source file %q: no package clause", path)
	}
	if inImport {
		if !importGroup && importState == importAfterPath {
			inImport = false
		} else {
			return "", nil, false, fmt.Errorf("parsing source file %q: incomplete import declaration", path)
		}
	}
	if len(delimiters) > 0 {
		return "", nil, false, fmt.Errorf("parsing source file %q: unclosed delimiter", path)
	}

	sortedImports := make([]string, 0, len(imports))
	for importPath := range imports {
		sortedImports = append(sortedImports, importPath)
	}
	sort.Strings(sortedImports)
	return packageName, sortedImports, usesCgo, nil
}

type importScanState uint8

const (
	importNeedsPathOrAlias importScanState = iota
	importNeedsPath
	importAfterPath
)

func matchingSourceDelimiter(open, close token.Token) bool {
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

// noGoPackageName reports the package name a source-less standard-library
// directory declares: the package clause of its first parseable .go file,
// reading the files the way go list does when it names a package whose
// every file the target's constraints exclude. ok=false when no file
// declares a package clause.
func noGoPackageName(sdk fs.FS, dir string) (string, bool) {
	entries, err := fs.ReadDir(sdk, dir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		filePath := path.Join(dir, name)
		source, err := fs.ReadFile(sdk, filePath)
		if err != nil {
			continue
		}
		packageName, _, _, err := scanGoSource(filePath, source)
		if err != nil {
			continue
		}
		return packageName, true
	}
	return "", false
}

// hasNonTestGoSource reports whether dir contains any non-test .go file.
// go list std omits a package whose every non-test file the target's
// constraints exclude, but it still names a test-only package (one whose
// every .go file is a _test.go file), so only the latter becomes a node.
func hasNonTestGoSource(sdk fs.FS, dir string, tags BuildTags) bool {
	entries, err := fs.ReadDir(sdk, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		filePath := path.Join(dir, name)
		source, err := fs.ReadFile(sdk, filePath)
		if err != nil {
			return true
		}
		_, _, usesCgo, err := scanGoSource(filePath, source)
		if err != nil || !usesCgo || tags.CgoEnabled {
			return true
		}
	}
	// A cgo-only non-test package with cgo disabled is represented by the Go
	// tool as a source-less package only when its directory also contains test
	// files. A regular package whose non-test files are merely build-tagged out
	// is not part of go list std.
	return false
}

func hasBuildableTestSource(sdk fs.FS, dir string, tags BuildTags) bool {
	entries, err := fs.ReadDir(sdk, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		filePath := path.Join(dir, name)
		if !fileMatchesBuildConstraintsWithTags(sdk, filePath, tags) {
			continue
		}
		source, err := fs.ReadFile(sdk, filePath)
		if err != nil {
			continue
		}
		_, _, usesCgo, err := scanGoSource(filePath, source)
		if err == nil && (!usesCgo || tags.CgoEnabled) {
			return true
		}
	}
	return false
}
