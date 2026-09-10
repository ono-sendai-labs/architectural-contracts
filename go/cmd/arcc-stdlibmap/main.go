// Package main is the generation-only executable used by Bazel's
// ArccStdlibMap action. It intentionally has no dependency on cmd/arcc/app or
// the component-check path. stdlibmap's packagelayout dependency also keeps
// the GOPACKAGESDRIVER self-exec entry point available to explicit generation.
package main

import (
	"fmt"
	"os"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

func main() {
	args := os.Args[1:]
	if len(args) < 2 || args[0] != "stdlibmap" || args[1] != "generate" {
		fmt.Fprintln(os.Stderr, "error: arcc-stdlibmap requires `stdlibmap generate`")
		os.Exit(2)
	}
	os.Exit(stdlibmap.RunGenerateCommand(args[2:], os.Stdout, os.Stderr))
}
