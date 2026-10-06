package packagelayout_test

import (
	"go/build"
	"os"
	"path/filepath"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

func TestFileMatchesBuildConstraints_MatchesBuildContext(t *testing.T) {
	tempDir := t.TempDir()

	testFiles := []struct {
		name    string
		content string
	}{
		{name: "plain.go", content: "package p\n"},
		{name: "file_linux.go", content: "package p\n"},
		{name: "file_windows.go", content: "package p\n"},
		{name: "file_windows_amd64.go", content: "package p\n"},
		{name: "file_linux_amd64.go", content: "package p\n"},
		{name: "file_cgo.go", content: "//go:build cgo\n\npackage p\n"},
		{name: "file_nocgo.go", content: "//go:build !cgo\n\npackage p\n"},
		{name: "file_complex.go", content: "//go:build (linux && amd64) || (windows && 386)\n\npackage p\n"},
		{name: "file_tags.go", content: "//go:build customtag && !othertag\n\npackage p\n"},
		{name: "file_release.go", content: "//go:build go1.20\n\npackage p\n"},
		{name: "file_future.go", content: "//go:build go1.99\n\npackage p\n"},
		{name: "file_legacy.go", content: "// +build linux,amd64\n\npackage p\n"},
		{name: "file_notgo.txt", content: "not a go file"},
	}

	for _, f := range testFiles {
		path := filepath.Join(tempDir, f.name)
		if err := os.WriteFile(path, []byte(f.content), 0644); err != nil {
			t.Fatalf("writing %s: %v", f.name, err)
		}
	}

	contexts := []struct {
		desc string
		bctx build.Context
		tags packagelayout.BuildTags
	}{
		{
			desc: "LinuxAmd64_CgoOn",
			bctx: build.Context{
				GOOS:        "linux",
				GOARCH:      "amd64",
				Compiler:    "gc",
				CgoEnabled:  true,
				ReleaseTags: []string{"go1.1", "go1.20", "go1.26"},
				BuildTags:   []string{"customtag"},
			},
			tags: packagelayout.BuildTags{
				GOOS:        "linux",
				GOARCH:      "amd64",
				Compiler:    "gc",
				CgoEnabled:  true,
				ReleaseTags: []string{"go1.1", "go1.20", "go1.26"},
				BuildTags:   []string{"customtag"},
			},
		},
		{
			desc: "WindowsAmd64_CgoOff",
			bctx: build.Context{
				GOOS:        "windows",
				GOARCH:      "amd64",
				Compiler:    "gc",
				CgoEnabled:  false,
				ReleaseTags: []string{"go1.1", "go1.20"},
				BuildTags:   []string{"othertag"},
			},
			tags: packagelayout.BuildTags{
				GOOS:        "windows",
				GOARCH:      "amd64",
				Compiler:    "gc",
				CgoEnabled:  false,
				ReleaseTags: []string{"go1.1", "go1.20"},
				BuildTags:   []string{"othertag"},
			},
		},
		{
			desc: "DarwinArm64_CgoOn",
			bctx: build.Context{
				GOOS:        "darwin",
				GOARCH:      "arm64",
				Compiler:    "gc",
				CgoEnabled:  true,
				ReleaseTags: []string{"go1.1", "go1.20", "go1.26"},
			},
			tags: packagelayout.BuildTags{
				GOOS:        "darwin",
				GOARCH:      "arm64",
				Compiler:    "gc",
				CgoEnabled:  true,
				ReleaseTags: []string{"go1.1", "go1.20", "go1.26"},
			},
		},
	}

	for _, tc := range contexts {
		t.Run(tc.desc, func(t *testing.T) {
			for _, f := range testFiles {
				path := filepath.Join(tempDir, f.name)
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("reading %s: %v", f.name, err)
				}

				wantMatch, wantErr := tc.bctx.MatchFile(tempDir, f.name)
				gotMatch, gotErr := packagelayout.FileMatchesBuildConstraints(tc.tags, path, src)

				if (wantErr != nil) != (gotErr != nil) {
					t.Errorf("file %s: error mismatch: bctx err=%v, got err=%v", f.name, wantErr, gotErr)
				}
				if wantMatch != gotMatch {
					t.Errorf("file %s: match mismatch: bctx=%v, got=%v", f.name, wantMatch, gotMatch)
				}
			}
		})
	}
}
