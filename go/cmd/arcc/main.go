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
