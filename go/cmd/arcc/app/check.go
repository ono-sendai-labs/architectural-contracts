package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

// AuthorityRequest names the inputs the check's stdlib-authority resolver may
// consult. LayoutPlatform is the active package layout's pinned platform
// declaration (nil when the layout declares no platform or the check runs in
// native mode). StdlibMapPath is the declared Step 4 stdlib-map artifact path
// (--stdlib-map), mandatory in layout/Bazel mode: the map is the declared,
// configuration-keyed source of every standard-library decision (N3), and its
// full target SDK key is validated before any verdict forms.
type AuthorityRequest struct {
	// LayoutPlatform is the active layout's pinned target declaration as
	// plain values (nil/unset when the layout is unpinned or the check runs
	// in native mode); see goanalysis.ActivePlatformIdentity.
	LayoutPlatform *goanalysis.PlatformIdentity
	// InLayoutMode reports whether a package layout driver environment is
	// active; native mode is its negation.
	InLayoutMode  bool
	StdlibMapPath string
}

// AuthorityResolver resolves the check's stdlib authority: the validated
// StdlibAuthority port every stdlib membership and classification decision
// reads. It fails closed before any verdict or artifact when the map is
// missing, corrupt, format- or classifier-mismatched, key-mismatched, or
// inventory-incomplete (AC 2). The production implementation opens the
// declared map in layout/Bazel mode and the Step 4 native on-demand
// cache/generator path in native mode.
type AuthorityResolver func(AuthorityRequest) (stdlibauthority.StdlibAuthority, error)

// SurfaceInputsLoader loads the component's member packages for surface
// derivation. The production implementation is goanalysis.LoadSurfaceInputs.
type SurfaceInputsLoader func(goanalysis.LoadRequest) (goanalysis.SurfaceInputs, error)

// ArtifactWriter replaces path with data atomically. The production
// implementation is artifactio.WriteFileAtomic, so an interrupted or failed
// write never exposes a partial file and leaves any previous target intact
// (technical requirement 6).
type ArtifactWriter func(path string, data []byte) error

// checkOptions is the parsed check command: display, persisted output, and
// exit policy are separate decisions (task approach step 1).
type checkOptions struct {
	manifestPath  string
	packageLayout string
	// workspaceDir is the layout mode's workspace root (set by the caller
	// that established the layout driver environment); source paths resolve
	// against it.
	workspaceDir string
	formatJSON   bool
	reportOut    string
	surfaceOut   string
	stdlibMap    string
	verdictOnly  bool
}

// parseCheckOptions parses the check command's options with precise
// single-occurrence, empty-value, and missing-value diagnostics that name the
// offending option (technical requirement 1, AC 6). Incompatible combinations
// (verdict-only without a report destination, stdlib-map without surface
// emission, surface emission from a layout without a declared map) are usage
// errors, so analysis never runs (AC 6).
func parseCheckOptions(args []string) (checkOptions, error) {
	opts := checkOptions{manifestPath: args[0]}
	hasLayout, hasFormat, hasReport, hasSurface, hasMap, hasVerdictOnly := false, false, false, false, false, false

	flagValue := func(arg, name, prefix string) (string, error) {
		val := strings.TrimPrefix(arg, prefix)
		if val == "" {
			return "", fmt.Errorf("empty %s value", name)
		}
		return val, nil
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--format=json":
			if hasFormat {
				return opts, fmt.Errorf("duplicate option: --format=json")
			}
			hasFormat = true
		case strings.HasPrefix(arg, "--package-layout="):
			if hasLayout {
				return opts, fmt.Errorf("duplicate option: --package-layout")
			}
			val, err := flagValue(arg, "--package-layout", "--package-layout=")
			if err != nil {
				return opts, err
			}
			opts.packageLayout = val
			hasLayout = true
		case strings.HasPrefix(arg, "--package-layout"):
			return opts, fmt.Errorf("missing --package-layout value")
		case strings.HasPrefix(arg, "--report-out="):
			if hasReport {
				return opts, fmt.Errorf("duplicate option: --report-out")
			}
			val, err := flagValue(arg, "--report-out", "--report-out=")
			if err != nil {
				return opts, err
			}
			opts.reportOut = val
			hasReport = true
		case strings.HasPrefix(arg, "--report-out"):
			return opts, fmt.Errorf("missing --report-out value")
		case strings.HasPrefix(arg, "--surface-out="):
			if hasSurface {
				return opts, fmt.Errorf("duplicate option: --surface-out")
			}
			val, err := flagValue(arg, "--surface-out", "--surface-out=")
			if err != nil {
				return opts, err
			}
			opts.surfaceOut = val
			hasSurface = true
		case strings.HasPrefix(arg, "--surface-out"):
			return opts, fmt.Errorf("missing --surface-out value")
		case strings.HasPrefix(arg, "--stdlib-map="):
			if hasMap {
				return opts, fmt.Errorf("duplicate option: --stdlib-map")
			}
			val, err := flagValue(arg, "--stdlib-map", "--stdlib-map=")
			if err != nil {
				return opts, err
			}
			opts.stdlibMap = val
			hasMap = true
		case strings.HasPrefix(arg, "--stdlib-map"):
			return opts, fmt.Errorf("missing --stdlib-map value")
		case arg == "--report-verdict-only":
			if hasVerdictOnly {
				return opts, fmt.Errorf("duplicate option: --report-verdict-only")
			}
			hasVerdictOnly = true
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unknown option: %s", arg)
		default:
			return opts, fmt.Errorf("check command requires exactly one argument")
		}
	}

	opts.formatJSON = hasFormat
	opts.verdictOnly = hasVerdictOnly
	if hasVerdictOnly && !hasReport {
		return opts, fmt.Errorf("--report-verdict-only requires a report artifact destination (--report-out)")
	}
	if hasLayout && !hasMap {
		return opts, fmt.Errorf("a layout-mode check requires the declared stdlib-map artifact (--stdlib-map)")
	}
	return opts, nil
}

// runCheck executes the check command for already-parsed options. The layout
// driver environment (when --package-layout was given) is established by the
// caller.
func (r *Runner) runCheck(opts checkOptions, stdout, stderr io.Writer) int {
	// 1. Open and parse the manifest, retaining its bytes as a surface
	// digest input.
	manifestBytes, err := os.ReadFile(opts.manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to open manifest file: %v\n", err)
		return 2
	}
	parsedManifest, err := manifest.Parse(bytes.NewReader(manifestBytes))
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to parse manifest: %v\n", err)
		return 2
	}

	// 2. Derive the component root from the cleaned manifest path's directory.
	componentRoot, err := filepath.Abs(filepath.Dir(filepath.Clean(opts.manifestPath)))
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve absolute path of component root: %v\n", err)
		return 2
	}

	// 3. Resolve the stdlib authority before loading or resolving any direct
	// dependency. The target SDK key is part of every surface comparison, so a
	// missing or mismatched map must fail before dependency artifacts can affect
	// the invocation (N3, DR-03).
	layoutPlatform := (*goanalysis.PlatformIdentity)(nil)
	if identity, ok := goanalysis.ActivePlatformIdentity(); ok {
		layoutPlatform = &identity
	}
	authority, err := r.authorityResolver()(AuthorityRequest{
		LayoutPlatform: layoutPlatform,
		InLayoutMode:   goanalysis.LayoutModeActive(),
		StdlibMapPath:  opts.stdlibMap,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	// 4. Load facts for that root.
	loadedFacts, err := r.loader()(goanalysis.LoadRequest{
		ComponentName:  parsedManifest.Name,
		ComponentRoot:  componentRoot,
		Members:        parsedManifest.Members,
		InterfaceFiles: parsedManifest.InterfaceFiles,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to load package facts: %v\n", err)
		return 2
	}

	// 5. Validate every declared interface file.
	interfaceExclusions, err := goanalysis.ValidateInterfaceFiles(componentRoot, parsedManifest.InterfaceFiles, loadedFacts)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to validate interface files: %v\n", err)
		return 2
	}

	// 6. Resolve direct component dependencies from their persisted surfaces
	// and reports. Native mode uses the sibling convention; layout mode uses
	// the validated provider bindings in the active package layout. No
	// dependency source is passed to a package loader on this path (R7, N1).
	resolvedDeps, err := resolveDependencySurfaces(parsedManifest.ComponentDependencies, componentRoot, authority.Key())
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve dependency surface: %v\n", err)
		return 2
	}

	// 7. Check the component against the typed reference/import facts.
	inputs := checker.Inputs{
		Manifest:  parsedManifest,
		Facts:     loadedFacts,
		DepIfaces: resolvedDeps,
		Authority: authority,
		SDKKey:    authority.Key(),
		Policy:    capanalyzer.StrictPolicy(),
	}
	conformanceReport, err := checker.Check(inputs)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	for _, exclusion := range interfaceExclusions {
		conformanceReport.Warnings = append(conformanceReport.Warnings, report.Finding{
			Kind:    report.InterfaceFileExcluded,
			Message: fmt.Sprintf("interface file %q excluded by %s", exclusion.File, exclusion.Constraint),
			Location: report.Location{
				File: exclusion.File,
				Line: 1,
			},
		})
	}
	// sort.SliceStable is UNANALYZED in the stdlib authority map; the
	// slices spelling is proved-pure, so the CLI's own check stays clean.
	slices.SortStableFunc(conformanceReport.Warnings, func(a, b report.Finding) int {
		if c := strings.Compare(a.Message, b.Message); c != 0 {
			return c
		}
		return strings.Compare(string(a.Kind), string(b.Kind))
	})

	// 9. Publish the canonical artifacts from this same analysis invocation.
	// Both artifacts are derived and encoded before either is written, so a
	// failed key resolution or derivation publishes nothing (AC 1, 4, 5).
	if opts.reportOut != "" || opts.surfaceOut != "" {
		var surfaceKey *stdlibauthority.SDKKey
		if opts.surfaceOut != "" {
			key := authority.Key()
			surfaceKey = &key
		}
		if err := r.publishArtifacts(opts, &parsedManifest, manifestBytes, componentRoot, loadedFacts, interfaceExclusions, surfaceKey, conformanceReport); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}

	// 10. Display the human/JSON report; the artifact bytes are canonical and
	// independent of the display format (technical requirement 5).
	if opts.formatJSON {
		marshaled, err := json.MarshalIndent(conformanceReport, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to marshal report to JSON: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "%s\n", marshaled)
	} else {
		fmt.Fprint(stdout, report.RenderText(conformanceReport))
	}

	// 11. Exit policy: with --report-verdict-only both pass and fail are
	// successful executions (the verdict lives in the report), while every
	// usage, load, analysis, key, or write error above exited 2 (technical
	// requirement 4). Without it the existing 0/1/2 contract is preserved.
	if opts.verdictOnly {
		return 0
	}
	if len(conformanceReport.Violations) > 0 {
		return 1
	}
	return 0
}

// resolveDependencySurfaces resolves every direct (declared or auto-attached)
// dependency through the persisted surface consumer. The resolver receives
// the complete target key already validated by the authority resolver, so no
// dependency can be compared under a different SDK configuration.
func resolveDependencySurfaces(deps []manifest.ComponentDependency, declaringRoot string, expectedSDKKey stdlibauthority.SDKKey) ([]facts.DependencyInterface, error) {
	mode := goanalysis.DependencySurfaceNative
	workspaceDir := ""
	bindingByName := map[string]goanalysis.DependencyArtifactBinding{}
	if bindings, activeWorkspace, active := goanalysis.ActiveDependencyArtifactBindings(); active {
		mode = goanalysis.DependencySurfaceLayout
		workspaceDir = activeWorkspace
		if bindings == nil && workspaceDir == "" {
			return nil, fmt.Errorf("layout mode is active without an active package layout")
		}
		for _, binding := range bindings {
			bindingByName[binding.Dependency] = binding
		}
	}

	resolved := make([]facts.DependencyInterface, 0, len(deps))
	for _, dep := range deps {
		var binding *goanalysis.DependencyArtifactBinding
		if mode == goanalysis.DependencySurfaceLayout {
			value, ok := bindingByName[dep.Name]
			if !ok {
				return nil, fmt.Errorf("dependency %q has no surface/report binding in the active package layout", dep.Name)
			}
			binding = &value
		}
		resolvedDependency, err := goanalysis.ResolveDependencySurface(goanalysis.DependencySurfaceRequest{
			DeclaringRoot:  declaringRoot,
			Dependency:     dep,
			Binding:        binding,
			Namespace:      goanalysis.CanonicalNamespace(),
			ExpectedSDKKey: expectedSDKKey,
			Mode:           mode,
			WorkspaceDir:   workspaceDir,
		})
		if err != nil {
			return nil, fmt.Errorf("dependency %q: %w", dep.Name, err)
		}
		resolved = append(resolved, resolvedDependency)
	}
	return resolved, nil
}

// publishArtifacts derives the exact surface from this invocation's analysis
// inputs under the already-validated target SDK key, encodes both artifacts
// canonically, and writes them atomically. Check decisions and emitted
// surfaces share the same exact declaring-object interface: the surface's
// Symbols are the same set the check classifies against (no
// implements-closure injection on either side).
func (r *Runner) publishArtifacts(
	opts checkOptions,
	parsedManifest *manifest.Manifest,
	manifestBytes []byte,
	componentRoot string,
	loadedFacts facts.PackageFacts,
	interfaceExclusions []goanalysis.InterfaceFileExclusion,
	sdkKey *stdlibauthority.SDKKey,
	conformanceReport report.ConformanceReport,
) error {
	var surfaceBytes []byte
	if opts.surfaceOut != "" {

		// Only the surviving interface files reach surface extraction:
		// build-constraint exclusions are deliberate non-fatal warnings, so
		// the excluded files are simply absent from the exact surface.
		surf, err := r.surfaceInputsLoader()(goanalysis.LoadRequest{
			ComponentName:  parsedManifest.Name,
			ComponentRoot:  componentRoot,
			Members:        parsedManifest.Members,
			InterfaceFiles: survivingInterfaceFiles(parsedManifest.InterfaceFiles, interfaceExclusions),
		})
		if err != nil {
			return fmt.Errorf("loading member packages for surface emission: %v", err)
		}

		sourceRoot := componentRoot
		if opts.workspaceDir != "" {
			sourceRoot = opts.workspaceDir
		}
		sources, err := readSurfaceSources(sourceRoot, surf.SourcePaths)
		if err != nil {
			return fmt.Errorf("reading member sources for surface emission: %v", err)
		}

		derived, err := surface.Derive(surface.Input{
			Component:       parsedManifest.Name,
			Style:           parsedManifest.InterfaceStyle,
			Authority:       parsedManifest.Authority,
			Namespace:       goanalysis.CanonicalNamespace(),
			Key:             sdkKey,
			ProducerVersion: producerVersion(),
			MemberPackages:  surf.MemberPackages,
			Manifest:        manifestBytes,
			InterfaceFiles:  surf.InterfaceFiles,
			InterfaceInfo:   surf.InterfaceInfo,
			Sources:         sources,
		})
		if err != nil {
			return fmt.Errorf("deriving the surface artifact: %v", err)
		}
		encoded, err := artifactio.MarshalSurface(derived)
		if err != nil {
			return fmt.Errorf("encoding the surface artifact: %v", err)
		}
		surfaceBytes = encoded
	}

	if opts.reportOut != "" {
		data, err := artifactio.MarshalReport(conformanceReport)
		if err != nil {
			return fmt.Errorf("encoding the report artifact: %v", err)
		}
		if err := r.artifactWriter()(opts.reportOut, data); err != nil {
			return fmt.Errorf("writing the report artifact %s: %v", opts.reportOut, err)
		}
	}
	if len(surfaceBytes) > 0 || opts.surfaceOut != "" {
		if err := r.artifactWriter()(opts.surfaceOut, surfaceBytes); err != nil {
			return fmt.Errorf("writing the surface artifact %s: %v", opts.surfaceOut, err)
		}
	}
	return nil
}

// readSurfaceSources reads the exact member paths selected by the package
// loader. Native self-hosting components may temporarily retain foreign
// module-cache packages as members; their component-relative paths contain
// parent segments, which are valid OS paths but intentionally rejected by the
// fs.ValidPath contract of artifactio.ReadSources. Keeping this adapter here
// preserves the exact byte set without enumerating or parsing anything else.
func readSurfaceSources(root string, paths []string) ([]surface.SourceFile, error) {
	sorted := slices.Clone(paths)
	slices.Sort(sorted)
	sources := make([]surface.SourceFile, 0, len(sorted))
	for _, path := range sorted {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, fmt.Errorf("read member source %s: %w", path, err)
		}
		sources = append(sources, surface.SourceFile{Path: path, Bytes: data})
	}
	return sources, nil
}

// survivingInterfaceFiles filters the manifest's declared interface files
// down to those ValidateInterfaceFiles did not exclude by build constraints.
func survivingInterfaceFiles(interfaceFiles []string, exclusions []goanalysis.InterfaceFileExclusion) []string {
	excluded := make(map[string]bool, len(exclusions))
	for _, e := range exclusions {
		excluded[e.File] = true
	}
	var surviving []string
	for _, f := range interfaceFiles {
		if !excluded[filepath.ToSlash(filepath.Clean(f))] {
			surviving = append(surviving, f)
		}
	}
	return surviving
}

// producerVersion stamps the emitting tool into the surface (audit
// information only; deterministic for a given arcc build).
func producerVersion() string {
	return "arcc " + version
}

// --- default seams -----------------------------------------------------------

func (r *Runner) loader() PackageLoader {
	if r.Loader != nil {
		return r.Loader
	}
	return goanalysis.LoadPackageFacts
}

func (r *Runner) authorityResolver() AuthorityResolver {
	if r.AuthorityResolver != nil {
		return r.AuthorityResolver
	}
	return defaultAuthorityResolver
}

func (r *Runner) surfaceInputsLoader() SurfaceInputsLoader {
	if r.SurfaceInputsLoader != nil {
		return r.SurfaceInputsLoader
	}
	return goanalysis.LoadSurfaceInputs
}

func (r *Runner) artifactWriter() ArtifactWriter {
	if r.ArtifactWriter != nil {
		return r.ArtifactWriter
	}
	return func(path string, data []byte) error {
		return artifactio.WriteFileAtomic(path, data, 0o644, nil)
	}
}

// defaultAuthorityResolver is the production stdlib-authority resolver. In
// layout/Bazel mode it opens and validates the declared stdlib-map artifact;
// in native mode it uses the Step 4 native target discovery and cache
// services. A missing, corrupt, format-mismatched, classifier-mismatched,
// target-mismatched, or inventory-incomplete map fails closed with actionable
// context (AC 2).
func defaultAuthorityResolver(req AuthorityRequest) (stdlibauthority.StdlibAuthority, error) {
	if req.StdlibMapPath != "" {
		return authorityFromDeclaredMap(req)
	}
	if req.InLayoutMode {
		return nil, fmt.Errorf("a layout-mode check requires the declared stdlib-map artifact (--stdlib-map); no check decision may run without a validated stdlib map")
	}
	return authorityFromNativeDiscovery()
}

// authorityFromDeclaredMap opens the declared stdlib-map artifact and
// validates it against the expected target key. The layout's pinned platform
// (when declared) is the target this invocation actually analyses; an
// explicit map made for another target is rejected instead of deciding the
// check (N3). When no target is declared (an unpinned layout), the map's own
// configuration is still checked against the classifier rules this arcc
// applies (I2).
func authorityFromDeclaredMap(req AuthorityRequest) (stdlibauthority.StdlibAuthority, error) {
	f, err := os.Open(req.StdlibMapPath)
	if err != nil {
		return nil, fmt.Errorf("opening the declared stdlib-map artifact %s: %v", req.StdlibMapPath, err)
	}
	defer f.Close()
	m, err := artifactio.DecodeMap(f)
	if err != nil {
		return nil, fmt.Errorf("decoding the declared stdlib-map artifact %s: %v", req.StdlibMapPath, err)
	}

	var expected *stdlibauthority.SDKKey
	switch {
	case req.LayoutPlatform != nil:
		key, err := deriveSDKKey(layoutTarget(req.LayoutPlatform))
		if err != nil {
			return nil, err
		}
		expected = &key
	case !req.InLayoutMode:
		target, err := stdlibmap.NativeTargetConfig()
		if err != nil {
			return nil, err
		}
		key, err := deriveSDKKey(target)
		if err != nil {
			return nil, err
		}
		expected = &key
	default:
		// Unpinned layout: validate the map against its own declared target
		// configuration under the current classifier rules (I2).
	}

	auth, err := artifactio.NewStdlibMapReader(m, expected)
	if err != nil {
		return nil, fmt.Errorf("validating the declared stdlib-map artifact %s: %v", req.StdlibMapPath, err)
	}
	if expected == nil && req.InLayoutMode {
		// Classifier mismatch (I2): the map was generated under different
		// classifier rules than this arcc applies. Derive the expected key
		// from the map's own target configuration and compare.
		mapKey := auth.Key()
		expected, err := deriveSDKKey(stdlibmap.TargetConfig{
			ToolchainVersion: mapKey.ToolchainVersion,
			GOOS:             mapKey.GOOS,
			GOARCH:           mapKey.GOARCH,
			CgoEnabled:       mapKey.CgoEnabled,
			BuildTags:        mapKey.BuildTags,
			GOEXPERIMENT:     mapKey.GOEXPERIMENT,
		})
		if err != nil {
			return nil, err
		}
		if expected.ClassifierHash != mapKey.ClassifierHash {
			return nil, fmt.Errorf("stdlib map %s was generated with classifier_hash %q but this arcc computes %q; regenerate the map", req.StdlibMapPath, mapKey.ClassifierHash, expected.ClassifierHash)
		}
	}
	return auth, nil
}

// authorityFromNativeDiscovery resolves the check's stdlib authority in
// native mode via the Step 4 target discovery and cache services: discover
// the target configuration, derive the expected key, and open (generating on
// demand) the cached map validated against it.
func authorityFromNativeDiscovery() (stdlibauthority.StdlibAuthority, error) {
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		return nil, err
	}
	expected, err := deriveSDKKey(target)
	if err != nil {
		return nil, err
	}
	return stdlibmap.OpenCachedMap(stdlibmap.CachedMapInput{
		Key: expected,
		Generation: stdlibmap.GenerationInput{
			Target:      target,
			RuleVersion: stdlibmap.RuleVersion,
			Oracle: stdlibmap.PackageOracleFunc(func() ([]stdlibmap.PackageEntry, error) {
				return stdlibmap.NativeStdPackageList(context.Background(), stdlibmap.NativeEnvironment(target))
			}),
			Loader: &stdlibmap.NativeLoader{Env: stdlibmap.TargetEnv(target)},
		},
	})
}

// layoutTarget converts a pinned layout platform identity into the target
// configuration the SDK key is derived from.
func layoutTarget(platform *goanalysis.PlatformIdentity) stdlibmap.TargetConfig {
	return stdlibmap.TargetConfig{
		ToolchainVersion: platform.ToolchainVersion,
		GOOS:             platform.GOOS,
		GOARCH:           platform.GOARCH,
		CgoEnabled:       platform.CgoEnabled,
		BuildTags:        platform.BuildTags,
		GOEXPERIMENT:     platform.GOEXPERIMENT,
	}
}

// deriveSDKKey derives the complete target SDK key (classifier hash and map
// format version included) from a target configuration under this arcc's
// current classifier rules.
func deriveSDKKey(target stdlibmap.TargetConfig) (stdlibauthority.SDKKey, error) {
	rules, err := stdlibmap.GenerationClassifierRules()
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("assembling the classifier rules: %v", err)
	}
	text, err := stdlibmap.CanonicalClassifierText(rules)
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("canonicalizing the classifier rules: %v", err)
	}
	return stdlibmap.DeriveSDKKey(stdlibmap.GenerationDescriptor{
		Target:           target,
		ClassifierText:   text,
		RuleVersion:      stdlibmap.RuleVersion,
		MapFormatVersion: artifactio.MapFormatVersion,
	}), nil
}
