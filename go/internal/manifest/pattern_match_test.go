package manifest_test

import (
	"path"
	"testing"
)

type patternTestCase struct {
	pattern string
	path    string
	want    bool
}

var sharedPatternCases = []patternTestCase{
	{"example.com/foo", "example.com/foo", true},
	{"example.com/foo", "example.com/bar", false},
	{"example.com/foo/*", "example.com/foo/bar", true},
	{"example.com/foo/*", "example.com/foo/bar/baz", false},
	{"example.com/foo/*", "example.com/foo", false},
	{"example.com/foo/b*", "example.com/foo/bar", true},
	{"example.com/foo/b*", "example.com/foo/car", false},
	{"example.com/foo/ba?", "example.com/foo/bar", true},
	{"example.com/foo/ba?", "example.com/foo/b", false},
	{"example.com/foo/ba?", "example.com/foo/barr", false},
	{"example.com/foo/ba?", "example.com/foo/ba/", false},
	{"*.com/*/*", "example.com/foo/bar", true},
	{"*.com/*/*", "example.com/foo/bar/baz", false},
	{"", "", true},
	{"", "a", false},
	{"a", "", false},
	{"example.com/foo*", "example.com/foobar", true},
	{"example.com/foo*", "example.com/foo/bar", false},
	{"example.com/foo/\\*", "example.com/foo/*", true},
	{"example.com/foo/\\*", "example.com/foo/bar", false},
	{"example.com/v[0-9]", "example.com/v1", true},
	{"example.com/v[0-9]", "example.com/va", false},
}

func TestSharedPatternCases_GoMatch(t *testing.T) {
	for _, tc := range sharedPatternCases {
		got, err := path.Match(tc.pattern, tc.path)
		if err != nil {
			t.Errorf("path.Match(%q, %q) returned unexpected error: %v", tc.pattern, tc.path, err)
			continue
		}
		if got != tc.want {
			t.Errorf("path.Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}
