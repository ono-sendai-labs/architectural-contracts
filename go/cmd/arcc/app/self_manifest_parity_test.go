package app_test

import (
	"flag"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifestparity"
)

// TestSelfManifestParity checks that the generated manifests for arcc's own
// components agree with their checked-in component.textproto counterparts.
//
// The test covers the components that have go_component targets: capanalyzer,
// report, facts, manifest, checker, goanalysis, artifactio, and schema.
//
// capslockadapter and cli are excluded by design: capslockadapter owns capslock,
// whose closure contains golang.org/x/sys/unix built with cgo, and the go_component
// rule fails closed on cgo closures because preprocessed cgo sources do not exist at
// analysis time. cli imports capslockadapter and inherits its cgo closure. Both
// components retain native coverage via `just selfcheck`.
func TestSelfManifestParity(t *testing.T) {
	manifestparity.Run(t, flag.Args())
}
