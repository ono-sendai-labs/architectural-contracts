// Package initimporter imports initauth. Its own code touches no authority, but
// initimporter.init calls initauth.init (Go package-init ordering), so Capslock
// attributes READ_SYSTEM_STATE to initimporter unless the dependency's init is
// pruned (review A2).
package initimporter

import "github.com/xtofian/architectural-contracts/spike/probes/initauth"

// Get returns the value captured by initauth's init.
func Get() string { return initauth.Value() }
