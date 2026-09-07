package stdlibmap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

// fixtureSDK builds a small SDK tree for the explicit-input tests: fmt, a
// build-excluded pinned package (constrained to a future release), an
// internal package, and unsafe.
func fixtureSDK(t *testing.T) string {
	t.Helper()
	sdkRoot := filepath.Join(t.TempDir(), "sdk", "src")
	write := func(rel, content string) {
		path := filepath.Join(sdkRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("fmt/format.go", "package fmt\n\nfunc Println(s string) {}\n")
	write("strings/strings.go", "package strings\n\nfunc Trim(s string) string { return s }\n")
	write("unsafe/unsafe.go", "package unsafe\n")
	write("pinned/future.go", "//go:build go1.27\n\npackage pinned\n")
	write("internal/testcap/testcap.go", "package testcap\n")
	return sdkRoot
}

func fixtureSDKLayout(t *testing.T, sdkRoot string) *packagelayout.Layout {
	t.Helper()
	layout, err := packagelayout.StdlibLayout(sdkRoot, &packagelayout.Platform{
		GOOS: "linux", GOARCH: "amd64", ToolchainVersion: "go1.26.4",
	})
	if err != nil {
		t.Fatalf("StdlibLayout() error = %v", err)
	}
	return layout
}

// --- target configuration file ---------------------------------------------------

func TestParseTargetConfig(t *testing.T) {
	valid := "toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=false\nbuild_tags=probe,extra\ngoexperiment=\n"
	got, err := ParseTargetConfig(strings.NewReader(valid))
	if err != nil {
		t.Fatalf("ParseTargetConfig() error = %v", err)
	}
	want := TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64", BuildTags: []string{"probe", "extra"}}
	if got.ToolchainVersion != want.ToolchainVersion || got.GOOS != want.GOOS || got.GOARCH != want.GOARCH ||
		got.CgoEnabled != want.CgoEnabled || got.GOEXPERIMENT != want.GOEXPERIMENT ||
		fmt.Sprint(got.BuildTags) != fmt.Sprint(want.BuildTags) {
		t.Fatalf("ParseTargetConfig() = %+v, want %+v", *got, want)
	}

	for _, tc := range []struct {
		name    string
		content string
		wantErr string
	}{
		{"missing key", "toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=false\nbuild_tags=\n", "goexperiment"},
		{"duplicate key", "toolchain_version=go1.26.4\ntoolchain_version=go1.26.5\ngoos=linux\ngoarch=amd64\ncgo_enabled=false\nbuild_tags=\ngoexperiment=\n", `key "toolchain_version" appears twice`},
		{"unknown key", "toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=false\nbuild_tags=\ngoexperiment=\ngoroot=/usr\n", `unknown key "goroot"`},
		{"malformed line", "toolchain_version\ngoos=linux\n", "not key=value"},
		{"malformed cgo", "toolchain_version=go1.26.4\ngoos=linux\ngoarch=amd64\ncgo_enabled=yes\nbuild_tags=\ngoexperiment=\n", `cgo_enabled "yes" is not true or false`},
		{"missing toolchain value", "toolchain_version=\ngoos=linux\ngoarch=amd64\ncgo_enabled=false\nbuild_tags=\ngoexperiment=\n", "toolchain_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTargetConfig(strings.NewReader(tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ParseTargetConfig() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}

	t.Run("render and parse round-trip", func(t *testing.T) {
		in := TargetConfig{ToolchainVersion: "go1.26.4", GOOS: "windows", GOARCH: "arm64", CgoEnabled: false, BuildTags: []string{"a", "b"}, GOEXPERIMENT: "arenayaslicit"}
		rendered, err := RenderTargetConfig(in)
		if err != nil {
			t.Fatalf("RenderTargetConfig() error = %v", err)
		}
		out, err := ParseTargetConfig(strings.NewReader(rendered))
		if err != nil {
			t.Fatalf("ParseTargetConfig() error = %v", err)
		}
		if out.ToolchainVersion != in.ToolchainVersion || out.GOOS != in.GOOS || out.GOARCH != in.GOARCH ||
			out.CgoEnabled != in.CgoEnabled || out.GOEXPERIMENT != in.GOEXPERIMENT ||
			fmt.Sprint(out.BuildTags) != fmt.Sprint(in.BuildTags) {
			t.Fatalf("round trip = %+v, want %+v", *out, in)
		}
	})
}

// --- toolchain package-list file -------------------------------------------------

func TestReadToolchainPackageList(t *testing.T) {
	list := "fmt\nstrings\nunsafe\n\ninternal/testcap\nruntime/cgo\ncmd/compile\n_internal/scratch\nvendor/golang.org/x/net/http2/hpack\n"
	paths, err := ReadToolchainPackageList(strings.NewReader(list))
	if err != nil {
		t.Fatalf("ReadToolchainPackageList() error = %v", err)
	}
	want := []string{"fmt", "strings", "unsafe", "internal/testcap", "runtime/cgo", "vendor/golang.org/x/net/http2/hpack"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if _, err := ReadToolchainPackageList(strings.NewReader("with space\n")); err == nil {
		t.Fatal("a malformed path must fail with an actionable error")
	}
}

// --- oracle reconciliation (task req 6, I3) ---------------------------------------

func TestReconcilePackageList(t *testing.T) {
	sdkRoot := fixtureSDK(t)
	layout := fixtureSDKLayout(t, sdkRoot)

	// The layout discovers fmt, strings, unsafe, internal/testcap; the
	// build-constrained pinned directory is not discovered.
	reconcile := func(list string) ([]PackageEntry, error) {
		paths, err := ReadToolchainPackageList(strings.NewReader(list))
		if err != nil {
			return nil, err
		}
		return ReconcilePackageList(paths, layout, sdkRoot)
	}

	t.Run("total agreement", func(t *testing.T) {
		entries, err := reconcile("fmt\nstrings\nunsafe\ninternal/testcap\n")
		if err != nil {
			t.Fatalf("ReconcilePackageList() error = %v", err)
		}
		got := map[string]bool{}
		for _, e := range entries {
			got[e.Path] = e.Importable
		}
		if !got["fmt"] || !got["strings"] || !got["unsafe"] {
			t.Fatalf("importable packages missing: %v", got)
		}
		if got["internal/testcap"] {
			t.Fatalf("internal package enumerated importable: %v", got)
		}
	})

	t.Run("build-excluded listed package is importable false", func(t *testing.T) {
		entries, err := reconcile("fmt\nstrings\nunsafe\ninternal/testcap\npinned\n")
		if err != nil {
			t.Fatalf("ReconcilePackageList() error = %v", err)
		}
		for _, e := range entries {
			if e.Path == "pinned" && e.Importable {
				t.Fatalf("build-excluded listed package enumerated importable: %+v", e)
			}
		}
	})

	t.Run("listed package with no directory fails naming it", func(t *testing.T) {
		_, err := reconcile("fmt\nnosuchpkg\n")
		if err == nil || !strings.Contains(err.Error(), "nosuchpkg") {
			t.Fatalf("ReconcilePackageList() error = %v, want naming nosuchpkg", err)
		}
	})

	t.Run("omitted discovered package fails naming it", func(t *testing.T) {
		_, err := reconcile("fmt\ninternal/testcap\nunsafe\n")
		if err == nil || !strings.Contains(err.Error(), "strings") {
			t.Fatalf("ReconcilePackageList() error = %v, want naming strings", err)
		}
	})
}
