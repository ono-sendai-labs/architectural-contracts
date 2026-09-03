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
		Members:        []string{"example.com/app", "example.com/app/internal"},
		InterfaceStyle: gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE,
		ComponentDependencies: []*gen.ComponentDependency{{
			Name:         "runtime",
			Manifest:     "runtime/component.textproto",
			AutoAttached: true,
		}},
	}

	componentFields := component.ProtoReflect().Descriptor().Fields()
	for name, wantNumber := range map[protoreflect.Name]protoreflect.FieldNumber{
		"members":         6,
		"interface_style": 7,
		"authority":       10,
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

	// Authority axis: DECLARED is the zero value so omitted fields default to
	// known-declared; UNKNOWN is the only other accepted value.
	if got := gen.Authority_AUTHORITY_DECLARED; got != 0 {
		t.Errorf("AUTHORITY_DECLARED = %d, want 0", got)
	}
	if got := gen.Authority_AUTHORITY_UNKNOWN; got != 1 {
		t.Errorf("AUTHORITY_UNKNOWN = %d, want 1", got)
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

// TestComponentSchema_RetiredVerificationFieldsReserved asserts that the
// retired self-declared verification fields cannot come back: field numbers 8
// and 9 and their names are reserved, so neither a number nor a name reuse
// compiles into a usable field, and the prior field-4 reservation is intact.
func TestComponentSchema_RetiredVerificationFieldsReserved(t *testing.T) {
	component := &gen.Component{}
	descriptor := component.ProtoReflect().Descriptor()

	for _, number := range []protoreflect.FieldNumber{4, 8, 9} {
		found := false
		for i := 0; i < descriptor.ReservedRanges().Len(); i++ {
			rn := descriptor.ReservedRanges().Get(i)
			if number >= rn[0] && number < rn[1] {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("field number %d is not reserved", number)
		}
	}

	reservedNames := map[protoreflect.Name]bool{}
	names := descriptor.ReservedNames()
	for i := 0; i < names.Len(); i++ {
		reservedNames[names.Get(i)] = true
	}
	for _, name := range []protoreflect.Name{"absorbed_dependencies", "own_check_runs", "certification_reference"} {
		if !reservedNames[name] {
			t.Errorf("field name %q is not reserved", name)
		}
	}

	for _, name := range []protoreflect.Name{"own_check_runs", "certification_reference"} {
		if field := descriptor.Fields().ByName(name); field != nil {
			t.Errorf("field %q still exists with number %d, want removed", name, field.Number())
		}
		if f8 := descriptor.Fields().ByNumber(8); f8 != nil {
			t.Errorf("field number 8 still exists as %q, want removed", f8.Name())
		}
		if f9 := descriptor.Fields().ByNumber(9); f9 != nil {
			t.Errorf("field number 9 still exists as %q, want removed", f9.Name())
		}
	}
}

func TestComponentSchema_LegacyTextprotoDefaultsInterfaceStyle(t *testing.T) {
	const legacy = `name: "legacy"
interface_files: "legacy.go"
component_dependencies {
  name: "dependency"
  manifest: "../dependency/component.textproto"
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
	if !reflect.DeepEqual(component.GetDeclaredAuthority(), []string{"FILES"}) {
		t.Fatalf("legacy declared authority changed: %v", component.GetDeclaredAuthority())
	}
}
