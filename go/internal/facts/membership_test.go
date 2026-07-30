package facts_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func TestMatchesMember(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{name: "literal", pattern: "example.com/app", path: "example.com/app", want: true},
		{name: "literal mismatch", pattern: "example.com/app", path: "example.com/other", want: false},
		{name: "pattern", pattern: "example.com/app/*", path: "example.com/app/internal", want: true},
		{name: "pattern mismatch", pattern: "example.com/app/*", path: "example.com/other", want: false},
		{name: "malformed pattern", pattern: "example.com/[", path: "example.com/app", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := facts.MatchesMember(tt.pattern, tt.path); got != tt.want {
				t.Fatalf("MatchesMember(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
