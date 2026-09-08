// Package globals is the classify fixture's member package for design
// fixtures 1 and 9: a stdlib function value taken but never called, and
// stdlib global variables whose authority is exactly the map's record.
package globals

import (
	"io"
	"os"
)

func ReadConfig() error {
	f := os.ReadFile // fixture 1: func value taken, never called
	_ = f
	_ = io.EOF   // fixture 9: SAFE const contributes no authority
	_ = os.Stdin // fixture 9: FILES var carries its map classification
	return nil
}
