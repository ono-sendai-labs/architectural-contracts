package scope_test

import (
	"os"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/spike/probes/scope"
)

// externalTestAuthority uses FILES from an external (_test package) test file —
// the second scope-exclusion case. Must not be reported either.
func externalTestAuthority() ([]byte, error) { return os.ReadFile("/etc/hosts") }

func TestExternalScope(t *testing.T) {
	_ = scope.Clean()
	_, _ = externalTestAuthority()
}
