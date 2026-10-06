package goanalysis

import (
	"io/fs"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

// DependencySurfaceMode selects the structural artifact source. Layout mode
// receives an explicit Task 1 build-graph binding; native mode uses the
// sibling-file convention and never manufactures build-graph provenance.
type DependencySurfaceMode string

const (
	DependencySurfaceNative DependencySurfaceMode = "native"
	DependencySurfaceLayout DependencySurfaceMode = "layout"
)

// DependencySurfaceReadFile is the narrow byte-reader seam used for artifacts,
// native manifests, and native source files. It intentionally returns bytes;
// the resolver never hands dependency source to a Go parser or package loader.
type DependencySurfaceReadFile func(path string) ([]byte, error)

// DependencySurfaceReadDir is the narrow directory-enumeration seam used by
// the default native byte reader. Directory names are metadata; file contents
// are read only after the surface has named the concrete member packages.
type DependencySurfaceReadDir func(path string) ([]fs.DirEntry, error)

// DependencySurfaceReadSources is an optional native freshness seam. A host
// may provide the already-enumerated member source byte set; returning an
// error means that freshness is UNKNOWN, not that surface resolution fails.
type DependencySurfaceReadSources func(root string, packages []string) ([]surface.SourceFile, error)

// DependencySurfaceRequest contains every input that can affect dependency
// surface resolution. The dependency identity is authored manifest metadata;
// Binding is structural Bazel metadata and is required in layout mode. The
// request carries the current namespace and complete target SDK identity
// explicitly so a caller cannot accidentally compare an artifact against a
// host-default configuration.
type DependencySurfaceRequest struct {
	// DeclaringRoot resolves Dependency.Manifest in native mode. It is not
	// used to locate a layout binding, whose paths are workspace-frame paths.
	DeclaringRoot string
	Dependency    manifest.ComponentDependency
	Binding       *packagelayout.DependencyArtifactBinding

	// Namespace is the current hostpolicy.NamespaceID. Empty uses the current
	// upstream default for convenience; a non-empty value is compared exactly.
	Namespace string
	// ExpectedSDKKey is the target configuration the surface must describe.
	ExpectedSDKKey stdlibauthority.SDKKey
	// ExpectedSDK is a compatibility spelling for callers of the initial
	// additive API. When ExpectedSDKKey is zero, it supplies the expected key.
	ExpectedSDK stdlibauthority.SDKKey

	Mode         DependencySurfaceMode
	WorkspaceDir string

	// ReadFile and ReadDir are deliberately byte/filesystem seams. When nil,
	// the resolver uses os.ReadFile/os.ReadDir. ReadSourceFiles, when present,
	// replaces only native member-source enumeration for freshness.
	ReadFile        DependencySurfaceReadFile
	ReadDir         DependencySurfaceReadDir
	ReadSourceFiles DependencySurfaceReadSources
}

// ResolveDependencySurface consumes one validated surface and its optional
// report into exact facts.DependencyInterface values. This is the production
// dependency boundary: after the Step 7 cutover no dependency package is
// loaded to reconstruct an architectural surface.
//
// No dependency package is loaded, parsed, type-checked, or passed to
// go/packages here. The only native freshness work is a byte comparison over
// the concrete package/source set that can be established from filesystem
// metadata and the surface.
func ResolveDependencySurface(req DependencySurfaceRequest) (facts.DependencyInterface, error) {
	return resolveDependencySurface(req)
}

// ResolveDependencySurfaceForDependency is the shell-facing convenience
// boundary used by the CLI. In layout mode it looks up the active structural
// provider binding by dependency name inside goanalysis, keeping the concrete
// packagelayout binding type out of callers' source-level dependency contracts.
// Native callers retain the sibling-file convention. The surface and report
// are still validated by ResolveDependencySurface; this helper only supplies
// the already-active binding.
func ResolveDependencySurfaceForDependency(req DependencySurfaceRequest) (facts.DependencyInterface, error) {
	return resolveDependencySurfaceForDependency(req)
}
