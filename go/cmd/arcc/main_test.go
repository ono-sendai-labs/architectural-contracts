package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
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

func TestCLI_Toprow_Success(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manifestPath := "../../examples/csvtool/toprow/component.textproto"
	code := run([]string{"check", manifestPath}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run(check toprow) returned %d, want 0. Stderr: %s", code, stderr.String())
	}

	got := stdout.String()
	want := `Component "toprow" conforms / ambient-authority-free`
	if !strings.Contains(got, want) {
		t.Errorf("stdout = %q, want to contain %q", got, want)
	}
}

func TestCLI_Csvfile_Success(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manifestPath := "../../examples/csvtool/csvfile/component.textproto"
	code := run([]string{"check", manifestPath}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run(check csvfile) returned %d, want 0. Stderr: %s", code, stderr.String())
	}

	got := stdout.String()
	// Since csvfile declares FILES and actually uses FILES (os.ReadFile), it should conform!
	want := `Component "csvfile" conforms / ambient-authority-free`
	if !strings.Contains(got, want) {
		t.Errorf("stdout = %q, want to contain %q", got, want)
	}
}

func createTempComponent(t *testing.T, name string, manifestContent string, files map[string]string) (string, string) {
	t.Helper()
	// Create inside go/ so it's part of the main module
	// go/ relative to go/cmd/arcc is ../../
	tmpDir, err := os.MkdirTemp("../../", "temp-"+name+"-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDir)
	})

	relDir, err := filepath.Rel("../../", tmpDir)
	if err != nil {
		t.Fatalf("failed to get rel path: %v", err)
	}

	importPath := "github.com/ono-sendai-labs/architectural-contracts/go/" + filepath.ToSlash(relDir)

	manifestPath := filepath.Join(tmpDir, "component.textproto")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write temp manifest: %v", err)
	}

	for f, content := range files {
		content = strings.ReplaceAll(content, "{{IMPORT_PATH}}", importPath)
		absPath := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp file %s: %v", f, err)
		}
	}
	return tmpDir, manifestPath
}

func TestCLI_Toprow_Failing_UndeclaredDependency(t *testing.T) {
	// Let's create a temporary toprow component that imports a package outside its root but omits it from absorbed_dependencies
	manifestContent := `
name: "toprow-fail"
interface_files: "toprow.go"
`
	files := map[string]string{
		"toprow.go": `package toprow
import "github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
func Hello() {
	_ = report.Finding{}
}
`,
	}
	_, manifestPath := createTempComponent(t, "toprow-fail", manifestContent, files)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check", manifestPath}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run returned %d, want 1. Stderr: %s", code, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "UNDECLARED_DEPENDENCY") {
		t.Errorf("stdout = %q, want to contain UNDECLARED_DEPENDENCY", got)
	}
}

func TestCLI_AbsorbApp_Failing_UndeclaredAuthority(t *testing.T) {
	// Create a throwaway composition fixture absorbapp that absorbs csvfile (or calls os.ReadFile directly)
	// while claiming no authority.
	manifestContent := `
name: "absorbapp"
interface_files: "main.go"
`
	files := map[string]string{
		"main.go": `package main
import "os"
func Hello() {
	_, _ = os.ReadFile("test.csv")
}
`,
	}
	_, manifestPath := createTempComponent(t, "absorbapp", manifestContent, files)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check", manifestPath}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run returned %d, want 1. Stderr: %s", code, stderr.String())
	}

	got := stdout.String()
	if !strings.Contains(got, "UNDECLARED_AUTHORITY") {
		t.Errorf("stdout = %q, want to contain UNDECLARED_AUTHORITY", got)
	}
}

func TestCLI_FormatJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manifestPath := "../../examples/csvtool/toprow/component.textproto"
	code := run([]string{"check", manifestPath, "--format=json"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run returned %d, want 0. Stderr: %s", code, stderr.String())
	}

	var rep report.ConformanceReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("failed to decode JSON output: %v, stdout: %s", err, stdout.String())
	}

	if rep.Component != "toprow" {
		t.Errorf("rep.Component = %q, want 'toprow'", rep.Component)
	}

	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Errorf("JSON output does not have a trailing newline")
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
