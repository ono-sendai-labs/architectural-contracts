// Package app is the composition root of the csvtool example.
//
// Contract (informal — FR10):
//   - does:      reads a CSV file (via the csvfile component) and prints its top
//     rows (via the toprow component).
//   - requires:  a path to a CSV file.
//   - provides:  Run.
//   - authority: NONE — this is the FR5b showcase. app depends on csvfile as a
//     *component dependency*; the declared component boundary ends authority at
//     csvfile's Read interface, so csvfile's FILES authority stays behind the
//     boundary. app orchestrates a file-reading component yet checks
//     ambient-authority-free.
//
// NOTE (design intent): this structure is *intentionally convoluted* to
// demonstrate the attribution of ambient FILES authority to a dependent
// component. A cleaner, idiomatic design would open the file in the shell and
// pass the resulting reader into parsecsv — leaving app authority-free without
// relying on a component boundary. app instead composes the FILES-holding
// csvfile component on purpose, so the example can exercise FR5b's structural
// boundary rule.
//
// The checker scans app's typed references and imports, then treats csvfile's
// declared component boundary as the authority boundary; it does not traverse
// csvfile's implementation.
package app

import (
	"fmt"

	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/csvfile"
	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/toprow"
)

// Run reads the CSV at path and prints its top 3 rows by the first column.
func Run(path string) error {
	rows, err := csvfile.Read(path)
	if err != nil {
		return err
	}
	for _, r := range toprow.Pick(rows, 0, 3) {
		fmt.Println(r)
	}
	return nil
}
