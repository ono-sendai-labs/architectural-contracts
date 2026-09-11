package goanalysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"golang.org/x/tools/go/packages"
)

func TestCollectExportDataDiagnostics_DeduplicatesResolvedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "exports", "first.a")
	second := filepath.Join(workspace, "exports", "second.a")
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("first export"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second export data"), 0o644); err != nil {
		t.Fatal(err)
	}

	leaf := &packages.Package{
		ID:         "example.com/leaf",
		PkgPath:    "example.com/leaf",
		ExportFile: "exports/second.a",
		Imports:    map[string]*packages.Package{},
	}
	firstDependency := &packages.Package{
		ID:         "example.com/first",
		PkgPath:    "example.com/first",
		ExportFile: "exports/first.a",
		Imports:    map[string]*packages.Package{"example.com/leaf": leaf},
	}
	secondDependency := &packages.Package{
		ID:         "example.com/second",
		PkgPath:    "example.com/second",
		ExportFile: "exports/first.a",
		Imports:    map[string]*packages.Package{},
	}
	root := &packages.Package{
		ID:      "example.com/member",
		PkgPath: "example.com/member",
		Imports: map[string]*packages.Package{
			"example.com/first":  firstDependency,
			"example.com/second": secondDependency,
		},
	}

	got, err := collectExportDataDiagnostics([]*packages.Package{root}, map[string]bool{"example.com/member": true}, workspace)
	if err != nil {
		t.Fatalf("collectExportDataDiagnostics() error = %v", err)
	}
	want := facts.ExportDataDiagnostics{
		NonMemberExportArtifactCount: 2,
		NonMemberExportBytes:         uint64(len("first export") + len("second export data")),
	}
	if got != want {
		t.Fatalf("diagnostics = %#v, want %#v", got, want)
	}
}

func TestCollectExportDataDiagnostics_RejectsMissingArtifact(t *testing.T) {
	root := &packages.Package{
		ID:      "example.com/member",
		PkgPath: "example.com/member",
		Imports: map[string]*packages.Package{
			"example.com/dep": {
				ID:         "example.com/dep",
				PkgPath:    "example.com/dep",
				ExportFile: "exports/missing.a",
				Imports:    map[string]*packages.Package{},
			},
		},
	}

	_, err := collectExportDataDiagnostics([]*packages.Package{root}, map[string]bool{"example.com/member": true}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "example.com/dep") || !strings.Contains(err.Error(), "missing.a") {
		t.Fatalf("error = %v, want missing package and artifact", err)
	}
}

type diagnosticsFileInfo struct {
	size int64
}

func (f diagnosticsFileInfo) Name() string       { return "export.a" }
func (f diagnosticsFileInfo) Size() int64        { return f.size }
func (f diagnosticsFileInfo) Mode() os.FileMode  { return 0o644 }
func (f diagnosticsFileInfo) ModTime() time.Time { return time.Time{} }
func (f diagnosticsFileInfo) IsDir() bool        { return false }
func (f diagnosticsFileInfo) Sys() any           { return nil }

func TestCollectExportDataDiagnostics_RejectsByteOverflow(t *testing.T) {
	root := &packages.Package{ID: "example.com/member", PkgPath: "example.com/member"}
	root.Imports = map[string]*packages.Package{}
	for _, name := range []string{"one", "two", "three"} {
		root.Imports["example.com/"+name] = &packages.Package{
			ID:         "example.com/" + name,
			PkgPath:    "example.com/" + name,
			ExportFile: "exports/" + name + ".a",
			Imports:    map[string]*packages.Package{},
		}
	}

	previous := exportDataStat
	exportDataStat = func(string) (os.FileInfo, error) {
		return diagnosticsFileInfo{size: int64(^uint64(0) >> 1)}, nil
	}
	defer func() { exportDataStat = previous }()

	_, err := collectExportDataDiagnostics([]*packages.Package{root}, map[string]bool{"example.com/member": true}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "overflows uint64") {
		t.Fatalf("error = %v, want uint64 overflow", err)
	}
}
