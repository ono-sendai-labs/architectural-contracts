//go:build integration

package stdlibmap_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// TestNativeOracleAgainstLocalToolchain covers the native toolchain seams
// (oracle, toolchain version, loader) against the local Go installation.
func TestNativeOracleAgainstLocalToolchain(t *testing.T) {
	entries, err := stdlibmap.NativeStdPackageList(context.Background(), nil)
	if err != nil {
		t.Fatalf("NativeStdPackageList: %v", err)
	}
	var haveOS, haveInternal bool
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.Path] {
			t.Fatalf("NativeStdPackageList: duplicate path %q", e.Path)
		}
		seen[e.Path] = true
		if e.Path == "os" {
			haveOS = true
		}
		if strings.HasPrefix(e.Path, "internal/") || e.Path == "internal" {
			haveInternal = true
			if e.Importable {
				t.Fatalf("internal package %q marked importable", e.Path)
			}
		}
		for i := 1; i < len(entries); i++ {
			if entries[i-1].Path > entries[i].Path {
				t.Fatalf("NativeStdPackageList not sorted at %q > %q", entries[i-1].Path, entries[i].Path)
			}
		}
	}
	if !haveOS || !haveInternal {
		t.Fatalf("NativeStdPackageList missing os=%t internal=%t", haveOS, haveInternal)
	}

	version, err := stdlibmap.NativeToolchainVersion(context.Background())
	if err != nil {
		t.Fatalf("NativeToolchainVersion: %v", err)
	}
	if !strings.HasPrefix(version, "go1.") {
		t.Fatalf("NativeToolchainVersion = %q, want a go1.x version", version)
	}

	loader := &stdlibmap.NativeLoader{}
	loaded, err := loader.Load([]string{"os", "strings"})
	if err != nil {
		t.Fatalf("NativeLoader.Load: %v", err)
	}
	if loaded["os"] == nil || loaded["os"].Scope().Lookup("Open") == nil {
		t.Fatalf("NativeLoader did not load package os with its declarations")
	}
	if _, err := loader.Load([]string{"os/nonexistent"}); err == nil {
		t.Fatalf("NativeLoader.Load(os/nonexistent): want error, got nil")
	}
}

// TestNativeToolchainVersionCanceledContext pins NativeToolchainVersion's
// error path: a canceled context must surface as an actionable wrapped
// context error (round-1 finding: the dropped cancellation regression).
func TestNativeToolchainVersionCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stdlibmap.NativeToolchainVersion(ctx); err == nil {
		t.Fatalf("NativeToolchainVersion with canceled context: want error, got nil")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("NativeToolchainVersion error %v: want it to wrap the context error", err)
	}
}

// TestNativeLoaderCrossTarget verifies that the loader's environment really
// selects the requested target: platform-specific declarations must appear
// and disappear with GOOS (round-1 finding: overrides must reach the
// toolchain).
func TestNativeLoaderCrossTarget(t *testing.T) {
	darwin := &stdlibmap.NativeLoader{Env: []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0", "GOEXPERIMENT="}}
	loaded, err := darwin.Load([]string{"syscall"})
	if err != nil {
		t.Fatalf("loading syscall for darwin/arm64: %v", err)
	}
	sc := loaded["syscall"].Scope()
	if sc.Lookup("Sysctl") == nil {
		t.Fatalf("darwin/arm64 syscall lacks Sysctl; the target environment did not reach the toolchain")
	}
	if sc.Lookup("EPOLLIN") != nil {
		t.Fatalf("darwin/arm64 syscall has linux-only EPOLLIN")
	}

	linux := &stdlibmap.NativeLoader{Env: []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOEXPERIMENT="}}
	loaded, err = linux.Load([]string{"syscall"})
	if err != nil {
		t.Fatalf("loading syscall for linux/amd64: %v", err)
	}
	sc = loaded["syscall"].Scope()
	if sc.Lookup("EPOLLIN") == nil {
		t.Fatalf("linux/amd64 syscall lacks EPOLLIN; the target environment did not reach the toolchain")
	}
	if sc.Lookup("Sysctl") != nil {
		t.Fatalf("linux/amd64 syscall has darwin-only Sysctl")
	}
}

// TestNativeDiscoveryCrossTarget verifies that package discovery itself
// follows the target configuration, not the host (round-1 finding: the
// oracle must describe the target).
func TestNativeDiscoveryCrossTarget(t *testing.T) {
	host, err := stdlibmap.NativeStdPackageList(context.Background(), nil)
	if err != nil {
		t.Fatalf("NativeStdPackageList(host): %v", err)
	}
	js, err := stdlibmap.NativeStdPackageList(context.Background(),
		(&stdlibmap.NativeLoader{Env: stdlibmap.TargetEnv(stdlibmap.TargetConfig{GOOS: "js", GOARCH: "wasm"})}).Environment())
	if err != nil {
		t.Fatalf("NativeStdPackageList(js/wasm): %v", err)
	}
	hostSet := map[string]bool{}
	for _, e := range host {
		hostSet[e.Path] = true
	}
	var wasmOnly []string
	for _, e := range js {
		if !hostSet[e.Path] {
			wasmOnly = append(wasmOnly, e.Path)
		}
	}
	if len(wasmOnly) == 0 {
		t.Fatalf("js/wasm enumeration identical to the host's; discovery ignored the target environment")
	}
	sort.Strings(wasmOnly)
	t.Logf("js/wasm-only packages: %v", wasmOnly[:min(3, len(wasmOnly))])
}

// TestNativeLoaderLoadsHostPackages exercises the native loader seam against
// the host toolchain (hermetic only under the integration tag).
func TestNativeLoaderLoadsHostPackages(t *testing.T) {
	loader := &stdlibmap.NativeLoader{}
	loaded, err := loader.Load([]string{"os", "strings"})
	if err != nil {
		t.Fatalf("NativeLoader.Load: %v", err)
	}
	if loaded["os"] == nil || loaded["os"].Scope().Lookup("Open") == nil {
		t.Fatalf("NativeLoader did not load package os with its declarations")
	}
	if _, err := loader.Load([]string{"os/nonexistent"}); err == nil {
		t.Fatalf("NativeLoader.Load(os/nonexistent): want error, got nil")
	}
}

// TestNativeLoaderEnvironmentMerged pins the environment contract: the loader
// runs the toolchain in the host environment with the target overrides
// merged (single GOOS/GOARCH/... key, one GOFLAGS carrying every build tag).
func TestNativeLoaderEnvironmentMerged(t *testing.T) {
	loader := &stdlibmap.NativeLoader{Env: []string{"GOOS=plan9", "GOARCH=arm64", "CGO_ENABLED=0", "GOEXPERIMENT=", "GOFLAGS=-tags=x,y"}}
	env := loader.Environment()
	count := func(key string) int {
		n := 0
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				n++
			}
		}
		return n
	}
	for _, key := range []string{"GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT", "GOFLAGS", "PATH"} {
		if n := count(key); n != 1 {
			t.Fatalf("environment has %d %q entries (%v); want exactly one", n, key, env)
		}
	}
	if !containsKV(env, "GOOS=plan9") || !containsKV(env, "GOARCH=arm64") {
		t.Fatalf("environment lost the target overrides")
	}
	flags := valueOf(env, "GOFLAGS")
	if !strings.Contains(flags, "-tags=x,y") {
		t.Fatalf("GOFLAGS %q must carry every requested tag in one canonical entry", flags)
	}
}

func containsKV(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func valueOf(env []string, key string) string {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, key+"="); ok {
			return v
		}
	}
	return ""
}

// TestRealSDKInventory is the real-toolchain coverage (task AC 6): the native
// oracle enumerates every `go list std` package of the local toolchain, the
// native loader inventories every importable package, and every inventoried
// ID parses canonically. No network access is involved: all packages come
// from the local SDK.
func TestRealSDKInventory(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	entries, err := stdlibmap.NativeStdPackageList(context.Background(), nil)
	if err != nil {
		t.Fatalf("NativeStdPackageList: %v", err)
	}
	if len(entries) < 100 {
		t.Fatalf("enumerated %d packages; the oracle is not total", len(entries))
	}

	loader := &stdlibmap.NativeLoader{}
	inv, err := stdlibmap.BuildInventory(entries, loader)
	if err != nil {
		t.Fatalf("BuildInventory: %v", err)
	}

	// The inventory's package list must equal the enumeration exactly:
	// every path, with identical importable pairs (round-1 finding).
	enumerated := map[string]bool{}
	importable := 0
	for _, e := range entries {
		enumerated[e.Path+"\x00"+fmtBool(e.Importable)] = true
		if e.Importable {
			importable++
		}
	}
	if len(inv.Packages) != len(entries) {
		t.Fatalf("inventory packages = %d, enumerated = %d", len(inv.Packages), len(entries))
	}
	for _, p := range inv.Packages {
		if !enumerated[p.Path+"\x00"+fmtBool(p.Importable)] {
			t.Fatalf("inventory package %+v disagrees with the enumeration", p)
		}
	}
	if len(inv.Inits) != importable {
		t.Fatalf("inits = %d, importable packages = %d", len(inv.Inits), importable)
	}

	symbolsByPkg := map[string]int{}
	for _, id := range inv.Symbols {
		if _, err := symbol.Parse(id.String()); err != nil {
			t.Fatalf("inventoried ID %q does not parse canonically: %v", id, err)
		}
		pkg := string(id[:strings.LastIndexByte(id.String(), '.')])
		symbolsByPkg[pkg]++
		if stdlibmap.IsInternalPath(pkg) {
			t.Fatalf("internal package %q contributed symbol %q", pkg, id)
		}
	}
	for _, id := range inv.Inits {
		if _, err := symbol.Parse(id.String()); err != nil {
			t.Fatalf("inventoried init ID %q does not parse canonically: %v", id, err)
		}
		pkg := strings.TrimSuffix(id.String(), ".init")
		if stdlibmap.IsInternalPath(pkg) {
			t.Fatalf("internal package %q contributed an init record", pkg)
		}
	}

	for _, want := range []string{"os", "strings", "fmt", "net/http"} {
		if symbolsByPkg[want] == 0 {
			t.Fatalf("expected symbols for %q; got %v", want, symbolsByPkg)
		}
	}
	// Doc-only packages (e.g. runtime/race) legitimately have zero exported
	// symbols for a target configuration; totality is about representation,
	// not non-emptiness, so no per-package non-empty assertion is made.

	// Determinism: a second inventory over the same enumeration is identical.
	inv2, err := stdlibmap.BuildInventory(entries, loader)
	if err != nil {
		t.Fatalf("second BuildInventory: %v", err)
	}
	if !equalStrings(idStrings(inv.Symbols), idStrings(inv2.Symbols)) {
		t.Fatalf("inventory is not deterministic between runs")
	}
	if !equalStrings(idStrings(inv.Inits), idStrings(inv2.Inits)) {
		t.Fatalf("init inventory is not deterministic between runs")
	}
}

func fmtBool(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func idStrings(ids []symbol.SymbolID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
