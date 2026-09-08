// Package rows is the classify fixture's import-table member package for
// design fixture 6: every import row is a genuine blank import with no
// corresponding object use — a member package, two packages of the declared
// dependency, the auto-attached infra dependency, unowned packages with real
// type data, stdlib-map packages whose aggregate inits carry authority
// (image/png, fixture 2) and are UNANALYZED (go/ast), and a stdlib import
// (os) whose loader entry is nil-ed out by the missing-type-data test before
// scanning. The named-import reference fixture lives in member/uses.
package rows

import (
	_ "errors"
	_ "sort"
	_ "strconv"

	_ "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/infra/infra"
	_ "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/member/globals"
	_ "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"
	_ "github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep/sub"

	_ "go/ast"    // fixture 6 row 2 (UNANALYZED init): analysis-defeating aggregate init
	_ "image/png" // fixture 2: blank import of a map-enumerated package
	_ "os"        // fixture 6 row 7: nil-entry seam turns this into missing type data
)

// Use keeps the package non-empty without touching any imported object.
func Use() {}
