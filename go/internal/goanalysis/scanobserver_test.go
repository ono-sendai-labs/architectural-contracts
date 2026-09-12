package goanalysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func TestScanObserver_ReportsMemberOnlyWorkForBothScanners(t *testing.T) {
	pkgs, members, root := syntheticScanObserverPackages(t)

	var observed []scanObserverEvent
	previous := scanObserver
	scanObserver = func(event scanObserverEvent) {
		observed = append(observed, event)
	}
	defer func() { scanObserver = previous }()

	withoutObserverRefs, withoutObserverImports, err := scanReferencesWithoutObserver(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected unobserved reference scan error: %v", err)
	}
	withoutObserverBypasses, err := scanAnalysisDefeatsWithoutObserver(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected unobserved analysis-defeat scan error: %v", err)
	}
	if len(observed) != 0 {
		t.Fatalf("disabled observer received %d events", len(observed))
	}

	refs, imports, err := ScanReferences(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected observed reference scan error: %v", err)
	}
	bypasses, err := ScanAnalysisDefeats(pkgs, members, root)
	if err != nil {
		t.Fatalf("unexpected observed analysis-defeat scan error: %v", err)
	}
	if !reflect.DeepEqual(refs, withoutObserverRefs) || !reflect.DeepEqual(imports, withoutObserverImports) || !reflect.DeepEqual(bypasses, withoutObserverBypasses) {
		t.Fatalf("observer changed scan results: refs=%v/%v imports=%v/%v bypasses=%v/%v", refs, withoutObserverRefs, imports, withoutObserverImports, bypasses, withoutObserverBypasses)
	}

	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverPackageConsidered, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverPackageConsidered, "example.com/closure", false, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverPackageEntered, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverPackageEntered, "example.com/closure", false, 0)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverSyntaxFiles, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverTypeInfoPackages, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverUses, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverSelections, "example.com/member", true, 0)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverImportPackages, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverASTFiles, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverASTDeclarations, "example.com/member", true, 2)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverImportDeclarations, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverReferences, scanObserverImportSpecs, "example.com/member", true, 1)
	assertNoNonMemberScanWork(t, observed, scanObserverReferences)

	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverPackageConsidered, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverPackageConsidered, "example.com/closure", false, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverPackageEntered, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverPackageEntered, "example.com/closure", false, 0)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverDeclaredFiles, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverGoSourceFiles, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverParsedSyntaxFiles, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverASTFiles, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverASTDeclarations, "example.com/member", true, 2)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverImportDeclarations, "example.com/member", true, 1)
	assertScanObserverCount(t, observed, scanObserverAnalysisDefeats, scanObserverImportSpecs, "example.com/member", true, 1)
	assertNoNonMemberScanWork(t, observed, scanObserverAnalysisDefeats)
}

func scanReferencesWithoutObserver(pkgs []*packages.Package, members facts.MemberSet, root string) ([]facts.ReferenceEdge, []facts.ImportEdge, error) {
	previous := scanObserver
	scanObserver = nil
	defer func() { scanObserver = previous }()
	return ScanReferences(pkgs, members, root)
}

func scanAnalysisDefeatsWithoutObserver(pkgs []*packages.Package, members facts.MemberSet, root string) ([]facts.BypassObservation, error) {
	previous := scanObserver
	scanObserver = nil
	defer func() { scanObserver = previous }()
	return ScanAnalysisDefeats(pkgs, members, root)
}

func assertScanObserverCount(t *testing.T, events []scanObserverEvent, scanner scanObserverScanner, operation scanObserverOperation, pkg string, member bool, want int) {
	t.Helper()
	got := 0
	for _, event := range events {
		if event.Scanner == scanner && event.Operation == operation && event.Package == pkg && event.Member == member {
			got += event.Count
		}
	}
	if got != want {
		t.Errorf("scan observer %s/%s package=%q member=%t count=%d, want %d", scanner, operation, pkg, member, got, want)
	}
}

func assertNoNonMemberScanWork(t *testing.T, events []scanObserverEvent, scanner scanObserverScanner) {
	t.Helper()
	for _, event := range events {
		if event.Scanner == scanner && !event.Member && event.Operation != scanObserverPackageConsidered {
			t.Errorf("non-member scan work observed: %+v", event)
		}
	}
}

func syntheticScanObserverPackages(t *testing.T) ([]*packages.Package, facts.MemberSet, string) {
	t.Helper()
	root := t.TempDir()
	memberPath := "example.com/member"
	closurePath := "example.com/closure"
	depPath := "example.com/dep"

	memberFile := filepath.Join(root, "member.go")
	memberSource := "package member\n\nimport dep \"" + depPath + "\"\n\nvar _ = dep.Value\n"
	if err := os.WriteFile(memberFile, []byte(memberSource), 0o644); err != nil {
		t.Fatalf("writing member fixture: %v", err)
	}
	nonMemberFile := filepath.Join(root, "closure.go")
	nonMemberSource := "package closure\n\n//go:linkname hidden example.com/other.hidden\nfunc hidden()\n"
	if err := os.WriteFile(nonMemberFile, []byte(nonMemberSource), 0o644); err != nil {
		t.Fatalf("writing non-member fixture: %v", err)
	}

	fset := token.NewFileSet()
	memberSyntax, err := parser.ParseFile(fset, memberFile, memberSource, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing member fixture: %v", err)
	}
	nonMemberSyntax, err := parser.ParseFile(fset, nonMemberFile, nonMemberSource, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing non-member fixture: %v", err)
	}

	depTypes := types.NewPackage(depPath, "dep")
	depValue := types.NewVar(token.NoPos, depTypes, "Value", types.Typ[types.Int])
	if previous := depTypes.Scope().Insert(depValue); previous != nil {
		t.Fatalf("synthetic dependency value conflicts with %v", previous)
	}
	var valueIdent *ast.Ident
	ast.Inspect(memberSyntax, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok && ident.Name == "Value" {
			valueIdent = ident
		}
		return true
	})
	if valueIdent == nil {
		t.Fatal("synthetic member value reference was not found")
	}

	dep := &packages.Package{PkgPath: depPath}
	member := &packages.Package{
		PkgPath:   memberPath,
		Fset:      fset,
		Syntax:    []*ast.File{memberSyntax},
		GoFiles:   []string{memberFile},
		TypesInfo: &types.Info{Uses: map[*ast.Ident]types.Object{valueIdent: depValue}, Selections: make(map[*ast.SelectorExpr]*types.Selection)},
		Imports:   map[string]*packages.Package{depPath: dep},
	}
	closure := &packages.Package{
		PkgPath: closurePath,
		Fset:    fset,
		Syntax:  []*ast.File{nonMemberSyntax},
		GoFiles: []string{nonMemberFile},
		TypesInfo: &types.Info{
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		},
		Imports: make(map[string]*packages.Package),
	}
	members, err := facts.NewMemberSet(memberPath)
	if err != nil {
		t.Fatalf("creating synthetic member set: %v", err)
	}
	return []*packages.Package{closure, member}, members, root
}
