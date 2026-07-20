// Command spikedriver validates the load-bearing seam of the hermetic arcc
// design: can Go package loading and Capslock capability analysis run entirely
// from a build-system-supplied package layout, with NO `go list` and NO
// `go.mod` at analysis time?
//
// It has three modes in one binary:
//
//   - `gen <moduleDir> <pattern>...` — loads packages the ordinary way (go list,
//     in a real module) and serializes the full closure to a package-layout
//     JSON on stdout. This stands in for what a Bazel aspect would emit from
//     Bazel's own dependency graph; the spike does not attempt to reproduce the
//     aspect, only to feed the consumer a realistic layout.
//
//   - GOPACKAGESDRIVER mode (auto-detected via ARCC_SPIKE_DRIVER=1) — answers
//     go/packages driver queries from the layout JSON named by ARCC_SPIKE_LAYOUT.
//     This is the self-exec: the analyze mode points GOPACKAGESDRIVER at this
//     same binary.
//
//   - `analyze <layout.json> <importpath>` — sets GOPACKAGESDRIVER to itself and
//     runs Capslock's real analysis path (the same calls arcc's capslockadapter
//     makes) against the driver-loaded packages, printing capability findings.
//     Intended to be run from a directory with no go.mod.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/google/capslock/analyzer"
	"golang.org/x/tools/go/packages"
)

const (
	envDriver = "ARCC_SPIKE_DRIVER" // when "1", this process is the GOPACKAGESDRIVER
	envLayout = "ARCC_SPIKE_LAYOUT" // path to the package-layout JSON the driver serves
)

// layoutFile is the on-disk package-layout format. packages.Package has custom
// (Un)MarshalJSON that emits/consumes the go/packages "flat" driver form
// (ID, Name, PkgPath, GoFiles, CompiledGoFiles, Imports as path->ID), so we get
// a faithful, round-trippable layout for free.
type layoutFile struct {
	Packages []*packages.Package `json:"packages"`
}

func main() {
	// GOPACKAGESDRIVER self-exec: go/packages invokes this binary with the query
	// patterns as command-line args and a DriverRequest JSON on stdin.
	if os.Getenv(envDriver) == "1" {
		if err := runDriver(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "spikedriver[driver]:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "gen":
		err = runGen(os.Args[2:])
	case "analyze":
		err = runAnalyze(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "spikedriver:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  spikedriver gen <moduleDir> <pattern>...   > layout.json")
	fmt.Fprintln(os.Stderr, "  spikedriver analyze <layout.json> <importpath>")
	os.Exit(2)
}

// runGen loads <pattern>... (plus the whole std library, because Capslock issues
// its own `packages.Load(nil, "std")` that the driver must be able to answer)
// from moduleDir the ordinary way, and writes the full transitive closure as a
// layout JSON to stdout.
func runGen(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: spikedriver gen <moduleDir> <pattern>...")
	}
	dir := args[0]
	patterns := append([]string{}, args[1:]...)
	patterns = append(patterns, "std")

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps,
		Dir: dir,
	}
	roots, err := packages.Load(cfg, patterns...)
	if err != nil {
		return fmt.Errorf("go list load: %w", err)
	}

	all := map[string]*packages.Package{}
	packages.Visit(roots, nil, func(p *packages.Package) {
		if len(p.Errors) > 0 {
			for _, e := range p.Errors {
				fmt.Fprintf(os.Stderr, "spikedriver[gen]: warning: %s: %s\n", p.PkgPath, e)
			}
		}
		all[p.ID] = p
	})

	list := make([]*packages.Package, 0, len(all))
	for _, p := range all {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(layoutFile{Packages: list})
}

// runDriver implements the GOPACKAGESDRIVER protocol: read the DriverRequest
// from stdin, resolve the query patterns against the layout, and write a
// DriverResponse to stdout. The full package set is always returned; Roots are
// the packages matching the query patterns.
func runDriver(patterns []string) error {
	// Drain the DriverRequest. The spike ignores Mode/Env/Overlay: it always
	// returns the full layout and lets go/packages populate the fields it needs.
	var req packages.DriverRequest
	_ = json.NewDecoder(os.Stdin).Decode(&req)

	layoutPath := os.Getenv(envLayout)
	if layoutPath == "" {
		return fmt.Errorf("%s not set", envLayout)
	}
	data, err := os.ReadFile(layoutPath)
	if err != nil {
		return fmt.Errorf("read layout: %w", err)
	}
	var layout layoutFile
	if err := json.Unmarshal(data, &layout); err != nil {
		return fmt.Errorf("parse layout: %w", err)
	}

	byPath := make(map[string]*packages.Package, len(layout.Packages))
	var stdIDs []string
	for _, p := range layout.Packages {
		byPath[p.PkgPath] = p
		if isStdlib(p.PkgPath) {
			stdIDs = append(stdIDs, p.ID)
		}
	}

	var roots []string
	seen := map[string]bool{}
	addRoot := func(id string) {
		if !seen[id] {
			seen[id] = true
			roots = append(roots, id)
		}
	}
	for _, pat := range patterns {
		switch pat {
		case "std":
			for _, id := range stdIDs {
				addRoot(id)
			}
		default:
			if p, ok := byPath[pat]; ok {
				addRoot(p.ID)
			} else {
				fmt.Fprintf(os.Stderr, "spikedriver[driver]: no layout package for pattern %q\n", pat)
			}
		}
	}

	resp := packages.DriverResponse{
		Compiler: "gc",
		Arch:     runtime.GOARCH,
		Roots:    roots,
		Packages: layout.Packages,
	}
	return json.NewEncoder(os.Stdout).Encode(&resp)
}

// runAnalyze points GOPACKAGESDRIVER at this binary and runs the same Capslock
// analysis calls arcc's capslockadapter makes, against driver-loaded packages.
func runAnalyze(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: spikedriver analyze <layout.json> <importpath>")
	}
	layoutPath, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	target := args[1]

	self, err := os.Executable()
	if err != nil {
		return err
	}
	// The self-exec wiring. These are inherited by the driver child that
	// go/packages spawns.
	os.Setenv("GOPACKAGESDRIVER", self)
	os.Setenv(envDriver, "1")
	os.Setenv(envLayout, layoutPath)

	// Exactly capslockadapter's load mode.
	cfg := &packages.Config{Mode: analyzer.PackagesLoadModeNeeded}
	pkgs, err := packages.Load(cfg, target)
	if err != nil {
		return fmt.Errorf("packages.Load via driver: %w", err)
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("driver returned no packages for %q", target)
	}

	var loadErrs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			loadErrs = append(loadErrs, e.Error())
		}
	})
	if len(loadErrs) > 0 {
		return fmt.Errorf("driver-loaded packages have %d error(s):\n%s",
			len(loadErrs), strings.Join(dedup(loadErrs), "\n"))
	}

	queried := analyzer.GetQueriedPackages(pkgs)
	cil := analyzer.GetCapabilityInfo(pkgs, queried, &analyzer.Config{
		Classifier:  analyzer.GetClassifier(true),
		Granularity: analyzer.GranularityFunction,
	})

	infos := cil.GetCapabilityInfo()
	fmt.Printf("target %s: %d capability finding(s)\n", target, len(infos))
	caps := map[string]int{}
	for _, ci := range infos {
		caps[ci.GetCapabilityName()]++
	}
	names := make([]string, 0, len(caps))
	for n := range caps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("  %-22s x%d\n", n, caps[n])
	}
	// Show one representative call path so we can see it really traced source.
	for _, ci := range infos {
		path := ci.GetPath()
		if len(path) < 2 {
			continue
		}
		fmt.Printf("  e.g. %s: ", ci.GetCapabilityName())
		parts := make([]string, 0, len(path))
		for _, fr := range path {
			parts = append(parts, fr.GetName())
		}
		fmt.Println(strings.Join(parts, " -> "))
		break
	}
	return nil
}

// isStdlib reports whether importPath is a standard-library package, using the
// same heuristic the go tool uses: the first path segment contains no dot.
func isStdlib(importPath string) bool {
	if importPath == "" {
		return false
	}
	first := importPath
	if i := strings.IndexByte(importPath, '/'); i >= 0 {
		first = importPath[:i]
	}
	return !strings.Contains(first, ".")
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
