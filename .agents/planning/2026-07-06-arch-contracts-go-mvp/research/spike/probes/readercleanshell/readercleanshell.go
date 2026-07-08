// Package readercleanshell feeds readerattr.Parse an in-memory bytes.Reader — the
// authority-free path (what the real shell does for manifest.Parse). Analyzing
// {readerattr, readercleanshell} should leave Parse FILES-free.
package readercleanshell

import (
	"bytes"

	"github.com/ono-sendai-labs/architectural-contracts/spike/probes/readerattr"
)

// Run parses in-memory data; only a *bytes.Reader ever reaches Parse.
func Run(data []byte) ([]byte, error) {
	return readerattr.Parse(bytes.NewReader(data))
}
