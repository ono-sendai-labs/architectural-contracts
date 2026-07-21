// Package api_test compiles against the component target rather than the
// go_library, which only works if _go_component forwards GoInfo and GoArchive
// from its interface library.
package api_test

import (
	"testing"

	"example.com/aspect/api"
)

func TestGreetThroughComponentTarget(t *testing.T) {
	if got, want := api.Greet(" hi "), "[[hi]]"; got != want {
		t.Errorf("api.Greet(%q) = %q, want %q", " hi ", got, want)
	}
}
