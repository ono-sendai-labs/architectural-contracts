package stdlibmap

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

// RenderTargetConfig renders the deterministic configuration-parameter file
// the explicit-input mode (and the Bazel stdlib-map rule of step 4 task 06)
// passes around: six fixed key=value lines in a fixed order, so analysis-time
// bytes are reproducible (task req 7).
func RenderTargetConfig(t TargetConfig) (string, error) {
	if t.ToolchainVersion == "" {
		return "", fmt.Errorf("rendering the target config: toolchain_version is required")
	}
	if t.GOOS == "" || t.GOARCH == "" {
		return "", fmt.Errorf("rendering the target config: goos and goarch are required")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "toolchain_version=%s\n", t.ToolchainVersion)
	fmt.Fprintf(&sb, "goos=%s\n", t.GOOS)
	fmt.Fprintf(&sb, "goarch=%s\n", t.GOARCH)
	fmt.Fprintf(&sb, "cgo_enabled=%t\n", t.CgoEnabled)
	fmt.Fprintf(&sb, "build_tags=%s\n", strings.Join(t.BuildTags, ","))
	fmt.Fprintf(&sb, "goexperiment=%s\n", t.GOEXPERIMENT)
	return sb.String(), nil
}

// ParseTargetConfig parses RenderTargetConfig's file format back into a
// TargetConfig (task req 7): every key must be present exactly once; an
// unknown, missing, duplicate, or malformed key is an error naming it. The
// cgo_enabled value must be exactly true or false.
func ParseTargetConfig(r io.Reader) (*TargetConfig, error) {
	fields := map[string]string{}
	order := []string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("parsing the target config: %q is not key=value", line)
		}
		if _, seen := fields[key]; seen {
			return nil, fmt.Errorf("parsing the target config: key %q appears twice", key)
		}
		fields[key] = value
		order = append(order, key)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parsing the target config: %w", err)
	}
	known := map[string]bool{}
	for _, key := range []string{"toolchain_version", "goos", "goarch", "cgo_enabled", "build_tags", "goexperiment"} {
		known[key] = true
		if _, ok := fields[key]; !ok {
			return nil, fmt.Errorf("parsing the target config: key %q is missing", key)
		}
	}
	for _, key := range order {
		if !known[key] {
			return nil, fmt.Errorf("parsing the target config: unknown key %q", key)
		}
	}
	cgo := false
	switch fields["cgo_enabled"] {
	case "true":
		cgo = true
	case "false":
	default:
		return nil, fmt.Errorf("parsing the target config: cgo_enabled %q is not true or false", fields["cgo_enabled"])
	}
	if fields["toolchain_version"] == "" || fields["goos"] == "" || fields["goarch"] == "" {
		return nil, fmt.Errorf("parsing the target config: toolchain_version, goos and goarch are required")
	}
	var tags []string
	if fields["build_tags"] != "" {
		tags = strings.Split(fields["build_tags"], ",")
	}
	return &TargetConfig{
		ToolchainVersion: fields["toolchain_version"],
		GOOS:             fields["goos"],
		GOARCH:           fields["goarch"],
		CgoEnabled:       cgo,
		BuildTags:        tags,
		GOEXPERIMENT:     fields["goexperiment"],
	}, nil
}

// ReadToolchainPackageList reads the explicit-input mode's package-list file —
// the toolchain's stdlib enumeration, one import path per line — and applies
// the `go list std` filter the layout discovery mirrors for free: no `cmd/…`
// tool tree and no path segment beginning with `_` or `.` (build-tool scratch
// directories). Vendored packages keep their `vendor/` spelling, matching both
// `go list std` and the layout discovery. Every retained path must be a
// canonical import path, so a malformed listing fails with an actionable error
// naming it instead of entering generation.
func ReadToolchainPackageList(r io.Reader) ([]string, error) {
	var paths []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		path := strings.TrimSpace(scanner.Text())
		if path == "" {
			continue
		}
		if path == "cmd" || strings.HasPrefix(path, "cmd/") || isToolScratchPath(path) {
			continue
		}
		if err := validatePackagePath(path); err != nil {
			return nil, fmt.Errorf("reading the toolchain package list: %w", err)
		}
		paths = append(paths, path)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading the toolchain package list: %w", err)
	}
	return paths, nil
}

// isToolScratchPath reports whether any segment of path is a tool-scratch
// directory the go tool ignores for package resolution: a segment beginning
// with `_` or `.`.
func isToolScratchPath(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "_") || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// ReconcilePackageList reconciles the explicit-input oracle — the toolchain's
// package list — against the whole-stdlib layout's discovery (task req 6,
// design I3): a listed package the layout discovers is enumerated under the
// existing internal/… rule; a listed package whose directory exists under the
// SDK root but that the target's build constraints excluded (or that carries
// no Go sources) is enumerated importable: false; a listed package with no
// directory, and a discovered package the list omits, fail with an error
// naming the package. The result is the total, canonical enumeration
// Generate's normalization consumes.
func ReconcilePackageList(listed []string, layout *packagelayout.Layout, sdkRoot string) ([]PackageEntry, error) {
	discovered := make(map[string]bool, len(layout.Packages))
	for _, p := range layout.Packages {
		discovered[p.PkgPath] = true
	}
	listedSet := make(map[string]bool, len(listed))
	entries := make([]PackageEntry, 0, len(listed))
	for _, path := range listed {
		listedSet[path] = true
		switch {
		case discovered[path]:
			entries = append(entries, PackageEntry{Path: path, Importable: !IsInternalPath(path)})
		case discovered["vendor/"+path]:
			// A list may spell a vendored package by its bare import path;
			// the discovered node is the vendored copy, which is the
			// enumerated package.
			entries = append(entries, PackageEntry{Path: "vendor/" + path, Importable: !IsInternalPath(path)})
		case directoryExists(filepath.Join(sdkRoot, filepath.FromSlash(path))):
			// The directory exists but the target's build constraints
			// excluded every file (or it carries no Go sources): enumerated,
			// not importable.
			entries = append(entries, PackageEntry{Path: path, Importable: false})
		default:
			return nil, fmt.Errorf("reconciling the package list with the SDK: listed package %q has no directory in the SDK root %q", path, sdkRoot)
		}
	}
	// Every discovered package the list omits is a discrepancy: the oracle is
	// the enumeration, so an unlisted discovery would silently shrink the
	// map's totality (I3).
	for _, p := range layout.Packages {
		if listedSet[p.PkgPath] || listedSet[strings.TrimPrefix(p.PkgPath, "vendor/")] {
			continue
		}
		return nil, fmt.Errorf("reconciling the package list with the SDK: package %q was discovered in the SDK but the package list omits it", p.PkgPath)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func directoryExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
