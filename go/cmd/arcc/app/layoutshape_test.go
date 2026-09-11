package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/packages"
)

func TestLayoutShapeSnapshotPreservesFinalLayoutContract(t *testing.T) {
	data := marshalLayoutShapeFixture(t, "example.com")

	snapshot, err := snapshotLayoutShape(data)
	if err != nil {
		t.Fatalf("snapshotLayoutShape: %v", err)
	}
	text := string(snapshot)
	for _, want := range []string{
		`"go_sdk_root": "<GO_SDK_ROOT>"`,
		`"goos": "<TARGET_GOOS>"`,
		`"GoFiles": [`,
		`"IgnoredFiles": [`,
		`"OtherFiles": [`,
		`"ExportFile": "<EXPORT_ROOT>/dep.x"`,
		`"Imports": {`,
		`"ordinary_import_data":`,
		`"stdlib_export_data":`,
		`"dependency_artifact_bindings":`,
		`"surface": "<ARTIFACT_ROOT>/dep.surface.json"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("layout shape missing %q:\n%s", want, snapshot)
		}
	}
	for _, forbidden := range []string{
		`"go_sdk_root": "rules_go++`,
		`_main/`,
		`bazel-out/`,
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("layout shape leaked host path %q:\n%s", forbidden, snapshot)
		}
	}
}

func TestLayoutShapeSnapshotCanonicalizesImportPaths(t *testing.T) {
	originalCanonicalize := hostpolicy.CanonicalizePath
	originalIsCanonical := hostpolicy.IsCanonicalPath
	t.Cleanup(func() {
		hostpolicy.CanonicalizePath = originalCanonicalize
		hostpolicy.IsCanonicalPath = originalIsCanonical
	})

	hostpolicy.CanonicalizePath = func(path string) string {
		return strings.Replace(path, "host.example/", "example.com/", 1)
	}
	hostpolicy.IsCanonicalPath = func(path string) bool {
		return hostpolicy.CanonicalizePath(path) == path
	}
	rewritten, err := snapshotLayoutShape(marshalLayoutShapeFixture(t, "host.example"))
	if err != nil {
		t.Fatalf("snapshot rewritten layout: %v", err)
	}

	hostpolicy.CanonicalizePath = originalCanonicalize
	hostpolicy.IsCanonicalPath = originalIsCanonical
	canonical, err := snapshotLayoutShape(marshalLayoutShapeFixture(t, "example.com"))
	if err != nil {
		t.Fatalf("snapshot canonical layout: %v", err)
	}
	if !bytes.Equal(rewritten, canonical) {
		t.Fatalf("rewritten layout shape differs from canonical layout:\nrewritten:\n%s\ncanonical:\n%s", rewritten, canonical)
	}
}

func TestLayoutShapeSnapshotIsDeterministicAcrossRepeatedUpdates(t *testing.T) {
	first := layoutShapeFixture("example.com")
	second := layoutShapeFixture("example.com")
	first.Packages[1].GoFiles = append(first.Packages[1].GoFiles, "_main/dep/dep_extra.go")
	first.Packages[1].CompiledGoFiles = append(first.Packages[1].CompiledGoFiles, "_main/dep/dep_extra.go")
	second.Packages[1].GoFiles = append(second.Packages[1].GoFiles, "_main/dep/dep_extra.go")
	second.Packages[1].CompiledGoFiles = append(second.Packages[1].CompiledGoFiles, "_main/dep/dep_extra.go")
	second.Packages[0], second.Packages[1] = second.Packages[1], second.Packages[0]
	second.Packages[0].GoFiles[0], second.Packages[0].GoFiles[1] = second.Packages[0].GoFiles[1], second.Packages[0].GoFiles[0]
	second.Packages[0].CompiledGoFiles[0], second.Packages[0].CompiledGoFiles[1] = second.Packages[0].CompiledGoFiles[1], second.Packages[0].CompiledGoFiles[0]
	second.StdlibExportData.ExportRoots[0], second.StdlibExportData.ExportRoots[1] =
		second.StdlibExportData.ExportRoots[1], second.StdlibExportData.ExportRoots[0]

	firstSnapshot, err := snapshotLayoutShape(marshalCanonicalLayoutShapeFixture(t, first))
	if err != nil {
		t.Fatalf("snapshot first layout: %v", err)
	}
	secondSnapshot, err := snapshotLayoutShape(marshalCanonicalLayoutShapeFixture(t, second))
	if err != nil {
		t.Fatalf("snapshot reordered layout: %v", err)
	}
	repeatedSnapshot, err := snapshotLayoutShape(marshalCanonicalLayoutShapeFixture(t, first))
	if err != nil {
		t.Fatalf("snapshot repeated layout: %v", err)
	}
	if !bytes.Equal(firstSnapshot, secondSnapshot) || !bytes.Equal(firstSnapshot, repeatedSnapshot) {
		t.Fatalf("logical reorder or repeated shape update changed bytes:\nfirst:\n%s\nreordered:\n%s\nrepeated:\n%s", firstSnapshot, secondSnapshot, repeatedSnapshot)
	}
}

func TestLayoutShapeSnapshotRejectsInvalidInputBeforeNormalization(t *testing.T) {
	base := layoutShapeFixture("example.com")
	canonical := marshalCanonicalLayoutShapeFixture(t, base)

	tests := []struct {
		name   string
		layout *packagelayout.Layout
		data   []byte
		want   string
	}{
		{name: "non-canonical bytes", data: append(canonical, ' '), want: "not canonical"},
		{name: "oversized bytes", data: bytes.Repeat([]byte{'x'}, int(maxLayoutShapeBytes)+1), want: "byte limit"},
		{name: "absolute sdk root", layout: func() *packagelayout.Layout {
			copy := layoutShapeFixture("example.com")
			copy.GoSDKRoot = "/sdk/src"
			return copy
		}(), want: "go_sdk_root"},
		{name: "non-member export missing", layout: func() *packagelayout.Layout {
			copy := layoutShapeFixture("example.com")
			copy.Packages[1].ExportFile = ""
			return copy
		}(), want: "ExportFile"},
		{name: "non-member imports omitted", layout: func() *packagelayout.Layout {
			copy := layoutShapeFixture("example.com")
			copy.Packages[1].Imports = nil
			return copy
		}(), want: "Imports"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.data
			if tt.layout != nil {
				data = marshalCanonicalLayoutShapeFixture(t, tt.layout)
			}
			if _, err := snapshotLayoutShape(data); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("snapshotLayoutShape error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestCompareLayoutShapeReportsFocusedDiff(t *testing.T) {
	data := marshalLayoutShapeFixture(t, "example.com")
	want, err := snapshotLayoutShape(data)
	if err != nil {
		t.Fatalf("snapshotLayoutShape: %v", err)
	}
	changed := strings.Replace(string(want), `"roots": [`, `"roots": [
    "example.com/changed",`, 1)
	if changed == string(want) {
		t.Fatal("test setup did not change layout shape")
	}
	if err := compareLayoutShape(data, []byte(changed)); err == nil ||
		!strings.Contains(err.Error(), "changed") {
		t.Fatalf("compareLayoutShape error = %v, want focused layout diff", err)
	}
}

func TestArtifactShapeCommandSupportsLayout(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "component.package-layout.json")
	goldenPath := filepath.Join(dir, "component.layout.shape.golden.json")
	data := marshalLayoutShapeFixture(t, "example.com")
	want, err := snapshotLayoutShape(data)
	if err != nil {
		t.Fatalf("snapshotLayoutShape: %v", err)
	}
	if err := os.WriteFile(artifactPath, data, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if err := os.WriteFile(goldenPath, want, 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := (&Runner{}).Run([]string{
		"artifact-shape", "layout", artifactPath, "--golden=" + goldenPath,
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("artifact-shape layout exit code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("successful layout shape comparison wrote stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func marshalLayoutShapeFixture(t *testing.T, prefix string) []byte {
	t.Helper()
	return marshalCanonicalLayoutShapeFixture(t, layoutShapeFixture(prefix))
}

func marshalCanonicalLayoutShapeFixture(t *testing.T, layout *packagelayout.Layout) []byte {
	t.Helper()
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		t.Fatalf("marshal layout fixture: %v", err)
	}
	return append(data, '\n')
}

func layoutShapeFixture(prefix string) *packagelayout.Layout {
	toolchain := "go1.26.4"
	dep := &packages.Package{
		ID:              prefix + "/dep",
		Name:            "dep",
		PkgPath:         prefix + "/dep",
		GoFiles:         []string{"_main/dep/dep.go"},
		CompiledGoFiles: []string{"_main/dep/dep.go"},
		ExportFile:      "_main/exports/dep.x",
		Imports:         map[string]*packages.Package{},
	}
	member := &packages.Package{
		ID:              prefix + "/member",
		Name:            "member",
		PkgPath:         prefix + "/member",
		GoFiles:         []string{"_main/member/member.go"},
		CompiledGoFiles: []string{"_main/member/member.go"},
		IgnoredFiles:    []string{"_main/member/ignored.go"},
		OtherFiles:      []string{"_main/member/stub.s"},
		Imports: map[string]*packages.Package{
			prefix + "/dep": dep,
		},
	}
	return &packagelayout.Layout{
		GoSDKRoot: "rules_go++go_sdk+main___download_0_linux_amd64/src",
		Platform: &packagelayout.Platform{
			GOOS:             "linux",
			GOARCH:           "amd64",
			BuildTags:        []string{},
			CgoEnabled:       false,
			ToolchainVersion: &toolchain,
		},
		Roots:    []string{prefix + "/member"},
		Packages: []*packages.Package{member, dep},
		OrdinaryImportData: &packagelayout.OrdinaryImportData{
			Metadata: "__ARCC_ORDINARY_IMPORT_DATA__",
		},
		StdlibExportData: &packagelayout.StdlibExportData{
			Metadata: "rules_go+/stdlib_/stdlib.pkg.json",
			Target: &packagelayout.StdlibExportTarget{
				ToolchainVersion: "go1.26.4",
				GOOS:             "linux",
				GOARCH:           "amd64",
				BuildTags:        []string{},
				GOEXPERIMENT:     "",
			},
			ExportRoots: []packagelayout.StdlibExportRoot{
				{RunfilesPath: "rules_go+/stdlib_/gocache", ExecPath: "bazel-out/k8-fastbuild/bin/external/rules_go+/stdlib_/gocache"},
				{RunfilesPath: "rules_go+/stdlib_/pkg", ExecPath: "bazel-out/k8-fastbuild/bin/external/rules_go+/stdlib_/pkg"},
			},
		},
		DependencyArtifactBindings: []packagelayout.DependencyArtifactBinding{
			{
				Dependency: "dep",
				Surface:    "_main/dep/dep.surface.json",
				Report:     "_main/dep/dep.report.json",
				Provenance: packagelayout.DependencyArtifactProvenanceChecked,
			},
		},
	}
}
