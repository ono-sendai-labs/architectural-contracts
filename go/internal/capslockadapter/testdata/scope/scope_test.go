package scope

import (
	"os"
	"testing"
)

// TestSomething is a test function that accesses system environment variables.
// It should be excluded from capability analysis.
func TestSomething(t *testing.T) {
	_ = os.Getenv("SOME_VAR")
}
