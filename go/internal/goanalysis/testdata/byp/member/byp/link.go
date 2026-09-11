package byp

import _ "unsafe"

//go:linkname Local runtime.nanotime1

func Local() int { return 1 }
