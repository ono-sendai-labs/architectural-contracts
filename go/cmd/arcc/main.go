// Package main is the entry point for the arcc CLI tool.
//
// Component Contract (FR10):
// - What it does: Serves as the main entrypoint that launches the arcc CLI tool.
// - What it requires: Command-line arguments specifying the check path and output format, as well as an environment for stdout/stderr output.
// - What it provides: Actionable conformance reports and deterministic exit codes.
// - Ambient Authority: This component is a shell component and holds FILES, REFLECT, READ_SYSTEM_STATE, and UNSAFE_POINTER.
package main

import (
	"io"
	"os"

	"github.com/ono-sendai-labs/architectural-contracts/go/cmd/arcc/app"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	runner := &app.Runner{
		Loader:   goanalysis.LoadPackageFacts,
		Analyzer: capslockadapter.NewAdapter(),
	}
	return runner.Run(args, stdout, stderr)
}
