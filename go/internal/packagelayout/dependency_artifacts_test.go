package packagelayout

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestDependencyArtifactBindingsMarshalSortsAndRoundTrips(t *testing.T) {
	layout := &Layout{
		Roots: []string{"example.com/member"},
		Packages: []*packages.Package{{
			ID: "example.com/member", Name: "member", PkgPath: "example.com/member",
		}},
		DependencyArtifactBindings: []DependencyArtifactBinding{
			{
				Dependency:   "zeta",
				Surface:      "workspace/zeta.surface.json",
				Report:       "workspace/zeta.report.json",
				AutoAttached: true,
				Provenance:   DependencyArtifactProvenanceChecked,
			},
			{
				Dependency: "alpha",
				Surface:    "workspace/alpha.surface.json",
				Provenance: DependencyArtifactProvenanceAsserted,
			},
		},
	}
	original := append([]DependencyArtifactBinding(nil), layout.DependencyArtifactBindings...)

	first, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	second, err := json.Marshal(layout)
	if err != nil {
		t.Fatalf("second json.Marshal() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated layout marshals differ:\n%s\n%s", first, second)
	}
	if !reflect.DeepEqual(layout.DependencyArtifactBindings, original) {
		t.Fatalf("marshal mutated dependency bindings: got %#v, want %#v", layout.DependencyArtifactBindings, original)
	}

	var wire struct {
		DependencyArtifactBindings []DependencyArtifactBinding `json:"dependency_artifact_bindings"`
	}
	if err := json.Unmarshal(first, &wire); err != nil {
		t.Fatalf("decode canonical binding JSON: %v", err)
	}
	if got := wire.DependencyArtifactBindings; len(got) != 2 || got[0].Dependency != "alpha" || got[1].Dependency != "zeta" {
		t.Fatalf("canonical dependency binding order = %#v, want alpha then zeta", got)
	}
	if got := wire.DependencyArtifactBindings[1].Report; got != "workspace/zeta.report.json" {
		t.Errorf("report for zeta = %q, want workspace/zeta.report.json", got)
	}

	parsed, err := Parse(bytes.NewReader(first))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !reflect.DeepEqual(parsed.DependencyArtifactBindings, []DependencyArtifactBinding{
		{
			Dependency: "alpha", Surface: "workspace/alpha.surface.json",
			Provenance: DependencyArtifactProvenanceAsserted,
		},
		{
			Dependency: "zeta", Surface: "workspace/zeta.surface.json",
			Report: "workspace/zeta.report.json", AutoAttached: true,
			Provenance: DependencyArtifactProvenanceChecked,
		},
	}) {
		t.Fatalf("parsed dependency bindings = %#v", parsed.DependencyArtifactBindings)
	}
}

func TestParseRejectsInvalidDependencyArtifactBindings(t *testing.T) {
	tests := []struct {
		name      string
		bindings  string
		wantInErr string
	}{
		{name: "empty dependency", bindings: `[{"surface":"dep.surface.json","provenance":"checked"}]`, wantInErr: "dependency name"},
		{name: "empty surface", bindings: `[{"dependency":"dep","provenance":"checked"}]`, wantInErr: "surface path"},
		{name: "checked without report", bindings: `[{"dependency":"dep","surface":"dep.surface.json","provenance":"checked"}]`, wantInErr: "has no report path"},
		{name: "asserted with report", bindings: `[{"dependency":"dep","surface":"dep.surface.json","report":"dep.report.json","provenance":"asserted"}]`, wantInErr: "must not have a report path"},
		{name: "unknown provenance", bindings: `[{"dependency":"dep","surface":"dep.surface.json","provenance":"verified"}]`, wantInErr: "unknown dependency artifact provenance"},
		{name: "duplicate dependency", bindings: `[{"dependency":"dep","surface":"one.surface.json","report":"one.report.json","provenance":"checked"},{"dependency":"dep","surface":"two.surface.json","report":"two.report.json","provenance":"checked"}]`, wantInErr: "duplicate dependency artifact binding"},
		{name: "duplicate surface", bindings: `[{"dependency":"one","surface":"dep.surface.json","report":"one.report.json","provenance":"checked"},{"dependency":"two","surface":"dep.surface.json","report":"two.report.json","provenance":"checked"}]`, wantInErr: "duplicate dependency surface path"},
		{name: "absolute surface", bindings: `[{"dependency":"dep","surface":"/dep.surface.json","provenance":"checked"}]`, wantInErr: "unsafe dependency artifact path"},
		{name: "windows absolute surface", bindings: `[{"dependency":"dep","surface":"C:/dep.surface.json","provenance":"checked"}]`, wantInErr: "unsafe dependency artifact path"},
		{name: "dot surface", bindings: `[{"dependency":"dep","surface":".","provenance":"checked"}]`, wantInErr: "unsafe dependency artifact path"},
		{name: "parent surface", bindings: `[{"dependency":"dep","surface":"../dep.surface.json","provenance":"checked"}]`, wantInErr: "unsafe dependency artifact path"},
		{name: "non-normalized surface", bindings: `[{"dependency":"dep","surface":"frame/../dep.surface.json","provenance":"checked"}]`, wantInErr: "non-normalized dependency artifact path"},
		{name: "backslash surface", bindings: `[{"dependency":"dep","surface":"frame\\dep.surface.json","provenance":"checked"}]`, wantInErr: "unsafe dependency artifact path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"roots":["example.com/member"],"dependency_artifact_bindings":` + tt.bindings + `}`
			_, err := Parse(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), tt.wantInErr) {
				t.Fatalf("Parse() error = %v, want substring %q", err, tt.wantInErr)
			}
		})
	}
}

func TestValidateAndResolveRejectsDirectLayoutDependencyArtifactBinding(t *testing.T) {
	layout := &Layout{
		Roots: []string{"example.com/member"},
		Packages: []*packages.Package{{
			ID: "example.com/member", Name: "member", PkgPath: "example.com/member",
		}},
		DependencyArtifactBindings: []DependencyArtifactBinding{{
			Dependency: "dep", Surface: "frame/../dep.surface.json",
			Provenance: DependencyArtifactProvenanceChecked,
		}},
	}
	if err := ValidateAndResolve(layout, t.TempDir()); err == nil || !strings.Contains(err.Error(), "non-normalized dependency artifact path") {
		t.Fatalf("ValidateAndResolve() error = %v, want binding validation error", err)
	}
}
