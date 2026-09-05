//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
)

// TestStdlibmapGenerateAndInspect is the CLI integration leg (task AC 4–5):
// a real native generation of the local toolchain's stdlib map, byte-identical
// regeneration, and the deterministic inspect summary plus the pinned
// semantic queries (`os.ReadFile`, `strings.TrimSpace`, `sort.Slice`, a
// package init). Unknown symbols and mismatched expected keys exit 2.
func TestStdlibmapGenerateAndInspect(t *testing.T) {
	runner := &app.Runner{}
	dir := t.TempDir()
	first := filepath.Join(dir, "map1.json")
	second := filepath.Join(dir, "map2.json")

	var stdout, stderr strings.Builder
	if code := runner.Run([]string{"stdlibmap", "generate", "--output=" + first}, &stdout, &stderr); code != 0 {
		t.Fatalf("first generate: exit %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sdk key: sdk{toolchain_version:") {
		t.Fatalf("generate summary missing the SDK key: %s", stdout.String())
	}
	data1, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("reading generated artifact: %v", err)
	}
	if len(data1) == 0 {
		t.Fatal("generated artifact is empty")
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "generate", "--output=" + second}, &stdout, &stderr); code != 0 {
		t.Fatalf("second generate: exit %d, stderr: %s", code, stderr.String())
	}
	data2, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("reading regenerated artifact: %v", err)
	}
	if string(data1) != string(data2) {
		t.Fatalf("two identical generations produced different bytes (%d vs %d)", len(data1), len(data2))
	}

	// Summary: the full SDK key is shown.
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect summary: exit %d, stderr: %s", code, stderr.String())
	}
	summary := stdout.String()
	for _, want := range []string{"sdk key: sdk{toolchain_version:", "format version: 1", "packages: ", "symbols: ", "inits: "} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q does not contain %q", summary, want)
		}
	}

	// os.ReadFile is FILES with ordered evidence.
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first, "symbol", "os.ReadFile"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect os.ReadFile: exit %d, stderr: %s", code, stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "os.ReadFile: CAPABILITIES [FILES]") || !strings.Contains(out, "evidence FILES:") {
		t.Fatalf("os.ReadFile inspect output: %q", out)
	}

	// strings.TrimSpace is SAFE with its proved-pure trust annotation;
	// sort.Slice is UNANALYZED (the spike's laundering case must be terminal,
	// not absent or silently pure); os.Exit is SAFE with Capslock's curation.
	for _, tc := range []struct{ symbol, want string }{
		{"strings.TrimSpace", "SAFE"},
		{"sort.Slice", "UNANALYZED"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := runner.Run([]string{"stdlibmap", "inspect", first, "symbol", tc.symbol}, &stdout, &stderr); code != 0 {
			t.Fatalf("inspect %s: exit %d, stderr: %s", tc.symbol, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), tc.symbol+": "+tc.want) {
			t.Fatalf("%s inspect output %q; want %s", tc.symbol, stdout.String(), tc.want)
		}
	}
	// Provenance is part of the answer (AC 5): curated SAFE records name
	// Capslock's curation, and the minting rule's project override names this
	// project's reclassification.
	for _, tc := range []struct{ symbol, provenance string }{
		{"strings.TrimSpace", "capslock-curated"},
		{"(os.File).Chmod", "project-override"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := runner.Run([]string{"stdlibmap", "inspect", first, "symbol", tc.symbol}, &stdout, &stderr); code != 0 {
			t.Fatalf("inspect %s provenance: exit %d, stderr: %s", tc.symbol, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "provenance: "+tc.provenance) {
			t.Fatalf("%s inspect output %q; want provenance %s", tc.symbol, stdout.String(), tc.provenance)
		}
	}
	// The init query carries its trust annotation too.
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first, "init", "os"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect init os provenance: exit %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "provenance: ") {
		t.Fatalf("init os inspect output %q has no provenance line", stdout.String())
	}

	// A package init query shows a terminal classification.
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first, "init", "os"}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect init os: exit %d, stderr: %s", code, stderr.String())
	}
	initOut := stdout.String()
	if !strings.Contains(initOut, "os.init: ") ||
		!(strings.Contains(initOut, "SAFE") || strings.Contains(initOut, "UNANALYZED") || strings.Contains(initOut, "CAPABILITIES [")) {
		t.Fatalf("init os inspect output %q is not a terminal classification", initOut)
	}

	// An unknown symbol is a tool error (exit 2), on stderr.
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first, "symbol", "os.NoSuchThing"}, &stdout, &stderr); code != 2 {
		t.Fatalf("inspect unknown symbol: exit %d; want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unknown symbol wrote to stdout: %q", stdout.String())
	}

	// A mismatched expected key is a tool error; a matching one succeeds.
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first, "--expect-key=toolchain_version=go-wrong"}, &stdout, &stderr); code != 2 {
		t.Fatalf("inspect mismatched expect-key: exit %d; want 2", code)
	}
	if !strings.Contains(stderr.String(), "toolchain_version") {
		t.Fatalf("mismatch diagnostics %q do not name the field", stderr.String())
	}

	// Derive the actual toolchain version from the summary for the matching
	// expectation.
	line := ""
	for _, l := range strings.Split(summary, "\n") {
		if strings.HasPrefix(l, "sdk key: ") {
			line = l
			break
		}
	}
	open := strings.Index(line, `toolchain_version:"`) + len(`toolchain_version:"`)
	close := strings.IndexByte(line[open:], '"') + open
	version := line[open:close]
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"stdlibmap", "inspect", first, "--expect-key=toolchain_version=" + version}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect matching expect-key: exit %d, stderr: %s", code, stderr.String())
	}
}
