package stdlibmap

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestProjectExportFilesCopiesOnlyMetadataReferencedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := filepath.Join(workspace, "rules_go", "gocache")
	outputRoot := filepath.Join(workspace, "projected")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "aa"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "bb"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		filepath.Join(sourceRoot, "aa", "export-a-d"),
		filepath.Join(sourceRoot, "bb", "export-b-d"),
		filepath.Join(sourceRoot, "unrelated-action-id-a"),
	} {
		if err := os.WriteFile(file, []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	metadataPath := filepath.Join(workspace, "stdlib.pkg.json")
	metadataRoot := filepath.ToSlash(sourceRoot)
	metadata := strings.Join([]string{
		`{"ID":"a","ExportFile":"__BAZEL_EXECROOT__/` + metadataRoot + `/aa/export-a-d"}`,
		`{"ID":"b","ExportFile":"__BAZEL_EXECROOT__/` + metadataRoot + `/bb/export-b-d"}`,
		`{"ID":"empty","ExportFile":""}`,
		"",
	}, "\n")
	if err := os.WriteFile(metadataPath, []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ProjectExportFiles(metadataPath, sourceRoot, outputRoot); err != nil {
		t.Fatalf("ProjectExportFiles() error = %v", err)
	}

	var got []string
	if err := filepath.WalkDir(outputRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, err := filepath.Rel(outputRoot, path)
			if err != nil {
				return err
			}
			got = append(got, filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"aa/export-a-d", "bb/export-b-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected files = %v, want %v", got, want)
	}
}

func TestProjectExportFilesRejectsExportOutsideDeclaredRoot(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := filepath.Join(workspace, "rules_go", "gocache")
	metadataPath := filepath.Join(workspace, "stdlib.pkg.json")
	metadata := `{"ID":"bad","ExportFile":"__BAZEL_EXECROOT__/rules_go/pkg/linux_amd64/bad.a"}` + "\n"
	if err := os.WriteFile(metadataPath, []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ProjectExportFiles(metadataPath, sourceRoot, filepath.Join(workspace, "projected"))
	if err == nil || !strings.Contains(err.Error(), "outside declared export root") {
		t.Fatalf("ProjectExportFiles() error = %v, want outside-root diagnostic", err)
	}
}

func TestProjectExportFilesFailsWhenMetadataArtifactIsMissing(t *testing.T) {
	workspace := t.TempDir()
	sourceRoot := filepath.Join(workspace, "rules_go", "gocache")
	metadataPath := filepath.Join(workspace, "stdlib.pkg.json")
	metadata := `{"ID":"missing","ExportFile":"__BAZEL_EXECROOT__/` + filepath.ToSlash(sourceRoot) + `/missing/export-d"}` + "\n"
	if err := os.WriteFile(metadataPath, []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ProjectExportFiles(metadataPath, sourceRoot, filepath.Join(workspace, "projected"))
	if err == nil || !strings.Contains(err.Error(), "is not available below declared root") {
		t.Fatalf("ProjectExportFiles() error = %v, want missing-artifact diagnostic", err)
	}
}
