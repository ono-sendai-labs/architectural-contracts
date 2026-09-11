//go:build cgo

package defeat

/*
#include <stdlib.h>
*/
import "C"

// FromC deliberately remains behind the cgo constraint. The pure Bazel target
// must not type-check it, but the defeat scan must still report its cgo import.
func FromC() int { return int(C.rand()) }
