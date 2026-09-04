package stdlibmap

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// TargetEnv renders the process environment that makes the Go toolchain
// build for the target configuration (task reqs 8–9): GOOS, GOARCH,
// CGO_ENABLED and GOEXPERIMENT override the host's values so every toolchain
// invocation underneath the loader describes the target. The returned slice
// is additive overrides, not a full environment.
func TargetEnv(t TargetConfig) []string {
	env := []string{
		"GOOS=" + t.GOOS,
		"GOARCH=" + t.GOARCH,
		"CGO_ENABLED=" + bool01(t.CgoEnabled),
		"GOEXPERIMENT=" + t.GOEXPERIMENT,
	}
	for _, tag := range t.BuildTags {
		env = append(env, "GOFLAGS=-tags="+tag)
	}
	return env
}

func bool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// NativeLoader loads importable packages from the target toolchain's SDK
// sources with x/tools go/packages (task req 4). The loader is constructed
// with the target configuration: its only process state is the environment
// overrides TargetEnv produced, so cross-compilation loads the target's
// packages and no global logging or network access is involved.
type NativeLoader struct {
	// Env holds the target environment overrides (TargetEnv).
	Env []string
}

// Load implements the batch Loader seam: every path is loaded for the target
// configuration; any load or type error, or any path missing from the
// result, fails the batch with an actionable error.
func (l *NativeLoader) Load(paths []string) (map[string]*types.Package, error) {
	if len(paths) == 0 {
		return map[string]*types.Package{}, nil
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes,
		Env:   l.Env,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, paths...)
	if err != nil {
		return nil, fmt.Errorf("loading %d SDK packages: %w", len(paths), err)
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
			return nil, fmt.Errorf("SDK package %q was not returned by the loader", path)
		}
	}
	return out, nil
}
