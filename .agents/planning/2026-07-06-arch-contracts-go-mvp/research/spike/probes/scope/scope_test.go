package scope

import (
	"os"
	"testing"
)

// inTestAuthority uses FILES from an in-package _test.go file. If Capslock's load
// included test files, this would surface FILES; the spike confirms it does not.
func inTestAuthority() ([]byte, error) { return os.ReadFile("/etc/passwd") }

func TestScope(t *testing.T) {
	if _, err := inTestAuthority(); err != nil {
		t.Skip("expected on hosts without /etc/passwd")
	}
	_ = Clean()
}
