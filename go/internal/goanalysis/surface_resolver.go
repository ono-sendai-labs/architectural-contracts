package goanalysis

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// DependencySurfaceMode selects the structural artifact source. Layout mode
// receives an explicit Task 1 build-graph binding; native mode uses the
// sibling-file convention and never manufactures build-graph provenance.
type DependencySurfaceMode string

const (
	DependencySurfaceNative DependencySurfaceMode = "native"
	DependencySurfaceLayout DependencySurfaceMode = "layout"
	// The longer names make call sites self-documenting and preserve a stable
	// vocabulary for hosts that prefer not to rely on the short aliases.
	DependencySurfaceModeNative = DependencySurfaceNative
	DependencySurfaceModeLayout = DependencySurfaceLayout
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

// SurfaceResolutionRequest is an alternate name kept for the shell-facing
// request type; both names describe the same operation.
type SurfaceResolutionRequest = DependencySurfaceRequest

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
	if err := validateSurfaceRequest(req); err != nil {
		return facts.DependencyInterface{}, err
	}

	readFile := req.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	readArtifact := func(path string) ([]byte, error) {
		data, err := readFile(path)
		if err == nil || req.Mode != DependencySurfaceLayout {
			return data, err
		}
		alternate := alternateLayoutArtifactPath(req, path)
		if alternate == path {
			return nil, err
		}
		if alternateData, alternateErr := readFile(alternate); alternateErr == nil {
			return alternateData, nil
		}
		return nil, err
	}
	paths, err := dependencyArtifactPaths(req)
	if err != nil {
		return facts.DependencyInterface{}, err
	}

	surfaceBytes, err := readArtifact(paths.surface)
	if err != nil {
		alternate := alternateLayoutArtifactPath(req, paths.surface)
		if alternate != paths.surface {
			return facts.DependencyInterface{}, fmt.Errorf("dependency %q surface %q (alternate %q): %w", req.Dependency.Name, paths.surface, alternate, err)
		}
		return facts.DependencyInterface{}, fmt.Errorf("dependency %q surface %q: %w", req.Dependency.Name, paths.surface, err)
	}
	decoded, err := artifactio.DecodeSurface(bytes.NewReader(surfaceBytes))
	if err != nil {
		return facts.DependencyInterface{}, fmt.Errorf("dependency %q surface %q is invalid: %w", req.Dependency.Name, paths.surface, err)
	}

	validated, err := validateDecodedSurface(req, decoded)
	if err != nil {
		return facts.DependencyInterface{}, err
	}

	verdict, reportPresent, err := readDependencyReport(req, paths.report, readArtifact, paths.reportRequired)
	if err != nil {
		return facts.DependencyInterface{}, err
	}

	provenance, freshness, err := deriveStatuses(req, validated, verdict, reportPresent)
	if err != nil {
		return facts.DependencyInterface{}, err
	}

	return facts.DependencyInterface{
		Component:      req.Dependency.Name,
		InterfaceStyle: validated.style,
		Packages:       slices.Clone(validated.packages),
		Symbols:        slices.Clone(validated.symbols),
		Provenance:     provenance,
		Freshness:      freshness,
		Authority:      validated.authority,
	}, nil
}

// alternateLayoutArtifactPath handles the two equivalent Bazel execution
// frames: the layout stores runfiles-root paths beginning with the workspace
// name, while the action process may already start inside that workspace
// directory. The fallback still addresses the exact declared artifact and
// never searches the filesystem.
func alternateLayoutArtifactPath(req DependencySurfaceRequest, path string) string {
	if req.Binding == nil {
		return path
	}
	workspace := req.WorkspaceDir
	if workspace == "" {
		workspace = packagelayout.GetActiveWorkspaceDir()
	}
	if workspace == "" {
		return path
	}
	frame := filepath.ToSlash(path)
	if req.Binding.Surface != "" && strings.Contains(frame, filepath.ToSlash(req.Binding.Surface)) {
		frame = filepath.ToSlash(req.Binding.Surface)
	} else if req.Binding.Report != "" && strings.Contains(frame, filepath.ToSlash(req.Binding.Report)) {
		frame = filepath.ToSlash(req.Binding.Report)
	}
	first, _, _ := strings.Cut(frame, "/")
	// In a Bazel sandbox the declared generated input is staged at its
	// execution-root `bazel-out/<configuration>/bin/...` path, while the
	// layout intentionally keeps the runfiles spelling. Derive that exact
	// sibling path from the declared layout artifact; this is a path transform,
	// not a directory search or an undeclared read.
	if layoutPath := packagelayout.GetActiveLayoutPath(); layoutPath != "" {
		normalizedLayout := filepath.ToSlash(layoutPath)
		if marker := strings.Index(normalizedLayout, "/bazel-out/"); marker >= 0 {
			rest := normalizedLayout[marker+len("/bazel-out/"):]
			parts := strings.SplitN(rest, "/", 3)
			if len(parts) == 3 && parts[1] == "bin" {
				frameWithoutWorkspace := frame
				workspaceName := filepath.Base(filepath.Clean(workspace))
				if strings.HasPrefix(frameWithoutWorkspace, workspaceName+"/") {
					frameWithoutWorkspace = strings.TrimPrefix(frameWithoutWorkspace, workspaceName+"/")
				}
				return filepath.Join(normalizedLayout[:marker], "bazel-out", parts[0], parts[1], filepath.FromSlash(frameWithoutWorkspace))
			}
		}
	}
	if first == "" || first != filepath.Base(filepath.Clean(workspace)) {
		return path
	}
	return filepath.Join(filepath.Dir(filepath.Clean(workspace)), filepath.FromSlash(frame))
}

// ResolveValidatedDependencySurface is the explicit fail-closed spelling for
// callers that want the validation boundary named in their code.
func ResolveValidatedDependencySurface(req DependencySurfaceRequest) (facts.DependencyInterface, error) {
	return ResolveDependencySurface(req)
}

type dependencyArtifactLocations struct {
	surface        string
	report         string
	reportRequired bool
}

func validateSurfaceRequest(req DependencySurfaceRequest) error {
	if req.Dependency.Name == "" {
		return errors.New("dependency surface resolver: dependency name is empty")
	}
	if req.Dependency.Manifest == "" {
		return fmt.Errorf("dependency %q surface resolver: manifest path is empty", req.Dependency.Name)
	}
	switch req.Mode {
	case DependencySurfaceNative, DependencySurfaceLayout:
	default:
		return fmt.Errorf("dependency %q surface resolver: unknown operating mode %q", req.Dependency.Name, req.Mode)
	}
	if req.Mode == DependencySurfaceNative && req.DeclaringRoot == "" {
		return fmt.Errorf("dependency %q surface resolver: native mode requires declaring root", req.Dependency.Name)
	}
	if req.Mode == DependencySurfaceLayout && req.Binding == nil {
		return fmt.Errorf("dependency %q surface resolver: layout mode requires a dependency artifact binding", req.Dependency.Name)
	}
	if req.Namespace == "" {
		req.Namespace = hostpolicy.NamespaceID
	}
	if req.Namespace == "" {
		return fmt.Errorf("dependency %q surface resolver: namespace is empty", req.Dependency.Name)
	}
	key := expectedSDKKey(req)
	if err := validateExpectedSDKKey(key); err != nil {
		return fmt.Errorf("dependency %q surface resolver: expected SDK key: %w", req.Dependency.Name, err)
	}
	if req.Mode == DependencySurfaceLayout {
		if err := validateBinding(*req.Binding, req.Dependency.Name); err != nil {
			return err
		}
	}
	return nil
}

func expectedSDKKey(req DependencySurfaceRequest) stdlibauthority.SDKKey {
	if !isZeroSDKKey(req.ExpectedSDKKey) {
		return req.ExpectedSDKKey
	}
	return req.ExpectedSDK
}

func isZeroSDKKey(k stdlibauthority.SDKKey) bool {
	return k.ToolchainVersion == "" && k.GOOS == "" && k.GOARCH == "" && !k.CgoEnabled &&
		len(k.BuildTags) == 0 && k.GOEXPERIMENT == "" && k.ClassifierHash == "" && k.MapFormatVersion == 0
}

func validateExpectedSDKKey(k stdlibauthority.SDKKey) error {
	var missing []string
	if k.ToolchainVersion == "" {
		missing = append(missing, "toolchain_version")
	}
	if k.GOOS == "" {
		missing = append(missing, "goos")
	}
	if k.GOARCH == "" {
		missing = append(missing, "goarch")
	}
	if k.ClassifierHash == "" {
		missing = append(missing, "classifier_hash")
	}
	if k.MapFormatVersion == 0 {
		missing = append(missing, "map_format_version")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return validateSortedTags(k.BuildTags, "expected SDK key")
}

func validateSortedTags(tags []string, what string) error {
	for i := 1; i < len(tags); i++ {
		if tags[i] == tags[i-1] {
			return fmt.Errorf("%s has duplicate build tag %q", what, tags[i])
		}
		if tags[i] < tags[i-1] {
			return fmt.Errorf("%s build tags are not sorted: %q occurs after %q", what, tags[i], tags[i-1])
		}
	}
	return nil
}

func validateBinding(binding packagelayout.DependencyArtifactBinding, dependency string) error {
	if binding.Dependency != dependency {
		return fmt.Errorf("dependency %q surface resolver: binding names dependency %q", dependency, binding.Dependency)
	}
	if binding.Surface == "" {
		return fmt.Errorf("dependency %q surface resolver: binding has empty surface path", dependency)
	}
	if err := validateArtifactFramePath(binding.Surface, dependency, "surface"); err != nil {
		return err
	}
	switch binding.Provenance {
	case packagelayout.DependencyArtifactProvenanceChecked:
		if binding.Report == "" {
			return fmt.Errorf("dependency %q surface resolver: checked binding has no report path", dependency)
		}
	case packagelayout.DependencyArtifactProvenanceAsserted:
		if binding.Report != "" {
			return fmt.Errorf("dependency %q surface resolver: asserted binding must not have a report path", dependency)
		}
	default:
		return fmt.Errorf("dependency %q surface resolver: unknown binding provenance %q", dependency, binding.Provenance)
	}
	if binding.Report != "" {
		if err := validateArtifactFramePath(binding.Report, dependency, "report"); err != nil {
			return err
		}
	}
	return nil
}

func validateArtifactFramePath(value, dependency, kind string) error {
	if value == "" || value == "." || strings.Contains(value, "\\") || strings.IndexFunc(value, unicode.IsControl) >= 0 || pathpkg.IsAbs(value) || filepath.VolumeName(value) != "" || hasWindowsVolumePrefix(value) {
		return fmt.Errorf("dependency %q surface resolver: unsafe %s artifact path %q", dependency, kind, value)
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("dependency %q surface resolver: unsafe %s artifact path %q", dependency, kind, value)
	}
	if pathpkg.Clean(value) != value {
		return fmt.Errorf("dependency %q surface resolver: non-normalized %s artifact path %q", dependency, kind, value)
	}
	return nil
}

func hasWindowsVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func dependencyArtifactPaths(req DependencySurfaceRequest) (dependencyArtifactLocations, error) {
	if req.Mode == DependencySurfaceNative {
		manifestPath := filepath.Clean(filepath.Join(req.DeclaringRoot, req.Dependency.Manifest))
		return dependencyArtifactLocations{
			surface:        artifactio.SurfacePath(manifestPath),
			report:         artifactio.ReportPath(manifestPath),
			reportRequired: false,
		}, nil
	}
	workspace := req.WorkspaceDir
	if workspace == "" {
		workspace = packagelayout.GetActiveWorkspaceDir()
	}
	if workspace == "" {
		return dependencyArtifactLocations{}, fmt.Errorf("dependency %q surface resolver: layout mode requires a workspace directory", req.Dependency.Name)
	}
	binding := req.Binding
	paths := dependencyArtifactLocations{
		surface: filepath.Join(workspace, filepath.FromSlash(binding.Surface)),
	}
	if binding.Report != "" {
		paths.report = filepath.Join(workspace, filepath.FromSlash(binding.Report))
	}
	paths.reportRequired = binding.Provenance == packagelayout.DependencyArtifactProvenanceChecked
	return paths, nil
}

type validatedDependencySurface struct {
	manifest  *gen.SurfaceManifest
	style     manifest.InterfaceStyle
	authority manifest.AuthorityDeclaration
	packages  []string
	symbols   []facts.SymbolID
}

func validateDecodedSurface(req DependencySurfaceRequest, m *gen.SurfaceManifest) (validatedDependencySurface, error) {
	if m == nil {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface is nil", req.Dependency.Name)
	}
	if m.Component == "" || m.Component != req.Dependency.Name {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface component mismatch: got %q", req.Dependency.Name, m.Component)
	}
	if m.ProducerVersion == "" {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface producer_version is empty", req.Dependency.Name)
	}
	namespace := req.Namespace
	if namespace == "" {
		namespace = hostpolicy.NamespaceID
	}
	if m.Namespace == "" || m.Namespace != namespace {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface namespace mismatch: got %q, want %q", req.Dependency.Name, m.Namespace, namespace)
	}
	if err := validateSurfaceSDKKey(req, m.SdkKey); err != nil {
		return validatedDependencySurface{}, err
	}
	authority, err := validateSurfaceAuthority(req.Dependency.Name, m.Authority)
	if err != nil {
		return validatedDependencySurface{}, err
	}

	style, err := surfaceStyle(m.InterfaceStyle)
	if err != nil {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface interface style: %w", req.Dependency.Name, err)
	}
	if len(m.Packages) == 0 {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface has no concrete member packages", req.Dependency.Name)
	}
	packages := slices.Clone(m.Packages)
	if !sort.StringsAreSorted(packages) {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface packages are not in canonical order", req.Dependency.Name)
	}
	for _, pkg := range packages {
		if err := validateSurfacePackagePath(req.Dependency.Name, pkg); err != nil {
			return validatedDependencySurface{}, err
		}
	}
	packageSet := make(map[string]bool, len(packages))
	for _, pkg := range packages {
		packageSet[pkg] = true
	}

	if style == manifest.InterfaceStylePackageSurface && len(m.Symbols) != 0 {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q PACKAGE_SURFACE surface must not carry symbols", req.Dependency.Name)
	}
	symbols := make([]facts.SymbolID, 0, len(m.Symbols))
	for _, text := range m.Symbols {
		id, err := symbol.Parse(text)
		if err != nil {
			return validatedDependencySurface{}, fmt.Errorf("dependency %q surface symbol %q is invalid: %w", req.Dependency.Name, text, err)
		}
		if id.Format() != text {
			return validatedDependencySurface{}, fmt.Errorf("dependency %q surface symbol %q is not canonical", req.Dependency.Name, text)
		}
		pkg, err := symbolPackagePath(text)
		if err != nil {
			return validatedDependencySurface{}, fmt.Errorf("dependency %q surface symbol %q has no package path: %w", req.Dependency.Name, text, err)
		}
		// Membership in the already-validated package set proves the symbol's
		// embedded path is canonical without canonicalizing it a second time.
		if !packageSet[pkg] {
			return validatedDependencySurface{}, fmt.Errorf("dependency %q surface symbol %q names package %q outside its concrete package set", req.Dependency.Name, text, pkg)
		}
		symbols = append(symbols, facts.SymbolID(id))
	}
	if !sort.SliceIsSorted(m.Symbols, func(i, j int) bool { return m.Symbols[i] < m.Symbols[j] }) {
		return validatedDependencySurface{}, fmt.Errorf("dependency %q surface symbols are not in canonical order", req.Dependency.Name)
	}

	return validatedDependencySurface{
		manifest:  m,
		style:     style,
		authority: authority,
		packages:  packages,
		symbols:   symbols,
	}, nil
}

func validateSurfacePackagePath(dependency, pkg string) error {
	canonical := hostpolicy.CanonicalizePath(pkg)
	if canonical != pkg {
		return fmt.Errorf("dependency %q surface package %q is not canonical: CanonicalizePath rewrites it to %q", dependency, pkg, canonical)
	}
	if !hostpolicy.IsCanonicalPath(pkg) {
		return fmt.Errorf("dependency %q surface package %q is not canonical according to IsCanonicalPath", dependency, pkg)
	}
	return nil
}

func validateSurfaceSDKKey(req DependencySurfaceRequest, got *gen.SDKKey) error {
	if got == nil {
		return fmt.Errorf("dependency %q surface SDK key is missing", req.Dependency.Name)
	}
	key := stdlibauthority.SDKKey{
		ToolchainVersion: got.ToolchainVersion,
		GOOS:             got.Goos,
		GOARCH:           got.Goarch,
		CgoEnabled:       got.CgoEnabled,
		BuildTags:        slices.Clone(got.BuildTags),
		GOEXPERIMENT:     got.Goexperiment,
		ClassifierHash:   got.ClassifierHash,
		MapFormatVersion: got.MapFormatVersion,
	}
	if err := validateExpectedSDKKey(key); err != nil {
		return fmt.Errorf("dependency %q surface SDK key is incomplete or non-canonical: %w", req.Dependency.Name, err)
	}
	expected := expectedSDKKey(req)
	if fields := stdlibauthority.EqualKeys(key, expected); len(fields) > 0 {
		return fmt.Errorf("dependency %q surface SDK key mismatch in fields: %s", req.Dependency.Name, strings.Join(fields, ", "))
	}
	return nil
}

func validateSurfaceAuthority(dependency string, a *gen.AuthorityDeclaration) (manifest.AuthorityDeclaration, error) {
	if a == nil {
		return manifest.AuthorityDeclaration{}, fmt.Errorf("dependency %q surface authority is missing", dependency)
	}
	declared, err := manifest.FromPersisted(a.Authority, a.DeclaredAuthority)
	if err != nil {
		return manifest.AuthorityDeclaration{}, fmt.Errorf("dependency %q surface authority is invalid: %w", dependency, err)
	}
	if declared.Known {
		if !sort.StringsAreSorted(a.DeclaredAuthority) {
			return manifest.AuthorityDeclaration{}, fmt.Errorf("dependency %q surface declared authority is not in canonical order", dependency)
		}
	}
	return declared, nil
}

func surfaceStyle(style gen.InterfaceStyle) (manifest.InterfaceStyle, error) {
	switch style {
	case gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED:
		return manifest.InterfaceStyleUnspecified, nil
	case gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE:
		return manifest.InterfaceStylePackageSurface, nil
	default:
		return 0, fmt.Errorf("unknown interface_style %d", style)
	}
}

func symbolPackagePath(text string) (string, error) {
	if strings.HasPrefix(text, "(") {
		end := strings.Index(text, ").")
		if end < 0 {
			return "", errors.New("method symbol has no receiver terminator")
		}
		receiver := text[1:end]
		dot := strings.LastIndexByte(receiver, '.')
		if dot <= 0 {
			return "", errors.New("method receiver has no package path")
		}
		return receiver[:dot], nil
	}
	dot := strings.LastIndexByte(text, '.')
	if dot <= 0 {
		return "", errors.New("symbol has no package separator")
	}
	return text[:dot], nil
}

func readDependencyReport(req DependencySurfaceRequest, path string, readFile DependencySurfaceReadFile, required bool) (string, bool, error) {
	if path == "" {
		if required {
			return "", false, fmt.Errorf("dependency %q checked surface has no report path", req.Dependency.Name)
		}
		return "", false, nil
	}
	data, err := readFile(path)
	if err != nil {
		if !required && isNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("dependency %q report %q: %w", req.Dependency.Name, path, err)
	}
	persisted, err := artifactio.DecodeReport(data)
	if err != nil {
		return "", false, fmt.Errorf("dependency %q report %q is invalid: %w", req.Dependency.Name, path, err)
	}
	if persisted.Report.Component != req.Dependency.Name {
		return "", false, fmt.Errorf("dependency %q report component mismatch: got %q", req.Dependency.Name, persisted.Report.Component)
	}
	return string(persisted.Verdict), true, nil
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err)
}

func deriveStatuses(req DependencySurfaceRequest, decoded validatedDependencySurface, verdict string, reportPresent bool) (facts.DependencyProvenance, facts.DependencyFreshness, error) {
	if req.Mode == DependencySurfaceLayout {
		binding := req.Binding
		switch binding.Provenance {
		case packagelayout.DependencyArtifactProvenanceChecked:
			if !reportPresent {
				return "", "", fmt.Errorf("dependency %q checked binding did not produce a report", req.Dependency.Name)
			}
			switch verdict {
			case "pass":
				if !decoded.authority.Known {
					return "", "", fmt.Errorf("dependency %q checked surface cannot carry UNKNOWN authority", req.Dependency.Name)
				}
				if err := requireContentDigest(req.Dependency.Name, decoded.manifest.Digest); err != nil {
					return "", "", err
				}
				return facts.DependencyProvenanceCheckedPass, facts.DependencyFreshnessBuildGraph, nil
			case "fail":
				if !decoded.authority.Known {
					return "", "", fmt.Errorf("dependency %q checked surface cannot carry UNKNOWN authority", req.Dependency.Name)
				}
				if err := requireContentDigest(req.Dependency.Name, decoded.manifest.Digest); err != nil {
					return "", "", err
				}
				return facts.DependencyProvenanceCheckedFail, facts.DependencyFreshnessBuildGraph, nil
			default:
				return "", "", fmt.Errorf("dependency %q report has unknown verdict %q", req.Dependency.Name, verdict)
			}
		case packagelayout.DependencyArtifactProvenanceAsserted:
			if reportPresent {
				return "", "", fmt.Errorf("dependency %q asserted binding unexpectedly has a report", req.Dependency.Name)
			}
			if decoded.style != manifest.InterfaceStylePackageSurface || decoded.authority.Known || len(decoded.manifest.Symbols) != 0 || decoded.manifest.Digest != "" {
				return "", "", fmt.Errorf("dependency %q asserted surface must be PACKAGE_SURFACE, UNKNOWN, symbol-free, and carry an empty digest", req.Dependency.Name)
			}
			return facts.DependencyProvenanceAsserted, facts.DependencyFreshnessBuildGraph, nil
		default:
			return "", "", fmt.Errorf("dependency %q has unknown binding provenance %q", req.Dependency.Name, binding.Provenance)
		}
	}

	provenance := facts.DependencyProvenanceAsserted
	freshness := facts.DependencyFreshnessUnknown
	if decoded.manifest.Digest == "" {
		return provenance, freshness, nil
	}
	if err := requireContentDigest(req.Dependency.Name, decoded.manifest.Digest); err != nil {
		return "", "", err
	}
	readFile := req.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	manifestPath := filepath.Clean(filepath.Join(req.DeclaringRoot, req.Dependency.Manifest))
	manifestBytes, err := readFile(manifestPath)
	if err != nil {
		return provenance, freshness, nil
	}
	readSources := req.ReadSourceFiles
	if readSources == nil {
		readDir := req.ReadDir
		if readDir == nil {
			readDir = os.ReadDir
		}
		readSources = func(root string, packages []string) ([]surface.SourceFile, error) {
			return readNativeDependencySources(root, packages, readFile, readDir)
		}
	}
	sources, err := readSources(filepath.Dir(manifestPath), decoded.packages)
	if err != nil || len(sources) == 0 {
		return provenance, freshness, nil
	}
	actual, err := surface.DigestForSources(sources, manifestBytes, decoded.manifest)
	if err != nil {
		return provenance, freshness, nil
	}
	if actual == decoded.manifest.Digest {
		freshness = facts.DependencyFreshnessVerified
	} else {
		freshness = facts.DependencyFreshnessStale
	}
	return provenance, freshness, nil
}

func requireContentDigest(dependency, digest string) error {
	if digest == "" {
		return fmt.Errorf("dependency %q surface digest is empty for a derived surface", dependency)
	}
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return fmt.Errorf("dependency %q surface digest is not a lowercase SHA-256 value", dependency)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return fmt.Errorf("dependency %q surface digest is not valid hexadecimal: %w", dependency, err)
	}
	return nil
}

// readNativeDependencySources enumerates only the concrete packages named by
// a surface. It reads go.mod text for module-to-directory mapping and file
// bytes for hashing, but it never parses Go syntax or invokes packages.Load.
func readNativeDependencySources(root string, packages []string, readFile DependencySurfaceReadFile, readDir DependencySurfaceReadDir) ([]surface.SourceFile, error) {
	moduleRoot, modulePath, err := findModuleRoot(root, readFile)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, pkg := range packages {
		var dir string
		switch {
		case pkg == modulePath:
			dir = moduleRoot
		case strings.HasPrefix(pkg, modulePath+"/"):
			dir = filepath.Join(moduleRoot, filepath.FromSlash(strings.TrimPrefix(pkg, modulePath+"/")))
		default:
			return nil, fmt.Errorf("package %q is outside module %q", pkg, modulePath)
		}
		entries, err := readDir(dir)
		if err != nil {
			return nil, err
		}
		var packageFiles []string
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			// The producer's p.GoFiles set is selected by the target build
			// context. A byte-only directory reader cannot prove that a
			// filename-constrained file is active without reimplementing the
			// Go build selector, so conservatively make freshness UNKNOWN.
			if hasBuildConstraintFilename(entry.Name()) {
				return nil, fmt.Errorf("source set contains build-constrained file %q", entry.Name())
			}
			packageFiles = append(packageFiles, filepath.Join(dir, entry.Name()))
		}
		if len(packageFiles) == 0 {
			return nil, fmt.Errorf("package %q has no readable member Go files", pkg)
		}
		slices.Sort(packageFiles)
		paths = append(paths, packageFiles...)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	out := make([]surface.SourceFile, 0, len(paths))
	for _, path := range paths {
		bytes, err := readFile(path)
		if err != nil {
			return nil, err
		}
		if hasBuildConstraintDirective(bytes) {
			return nil, fmt.Errorf("source set contains a Go build constraint in %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return nil, fmt.Errorf("member source %q is not under dependency root %q", path, root)
		}
		out = append(out, surface.SourceFile{Path: filepath.ToSlash(rel), Bytes: bytes})
	}
	slices.SortFunc(out, func(a, b surface.SourceFile) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

// nativeBuildConstraintNames is deliberately conservative. If a filename
// carries one of these standard GOOS/GOARCH suffixes, the byte-only reader
// cannot establish whether it belongs to the producer's target without
// duplicating the Go build selector. Custom tags and directives are handled by
// hasBuildConstraintDirective below.
var nativeBuildConstraintNames = map[string]bool{
	"386": true, "aix": true, "amd64": true, "android": true, "arm": true,
	"arm64": true, "cgo": true, "darwin": true, "dragonfly": true,
	"freebsd": true, "hurd": true, "illumos": true, "ios": true, "js": true,
	"linux": true, "loong64": true, "mips": true, "mips64": true,
	"mips64le": true, "mipsle": true, "netbsd": true, "openbsd": true,
	"plan9": true, "ppc64": true, "ppc64le": true, "riscv64": true,
	"s390x": true, "solaris": true, "unix": true, "wasip1": true,
	"wasm": true, "windows": true,
}

func hasBuildConstraintFilename(name string) bool {
	stem := strings.TrimSuffix(name, ".go")
	parts := strings.Split(stem, "_")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts[1:] {
		if nativeBuildConstraintNames[part] {
			return true
		}
	}
	return false
}

func hasBuildConstraintDirective(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//go:build") || strings.HasPrefix(trimmed, "// +build") {
			return true
		}
	}
	return false
}

func findModuleRoot(root string, readFile DependencySurfaceReadFile) (string, string, error) {
	current := filepath.Clean(root)
	for {
		data, err := readFile(filepath.Join(current, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					module := strings.TrimSpace(strings.TrimPrefix(line, "module "))
					if module != "" && !strings.ContainsAny(module, " \t\r") {
						return current, module, nil
					}
					return "", "", fmt.Errorf("invalid module path in %q", filepath.Join(current, "go.mod"))
				}
			}
			return "", "", fmt.Errorf("go.mod %q has no module directive", filepath.Join(current, "go.mod"))
		}
		if !isNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", errors.New("no go.mod found for dependency source root")
		}
		current = parent
	}
}
