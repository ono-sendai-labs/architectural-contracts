package artifactio

import (
	"slices"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// TestKnownCapabilitiesMatchesManifestTaxonomy guards the local spelling of
// the known Capslock capability taxonomy against drift from the manifest
// component's canonical set. The artifact-I/O shell validates capability
// names against its own copy because it consumes the taxonomy value set
// without a component dependency on the manifest component; if the two sets
// diverge, this test must fail.
func TestKnownCapabilitiesMatchesManifestTaxonomy(t *testing.T) {
	local := make([]string, 0, len(knownCapabilities))
	for cap := range knownCapabilities {
		local = append(local, cap)
	}
	slices.Sort(local)

	manifestSet := make([]string, 0, len(manifest.KnownCapabilities))
	for cap := range manifest.KnownCapabilities {
		manifestSet = append(manifestSet, cap)
	}
	slices.Sort(manifestSet)

	if !slices.Equal(local, manifestSet) {
		t.Errorf("knownCapabilities %v diverges from manifest.KnownCapabilities %v", local, manifestSet)
	}
}
