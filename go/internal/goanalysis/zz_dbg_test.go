package goanalysis_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestZZDebugLoad(t *testing.T) {
	for _, dir := range []string{"testdata/classify", "testdata/refscan"} {
		root, _ := filepath.Abs(dir)
		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
				packages.NeedImports | packages.NeedDeps | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
			Dir: root,
		}
		patterns := []string{"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/member/globals"}
		pkgs, err := packages.Load(cfg, patterns...)
		fmt.Println(dir, "err:", err, "n:", len(pkgs))
		for _, p := range pkgs {
			fmt.Println("  ", p.PkgPath, len(p.Errors), len(p.GoFiles))
			for _, e := range p.Errors {
				fmt.Println("    ", e.Msg)
			}
		}
	}
}
