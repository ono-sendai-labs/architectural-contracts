package hosttoolchain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap/internal/hosttoolchain"
)

func TestHostToolchainFilesystemHelpers(t *testing.T) {
	dir := t.TempDir()
	if !hosttoolchain.DirectoryExists(dir) {
		t.Fatalf("DirectoryExists(%q) = false, want true", dir)
	}
	if hosttoolchain.DirectoryExists(filepath.Join(dir, "missing")) {
		t.Fatalf("DirectoryExists(missing) = true, want false")
	}

	path, cleanup, err := hosttoolchain.WriteTempLayout([]byte(`{"packages":[]}`))
	if err != nil {
		t.Fatalf("WriteTempLayout: %v", err)
	}
	rc, err := hosttoolchain.OpenFile(path)
	if err != nil {
		cleanup()
		t.Fatalf("OpenFile(%q): %v", path, err)
	}
	_ = rc.Close()
	cleanup()
	if _, err := os.Stat(path); !hosttoolchain.IsNotExist(err) {
		t.Fatalf("Stat(%q) after cleanup = %v, want not-exist", path, err)
	}
}
