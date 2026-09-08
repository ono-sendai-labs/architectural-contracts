// Package uses is the classify fixture's reference member package: the named
// imports and object references that exercise the reference dispatch against
// declared, infra, member, and unowned packages alongside the import-only
// blank-import table in member/rows (design fixture 6 and AC6's
// import-vs-reference separation in fixture form).
package uses

import (
	"errors"
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/infra/infra"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/classify/member/globals"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"
)

func Use() {
	_ = dep.NewGreeter() // the declared dependency's listed interface object
	_ = sort.Ints        // unowned resolved package: UNDECLARED_DEPENDENCY
	_ = errors.Is        // unowned resolved package: UNDECLARED_DEPENDENCY
	_ = infra.Infra
	_ = globals.ReadConfig
}
