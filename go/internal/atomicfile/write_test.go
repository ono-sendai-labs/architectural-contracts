package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errSeam = errors.New("injected seam failure")

// writeHarness prepares a directory containing a previous target file and returns
// its path and contents.
func writeHarness(t *testing.T) (dir, target, previous string) {
	t.Helper()
	dir = t.TempDir()
	target = filepath.Join(dir, "artifact.json")
	previous = "{\"previous\":true}"
	if err := os.WriteFile(target, []byte(previous), 0o644); err != nil {
		t.Fatalf("write previous target: %v", err)
	}
	return dir, target, previous
}

// TestAtomicWriteSuccess pins the happy path: the new bytes replace the old
// and no temporary files remain.
func TestAtomicWriteSuccess(t *testing.T) {
	dir, target, _ := writeHarness(t)
	next := "{\"next\":true}"
	if err := WriteFileAtomic(target, []byte(next), 0o644, nil); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != next {
		t.Errorf("target = %q, want %q", got, next)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("residue in target directory: %v", names)
	}
}

// TestAtomicWriteFailuresPreserveTarget pins AC5: a failure at every stage
// before the rename leaves the previous target bytes intact and removes all
// temporary residue.
func TestAtomicWriteFailuresPreserveTarget(t *testing.T) {
	next := "{\"next\":true}"

	cases := []struct {
		name  string
		seams *WriteSeams
	}{
		{
			name:  "create fails",
			seams: &WriteSeams{CreateTemp: func(string, string) (*os.File, error) { return nil, errSeam }},
		},
		{
			name: "write fails",
			seams: &WriteSeams{Write: func(*os.File, []byte) error {
				return errSeam
			}},
		},
		{
			name:  "sync fails",
			seams: &WriteSeams{Sync: func(*os.File) error { return errSeam }},
		},
		{
			name:  "close fails",
			seams: &WriteSeams{Close: func(*os.File) error { return errSeam }},
		},
		{
			name:  "chmod fails",
			seams: &WriteSeams{Chmod: func(string, os.FileMode) error { return errSeam }},
		},
		{
			name:  "rename fails",
			seams: &WriteSeams{Rename: func(string, string) error { return errSeam }},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, target, previous := writeHarness(t)
			err := WriteFileAtomic(target, []byte(next), 0o644, tc.seams)
			if err == nil {
				t.Fatal("WriteFileAtomic succeeded despite an injected failure")
			}
			if !errors.Is(err, errSeam) {
				t.Errorf("error = %v, want it to wrap the injected sentinel", err)
			}

			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("read target: %v", err)
			}
			if string(got) != previous {
				t.Errorf("previous target bytes were disturbed: %q, want %q", got, previous)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read dir: %v", err)
			}
			if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
				names := make([]string, len(entries))
				for i, e := range entries {
					names[i] = e.Name()
				}
				t.Errorf("temporary residue left behind: %v", names)
			}
		})
	}
}

// TestFailedRenameLeavesNoTempFile explicitly verifies that when rename fails,
// the temporary file is removed and no temp file residue is left behind.
func TestFailedRenameLeavesNoTempFile(t *testing.T) {
	dir, target, previous := writeHarness(t)
	seams := &WriteSeams{
		Rename: func(string, string) error { return errSeam },
	}
	err := WriteFileAtomic(target, []byte("{\"next\":true}"), 0o644, seams)
	if err == nil {
		t.Fatal("WriteFileAtomic succeeded despite injected rename failure")
	}
	if !errors.Is(err, errSeam) {
		t.Errorf("error = %v, want it to wrap injected sentinel", err)
	}

	// Verify original file unchanged
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != previous {
		t.Errorf("target bytes changed: got %q, want %q", got, previous)
	}

	// Verify directory contains only target file, no temp files
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("unexpected files in dir: %v", names)
	}
}

// TestAtomicWriteCleanupFailure verifies that when cleanup removal also fails,
// both the original failure and the cleanup failure are surfaced together.
func TestAtomicWriteCleanupFailure(t *testing.T) {
	_, target, previous := writeHarness(t)
	errCleanup := os.ErrPermission
	seams := &WriteSeams{
		Rename: func(string, string) error { return errSeam },
		Remove: func(string) error { return errCleanup },
	}
	err := WriteFileAtomic(target, []byte("{\"next\":true}"), 0o644, seams)
	if err == nil {
		t.Fatal("WriteFileAtomic succeeded despite injected failures")
	}
	if !strings.Contains(err.Error(), errSeam.Error()) {
		t.Errorf("error %v lost the original failure", err)
	}
	if !strings.Contains(err.Error(), errCleanup.Error()) {
		t.Errorf("error %v lost the cleanup failure", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != previous {
		t.Errorf("target = %q (%v), want %q", got, err, previous)
	}
}

// TestWriteFileAtomicAppliesRequestedMode verifies that the permission mode
// requested by the caller is applied to the written file.
func TestWriteFileAtomicAppliesRequestedMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact.json")
	next := []byte("{\"mode\":true}")

	if err := WriteFileAtomic(target, next, 0o640, nil); err != nil {
		t.Fatalf("WriteFileAtomic (new target): %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 0640", info.Mode().Perm())
	}

	if err := WriteFileAtomic(target, next, 0o600, nil); err != nil {
		t.Fatalf("WriteFileAtomic (existing target): %v", err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
}
