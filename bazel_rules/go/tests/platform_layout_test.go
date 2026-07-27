package tests

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

func findLayoutFile(t *testing.T) (string, error) {
	startDir := "."
	if srcDir := os.Getenv("TEST_SRCDIR"); srcDir != "" {
		startDir = srcDir
	}

	var found string
	err := filepath.Walk(startDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			t.Logf("File: %s", path)
			if strings.HasSuffix(path, ".package-layout.json") {
				found = path
			}
		}
		return nil
	})
	if found != "" {
		return found, nil
	}
	return "", err
}

func TestTransitionedPlatformLayout(t *testing.T) {
	pPath, err := findLayoutFile(t)
	if err != nil || pPath == "" {
		t.Fatalf("failed to walk directory to find layout file: %v", err)
	}

	t.Logf("Found transitioned layout file: %s", pPath)

	data, err := os.ReadFile(pPath)
	if err != nil {
		t.Fatalf("failed to read transitioned layout file: %v", err)
	}

	layout, err := packagelayout.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to parse layout: %v", err)
	}

	if layout.Platform == nil {
		t.Fatal("layout.Platform is nil, want non-nil platform block")
	}

	p := layout.Platform
	if p.GOOS != "darwin" {
		t.Errorf("platform.goos = %q, want %q", p.GOOS, "darwin")
	}
	if p.GOARCH != "arm64" {
		t.Errorf("platform.goarch = %q, want %q", p.GOARCH, "arm64")
	}
	if p.CgoEnabled {
		t.Errorf("platform.cgo_enabled = %v, want false", p.CgoEnabled)
	}

	foundTag := false
	for _, tag := range p.BuildTags {
		if tag == "adapter_probe" {
			foundTag = true
			break
		}
	}
	if !foundTag {
		t.Errorf("platform.build_tags = %v, want it to contain %q", p.BuildTags, "adapter_probe")
	}
}
