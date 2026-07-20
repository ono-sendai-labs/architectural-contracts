// Package clean is a fixture component that performs pure computation and
// exercises no ambient authority, so Capslock should report no capabilities.
package clean

// Add is pure computation.
func Add(a, b int) int { return a + b }
