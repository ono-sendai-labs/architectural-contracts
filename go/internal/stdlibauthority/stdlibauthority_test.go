package stdlibauthority_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// TestValidateClassification pins the terminal-classification contract (DR-05):
// exactly one of SAFE, a non-empty sorted capability set, or UNANALYZED is
// valid; every ambiguous or empty state is rejected.
func TestValidateClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		class   stdlibauthority.Classification
		wantErr string
	}{
		{"safe", stdlibauthority.Classification{Safe: true}, ""},
		{"capabilities", stdlibauthority.Classification{Capabilities: []stdlibauthority.Capability{"FILES"}}, ""},
		{"unanalyzed", stdlibauthority.Classification{Unanalyzed: true}, ""},
		{"empty zero value", stdlibauthority.Classification{}, "no terminal state"},
		{"safe and capabilities", stdlibauthority.Classification{Safe: true, Capabilities: []stdlibauthority.Capability{"FILES"}}, "exactly one"},
		{"safe and unanalyzed", stdlibauthority.Classification{Safe: true, Unanalyzed: true}, "exactly one"},
		{"capabilities and unanalyzed", stdlibauthority.Classification{Unanalyzed: true, Capabilities: []stdlibauthority.Capability{"FILES"}}, "exactly one"},
		{"empty capability set", stdlibauthority.Classification{Capabilities: []stdlibauthority.Capability{}}, "no terminal state"},
		{"unsorted capabilities", stdlibauthority.Classification{Capabilities: []stdlibauthority.Capability{"READ_SYSTEM_STATE", "FILES"}}, "sorted"},
		{"duplicate capabilities", stdlibauthority.Classification{Capabilities: []stdlibauthority.Capability{"FILES", "FILES"}}, "duplicate-free"},
		{"all three", stdlibauthority.Classification{Safe: true, Unanalyzed: true, Capabilities: []stdlibauthority.Capability{"FILES"}}, "exactly one"},
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
	base := stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		CgoEnabled:       true,
		BuildTags:        []string{"a", "b"},
		GOEXPERIMENT:     "none",
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
	if fields := stdlibauthority.EqualKeys(base, base); len(fields) != 0 {
		t.Errorf("stdlibauthority.EqualKeys(base, base) = %v, want empty", fields)
	}

	for _, tc := range []struct {
		name  string
		mut   func(*stdlibauthority.SDKKey)
		field string
	}{
		{"toolchain", func(k *stdlibauthority.SDKKey) { k.ToolchainVersion = "go1.26.5" }, "toolchain_version"},
		{"goos", func(k *stdlibauthority.SDKKey) { k.GOOS = "darwin" }, "goos"},
		{"goarch", func(k *stdlibauthority.SDKKey) { k.GOARCH = "arm64" }, "goarch"},
		{"cgo", func(k *stdlibauthority.SDKKey) { k.CgoEnabled = false }, "cgo_enabled"},
		{"build tags", func(k *stdlibauthority.SDKKey) { k.BuildTags = []string{"a", "c"} }, "build_tags"},
		{"goexperiment", func(k *stdlibauthority.SDKKey) { k.GOEXPERIMENT = "greenteagc" }, "goexperiment"},
		{"classifier hash", func(k *stdlibauthority.SDKKey) { k.ClassifierHash = "def456" }, "classifier_hash"},
		{"map format", func(k *stdlibauthority.SDKKey) { k.MapFormatVersion = 2 }, "map_format_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := base
			tc.mut(&other)
			fields := stdlibauthority.EqualKeys(base, other)
			if len(fields) != 1 || fields[0] != tc.field {
				t.Errorf("EqualKeys = %v, want [%s]", fields, tc.field)
			}
			if !errors.Is(&stdlibauthority.KeyMismatchError{Fields: fields}, stdlibauthority.ErrKeyMismatch) {
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
