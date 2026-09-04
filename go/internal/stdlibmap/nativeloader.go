package stdlibmap

import (
	"fmt"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// TargetEnv renders the target environment overrides that make the Go
// toolchain build for the target configuration (task reqs 8–9): GOOS,
// GOARCH, CGO_ENABLED and GOEXPERIMENT replace the host's values so every
// toolchain invocation underneath the loader and oracle describes the
// target. Every requested build tag is carried in one canonical GOFLAGS
// value, so multiple tags reach the toolchain reliably. The result is
// override pairs, not a complete environment; NativeLoader.Environment
// merges them onto the host environment.
func TargetEnv(t TargetConfig) []string {
	env := []string{
		"GOOS=" + t.GOOS,
		"GOARCH=" + t.GOARCH,
		"CGO_ENABLED=" + bool01(t.CgoEnabled),
		"GOEXPERIMENT=" + t.GOEXPERIMENT,
	}
	if len(t.BuildTags) > 0 {
		env = append(env, "GOFLAGS=-tags="+strings.Join(t.BuildTags, ","))
	}
	return env
}

func bool01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// envOverrideKeys reports whether override is an override of key.
func envOverrideKeys(overrides []string) map[string]bool {
	keys := make(map[string]bool, len(overrides))
	for _, kv := range overrides {
		if k, _, ok := strings.Cut(kv, "="); ok {
			keys[k] = true
		}
	}
	return keys
}

// mergeEnv merges override pairs into a copy of base: every overridden key is
// replaced by the override (last wins), all other entries pass through. The
// result is a complete environment.
func mergeEnv(base, overrides []string) []string {
	overridden := envOverrideKeys(overrides)
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok && overridden[k] {
			continue
		}
		out = append(out, kv)
	}
	return append(out, overrides...)
}

// NativeLoader loads importable packages from the target toolchain's SDK
// sources with x/tools go/packages (task req 4). The loader is constructed
// with the target configuration's environment overrides (TargetEnv): it runs
// the toolchain in the host environment merged with those overrides, so
// cross-compilation loads the target's packages and no global logging or
// network access is involved.
type NativeLoader struct {
	// Env holds the target environment overrides (TargetEnv).
	Env []string
}

// Environment returns the complete environment the loader runs the toolchain
// in: the host environment with the target overrides merged (every override
// key replaced by the override, GOFLAGS's requested build tags carried in one
// canonical entry).
func (l *NativeLoader) Environment() []string {
	if l == nil {
		return nil
	}
	return mergeEnv(os.Environ(), l.Env)
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
		Env:   l.Environment(),
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
