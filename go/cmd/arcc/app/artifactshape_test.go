package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func TestArtifactShapeCommandComparesValidatedSnapshot(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "component.report.json")
	goldenPath := filepath.Join(dir, "component.report.shape.golden.json")
	artifact, err := artifactio.MarshalReport(report.ConformanceReport{Component: "shape"})
	if err != nil {
		t.Fatalf("MarshalReport: %v", err)
	}
	golden, err := artifactio.Snapshot(artifactio.ShapeReport, bytes.NewReader(artifact))
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if err := os.WriteFile(artifactPath, artifact, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(goldenPath, golden, 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := (&Runner{}).Run([]string{
		"artifact-shape", "report", artifactPath, "--golden=" + goldenPath,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("artifact-shape exit code = %d, stderr = %s", code, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("successful artifact-shape wrote stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := (&Runner{}).Run([]string{
		"artifact-shape", "report", artifactPath, "--print",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("artifact-shape --print exit code = %d, stderr = %s", code, stderr.String())
	}
	if stdout.String() != string(golden) {
		t.Errorf("printed artifact shape = %q, want %q", stdout.String(), string(golden))
	}

	changed := strings.Replace(string(golden), `"component": "shape"`, `"component": "changed"`, 1)
	if err := os.WriteFile(goldenPath, []byte(changed), 0o644); err != nil {
		t.Fatalf("write changed golden: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := (&Runner{}).Run([]string{
		"artifact-shape", "report", artifactPath, "--golden=" + goldenPath,
	}, &stdout, &stderr); code != 1 {
		t.Fatalf("mismatched artifact-shape exit code = %d, want 1; stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "component") || !strings.Contains(stderr.String(), "changed") {
		t.Errorf("mismatch diagnostic = %q, want focused component diff", stderr.String())
	}
}

func TestArtifactShapeCommandRejectsNonCanonicalInput(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "component.report.json")
	goldenPath := filepath.Join(dir, "component.report.shape.golden.json")
	if err := os.WriteFile(artifactPath, []byte(`{"format_version":1,"verdict":"pass","report":{"component":"shape"}}
`), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(goldenPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}

	var stderr bytes.Buffer
	if code := (&Runner{}).Run([]string{
		"artifact-shape", "report", artifactPath, "--golden=" + goldenPath,
	}, &bytes.Buffer{}, &stderr); code != 2 {
		t.Fatalf("non-canonical artifact-shape exit code = %d, want 2; stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not canonical") {
		t.Errorf("non-canonical diagnostic = %q, want canonicality error", stderr.String())
	}
}
