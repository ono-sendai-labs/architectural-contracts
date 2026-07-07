// Package readerattr models a component function that takes an io.Reader — the
// shape of manifest.Parse(io.Reader). It holds no authority of its own; whether
// Capslock attributes FILES to Parse depends entirely on which concrete type the
// *shell* feeds in (design §11). The two shell packages readercleanshell and
// readerdirtyshell exercise the two cases; each is analyzed in its own package
// load so VTA sees only one concrete io.Reader type.
package readerattr

import "io"

// Parse reads everything from r. Authority-free in itself.
func Parse(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
