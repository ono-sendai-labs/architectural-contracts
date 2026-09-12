//go:build integration

package goanalysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

const (
	fullBuildDeterminismDependency = "//bazel_rules/go/tests/testdata/determinism:dependency_component"
	fullBuildDeterminismConsumer   = "//bazel_rules/go/tests/testdata/determinism:consumer_component"
	fullBuildDeterminismUnknown    = "//bazel_rules/go/tests/testdata/determinism:unknown_component"
)

var fullBuildDeterminismComponents = map[string]bool{
	"dependency_component": true,
	"consumer_component":   true,
	"unknown_component":    true,
}

type fullBuildArtifact struct {
	Logical string
	Kind    string
	Path    string
	Digest  string
	Bytes   []byte
}

// assertFullBuildArtifactsAreDeterministic runs the same producer graph in
// two isolated Bazel output bases. The shared routine producer-chain run
// materializes the real default map once; both deterministic graph builds use
// the checked canonical copy with the same semantic configuration. That keeps
// routine CI at one expensive whole-SDK generation while still rebuilding
// every component producer independently in the second tree.
func assertFullBuildArtifactsAreDeterministic(t *testing.T, baseline hermeticityBazelRun) {
	t.Helper()
	assertFullBuildArtifactsAgainstBaseline(t, baseline)
}

func assertFullBuildArtifactsAgainstBaseline(t *testing.T, baseline hermeticityBazelRun) {
	t.Helper()
	firstArtifacts := collectFullBuildArtifacts(t, baseline.Execroot)
	assertFullBuildArtifactSemantics(t, firstArtifacts)
	convenienceBefore, err := snapshotHermeticityConvenienceLinks(baseline.RepoRoot)
	if err != nil {
		t.Fatalf("snapshotting convenience links before second full determinism build: %v", err)
	}

	secondRunDir := t.TempDir()
	secondOutputUserRoot := filepath.Join(secondRunDir, "bazel-user-root")
	secondOutputBase := filepath.Join(secondOutputUserRoot, "output-base")
	secondWritablePath := filepath.Join(secondRunDir, "sandbox-writable")
	secondPoisonRoot := filepath.Join(secondRunDir, "poison")
	if err := os.MkdirAll(secondWritablePath, 0o700); err != nil {
		t.Fatalf("creating second restricted sandbox writable directory: %v", err)
	}
	secondPoisonBefore := createHermeticityPoisonTree(t, secondPoisonRoot)
	secondEnv, _ := createHermeticityEnvironment(t, secondRunDir, secondWritablePath, secondPoisonRoot)
	assertHermeticityPath(t, filepath.Join(secondRunDir, "restricted-path"))
	secondBazelPath := hermeticityBazelPath(t)
	t.Cleanup(func() {
		command := hermeticityShutdownCommand(secondBazelPath, baseline.RepoRoot, secondOutputUserRoot, secondOutputBase, secondEnv)
		_ = command.Run()
		makeProducerChainTreeWritable(secondRunDir)
	})
	secondLabels := []string{
		fullBuildDeterminismDependency,
		fullBuildDeterminismUnknown,
		fullBuildDeterminismConsumer,
	}
	secondPrefetchArgs := []string{
		"--output_user_root=" + secondOutputUserRoot,
		"--output_base=" + secondOutputBase,
		"fetch",
		"--repository_cache=" + baseline.RepositoryCache,
		"--experimental_convenience_symlinks=ignore",
		"--noshow_progress",
	}
	secondPrefetchArgs = append(secondPrefetchArgs, secondLabels...)
	prefetchOutput, prefetchErr := runHermeticityBazelCommand(
		t,
		secondBazelPath,
		baseline.RepoRoot,
		hermeticityPrefetchEnvironment(),
		secondPrefetchArgs...,
	)
	if prefetchErr != nil {
		t.Fatalf("preparing second full determinism Bazel output root failed: %v\n%s", prefetchErr, prefetchOutput)
	}
	secondLog := filepath.Join(secondRunDir, "full-determinism-second.execution.json")
	secondArgs := hermeticityBazelArgs(
		"build",
		secondOutputUserRoot,
		secondOutputBase,
		baseline.RepositoryCache,
		secondWritablePath,
		secondPoisonRoot,
		secondLog,
		"",
		secondLabels,
	)
	secondOutput, secondErr := runHermeticityBazelCommand(t, secondBazelPath, baseline.RepoRoot, secondEnv, secondArgs...)
	assertHermeticityPoisonUnchanged(t, secondPoisonRoot, secondPoisonBefore, "second full determinism build")
	assertHermeticityConvenienceLinksUnchanged(t, baseline.RepoRoot, convenienceBefore, "second full determinism build")
	if secondErr != nil {
		t.Fatalf("second full determinism Bazel build failed: %v\n%s", secondErr, secondOutput)
	}
	secondActions := readHermeticityExecutionLogIfNonEmpty(t, secondLog)
	if maps := actionsWithMnemonic(secondActions, "ArccStdlibMap"); len(maps) != 0 {
		t.Fatalf("second full determinism build executed %d stdlib maps; it must consume the checked canonical map", len(maps))
	}
	secondExecroot := filepath.Join(secondOutputBase, "execroot", "_main")
	secondArtifacts := collectFullBuildArtifacts(t, secondExecroot)
	assertFullBuildArtifactSemantics(t, secondArtifacts)
	compareFullBuildArtifacts(t, firstArtifacts, secondArtifacts)
	logFullBuildArtifactDigests(t, firstArtifacts)
}

func hermeticityShutdownCommand(bazelPath, repoRoot, outputUserRoot, outputBase string, environment []string) *exec.Cmd {
	command := exec.Command(
		bazelPath,
		"--output_user_root="+outputUserRoot,
		"--output_base="+outputBase,
		"shutdown",
	)
	command.Dir = repoRoot
	command.Env = environment
	return command
}

func collectFullBuildArtifacts(t *testing.T, execroot string) map[string]fullBuildArtifact {
	t.Helper()
	outputRoot := filepath.Join(execroot, "bazel-out")
	resolvedRoot, err := filepath.EvalSymlinks(outputRoot)
	if err != nil {
		t.Fatalf("resolving Bazel output root %q: %v", outputRoot, err)
	}
	artifacts := make(map[string]fullBuildArtifact)
	err = filepath.WalkDir(resolvedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".stdlib-map.json") &&
			!strings.HasSuffix(path, ".surface.json") && !strings.HasSuffix(path, ".report.json")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %q: %w", path, err)
		}
		logical, kind := fullBuildArtifactIdentity(t, path, data)
		if !fullBuildArtifactExpected(logical) {
			return nil
		}
		digest := sha256.Sum256(data)
		artifact := fullBuildArtifact{
			Logical: logical,
			Kind:    kind,
			Path:    path,
			Digest:  hex.EncodeToString(digest[:]),
			Bytes:   append([]byte(nil), data...),
		}
		if previous, exists := artifacts[logical]; exists {
			if !bytes.Equal(previous.Bytes, data) {
				return fmt.Errorf("logical artifact %q has different bytes at %q and %q", logical, previous.Path, path)
			}
			return nil
		}
		artifacts[logical] = artifact
		return nil
	})
	if err != nil {
		t.Fatalf("collecting full-build artifacts under %q: %v", resolvedRoot, err)
	}
	return artifacts
}

func fullBuildArtifactIdentity(t *testing.T, path string, data []byte) (string, string) {
	t.Helper()
	switch {
	case strings.HasSuffix(path, ".stdlib-map.json"):
		decoded, err := artifactio.DecodeMap(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decoding map artifact %q: %v", path, err)
		}
		if decoded.Key == nil {
			t.Fatalf("map artifact %q has no SDK key", path)
		}
		return "stdlib-map:" + fullBuildSDKKey(decoded.Key), "stdlib-map"
	case strings.HasSuffix(path, ".surface.json"):
		decoded, err := artifactio.DecodeSurface(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("decoding surface artifact %q: %v", path, err)
		}
		return "surface:" + decoded.Component, "surface"
	case strings.HasSuffix(path, ".report.json"):
		decoded, err := artifactio.DecodeReport(data)
		if err != nil {
			t.Fatalf("decoding report artifact %q: %v", path, err)
		}
		return "report:" + decoded.Report.Component, "report"
	default:
		t.Fatalf("unsupported full-build artifact %q", path)
		return "", ""
	}
}

func fullBuildArtifactExpected(logical string) bool {
	if strings.HasPrefix(logical, "stdlib-map:") {
		return true
	}
	for component := range fullBuildDeterminismComponents {
		if logical == "surface:"+component || logical == "report:"+component {
			return true
		}
	}
	return false
}

func fullBuildSDKKey(key *gen.SDKKey) string {
	return strings.Join([]string{
		key.ToolchainVersion,
		key.Goos,
		key.Goarch,
		fmt.Sprintf("%t", key.CgoEnabled),
		strings.Join(key.BuildTags, ","),
		key.Goexperiment,
		key.ClassifierHash,
		fmt.Sprintf("%d", key.MapFormatVersion),
	}, "/")
}

func assertFullBuildArtifactSemantics(t *testing.T, artifacts map[string]fullBuildArtifact) {
	t.Helper()
	want := map[string]bool{
		"surface:dependency_component": true,
		"surface:consumer_component":   true,
		"surface:unknown_component":    true,
		"report:dependency_component":  true,
		"report:consumer_component":    true,
	}
	mapCount := 0
	for logical, artifact := range artifacts {
		assertNoTimingJSONFields(t, logical, artifact.Bytes)
		if artifact.Kind == "stdlib-map" {
			mapCount++
			decoded, err := artifactio.DecodeMap(bytes.NewReader(artifact.Bytes))
			if err != nil {
				t.Fatalf("%s map decode: %v", logical, err)
			}
			if decoded.Key == nil || decoded.Key.CgoEnabled || decoded.Key.Goos != "linux" || decoded.Key.Goarch != "amd64" {
				t.Fatalf("%s SDK key = %#v, want pinned Linux/amd64 cgo-disabled identity", logical, decoded.Key)
			}
		}
	}
	if mapCount != 1 {
		t.Fatalf("determinism artifact map count = %d, want exactly one canonical map; artifacts=%v", mapCount, sortedFullBuildArtifactKeys(artifacts))
	}
	for logical := range want {
		if _, ok := artifacts[logical]; !ok {
			t.Fatalf("missing meaningful determinism artifact %q; got %v", logical, sortedFullBuildArtifactKeys(artifacts))
		}
	}

	dependencySurface := decodeFullBuildSurface(t, artifacts["surface:dependency_component"])
	consumerSurface := decodeFullBuildSurface(t, artifacts["surface:consumer_component"])
	unknownSurface := decodeFullBuildSurface(t, artifacts["surface:unknown_component"])
	if len(dependencySurface.Packages) < 2 || len(dependencySurface.Symbols) < 3 ||
		!sort.StringsAreSorted(dependencySurface.Packages) || !sort.StringsAreSorted(dependencySurface.Symbols) ||
		dependencySurface.Digest == "" {
		t.Fatalf("dependency surface does not exercise sorted multi-package/symbol content and a digest: packages=%v symbols=%v digest=%q", dependencySurface.Packages, dependencySurface.Symbols, dependencySurface.Digest)
	}
	if consumerSurface.Digest == "" || !sort.StringsAreSorted(consumerSurface.Packages) || !sort.StringsAreSorted(consumerSurface.Symbols) {
		t.Fatalf("consumer checked surface is not canonical/non-empty: packages=%v symbols=%v digest=%q", consumerSurface.Packages, consumerSurface.Symbols, consumerSurface.Digest)
	}
	if unknownSurface.Authority == nil || unknownSurface.Authority.Authority != gen.Authority_UNKNOWN ||
		unknownSurface.InterfaceStyle != gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE ||
		len(unknownSurface.Symbols) != 0 || unknownSurface.Digest != "" ||
		!sort.StringsAreSorted(unknownSurface.Packages) {
		t.Fatalf("unknown asserted surface = %#v, want package-level UNKNOWN with empty digest and no symbols", unknownSurface)
	}

	dependencyReport := decodeFullBuildReport(t, artifacts["report:dependency_component"])
	if dependencyReport.Verdict != report.VerdictPass {
		t.Fatalf("dependency report verdict = %q, want pass", dependencyReport.Verdict)
	}
	consumerReport := decodeFullBuildReport(t, artifacts["report:consumer_component"])
	var multiSite bool
	for _, finding := range append(append([]report.Finding{}, consumerReport.Report.Violations...), consumerReport.Report.Warnings...) {
		if len(finding.Sites) >= 3 {
			multiSite = true
			if !sort.SliceIsSorted(finding.Sites, func(i, j int) bool {
				if finding.Sites[i].File != finding.Sites[j].File {
					return finding.Sites[i].File < finding.Sites[j].File
				}
				if finding.Sites[i].Line != finding.Sites[j].Line {
					return finding.Sites[i].Line < finding.Sites[j].Line
				}
				return finding.Sites[i].Symbol < finding.Sites[j].Symbol
			}) {
				t.Fatalf("consumer multi-site finding is not sorted: %#v", finding.Sites)
			}
		}
	}
	if !multiSite {
		t.Fatalf("consumer report has no sorted multi-site finding: violations=%#v warnings=%#v", consumerReport.Report.Violations, consumerReport.Report.Warnings)
	}
	statusByComponent := make(map[string]report.DependencyBoundary, len(consumerReport.Report.Dependencies))
	for _, dependency := range consumerReport.Report.Dependencies {
		statusByComponent[dependency.Component] = dependency
	}
	checked := statusByComponent["dependency_component"]
	if checked.Provenance != report.DependencyProvenanceCheckedPass ||
		checked.Freshness != report.DependencyFreshnessBuildGraph ||
		checked.Authority != report.DependencyAuthorityDeclared {
		t.Fatalf("checked dependency axes = %#v, want CHECKED_PASS/BUILD_GRAPH/DECLARED", checked)
	}
	asserted := statusByComponent["unknown_component"]
	if asserted.Provenance != report.DependencyProvenanceAsserted ||
		asserted.Freshness != report.DependencyFreshnessBuildGraph ||
		asserted.Authority != report.DependencyAuthorityUnknown {
		t.Fatalf("asserted dependency axes = %#v, want ASSERTED/BUILD_GRAPH/UNKNOWN", asserted)
	}
}

func decodeFullBuildSurface(t *testing.T, artifact fullBuildArtifact) *gen.SurfaceManifest {
	t.Helper()
	decoded, err := artifactio.DecodeSurface(bytes.NewReader(artifact.Bytes))
	if err != nil {
		t.Fatalf("decode %s: %v", artifact.Logical, err)
	}
	return decoded
}

func decodeFullBuildReport(t *testing.T, artifact fullBuildArtifact) artifactio.PersistedReport {
	t.Helper()
	decoded, err := artifactio.DecodeReport(artifact.Bytes)
	if err != nil {
		t.Fatalf("decode %s: %v", artifact.Logical, err)
	}
	return decoded
}

func assertNoTimingJSONFields(t *testing.T, logical string, data []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode JSON for timing-field audit of %s: %v", logical, err)
	}
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "duration") || strings.Contains(lower, "elapsed") ||
					strings.Contains(lower, "profile") || strings.Contains(lower, "wall_time") ||
					strings.Contains(lower, "timing") {
					t.Fatalf("%s contains benchmark timing/profile field %q", logical, key)
				}
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
}

func compareFullBuildArtifacts(t *testing.T, first, second map[string]fullBuildArtifact) {
	t.Helper()
	firstKeys := sortedFullBuildArtifactKeys(first)
	secondKeys := sortedFullBuildArtifactKeys(second)
	if !equalStringSlices(firstKeys, secondKeys) {
		t.Fatalf("full-build logical artifact sets differ: first=%v second=%v", firstKeys, secondKeys)
	}
	for _, logical := range firstKeys {
		left := first[logical]
		right := second[logical]
		if bytes.Equal(left.Bytes, right.Bytes) && left.Digest == right.Digest {
			continue
		}
		offset := firstDifferingByte(left.Bytes, right.Bytes)
		t.Fatalf("logical artifact %q differs at byte %d: first digest=%s path=%q, second digest=%s path=%q", logical, offset, left.Digest, left.Path, right.Digest, right.Path)
	}
}

func sortedFullBuildArtifactKeys(artifacts map[string]fullBuildArtifact) []string {
	keys := make([]string, 0, len(artifacts))
	for key := range artifacts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstDifferingByte(left, right []byte) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}

func logFullBuildArtifactDigests(t *testing.T, artifacts map[string]fullBuildArtifact) {
	t.Helper()
	for _, logical := range sortedFullBuildArtifactKeys(artifacts) {
		artifact := artifacts[logical]
		t.Logf("full-build-determinism-v1 logical=%s sha256=%s path=%s", logical, artifact.Digest, artifact.Path)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
