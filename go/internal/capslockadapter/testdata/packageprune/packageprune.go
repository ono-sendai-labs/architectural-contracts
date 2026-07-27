package packageprune

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter/testdata/filereader"
)

// DoWork calls a function in filereader package.
func DoWork() ([]byte, error) {
	return filereader.ReadSomeFile()
}
