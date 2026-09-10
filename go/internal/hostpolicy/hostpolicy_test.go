package hostpolicy_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

func TestCanonicalizePathDefaultIsIdentity(t *testing.T) {
	for _, p := range []string{
		"",
		"fmt",
		"example.com/aspect/core",
		"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker",
	} {
		if got := hostpolicy.CanonicalizePath(p); got != p {
			t.Errorf("CanonicalizePath(%q) = %q, want identity %q", p, got, p)
		}
	}
}
