//go:build integration

package goanalysis

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func TestMemberAnalysisScaling_Primary(t *testing.T) {
	depths := []int{1, 4, 16}
	widths := []int{1, 2, 4}
	var baseline *scalingSample
	byDepth := make(map[int]map[int]scalingSample, len(depths))
	byWidth := make(map[int]map[int]scalingSample, len(widths))

	for _, depth := range depths {
		byDepth[depth] = make(map[int]scalingSample, len(widths))
		for _, width := range widths {
			fixture := buildScalingFixture(t, scalingFixtureParameters{
				Depth: depth,
				Width: width,
			})
			sample := measureScalingSample(t, fixture)
			if baseline == nil {
				baseline = &sample
			} else {
				if !reflect.DeepEqual(sample.Workload, baseline.Workload) {
					t.Errorf("depth=%d width=%d member workload = %#v, want %#v", depth, width, sample.Workload, baseline.Workload)
				}
				if !reflect.DeepEqual(sample.Facts.References, baseline.Facts.References) {
					t.Errorf("depth=%d width=%d typed references changed: got %#v, want %#v", depth, width, sample.Facts.References, baseline.Facts.References)
				}
				if !reflect.DeepEqual(sample.Facts.Imports, baseline.Facts.Imports) {
					t.Errorf("depth=%d width=%d imports changed: got %#v, want %#v", depth, width, sample.Facts.Imports, baseline.Facts.Imports)
				}
			}
			wantArtifacts := uint64(1 + depth*width)
			if got := sample.Facts.ExportDataDiagnostics.NonMemberExportArtifactCount; got != wantArtifacts {
				t.Errorf("depth=%d width=%d export artifact count = %d, want exact count %d", depth, width, got, wantArtifacts)
			}
			assertScalingGraph(t, sample.Graph, fixture)
			byDepth[depth][width] = sample
			byWidth[width] = ensureScalingSamples(byWidth[width])
			byWidth[width][depth] = sample
			t.Logf("primary depth=%d width=%d raw=%s median=%s artifacts=%d bytes=%d", depth, width, formatScalingSamples(sample.Phases), formatScalingPhases(sample.Phases), sample.Facts.ExportDataDiagnostics.NonMemberExportArtifactCount, sample.Facts.ExportDataDiagnostics.NonMemberExportBytes)
		}
	}

	for _, depth := range depths {
		previous := byDepth[depth][widths[0]].Facts.ExportDataDiagnostics
		for _, width := range widths[1:] {
			current := byDepth[depth][width].Facts.ExportDataDiagnostics
			assertScalingVolumeIncreases(t, "width", depth, widths[0], width, previous, current)
			previous = current
		}
	}
	for _, width := range widths {
		previous := byWidth[width][depths[0]].Facts.ExportDataDiagnostics
		for _, depth := range depths[1:] {
			current := byWidth[width][depth].Facts.ExportDataDiagnostics
			assertScalingVolumeIncreases(t, "depth", width, depths[0], depth, previous, current)
			previous = current
		}
	}

	assertScanTimingFlat(t, baseline, byDepth, byWidth)
}

func TestMemberAnalysisScaling_DirectTypeSurface(t *testing.T) {
	typeSurfaces := []int{0, 8, 32, 128}
	var baseline *scalingSample
	var previous facts.ExportDataDiagnostics
	for _, typeSurface := range typeSurfaces {
		fixture := buildScalingFixture(t, scalingFixtureParameters{
			Depth:       1,
			Width:       1,
			TypeSurface: typeSurface,
		})
		sample := measureScalingSample(t, fixture)
		if got, want := sample.Facts.ExportDataDiagnostics.NonMemberExportArtifactCount, uint64(2); got != want {
			t.Errorf("type surface=%d export artifact count = %d, want exact count %d", typeSurface, got, want)
		}
		assertScalingGraph(t, sample.Graph, fixture)
		if baseline == nil {
			baseline = &sample
		} else {
			if !reflect.DeepEqual(sample.Workload, baseline.Workload) {
				t.Errorf("type surface=%d member workload = %#v, want %#v", typeSurface, sample.Workload, baseline.Workload)
			}
			if !reflect.DeepEqual(sample.Facts.References, baseline.Facts.References) {
				t.Errorf("type surface=%d typed references changed: got %#v, want %#v", typeSurface, sample.Facts.References, baseline.Facts.References)
			}
			if !reflect.DeepEqual(sample.Facts.Imports, baseline.Facts.Imports) {
				t.Errorf("type surface=%d imports changed: got %#v, want %#v", typeSurface, sample.Facts.Imports, baseline.Facts.Imports)
			}
			if sample.Facts.ExportDataDiagnostics.NonMemberExportBytes <= previous.NonMemberExportBytes {
				t.Errorf("type surface=%d export bytes = %d, want growth beyond %d", typeSurface, sample.Facts.ExportDataDiagnostics.NonMemberExportBytes, previous.NonMemberExportBytes)
			}
		}
		previous = sample.Facts.ExportDataDiagnostics
		if baseline != nil {
			got := medianPhase(sample.Phases, analysisPhaseScanReferences)
			bound := medianPhase(baseline.Phases, analysisPhaseScanReferences)*20 + 20*time.Millisecond
			if got > bound {
				t.Errorf("direct type-surface scan median for %d declarations = %s, want no more than %s", typeSurface, got, bound)
			}
		}
		t.Logf("direct type surface=%d raw=%s median=%s artifacts=%d bytes=%d", typeSurface, formatScalingSamples(sample.Phases), formatScalingPhases(sample.Phases), previous.NonMemberExportArtifactCount, previous.NonMemberExportBytes)
	}
	if baseline == nil {
		t.Fatal("direct type-surface series produced no baseline sample")
	}
}

func TestMemberAnalysisScaling_CheckPathDependencyAudit(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed while locating the Go module")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	command := exec.Command("go", "list", "-deps", "./internal/goanalysis")
	command.Dir = moduleRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps for the member-only check path: %v\n%s", err, output)
	}
	for _, forbidden := range []string{
		"golang.org/x/tools/go/ssa",
		"golang.org/x/tools/go/callgraph/vta",
		"github.com/google/capslock",
	} {
		if strings.Contains(string(output), forbidden) {
			t.Errorf("member-only check dependency graph contains forbidden analyzer package %q", forbidden)
		}
	}
}

func TestMemberAnalysisScaling_RepeatedFactsAreTimingIndependent(t *testing.T) {
	fixture := buildScalingFixture(t, scalingFixtureParameters{Depth: 4, Width: 2})
	sample := measureScalingSample(t, fixture)
	encoded, err := json.Marshal(sample.Facts)
	if err != nil {
		t.Fatalf("marshalling measured facts: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("marshalled measured facts are empty")
	}
	if len(sample.ReportBytes) == 0 || len(sample.SurfaceBytes) == 0 {
		t.Fatalf("canonical artifacts are empty: report=%d surface=%d", len(sample.ReportBytes), len(sample.SurfaceBytes))
	}
	if !scalingPhasesHaveDistinctDurations(sample.Phases) {
		t.Fatalf("retained phase observations are identical; want natural duration variation to exercise artifact independence")
	}
	if len(sample.Phases[analysisPhaseLoadPackageFacts]) != scalingSampleCount || len(sample.Phases[analysisPhaseLoadPackages]) != scalingSampleCount || len(sample.Phases[analysisPhaseScanReferences]) != scalingSampleCount {
		t.Fatalf("phase observations = %#v, want %d observations for each phase", sample.Phases, scalingSampleCount)
	}
}

func ensureScalingSamples(samples map[int]scalingSample) map[int]scalingSample {
	if samples == nil {
		return make(map[int]scalingSample)
	}
	return samples
}

func assertScalingVolumeIncreases(t *testing.T, axis string, fixed, previousValue, currentValue int, previous, current facts.ExportDataDiagnostics) {
	t.Helper()
	if current.NonMemberExportArtifactCount <= previous.NonMemberExportArtifactCount {
		t.Errorf("%s fixed=%d values %d→%d artifact count = %d, want growth beyond %d", axis, fixed, previousValue, currentValue, current.NonMemberExportArtifactCount, previous.NonMemberExportArtifactCount)
	}
	if current.NonMemberExportBytes <= previous.NonMemberExportBytes {
		t.Errorf("%s fixed=%d values %d→%d export bytes = %d, want growth beyond %d", axis, fixed, previousValue, currentValue, current.NonMemberExportBytes, previous.NonMemberExportBytes)
	}
}

func assertScanTimingFlat(t *testing.T, baseline *scalingSample, byDepth map[int]map[int]scalingSample, byWidth map[int]map[int]scalingSample) {
	t.Helper()
	if baseline == nil {
		t.Fatal("primary scaling produced no baseline sample")
	}
	minimum := medianPhase(baseline.Phases, analysisPhaseScanReferences)
	for _, samples := range []map[int]scalingSample{byDepth[1], byDepth[4], byDepth[16], byWidth[1], byWidth[2], byWidth[4]} {
		for _, sample := range samples {
			got := medianPhase(sample.Phases, analysisPhaseScanReferences)
			// ScanReferences is member-only work. The exact workload assertions
			// above are the primary invariant; this broad factor-plus-slack guard
			// catches a material closure-correlated regression without treating
			// scheduler noise at microsecond scale as a failure.
			if got > minimum*20+20*time.Millisecond {
				t.Errorf("scan median %s exceeds flatness bound %s", got, minimum*20+20*time.Millisecond)
			}
		}
	}
}
