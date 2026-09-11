// Package app implements CLI orchestration and formatting of architectural checks.
//
// Component Contract (FR10):
// - What it does: Orchestrates manifest parsing, fact loading, stdlib-authority resolution, manifest-carried analysis-defeating policy selection, checker execution over the typed reference/import facts, report rendering, report-verdict assertion, check artifact emission (canonical report and exact surface from one analysis invocation), and the stdlibmap subcommands (generate native and explicit-input mode, inspect).
// - What it requires: Command-line arguments specifying the command path and output format, as well as an environment for stdout/stderr output. Explicit-input `stdlibmap generate` declares its whole target (package list, config file, SDK root) and performs no host discovery. `verdict` reads its named report artifact and, for verdict-golden assertions, its named one-line expected-verdict artifact. Every check decision reads the declared stdlib-map artifact (--stdlib-map) in layout mode and the Step 4 native discovery/cache services in native mode; the map is validated fail-closed before any verdict.
// - What it provides: Actionable conformance reports and deterministic exit codes, plus the canonical report and exact surface artifacts consumed by Bazel and native workflows. Check and emitted surface share the same exact declaring-object interface.
// - Ambient Authority: This component is a shell component and holds FILES, REFLECT, READ_SYSTEM_STATE, and UNSAFE_POINTER. Explicit-input map generation adds no EXEC beyond arcc's own self-exec layout driver: it never runs the toolchain (`go env`, `go list`), while native mode runs the host toolchain.
package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
)

const version = "0.0.0-dev"

// PackageLoader is a function type that loads package facts for a component-scoped request.
type PackageLoader func(goanalysis.LoadRequest) (facts.PackageFacts, error)

// DependencySurfaceResolver consumes one persisted dependency surface. The
// production resolver is goanalysis.ResolveDependencySurface; the seam lets
// integration tests wrap its byte readers and prove that dependency freshness
// does not reuse the component package loader.
type DependencySurfaceResolver func(goanalysis.DependencySurfaceRequest) (facts.DependencyInterface, error)

// Runner orchestrates the CLI execution of the architectural contracts check.
// The Loader, AuthorityResolver, DependencySurfaceResolver, SurfaceInputsLoader,
// and ArtifactWriter
// fields are injection seams: a nil field uses the production operation, and
// tests substitute fakes to exercise exit policy and artifact publication
// without host loading. The authority resolver is the narrow stdlib seam:
// there is no check-time capability analyzer.
type Runner struct {
	Loader                    PackageLoader
	AuthorityResolver         AuthorityResolver
	DependencySurfaceResolver DependencySurfaceResolver
	SurfaceInputsLoader       SurfaceInputsLoader
	ArtifactWriter            ArtifactWriter
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

	if args[0] == "package-imports" {
		return r.runPackageImports(args[1:], stdout, stderr)
	}

	if args[0] == "package-layout-merge" {
		return r.runPackageLayoutMerge(args[1:], stdout, stderr)
	}

	if args[0] == "artifact-shape" {
		return r.runArtifactShape(args[1:], stdout, stderr)
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
		opts.workspaceDir = workspaceDir
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
	fmt.Fprintln(w, "        [--report-out=<path>] [--surface-out=<path>] [--stdlib-map=<artifact>]")
	fmt.Fprintln(w, "        [--report-verdict-only]")
	fmt.Fprintln(w, "  arcc verdict <report> (--expect=pass|fail | --expect-file=<verdict-golden>)")
	fmt.Fprintln(w, "  arcc package-imports --config=<request.json> --output=<imports.json>")
	fmt.Fprintln(w, "  arcc package-layout-merge --layout=<base.json> --imports=<imports.json> --output=<layout.json>")
	fmt.Fprintln(w, "  arcc artifact-shape <report|surface|stdlib-map> <artifact> --golden=<shape.json>")
	fmt.Fprintln(w, "  arcc stdlibmap generate --output=<path> [--toolchain=<version>] [--goos=<os>] [--goarch=<arch>] [--cgo] [--tags=<t1,t2>] [--goexperiment=<exp>]")
	fmt.Fprintln(w, "  arcc stdlibmap generate --output=<path> --package-list=<file> --config-file=<file> --sdk-root=<dir>  (explicit-input mode)")
	fmt.Fprintln(w, "  arcc stdlibmap inspect <artifact> [--expect-key=<field=value,...>] [summary | symbol <id> | init <pkg>]...")
	fmt.Fprintln(w, "  arcc --version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "check artifact emission: --report-out writes the canonical report JSON and")
	fmt.Fprintln(w, "--surface-out writes the exact canonical surface JSON, both atomically from")
	fmt.Fprintln(w, "one analysis invocation. Check and surface share the same exact")
	fmt.Fprintln(w, "declaring-object interface: no implements-closure injection on either side.")
	fmt.Fprintln(w, "The declared stdlib-map artifact (--stdlib-map) is mandatory in layout mode")
	fmt.Fprintln(w, "and supplies the validated target SDK identity and every standard-library")
	fmt.Fprintln(w, "classification the check decides from; native mode uses the on-demand cache.")
	fmt.Fprintln(w, "--report-verdict-only requires --report-out and exits 0 for both pass and")
	fmt.Fprintln(w, "fail after analysis and publication; tool errors still exit 2.")
	fmt.Fprintln(w, "Manifests are strict by default; analysis_defeating_policy: WARN")
	fmt.Fprintln(w, "explicitly keeps AnalysisDefeating findings visible as non-fatal ANALYSIS_LIMITATION warnings.")
}
