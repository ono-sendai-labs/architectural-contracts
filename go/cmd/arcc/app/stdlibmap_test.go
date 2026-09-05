package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
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
