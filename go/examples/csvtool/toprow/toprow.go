// Package toprow selects the top N rows of a CSV table, ordered by a column.
//
// Contract (informal — FR10):
//   - does:      parses in-memory CSV text (via the parsecsv helper across a component boundary)
//     and/or sorts already-parsed rows, returning the first N. Pure computation.
//   - requires:  CSV text or rows supplied by the caller. No I/O, no filesystem.
//   - provides:  TopN, Pick — deterministic, descending order by column.
//   - authority: NONE. This component holds no ambient authority. parsecsv is resolved
//     across a component boundary, and parsecsv touches no filesystem, so toprow stays
//     ambient-authority-free (FR7).
package toprow

import (
	"cmp"
	"slices"

	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"
)

// Pick sorts already-parsed rows descending by col and returns the first n.
// The stdlib authority map classifies slices.SortFunc and cmp.Compare as
// SAFE (proved-pure), so the sort stays inside the strict-safe stdlib
// envelope; sort.Sort/sort.Slice are UNANALYZED in the map and would be
// analysis-defeating findings under the typed reference check.
func Pick(rows [][]string, col, n int) [][]string {
	slices.SortFunc(rows, func(a, b []string) int { return cmp.Compare(b[col], a[col]) })
	if n > len(rows) {
		n = len(rows)
	}
	return rows[:n]
}

// TopN parses CSV text (via the parsecsv component-boundary dependency) and returns the
// top n rows by col.
func TopN(csvText string, col, n int) ([][]string, error) {
	rows, err := parsecsv.Parse(csvText)
	if err != nil {
		return nil, err
	}
	return Pick(rows, col, n), nil
}
