package dispatch

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"
)

func UseGreeter() string {
	g := dep.NewGreeter()
	return g.Greet()
}
