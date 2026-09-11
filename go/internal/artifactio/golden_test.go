package artifactio

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

func TestShapeSnapshotReportRetainsContractAndUsesNamedPlaceholders(t *testing.T) {
	input := report.ConformanceReport{
		Component: "shape-report",
		Dependencies: []report.DependencyBoundary{
			{
				Component:         "asserted-dep",
				Provenance:        report.DependencyProvenanceAsserted,
				Freshness:         report.DependencyFreshnessBuildGraph,
				Authority:         report.DependencyAuthorityUnknown,
				DeclaredAuthority: nil,
			},
			{
				Component:         "checked-dep",
				Provenance:        report.DependencyProvenanceCheckedPass,
				Freshness:         report.DependencyFreshnessBuildGraph,
				Authority:         report.DependencyAuthorityDeclared,
				DeclaredAuthority: []string{"FILES", "NETWORK"},
			},
		},
		Diagnostics: report.Diagnostics{
			NonMemberExportArtifactCount: 7,
			NonMemberExportBytes:         99,
		},
		Violations: []report.Finding{
			{
				Kind:     report.UndeclaredAuthority,
				Message:  "use of undeclared authority \"FILES\"",
				Location: report.Location{File: "host/path/member.go", Line: 4},
				Evidence: []string{"host/path/member.go:4", "os.Open at :0"},
				Class:    "TrueAuthority",
				SDKKey:   "sdk{target-specific}",
				Sites: []report.AuthoritySite{
					{File: "host/path/member.go", Line: 4, Symbol: "os.Open"},
				},
			},
		},
		Warnings: []report.Finding{},
	}
	data, err := MarshalReport(input)
	if err != nil {
		t.Fatalf("MarshalReport: %v", err)
	}
	got, err := Snapshot(ShapeReport, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Snapshot(report): %v", err)
	}

	want := `{
  "format_version": 1,
  "verdict": "fail",
  "report": {
    "component": "shape-report",
    "dependencies": [
      {
        "component": "asserted-dep",
        "provenance": "ASSERTED",
        "freshness": "BUILD_GRAPH",
        "authority": "UNKNOWN",
        "declared_authority": []
      },
      {
        "component": "checked-dep",
        "provenance": "CHECKED_PASS",
        "freshness": "BUILD_GRAPH",
        "authority": "DECLARED",
        "declared_authority": [
          "FILES",
          "NETWORK"
        ]
      }
    ],
    "diagnostics": {
      "non_member_export_artifact_count": "<EXPORT_ARTIFACT_COUNT>",
      "non_member_export_bytes": "<EXPORT_BYTES>"
    },
    "violations": [
      {
        "kind": "UNDECLARED_AUTHORITY",
        "message": "<FINDING_MESSAGE>",
        "location": {
          "file": "<SOURCE_PATH>",
          "line": 4
        },
        "evidence": [
          "<EVIDENCE>",
          "<EVIDENCE>"
        ],
        "class": "TrueAuthority",
        "sdk_key": "<SDK_KEY>",
        "sites": [
          {
            "file": "<SOURCE_PATH>",
            "line": 4,
            "symbol": "os.Open"
          }
        ]
      }
    ],
    "warnings": []
  }
}
`
	if string(got) != want {
		t.Errorf("report shape snapshot differs:\n%s", got)
	}
}

func TestShapeSnapshotSurfaceMakesDefaultSemanticsExplicit(t *testing.T) {
	input := validSurface()
	input.InterfaceStyle = gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED
	input.Authority = &gen.AuthorityDeclaration{Authority: gen.Authority_DECLARED}
	input.Namespace = "upstream"
	input.SdkKey = &gen.SDKKey{
		ToolchainVersion: "go1.26.4",
		Goos:             "linux",
		Goarch:           "amd64",
		ClassifierHash:   "classifier",
		MapFormatVersion: 1,
	}
	input.ProducerVersion = "arcc test"
	input.Digest = "0123456789abcdef"
	data, err := MarshalSurface(input)
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	got, err := Snapshot(ShapeSurface, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Snapshot(surface): %v", err)
	}
	for _, want := range []string{
		`"interfaceStyle": "INTERFACE_STYLE_UNSPECIFIED"`,
		`"authority": "DECLARED"`,
		`"digest": "<CONTENT_DIGEST>"`,
		`"toolchainVersion": "<SDK_TOOLCHAIN_VERSION>"`,
		`"classifierHash": "<SDK_CLASSIFIER_HASH>"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("surface snapshot missing %q:\n%s", want, got)
		}
	}
}

func TestShapeSnapshotMapRetainsTerminalClassesAndOrdering(t *testing.T) {
	input := validMap()
	input.Symbols = append(input.Symbols, &gen.SymbolRecord{
		Package:        "bytes",
		Id:             "bytes.Reader",
		Classification: gen.Classification_UNANALYZED,
	})
	data, err := MarshalMap(input)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	got, err := Snapshot(ShapeMap, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Snapshot(map): %v", err)
	}
	text := string(got)
	for _, want := range []string{
		`"classification": "SAFE"`,
		`"classification": "CAPABILITIES"`,
		`"classification": "UNANALYZED"`,
		`"packages": [`,
		`"symbols": [`,
		`"inits": [`,
		`"evidence": [`,
		`"classifierHash": "<SDK_CLASSIFIER_HASH>"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("map snapshot missing %q:\n%s", want, got)
		}
	}
}

func TestShapeSnapshotsAreByteIdenticalAcrossLogicalOrderings(t *testing.T) {
	surfaceA := validSurface()
	surfaceA.SdkKey = &gen.SDKKey{
		ToolchainVersion: "go1.26.4",
		Goos:             "linux",
		Goarch:           "amd64",
		ClassifierHash:   "classifier",
		MapFormatVersion: 1,
	}
	surfaceB := validSurface()
	surfaceB.SdkKey = &gen.SDKKey{
		ToolchainVersion: "go1.26.4",
		Goos:             "linux",
		Goarch:           "amd64",
		ClassifierHash:   "classifier",
		MapFormatVersion: 1,
	}
	surfaceB.Packages = []string{"b.example/pkg", "a.example/pkg"}
	surfaceB.Symbols = []string{"(a.example/pkg.T).M", "a.example/pkg.F"}
	surfaceSnapshot := func(m *gen.SurfaceManifest) []byte {
		t.Helper()
		data, err := MarshalSurface(m)
		if err != nil {
			t.Fatalf("MarshalSurface: %v", err)
		}
		snapshot, err := Snapshot(ShapeSurface, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Snapshot(surface): %v", err)
		}
		return snapshot
	}
	if got, want := string(surfaceSnapshot(surfaceB)), string(surfaceSnapshot(surfaceA)); got != want {
		t.Errorf("reordered surface snapshot differs:\n%s\n---\n%s", got, want)
	}

	mapA := validMap()
	mapB := validMap()
	mapB.Packages = []*gen.PackageInventory{mapB.Packages[2], mapB.Packages[0], mapB.Packages[1]}
	mapB.Symbols = []*gen.SymbolRecord{mapB.Symbols[1], mapB.Symbols[0]}
	mapB.Inits = []*gen.InitRecord{mapB.Inits[1], mapB.Inits[0]}
	mapB.Evidence = []*gen.Evidence{mapB.Evidence[1], mapB.Evidence[0]}
	mapB.Symbols[0].Capabilities = []string{"READ_SYSTEM_STATE", "FILES"}
	mapSnapshot := func(m *gen.StdlibMap) []byte {
		t.Helper()
		data, err := MarshalMap(m)
		if err != nil {
			t.Fatalf("MarshalMap: %v", err)
		}
		snapshot, err := Snapshot(ShapeMap, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Snapshot(map): %v", err)
		}
		return snapshot
	}
	if got, want := string(mapSnapshot(mapB)), string(mapSnapshot(mapA)); got != want {
		t.Errorf("reordered map snapshot differs:\n%s\n---\n%s", got, want)
	}

	reportA := report.ConformanceReport{
		Component: "shape",
		Dependencies: []report.DependencyBoundary{
			{Component: "zeta", Provenance: report.DependencyProvenanceCheckedPass, Freshness: report.DependencyFreshnessBuildGraph, Authority: report.DependencyAuthorityDeclared},
			{Component: "alpha", Provenance: report.DependencyProvenanceAsserted, Freshness: report.DependencyFreshnessBuildGraph, Authority: report.DependencyAuthorityUnknown},
		},
		Violations: []report.Finding{
			{Kind: report.UndeclaredDependency, Message: "z", Location: report.Location{File: "z.go", Line: 2}},
			{Kind: report.UndeclaredAuthority, Message: "a", Location: report.Location{File: "a.go", Line: 1}},
		},
	}
	reportB := report.ConformanceReport{
		Component:    reportA.Component,
		Dependencies: []report.DependencyBoundary{reportA.Dependencies[1], reportA.Dependencies[0]},
		Violations:   []report.Finding{reportA.Violations[1], reportA.Violations[0]},
	}
	reportSnapshot := func(r report.ConformanceReport) []byte {
		t.Helper()
		data, err := MarshalReport(r)
		if err != nil {
			t.Fatalf("MarshalReport: %v", err)
		}
		snapshot, err := Snapshot(ShapeReport, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Snapshot(report): %v", err)
		}
		return snapshot
	}
	if got, want := string(reportSnapshot(reportB)), string(reportSnapshot(reportA)); got != want {
		t.Errorf("reordered report snapshot differs:\n%s\n---\n%s", got, want)
	}
}

func TestShapeSnapshotCanonicalizesOnlyThroughProductionBoundary(t *testing.T) {
	surfaceData, err := MarshalSurface(validSurface())
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	mapData, err := MarshalMap(validMap())
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	reportData, err := MarshalReport(report.ConformanceReport{Component: "clean"})
	if err != nil {
		t.Fatalf("MarshalReport: %v", err)
	}

	tests := []struct {
		name string
		kind ShapeKind
		data []byte
		want string
	}{
		{name: "malformed report", kind: ShapeReport, data: []byte("{not json"), want: "parse report artifact"},
		{name: "wrong report version", kind: ShapeReport, data: []byte(`{"format_version":2,"verdict":"pass","report":{"component":"clean"}}`), want: "unsupported report format version"},
		{name: "oversized report", kind: ShapeReport, data: []byte(strings.Repeat("x", int(MaxReportBytes)+1)), want: "byte limit"},
		{name: "non-canonical report", kind: ShapeReport, data: append(reportData, '\n'), want: "not canonical"},
		{name: "wrong surface version", kind: ShapeSurface, data: []byte(`{"formatVersion":2}`), want: "unsupported artifact format version"},
		{name: "non-canonical surface", kind: ShapeSurface, data: append(surfaceData, ' '), want: "not canonical"},
		{name: "wrong map version", kind: ShapeMap, data: []byte(`{"formatVersion":99}`), want: "unsupported artifact format version"},
		{name: "non-canonical map", kind: ShapeMap, data: append(mapData, ' '), want: "not canonical"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Snapshot(tc.kind, bytes.NewReader(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Snapshot() error = %v, want text %q", err, tc.want)
			}
		})
	}
}

func TestCompareShapeReportsFocusedDiff(t *testing.T) {
	input := validSurface()
	input.SdkKey = &gen.SDKKey{
		ToolchainVersion: "go1.26.4",
		Goos:             "linux",
		Goarch:           "amd64",
		ClassifierHash:   "classifier",
		MapFormatVersion: 1,
	}
	data, err := MarshalSurface(input)
	if err != nil {
		t.Fatalf("MarshalSurface: %v", err)
	}
	golden, err := Snapshot(ShapeSurface, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Snapshot(surface): %v", err)
	}
	changed := strings.Replace(string(golden), `"component": "widget"`, `"component": "changed"`, 1)
	if changed == string(golden) {
		t.Fatal("test setup did not change the component")
	}
	err = CompareShape(ShapeSurface, bytes.NewReader(data), strings.NewReader(changed))
	if err == nil {
		t.Fatal("CompareShape accepted a changed stable field")
	}
	if !strings.Contains(err.Error(), "component") || !strings.Contains(err.Error(), "widget") || !strings.Contains(err.Error(), "changed") {
		t.Errorf("CompareShape error = %v, want focused component diff", err)
	}
}
