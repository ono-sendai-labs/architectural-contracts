// Package parsecsv is an absorbed implementation-detail dependency: it wraps
// encoding/csv to turn in-memory CSV text into rows. It has no manifest of its
// own — a component that uses it lists it as an absorbed_dependency, and any
// ambient authority it reached would be absorbed into (and surfaced by) that
// component (FR7). It reaches none: it only parses an in-memory string.
package parsecsv

import (
	"encoding/csv"
	"strings"
)

// Parse turns CSV text into rows. It reads from an in-memory strings.Reader, so
// it touches no filesystem and holds no ambient authority.
func Parse(text string) ([][]string, error) {
	return csv.NewReader(strings.NewReader(text)).ReadAll()
}
