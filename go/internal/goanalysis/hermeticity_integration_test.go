//go:build integration

package goanalysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

const (
	hermeticityAPIComponent           = "//bazel_rules/go/tests/testdata/api:api_component"
	hermeticitySharedComponent        = "//bazel_rules/go/tests/testdata/shared:shared_component"
	hermeticityReportBoundaryConsumer = "//bazel_rules/go/tests/testdata/reportboundary/consumer:consumer_component"
	hermeticityCheckTest              = "//bazel_rules/go/tests:checked_api_analysis_test"
)

var hermeticityActionMnemonics = []string{
	"ArccImportGraph",
	"ArccLayout",
	"ArccCheck",
	"ArccStdlibExportProjection",
	"ArccStdlibMap",
}

var hermeticityForbiddenTools = []string{
	"go",
	"gcc",
	"g++",
	"cc",
	"c++",
	"clang",
	"clang++",
	"as",
	"ld",
	"ar",
	"cgo",
	"compile",
	"link",
}

var hermeticitySafePathCommands = []string{
	"awk",
	"basename",
	"bash",
	"cat",
	"chmod",
	"cmp",
	"cp",
	"cut",
	"diff",
	"dirname",
	"env",
	"false",
	"find",
	"grep",
	"head",
	"ln",
	"mkdir",
	"mktemp",
	"pwd",
	"readlink",
	"rm",
	"sed",
	"sh",
	"sort",
	"tail",
	"test",
	"touch",
	"tr",
	"true",
	"uname",
	"which",
}

var hermeticityPoisonVariables = []string{
	"GOROOT",
	"GOCACHE",
	"GOMODCACHE",
	"GOPATH",
	"HOME",
	"XDG_CACHE_HOME",
	"XDG_CONFIG_HOME",
	"XDG_DATA_HOME",
	"XDG_STATE_HOME",
}

var hermeticityRemovedVariables = []string{
	"ARCC_DRIVER_MODE",
	"BASH_ENV",
	"CC",
	"CGO_ENABLED",
	"CXX",
	"ENV",
	"GO111MODULE",
	"GOARCH",
	"GOFLAGS",
	"GOOS",
	"GOENV",
	"GOBIN",
	"GOTOOLDIR",
	"GOROOT_FINAL",
	"GOPACKAGESDRIVER",
	"GOPROXY",
	"GOSUMDB",
	"GOTOOLCHAIN",
	"GOWORK",
}

type hermeticityBazelRun struct {
	RepoRoot        string
	OutputUserRoot  string
	OutputBase      string
	Execroot        string
	RepositoryCache string
	WritablePath    string
	RestrictedEnv   []string
	ProfilePath     string
	ExecutionLog    string
	PoisonRoot      string
	FirstActions    []producerChainAction
	SecondActions   []producerChainAction
}

type hermeticityTreeSnapshot map[string]hermeticityTreeEntry

type hermeticityTreeEntry struct {
	Kind       uint32
	Mode       uint32
	Size       int64
	ModTimeNS  int64
	LinkTarget string
	Digest     string
}

func TestHermeticityBazelArgsSuppressConvenienceSymlinks(t *testing.T) {
	args := hermeticityBazelArgs(
		"test",
		"/tmp/arcc-hermetic-user-root",
		"/tmp/arcc-hermetic-user-root/output-base",
		"/tmp/arcc-bazel-repository-cache",
		"/tmp/arcc-hermetic-writable",
		"/tmp/arcc-hermetic-poison",
		"/tmp/arcc-hermetic-execution.json",
		"",
		[]string{hermeticityCheckTest},
	)
	for _, arg := range args {
		if arg == "--experimental_convenience_symlinks=ignore" {
			return
		}
	}
	t.Fatalf("restricted Bazel args omit convenience-symlink suppression: %v", args)
}

func TestHermeticityForbiddenPathsDoNotTrustStdlibParent(t *testing.T) {
	for _, path := range []string{
		"/workspace/stdlib_/bin/go",
		"/workspace/stdlib_/pkg/tool/linux_amd64/compile",
		"/workspace/stdlib_/pkg/linux_amd64/link",
		"/workspace/other/pkg/tool/linux_amd64/compile",
		"/workspace/stdlib_/.cache/go-build/entry",
		"/workspace/stdlib_/gocache/entry",
	} {
		if !hermeticityForbiddenToolPath(path) && !hermeticityForbiddenCachePath(path) {
			t.Errorf("forbidden stdlib path %q was accepted", path)
		}
	}
	if !hermeticityForbiddenUnclassifiedStdlibPath("/workspace/stdlib_/src/fmt/format.go") {
		t.Error("SDK source beneath stdlib_ was accepted")
	}
	if hermeticityForbiddenToolPath("/workspace/arcc_stdlib_map.stdlib-export/aa/export-d") {
		t.Error("projected export artifact was classified as a tool")
	}
}

func runHermeticityProducerChainBazelSuite(t *testing.T, variants []producerChainVariant) hermeticityBazelRun {
	t.Helper()
	labels := []string{hermeticityCheckTest}
	for _, variant := range variants {
		labels = append(labels, variant.Component)
	}
	// Build the deterministic artifact fixture in this same isolated routine
	// invocation. It reuses the one real default map and lets the full-build
	// comparison take this output tree as its first run.
	labels = append(labels,
		fullBuildDeterminismConsumer,
		fullBuildDeterminismUnknown,
		fullBuildDeterminismDependency,
		hermeticityReportBoundaryConsumer,
	)
	run := runHermeticityBazelSuiteForTargets(t, labels, true)
	assertHermeticityRun(t, run)
	return run
}

func runHermeticityBazelSuiteForTargets(t *testing.T, labels []string, withProfile bool) hermeticityBazelRun {
	t.Helper()
	bazelPath := hermeticityBazelPath(t)
	repoRoot := hermeticityRepoRoot(t)
	convenienceBefore, err := snapshotHermeticityConvenienceLinks(repoRoot)
	if err != nil {
		t.Fatalf("snapshotting repository-root Bazel convenience links before setup: %v", err)
	}
	repositoryCache := hermeticityRepositoryCache(t, bazelPath, repoRoot)
	assertHermeticityConvenienceLinksUnchanged(t, repoRoot, convenienceBefore, "Bazel repository-cache lookup")
	runDir := t.TempDir()
	outputUserRoot := filepath.Join(runDir, "bazel-user-root")
	outputBase := filepath.Join(outputUserRoot, "output-base")
	profilePath := ""
	if withProfile {
		profilePath = filepath.Join(runDir, "hermeticity.profile.json.gz")
	}
	executionLog := filepath.Join(runDir, "hermeticity.execution.json")
	secondLog := filepath.Join(runDir, "hermeticity-second.execution.json")
	writablePath := filepath.Join(runDir, "sandbox-writable")
	poisonRoot := filepath.Join(runDir, "poison")
	if err := os.MkdirAll(writablePath, 0o700); err != nil {
		t.Fatalf("creating restricted sandbox writable directory: %v", err)
	}
	poisonBefore := createHermeticityPoisonTree(t, poisonRoot)
	restrictedEnv, safePath := createHermeticityEnvironment(t, runDir, writablePath, poisonRoot)
	assertHermeticityPath(t, safePath)

	// The isolated server must be shut down before t.TempDir removes the
	// output tree. Making the tree writable also handles linux-sandbox's
	// read-only cleanup artifacts without touching the repository.
	t.Cleanup(func() {
		command := exec.Command(
			bazelPath,
			"--output_user_root="+outputUserRoot,
			"--output_base="+outputBase,
			"shutdown",
		)
		command.Dir = repoRoot
		command.Env = restrictedEnv
		_ = command.Run()
		makeProducerChainTreeWritable(runDir)
	})

	// A fresh output user root has no Skyframe repository state. Materialise
	// the already-cached external repositories before entering the hostile
	// environment, with module-network access disabled; the acceptance run
	// itself uses --nofetch so repository setup cannot turn into a network
	// escape hatch. This is Bazel setup state, distinct from native Go caches.
	prefetchArgs := []string{
		"--output_user_root=" + outputUserRoot,
		"--output_base=" + outputBase,
		"fetch",
		"--repository_cache=" + repositoryCache,
		"--experimental_convenience_symlinks=ignore",
		"--noshow_progress",
	}
	prefetchArgs = append(prefetchArgs, labels...)
	prefetchOutput, prefetchErr := runHermeticityBazelCommand(t, bazelPath, repoRoot, hermeticityPrefetchEnvironment(), prefetchArgs...)
	assertHermeticityConvenienceLinksUnchanged(t, repoRoot, convenienceBefore, "Bazel repository setup")
	if prefetchErr != nil {
		t.Fatalf("preparing cached Bazel repositories without network failed: %v\n%s", prefetchErr, prefetchOutput)
	}

	firstArgs := hermeticityBazelArgs(
		"test",
		outputUserRoot,
		outputBase,
		repositoryCache,
		writablePath,
		poisonRoot,
		executionLog,
		profilePath,
		labels,
	)
	firstOutput, firstErr := runHermeticityBazelCommand(t, bazelPath, repoRoot, restrictedEnv, firstArgs...)
	assertHermeticityPoisonUnchanged(t, poisonRoot, poisonBefore, "first Bazel run")
	assertHermeticityConvenienceLinksUnchanged(t, repoRoot, convenienceBefore, "first Bazel run")
	if firstErr != nil {
		t.Fatalf("restricted Bazel test failed (Bazel or linux-sandbox support is required): %v\n%s", firstErr, firstOutput)
	}
	firstActions := readProducerChainExecutionLog(t, executionLog)
	addHermeticityExecutionInfo(t, bazelPath, repoRoot, restrictedEnv, outputUserRoot, outputBase, repositoryCache, labels, firstActions)
	if withProfile {
		if _, err := os.Stat(profilePath); err != nil {
			t.Fatalf("Bazel profile was not produced: %v\n%s", err, firstOutput)
		}
	}
	execroot := filepath.Join(outputBase, "execroot", "_main")
	firstOutputs := hermeticityOutputSnapshot(t, execroot, firstActions)
	if len(firstOutputs) != 5 {
		t.Fatalf("restricted run captured %d map/report/surface outputs, want map plus two checked components", len(firstOutputs))
	}

	secondArgs := hermeticityBazelArgs(
		"build",
		outputUserRoot,
		outputBase,
		repositoryCache,
		writablePath,
		poisonRoot,
		secondLog,
		"",
		labels,
	)
	secondOutput, secondErr := runHermeticityBazelCommand(t, bazelPath, repoRoot, restrictedEnv, secondArgs...)
	assertHermeticityPoisonUnchanged(t, poisonRoot, poisonBefore, "repeat Bazel run")
	assertHermeticityConvenienceLinksUnchanged(t, repoRoot, convenienceBefore, "repeat Bazel run")
	if secondErr != nil {
		t.Fatalf("restricted repeat Bazel build failed: %v\n%s", secondErr, secondOutput)
	}
	for outputPath, want := range firstOutputs {
		path := hermeticityPhysicalPath(execroot, outputPath)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading repeated output %q: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("repeated Bazel build changed output %q", outputPath)
		}
	}

	secondActions := readHermeticityExecutionLogIfNonEmpty(t, secondLog)
	return hermeticityBazelRun{
		RepoRoot:        repoRoot,
		OutputUserRoot:  outputUserRoot,
		OutputBase:      outputBase,
		Execroot:        execroot,
		RepositoryCache: repositoryCache,
		WritablePath:    writablePath,
		RestrictedEnv:   restrictedEnv,
		ProfilePath:     profilePath,
		ExecutionLog:    executionLog,
		PoisonRoot:      poisonRoot,
		FirstActions:    firstActions,
		SecondActions:   secondActions,
	}
}

func readHermeticityExecutionLogIfNonEmpty(t *testing.T, path string) []producerChainAction {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading optional Bazel execution log %q: %v", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return readProducerChainExecutionLog(t, path)
}

func hermeticityBazelPath(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("bazel")
	if err != nil {
		t.Fatalf("Bazel is required for the restricted-sandbox acceptance run: %v", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolving the absolute Bazel path %q: %v", path, err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("Bazel path %q is not absolute", path)
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		t.Fatalf("absolute Bazel path %q is not an executable file: %v", path, err)
	}
	return path
}

func hermeticityRepoRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed while locating the repository")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../.."))
}

func hermeticityRepositoryCache(t *testing.T, bazelPath, repoRoot string) string {
	t.Helper()
	command := exec.Command(bazelPath, "info", "--experimental_convenience_symlinks=ignore", "repository_cache")
	command.Dir = repoRoot
	output, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("locating Bazel's repository cache: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("locating Bazel's repository cache: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		path := strings.TrimSpace(lines[i])
		if !filepath.IsAbs(path) {
			continue
		}
		info, statErr := os.Stat(path)
		if statErr == nil && info.IsDir() {
			return path
		}
	}
	t.Fatalf("Bazel repository_cache output did not name an existing absolute directory: %q", string(output))
	return ""
}

func hermeticityBazelArgs(command, outputUserRoot, outputBase, repositoryCache, writablePath, poisonRoot, executionLog, profilePath string, labels []string) []string {
	args := []string{
		"--output_user_root=" + outputUserRoot,
		"--output_base=" + outputBase,
		command,
		"--repository_cache=" + repositoryCache,
		"--nofetch",
		"--experimental_convenience_symlinks=ignore",
		"--spawn_strategy=linux-sandbox",
		"--sandbox_default_allow_network=false",
		"--sandbox_fake_hostname=true",
		"--sandbox_writable_path=" + writablePath,
		"--sandbox_block_path=" + poisonRoot,
		"--execution_log_json_file=" + executionLog,
		"--execution_log_sort",
		"--noshow_progress",
		"--output_groups=+arcc",
	}
	if profilePath != "" {
		args = append(args,
			"--profile="+profilePath,
			"--noslim_profile",
			"--experimental_profile_additional_tasks=action",
		)
	}
	if command == "test" {
		args = append(args, "--test_output=errors")
	}
	// Preserve caller order so determinism tests can deliberately reverse the
	// requested targets. Bazel's graph is the same set either way; the
	// scheduling/request-order perturbation is part of the acceptance input.
	return append(args, labels...)
}

func runHermeticityBazelCommand(t *testing.T, bazelPath, repoRoot string, environment []string, args ...string) ([]byte, error) {
	t.Helper()
	command := exec.Command(bazelPath, args...)
	command.Dir = repoRoot
	command.Env = environment
	return command.CombinedOutput()
}

type hermeticityAQuery struct {
	Actions []hermeticityAQueryAction `json:"actions"`
	Targets []hermeticityAQueryTarget `json:"targets"`
}

type hermeticityAQueryAction struct {
	TargetID      int                          `json:"targetId"`
	Mnemonic      string                       `json:"mnemonic"`
	ExecutionInfo []producerChainExecutionInfo `json:"executionInfo"`
}

type hermeticityAQueryTarget struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

func addHermeticityExecutionInfo(t *testing.T, bazelPath, repoRoot string, environment []string, outputUserRoot, outputBase, repositoryCache string, labels []string, actions []producerChainAction) {
	t.Helper()
	args := []string{
		"--output_user_root=" + outputUserRoot,
		"--output_base=" + outputBase,
		"aquery",
		"--repository_cache=" + repositoryCache,
		"--nofetch",
		"--experimental_convenience_symlinks=ignore",
		"--noshow_progress",
		"--output=jsonproto",
		"deps(set(" + strings.Join(labels, " ") + "))",
	}
	command := exec.Command(bazelPath, args...)
	command.Dir = repoRoot
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("Bazel aquery execution-contract inspection failed: %v\n%s", err, stderr.String())
	}
	var query hermeticityAQuery
	if err := json.Unmarshal(stdout.Bytes(), &query); err != nil {
		t.Fatalf("decoding Bazel aquery execution-contract inspection: %v", err)
	}
	targetLabels := make(map[int]string, len(query.Targets))
	for _, target := range query.Targets {
		targetLabels[target.ID] = target.Label
	}
	byTargetAndMnemonic := make(map[string][][]producerChainExecutionInfo)
	for _, action := range query.Actions {
		if !containsHermeticityAction(action.Mnemonic) {
			continue
		}
		target, ok := targetLabels[action.TargetID]
		if !ok {
			t.Fatalf("Bazel aquery action %s has unknown target id %d", action.Mnemonic, action.TargetID)
		}
		key := target + "\x00" + action.Mnemonic
		byTargetAndMnemonic[key] = append(byTargetAndMnemonic[key], action.ExecutionInfo)
	}
	for i := range actions {
		if !containsHermeticityAction(actions[i].Mnemonic) {
			continue
		}
		key := actions[i].TargetLabel + "\x00" + actions[i].Mnemonic
		matches := byTargetAndMnemonic[key]
		if len(matches) != 1 {
			t.Fatalf("Bazel aquery execution-contract actions for %s %s = %d, want exactly one", actions[i].TargetLabel, actions[i].Mnemonic, len(matches))
		}
		actions[i].ExecutionInfo = matches[0]
	}
}

func hermeticityPrefetchEnvironment() []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	values["GOPROXY"] = hermeticityLocalModuleProxy(values)
	values["GOSUMDB"] = "off"
	values["GOTOOLCHAIN"] = "local"
	values["GOWORK"] = "off"
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

func hermeticityLocalModuleProxy(values map[string]string) string {
	moduleCache := values["GOMODCACHE"]
	if moduleCache == "" {
		gopath := values["GOPATH"]
		if index := strings.IndexByte(gopath, os.PathListSeparator); index >= 0 {
			gopath = gopath[:index]
		}
		if gopath != "" {
			moduleCache = filepath.Join(gopath, "pkg", "mod")
		}
	}
	if moduleCache == "" {
		if home, err := os.UserHomeDir(); err == nil {
			moduleCache = filepath.Join(home, "go", "pkg", "mod")
		}
	}
	if moduleCache == "" {
		return "off"
	}
	proxy := filepath.Join(moduleCache, "cache", "download")
	if info, err := os.Stat(proxy); err == nil && info.IsDir() {
		return "file://" + filepath.ToSlash(proxy)
	}
	return "off"
}

func createHermeticityEnvironment(t *testing.T, runDir, writablePath, poisonRoot string) ([]string, string) {
	t.Helper()
	safePath := filepath.Join(runDir, "restricted-path")
	if err := os.MkdirAll(safePath, 0o700); err != nil {
		t.Fatalf("creating restricted PATH directory: %v", err)
	}
	for _, name := range hermeticitySafePathCommands {
		source, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("restricted Bazel environment requires utility %q: %v", name, err)
		}
		destination := filepath.Join(safePath, name)
		if err := os.Symlink(source, destination); err != nil {
			t.Fatalf("linking restricted PATH utility %q: %v", name, err)
		}
	}

	poisonPaths := make(map[string]string, len(hermeticityPoisonVariables))
	for _, variable := range hermeticityPoisonVariables {
		path := filepath.Join(poisonRoot, strings.ToLower(variable))
		poisonPaths[variable] = path
	}

	values := make(map[string]string)
	removed := make(map[string]bool, len(hermeticityRemovedVariables)+len(hermeticityPoisonVariables))
	for _, variable := range hermeticityRemovedVariables {
		removed[variable] = true
	}
	for _, variable := range hermeticityPoisonVariables {
		removed[variable] = true
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && !removed[key] {
			values[key] = value
		}
	}
	values["CGO_ENABLED"] = "0"
	values["GOWORK"] = "off"
	values["GOPROXY"] = "off"
	values["GOSUMDB"] = "off"
	values["PATH"] = safePath
	values["TMPDIR"] = writablePath
	for variable, path := range poisonPaths {
		values[variable] = path
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment, safePath
}

func assertHermeticityPath(t *testing.T, safePath string) {
	t.Helper()
	for _, tool := range hermeticityForbiddenTools {
		if hermeticityPathResolves(safePath, tool) {
			t.Errorf("restricted PATH resolves forbidden host tool %q", tool)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func hermeticityPathResolves(pathList, name string) bool {
	for _, directory := range strings.Split(pathList, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		path := filepath.Join(directory, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

func createHermeticityPoisonTree(t *testing.T, root string) hermeticityTreeSnapshot {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("creating poison root %q: %v", root, err)
	}
	for _, variable := range hermeticityPoisonVariables {
		directory := filepath.Join(root, strings.ToLower(variable))
		nested := filepath.Join(directory, "nested")
		if err := os.MkdirAll(nested, 0o700); err != nil {
			t.Fatalf("creating poison directory %q: %v", directory, err)
		}
		sentinel := filepath.Join(nested, "arcc-hermeticity-sentinel")
		content := []byte("poison sentinel for " + variable + "\n")
		if err := os.WriteFile(sentinel, content, 0o600); err != nil {
			t.Fatalf("seeding poison sentinel %q: %v", sentinel, err)
		}
		if err := os.Chmod(sentinel, 0o444); err != nil {
			t.Fatalf("making poison sentinel read-only %q: %v", sentinel, err)
		}
		if err := os.Chmod(nested, 0o555); err != nil {
			t.Fatalf("making poison nested directory read-only %q: %v", nested, err)
		}
		if err := os.Chmod(directory, 0o555); err != nil {
			t.Fatalf("making poison directory read-only %q: %v", directory, err)
		}
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatalf("making poison root read-only %q: %v", root, err)
	}
	snapshot, err := snapshotHermeticityTree(root)
	if err != nil {
		t.Fatalf("snapshotting seeded poison tree %q: %v", root, err)
	}
	return snapshot
}

func snapshotHermeticityTree(root string) (hermeticityTreeSnapshot, error) {
	snapshot := make(hermeticityTreeSnapshot)
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry, err := hermeticityEntryForPath(path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(relative)] = entry
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshotting hermeticity tree %q: %w", root, err)
	}
	return snapshot, nil
}

func snapshotHermeticityConvenienceLinks(repoRoot string) (hermeticityTreeSnapshot, error) {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("reading repository root %q: %w", repoRoot, err)
	}
	snapshot := make(hermeticityTreeSnapshot)
	for _, directoryEntry := range entries {
		if !strings.HasPrefix(directoryEntry.Name(), "bazel-") {
			continue
		}
		path := filepath.Join(repoRoot, directoryEntry.Name())
		metadata, err := hermeticityEntryForPath(path)
		if err != nil {
			return nil, fmt.Errorf("reading convenience link %q: %w", path, err)
		}
		snapshot[directoryEntry.Name()] = metadata
	}
	return snapshot, nil
}

func hermeticityEntryForPath(path string) (hermeticityTreeEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return hermeticityTreeEntry{}, err
	}
	entry := hermeticityTreeEntry{
		Kind:      uint32(info.Mode().Type()),
		Mode:      uint32(info.Mode().Perm()),
		Size:      info.Size(),
		ModTimeNS: info.ModTime().UnixNano(),
	}
	if info.Mode()&os.ModeSymlink != 0 {
		entry.LinkTarget, err = os.Readlink(path)
		if err != nil {
			return hermeticityTreeEntry{}, err
		}
	}
	if info.Mode().IsRegular() {
		entry.Digest, err = hermeticityFileDigest(path)
		if err != nil {
			return hermeticityTreeEntry{}, err
		}
	}
	return entry, nil
}

func hermeticityFileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func assertHermeticityPoisonUnchanged(t *testing.T, root string, before hermeticityTreeSnapshot, phase string) {
	t.Helper()
	after, err := snapshotHermeticityTree(root)
	if err != nil {
		t.Fatalf("snapshotting poison tree after %s: %v", phase, err)
	}
	if difference := hermeticitySnapshotDifference(before, after); difference != "" {
		t.Fatalf("poison tree changed during %s: %s", phase, difference)
	}
}

func assertHermeticityConvenienceLinksUnchanged(t *testing.T, repoRoot string, before hermeticityTreeSnapshot, phase string) {
	t.Helper()
	after, err := snapshotHermeticityConvenienceLinks(repoRoot)
	if err != nil {
		t.Fatalf("snapshotting repository-root Bazel convenience links after %s: %v", phase, err)
	}
	if difference := hermeticitySnapshotDifference(before, after); difference != "" {
		t.Fatalf("repository-root Bazel convenience links changed during %s: %s", phase, difference)
	}
}

func hermeticitySnapshotDifference(before, after hermeticityTreeSnapshot) string {
	keys := make(map[string]bool, len(before)+len(after))
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var differences []string
	for _, key := range ordered {
		left, leftOK := before[key]
		right, rightOK := after[key]
		if !leftOK || !rightOK || !reflect.DeepEqual(left, right) {
			differences = append(differences, fmt.Sprintf("%s: before=%#v after=%#v", key, left, right))
		}
	}
	return strings.Join(differences, "; ")
}

func hermeticityOutputSnapshot(t *testing.T, execroot string, actions []producerChainAction) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte)
	for _, action := range actions {
		if action.Mnemonic == "ArccStdlibMap" && action.TargetLabel != "//:arcc_stdlib_map" {
			continue
		}
		if action.Mnemonic == "ArccCheck" && action.TargetLabel != hermeticityAPIComponent && action.TargetLabel != hermeticitySharedComponent {
			continue
		}
		for _, output := range action.ActualOutputs {
			if !strings.HasSuffix(output.Path, ".stdlib-map.json") &&
				!strings.HasSuffix(output.Path, ".surface.json") &&
				!strings.HasSuffix(output.Path, ".report.json") {
				continue
			}
			path := hermeticityPhysicalPath(execroot, output.Path)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading hermeticity output %q: %v", path, err)
			}
			if len(data) == 0 {
				t.Fatalf("hermeticity output %q is empty", path)
			}
			if previous, exists := snapshot[output.Path]; exists && !bytes.Equal(previous, data) {
				t.Fatalf("execution log maps output path %q to different bytes", output.Path)
			}
			snapshot[output.Path] = data
		}
	}
	return snapshot
}

func hermeticityPhysicalPath(execroot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(execroot, filepath.FromSlash(path))
}

func assertHermeticityRun(t *testing.T, run hermeticityBazelRun) {
	t.Helper()
	assertHermeticityActionCounts(t, run.FirstActions)
	for _, action := range run.FirstActions {
		if !containsHermeticityAction(action.Mnemonic) {
			continue
		}
		if action.Mnemonic == "ArccStdlibMap" || action.Mnemonic == "ArccStdlibExportProjection" || action.TargetLabel == hermeticityAPIComponent || action.TargetLabel == hermeticitySharedComponent {
			assertHermeticityAction(t, action, run.PoisonRoot)
		} else {
			assertHermeticityActionContract(t, action, run.PoisonRoot)
		}
	}
	assertHermeticityReportsAndSurfaces(t, run)
	assertHermeticityMap(t, run)

	secondMaps := actionsWithMnemonic(run.SecondActions, "ArccStdlibMap")
	if len(secondMaps) > 1 {
		t.Fatalf("repeated restricted build executed %d stdlib-map actions, want at most one cached action record", len(secondMaps))
	}
	if len(secondMaps) == 1 && !secondMaps[0].CacheHit {
		t.Fatalf("repeated restricted build did not reuse the default stdlib-map action")
	}
}

func containsHermeticityAction(mnemonic string) bool {
	for _, expected := range hermeticityActionMnemonics {
		if mnemonic == expected {
			return true
		}
	}
	return false
}

func assertHermeticityActionCounts(t *testing.T, actions []producerChainAction) {
	t.Helper()
	counts := make(map[string]int)
	for _, action := range actions {
		if containsHermeticityAction(action.Mnemonic) {
			counts[action.Mnemonic]++
		}
	}
	for _, mnemonic := range hermeticityActionMnemonics {
		if counts[mnemonic] == 0 {
			t.Errorf("restricted execution log contains no %s action", mnemonic)
		}
	}
	if counts["ArccStdlibMap"] != 1 {
		t.Errorf("restricted execution log has %d ArccStdlibMap actions, want exactly one default action", counts["ArccStdlibMap"])
	}
	for _, target := range []string{hermeticityAPIComponent, hermeticitySharedComponent} {
		for _, mnemonic := range []string{"ArccImportGraph", "ArccLayout", "ArccCheck"} {
			matches := 0
			for _, action := range actions {
				if action.TargetLabel == target && action.Mnemonic == mnemonic {
					matches++
				}
			}
			if matches != 1 {
				t.Errorf("%s %s actions = %d, want exactly one", target, mnemonic, matches)
			}
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func assertHermeticityAction(t *testing.T, action producerChainAction, poisonRoot string) {
	t.Helper()
	assertHermeticityActionContract(t, action, poisonRoot)

	args := strings.Join(action.CommandArgs, "\x00")
	switch action.Mnemonic {
	case "ArccImportGraph":
		assertHermeticityArgs(t, action, args, "package-imports", "--config=", "--output=")
		assertHermeticityInputs(t, action, func(path string) bool {
			return strings.HasSuffix(path, ".package-imports.request.json") || strings.HasSuffix(path, ".go")
		}, "ordinary source projection")
	case "ArccLayout":
		assertHermeticityArgs(t, action, args, "package-layout-merge", "--layout=", "--imports=", "--output=")
		assertHermeticityInputs(t, action, func(path string) bool {
			return strings.HasSuffix(path, ".package-layout.base.json") || strings.HasSuffix(path, ".package-imports.json")
		}, "layout merge")
	case "ArccCheck":
		assertHermeticityArgs(t, action, args, "check", "--package-layout=", "--stdlib-map=", "--report-out=", "--surface-out=", "--report-verdict-only")
		assertHermeticityCheckInputs(t, action)
	case "ArccStdlibExportProjection":
		assertHermeticityArgs(t, action, args, "stdlibmap", "project", "--metadata=", "--source-root=", "--output=")
		assertHermeticityInputs(t, action, func(path string) bool {
			return strings.HasSuffix(filepath.Base(path), "stdlib.pkg.json") || strings.Contains(path, "/stdlib_/gocache")
		}, "raw stdlib export projection")
	case "ArccStdlibMap":
		assertHermeticityArgs(t, action, args, "stdlibmap", "generate", "--output=", "--package-list=", "--config-file=", "--sdk-root=")
		assertHermeticityInputs(t, action, func(path string) bool {
			return strings.HasSuffix(path, "/packages.txt") ||
				strings.HasSuffix(path, ".stdlib-map-config") ||
				(strings.Contains(path, "go_sdk") && strings.Contains(path, "/src/"))
		}, "SDK source/oracle")
	}
}

func assertHermeticityActionContract(t *testing.T, action producerChainAction, poisonRoot string) {
	t.Helper()
	if len(action.Environment) != 0 {
		t.Errorf("%s %s environment = %#v, want empty", action.TargetLabel, action.Mnemonic, action.Environment)
	}
	blockNetwork := false
	for _, requirement := range action.ExecutionInfo {
		if requirement.Key == "block-network" && requirement.Value == "1" {
			blockNetwork = true
		}
	}
	if !blockNetwork {
		t.Errorf("%s %s execution requirements = %#v, want block-network=1", action.TargetLabel, action.Mnemonic, action.ExecutionInfo)
	}
	toolCount := 0
	for _, input := range action.Inputs {
		if !input.IsTool {
			continue
		}
		toolCount++
		if !hermeticityExpectedToolPath(action.Mnemonic, filepath.ToSlash(input.Path)) {
			t.Errorf("%s %s declares undocumented tool input %q", action.TargetLabel, action.Mnemonic, input.Path)
		}
	}
	if toolCount == 0 {
		t.Errorf("%s %s declares no arcc tool input", action.TargetLabel, action.Mnemonic)
	}

	values := append([]string{}, action.CommandArgs...)
	for _, input := range action.Inputs {
		values = append(values, input.Path)
	}
	for _, environment := range action.Environment {
		values = append(values, environment.Name, environment.Value)
	}
	for _, value := range values {
		if strings.Contains(filepath.ToSlash(value), filepath.ToSlash(poisonRoot)) {
			t.Errorf("%s %s mentions poison path %q", action.TargetLabel, action.Mnemonic, value)
		}
		path := filepath.ToSlash(value)
		if action.Mnemonic == "ArccStdlibExportProjection" && strings.Contains(path, "/stdlib_/gocache") {
			// This is the explicitly classified raw input of the one projection
			// action. Its output is the only stdlib tree allowed at ArccCheck.
			continue
		}
		if hermeticityForbiddenToolPath(value) {
			t.Errorf("%s %s mentions host Go/compiler tool %q", action.TargetLabel, action.Mnemonic, value)
		}
		if hermeticityForbiddenCachePath(value) {
			t.Errorf("%s %s mentions forbidden native cache path %q", action.TargetLabel, action.Mnemonic, value)
		}
	}

	executable := hermeticityActionExecutable(action)
	if action.Mnemonic == "ArccStdlibMap" || action.Mnemonic == "ArccStdlibExportProjection" {
		if !strings.Contains(filepath.ToSlash(executable), "/go/cmd/arcc-stdlibmap/") {
			t.Errorf("ArccStdlibMap executable = %q, want generation-only arcc-stdlibmap", executable)
		}
	} else if !strings.Contains(filepath.ToSlash(executable), "/go/cmd/arcc/") {
		t.Errorf("%s executable = %q, want arcc binary", action.Mnemonic, executable)
	}
}

func hermeticityActionExecutable(action producerChainAction) string {
	if len(action.CommandArgs) == 0 {
		return ""
	}
	if action.Mnemonic == "ArccCheck" && len(action.CommandArgs) > 1 {
		return action.CommandArgs[1]
	}
	return action.CommandArgs[0]
}

func hermeticityExpectedToolPath(mnemonic, path string) bool {
	if mnemonic == "ArccStdlibMap" || mnemonic == "ArccStdlibExportProjection" {
		return strings.Contains(path, "/go/cmd/arcc-stdlibmap/")
	}
	return strings.Contains(path, "/go/cmd/arcc/") || strings.HasSuffix(path, ".arcc-check-wrapper.sh")
}

func assertHermeticityArgs(t *testing.T, action producerChainAction, args string, required ...string) {
	t.Helper()
	for _, requiredArg := range required {
		if !strings.Contains(args, requiredArg) {
			t.Errorf("%s %s argv omits %q: %v", action.TargetLabel, action.Mnemonic, requiredArg, action.CommandArgs)
		}
	}
	if strings.Contains(args, "--generate-stdlib-map") || strings.Contains(args, "go list") || strings.Contains(args, "go env") || strings.Contains(args, "go tool") {
		t.Errorf("%s %s argv contains native discovery/fallback: %v", action.TargetLabel, action.Mnemonic, action.CommandArgs)
	}
}

func assertHermeticityInputs(t *testing.T, action producerChainAction, allowed func(string) bool, role string) {
	t.Helper()
	nonToolCount := 0
	for _, input := range action.Inputs {
		if input.IsTool {
			continue
		}
		nonToolCount++
		path := filepath.ToSlash(input.Path)
		if !allowed(path) {
			t.Errorf("%s %s has %s input outside its role: %q", action.TargetLabel, action.Mnemonic, role, input.Path)
		}
	}
	if nonToolCount == 0 {
		t.Errorf("%s %s has no declared non-tool %s inputs", action.TargetLabel, action.Mnemonic, role)
	}
}

func assertHermeticityCheckInputs(t *testing.T, action producerChainAction) {
	t.Helper()
	memberSources := map[string]bool{}
	switch action.TargetLabel {
	case hermeticityAPIComponent:
		for _, name := range []string{"api.go", "core.go", "core_extra.go", "extra_impl.go", "extradep.go", "lowlevel.go"} {
			memberSources[name] = true
		}
	case hermeticitySharedComponent:
		memberSources["shared.go"] = true
	}
	required := map[string]bool{
		".component.textproto":  false,
		".package-layout.json":  false,
		".package-imports.json": false,
		".stdlib-map.json":      false,
		"stdlib.pkg.json":       false,
		".stdlib-export":        false,
	}
	if action.TargetLabel == hermeticityAPIComponent {
		required[".surface.json"] = false
		required[".report.json"] = false
	}
	for _, input := range action.Inputs {
		if input.IsTool {
			continue
		}
		path := filepath.ToSlash(input.Path)
		base := filepath.Base(path)
		if strings.HasSuffix(path, ".go") {
			if !memberSources[base] {
				t.Errorf("%s ArccCheck received non-member source %q", action.TargetLabel, input.Path)
			}
			continue
		}
		if hermeticityForbiddenUnclassifiedStdlibPath(path) {
			t.Errorf("%s ArccCheck received unclassified stdlib input %q", action.TargetLabel, input.Path)
			continue
		}
		allowed := false
		if strings.HasSuffix(path, "stdlib.pkg.json") {
			required["stdlib.pkg.json"] = true
			allowed = true
		}
		if hermeticityProjectedStdlibExportPath(path) {
			required[".stdlib-export"] = true
			allowed = true
		}
		for suffix := range required {
			if strings.HasSuffix(path, suffix) {
				required[suffix] = true
				allowed = true
			}
		}
		if strings.HasSuffix(path, ".x") || strings.HasSuffix(path, ".surface.json") || strings.HasSuffix(path, ".report.json") {
			for suffix := range required {
				if strings.HasSuffix(path, suffix) {
					required[suffix] = true
				}
			}
			allowed = true
		}
		if !allowed {
			t.Errorf("%s ArccCheck received undocumented input %q", action.TargetLabel, input.Path)
		}
	}
	for suffix, seen := range required {
		if !seen {
			t.Errorf("%s ArccCheck is missing documented input role %s", action.TargetLabel, suffix)
		}
	}
}

func hermeticityProjectedStdlibExportPath(value string) bool {
	path := filepath.ToSlash(value)
	return strings.HasSuffix(path, ".stdlib-export") || strings.Contains(path, ".stdlib-export/")
}

func hermeticityForbiddenUnclassifiedStdlibPath(value string) bool {
	path := filepath.ToSlash(value)
	if !strings.Contains(path, "/stdlib_/") {
		return false
	}
	return !strings.HasSuffix(path, "stdlib.pkg.json") && !hermeticityProjectedStdlibExportPath(path)
}

func hermeticityForbiddenToolPath(value string) bool {
	path := filepath.ToSlash(value)
	if strings.Contains(path, "/pkg/tool/") || strings.HasSuffix(path, "/bin/go") {
		return true
	}
	base := filepath.Base(filepath.FromSlash(path))
	for _, tool := range hermeticityForbiddenTools {
		if base == tool || strings.HasPrefix(base, tool+"-") {
			return true
		}
	}
	return false
}

func hermeticityForbiddenCachePath(value string) bool {
	path := filepath.ToSlash(value)
	if strings.Contains(path, "/.cache/") || strings.Contains(path, "/gomodcache/") || strings.Contains(path, "/gopath/") {
		return true
	}
	if strings.Contains(path, "/pkg/mod/") || strings.Contains(path, "/go-build/") {
		return true
	}
	if strings.Contains(path, "/gocache/") {
		return true
	}
	return false
}

func assertHermeticityReportsAndSurfaces(t *testing.T, run hermeticityBazelRun) {
	t.Helper()
	for _, target := range []string{hermeticityAPIComponent, hermeticitySharedComponent} {
		var matching *producerChainAction
		for i := range run.FirstActions {
			action := &run.FirstActions[i]
			if action.Mnemonic == "ArccCheck" && action.TargetLabel == target {
				matching = action
				break
			}
		}
		if matching == nil {
			t.Fatalf("missing ArccCheck action for %s", target)
		}
		reportPath := producerChainOutputPath(t, run.Execroot, *matching, ".report.json")
		persisted, err := artifactio.ReadReportFile(reportPath)
		if err != nil {
			t.Fatalf("reading %s report %q: %v", target, reportPath, err)
		}
		if persisted.Verdict != report.VerdictPass {
			t.Fatalf("%s report verdict = %q, want pass", target, persisted.Verdict)
		}
		surfacePath := producerChainOutputPath(t, run.Execroot, *matching, ".surface.json")
		surfaceBytes, err := os.ReadFile(surfacePath)
		if err != nil {
			t.Fatalf("reading %s surface %q: %v", target, surfacePath, err)
		}
		surface, err := artifactio.DecodeSurface(bytes.NewReader(surfaceBytes))
		if err != nil {
			t.Fatalf("decoding %s surface %q: %v", target, surfacePath, err)
		}
		componentName := target[strings.LastIndex(target, ":")+1:]
		if surface.Component != componentName {
			t.Errorf("%s surface component = %q, want %q", target, surface.Component, componentName)
		}
		if surface.SdkKey == nil || surface.SdkKey.CgoEnabled {
			t.Errorf("%s surface SDK key = %#v, want cgo-disabled target identity", target, surface.SdkKey)
		}
	}
}

func assertReportBoundaryUnknownText(t *testing.T, run hermeticityBazelRun) {
	t.Helper()
	var checkActions []producerChainAction
	for _, action := range run.FirstActions {
		if action.TargetLabel == hermeticityReportBoundaryConsumer && action.Mnemonic == "ArccCheck" {
			checkActions = append(checkActions, action)
		}
	}
	if len(checkActions) != 1 {
		t.Fatalf("%s ArccCheck actions = %d, want exactly one persisted-report producer", hermeticityReportBoundaryConsumer, len(checkActions))
	}
	reportPath := producerChainOutputPath(t, run.Execroot, checkActions[0], ".report.json")
	persisted, err := artifactio.ReadReportFile(reportPath)
	if err != nil {
		t.Fatalf("reading %s report %q: %v", hermeticityReportBoundaryConsumer, reportPath, err)
	}

	rendered := report.RenderText(persisted.Report)
	var manualBoundary string
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, "- manual_component") {
			if manualBoundary != "" {
				t.Fatalf("rendered report contains multiple manual_component boundaries: %q", rendered)
			}
			manualBoundary = line
		}
	}
	if manualBoundary == "" {
		t.Fatalf("rendered report omits the manual_component boundary:\n%s", rendered)
	}
	if !strings.Contains(manualBoundary, "asserted") || !strings.Contains(manualBoundary, "untrusted") {
		t.Errorf("manual_component boundary = %q, want asserted and untrusted status words", manualBoundary)
	}
	lowerBoundary := strings.ToLower(manualBoundary)
	if strings.Contains(lowerBoundary, "certified") || strings.Contains(lowerBoundary, "declared") {
		t.Errorf("manual_component boundary = %q, must not use certified or declared-authority wording", manualBoundary)
	}
}

func assertHermeticityMap(t *testing.T, run hermeticityBazelRun) {
	t.Helper()
	var mapAction *producerChainAction
	for i := range run.FirstActions {
		action := &run.FirstActions[i]
		if action.Mnemonic == "ArccStdlibMap" && action.TargetLabel == "//:arcc_stdlib_map" {
			mapAction = action
			break
		}
	}
	if mapAction == nil {
		t.Fatal("missing canonical default ArccStdlibMap action")
	}
	if mapAction.CacheHit {
		t.Fatal("canonical default ArccStdlibMap action unexpectedly came from an action cache")
	}
	mapPath := producerChainOutputPath(t, run.Execroot, *mapAction, ".stdlib-map.json")
	generated, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatalf("reading canonical generated map %q: %v", mapPath, err)
	}
	pinnedPath := filepath.Join(run.RepoRoot, "go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json")
	pinned, err := os.ReadFile(pinnedPath)
	if err != nil {
		t.Fatalf("reading pinned canonical map %q: %v", pinnedPath, err)
	}
	if !bytes.Equal(generated, pinned) {
		t.Fatalf("canonical default map differs from pinned map %q", pinnedPath)
	}
	decoded, err := artifactio.DecodeMap(bytes.NewReader(generated))
	if err != nil {
		t.Fatalf("decoding canonical generated map: %v", err)
	}
	if decoded.Key == nil || decoded.Key.ToolchainVersion != "go1.26.4" || decoded.Key.Goos != "linux" || decoded.Key.Goarch != "amd64" || decoded.Key.CgoEnabled || len(decoded.Key.BuildTags) != 0 || decoded.Key.Goexperiment != "" {
		t.Fatalf("canonical map SDK key = %#v, want go1.26.4/linux/amd64/cgo-disabled with no tags or experiment", decoded.Key)
	}
}
