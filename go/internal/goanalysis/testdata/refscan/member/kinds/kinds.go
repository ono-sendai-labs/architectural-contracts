package kinds

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"
	. "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep/sub"
)

import _ "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep/initpkg"

func UseAll() int {
	b := dep.Base{}
	_ = b.Name
	_ = dep.ExportedVar
	_ = dep.ExportedConst
	var x dep.Plain
	var a dep.Alias = b
	_ = a.Name
	o := dep.Outer{}
	_ = o.Name
	_ = dep.Box[int]{}
	bx := dep.Box[string]{}
	_ = bx.Get()
	p := &dep.Ptr{}
	p.Touch()
	f := dep.Free
	_ = f
	_ = len(b.Name)
	return int(x) + Sub()
}
