// Package app implements CLI orchestration and formatting of architectural checks.
//
// Component Contract (FR10):
// - What it does: Orchestrates manifest parsing, fact loading, capability analysis, checker execution, and report rendering.
// - What it requires: Command-line arguments specifying the check path and output format, as well as an environment for stdout/stderr output.
// - What it provides: Actionable conformance reports and deterministic exit codes.
// - Ambient Authority: This component is a shell component and holds FILES, REFLECT, READ_SYSTEM_STATE, and UNSAFE_POINTER.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

const version = "0.0.0-dev"

// PackageLoader is a function type that loads package facts for a component-scoped request.
type PackageLoader func(goanalysis.LoadRequest) (facts.PackageFacts, error)

// Runner orchestrates the CLI execution of the architectural contracts check.
type Runner struct {
	Loader   PackageLoader
	Analyzer capanalyzer.CapabilityAnalyzer
}

// Run executes the application logic based on the provided CLI arguments.
func (r *Runner) Run(args []string, stdout, stderr io.Writer) int {
	packagelayout.CheckMu.Lock()
	defer packagelayout.CheckMu.Unlock()

	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}

	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintf(stdout, "arcc %s\n", version)
		return 0
	}

	if args[0] != "check" {
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}

	if len(args) < 2 {
		fmt.Fprintln(stderr, "error: check command requires exactly one argument")
		printUsage(stderr)
		return 2
	}

	manifestPath := args[1]
	var packageLayoutPath string
	var formatJSON bool
	var hasLayout, hasFormat bool

	for i := 2; i < len(args); i++ {
		arg := args[i]
		if arg == "--format=json" {
			if hasFormat {
				fmt.Fprintln(stderr, "error: duplicate option: --format=json")
				return 2
			}
			formatJSON = true
			hasFormat = true
		} else if strings.HasPrefix(arg, "--package-layout=") {
			if hasLayout {
				fmt.Fprintln(stderr, "error: duplicate option: --package-layout")
				return 2
			}
			val := strings.TrimPrefix(arg, "--package-layout=")
			if val == "" {
				fmt.Fprintln(stderr, "error: empty package layout value")
				return 2
			}
			packageLayoutPath = val
			hasLayout = true
		} else if strings.HasPrefix(arg, "--package-layout") {
			fmt.Fprintln(stderr, "error: missing package layout value")
			return 2
		} else if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(stderr, "unknown option: %s\n", arg)
			printUsage(stderr)
			return 2
		} else {
			fmt.Fprintln(stderr, "error: check command requires exactly one argument")
			printUsage(stderr)
			return 2
		}
	}

	if packageLayoutPath != "" {
		absLayoutPath, err := filepath.Abs(packageLayoutPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to resolve absolute path of package layout: %v\n", err)
			return 2
		}
		workspaceDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to get current working directory: %v\n", err)
			return 2
		}

		var exitCode int
		err = packagelayout.WithDriverEnv(absLayoutPath, workspaceDir, func() error {
			exitCode = r.runCheck(manifestPath, formatJSON, stdout, stderr)
			return nil
		})
		if err != nil {
			fmt.Fprintf(stderr, "error: package-layout loading failed: %v\n", err)
			return 2
		}
		return exitCode
	}

	return r.runCheck(manifestPath, formatJSON, stdout, stderr)
}

func (r *Runner) runCheck(manifestPath string, formatJSON bool, stdout, stderr io.Writer) int {
	// 1. Open and parse manifest
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to open manifest file: %v\n", err)
		return 2
	}
	defer manifestFile.Close()

	parsedManifest, err := manifest.Parse(manifestFile)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to parse manifest: %v\n", err)
		return 2
	}

	// Canonicalize the import paths the checker compares against loaded facts, so a
	// host that rewrites import paths at build time compares in one namespace.
	// Identity by default (hostpolicy.CanonicalizePath), so this is a no-op upstream.
	for i := range parsedManifest.AbsorbedDependencies {
		parsedManifest.AbsorbedDependencies[i].ImportPath =
			hostpolicy.CanonicalizePath(parsedManifest.AbsorbedDependencies[i].ImportPath)
	}

	// 2. Derive the component root from the cleaned manifest path's directory
	cleanPath := filepath.Clean(manifestPath)
	componentRoot, err := filepath.Abs(filepath.Dir(cleanPath))
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve absolute path of component root: %v\n", err)
		return 2
	}

	// 3. Load facts for that root
	loadedFacts, err := r.Loader(goanalysis.LoadRequest{
		ComponentRoot:  componentRoot,
		Members:        parsedManifest.Members,
		InterfaceFiles: parsedManifest.InterfaceFiles,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to load package facts: %v\n", err)
		return 2
	}

	// 4. Validate every declared interface file with goanalysis.ValidateInterfaceFiles
	if err := goanalysis.ValidateInterfaceFiles(componentRoot, parsedManifest.InterfaceFiles, loadedFacts); err != nil {
		fmt.Fprintf(stderr, "error: failed to validate interface files: %v\n", err)
		return 2
	}

	// 5. Resolve direct component dependencies
	var resolvedDeps []facts.DependencyInterface
	for _, dep := range parsedManifest.ComponentDependencies {
		depIface, err := goanalysis.ResolveDependencyInterface(componentRoot, componentRoot, dep)
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to resolve dependency %q: %v\n", dep.Name, err)
			return 2
		}
		resolvedDeps = append(resolvedDeps, depIface)
	}

	// 6. Build AnalyzeRequest.PruneAt from resolved dependencies
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
	sort.Slice(pruneAt, func(i, j int) bool {
		return pruneAt[i] < pruneAt[j]
	})

	// 7. Pass all loaded component package import paths to the analyzer
	var pkgs []string
	for _, p := range loadedFacts.Packages {
		pkgs = append(pkgs, p.ImportPath)
	}

	findings, err := r.Analyzer.Analyze(capanalyzer.AnalyzeRequest{
		Packages: pkgs,
		PruneAt:  pruneAt,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: capability analysis failed: %v\n", err)
		return 2
	}

	// 8. Start from capanalyzer.StrictPolicy, merging Manifest.DeclaredAuthority
	inputs := checker.Inputs{
		Manifest:  parsedManifest,
		Facts:     loadedFacts,
		DepIfaces: resolvedDeps,
		Caps:      findings,
		Policy:    capanalyzer.StrictPolicy(),
	}

	conformanceReport := checker.Check(inputs)

	// 7. Format output
	if formatJSON {
		marshaled, err := json.MarshalIndent(conformanceReport, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to marshal report to JSON: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "%s\n", marshaled)
	} else {
		fmt.Fprint(stdout, report.RenderText(conformanceReport))
	}

	// 8. Return exit code: 0 for no violations, 1 for violations
	if len(conformanceReport.Violations) > 0 {
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "arcc checks Go architectural component contracts.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  arcc check <manifest> [--package-layout=<layout>] [--format=json]")
	fmt.Fprintln(w, "  arcc --version")
}
