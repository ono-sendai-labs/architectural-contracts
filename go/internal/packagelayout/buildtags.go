package packagelayout

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build/constraint"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout/internal/hostenv"
)

// supportedGoTargets mirrors the Go 1.26 target list reported by
// `go tool dist list`, the supported-target source for this module's Go
// toolchain. Keep this list synchronized when the module's Go version changes;
// unlike go/build's internal tag tables, it excludes retired and broken ports.
var supportedGoTargets = [...]string{
	"aix/ppc64",
	"android/386", "android/amd64", "android/arm", "android/arm64",
	"darwin/amd64", "darwin/arm64",
	"dragonfly/amd64",
	"freebsd/386", "freebsd/amd64", "freebsd/arm", "freebsd/arm64",
	"illumos/amd64",
	"ios/amd64", "ios/arm64",
	"js/wasm",
	"linux/386", "linux/amd64", "linux/arm", "linux/arm64", "linux/loong64",
	"linux/mips", "linux/mips64", "linux/mips64le", "linux/mipsle",
	"linux/ppc64", "linux/ppc64le", "linux/riscv64", "linux/s390x",
	"netbsd/386", "netbsd/amd64", "netbsd/arm", "netbsd/arm64",
	"openbsd/386", "openbsd/amd64", "openbsd/arm", "openbsd/arm64",
	"openbsd/ppc64", "openbsd/riscv64",
	"plan9/386", "plan9/amd64", "plan9/arm",
	"solaris/amd64",
	"wasip1/wasm",
	"windows/386", "windows/amd64", "windows/arm64",
}

func supportedGOOS(goos string) bool {
	for _, target := range supportedGoTargets {
		if targetOS, _, _ := strings.Cut(target, "/"); targetOS == goos {
			return true
		}
	}
	return false
}

func supportedGOARCH(goarch string) bool {
	for _, target := range supportedGoTargets {
		_, targetArch, _ := strings.Cut(target, "/")
		if targetArch == goarch {
			return true
		}
	}
	return false
}

func supportedGoTarget(goos, goarch string) bool {
	target := goos + "/" + goarch
	for _, supported := range supportedGoTargets {
		if supported == target {
			return true
		}
	}
	return false
}

// baselineExperimentTags mirrors internal/buildcfg's baseline experiment
// configuration for this module's Go toolchain version (the experiments
// enabled by default for a target configuration, before any GOEXPERIMENT
// override): regabi wrappers/args on the register-ABI ports, DWARF5 where the
// platform's debug tooling supports it, and the always-on experiments. Keep
// this synchronized with the toolchain this module pins (see
// supportedGoTargets).
func baselineExperimentTags(goos, goarch string) []string {
	var tags []string
	switch goarch {
	case "amd64", "arm64", "loong64", "ppc64", "ppc64le", "riscv64":
		tags = append(tags, "goexperiment.regabiwrappers", "goexperiment.regabiargs")
	}
	switch goos {
	case "darwin", "ios", "aix":
	default:
		tags = append(tags, "goexperiment.dwarf5")
	}
	tags = append(tags, "goexperiment.greenteagc", "goexperiment.randomizedheapbase64")
	return tags
}

// effectiveExperimentTags derives the `goexperiment.<name>` tool tags of a
// target configuration: the baseline experiments for the target plus the
// GOEXPERIMENT value's overrides, using the toolchain's own semantics — a
// comma-separated list where `none` disables every experiment and a `no`
// prefix disables a single one. Every experiment name must be a valid Go
// identifier; a malformed name fails naming it. The result is sorted.
func effectiveExperimentTags(goos, goarch, goexperiment string) ([]string, error) {
	enabled := map[string]bool{}
	for _, tag := range baselineExperimentTags(goos, goarch) {
		enabled[strings.TrimPrefix(tag, "goexperiment.")] = true
	}
	for _, f := range strings.Split(goexperiment, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if f == "none" {
			enabled = map[string]bool{}
			continue
		}
		name, on := f, true
		if off, ok := strings.CutPrefix(f, "no"); ok {
			name, on = off, false
		}
		if !isGoIdentifier(name) {
			return nil, fmt.Errorf("platform.goexperiment has invalid experiment name %q", name)
		}
		enabled[name] = on
	}
	var tags []string
	for name, on := range enabled {
		if on {
			tags = append(tags, "goexperiment."+name)
		}
	}
	sort.Strings(tags)
	return tags, nil
}

// releaseTagsForVersion derives the release tags a `go1.N(.M)` toolchain
// version enables: `go1.1` … `go1.N` (the last element is the current release).
func releaseTagsForVersion(version string) ([]string, error) {
	major, ok := strings.CutPrefix(version, "go1.")
	if !ok {
		return nil, fmt.Errorf("platform.toolchain_version %q is not a go1.N(.M) toolchain version", version)
	}
	major, _, _ = strings.Cut(major, ".")
	n, err := strconv.Atoi(major)
	if err != nil || n < 1 {
		return nil, fmt.Errorf("platform.toolchain_version %q is not a go1.N(.M) toolchain version", version)
	}
	tags := make([]string, n)
	for i := range tags {
		tags[i] = "go1." + strconv.Itoa(i+1)
	}
	return tags, nil
}

// isGoIdentifier reports whether name is a valid Go identifier (used for
// experiment names, which are identifiers).
func isGoIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// defaultArchToolTags returns the arch feature tool tags the Go toolchain
// enables by default for the target GOARCH — the same tags
// internal/buildcfg's gogoarchTags derives for a target configuration with no
// GO$GOARCH override, mirrored here because buildcfg is internal to the
// toolchain and a layout must describe a foreign target. The host's
// (possibly non-default) feature levels are never copied.
func defaultArchToolTags(goarch string) []string {
	switch goarch {
	case "386":
		return []string{"386.sse2"}
	case "amd64":
		return []string{"amd64.v1"}
	case "arm":
		return []string{"arm.5", "arm.6", "arm.7"}
	case "arm64":
		return []string{"arm64.v8.0"}
	case "mips", "mipsle", "mips64", "mips64le":
		return []string{goarch + ".hardfloat"}
	case "ppc64", "ppc64le":
		return []string{goarch + ".power8"}
	case "riscv64":
		return []string{"riscv64.rva20u64"}
	default:
		return nil
	}
}

// nativeBuildTags reconstructs the fields of build.Default that the layout
// code consumes, backed by hostenv.NativeTargetDefaults().
func nativeBuildTags() BuildTags {
	d := hostenv.NativeTargetDefaults()
	releaseTags, _ := releaseTagsForVersion(d.Version)
	toolTags := baselineExperimentTags(d.GOOS, d.GOARCH)
	toolTags = append(toolTags, defaultArchToolTags(d.GOARCH)...)
	return BuildTags{
		GOOS:        d.GOOS,
		GOARCH:      d.GOARCH,
		GOROOT:      d.GOROOT,
		GOPATH:      d.GOPATH,
		CgoEnabled:  d.CgoEnabled,
		Compiler:    d.Compiler,
		ToolTags:    toolTags,
		ReleaseTags: releaseTags,
	}
}

func buildTagsForLayout(l *Layout) (BuildTags, error) {
	tags := nativeBuildTags()
	tags.BuildTags = slices.Clone(tags.BuildTags)
	tags.ToolTags = slices.Clone(tags.ToolTags)
	tags.ReleaseTags = slices.Clone(tags.ReleaseTags)
	if l == nil || l.Platform == nil {
		return tags, nil
	}

	platform := l.Platform
	if !supportedGOOS(platform.GOOS) {
		return BuildTags{}, fmt.Errorf("platform.goos has invalid value %q", platform.GOOS)
	}
	if !supportedGOARCH(platform.GOARCH) {
		return BuildTags{}, fmt.Errorf("platform.goarch has invalid value %q", platform.GOARCH)
	}
	if !supportedGoTarget(platform.GOOS, platform.GOARCH) {
		return BuildTags{}, fmt.Errorf("platform.goarch has unsupported value %q for goos %q", platform.GOARCH, platform.GOOS)
	}
	tags.GOOS = platform.GOOS
	tags.GOARCH = platform.GOARCH
	tags.BuildTags = append([]string(nil), platform.BuildTags...)
	tags.CgoEnabled = platform.CgoEnabled

	pinned := platform.ToolchainVersion != nil || platform.GOEXPERIMENT != nil
	if pinned {
		if platform.ToolchainVersion != nil {
			relTags, err := releaseTagsForVersion(*platform.ToolchainVersion)
			if err != nil {
				return BuildTags{}, err
			}
			tags.ReleaseTags = relTags
		}
		goexperiment := ""
		if platform.GOEXPERIMENT != nil {
			goexperiment = *platform.GOEXPERIMENT
		}
		experiments, err := effectiveExperimentTags(platform.GOOS, platform.GOARCH, goexperiment)
		if err != nil {
			return BuildTags{}, err
		}
		tags.ToolTags = defaultArchToolTags(platform.GOARCH)
		tags.ToolTags = append(tags.ToolTags, experiments...)
		sort.Strings(tags.ToolTags)
	}
	return tags, nil
}

func fileMatchesBuildConstraintsForBytes(tags BuildTags, path string, src []byte) (bool, error) {
	if !strings.HasSuffix(path, ".go") {
		return false, nil
	}
	if !filenameMatchesBuildTags(filepath.Base(path), tags) {
		return false, nil
	}
	if _, expr, ok := leadingBuildConstraintFromBytes(src); ok {
		matched := expr.Eval(tags.HasTag)
		return matched, nil
	}
	return true, nil
}

func fileMatchesBuildConstraints(path string) bool {
	return fileMatchesBuildConstraintsWithTags(defaultFS, path, nativeBuildTags())
}

func filenameMatchesBuildTags(name string, tags BuildTags) bool {
	if !strings.HasSuffix(name, ".go") {
		return true
	}
	base := strings.TrimSuffix(name, ".go")
	base = strings.TrimSuffix(base, "_test")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return true
	}
	last := parts[len(parts)-1]
	if len(parts) >= 3 && isFilenameGOOS(parts[len(parts)-2]) && isFilenameGOARCH(last) {
		return tags.HasTag(parts[len(parts)-2]) && tags.HasTag(last)
	}
	if isFilenameGOOS(last) || isFilenameGOARCH(last) {
		return tags.HasTag(last)
	}
	return true
}

func leadingBuildConstraintFromBytes(src []byte) (string, constraint.Expr, bool) {
	var legacy string
	var legacyExpr constraint.Expr
	var goBuild string
	var goBuildExpr constraint.Expr
	scanner := bufio.NewScanner(bytes.NewReader(src))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "//") {
			break
		}
		if constraint.IsGoBuild(line) {
			if expression, err := constraint.Parse(line); err == nil && goBuild == "" {
				goBuild = strings.Join(strings.Fields(line), " ")
				goBuildExpr = expression
			}
		}
		if constraint.IsPlusBuild(line) && legacy == "" {
			if expression, err := constraint.Parse(line); err == nil {
				legacy = strings.Join(strings.Fields(line), " ")
				legacyExpr = expression
			}
		}
	}
	if scanner.Err() != nil {
		return "", nil, false
	}
	if goBuildExpr != nil {
		return goBuild, goBuildExpr, true
	}
	if legacyExpr != nil {
		return legacy, legacyExpr, true
	}
	return "", nil, false
}

func fileMatchesBuildConstraintsWithTags(fsys fs.FS, path string, tags BuildTags) bool {
	if !strings.HasSuffix(path, ".go") {
		return true
	}
	if !filenameMatchesBuildTags(filepath.Base(path), tags) {
		return false
	}
	src, err := fs.ReadFile(fsys, filepath.ToSlash(path))
	if err != nil {
		return true
	}
	matched, err := fileMatchesBuildConstraintsForBytes(tags, path, src)
	if err != nil {
		return true
	}
	return matched
}

func isFilenameGOOS(value string) bool {
	switch value {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js", "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos":
		return true
	default:
		return false
	}
}

func isFilenameGOARCH(value string) bool {
	switch value {
	case "386", "amd64", "amd64p32", "arm", "arm64", "arm64be", "armbe", "loong64", "mips", "mips64", "mips64le", "mips64p32", "mips64p32le", "mipsle", "ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390", "s390x", "sparc", "sparc64", "wasm":
		return true
	default:
		return false
	}
}
