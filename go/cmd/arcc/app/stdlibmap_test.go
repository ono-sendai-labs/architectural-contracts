package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
)

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
