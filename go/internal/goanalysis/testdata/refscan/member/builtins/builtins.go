package builtins

import (
	"fmt"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/refscan/dep"
)

func Loops(items []string) {
	for i := range items {
		if i < 0 {
			continue
		}
		fmt.Println(dep.Hello())
	}
Loop:
	for range items {
		break Loop
	}
	var err error
	_ = err
}
