package scope

import (
	"os"
)

// Clean is a safe entrypoint that has no capability use.
func Clean() int {
	return 42
}

// Unreached is an exported helper that is not reachable from Clean,
// but because it is part of the package, its capability should be reported.
func Unreached() ([]byte, error) {
	return os.ReadFile("testdata/pure/pure.go")
}
