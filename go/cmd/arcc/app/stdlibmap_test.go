package app_test

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// oversizedPath creates a sparse file one byte over the decoder's bound, so
// the size guard (not the content) is what fails (review round 3).
func oversizedPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "oversized.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(artifactio.MaxMapBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestStdlibmapUsageErrors pins the stdlibmap commands' arg-parsing and
// lookup exit-code contract (task req 7): usage, decode, and lookup errors
// exit 2 with an actionable stderr message and nothing on stdout.
func TestStdlibmapUsageErrors(t *testing.T) {
	runner := &app.Runner{}
	garbage := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(garbage, []byte("{not a map"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "unknown stdlibmap subcommand",
			args:       []string{"stdlibmap", "frobnicate"},
			wantStderr: "unknown stdlibmap command",
		},
		{
			name:       "generate without --output",
			args:       []string{"stdlibmap", "generate"},
			wantStderr: "--output",
		},
		{
			name:       "generate with unknown option",
			args:       []string{"stdlibmap", "generate", "--bogus"},
			wantStderr: "unknown option: --bogus",
		},
		{
			name:       "generate with duplicate output",
			args:       []string{"stdlibmap", "generate", "--output=a", "--output=b"},
			wantStderr: "duplicate option: --output",
		},
		{
			name:       "inspect without an artifact",
			args:       []string{"stdlibmap", "inspect"},
			wantStderr: "requires exactly one artifact path",
		},
		{
			name:       "inspect missing file",
			args:       []string{"stdlibmap", "inspect", filepath.Join(t.TempDir(), "absent.json")},
			wantStderr: "no such file",
		},
		{
			name:       "inspect corrupt artifact",
			args:       []string{"stdlibmap", "inspect", garbage},
			wantStderr: "decoding",
		},
		{
			name:       "inspect unknown query kind",
			args:       []string{"stdlibmap", "inspect", garbage, "frobnicate"},
			wantStderr: "unknown query",
		},
		{
			name:       "inspect symbol query without an argument",
			args:       []string{"stdlibmap", "inspect", garbage, "symbol"},
			wantStderr: "symbol query requires a symbol ID",
		},
		{
			name:       "inspect with a bad --expect-key spec",
			args:       []string{"stdlibmap", "inspect", garbage, "--expect-key=nonsense"},
			wantStderr: "--expect-key",
		},
		{
			name:       "inspect with a duplicate expect-key field",
			args:       []string{"stdlibmap", "inspect", garbage, "--expect-key=goos=linux,goos=windows"},
			wantStderr: "given twice",
		},
		{
			name:       "inspect with a malformed cgo_enabled value",
			args:       []string{"stdlibmap", "inspect", garbage, "--expect-key=cgo_enabled=garbage"},
			wantStderr: "not true or false",
		},
		{
			name:       "inspect with a malformed map_format_version value",
			args:       []string{"stdlibmap", "inspect", garbage, "--expect-key=map_format_version=12x"},
			wantStderr: "not a decimal integer",
		},
		{
			name:       "generate with a mismatched toolchain override",
			args:       []string{"stdlibmap", "generate", "--output=" + filepath.Join(t.TempDir(), "map.json"), "--toolchain=go-wrong"},
			wantStderr: "does not match the current toolchain",
		},
		{
			name:       "inspect an oversized artifact is a bounded tool error",
			args:       []string{"stdlibmap", "inspect", oversizedPath(t), "summary"},
			wantStderr: "decoding the stdlib map artifact",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := runner.Run(tt.args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit code = %d; want 2 (stderr: %q)", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout on error = %q; want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q; want it to mention %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// --- task-05: explicit-input mode usage contract and the expect-key tag fix ------

// writeProbeMap generates a minimal stdlib map artifact whose SDK key carries
// the build tag "probe", via the injected generation seams (no toolchain).
func writeProbeMap(t *testing.T) string {
	t.Helper()
	out, err := stdlibmap.Generate(stdlibmap.GenerationInput{
		Target:      stdlibmap.TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"probe"}},
		RuleVersion: stdlibmap.RuleVersion,
		Oracle:      stdlibmap.PackageOracleFunc(func() ([]stdlibmap.PackageEntry, error) { return nil, nil }),
		Loader:      stdlibmap.LoaderFunc(func([]string) (map[string]*types.Package, error) { return map[string]*types.Package{}, nil }),
		Findings:    stdlibmap.FindingSourceFunc(func([]string) ([]capslockadapter.GenerationFinding, error) { return nil, nil }),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "probe-map.json")
	if err := stdlibmap.WriteArtifactAtomic(path, out.Bytes, 0o644); err != nil {
		t.Fatalf("WriteArtifactAtomic() error = %v", err)
	}
	return path
}

func TestInspectExpectKeyTag(t *testing.T) {
	path := writeProbeMap(t)
	runner := &app.Runner{}

	var stdout, stderr strings.Builder
	if code := runner.Run([]string{"stdlibmap", "inspect", path, "--expect-key=tag=other"}, &stdout, &stderr); code != 2 {
		t.Fatalf("expect-key tag=other: exit %d; want 2 (stderr: %q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "build_tags") {
		t.Fatalf("mismatch diagnostics %q do not name build_tags", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", path, "--expect-key=tag=probe"}, &stdout, &stderr); code != 0 {
		t.Fatalf("expect-key tag=probe: exit %d; want 0 (stderr: %q)", code, stderr.String())
	}
}

func TestStdlibmapGenerateExplicitUsage(t *testing.T) {
	runner := &app.Runner{}
	dir := t.TempDir()
	list := filepath.Join(dir, "list.txt")
	if err := os.WriteFile(list, []byte("fmt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	next := 0
	config := func(content string) string {
		next++
		path := filepath.Join(dir, fmt.Sprintf("config-%d.txt", next))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	validConfig := config("toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=false\nbuild_tags=\ngoexperiment=\n")
	cgoConfig := config("toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=true\nbuild_tags=\ngoexperiment=\n")
	malformedConfig := config("toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=yes\nbuild_tags=\ngoexperiment=\n")
	out := filepath.Join(dir, "map.json")
	sdkRoot := "/nonexistent-sdk"

	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "config without package list",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--config-file=" + validConfig, "--sdk-root=" + sdkRoot},
			wantStderr: "all-or-nothing",
		},
		{
			name:       "list without sdk root",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig},
			wantStderr: "all-or-nothing",
		},
		{
			name:       "duplicate sdk root",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig, "--sdk-root=/a", "--sdk-root=/b"},
			wantStderr: "duplicate option: --sdk-root",
		},
		{
			name:       "native flag alongside explicit flags",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig, "--sdk-root=" + sdkRoot, "--goos=linux"},
			wantStderr: "--goos",
		},
		{
			name:       "toolchain flag alongside explicit flags",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig, "--sdk-root=" + sdkRoot, "--toolchain=go1.26.4"},
			wantStderr: "--toolchain",
		},
		{
			name:       "tags flag alongside explicit flags",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig, "--sdk-root=" + sdkRoot, "--tags=probe"},
			wantStderr: "--tags",
		},
		{
			name:       "cgo flag alongside explicit flags",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig, "--sdk-root=" + sdkRoot, "--cgo"},
			wantStderr: "--cgo",
		},
		{
			name:       "goexperiment flag alongside explicit flags",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + validConfig, "--sdk-root=" + sdkRoot, "--goexperiment=arenayaslicit"},
			wantStderr: "--goexperiment",
		},
		{
			name:       "malformed cgo_enabled in the config",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + malformedConfig, "--sdk-root=" + sdkRoot},
			wantStderr: "cgo_enabled",
		},
		{
			name:       "cgo_enabled true is rejected naming the out-of-scope decision",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + cgoConfig, "--sdk-root=" + sdkRoot},
			wantStderr: "out of scope",
		},
		{
			name:       "missing config file",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + list, "--config-file=" + filepath.Join(dir, "absent.txt"), "--sdk-root=" + sdkRoot},
			wantStderr: "config",
		},
		{
			name:       "missing package list file",
			args:       []string{"stdlibmap", "generate", "--output=" + out, "--package-list=" + filepath.Join(dir, "absent.txt"), "--config-file=" + validConfig, "--sdk-root=" + sdkRoot},
			wantStderr: "package list",
		},
	}
	// The usage contract is checked before any host discovery: an empty PATH
	// would make any accidental `go env`/`go list` fail, so the specific
	// diagnostics below also prove explicit mode did not discover from the
	// host. (GOPACKAGESDRIVER stays unset: explicit mode sets it itself.)
	t.Setenv("PATH", "")
	t.Setenv("GOROOT", "")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := runner.Run(tt.args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit code = %d; want 2 (stderr: %q)", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout on error = %q; want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Fatalf("stderr = %q; want it to mention %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
