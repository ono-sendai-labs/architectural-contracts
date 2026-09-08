//go:build integration

package goanalysis_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
)

const (
	bypRoot   = "testdata/byp"
	bypMember = "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/byp/member/byp"
)

// hermeticLoadEnv pins the fixture load to CGO_ENABLED=0 so the cgo member
// file is observed through the declared ignored-file metadata path and the
// test never depends on a host C toolchain (or on a host that happens to
// enable cgo). The rest of the toolchain and module environment is inherited.
func hermeticLoadEnv() []string {
	return append(os.Environ(), "CGO_ENABLED=0")
}

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
		Env: hermeticLoadEnv(),
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
// the linkname directive, the assembly files (both the target-selected one
// and the one the current configuration ignores) and the cgo use of the
// member fixture at exact component-relative sites, from declared package
// metadata only. The cgo file is pinned to the ignored-file path: the
// hermetic CGO_ENABLED=0 load must present it there, or the fixture would
// silently stop exercising the configuration it describes.
func TestScanAnalysisDefeatsFindsEveryBypass(t *testing.T) {
	pkg, root := loadBypFixture(t)
	members, err := facts.NewMemberSet(bypMember)
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	if !containsFile(pkg.IgnoredFiles, "cgo.go") {
		t.Fatalf("the hermetic load must ignore the cgo source (declared metadata path), got ignored=%v", pkg.IgnoredFiles)
	}
	got, err := goanalysis.ScanAnalysisDefeats([]*packages.Package{pkg}, members, root)
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}
	want := []facts.BypassObservation{
		{Kind: facts.BypassCgo, Site: facts.SourceSite{File: "member/byp/cgo.go", Line: 4}},
		{Kind: facts.BypassLinkname, Site: facts.SourceSite{File: "member/byp/link.go", Line: 3}},
		{Kind: facts.BypassAssembly, Site: facts.SourceSite{File: "member/byp/stub.s", Line: 1}},
		{Kind: facts.BypassAssembly, Site: facts.SourceSite{File: "member/byp/stub_plan9.s", Line: 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bypass observations = %+v, want %+v", got, want)
	}
}

// TestScanAnalysisDefeatsMalformedIgnoredFileFailsClosed pins that a declared
// member file which does not parse as Go source fails the scan instead of
// yielding a partial-AST success that could miss a bypass outside the
// recoverable portion.
func TestScanAnalysisDefeatsMalformedIgnoredFileFailsClosed(t *testing.T) {
	root, err := filepath.Abs(bypRoot)
	if err != nil {
		t.Fatalf("failed to resolve byp fixture root: %v", err)
	}
	brokenMember := bypMember[:strings.LastIndex(bypMember, "/")] + "/broken"
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
		Env: hermeticLoadEnv(),
	}
	pkgs, err := packages.Load(cfg, brokenMember)
	if err != nil {
		t.Fatalf("failed to load broken member package: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("broken member package has load errors")
	}
	members, err := facts.NewMemberSet(brokenMember)
	if err != nil {
		t.Fatalf("invalid member set: %v", err)
	}
	got, err := goanalysis.ScanAnalysisDefeats(pkgs, members, root)
	if err == nil {
		t.Fatalf("malformed declared member file must fail the scan closed, got %+v", got)
	}
	if !strings.Contains(err.Error(), "ignored_bad.go") {
		t.Errorf("scan error must name the malformed file, got %v", err)
	}
}

func containsFile(files []string, name string) bool {
	for _, f := range files {
		if filepath.Base(f) == name {
			return true
		}
	}
	return false
}
