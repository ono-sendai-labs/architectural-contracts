package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// SDKKeyRequest names the inputs the check's SDK-key resolver may consult
// (plan Step 5 task 3, technical requirement 3). LayoutPlatform is the active
// package layout's pinned platform declaration (nil when the layout declares
// no platform or the check runs in native mode). StdlibMapPath is the
// declared Step 4 stdlib-map artifact path (--stdlib-map); the map is the
// declared, configuration-keyed source of the target SDK identity in Bazel
// and layout mode.
type SDKKeyRequest struct {
	// LayoutPlatform is the active layout's pinned target declaration as
	// plain values (nil/unset when the layout is unpinned or the check runs
	// in native mode); see goanalysis.ActivePlatformIdentity.
	LayoutPlatform *goanalysis.PlatformIdentity
	// InLayoutMode reports whether a package layout driver environment is
	// active; native mode is its negation.
	InLayoutMode  bool
	StdlibMapPath string
}

// SDKKeyResolver resolves the target SDK identity used for surface emission.
// The map contributes only the surface's SDK key in this step: its symbol
// classifications are not consulted by checker decisions until Step 6.
type SDKKeyResolver func(SDKKeyRequest) (stdlibauthority.SDKKey, error)

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
	if hasMap && !hasSurface {
		return opts, fmt.Errorf("--stdlib-map requires surface emission (--surface-out)")
	}
	if hasSurface && hasLayout && !hasMap {
		return opts, fmt.Errorf("surface emission from a package layout requires the declared stdlib-map artifact (--stdlib-map)")
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

	// 3. Load facts for that root.
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

	// 4. Validate every declared interface file.
	interfaceExclusions, err := goanalysis.ValidateInterfaceFiles(componentRoot, parsedManifest.InterfaceFiles, loadedFacts)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to validate interface files: %v\n", err)
		return 2
	}

	// 5. Resolve direct component dependencies.
	var resolvedDeps []facts.DependencyInterface
	for _, dep := range parsedManifest.ComponentDependencies {
		depIface, err := goanalysis.ResolveDependencyInterface(componentRoot, componentRoot, dep)
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to resolve dependency %q: %v\n", dep.Name, err)
			return 2
		}
		resolvedDeps = append(resolvedDeps, depIface)
	}

	// 6. Build AnalyzeRequest.PruneAt from the resolved dependencies.
	pruneSet := make(map[string]bool)
	for _, di := range resolvedDeps {
		for _, sym := range di.Symbols {
			pruneSet[string(sym)] = true
		}
		for _, pkg := range di.Packages {
			pruneSet["func "+pkg+".init"] = true
		}
	}
	var pruneAt []capanalyzer.InterfaceSymbol
	for k := range pruneSet {
		pruneAt = append(pruneAt, capanalyzer.InterfaceSymbol(k))
	}
	sort.Slice(pruneAt, func(i, j int) bool { return pruneAt[i] < pruneAt[j] })

	// 7. Analyze the member packages' capability findings.
	var pkgs []string
	for _, p := range loadedFacts.Packages {
		pkgs = append(pkgs, p.ImportPath)
	}
	var findings []capanalyzer.CapabilityFinding
	if len(pkgs) > 0 {
		findings, err = r.Analyzer.Analyze(capanalyzer.AnalyzeRequest{
			Packages: pkgs,
			PruneAt:  pruneAt,
		})
		if err != nil {
			fmt.Fprintf(stderr, "error: capability analysis failed: %v\n", err)
			return 2
		}
	}

	// 8. Check the merged policy.
	inputs := checker.Inputs{
		Manifest:  parsedManifest,
		Facts:     loadedFacts,
		DepIfaces: resolvedDeps,
		Caps:      findings,
		Policy:    capanalyzer.StrictPolicy(),
	}
	conformanceReport := checker.Check(inputs)
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
	sort.SliceStable(conformanceReport.Warnings, func(i, j int) bool {
		if conformanceReport.Warnings[i].Message != conformanceReport.Warnings[j].Message {
			return conformanceReport.Warnings[i].Message < conformanceReport.Warnings[j].Message
		}
		return conformanceReport.Warnings[i].Kind < conformanceReport.Warnings[j].Kind
	})

	// 9. Publish the canonical artifacts from this same analysis invocation.
	// Both artifacts are derived and encoded before either is written, so a
	// failed key resolution or derivation publishes nothing (AC 1, 4, 5).
	if opts.reportOut != "" || opts.surfaceOut != "" {
		if err := r.publishArtifacts(opts, &parsedManifest, manifestBytes, componentRoot, loadedFacts, interfaceExclusions, conformanceReport); err != nil {
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

// publishArtifacts resolves the target SDK identity, derives the exact
// surface from this invocation's analysis inputs, encodes both artifacts
// canonically, and writes them atomically. The map contributes only the
// surface's SDK key in this step; checker decisions still run the old check
// path (with the implements-closure workaround) until Step 6.
func (r *Runner) publishArtifacts(
	opts checkOptions,
	parsedManifest *manifest.Manifest,
	manifestBytes []byte,
	componentRoot string,
	loadedFacts facts.PackageFacts,
	interfaceExclusions []goanalysis.InterfaceFileExclusion,
	conformanceReport report.ConformanceReport,
) error {
	var sdkKey *stdlibauthority.SDKKey

	surfaceBytes := []byte(nil)
	if opts.surfaceOut != "" {
		layoutPlatform := (*goanalysis.PlatformIdentity)(nil)
		if identity, ok := goanalysis.ActivePlatformIdentity(); ok {
			layoutPlatform = &identity
		}
		key, err := r.keyResolver()(SDKKeyRequest{
			LayoutPlatform: layoutPlatform,
			InLayoutMode:   goanalysis.LayoutModeActive(),
			StdlibMapPath:  opts.stdlibMap,
		})
		if err != nil {
			return fmt.Errorf("resolving the target SDK key: %v", err)
		}
		sdkKey = &key

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
		sources, err := artifactio.ReadSources(os.DirFS(sourceRoot), surf.SourcePaths)
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

func (r *Runner) keyResolver() SDKKeyResolver {
	if r.KeyResolver != nil {
		return r.KeyResolver
	}
	return defaultKeyResolver
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

// defaultKeyResolver is the production SDK-key resolver (technical
// requirement 3). In layout/Bazel mode it validates and uses the declared
// Step 4 stdlib-map artifact, checking its key against the layout's pinned
// target and this arcc's classifier rules. In native mode it uses the Step 4
// target discovery and cache services.
func defaultKeyResolver(req SDKKeyRequest) (stdlibauthority.SDKKey, error) {
	if req.StdlibMapPath != "" {
		return keyFromDeclaredMap(req)
	}
	return keyFromNativeDiscovery()
}

// keyFromDeclaredMap opens and validates the declared stdlib-map artifact and
// returns its SDK key. A missing, corrupt, format-mismatched,
// classifier-mismatched, or target-mismatched map fails closed with actionable
// context (AC 4).
func keyFromDeclaredMap(req SDKKeyRequest) (stdlibauthority.SDKKey, error) {
	f, err := os.Open(req.StdlibMapPath)
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("opening the declared stdlib-map artifact %s: %v", req.StdlibMapPath, err)
	}
	defer f.Close()
	m, err := artifactio.DecodeMap(f)
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("decoding the declared stdlib-map artifact %s: %v", req.StdlibMapPath, err)
	}
	reader, err := artifactio.NewStdlibMapReader(m, nil)
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("validating the declared stdlib-map artifact %s: %v", req.StdlibMapPath, err)
	}
	key := reader.Key()

	// Classifier mismatch: the map was generated under different classifier
	// rules than this arcc applies (I2). The expected hash is derived from
	// the map's own target configuration and the current rules.
	expected, err := deriveSDKKey(stdlibmap.TargetConfig{
		ToolchainVersion: key.ToolchainVersion,
		GOOS:             key.GOOS,
		GOARCH:           key.GOARCH,
		CgoEnabled:       key.CgoEnabled,
		BuildTags:        key.BuildTags,
		GOEXPERIMENT:     key.GOEXPERIMENT,
	})
	if err != nil {
		return stdlibauthority.SDKKey{}, err
	}
	if expected.ClassifierHash != key.ClassifierHash {
		return stdlibauthority.SDKKey{}, fmt.Errorf("stdlib map %s was generated with classifier_hash %q but this arcc computes %q; regenerate the map", req.StdlibMapPath, key.ClassifierHash, expected.ClassifierHash)
	}

	// Target mismatch (N3): compare the map's key against the target this
	// invocation actually analyses. In layout mode the declared target is the
	// layout's pinned platform; an unpinned layout carries no declared
	// target, so only the map's own key is available. In native mode the
	// target is discovered from the current toolchain, so an explicit map
	// made for another target is rejected instead of being stamped into the
	// surface.
	if req.LayoutPlatform != nil {
		expectedKey, err := deriveSDKKey(layoutTarget(req.LayoutPlatform))
		if err != nil {
			return stdlibauthority.SDKKey{}, err
		}
		if fields := stdlibauthority.EqualKeys(key, expectedKey); len(fields) > 0 {
			return stdlibauthority.SDKKey{}, fmt.Errorf("stdlib map %s key does not match the declared target: mismatched %s", req.StdlibMapPath, strings.Join(fields, ", "))
		}
	} else if !req.InLayoutMode {
		target, err := stdlibmap.NativeTargetConfig()
		if err != nil {
			return stdlibauthority.SDKKey{}, err
		}
		expectedKey, err := deriveSDKKey(target)
		if err != nil {
			return stdlibauthority.SDKKey{}, err
		}
		if fields := stdlibauthority.EqualKeys(key, expectedKey); len(fields) > 0 {
			return stdlibauthority.SDKKey{}, fmt.Errorf("stdlib map %s key does not match the discovered native target: mismatched %s", req.StdlibMapPath, strings.Join(fields, ", "))
		}
	}
	return key, nil
}

// keyFromNativeDiscovery resolves the target SDK identity in native mode via
// the Step 4 target discovery and cache services: discover the target
// configuration, derive the expected key, and open (generating on demand) the
// cached map validated against it.
func keyFromNativeDiscovery() (stdlibauthority.SDKKey, error) {
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		return stdlibauthority.SDKKey{}, err
	}
	expected, err := deriveSDKKey(target)
	if err != nil {
		return stdlibauthority.SDKKey{}, err
	}
	auth, err := stdlibmap.OpenCachedMap(stdlibmap.CachedMapInput{
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
	if err != nil {
		return stdlibauthority.SDKKey{}, err
	}
	return auth.Key(), nil
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
