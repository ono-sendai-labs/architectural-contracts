package stdlibauthority

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateClassification pins the terminal-classification contract (DR-05):
// exactly one of SAFE, a non-empty sorted capability set, or UNANALYZED is
// valid; every ambiguous or empty state is rejected.
func TestValidateClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		class   Classification
		wantErr string
	}{
		{"safe", Classification{Safe: true}, ""},
		{"capabilities", Classification{Capabilities: []Capability{"FILES"}}, ""},
		{"unanalyzed", Classification{Unanalyzed: true}, ""},
		{"empty zero value", Classification{}, "no terminal state"},
		{"safe and capabilities", Classification{Safe: true, Capabilities: []Capability{"FILES"}}, "exactly one"},
		{"safe and unanalyzed", Classification{Safe: true, Unanalyzed: true}, "exactly one"},
		{"capabilities and unanalyzed", Classification{Unanalyzed: true, Capabilities: []Capability{"FILES"}}, "exactly one"},
		{"empty capability set", Classification{Capabilities: []Capability{}}, "no terminal state"},
		{"all three", Classification{Safe: true, Unanalyzed: true, Capabilities: []Capability{"FILES"}}, "exactly one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.class.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestSDKKeyEqualKeysAndString pins SDKKey's deterministic identity behavior:
// EqualKeys lists exactly the mismatched fields, equal keys list nothing, and
// String is deterministic over sorted build tags.
func TestSDKKeyEqualKeysAndString(t *testing.T) {
	base := SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		CgoEnabled:       true,
		BuildTags:        []string{"a", "b"},
		GOEXPERIMENT:     "none",
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
	if fields := EqualKeys(base, base); len(fields) != 0 {
		t.Errorf("EqualKeys(base, base) = %v, want empty", fields)
	}

	for _, tc := range []struct {
		name  string
		mut   func(*SDKKey)
		field string
	}{
		{"toolchain", func(k *SDKKey) { k.ToolchainVersion = "go1.26.5" }, "toolchain_version"},
		{"goos", func(k *SDKKey) { k.GOOS = "darwin" }, "goos"},
		{"goarch", func(k *SDKKey) { k.GOARCH = "arm64" }, "goarch"},
		{"cgo", func(k *SDKKey) { k.CgoEnabled = false }, "cgo_enabled"},
		{"build tags", func(k *SDKKey) { k.BuildTags = []string{"a", "c"} }, "build_tags"},
		{"goexperiment", func(k *SDKKey) { k.GOEXPERIMENT = "greenteagc" }, "goexperiment"},
		{"classifier hash", func(k *SDKKey) { k.ClassifierHash = "def456" }, "classifier_hash"},
		{"map format", func(k *SDKKey) { k.MapFormatVersion = 2 }, "map_format_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			tc.mut(&other)
			fields := EqualKeys(base, other)
			if len(fields) != 1 || fields[0] != tc.field {
				t.Errorf("EqualKeys = %v, want [%s]", fields, tc.field)
			}
			if !errors.Is(&KeyMismatchError{Fields: fields}, ErrKeyMismatch) {
				t.Errorf("fields %v do not report as ErrKeyMismatch", fields)
			}
		})
	}

	// String is deterministic and order-insensitive over build tags: two keys
	// differing only in tag order render identically.
	reordered := base
	reordered.BuildTags = []string{"b", "a"}
	if base.String() != reordered.String() {
		t.Errorf("String differs on tag order:\n%s\n%s", base.String(), reordered.String())
	}
	if s := base.String(); !strings.Contains(s, "go1.26.4") || !strings.Contains(s, "linux") || !strings.Contains(s, "amd64") {
		t.Errorf("String %q does not name the key fields", s)
	}
}
