// Package app implements CLI orchestration and formatting of architectural checks.
//
// Component Contract (FR10):
// - What it does: Orchestrates manifest parsing, fact loading, capability analysis, checker execution, report rendering, report-verdict assertion, and the stdlibmap subcommands (generate native and explicit-input mode, inspect).
// - What it requires: Command-line arguments specifying the command path and output format, as well as an environment for stdout/stderr output. Explicit-input `stdlibmap generate` declares its whole target (package list, config file, SDK root) and performs no host discovery. `verdict` reads only its named report artifact argument.
// - What it provides: Actionable conformance reports and deterministic exit codes.
// - Ambient Authority: This component is a shell component and holds FILES, REFLECT, READ_SYSTEM_STATE, and UNSAFE_POINTER. Explicit-input map generation adds no EXEC beyond arcc's own self-exec layout driver: it never runs the toolchain (`go env`, `go list`), while native mode runs the host toolchain.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
)

const version = "0.0.0-dev"

// PackageLoader is a function type that loads package facts for a component-scoped request.
type PackageLoader func(goanalysis.LoadRequest) (facts.PackageFacts, error)

// Runner orchestrates the CLI execution of the architectural contracts check.
// The Loader, Analyzer, KeyResolver, SurfaceInputsLoader, and ArtifactWriter
// fields are injection seams: a nil field uses the production operation, and
// tests substitute fakes to exercise exit policy and artifact publication
// without host loading (plan Step 5 task 3).
type Runner struct {
	Loader              PackageLoader
	Analyzer            capanalyzer.CapabilityAnalyzer
	KeyResolver         SDKKeyResolver
	SurfaceInputsLoader SurfaceInputsLoader
	ArtifactWriter      ArtifactWriter
}

// Run executes the application logic based on the provided CLI arguments.
func (r *Runner) Run(args []string, stdout, stderr io.Writer) int {
	// Serialize all check executions in this process: layout mode mutates
	// process-global state (active layout, driver env) that every check reads.
	var code int
	goanalysis.SerializeChecks(func() {
		code = r.run(args, stdout, stderr)
	})
	return code
}

func (r *Runner) run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}

	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintf(stdout, "arcc %s\n", version)
		return 0
	}

	if args[0] == "stdlibmap" {
		return r.runStdlibmap(args[1:], stdout, stderr)
	}

	if args[0] == "verdict" {
		return r.runVerdict(args[1:], stdout, stderr)
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

	opts, err := parseCheckOptions(args[1:])
	if err != nil {
		if strings.HasPrefix(err.Error(), "unknown option") || strings.Contains(err.Error(), "exactly one argument") {
			fmt.Fprintf(stderr, "error: %v\n", err)
			printUsage(stderr)
		} else {
			fmt.Fprintf(stderr, "error: %v\n", err)
		}
		return 2
	}

	if opts.packageLayout != "" {
		absLayoutPath, err := filepath.Abs(opts.packageLayout)
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
		err = goanalysis.WithDriverEnv(absLayoutPath, workspaceDir, func() error {
			exitCode = r.runCheck(opts, stdout, stderr)
			return nil
		})
		if err != nil {
			fmt.Fprintf(stderr, "error: package-layout loading failed: %v\n", err)
			return 2
		}
		return exitCode
	}

	return r.runCheck(opts, stdout, stderr)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "arcc checks Go architectural component contracts.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  arcc check <manifest> [--package-layout=<layout>] [--format=json]")
	fmt.Fprintln(w, "  arcc verdict <report> --expect=pass|fail")
	fmt.Fprintln(w, "  arcc stdlibmap generate --output=<path> [--toolchain=<version>] [--goos=<os>] [--goarch=<arch>] [--cgo] [--tags=<t1,t2>] [--goexperiment=<exp>]")
	fmt.Fprintln(w, "  arcc stdlibmap generate --output=<path> --package-list=<file> --config-file=<file> --sdk-root=<dir>  (explicit-input mode)")
	fmt.Fprintln(w, "  arcc stdlibmap inspect <artifact> [--expect-key=<field=value,...>] [summary | symbol <id> | init <pkg>]...")
	fmt.Fprintln(w, "  arcc --version")
}
