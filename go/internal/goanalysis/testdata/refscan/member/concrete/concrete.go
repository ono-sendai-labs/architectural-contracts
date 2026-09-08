package concrete

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"
)

func UseImpl() string {
	i := dep.Impl{}
	return i.Greet()
}
