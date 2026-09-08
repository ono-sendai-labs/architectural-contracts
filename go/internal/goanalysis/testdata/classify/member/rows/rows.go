// Package rows is the classify fixture's member package for design fixture 6,
// the import table: one import of each row's written path — a member package,
// a stdlib-map blank import whose aggregate init carries authority (fixture
// 2), a stdlib-map blank import whose aggregate init is UNANALYZED, a blank
// import of a declared dependency package, a blank import of an auto-attached
// infra dependency package, unowned packages with real type data (named and
// blank), and a blank stdlib import whose loader entry is nil-ed out by the
// missing-type-data test before scanning.
package rows

import (
	"errors"
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/infra/infra"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/member/globals"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"

	_ "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep/sub"
	_ "go/ast"    // fixture 6 row 2 (UNANALYZED init): analysis-defeating aggregate init
	_ "image/png" // fixture 2: blank import of a map-enumerated package
	_ "os"        // fixture 6 row 7: nil-entry seam turns this into missing type data
	_ "strconv"   // fixture 6 row 6: unowned resolved package, blank import
)

func Use() {
	_ = dep.NewGreeter() // the declared dependency's listed interface object
	_ = sort.Ints        // unowned resolved package: UNDECLARED_DEPENDENCY
	_ = errors.Is        // unowned resolved package: UNDECLARED_DEPENDENCY
	_ = infra.Infra
	_ = globals.ReadConfig
}
