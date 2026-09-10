// Package csvfile is a component that legitimately holds the FILES capability:
// it reads a named CSV file from the real filesystem.
//
// Contract (informal — FR10):
//   - does:      reads the file at the given path and parses it into rows.
//   - requires:  the path names a readable CSV file on the real filesystem.
//   - provides:  Read and ReadWithCallback.
//   - authority: FILES. This component declares FILES in its manifest; checked on
//     its own it conforms (FILES is declared). A component that depends on csvfile
//     as a *component dependency* stops at the declared Read boundary and is
//     therefore NOT re-attributed FILES (FR5b) — csvfile owns that authority
//     behind its contract.
package csvfile

import (
	"os"

	"github.com/ono-sendai-labs/architectural-contracts/go/examples/csvtool/internal/parsecsv"
)

// Read reads the CSV file at path from the real filesystem and parses it.
func Read(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsecsv.Parse(string(data))
}

// ReadWithCallback reads the CSV file at path and processes it with a callback.
//
// Contract (informal — FR10):
//   - does:      reads the file at the given path, parses it, and calls callback with the rows.
//   - requires:  the path names a readable CSV file on the real filesystem.
//   - provides:  ReadWithCallback.
//   - authority: FILES.
func ReadWithCallback(path string, cb func([][]string)) error {
	rows, err := Read(path)
	if err != nil {
		return err
	}
	cb(rows)
	return nil
}
