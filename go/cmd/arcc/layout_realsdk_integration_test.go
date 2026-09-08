//go:build integration

package main_test

import (
	"encoding/json"
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rest of the layout-mode integration suite builds a *synthetic* SDK tree
// (createLayoutFixture's mock_sdk, three hand-written packages). That is fast
// and hermetic, but a synthetic tree omits exactly what breaks against a real
// SDK — the `unsafe` pseudo-package, cgo's `C` pseudo-import, the 360-package
// `std` graph, and the GOROOT vendor rewrite — the defects Step 5a's stdlib
// enumeration exists to handle. This test is the automated form of the by-hand
// Step 5a demo: a layout that names *only* the member package, its stdlib edges
// recovered by arcc and its stdlib packages enumerated from a genuine
// `$GOROOT/src`, checked with no Go toolchain on PATH.
func TestIntegration_LayoutMode_RealSDK(t *testing.T) {
	sdkSrc := filepath.Join(build.Default.GOROOT, "src")
	if info, err := os.Stat(sdkSrc); err != nil || !info.IsDir() {
		t.Skipf("no Go SDK source tree at %s; skipping real-SDK layout test", sdkSrc)
	}

	// A member that imports two stdlib packages — one that mints authority
	// (os.Open → FILES) and one that does not (strings) — with neither edge
	// recorded in the layout's Imports map. Recovering both from the source is
	// Step 5a's job, so this exercises it end to end.
	memberSrc := `package member

import (
	"os"
	"strings"
)

func OpenSomething() (*os.File, error) {
	return os.Open(strings.ToLower("/etc/hostname"))
}
`

	// writeFixture lays out a workspace with the member source, a manifest, and
	// a layout whose Imports map is deliberately empty and whose go_sdk_root is
	// the caller's choice. Returns the absolute workspace, manifest and layout
	// paths.
	writeFixture := func(t *testing.T, manifest, goSDKRoot string) (string, string, string) {
		t.Helper()
		ws := t.TempDir()

		memberDir := filepath.Join(ws, "member")
		if err := os.MkdirAll(memberDir, 0755); err != nil {
			t.Fatalf("mkdir member: %v", err)
		}
		if err := os.WriteFile(filepath.Join(memberDir, "api.go"), []byte(memberSrc), 0644); err != nil {
			t.Fatalf("write member: %v", err)
		}

		manifestPath := filepath.Join(ws, "component.textproto")
		if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}

		layout := map[string]any{
			"go_sdk_root": goSDKRoot,
			"roots":       []string{"example.com/member"},
			"packages": []map[string]any{{
				"id":              "example.com/member",
				"name":            "member",
				"pkgPath":         "example.com/member",
				"goFiles":         []string{"member/api.go"},
				"compiledGoFiles": []string{"member/api.go"},
				// Empty on purpose: arcc recovers os and strings itself.
				"imports": map[string]string{},
			}},
		}
		data, err := json.MarshalIndent(layout, "", "  ")
		if err != nil {
			t.Fatalf("marshal layout: %v", err)
		}
		layoutPath := filepath.Join(ws, "package-layout.json")
		if err := os.WriteFile(layoutPath, data, 0644); err != nil {
			t.Fatalf("write layout: %v", err)
		}
		return ws, manifestPath, layoutPath
	}

	// runFrom runs arcc hermetically with its working directory at ws, the
	// frame the relative goFiles resolve against.
	runFrom := func(t *testing.T, ws string, args []string) (string, string, int) {
		t.Helper()
		origWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if err := os.Chdir(ws); err != nil {
			t.Fatalf("chdir %s: %v", ws, err)
		}
		defer os.Chdir(origWd)
		return runArccHermetic(t, args)
	}

	t.Run("declared authority conforms", func(t *testing.T) {
		manifest := `name: "cleanmember"
interface_files: "member/api.go"
declared_authority: "FILES"
`
		ws, manifestPath, layoutPath := writeFixture(t, manifest, sdkSrc)
		stdout, stderr, code := runFrom(t, ws, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

		if code != 0 {
			t.Fatalf("expected exit 0, got %d.\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		if want := `Component "cleanmember" conforms`; !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout, want)
		}
	})

	t.Run("undeclared authority is a violation", func(t *testing.T) {
		manifest := `name: "leakymember"
interface_files: "member/api.go"
`
		ws, manifestPath, layoutPath := writeFixture(t, manifest, sdkSrc)
		stdout, stderr, code := runFrom(t, ws, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

		if code != 1 {
			t.Fatalf("expected exit 1, got %d.\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "UNDECLARED_AUTHORITY") {
			t.Errorf("expected UNDECLARED_AUTHORITY in stdout: %s", stdout)
		}
		if !strings.Contains(stdout, `use of undeclared authority "FILES"`) {
			t.Errorf("expected FILES violation in stdout: %s", stdout)
		}
	})

	t.Run("missing member source is a tool error", func(t *testing.T) {
		// The negative control: a layout that names a member file the
		// workspace does not contain is a load-graph fault - the check
		// fails as a tool error (exit 2) rather than reporting a spurious
		// verdict. (Standard-library decisions no longer depend on the
		// layout's SDK enumeration: the declared map decides them.)
		manifest := `name: "cleanmember"
interface_files: "member/api.go"
declared_authority: "FILES"
`
		ws, manifestPath, layoutPath := writeFixture(t, manifest, "")
		layoutBytes, err := os.ReadFile(layoutPath)
		if err != nil {
			t.Fatalf("read layout: %v", err)
		}
		mutated := strings.Replace(string(layoutBytes), "member/api.go", "member/nonexistent.go", 1)
		if err := os.WriteFile(layoutPath, []byte(mutated), 0o644); err != nil {
			t.Fatalf("write mutated layout: %v", err)
		}
		stdout, stderr, code := runFrom(t, ws, []string{"check", manifestPath, "--package-layout=" + layoutPath, "--stdlib-map=" + sharedNativeMapDefault(t)})

		if code != 2 {
			t.Fatalf("expected exit 2, got %d.\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
}
