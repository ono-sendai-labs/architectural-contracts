package filereader

import (
	"os"
)

// ReadSomeFile reads a file from the ambient filesystem, exercising FILES capability.
func ReadSomeFile() ([]byte, error) {
	return os.ReadFile("testdata/pure/pure.go")
}
