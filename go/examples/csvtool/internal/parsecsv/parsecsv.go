// Package parsecsv is a shared parser utility component: it wraps encoding/csv
// to turn in-memory CSV text into rows. It is declared as its own component, and
// other components depend on it across a component boundary rather than absorbing it.
// It reaches no ambient authority, only parsing in-memory strings.
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
