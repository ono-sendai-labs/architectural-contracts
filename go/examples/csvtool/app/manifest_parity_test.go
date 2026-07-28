package app_test

import (
	"flag"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifestparity"
)

// TestManifestParity checks that the manifests the go_component rule generates
// say the same thing as the hand-written colocated manifests they mirror. It is
// driven by Bazel, which passes every relevant manifest as a positional
// argument (see //go/examples/csvtool/app:manifest_parity_test). Under plain
// `go test` it gets none and skips.
func TestManifestParity(t *testing.T) {
	manifestparity.Run(t, flag.Args())
}
