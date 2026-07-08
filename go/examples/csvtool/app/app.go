// Package app is the composition root of the csvtool example.
//
// Contract (informal — FR10):
//   - does:      reads a CSV file (via the csvfile component) and prints its top
//     rows (via the toprow component).
//   - requires:  a path to a CSV file.
//   - provides:  Run.
//   - authority: NONE — this is the FR5b showcase. app depends on csvfile as a
//     *component dependency*; Capslock traversal is pruned at csvfile's declared
//     interface (Read), so the FILES authority csvfile holds is NOT re-absorbed
//     into app. app orchestrates a file-reading component yet checks
//     ambient-authority-free.
//
// NOTE (design intent): this structure is *intentionally convoluted* to
// demonstrate the attribution of ambient FILES authority to a dependent
// component. A cleaner, idiomatic design would open the file in the shell and
// pass the resulting reader into parsecsv — leaving app authority-free without
// relying on boundary pruning at all. app instead composes the FILES-holding
// csvfile component on purpose, so the example can exercise FR5b pruning.
//
// NOTE (Step 0 draft): the actual pruning is wired in Step 9. Un-pruned, this
// package is attributed FILES transitively through csvfile.Read; that is exactly
// what the spike measures.
package app

import (
	"fmt"

	"github.com/xtofian/architectural-contracts/go/examples/csvtool/csvfile"
	"github.com/xtofian/architectural-contracts/go/examples/csvtool/toprow"
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
