// Package readerdirtyshell feeds readerattr.Parse an *os.File — the case the
// design forbids for manifest.Parse. Analyzing {readerattr, readerdirtyshell}
// should make VTA flow *os.File into Parse's io.Reader and attribute FILES,
// demonstrating why the shell must wrap the manifest in a bytes.Reader.
package readerdirtyshell

import (
	"os"

	"github.com/xtofian/architectural-contracts/spike/probes/readerattr"
)

// Run opens a file and hands the *os.File to Parse; FILES flows through.
func Run(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readerattr.Parse(f)
}
