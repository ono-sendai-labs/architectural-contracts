package stdlibmap

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap/internal/hosttoolchain"
	"golang.org/x/tools/go/packages"
)

func targetEnv(t TargetConfig) []string {
	env := []string{
		"GOOS=" + t.GOOS,
		"GOARCH=" + t.GOARCH,
		"CGO_ENABLED=" + bool01(t.CgoEnabled),
		"GOEXPERIMENT=" + t.GOEXPERIMENT,
		"GOFLAGS=",
	}
	if len(t.BuildTags) > 0 {
		env[len(env)-1] = "GOFLAGS=-tags=" + strings.Join(t.BuildTags, ",")
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

func nativeLoaderEnvironment(l *NativeLoader) []string {
	if l == nil {
		return nil
	}
	return mergeEnv(hosttoolchain.Environ(), l.Env)
}

func nativeLoaderLoad(l *NativeLoader, paths []string) (map[string]*types.Package, error) {
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
