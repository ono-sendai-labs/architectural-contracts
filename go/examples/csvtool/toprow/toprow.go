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
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"
)

// byColumn is a concrete sort.Interface. The Step-0 spike confirmed sort.Sort
// and sort.Slice are both acceptable under the strict-safe stdlib envelope.
type byColumn struct {
	rows [][]string
	col  int
}

func (b byColumn) Len() int           { return len(b.rows) }
func (b byColumn) Less(i, j int) bool { return b.rows[i][b.col] > b.rows[j][b.col] }
func (b byColumn) Swap(i, j int)      { b.rows[i], b.rows[j] = b.rows[j], b.rows[i] }

// Pick sorts already-parsed rows descending by col and returns the first n.
func Pick(rows [][]string, col, n int) [][]string {
	sort.Sort(byColumn{rows: rows, col: col})
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
