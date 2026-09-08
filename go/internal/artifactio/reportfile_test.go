package artifactio_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

func writeTempReport(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "component.report.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp report: %v", err)
	}
	return path
}

func TestReadReportFile_RoundTrip(t *testing.T) {
	input := report.ConformanceReport{
		Component: "csvtool",
		Violations: []report.Finding{
			{Kind: report.UndeclaredAuthority, Message: "FILES", Location: report.Location{File: "a.go", Line: 7}},
		},
	}
	data, err := report.MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport() error = %v", err)
	}
	path := writeTempReport(t, data)

	got, err := artifactio.ReadReportFile(path)
	if err != nil {
		t.Fatalf("ReadReportFile() error = %v", err)
	}
	if got.Verdict != report.VerdictFail {
		t.Errorf("verdict = %q, want %q", got.Verdict, report.VerdictFail)
	}
	if !reflect.DeepEqual(got.Report, input) {
		t.Errorf("decoded report = %#v, want %#v", got.Report, input)
	}
}

func TestReadReportFile_OpenError(t *testing.T) {
	_, err := artifactio.ReadReportFile(filepath.Join(t.TempDir(), "missing.report.json"))
	if err == nil {
		t.Fatal("ReadReportFile() succeeded for a missing file, want error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want wrapping os.ErrNotExist", err)
	}
}

func TestReadReportFile_RejectsMalformed(t *testing.T) {
	path := writeTempReport(t, []byte("{not a report"))
	_, err := artifactio.ReadReportFile(path)
	if !errors.Is(err, report.ErrMalformedReport) {
		t.Errorf("error = %v, want wrapping report.ErrMalformedReport", err)
	}
}

func TestReadReportFile_RejectsOversizedInput(t *testing.T) {
	oversized := strings.Repeat("a", int(report.MaxReportBytes)+1)
	path := writeTempReport(t, []byte(oversized))
	_, err := artifactio.ReadReportFile(path)
	if err == nil {
		t.Fatal("ReadReportFile() succeeded on an oversized input, want error")
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("error = %v, want a size-limit diagnostic", err)
	}
}
