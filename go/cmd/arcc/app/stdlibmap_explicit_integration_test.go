//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// TestExplicitGenerateUsesBoundedSDKAndNativeOptionsAreChecked is command-
// plumbing coverage (task requirement 4): explicit generation uses a tiny
// declared SDK and package list, while native option validation is exercised
// only through the early toolchain-version check. Whole-host-SDK totality,
// semantic, and determinism coverage remains in the stdlibmap full lane.
func TestExplicitGenerateUsesBoundedSDKAndNativeOptionsAreChecked(t *testing.T) {
	sdkRoot := syntheticSDK(t)
	target := stdlibmap.TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64"}

	dir := t.TempDir()
	listPath := filepath.Join(dir, "package-list.txt")
	if err := os.WriteFile(listPath, []byte("fmt\ninternal/testcap\nunsafe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rendered, err := stdlibmap.RenderTargetConfig(target)
	if err != nil {
		t.Fatalf("RenderTargetConfig: %v", err)
	}
	configPath := filepath.Join(dir, "target-config.txt")
	if err := os.WriteFile(configPath, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}

	explicitPath := filepath.Join(dir, "explicit-map.json")
	runner := &app.Runner{}
	var stdout, stderr strings.Builder
	hostPath := os.Getenv("PATH")

	// The hermetic run: no PATH (so no executable lookup at all), no GOROOT,
	// GOCACHE or GOPACKAGESDRIVER. The bounded SDK is loaded through arcc's
	// own package-layout driver.
	t.Setenv("PATH", "")
	t.Setenv("GOROOT", "")
	t.Setenv("GOCACHE", "")
	t.Setenv("GOPACKAGESDRIVER", "")
	if code := runner.Run([]string{
		"stdlibmap", "generate",
		"--output=" + explicitPath,
		"--package-list=" + listPath,
		"--config-file=" + configPath,
		"--sdk-root=" + sdkRoot,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("explicit generate: exit %d, stderr: %s", code, stderr.String())
	}

	explicit, err := os.ReadFile(explicitPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(explicit) == 0 {
		t.Fatal("the generated map is empty")
	}

	// The map's key matches the config file.
	stdout.Reset()
	stderr.Reset()
	expect := "--expect-key=toolchain_version=" + target.ToolchainVersion +
		",goos=" + target.GOOS + ",goarch=" + target.GOARCH +
		",cgo_enabled=false,goexperiment=" + target.GOEXPERIMENT
	if code := runner.Run([]string{"stdlibmap", "inspect", explicitPath, expect}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect expect-key: exit %d, stderr: %s", code, stderr.String())
	}
	summary := stdout.String()
	for _, want := range []string{"packages: ", "symbols: ", "inits: "} {
		if !strings.Contains(summary, want) {
			t.Fatalf("explicit map summary %q missing %q", summary, want)
		}
	}

	// Native generation is not started by this command-plumbing test. Its
	// option contract rejects a mismatched version immediately after the small
	// `go env` discovery, before package enumeration or map generation.
	stdout.Reset()
	stderr.Reset()
	t.Setenv("PATH", hostPath)
	nativePath := filepath.Join(dir, "native-map.json")
	if code := runner.Run([]string{"stdlibmap", "generate", "--output=" + nativePath, "--toolchain=not-the-active-toolchain"}, &stdout, &stderr); code != 2 {
		t.Fatalf("native option validation: exit %d, want 2; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "does not match the current toolchain") {
		t.Fatalf("native option validation stderr %q does not identify the mismatch", stderr.String())
	}
	if _, err := os.Stat(nativePath); !os.IsNotExist(err) {
		t.Fatalf("native option validation created %s", nativePath)
	}
}

func syntheticSDK(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "sdk", "src")
	write := func(rel, source string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("fmt/format.go", "package fmt\n\nfunc Println(s string) {}\n")
	write("unsafe/unsafe.go", "package unsafe\n")
	write("internal/testcap/testcap.go", "package testcap\n")
	return root
}
