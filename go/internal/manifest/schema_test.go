package manifest_test

import (
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestComponentSchema_AdditiveFieldsAndTextproto(t *testing.T) {
	component := &gen.Component{
		Members:                []string{"example.com/app", "example.com/app/internal"},
		InterfaceStyle:         gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE,
		OwnCheckRuns:           true,
		CertificationReference: "build://component-check",
		ComponentDependencies: []*gen.ComponentDependency{{
			Name:         "runtime",
			Manifest:     "runtime/component.textproto",
			AutoAttached: true,
		}},
	}

	componentFields := component.ProtoReflect().Descriptor().Fields()
	for name, wantNumber := range map[protoreflect.Name]protoreflect.FieldNumber{
		"members":                 6,
		"interface_style":         7,
		"own_check_runs":          8,
		"certification_reference": 9,
	} {
		field := componentFields.ByName(name)
		if field == nil {
			t.Fatalf("component field %q is missing", name)
		}
		if got := field.Number(); got != wantNumber {
			t.Errorf("component field %q number = %d, want %d", name, got, wantNumber)
		}
	}

	dependencyFields := component.GetComponentDependencies()[0].ProtoReflect().Descriptor().Fields()
	autoAttached := dependencyFields.ByName("auto_attached")
	if autoAttached == nil {
		t.Fatal("component dependency field auto_attached is missing")
	}
	if got := autoAttached.Number(); got != 3 {
		t.Errorf("component dependency field auto_attached number = %d, want 3", got)
	}

	if got := gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED; got != 0 {
		t.Errorf("INTERFACE_STYLE_UNSPECIFIED = %d, want 0", got)
	}
	if got := gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE; got != 1 {
		t.Errorf("INTERFACE_STYLE_PACKAGE_SURFACE = %d, want 1", got)
	}

	encoded, err := prototext.Marshal(component)
	if err != nil {
		t.Fatalf("marshal extended component: %v", err)
	}
	var decoded gen.Component
	if err := prototext.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal extended component: %v", err)
	}
	if !reflect.DeepEqual(component, &decoded) {
		t.Errorf("extended component round trip mismatch:\n got: %v\nwant: %v", &decoded, component)
	}
}

func TestComponentSchema_LegacyTextprotoDefaultsInterfaceStyle(t *testing.T) {
	const legacy = `name: "legacy"
interface_files: "legacy.go"
component_dependencies {
  name: "dependency"
  manifest: "../dependency/component.textproto"
}
absorbed_dependencies {
  import_path: "example.com/absorbed"
  reason: "implementation detail"
}
declared_authority: "FILES"
`

	var component gen.Component
	if err := prototext.Unmarshal([]byte(legacy), &component); err != nil {
		t.Fatalf("unmarshal legacy component: %v", err)
	}
	if got := component.GetInterfaceStyle(); got != gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED {
		t.Fatalf("legacy interface style = %s, want %s", got, gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED)
	}
	if component.GetName() != "legacy" || !reflect.DeepEqual(component.GetInterfaceFiles(), []string{"legacy.go"}) {
		t.Fatalf("legacy component fields changed: %v", &component)
	}
	dependencies := component.GetComponentDependencies()
	if len(dependencies) != 1 {
		t.Fatalf("legacy dependency fields changed: %v", component.GetComponentDependencies())
	}
	dependency := dependencies[0]
	if dependency.GetName() != "dependency" || dependency.GetManifest() != "../dependency/component.textproto" || dependency.GetAutoAttached() {
		t.Fatalf("legacy dependency fields changed: %v", dependency)
	}
	absorbedDependencies := component.GetAbsorbedDependencies()
	if len(absorbedDependencies) != 1 {
		t.Fatalf("legacy absorbed dependencies changed: %v", absorbedDependencies)
	}
	absorbed := absorbedDependencies[0]
	if absorbed.GetImportPath() != "example.com/absorbed" || absorbed.GetReason() != "implementation detail" {
		t.Fatalf("legacy absorbed dependency changed: %v", absorbed)
	}
	if !reflect.DeepEqual(component.GetDeclaredAuthority(), []string{"FILES"}) {
		t.Fatalf("legacy declared authority changed: %v", component.GetDeclaredAuthority())
	}
}
