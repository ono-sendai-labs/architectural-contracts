package stdlibmap

import (
	"encoding/json"
	"fmt"
	"go/types"
	"os"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"golang.org/x/tools/go/packages"
)

// loadMode is the package-load mode every generation load uses (the same mode
// the native loader type-loads with: full syntax and types over the batch).
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
	packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes

// LayoutLoader implements the batch Loader seam for explicit-input generation
// (task req 5): every Load is served through the packagelayout
// GOPACKAGESDRIVER self-exec driver — arcc's own binary re-executed with the
// layout — so no toolchain binary is executed and go/packages can never fall
// back to the go list driver. The layout must have been computed for the
// pinned target configuration (packagelayout.StdlibLayout).
type LayoutLoader struct {
	// Layout is the validated whole-stdlib layout the driver serves.
	Layout *packagelayout.Layout
}

// Load implements the batch Loader seam: every path is type-loaded from the
// layout's sources through the driver; any load or type error, or any path
// missing from the result, fails the batch with an actionable error.
func (l *LayoutLoader) Load(paths []string) (map[string]*types.Package, error) {
	if l == nil || l.Layout == nil {
		return nil, fmt.Errorf("loading through the layout driver: the layout is required")
	}
	var out map[string]*types.Package
	err := withLayoutDriverEnv(l.Layout, func() error {
		var err error
		out, err = loadUnderDriverEnv(paths)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// withLayoutDriverEnv materialises the layout to a deterministic temporary
// file and runs fn inside packagelayout.WithDriverEnv: GOPACKAGESDRIVER points
// at this process's own executable, so every packages.Load inside fn (with an
// inherited environment) is served by the self-exec driver.
func withLayoutDriverEnv(layout *packagelayout.Layout, fn func() error) error {
	// Already inside a driver environment (GenerateViaLayout wraps the whole
	// generation; a nested Load must not re-enter WithDriverEnv, whose
	// environment mutex is not reentrant): the ambient environment already
	// carries GOPACKAGESDRIVER and the ARCC_DRIVER_MODE marker.
	if os.Getenv("ARCC_DRIVER_MODE") == "1" {
		return fn()
	}
	data, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("materialising the layout for the driver: %w", err)
	}
	f, err := os.CreateTemp("", "arcc-stdlib-layout-*.json")
	if err != nil {
		return fmt.Errorf("materialising the layout for the driver: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("materialising the layout for the driver: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("materialising the layout for the driver: %w", err)
	}
	if err := packagelayout.WithDriverEnv(path, "", fn); err != nil {
		return fmt.Errorf("loading through the layout driver: %w", err)
	}
	return nil
}

// loadUnderDriverEnv type-loads the paths with go/packages in the ambient
// environment — withLayoutDriverEnv has made it the driver environment, so
// this never consults the go list driver.
func loadUnderDriverEnv(paths []string) (map[string]*types.Package, error) {
	if len(paths) == 0 {
		return map[string]*types.Package{}, nil
	}
	cfg := &packages.Config{
		Mode:  loadMode,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, fmt.Errorf("loading %d SDK packages through the driver: %w", len(paths), err)
	}
	out := make(map[string]*types.Package, len(pkgs))
	var firstErr error
	for _, p := range pkgs {
		if len(p.Errors) > 0 && firstErr == nil {
			firstErr = fmt.Errorf("SDK package %q: %v", p.PkgPath, p.Errors[0])
		}
		out[p.PkgPath] = p.Types
	}
	if firstErr != nil {
		return nil, firstErr
	}
	for _, path := range paths {
		if out[path] == nil {
			return nil, fmt.Errorf("SDK package %q was not returned by the driver loader", path)
		}
	}
	return out, nil
}

// driverFindingsSource is the FindingSource that runs the Capslock batch
// inside the same driver environment as the loader: the analysis describes the
// layout's target configuration, never the host, and never runs a toolchain
// binary.
func driverFindingsSource() FindingSource {
	return FindingSourceFunc(func(paths []string) ([]capslockadapter.GenerationFinding, error) {
		return capslockadapter.GenerationFindingsForEnv(nil, paths)
	})
}

// GenerateViaLayout runs Generate with a whole-stdlib layout as the loading
// substrate (task req 5, design I5): both the inventory load and the Capslock
// load are served through the packagelayout self-exec driver inside a single
// driver environment, so explicit-input generation executes no binary other
// than arcc's own self-exec driver. When in.Loader or in.Findings is nil the
// driver-backed implementations are used.
func GenerateViaLayout(in GenerationInput, layout *packagelayout.Layout) (*GeneratedMap, error) {
	if layout == nil {
		return nil, fmt.Errorf("generating from the layout driver: the layout is required")
	}
	if in.Loader == nil {
		in.Loader = &LayoutLoader{Layout: layout}
	}
	if in.Findings == nil {
		in.Findings = driverFindingsSource()
	}
	var out *GeneratedMap
	err := withLayoutDriverEnv(layout, func() error {
		var err error
		out, err = Generate(in)
		return err
	})
	return out, err
}

// GenerateExplicit is the explicit-input generation entry point the CLI wires
// (task reqs 4–8): the target configuration and package list are explicit
// inputs, the standard library is loaded from the SDK root through the
// layout driver, and the oracle is reconciled against the layout's discovery.
// A cgo-enabled target configuration is rejected before any loading, with an
// error naming the design's out-of-scope decision (task req 8, design
// §Explicitly out of scope).
func GenerateExplicit(target TargetConfig, packageList []string, sdkRoot string) (*GeneratedMap, error) {
	if target.CgoEnabled {
		return nil, fmt.Errorf("generating the stdlib map for target %s/%s: cgo-enabled target configurations are out of scope (the design's explicit out-of-scope decision, DR-11); hermetic generation needs the SDK's cgo packages preprocessed by a C toolchain, which this design defines no seam for; regenerate with cgo_enabled=false", target.GOOS, target.GOARCH)
	}
	if target.ToolchainVersion == "" {
		return nil, fmt.Errorf("generating the stdlib map: toolchain_version is required")
	}
	toolchainVersion, goexperiment := target.ToolchainVersion, target.GOEXPERIMENT
	platform := &packagelayout.Platform{
		GOOS:             target.GOOS,
		GOARCH:           target.GOARCH,
		BuildTags:        target.BuildTags,
		ToolchainVersion: &toolchainVersion,
		GOEXPERIMENT:     &goexperiment,
	}
	layout, err := packagelayout.StdlibLayout(sdkRoot, platform)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	entries, err := ReconcilePackageList(packageList, layout, sdkRoot)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	in := GenerationInput{
		Target:      target,
		RuleVersion: RuleVersion,
		Oracle:      PackageOracleFunc(func() ([]PackageEntry, error) { return entries, nil }),
	}
	return GenerateViaLayout(in, layout)
}
