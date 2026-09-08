package byp

// #include <stdlib.h>
import "C"

// FromC returns a value from the ambient C runtime.
func FromC() int { return int(C.rand()) }
