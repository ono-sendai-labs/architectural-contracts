//go:build windows

package defeat

import _ "unsafe"

//go:linkname hidden runtime.nanotime1
var hidden int64
