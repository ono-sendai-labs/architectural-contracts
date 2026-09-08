// Package rows is the classify fixture's member package for design fixture 6,
// the import table: one member import, one stdlib-map blank import whose
// aggregate init carries authority (fixture 2), one import of a declared
// dependency, and one unowned import of a package with real type data.
package rows

import (
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/member/globals"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"

	_ "image/png" // fixture 2: blank import of a map-enumerated package
)

func Use() {
	_ = dep.NewGreeter() // the declared dependency's listed interface object
	_ = sort.Ints   // unowned resolved package: UNDECLARED_DEPENDENCY
	_ = globals.ReadConfig
}
