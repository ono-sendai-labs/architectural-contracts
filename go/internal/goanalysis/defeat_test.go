//go:build integration

package goanalysis_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
)

const (
	bypRoot   = "testdata/byp"
	bypMember = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/byp/member/byp"
)

// loadBypFixture loads the analysis-defeating member package.
func loadBypFixture(t *testing.T) (*packages.Package, string) {
	t.Helper()
	root, err := filepath.Abs(bypRoot)
	if err != nil {
		t.Fatalf("failed to resolve byp fixture root: %v", err)
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, bypMember)
	if err != nil {
		t.Fatalf("failed to load byp member package: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("byp member package has load errors")
	}
	return pkgs[0], root
}

// TestScanAnalysisDefeatsFindsEveryBypass pins that the defeat scan observes
// the linkname directive, the assembly file and the cgo use of the member
// fixture at exact component-relative sites, from declared package metadata
// only.
func TestScanAnalysisDefeatsFindsEveryBypass(t *testing.T) {
	pkg, root := loadBypFixture(t)
	members, err := facts.NewMemberSet(bypMember)
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	got, err := goanalysis.ScanAnalysisDefeats([]*packages.Package{pkg}, members, root)
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}
	want := []facts.BypassObservation{
		{Kind: facts.BypassCgo, Site: facts.SourceSite{File: "member/byp/cgo.go", Line: 4}},
		{Kind: facts.BypassLinkname, Site: facts.SourceSite{File: "member/byp/link.go", Line: 3}},
		{Kind: facts.BypassAssembly, Site: facts.SourceSite{File: "member/byp/stub.s", Line: 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bypass observations = %+v, want %+v", got, want)
	}
}
