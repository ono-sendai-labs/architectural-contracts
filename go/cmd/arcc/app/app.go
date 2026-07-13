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
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

const version = "0.0.0-dev"

// PackageLoader is a function type that loads package facts for a given component root.
type PackageLoader func(componentRoot string) (facts.PackageFacts, error)

// Runner orchestrates the CLI execution of the architectural contracts check.
type Runner struct {
	Loader   PackageLoader
	Analyzer capanalyzer.CapabilityAnalyzer
}

// Run executes the application logic based on the provided CLI arguments.
func (r *Runner) Run(args []string, stdout, stderr io.Writer) int {
	var formatJSON bool
	var cleanArgs []string

	for _, arg := range args {
		if arg == "--format=json" {
			formatJSON = true
		} else if strings.HasPrefix(arg, "-") && arg != "--version" && arg != "--help" && arg != "-h" {
			fmt.Fprintf(stderr, "unknown option: %s\n", arg)
			printUsage(stderr)
			return 2
		} else {
			cleanArgs = append(cleanArgs, arg)
		}
	}

	if len(cleanArgs) == 1 && cleanArgs[0] == "--version" {
		fmt.Fprintf(stdout, "arcc %s\n", version)
		return 0
	}

	if len(cleanArgs) == 0 || cleanArgs[0] == "help" || cleanArgs[0] == "--help" || cleanArgs[0] == "-h" {
		printUsage(stdout)
		return 0
	}

	if cleanArgs[0] != "check" {
		fmt.Fprintf(stderr, "unknown command: %s\n", cleanArgs[0])
		printUsage(stderr)
		return 2
	}

	if len(cleanArgs) != 2 {
		fmt.Fprintln(stderr, "error: check command requires exactly one argument")
		printUsage(stderr)
		return 2
	}

	manifestPath := cleanArgs[1]

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

	// 2. Derive the component root from the cleaned manifest path's directory
	cleanPath := filepath.Clean(manifestPath)
	componentRoot, err := filepath.Abs(filepath.Dir(cleanPath))
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve absolute path of component root: %v\n", err)
		return 2
	}

	// 3. Load facts for that root
	loadedFacts, err := r.Loader(componentRoot)
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
	fmt.Fprintln(w, "  arcc check <manifest>")
	fmt.Fprintln(w, "  arcc --version")
}
