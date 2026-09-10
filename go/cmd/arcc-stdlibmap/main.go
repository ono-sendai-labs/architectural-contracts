// Package main is the generation-only executable used by Bazel's
// ArccStdlibMap action.
//
// Component Contract (FR10):
//   - What it does: Runs the explicit stdlib-map generation command for Bazel.
//   - What it requires: Bazel-declared SDK sources, package list, target config,
//     and output arguments, plus the shared stdlibmap generator.
//   - What it provides: One hermetic stdlib-map artifact and deterministic exit
//     status/output for the ArccStdlibMap action.
//   - Ambient Authority: FILES and the process environment needed by arcc's
//     self-exec package-layout driver; it does not run the host Go toolchain or
//     depend on the normal check path.
//
// It intentionally has no dependency on cmd/arcc/app or the component-check
// path. stdlibmap's packagelayout dependency keeps the GOPACKAGESDRIVER
// self-exec entry point available to explicit generation.
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
