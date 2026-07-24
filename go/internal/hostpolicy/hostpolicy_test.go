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

func TestIsStdlibPathDefaultHeuristic(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"", false},
		{"fmt", true},
		{"net/http", true},
		{"os", true},
		{"example.com/aspect/core", false},
		{"github.com/foo/bar", false},
	}
	for _, tt := range tests {
		if got := hostpolicy.IsStdlibPath(tt.path); got != tt.want {
			t.Errorf("IsStdlibPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
