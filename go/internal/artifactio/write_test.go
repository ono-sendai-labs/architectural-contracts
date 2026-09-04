package artifactio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errSeam = errors.New("injected seam failure")

// harness prepares a directory containing a previous target file and returns
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
