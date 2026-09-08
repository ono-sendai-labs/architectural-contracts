package surface_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

// importerDefault wraps importer.Default so fixtures resolve cleanly.
func importerDefault() types.Importer { return importer.Default() }

// fullKey returns a complete SDK key for tests.
func fullKey() *stdlibauthority.SDKKey {
	return &stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
}

// interfaceOnlyFixture is a two-file package. iface.go is the surviving
// interface file; impl.go declares the concrete method that only the legacy
// implements-closure workaround admits into the check (Step 5 gap).
const interfaceFixture = `
package dep

type Greeter interface{ Greet() string }

type Store[T any] struct{ v T }

func (s Store[T]) Get() T { return s.v }

type target struct{ F int }

func (target) Hit() {}

type DB = target

const MaxSize = 1

var DefaultLimit = MaxSize

func Hello() {}
`

const implFixture = `
package dep

type impl struct{}

func (impl) Greet() string { return "hi" }
`

// parseTwoFiles parses and type-checks the fixture package and returns the
// interface file's AST together with the full-package type info, mirroring
// what member loading supplies.
func parseTwoFiles(t *testing.T) ([]*ast.File, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	iface, err := parser.ParseFile(fset, "iface.go", interfaceFixture, 0)
	if err != nil {
		t.Fatalf("parse iface.go: %v", err)
	}
	impl, err := parser.ParseFile(fset, "impl.go", implFixture, 0)
	if err != nil {
		t.Fatalf("parse impl.go: %v", err)
	}
	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: importerDefault()}
	if _, err := conf.Check("example.com/dep", fset, []*ast.File{iface, impl}, info); err != nil {
		t.Fatalf("check: %v", err)
	}
	return []*ast.File{iface}, info
}

// validInput returns a fully-specified declared-interface input.
func validInput(t *testing.T) surface.Input {
	t.Helper()
	files, info := parseTwoFiles(t)
	auth, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("authority: %v", err)
	}
	return surface.Input{
		Component:       "dep",
		Style:           manifest.InterfaceStyleUnspecified,
		Authority:       auth,
		Namespace:       "upstream",
		Key:             fullKey(),
		ProducerVersion: "test-1",
		MemberPackages:  []string{"example.com/dep"},
		Manifest:        []byte("name: \"dep\"\n"),
		InterfaceFiles:  files,
		InterfaceInfo:   info,
		Sources: []surface.SourceFile{
			{Path: "iface.go", Bytes: []byte("package dep\n")},
		},
	}
}

func TestDerive_DeclaredInterfaceExact(t *testing.T) {
	m, err := surface.Derive(validInput(t))
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	want := []string{
		"(example.com/dep.Store).Get",
		"(example.com/dep.target).Hit",
		"example.com/dep.DB",
		"example.com/dep.DefaultLimit",
		"example.com/dep.Greeter",
		"example.com/dep.Hello",
		"example.com/dep.MaxSize",
		"example.com/dep.Store",
		"example.com/dep.target",
	}
	if !slicesEqual(m.Symbols, want) {
		t.Errorf("symbols = %v, want %v", m.Symbols, want)
	}
	// The workaround-only concrete method must not leak into the surface.
	for _, s := range m.Symbols {
		if strings.Contains(s, ".impl)") {
			t.Errorf("workaround-only method %q leaked into the surface", s)
		}
	}
}

func TestDerive_PackageSurface(t *testing.T) {
	in := surface.Input{
		Component:       "pb",
		Style:           manifest.InterfaceStylePackageSurface,
		Authority:       manifest.UnknownAuthority(),
		Namespace:       "upstream",
		Key:             fullKey(),
		ProducerVersion: "test-1",
		MemberPackages:  []string{"example.com/b", "example.com/a", "example.com/a"},
		Manifest:        []byte("name: \"pb\"\n"),
	}
	m, err := surface.Derive(in)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(m.Symbols) != 0 {
		t.Errorf("PACKAGE_SURFACE must carry no symbols, got %v", m.Symbols)
	}
	want := []string{"example.com/a", "example.com/b"}
	if !slicesEqual(m.Packages, want) {
		t.Errorf("packages = %v, want %v", m.Packages, want)
	}
}

func TestDerive_IdentityFields(t *testing.T) {
	auth, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("authority: %v", err)
	}
	in := validInput(t)
	in.Authority = auth
	m, err := surface.Derive(in)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if m.FormatVersion != 1 {
		t.Errorf("format_version = %d, want 1", m.FormatVersion)
	}
	if m.Component != "dep" {
		t.Errorf("component = %q", m.Component)
	}
	if m.InterfaceStyle != gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED {
		t.Errorf("interface_style = %v", m.InterfaceStyle)
	}
	if m.Authority == nil || m.Authority.Authority != gen.Authority_DECLARED || !slicesEqual(m.Authority.DeclaredAuthority, []string{"FILES"}) {
		t.Errorf("authority = %+v", m.Authority)
	}
	if m.Namespace != "upstream" {
		t.Errorf("namespace = %q", m.Namespace)
	}
	if m.ProducerVersion != "test-1" {
		t.Errorf("producer_version = %q", m.ProducerVersion)
	}
	k := m.SdkKey
	if k == nil || k.ToolchainVersion != "go1.26.4" || k.Goos != "linux" || k.Goarch != "amd64" || k.ClassifierHash != "abc123" || k.MapFormatVersion != 1 {
		t.Errorf("sdk_key = %+v", k)
	}
	if len(m.Digest) != 64 {
		t.Errorf("digest = %q, want 64 hex chars", m.Digest)
	}

	// UNKNOWN authority round-trips structurally.
	in2 := surface.Input{
		Component:       "pb",
		Style:           manifest.InterfaceStylePackageSurface,
		Authority:       manifest.UnknownAuthority(),
		Namespace:       "upstream",
		Key:             fullKey(),
		ProducerVersion: "test-1",
		MemberPackages:  []string{"example.com/pb"},
		Manifest:        []byte("name: \"pb\"\n"),
	}
	m2, err := surface.Derive(in2)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if m2.Authority == nil || m2.Authority.Authority != gen.Authority_UNKNOWN || len(m2.Authority.DeclaredAuthority) != 0 {
		t.Errorf("UNKNOWN authority = %+v", m2.Authority)
	}
}

func TestDerive_IncompleteSDKKeyFailsClosed(t *testing.T) {
	cases := map[string]func(k *stdlibauthority.SDKKey){
		"missing toolchain":   func(k *stdlibauthority.SDKKey) { k.ToolchainVersion = "" },
		"missing goos":        func(k *stdlibauthority.SDKKey) { k.GOOS = "" },
		"missing goarch":      func(k *stdlibauthority.SDKKey) { k.GOARCH = "" },
		"missing classifier":  func(k *stdlibauthority.SDKKey) { k.ClassifierHash = "" },
		"missing map version": func(k *stdlibauthority.SDKKey) { k.MapFormatVersion = 0 },
		"nil key":             func(k *stdlibauthority.SDKKey) {},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput(t)
			k := fullKey()
			if name != "nil key" {
				mutate(k)
			} else {
				k = nil
			}
			in.Key = k
			if _, err := surface.Derive(in); err == nil {
				t.Errorf("Derive with %s: want fail-closed error, got nil", name)
			}
		})
	}
}

func TestDerive_DeterministicBytes(t *testing.T) {
	in1 := validInput(t)
	in2 := validInput(t)
	// Different enumeration order of the same content.
	in2.MemberPackages = []string{"example.com/dep"}
	in2.Sources = []surface.SourceFile{{Path: "iface.go", Bytes: []byte("package dep\n")}}
	m1, err := surface.Derive(in1)
	if err != nil {
		t.Fatalf("Derive 1: %v", err)
	}
	m2, err := surface.Derive(in2)
	if err != nil {
		t.Fatalf("Derive 2: %v", err)
	}
	b1, err := artifactio.MarshalSurface(m1)
	if err != nil {
		t.Fatalf("MarshalSurface 1: %v", err)
	}
	b2, err := artifactio.MarshalSurface(m2)
	if err != nil {
		t.Fatalf("MarshalSurface 2: %v", err)
	}
	if string(b1) != string(b2) {
		t.Errorf("repeated derivation not byte-identical:\n%s\n%s", b1, b2)
	}
}

// TestDerive_CanonicalizesSDKBuildTags pins the SDK-key canonicalization
// (review round 1): reordered build tags derive identical manifests, and a
// duplicate tag fails closed instead of reaching the encoder boundary.
func TestDerive_CanonicalizesSDKBuildTags(t *testing.T) {
	base := validInput(t)
	base.Key = &stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		BuildTags:        []string{"b_tag", "a_tag"},
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
	m, err := surface.Derive(base)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if !slicesEqual(m.SdkKey.BuildTags, []string{"a_tag", "b_tag"}) {
		t.Errorf("build tags = %v, want sorted [a_tag b_tag]", m.SdkKey.BuildTags)
	}

	reordered := base
	reordered.Key = &stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		BuildTags:        []string{"a_tag", "b_tag"},
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
	m2, err := surface.Derive(reordered)
	if err != nil {
		t.Fatalf("Derive reordered: %v", err)
	}
	b1, err := artifactio.MarshalSurface(m)
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	b2, err := artifactio.MarshalSurface(m2)
	if err != nil {
		t.Fatalf("MarshalSurface reordered: %v", err)
	}
	if string(b1) != string(b2) {
		t.Errorf("reordered build tags changed the marshaled manifest:\n%s\n%s", b1, b2)
	}

	duplicated := base
	duplicated.Key = &stdlibauthority.SDKKey{
		ToolchainVersion: "go1.26.4",
		GOOS:             "linux",
		GOARCH:           "amd64",
		BuildTags:        []string{"a_tag", "a_tag"},
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
	if _, err := surface.Derive(duplicated); err == nil {
		t.Errorf("Derive with duplicate build tags: want fail-closed error, got nil")
	}
}

func slicesEqual(a, b []string) bool { return reflect.DeepEqual(a, b) }
