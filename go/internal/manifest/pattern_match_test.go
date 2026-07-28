package manifest_test

import (
	"encoding/json"
	"os"
	"path"
	"testing"
)

type patternTestCase struct {
	Pattern   string `json:"pattern"`
	Path      string `json:"path"`
	Want      bool   `json:"want"`
	Malformed bool   `json:"malformed"`
}

func loadSharedPatternCases(t *testing.T) []patternTestCase {
	t.Helper()
	// Try paths from go package directory up to repo root
	candidates := []string{
		"../../../bazel_rules/go/tests/pattern_cases.json",
		"../../bazel_rules/go/tests/pattern_cases.json",
		"bazel_rules/go/tests/pattern_cases.json",
	}
	var data []byte
	var err error
	for _, cand := range candidates {
		data, err = os.ReadFile(cand)
		if err == nil {
			break
		}
	}
	if err != nil {
		// Fallback: search relative to repo root
		wd, _ := os.Getwd()
		t.Fatalf("could not read pattern_cases.json (wd=%s): %v", wd, err)
	}

	var cases []patternTestCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to unmarshal pattern_cases.json: %v", err)
	}
	return cases
}

func TestSharedPatternCases_GoMatch(t *testing.T) {
	cases := loadSharedPatternCases(t)
	for _, tc := range cases {
		got, err := path.Match(tc.Pattern, tc.Path)
		if tc.Malformed {
			if err != path.ErrBadPattern {
				t.Errorf("path.Match(%q, %q) err = %v, want ErrBadPattern", tc.Pattern, tc.Path, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("path.Match(%q, %q) returned unexpected error: %v", tc.Pattern, tc.Path, err)
			continue
		}
		if got != tc.Want {
			t.Errorf("path.Match(%q, %q) = %v, want %v", tc.Pattern, tc.Path, got, tc.Want)
		}
	}
}
