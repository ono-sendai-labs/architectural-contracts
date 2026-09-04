package archcontracts

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	manifestgen "github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

// fieldNumber asserts the field named name has the expected number and kind.
func fieldNumber(t *testing.T, md protoreflect.MessageDescriptor, name protoreflect.Name, want int32, kind protoreflect.Kind) {
	t.Helper()
	fd := md.Fields().ByName(name)
	if fd == nil {
		t.Fatalf("%s: field %q not found", md.FullName(), name)
	}
	if protoreflect.FieldNumber(want) != fd.Number() {
		t.Errorf("%s.%s: field number = %d, want %d", md.FullName(), name, fd.Number(), want)
	}
	if fd.Kind() != kind {
		t.Errorf("%s.%s: kind = %v, want %v", md.FullName(), name, fd.Kind(), kind)
	}
}

// assertNotMap rejects proto map fields: persisted artifacts use repeated
// entries with documented sort keys instead (DR-15).
func assertNotMap(t *testing.T, md protoreflect.MessageDescriptor) {
	t.Helper()
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.IsMap() {
			t.Errorf("%s.%s: proto map field is forbidden (DR-15)", md.FullName(), fd.Name())
		}
	}
}

// TestSurface_FormatVersionIsFieldOne pins that format_version is the first
// field of the top-level surface message (DR-15).
func TestSurface_FormatVersionIsFieldOne(t *testing.T) {
	md := (&manifestgen.SurfaceManifest{}).ProtoReflect().Descriptor()
	fieldNumber(t, md, "format_version", 1, protoreflect.Int32Kind)
}

// TestSurface_FieldNumbersAndTypes pins the surface schema shape: design Data
// Models §The surface manifest, task requirement 1.
func TestSurface_FieldNumbersAndTypes(t *testing.T) {
	md := (&manifestgen.SurfaceManifest{}).ProtoReflect().Descriptor()
	assertNotMap(t, md)
	fieldNumber(t, md, "component", 2, protoreflect.StringKind)
	fieldNumber(t, md, "interface_style", 3, protoreflect.EnumKind)
	fieldNumber(t, md, "authority", 4, protoreflect.MessageKind)
	fieldNumber(t, md, "namespace", 7, protoreflect.StringKind)
	fieldNumber(t, md, "sdk_key", 8, protoreflect.MessageKind)
	fieldNumber(t, md, "producer_version", 9, protoreflect.StringKind)
	fieldNumber(t, md, "digest", 10, protoreflect.StringKind)

	for _, name := range []protoreflect.Name{"packages", "symbols"} {
		fd := md.Fields().ByName(name)
		if fd == nil {
			t.Fatalf("surface: field %q not found", name)
		}
		if !fd.IsList() || fd.Kind() != protoreflect.StringKind {
			t.Errorf("surface.%s: want repeated string (sortable entry list)", name)
		}
	}
}

// TestSurface_AuthorityDistinguishesUnknownFromEmpty checks that the authority
// message carries the manifest Authority enum (R10, R11): UNKNOWN must not be
// conflated with DECLARED{}.
func TestSurface_AuthorityDistinguishesUnknownFromEmpty(t *testing.T) {
	md := (&manifestgen.AuthorityDeclaration{}).ProtoReflect().Descriptor()
	fieldNumber(t, md, "authority", 1, protoreflect.EnumKind)
	fieldNumber(t, md, "declared_authority", 2, protoreflect.StringKind)

	enum := md.Fields().ByName("authority").Enum()
	if got, want := string(enum.FullName()), "archcontracts.v1.Authority"; got != want {
		t.Errorf("authority enum = %q, want %q (must reuse the manifest enum)", got, want)
	}
	values := enum.Values()
	if v := values.ByName("UNKNOWN"); v == nil || v.Number() != 1 {
		t.Errorf("Authority.UNKNOWN = %v, want 1", v)
	}
	if v := values.ByName("DECLARED"); v == nil || v.Number() != 0 {
		t.Errorf("Authority.DECLARED = %v, want 0", v)
	}
}

// TestSurface_InterfaceStyleReusesComponentEnum pins the deliberate reuse of
// the component enum rather than a duplicated definition (task req 4).
func TestSurface_InterfaceStyleReusesComponentEnum(t *testing.T) {
	md := (&manifestgen.SurfaceManifest{}).ProtoReflect().Descriptor()
	enum := md.Fields().ByName("interface_style").Enum()
	if got, want := string(enum.FullName()), "archcontracts.v1.InterfaceStyle"; got != want {
		t.Errorf("interface_style enum = %q, want %q", got, want)
	}
	if enum.Values().ByName("INTERFACE_STYLE_PACKAGE_SURFACE") == nil {
		t.Error("InterfaceStyle missing INTERFACE_STYLE_PACKAGE_SURFACE")
	}
}

// TestSurface_ImportsComponentAndStdlibMap pins the Go/proto import directions
// (task req 4): surface reuses component enums and the shared SDKKey.
func TestSurface_ImportsComponentAndStdlibMap(t *testing.T) {
	fd := (&manifestgen.SurfaceManifest{}).ProtoReflect().Descriptor().ParentFile()
	deps := map[string]bool{}
	for i := 0; i < fd.Imports().Len(); i++ {
		deps[string(fd.Imports().Get(i).Path())] = true
	}
	if !deps["archcontracts/v1/component.proto"] {
		t.Error("surface.proto must import archcontracts/v1/component.proto")
	}
	if !deps["archcontracts/v1/stdlibmap.proto"] {
		t.Error("surface.proto must import archcontracts/v1/stdlibmap.proto for SDKKey")
	}
}

// TestNoMapFields_AnySchema walks every message in all three schemas and
// rejects proto map fields (task requirement 5, DR-15).
func TestNoMapFields_AnySchema(t *testing.T) {
	var fileDescriptors []protoreflect.FileDescriptor
	for _, m := range []proto.Message{
		&manifestgen.Component{},
		&manifestgen.StdlibMap{},
		&manifestgen.SurfaceManifest{},
	} {
		fileDescriptors = append(fileDescriptors, m.ProtoReflect().Descriptor().ParentFile())
	}
	for _, fd := range fileDescriptors {
		walkMessages(t, fd.Messages())
	}
}

func walkMessages(t *testing.T, msgs protoreflect.MessageDescriptors) {
	t.Helper()
	for i := 0; i < msgs.Len(); i++ {
		assertNotMap(t, msgs.Get(i))
		walkMessages(t, msgs.Get(i).Messages())
	}
}

// TestSDKKey_FieldsComplete pins every DR-09 SDK identity component
// (task requirement 2).
func TestSDKKey_FieldsComplete(t *testing.T) {
	md := (&manifestgen.SDKKey{}).ProtoReflect().Descriptor()
	assertNotMap(t, md)
	fieldNumber(t, md, "toolchain_version", 1, protoreflect.StringKind)
	fieldNumber(t, md, "goos", 2, protoreflect.StringKind)
	fieldNumber(t, md, "goarch", 3, protoreflect.StringKind)
	fieldNumber(t, md, "cgo_enabled", 4, protoreflect.BoolKind)
	fieldNumber(t, md, "goexperiment", 6, protoreflect.StringKind)
	fieldNumber(t, md, "classifier_hash", 7, protoreflect.StringKind)
	fieldNumber(t, md, "map_format_version", 8, protoreflect.Int32Kind)
	fd := md.Fields().ByName("build_tags")
	if fd == nil || !fd.IsList() || fd.Kind() != protoreflect.StringKind {
		t.Error("SDKKey.build_tags: want repeated string (sorted)")
	}
}

// TestStdlibMap_FormatVersionIsFieldOne pins format_version first on the map
// (DR-15).
func TestStdlibMap_FormatVersionIsFieldOne(t *testing.T) {
	md := (&manifestgen.StdlibMap{}).ProtoReflect().Descriptor()
	fieldNumber(t, md, "format_version", 1, protoreflect.Int32Kind)
}

// TestStdlibMap_FieldNumbersAndTypes pins the map's collection shape
// (task requirements 2 and 3).
func TestStdlibMap_FieldNumbersAndTypes(t *testing.T) {
	md := (&manifestgen.StdlibMap{}).ProtoReflect().Descriptor()
	assertNotMap(t, md)
	fieldNumber(t, md, "key", 2, protoreflect.MessageKind)
	for _, name := range []protoreflect.Name{"packages", "symbols", "inits", "evidence"} {
		fd := md.Fields().ByName(name)
		if fd == nil || !fd.IsList() || fd.Kind() != protoreflect.MessageKind {
			t.Errorf("stdlibmap.%s: want repeated message entries", name)
		}
	}
}

// TestStdlibMap_ClassificationEnumValues pins the terminal classification
// (task requirement 3): exactly SAFE, non-empty capabilities, or UNANALYZED,
// with UNSPECIFIED guarding the zero value.
func TestStdlibMap_ClassificationEnumValues(t *testing.T) {
	md := (&manifestgen.SymbolRecord{}).ProtoReflect().Descriptor()
	enum := md.Fields().ByName("classification").Enum()
	for _, want := range []struct {
		name  string
		value protoreflect.EnumNumber
	}{{"CLASSIFICATION_UNSPECIFIED", 0}, {"SAFE", 1}, {"CAPABILITIES", 2}, {"UNANALYZED", 3}} {
		v := enum.Values().ByName(protoreflect.Name(want.name))
		if v == nil || v.Number() != want.value {
			t.Errorf("Classification.%s: got %v, want %d", want.name, v, want.value)
		}
	}
}

// TestStdlibMap_NoDependencies pins that stdlibmap.proto is the schema root:
// it imports nothing, so component.proto → stdlibmap.proto → surface.proto is
// an acyclic chain (task req 4).
func TestStdlibMap_NoDependencies(t *testing.T) {
	fd := (&manifestgen.StdlibMap{}).ProtoReflect().Descriptor().ParentFile()
	if got := fd.Imports().Len(); got != 0 {
		t.Errorf("stdlibmap.proto dependencies = %d, want 0", got)
	}
}

// TestStdlibMap_RecordsCarryRequiredData checks that package inventory records
// importability, symbol records carry curated provenance, and evidence frames
// retain the function/file/line data the report needs later (DR-17, task req 3).
func TestStdlibMap_RecordsCarryRequiredData(t *testing.T) {
	pkgMD := (&manifestgen.PackageInventory{}).ProtoReflect().Descriptor()
	fieldNumber(t, pkgMD, "path", 1, protoreflect.StringKind)
	fieldNumber(t, pkgMD, "importable", 2, protoreflect.BoolKind)

	symMD := (&manifestgen.SymbolRecord{}).ProtoReflect().Descriptor()
	fieldNumber(t, symMD, "package", 1, protoreflect.StringKind)
	fieldNumber(t, symMD, "id", 2, protoreflect.StringKind)
	fieldNumber(t, symMD, "classification", 3, protoreflect.EnumKind)
	fieldNumber(t, symMD, "provenance", 5, protoreflect.StringKind)

	frameMD := (&manifestgen.Frame{}).ProtoReflect().Descriptor()
	fieldNumber(t, frameMD, "function", 1, protoreflect.StringKind)
	fieldNumber(t, frameMD, "file", 2, protoreflect.StringKind)
	fieldNumber(t, frameMD, "line", 3, protoreflect.Int32Kind)

	initMD := (&manifestgen.InitRecord{}).ProtoReflect().Descriptor()
	fieldNumber(t, initMD, "package", 1, protoreflect.StringKind)
	fieldNumber(t, initMD, "classification", 2, protoreflect.EnumKind)

	evMD := (&manifestgen.Evidence{}).ProtoReflect().Descriptor()
	fieldNumber(t, evMD, "symbol_id", 1, protoreflect.StringKind)
	fieldNumber(t, evMD, "capability", 2, protoreflect.StringKind)
}

// TestStdlibMapAndSurface_RoundTrip exercises binary and protojson round trips
// on representative surface and map messages (task requirement 8).
func TestStdlibMapAndSurface_RoundTrip(t *testing.T) {
	sdkKey := &manifestgen.SDKKey{
		ToolchainVersion: "go1.26.4",
		Goos:             "darwin",
		Goarch:           "arm64",
		CgoEnabled:       true,
		BuildTags:        []string{"darwin", "unix"},
		Goexperiment:     "arenas",
		ClassifierHash:   "abc123",
		MapFormatVersion: 1,
	}
	surface := &manifestgen.SurfaceManifest{
		FormatVersion:   1,
		Component:       "checker",
		InterfaceStyle:  manifestgen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE,
		Authority:       &manifestgen.AuthorityDeclaration{Authority: manifestgen.Authority_UNKNOWN},
		Packages:        []string{"example.com/a", "example.com/b"},
		Namespace:       "upstream",
		SdkKey:          sdkKey,
		ProducerVersion: "arcc-test",
		Digest:          "deadbeef",
	}
	surfaceDeclared := &manifestgen.SurfaceManifest{
		FormatVersion:  1,
		Component:      "checker",
		InterfaceStyle: manifestgen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED,
		Authority: &manifestgen.AuthorityDeclaration{
			Authority:         manifestgen.Authority_DECLARED,
			DeclaredAuthority: []string{"FILES"},
		},
		Symbols: []string{"checker.Check", "(Checker).Verify"},
	}
	m := &manifestgen.StdlibMap{
		FormatVersion: 1,
		Key:           sdkKey,
		Packages: []*manifestgen.PackageInventory{
			{Path: "internal/goarch", Importable: false},
			{Path: "os", Importable: true},
		},
		Symbols: []*manifestgen.SymbolRecord{
			{Package: "os", Id: "(os.File).Read", Classification: manifestgen.Classification_CAPABILITIES, Capabilities: []string{"FILES"}},
			{Package: "strings", Id: "strings.Title", Classification: manifestgen.Classification_SAFE},
			{Package: "unsafe", Id: "unsafe.Pointer", Classification: manifestgen.Classification_UNANALYZED},
		},
		Inits: []*manifestgen.InitRecord{
			{Package: "net", Classification: manifestgen.Classification_CAPABILITIES, Capabilities: []string{"NETWORK"}},
		},
		Evidence: []*manifestgen.Evidence{
			{SymbolId: "(os.File).Read", Capability: "FILES", Frames: []*manifestgen.Frame{
				{Function: "(os.File).Read", File: "os/file.go", Line: 42},
			}},
		},
	}

	for _, tc := range []struct {
		name string
		msg  proto.Message
	}{{"surface UNKNOWN", surface}, {"surface DECLARED", surfaceDeclared}, {"map", m}} {
		t.Run(tc.name, func(t *testing.T) {
			binaryRoundTrip(t, tc.msg)
			jsonRoundTrip(t, tc.msg)
		})
	}
}

func binaryRoundTrip(t *testing.T, msg proto.Message) {
	t.Helper()
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := msg.ProtoReflect().New().Interface()
	if err := proto.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !proto.Equal(msg, out) {
		t.Errorf("binary round trip changed the message")
	}
}

func jsonRoundTrip(t *testing.T, msg proto.Message) {
	t.Helper()
	data, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatalf("protojson marshal: %v", err)
	}
	out := msg.ProtoReflect().New().Interface()
	if err := protojson.Unmarshal(data, out); err != nil {
		t.Fatalf("protojson unmarshal: %v", err)
	}
	if !proto.Equal(msg, out) {
		t.Errorf("protojson round trip changed the message")
	}
}
