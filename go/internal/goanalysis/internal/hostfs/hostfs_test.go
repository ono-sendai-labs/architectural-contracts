package hostfs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestHostFS_ReadFileAndStat(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "sample.txt")
	content := []byte("hello hostfs")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("writing sample file: %v", err)
	}

	got, err := ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", filePath, err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("ReadFile(%q) = %q, want %q", filePath, got, content)
	}

	info, err := Stat(filePath)
	if err != nil {
		t.Fatalf("Stat(%q): %v", filePath, err)
	}
	if info.Size() != int64(len(content)) {
		t.Fatalf("Stat(%q).Size() = %d, want %d", filePath, info.Size(), len(content))
	}
}

func TestHostFS_ReadDir(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(file1, []byte("a"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", file1, err)
	}
	if err := os.WriteFile(file2, []byte("b"), 0o600); err != nil {
		t.Fatalf("writing %s: %v", file2, err)
	}

	entries, err := ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir(%q) returned %d entries, want 2", dir, len(entries))
	}
	if entries[0].Name() != "a.txt" || entries[1].Name() != "b.txt" {
		t.Fatalf("ReadDir(%q) entries = [%s, %s], want [a.txt, b.txt]", dir, entries[0].Name(), entries[1].Name())
	}
}

func TestHostFS_OpenExportData(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "export.data")
	content := []byte("export data payload")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("writing export file: %v", err)
	}

	rc, err := OpenExportData(filePath)
	if err != nil {
		t.Fatalf("OpenExportData(%q): %v", filePath, err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading from OpenExportData stream: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("OpenExportData content = %q, want %q", got, content)
	}
}

func TestHostFS_Abs(t *testing.T) {
	rel := "some/relative/path"
	abs, err := Abs(rel)
	if err != nil {
		t.Fatalf("Abs(%q): %v", rel, err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("Abs(%q) = %q, want absolute path", rel, abs)
	}
}
