// Package initauth has an explicit func init() that uses ambient authority
// (reads an env var → READ_SYSTEM_STATE). It is imported by initimporter to test
// review-A2: init authority in a dependency is attributed to the importer unless
// `func <pkg>.init` is pruned.
package initauth

import "os"

var envVal string

func init() { envVal = os.Getenv("SPIKE_ENV") }

// Value returns the captured env value. It uses no authority itself; the
// authority lives entirely in init.
func Value() string { return envVal }
