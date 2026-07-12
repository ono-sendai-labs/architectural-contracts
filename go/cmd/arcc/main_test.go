package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned exit code %d, want 0", code)
	}
	if got, want := stdout.String(), "arcc 0.0.0-dev\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestUsageAndHelp(t *testing.T) {
	tests := [][]string{
		{"help"},
		{"--help"},
		{"-h"},
		{},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%v) = %d, want 0", args, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("run(%v) stdout = %q, want to contain 'Usage:'", args, stdout.String())
		}
	}
}

func TestCLI_InvalidInput_Exit2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "nonexistent-manifest.textproto"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run returned %d, want 2. Stdout: %s", code, stdout.String())
	}

	if got := stderr.String(); !strings.Contains(got, "failed to open manifest file") {
		t.Errorf("stderr = %q, want error regarding manifest file opening", got)
	}
}
