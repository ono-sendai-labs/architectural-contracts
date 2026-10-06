package artifactio

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

// SurfacePath returns the native-mode companion surface path for a dependency
// manifest path: the manifest's extension deterministically replaced by
// `.surface.json` (DR-01). The authored manifest never names a surface path;
// the convention is the only locator (task req 6, AC7).
func SurfacePath(manifestPath string) string {
	return replaceManifestExt(manifestPath, ".surface.json")
}

// ReportPath returns the alongside report path for a dependency manifest path:
// the manifest's extension deterministically replaced by `.report.json`. It is
// reserved by convention; absence of the file is not an error.
func ReportPath(manifestPath string) string {
	return replaceManifestExt(manifestPath, ".report.json")
}

// replaceManifestExt swaps the manifest's extension for the given suffix,
// preserving the directory and base name.
func replaceManifestExt(manifestPath, suffix string) string {
	ext := filepath.Ext(manifestPath)
	return strings.TrimSuffix(manifestPath, ext) + suffix
}

// ReadSources is the shell-side filesystem boundary of surface emission
// (task req 1): it reads exactly the supplied member source paths — nothing
// else is enumerated or read — so dependency, test and unrelated files never
// reach the digest (task req 5). The result is sorted by canonical path.
func ReadSources(fsys fs.FS, paths []string) ([]surface.SourceFile, error) {
	sorted := slices.Clone(paths)
	slices.Sort(sorted)
	out := make([]surface.SourceFile, 0, len(sorted))
	for _, p := range sorted {
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read member source %s: %w", p, err)
		}
		out = append(out, surface.SourceFile{Path: p, Bytes: data})
	}
	return out, nil
}
