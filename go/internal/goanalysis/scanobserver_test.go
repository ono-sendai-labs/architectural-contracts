package goanalysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

type scanObserverCapture struct {
	events []scanObserverEvent
}

func (capture *scanObserverCapture) observe(event scanObserverEvent) {
	capture.events = append(capture.events, event)
}

type scanObservationSnapshot struct {
	References      scanWorkSnapshot
	AnalysisDefeats scanWorkSnapshot
}

type scanWorkSnapshot struct {
	ConsideredPackages       []string
	MemberEnteredPackages    []string
	NonMemberEnteredPackages []string
	Member                   scanWorkCounts
	NonMember                scanWorkCounts
}

type scanWorkCounts struct {
	SyntaxFiles        int
	TypeInfoPackages   int
	Uses               int
	Selections         int
	ImportPackages     int
	ASTFiles           int
	ASTCommentGroups   int
	ASTComments        int
	ASTDeclarations    int
	ImportDeclarations int
	ImportSpecs        int
	DeclaredFiles      int
	AssemblyFiles      int
	GoSourceFiles      int
	ParsedSyntaxFiles  int
	LexedSourceFiles   int
	LexedSourceBytes   int
	SourceTokens       int
	Other              int
}

func snapshotScanObserver(events []scanObserverEvent) scanObservationSnapshot {
	return scanObservationSnapshot{
		References:      snapshotScanObserverScanner(events, scanObserverReferences),
		AnalysisDefeats: snapshotScanObserverScanner(events, scanObserverAnalysisDefeats),
	}
}

func snapshotScanObserverScanner(events []scanObserverEvent, scanner scanObserverScanner) scanWorkSnapshot {
	var snapshot scanWorkSnapshot
	for _, event := range events {
		if event.Scanner != scanner {
			continue
		}
		switch event.Operation {
		case scanObserverPackageConsidered:
			appendScanObserverPackages(&snapshot.ConsideredPackages, event.Package, event.Count)
		case scanObserverPackageEntered:
			if event.Member {
				appendScanObserverPackages(&snapshot.MemberEnteredPackages, event.Package, event.Count)
			} else {
				appendScanObserverPackages(&snapshot.NonMemberEnteredPackages, event.Package, event.Count)
			}
		default:
			if event.Member {
				snapshot.Member.add(event)
			} else {
				snapshot.NonMember.add(event)
			}
		}
	}
	sort.Strings(snapshot.ConsideredPackages)
	sort.Strings(snapshot.MemberEnteredPackages)
	sort.Strings(snapshot.NonMemberEnteredPackages)
	return snapshot
}

func appendScanObserverPackages(packages *[]string, packagePath string, count int) {
	for i := 0; i < count; i++ {
		*packages = append(*packages, packagePath)
	}
}

func (counts *scanWorkCounts) add(event scanObserverEvent) {
	switch event.Operation {
	case scanObserverSyntaxFiles:
		counts.SyntaxFiles += event.Count
	case scanObserverTypeInfoPackages:
		counts.TypeInfoPackages += event.Count
	case scanObserverUses:
		counts.Uses += event.Count
	case scanObserverSelections:
		counts.Selections += event.Count
	case scanObserverImportPackages:
		counts.ImportPackages += event.Count
	case scanObserverASTFiles:
		counts.ASTFiles += event.Count
	case scanObserverASTCommentGroups:
		counts.ASTCommentGroups += event.Count
	case scanObserverASTComments:
		counts.ASTComments += event.Count
	case scanObserverASTDeclarations:
		counts.ASTDeclarations += event.Count
	case scanObserverImportDeclarations:
		counts.ImportDeclarations += event.Count
	case scanObserverImportSpecs:
		counts.ImportSpecs += event.Count
	case scanObserverDeclaredFiles:
		counts.DeclaredFiles += event.Count
	case scanObserverAssemblyFiles:
		counts.AssemblyFiles += event.Count
	case scanObserverGoSourceFiles:
		counts.GoSourceFiles += event.Count
	case scanObserverParsedSyntaxFiles:
		counts.ParsedSyntaxFiles += event.Count
	case scanObserverLexedSourceFiles:
		counts.LexedSourceFiles += event.Count
	case scanObserverLexedSourceBytes:
		counts.LexedSourceBytes += event.Count
	case scanObserverSourceTokens:
		counts.SourceTokens += event.Count
	default:
		counts.Other += event.Count
	}
}

func sameScanObservationWork(left, right scanObservationSnapshot) bool {
	return sameScanWork(left.References, right.References) && sameScanWork(left.AnalysisDefeats, right.AnalysisDefeats)
}

func sameScanWork(left, right scanWorkSnapshot) bool {
	return reflect.DeepEqual(left.Member, right.Member) &&
		reflect.DeepEqual(left.NonMember, right.NonMember) &&
		len(left.MemberEnteredPackages) == len(right.MemberEnteredPackages) &&
		len(left.NonMemberEnteredPackages) == len(right.NonMemberEnteredPackages)
}

func scanObservationWorkToken(observation scanObservationSnapshot) string {
	return scanWorkSnapshotToken(observation.References) + ";" + scanWorkSnapshotToken(observation.AnalysisDefeats)
}

func scanWorkSnapshotToken(snapshot scanWorkSnapshot) string {
	return strings.Join([]string{
		strconv.Itoa(len(snapshot.ConsideredPackages)),
		strconv.Itoa(len(snapshot.MemberEnteredPackages)),
		strconv.Itoa(len(snapshot.NonMemberEnteredPackages)),
		scanWorkCountsToken(snapshot.Member),
		scanWorkCountsToken(snapshot.NonMember),
	}, ":")
}

func scanWorkCountsToken(counts scanWorkCounts) string {
	values := []int{
		counts.SyntaxFiles,
		counts.TypeInfoPackages,
		counts.Uses,
		counts.Selections,
		counts.ImportPackages,
		counts.ASTFiles,
		counts.ASTCommentGroups,
		counts.ASTComments,
		counts.ASTDeclarations,
		counts.ImportDeclarations,
		counts.ImportSpecs,
		counts.DeclaredFiles,
		counts.AssemblyFiles,
		counts.GoSourceFiles,
		counts.ParsedSyntaxFiles,
		counts.LexedSourceFiles,
		counts.LexedSourceBytes,
		counts.SourceTokens,
		counts.Other,
	}
	text := make([]string, len(values))
	for i, value := range values {
		text[i] = strconv.Itoa(value)
	}
	return strings.Join(text, ",")
}

func assertScanObservationMemberOnly(t testing.TB, label string, observation scanObservationSnapshot, memberPath string, wantConsidered int) {
	t.Helper()
	assertScanWorkMemberOnly(t, label+" typed", observation.References, memberPath, wantConsidered)
	assertScanWorkMemberOnly(t, label+" analysis-defeat", observation.AnalysisDefeats, memberPath, wantConsidered)
	if observation.References.Member.TypeInfoPackages != 1 || observation.References.Member.SyntaxFiles == 0 {
		t.Errorf("%s typed member syntax/type-info work = %#v, want one typed member and syntax", label, observation.References.Member)
	}
	if observation.References.Member.ImportPackages == 0 || observation.References.Member.ImportSpecs == 0 {
		t.Errorf("%s typed member import work = %#v, want at least one import package/spec", label, observation.References.Member)
	}
	if observation.AnalysisDefeats.Member.DeclaredFiles == 0 || observation.AnalysisDefeats.Member.GoSourceFiles == 0 || observation.AnalysisDefeats.Member.ASTFiles == 0 {
		t.Errorf("%s analysis-defeat member work = %#v, want declared Go AST work", label, observation.AnalysisDefeats.Member)
	}
}

func assertScanWorkMemberOnly(t testing.TB, label string, work scanWorkSnapshot, memberPath string, wantConsidered int) {
	t.Helper()
	if len(work.ConsideredPackages) != wantConsidered {
		t.Errorf("%s considered packages = %d, want %d", label, len(work.ConsideredPackages), wantConsidered)
	}
	if !reflect.DeepEqual(work.MemberEnteredPackages, []string{memberPath}) {
		t.Errorf("%s member entries = %v, want [%q]", label, work.MemberEnteredPackages, memberPath)
	}
	if len(work.NonMemberEnteredPackages) != 0 {
		t.Errorf("%s non-member entries = %v, want none", label, work.NonMemberEnteredPackages)
	}
	if work.NonMember != (scanWorkCounts{}) {
		t.Errorf("%s non-member work = %#v, want zero", label, work.NonMember)
	}
}

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

	ordered := snapshotScanObserver(observed)
	observed = nil
	if _, _, err := ScanReferences([]*packages.Package{pkgs[1], pkgs[0]}, members, root); err != nil {
		t.Fatalf("unexpected reordered observed reference scan error: %v", err)
	}
	if _, err := ScanAnalysisDefeats([]*packages.Package{pkgs[1], pkgs[0]}, members, root); err != nil {
		t.Fatalf("unexpected reordered observed analysis-defeat scan error: %v", err)
	}
	if got := snapshotScanObserver(observed); !reflect.DeepEqual(got, ordered) {
		t.Fatalf("reordering presented packages changed scan observation: got %#v, want %#v", got, ordered)
	}
}

func TestScanObserver_ReportsIgnoredMemberSourceWork(t *testing.T) {
	root := t.TempDir()
	memberPath := "example.com/member"
	closurePath := "example.com/closure"
	memberFile := filepath.Join(root, "member_ignored.go")
	closureFile := filepath.Join(root, "closure_ignored.go")
	memberSource := "package member\n\n//go:linkname hidden example.com/other.hidden\nfunc hidden()\n"
	closureSource := "package closure\n\n//go:linkname hidden example.com/other.hidden\nfunc hidden()\n"
	if err := os.WriteFile(memberFile, []byte(memberSource), 0o644); err != nil {
		t.Fatalf("writing ignored member fixture: %v", err)
	}
	if err := os.WriteFile(closureFile, []byte(closureSource), 0o644); err != nil {
		t.Fatalf("writing ignored non-member fixture: %v", err)
	}

	members, err := facts.NewMemberSet(memberPath)
	if err != nil {
		t.Fatalf("creating ignored-source member set: %v", err)
	}
	packages := []*packages.Package{
		{PkgPath: closurePath, IgnoredFiles: []string{closureFile}},
		{PkgPath: memberPath, IgnoredFiles: []string{memberFile}},
	}
	capture := &scanObserverCapture{}
	previous := scanObserver
	scanObserver = capture.observe
	defer func() { scanObserver = previous }()

	bypasses, err := ScanAnalysisDefeats(packages, members, root)
	if err != nil {
		t.Fatalf("unexpected ignored-source analysis-defeat scan error: %v", err)
	}
	if len(bypasses) != 1 || bypasses[0].Kind != facts.BypassLinkname || bypasses[0].Site.File != "member_ignored.go" {
		t.Fatalf("ignored-source bypasses = %+v, want one member linkname observation", bypasses)
	}
	snapshot := snapshotScanObserver(capture.events).AnalysisDefeats
	assertScanWorkMemberOnly(t, "ignored-source analysis-defeat", snapshot, memberPath, 2)
	if snapshot.Member.DeclaredFiles != 1 || snapshot.Member.GoSourceFiles != 1 || snapshot.Member.LexedSourceFiles != 1 || snapshot.Member.LexedSourceBytes != len(memberSource) || snapshot.Member.SourceTokens == 0 {
		t.Errorf("ignored-source lexical work = %#v, want one member source with bytes and tokens", snapshot.Member)
	}
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
