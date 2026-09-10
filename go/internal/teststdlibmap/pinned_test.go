package teststdlibmap_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/teststdlibmap"
)

func TestPinnedMapMatchesCurrentProductionKey(t *testing.T) {
	authority := teststdlibmap.OpenPinned(t)
	key := authority.Key()
	if key.ToolchainVersion != "go1.26.4" || key.GOOS != "linux" || key.GOARCH != "amd64" || key.CgoEnabled {
		t.Fatalf("pinned key = %+v, want Go 1.26.4 linux/amd64 cgo-off", key)
	}
	if key.MapFormatVersion != 1 || key.ClassifierHash == "" {
		t.Fatalf("pinned key lacks production format/classifier identity: %+v", key)
	}
	if !authority.IsStdlibPackage("os") {
		t.Fatal("pinned map does not contain os")
	}
	if _, err := authority.SymbolAuthority(teststdlibmap.MustSymbolID(t, "os.ReadFile")); err != nil {
		t.Fatalf("pinned map lookup: %v", err)
	}
}

func TestPinnedMapCanBeCopiedForSubprocessTests(t *testing.T) {
	path := teststdlibmap.WritePinned(t)
	if path == "" {
		t.Fatal("WritePinned returned an empty path")
	}
}
