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
	} {
		if got := hostpolicy.CanonicalizePath(p); got != p {
			t.Errorf("CanonicalizePath(%q) = %q, want identity %q", p, got, p)
		}
	}

	const modulePath = "github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	if hostpolicy.NamespaceID != "upstream" {
		t.Logf("skipping module-path identity assertion under host namespace %q", hostpolicy.NamespaceID)
		return
	}
	if got := hostpolicy.CanonicalizePath(modulePath); got != modulePath {
		t.Errorf("CanonicalizePath(%q) = %q, want identity %q", modulePath, got, modulePath)
	}
}
