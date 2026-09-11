package manifestparity_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifestparity"
)

func TestDiscoverProductionManifests_RecursesAndExcludesNamedFixtures(t *testing.T) {
	root := t.TempDir()
	writeInventoryFixture(t, root, "go/internal/shallow/component.textproto", `name: "shallow"
interface_files: "shallow.go"
`)
	writeInventoryFixture(t, root, "go/cmd/tool/deep/nested/component.textproto", `name: "nested-command"
interface_files: "command.go"
`)
	writeInventoryFixture(t, root, "go/examples/example/deeper/component.textproto", `name: "nested-example"
interface_files: "example.go"
`)
	// This is deliberately invalid. A named testdata directory is the explicit
	// fixture boundary; the inventory must not parse this file.
	writeInventoryFixture(t, root, "go/examples/example/deeper/testdata/ignored/component.textproto", `name: "ignored"
`)

	inventory, err := manifestparity.DiscoverProductionManifests(root)
	if err != nil {
		t.Fatalf("DiscoverProductionManifests() error = %v", err)
	}
	got := inventory.ByName()
	if len(got) != 3 {
		t.Fatalf("discovered %d manifests, want 3: %v", len(got), got)
	}
	for _, name := range []string{"shallow", "nested-command", "nested-example"} {
		if _, ok := got[name]; !ok {
			t.Errorf("discovered manifests do not contain %q: %v", name, got)
		}
	}
	for _, component := range inventory.Components {
		if strings.Contains(component.Path, "testdata") {
			t.Errorf("fixture manifest was included in inventory: %q", component.Path)
		}
	}
}

func TestDiscoverProductionManifests_DuplicateNamesFailClosed(t *testing.T) {
	root := t.TempDir()
	first := "go/examples/csvtool/internal/one/component.textproto"
	second := "go/examples/csvtool/internal/two/deeper/component.textproto"
	content := `name: "same-component"
interface_files: "api.go"
`
	writeInventoryFixture(t, root, first, content)
	writeInventoryFixture(t, root, second, content)

	inventory, err := manifestparity.DiscoverProductionManifests(root)
	if err == nil {
		t.Fatalf("DiscoverProductionManifests() returned inventory without rejecting duplicate names: %+v", inventory)
	}
	message := err.Error()
	for _, want := range []string{"same-component", filepath.ToSlash(first), filepath.ToSlash(second)} {
		if !strings.Contains(message, want) {
			t.Errorf("duplicate diagnostic %q does not contain %q", message, want)
		}
	}
}

func TestAuditForeignManifestOwnership_RejectsNestedExampleOwner(t *testing.T) {
	root := t.TempDir()
	writeInventoryFixture(t, root, "go/internal/protobufruntime/component.textproto", `name: "protobuf-runtime"
interface_style: INTERFACE_STYLE_PACKAGE_SURFACE
members: "google.golang.org/protobuf/proto"
`)
	foreignPath := "go/examples/csvtool/internal/analysis/deep/component.textproto"
	writeInventoryFixture(t, root, foreignPath, `name: "example-foreign-owner"
interface_files: "api.go"
members: "google.golang.org/protobuf/proto"
`)

	inventory, err := manifestparity.DiscoverProductionManifests(root)
	if err != nil {
		t.Fatalf("DiscoverProductionManifests() error = %v", err)
	}
	if err := manifestparity.AuditForeignManifestOwnership(inventory); err == nil {
		t.Fatal("AuditForeignManifestOwnership() accepted a nested non-wrapper protobuf owner")
	} else {
		message := err.Error()
		for _, want := range []string{"example-foreign-owner", filepath.ToSlash(foreignPath), "protobuf-runtime"} {
			if !strings.Contains(message, want) {
				t.Errorf("foreign-owner diagnostic %q does not contain %q", message, want)
			}
		}
	}
}

func TestDiscoverProductionBazelComponents_RecursesAndAuditsMembersOnly(t *testing.T) {
	root := t.TempDir()
	writeInventoryFixture(t, root, "go/examples/csvtool/internal/foreign/deeper/BUILD.bazel", `
go_component(
    name = "nested-foreign",
    interface_style = PACKAGE_SURFACE,
    members = [
        "@org_golang_google_protobuf//proto:proto",
        "@org_golang_x_tools//go/packages:packages",
    ],
)
`)
	// A malformed fixture below the named exclusion must not affect the
	// production inventory, even though it contains a go_component token.
	writeInventoryFixture(t, root, "go/examples/example/testdata/ignored/BUILD.bazel", `
go_component(members = [not_a_literal])
`)

	inventory, err := manifestparity.DiscoverProductionBazelComponents(root)
	if err != nil {
		t.Fatalf("DiscoverProductionBazelComponents() error = %v", err)
	}
	if len(inventory.Components) != 1 {
		t.Fatalf("discovered %d Bazel components, want 1: %+v", len(inventory.Components), inventory.Components)
	}
	component := inventory.Components[0]
	if component.Name != "nested-foreign" || component.BuildPath != "go/examples/csvtool/internal/foreign/deeper/BUILD.bazel" {
		t.Fatalf("nested Bazel component = %+v", component)
	}
	if len(component.Members) != 2 || component.Members[0].ImportPath != "google.golang.org/protobuf/proto" || component.Members[1].ImportPath != "golang.org/x/tools/go/packages" {
		t.Fatalf("nested Bazel members = %+v", component.Members)
	}
	if err := manifestparity.AuditForeignBazelOwnership(inventory); err == nil {
		t.Fatal("AuditForeignBazelOwnership() accepted a foreign member in a non-wrapper component")
	} else if !strings.Contains(err.Error(), "nested-foreign") || !strings.Contains(err.Error(), component.BuildPath) {
		t.Errorf("foreign Bazel diagnostic = %q, want component and BUILD path", err)
	}
}

func TestAuditForeignBazelOwnership_IgnoresGoLibraryDependencies(t *testing.T) {
	root := t.TempDir()
	writeInventoryFixture(t, root, "go/internal/clean/BUILD.bazel", `
go_library(
    name = "clean",
    importpath = "example.com/clean",
    deps = ["@org_golang_google_protobuf//proto:proto", "@org_golang_x_tools//go/packages:packages"],
)

go_component(
    name = "clean-component",
    interface_style = PACKAGE_SURFACE,
    members = [":clean"],
)
`)

	inventory, err := manifestparity.DiscoverProductionBazelComponents(root)
	if err != nil {
		t.Fatalf("DiscoverProductionBazelComponents() error = %v", err)
	}
	if err := manifestparity.AuditForeignBazelOwnership(inventory); err != nil {
		t.Fatalf("AuditForeignBazelOwnership() treated go_library deps as ownership: %v", err)
	}
}

func TestAuditForeignBazelOwnership_InspectsInterfaceRoot(t *testing.T) {
	root := t.TempDir()
	writeInventoryFixture(t, root, "go/internal/foreign/BUILD.bazel", `
go_library(
    name = "foreign",
    importpath = "google.golang.org/protobuf/proto",
)

go_component(
    name = "bad-interface-owner",
    interface = ":foreign",
)
`)

	inventory, err := manifestparity.DiscoverProductionBazelComponents(root)
	if err != nil {
		t.Fatalf("DiscoverProductionBazelComponents() error = %v", err)
	}
	if err := manifestparity.AuditForeignBazelOwnership(inventory); err == nil {
		t.Fatal("AuditForeignBazelOwnership() accepted a foreign interface root in a non-wrapper component")
	} else if !strings.Contains(err.Error(), "bad-interface-owner") {
		t.Errorf("foreign interface diagnostic = %q, want component name", err)
	}
}

func TestCheckedInProductionInventoriesIncludeNestedExamples(t *testing.T) {
	manifestInventory := checkedInManifestInventory(t)
	wantManifests := []struct {
		name string
		path string
	}{
		{name: "app", path: "go/examples/csvtool/app/component.textproto"},
		{name: "csvfile", path: "go/examples/csvtool/csvfile/component.textproto"},
		{name: "parsecsv", path: "go/examples/csvtool/internal/parsecsv/component.textproto"},
		{name: "toprow", path: "go/examples/csvtool/toprow/component.textproto"},
	}
	for _, want := range wantManifests {
		found := false
		for _, component := range manifestInventory.Components {
			if component.Manifest.Name == want.name && component.Path == want.path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("recursive manifest inventory is missing %q at %q", want.name, want.path)
		}
	}

	bazelInventory := checkedInBazelComponents(t)
	wantBazel := []struct {
		name string
		path string
	}{
		{name: "app_component", path: "go/examples/csvtool/app/BUILD.bazel"},
		{name: "csvfile_component", path: "go/examples/csvtool/csvfile/BUILD.bazel"},
		{name: "parsecsv_component", path: "go/examples/csvtool/internal/parsecsv/BUILD.bazel"},
		{name: "toprow_component", path: "go/examples/csvtool/toprow/BUILD.bazel"},
	}
	for _, want := range wantBazel {
		found := false
		for _, component := range bazelInventory.Components {
			if component.Name == want.name && component.BuildPath == want.path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("recursive Bazel inventory is missing %q at %q", want.name, want.path)
		}
	}
}

func writeInventoryFixture(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
