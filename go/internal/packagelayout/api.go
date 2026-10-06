package packagelayout

import (
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout/internal/hostenv"
	"golang.org/x/tools/go/packages"
)

// NativeGoVersion returns the version string of the host Go runtime.
func NativeGoVersion() string {
	return hostenv.NativeTargetDefaults().Version
}

// DriverRequestFromEnv checks if GOPACKAGESDRIVER mode was requested via the environment.
func DriverRequestFromEnv() (ok bool, layoutPath, workspaceDir string) {
	return hostenv.DriverRequestFromEnv()
}

// BuildTagsForLayout returns the build tags declared by l. Its native
// baseline is assembled from the running toolchain identity rather than the
// mutable build.Default value.
func BuildTagsForLayout(l *Layout) (BuildTags, error) {
	return buildTagsForLayout(l)
}

// FileMatchesBuildConstraints evaluates filename suffixes and //go:build directives
// in src against tags.
func FileMatchesBuildConstraints(tags BuildTags, path string, src []byte) (bool, error) {
	return fileMatchesBuildConstraintsForBytes(tags, path, src)
}

// ShapeImports projects a package import map into its stable JSON identity
// form. The caller supplies the host namespace policy because package-layout
// owns the package graph while the host chooses the canonical import namespace.
func ShapeImports(imports map[string]*packages.Package, canonicalize func(string) string) (map[string]string, error) {
	if imports == nil {
		return nil, nil
	}
	if canonicalize == nil {
		canonicalize = func(path string) string { return path }
	}
	shaped := make(map[string]string, len(imports))
	for importPath, pkg := range imports {
		if pkg == nil || pkg.ID == "" {
			return nil, fmt.Errorf("import %q has no package ID", importPath)
		}
		canonicalImport := canonicalize(importPath)
		canonicalID := canonicalize(pkg.ID)
		if _, exists := shaped[canonicalImport]; exists {
			return nil, fmt.Errorf("canonical import path %q has multiple entries", canonicalImport)
		}
		shaped[canonicalImport] = canonicalID
	}
	return shaped, nil
}

// StdlibLayout computes the whole-standard-library layout for sdk under the
// pinned target platform.
func StdlibLayout(sdk fs.FS, platform *Platform) (*Layout, error) {
	return stdlibLayout(sdk, platform)
}

// MergeImportGraphLayout applies the exact ordinary source-import graph to a
// base layout artifact and returns the merged layout bytes.
func MergeImportGraphLayout(base, graph []byte) ([]byte, error) {
	return mergeImportGraphLayout(base, graph)
}

// WriteImportGraph parses the selected source files described by config and
// returns the deterministic exact direct-import descriptor bytes.
func WriteImportGraph(config []byte) ([]byte, error) {
	return writeImportGraph(config, defaultFS)
}

// DecodeStdlibExportFiles reads a rules_go flat-package JSON stream from r and
// returns all non-empty ExportFile paths in stream order.
func DecodeStdlibExportFiles(r io.Reader) ([]string, error) {
	return decodeStdlibExportFiles(r)
}

// RunDriver executes the complete GOPACKAGESDRIVER protocol logic against parsed layout bytes.
func RunDriver(layout []byte, layoutPath, workspaceDir string, patterns []string, stdin io.Reader, stdout io.Writer) error {
	return runDriverImpl(layout, layoutPath, workspaceDir, patterns, stdin, stdout)
}

// CheckMu is a global mutex to serialize all concurrent check executions
// in the same process, protecting against environment and global state races.
var CheckMu sync.Mutex

// IsLayoutMode reports whether package-layout mode is currently active.
func IsLayoutMode() bool {
	return isLayoutMode()
}

// GetActiveLayout returns the active package layout, or nil if not in layout mode.
func GetActiveLayout() *Layout {
	return getActiveLayout()
}

// GetActiveLayoutPath returns the path to the active package layout file.
func GetActiveLayoutPath() string {
	return getActiveLayoutPath()
}

// GetActiveWorkspaceDir returns the active workspace directory.
func GetActiveWorkspaceDir() string {
	return getActiveWorkspaceDir()
}

// WithDriverEnv configures GOPACKAGESDRIVER to point to the current executable
// and loads the layout file, caching it and making it available in layout mode.
// It restores the original environment and variables when fn returns.
func WithDriverEnv(layoutPath, workspaceDir string, fn func() error) error {
	return withDriverEnvImpl(layoutPath, workspaceDir, fn)
}
