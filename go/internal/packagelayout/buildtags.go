package packagelayout

import (
	"bufio"
	"bytes"
	"fmt"
	"go/build/constraint"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout/internal/hostenv"
)

// BuildTags models the platform identities, compiler, release tags, and custom
// build tags needed to evaluate Go build constraints without ambient authority.
type BuildTags struct {
	GOOS        string
	GOARCH      string
	GOROOT      string
	GOPATH      string
	Compiler    string
	CgoEnabled  bool
	ReleaseTags []string
	BuildTags   []string
	ToolTags    []string
}

// NativeGoVersion returns the version string of the host Go runtime.
func NativeGoVersion() string {
	return hostenv.NativeTargetDefaults().Version
}

// DriverRequestFromEnv checks if GOPACKAGESDRIVER mode was requested via the environment.
func DriverRequestFromEnv() (ok bool, layoutPath, workspaceDir string) {
	return hostenv.DriverRequestFromEnv()
}

// HasTag reports whether the given tag is satisfied by the BuildTags configuration.
func (b BuildTags) HasTag(tag string) bool {
	if tag == "cgo" {
		return b.CgoEnabled
	}
	if tag == b.GOOS || tag == b.GOARCH || tag == b.Compiler {
		return true
	}
	if (b.GOOS == "android" && tag == "linux") ||
		(b.GOOS == "illumos" && tag == "solaris") ||
		(b.GOOS == "ios" && tag == "darwin") {
		return true
	}
	if tag == "unix" {
		switch b.GOOS {
		case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
			return true
		}
	}
	if tag == "boringcrypto" {
		tag = "goexperiment.boringcrypto"
	}
	for _, candidate := range b.BuildTags {
		if candidate == tag {
			return true
		}
	}
	for _, candidate := range b.ToolTags {
		if candidate == tag {
			return true
		}
	}
	for _, candidate := range b.ReleaseTags {
		if candidate == tag {
			return true
		}
	}
	return false
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

// BuildTagsForLayout returns the build tags declared by l. Its native
// baseline is assembled from the running toolchain identity rather than the
// mutable build.Default value.
func BuildTagsForLayout(l *Layout) (BuildTags, error) {
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

// FileMatchesBuildConstraints evaluates filename suffixes and //go:build directives
// in src against tags.
func FileMatchesBuildConstraints(tags BuildTags, path string, src []byte) (bool, error) {
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

func fileMatchesBuildConstraintsWithTags(path string, tags BuildTags) bool {
	if !strings.HasSuffix(path, ".go") {
		return true
	}
	if !filenameMatchesBuildTags(filepath.Base(path), tags) {
		return false
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	matched, err := FileMatchesBuildConstraints(tags, path, src)
	if err != nil {
		return true
	}
	return matched
}
