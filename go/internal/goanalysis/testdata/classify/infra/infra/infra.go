// Package infra is the classify fixture's auto-attached infra dependency
// package: rows.go blank-imports it, and the fixture classification marks the
// owning infra dependency used without any symbol-level check.
package infra

// Infra does nothing; the package exists to be owned by an infra dependency
// surface and imported by the rows member.
func Infra() {}
