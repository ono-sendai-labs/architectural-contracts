//go:build integration && stdlibmap_full

package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// TestExplicitGenerateHermeticAndByteIdentical is the explicit-input mode's
// end-to-end leg (task ACs 3–4): a map generated with --package-list,
// --config-file and --sdk-root — with PATH, GOROOT, GOCACHE and
// GOPACKAGESDRIVER removed from the environment, so no toolchain binary can
// be executed and go/packages cannot fall back to the go list driver —
// succeeds, is keyed by the config file, and is byte-identical to the native
// generation over the same SDK root and configuration.
func TestExplicitGenerateHermeticAndByteIdentical(t *testing.T) {
	// Inputs prepared while the toolchain is reachable: the oracle is `go
	// list std` written to the package-list file, and the target config is
	// the host toolchain's own configuration.
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	sdkRoot := filepath.Join(strings.TrimSpace(string(goroot)), "src")
	listOut, err := exec.Command("go", "list", "std").Output()
	if err != nil {
		t.Fatalf("go list std: %v", err)
	}
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		t.Fatalf("NativeTargetConfig: %v", err)
	}

	dir := t.TempDir()
	listPath := filepath.Join(dir, "package-list.txt")
	if err := os.WriteFile(listPath, listOut, 0o644); err != nil {
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

	nativePath := filepath.Join(dir, "native-map.json")
	explicitPath := filepath.Join(dir, "explicit-map.json")
	runner := &app.Runner{}

	var stdout, stderr strings.Builder
	if code := runner.Run([]string{"stdlibmap", "generate", "--output=" + nativePath}, &stdout, &stderr); code != 0 {
		t.Fatalf("native generate: exit %d, stderr: %s", code, stderr.String())
	}

	// The hermetic run: no PATH (so no executable lookup at all), no GOROOT,
	// GOCACHE or GOPACKAGESDRIVER.
	t.Setenv("PATH", "")
	t.Setenv("GOROOT", "")
	t.Setenv("GOCACHE", "")
	t.Setenv("GOPACKAGESDRIVER", "")

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{
		"stdlibmap", "generate",
		"--output=" + explicitPath,
		"--package-list=" + listPath,
		"--config-file=" + configPath,
		"--sdk-root=" + sdkRoot,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("explicit generate: exit %d, stderr: %s", code, stderr.String())
	}

	native, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := os.ReadFile(explicitPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(native) == 0 || len(explicit) == 0 {
		t.Fatal("a generated map is empty")
	}
	if string(native) != string(explicit) {
		t.Fatalf("explicit and native maps differ (%d vs %d bytes)", len(native), len(explicit))
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
}
