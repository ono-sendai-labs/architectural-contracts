package manifestparity_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

// retainedXToolsMembers is the exact post-Step-6 closure retained by the
// self-hosting analyzer and map-generator shells. It includes the Capslock
// generation path's SSA/VTA packages even though those packages no longer run
// during ordinary component checks.
var retainedXToolsMembers = []string{
	"golang.org/x/mod/semver",
	"golang.org/x/sync/errgroup",
	"golang.org/x/tools/go/ast/astutil",
	"golang.org/x/tools/go/ast/edge",
	"golang.org/x/tools/go/ast/inspector",
	"golang.org/x/tools/go/buildutil",
	"golang.org/x/tools/go/callgraph",
	"golang.org/x/tools/go/callgraph/internal/chautil",
	"golang.org/x/tools/go/callgraph/vta",
	"golang.org/x/tools/go/callgraph/vta/internal/trie",
	"golang.org/x/tools/go/gcexportdata",
	"golang.org/x/tools/go/internal/cgo",
	"golang.org/x/tools/go/loader",
	"golang.org/x/tools/go/packages",
	"golang.org/x/tools/go/ssa",
	"golang.org/x/tools/go/ssa/ssautil",
	"golang.org/x/tools/go/types/objectpath",
	"golang.org/x/tools/go/types/typeutil",
	"golang.org/x/tools/internal/aliases",
	"golang.org/x/tools/internal/event",
	"golang.org/x/tools/internal/event/core",
	"golang.org/x/tools/internal/event/keys",
	"golang.org/x/tools/internal/event/label",
	"golang.org/x/tools/internal/gcimporter",
	"golang.org/x/tools/internal/gocommand",
	"golang.org/x/tools/internal/packagesinternal",
	"golang.org/x/tools/internal/pkgbits",
	"golang.org/x/tools/internal/stdlib",
	"golang.org/x/tools/internal/typeparams",
	"golang.org/x/tools/internal/typesinternal",
	"golang.org/x/tools/internal/versions",
}

// TestXToolsHasOneConcreteOwnerAndBuildParity audits the complete foreign
// boundary. The manifest is the native import-path spelling and BUILD is the
// Bazel label spelling; both must enumerate the same retained closure exactly.
func TestXToolsHasOneConcreteOwnerAndBuildParity(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	wrapper, ok := manifests["x-tools"]
	if !ok {
		t.Fatal("checked-in manifest for component x-tools not found")
	}
	if wrapper.InterfaceStyle != manifest.InterfaceStylePackageSurface {
		t.Fatalf("x-tools interface style = %v, want PACKAGE_SURFACE", wrapper.InterfaceStyle)
	}
	if wrapper.Authority.Known || len(wrapper.DeclaredAuthority) != 0 {
		t.Fatalf("x-tools authority = %+v/%v, want UNKNOWN with no declared authority", wrapper.Authority, wrapper.DeclaredAuthority)
	}
	if !slices.IsSorted(wrapper.Members) || hasDuplicate(wrapper.Members) {
		t.Fatalf("x-tools manifest members are not sorted and unique: %v", wrapper.Members)
	}
	if !slices.Equal(wrapper.Members, retainedXToolsMembers) {
		t.Fatalf("x-tools manifest members = %v, want retained closure %v", wrapper.Members, retainedXToolsMembers)
	}

	buildPath := filepath.Join("..", "xtools", "BUILD.bazel")
	buildMembers := xToolsBuildMembers(t, string(mustRead(t, buildPath)))
	if !slices.Equal(buildMembers, wrapper.Members) {
		t.Fatalf("x-tools BUILD members = %v, want checked-in manifest members %v", buildMembers, wrapper.Members)
	}

	for name, component := range manifests {
		if name == "x-tools" {
			continue
		}
		for _, member := range component.Members {
			if isXToolsBoundaryPackage(member) {
				t.Errorf("foreign package %q is also a member of component %q", member, name)
			}
		}
	}
}

// TestXToolsConsumersDeclareTheSharedBoundary ensures that every native
// self-hosting consumer uses the one wrapper edge after the source-backed
// dependency resolver cutover. The Go library imports remain ordinary compile
// dependencies; this test audits architectural ownership declarations only.
func TestXToolsConsumersDeclareTheSharedBoundary(t *testing.T) {
	manifests := checkedInComponentManifests(t)
	for _, name := range []string{"goanalysis", "capslockadapter", "stdlibmap"} {
		component, ok := manifests[name]
		if !ok {
			t.Fatalf("checked-in manifest for component %q not found", name)
		}
		found := false
		for _, dep := range component.ComponentDependencies {
			if dep.Name == "x-tools" && dep.Manifest == "../xtools/component.textproto" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("component %q does not declare the shared x-tools boundary", name)
		}
	}
}

// TestXToolsNativeSurfaceIsAsserted checks the checked-in native convention
// artifact. It carries only the package-level UNKNOWN assertion and the
// target identity; no source-derived symbols, digest, or report is present.
func TestXToolsNativeSurfaceIsAsserted(t *testing.T) {
	const surfacePath = "../xtools/component.surface.json"
	data := mustRead(t, surfacePath)
	var got assertedSurfaceShape
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode %s: %v", surfacePath, err)
	}
	wrapper, err := manifest.Parse(bytes.NewReader(mustRead(t, "../xtools/component.textproto")))
	if err != nil {
		t.Fatalf("parse x-tools manifest: %v", err)
	}
	if got.FormatVersion != 1 || got.Component != "x-tools" || got.InterfaceStyle != "INTERFACE_STYLE_PACKAGE_SURFACE" {
		t.Fatalf("native x-tools surface identity = %+v", got)
	}
	if got.Authority.Authority != "UNKNOWN" || len(got.Symbols) != 0 || got.Digest != "" {
		t.Fatalf("native x-tools surface is not an empty-digest UNKNOWN assertion: %+v", got)
	}
	if !slices.Equal(got.Packages, wrapper.Members) || !slices.IsSorted(got.Packages) {
		t.Fatalf("native x-tools surface packages = %v, want sorted manifest members %v", got.Packages, wrapper.Members)
	}
	if got.Namespace != "upstream" || got.SDKKey.ToolchainVersion != "go1.26.4" || got.SDKKey.GOOS != "linux" || got.SDKKey.GOARCH != "amd64" || got.SDKKey.CgoEnabled || len(got.SDKKey.BuildTags) != 0 || got.SDKKey.Goexperiment != "" || got.SDKKey.ClassifierHash == "" || got.SDKKey.MapFormatVersion != 1 || got.ProducerVersion == "" {
		t.Fatalf("native x-tools surface target identity = %+v", got)
	}

	var pinned struct {
		Key assertedSDKKey `json:"key"`
	}
	if err := json.Unmarshal(mustRead(t, "../teststdlibmap/testdata/linux_amd64.stdlib-map.json"), &pinned); err != nil {
		t.Fatalf("decode pinned stdlib map: %v", err)
	}
	if !reflectEqualSDKKey(got.SDKKey, pinned.Key) {
		t.Fatalf("native x-tools surface SDK key = %+v, want pinned map key %+v", got.SDKKey, pinned.Key)
	}
	if _, err := os.Stat("../xtools/component.report.json"); !os.IsNotExist(err) {
		t.Fatalf("asserted x-tools convention has a report artifact: %v", err)
	}
}

type assertedSurfaceShape struct {
	FormatVersion  int    `json:"formatVersion"`
	Component      string `json:"component"`
	InterfaceStyle string `json:"interfaceStyle"`
	Authority      struct {
		Authority string `json:"authority"`
	} `json:"authority"`
	Packages        []string       `json:"packages"`
	Symbols         []string       `json:"symbols"`
	Namespace       string         `json:"namespace"`
	SDKKey          assertedSDKKey `json:"sdkKey"`
	ProducerVersion string         `json:"producerVersion"`
	Digest          string         `json:"digest"`
}

type assertedSDKKey struct {
	ToolchainVersion string   `json:"toolchainVersion"`
	GOOS             string   `json:"goos"`
	GOARCH           string   `json:"goarch"`
	CgoEnabled       bool     `json:"cgoEnabled"`
	BuildTags        []string `json:"buildTags"`
	Goexperiment     string   `json:"goexperiment"`
	ClassifierHash   string   `json:"classifierHash"`
	MapFormatVersion int      `json:"mapFormatVersion"`
}

func reflectEqualSDKKey(a, b assertedSDKKey) bool {
	return a.ToolchainVersion == b.ToolchainVersion &&
		a.GOOS == b.GOOS &&
		a.GOARCH == b.GOARCH &&
		a.CgoEnabled == b.CgoEnabled &&
		slices.Equal(a.BuildTags, b.BuildTags) &&
		a.Goexperiment == b.Goexperiment &&
		a.ClassifierHash == b.ClassifierHash &&
		a.MapFormatVersion == b.MapFormatVersion
}

func xToolsBuildMembers(t *testing.T, build string) []string {
	t.Helper()
	inMembers := false
	var members []string
	for _, raw := range strings.Split(build, "\n") {
		line := strings.TrimSpace(raw)
		if line == "members = [" {
			inMembers = true
			continue
		}
		if !inMembers {
			continue
		}
		if line == "]," {
			break
		}
		line = strings.TrimSuffix(line, ",")
		line = strings.Trim(line, `"`)
		var importPrefix string
		switch {
		case strings.HasPrefix(line, "@org_golang_x_mod//"):
			importPrefix = "golang.org/x/mod/"
			line = strings.TrimPrefix(line, "@org_golang_x_mod//")
		case strings.HasPrefix(line, "@org_golang_x_sync//"):
			importPrefix = "golang.org/x/sync/"
			line = strings.TrimPrefix(line, "@org_golang_x_sync//")
		case strings.HasPrefix(line, "@org_golang_x_tools//"):
			importPrefix = "golang.org/x/tools/"
			line = strings.TrimPrefix(line, "@org_golang_x_tools//")
		default:
			t.Fatalf("unexpected x-tools component member label %q", line)
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || filepath.Base(parts[0]) != parts[1] {
			t.Fatalf("malformed x-tools component member label %q", line)
		}
		members = append(members, importPrefix+parts[0])
	}
	if !inMembers || len(members) == 0 {
		t.Fatal("x-tools BUILD target has no external member labels")
	}
	if !slices.IsSorted(members) || hasDuplicate(members) {
		t.Fatalf("x-tools BUILD member labels are not sorted and unique: %v", members)
	}
	return members
}

func isXToolsBoundaryPackage(pkg string) bool {
	return strings.HasPrefix(pkg, "golang.org/x/tools/") ||
		strings.HasPrefix(pkg, "golang.org/x/mod/") ||
		strings.HasPrefix(pkg, "golang.org/x/sync/")
}
