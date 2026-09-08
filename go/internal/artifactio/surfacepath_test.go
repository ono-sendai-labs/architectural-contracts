package artifactio_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

// fullKey returns a complete SDK key for tests.
func pathFullKey() *stdlibauthority.SDKKey {
	return &stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
}

// TestSurfacePathConvention pins the native dependency-surface location rule
// (task req 6, AC7): the manifest's extension is deterministically replaced by
// `.surface.json`, and the report convention reserves the alongside
// `.report.json` path. No manifest field or caller-selected path participates.
func TestSurfacePathConvention(t *testing.T) {
	cases := map[string][2]string{
		"store.component.textproto": {"store.component.surface.json", "store.component.report.json"},
		"dep.json":                  {"dep.surface.json", "dep.report.json"},
		"noext":                     {"noext.surface.json", "noext.report.json"},
	}
	for manifestPath, want := range cases {
		if got := artifactio.SurfacePath(manifestPath); got != want[0] {
			t.Errorf("SurfacePath(%q) = %q, want %q", manifestPath, got, want[0])
		}
		if got := artifactio.ReportPath(manifestPath); got != want[1] {
			t.Errorf("ReportPath(%q) = %q, want %q", manifestPath, got, want[1])
		}
	}
}

// TestReadSourcesReadsOnlyRequestedPaths verifies the shell adapter that
// isolates filesystem reads (task req 1): only the supplied member paths are
// read, so dependency, test and unrelated files cannot enter the digest
// (AC4's exclusion half) and path order does not affect the result.
func TestReadSourcesReadsOnlyRequestedPaths(t *testing.T) {
	fsys := fstest.MapFS{
		"src/a.go":        {Data: []byte("package dep // member")},
		"src/b.go":        {Data: []byte("package dep // member")},
		"src/depother.go": {Data: []byte("package other // dependency source")},
		"src/a_test.go":   {Data: []byte("package dep // test file")},
		"src/README.md":   {Data: []byte("unrelated file")},
		"other/c.go":      {Data: []byte("package c // outside root")},
	}
	got, err := artifactio.ReadSources(fsys, []string{"src/b.go", "src/a.go"})
	if err != nil {
		t.Fatalf("ReadSources: %v", err)
	}
	if len(got) != 2 || got[0].Path != "src/a.go" || got[1].Path != "src/b.go" {
		t.Fatalf("ReadSources = %+v, want sorted member sources only", got)
	}
	if string(got[0].Bytes) != "package dep // member" {
		t.Errorf("a.go bytes = %q", got[0].Bytes)
	}

	// A missing member source fails rather than silently digesting less.
	if _, err := artifactio.ReadSources(fsys, []string{"src/a.go", "src/missing.go"}); err == nil {
		t.Errorf("ReadSources with missing member file: want error, got nil")
	}
}

// TestWriteSurface verifies the emission write path: canonical bytes via
// MarshalSurface, atomically placed (task req 6).
func TestWriteSurface(t *testing.T) {
	path := "/tmp/opencode/task02-write/comp.surface.json"
	m, err := surface.Derive(surface.Input{
		Component:       "pb",
		Style:           manifest.InterfaceStylePackageSurface,
		Authority:       manifest.UnknownAuthority(),
		Namespace:       "upstream",
		Key:             pathFullKey(),
		ProducerVersion: "test-1",
		MemberPackages:  []string{"example.com/pb"},
		Manifest:        []byte("name: \"pb\"\n"),
		Sources:         []surface.SourceFile{{Path: "a.go", Bytes: []byte("package pb\n")}},
	})
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if err := os.MkdirAll("/tmp/opencode/task02-write", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := artifactio.WriteSurface(path, m); err != nil {
		t.Fatalf("WriteSurface: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.HasPrefix(string(data), "{") {
		t.Fatalf("surface bytes are not JSON: %q", data[:min(20, len(data))])
	}
	decoded, err := artifactio.DecodeSurface(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("DecodeSurface: %v", err)
	}
	if decoded.Component != "pb" || decoded.InterfaceStyle != gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE {
		t.Errorf("decoded surface = %+v", decoded)
	}
}
