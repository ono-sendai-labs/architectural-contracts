// Package violation opens a file, using FILES ambient authority. Its component
// declares no authority, so its check must fail — this is the negative fixture
// for arcc_check_test.
package violation

import "os"

// Read mints FILES authority via os.Open.
func Read() (*os.File, error) {
	return os.Open("/etc/hostname")
}
