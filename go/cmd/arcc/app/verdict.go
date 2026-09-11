package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

// runVerdict implements `arcc verdict <report> --expect=pass|fail` and its
// file-backed equivalent used by verdict goldens. It asserts the recorded
// verdict of a persisted report artifact. Exit 0 when the report is valid and
// its verdict matches the expectation, 1 when the verdict is the valid
// opposite, and 2 for usage, open, decode, or validation errors. The mismatch
// diagnostic names both the expected and actual verdict (task req 4).
func (r *Runner) runVerdict(args []string, stdout, stderr io.Writer) int {
	var reportPath string
	expectSpecified := false
	expectFile := ""
	expectOption := ""
	var expect report.Verdict

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--expect="):
			if expectSpecified {
				if expectOption == "--expect" {
					fmt.Fprintln(stderr, "error: duplicate option: --expect")
				} else {
					fmt.Fprintln(stderr, "error: verdict command accepts one of --expect or --expect-file")
				}
				return 2
			}
			val := strings.TrimPrefix(arg, "--expect=")
			switch report.Verdict(val) {
			case report.VerdictPass, report.VerdictFail:
				expect = report.Verdict(val)
			default:
				fmt.Fprintf(stderr, "error: --expect must be pass or fail, got %q\n", val)
				return 2
			}
			expectSpecified = true
			expectOption = "--expect"
		case strings.HasPrefix(arg, "--expect-file="):
			if expectSpecified {
				if expectOption == "--expect-file" {
					fmt.Fprintln(stderr, "error: duplicate option: --expect-file")
				} else {
					fmt.Fprintln(stderr, "error: verdict command accepts one of --expect or --expect-file")
				}
				return 2
			}
			expectFile = strings.TrimPrefix(arg, "--expect-file=")
			if expectFile == "" {
				fmt.Fprintln(stderr, "error: empty --expect-file value")
				return 2
			}
			expectSpecified = true
			expectOption = "--expect-file"
		case arg == "--expect":
			fmt.Fprintln(stderr, "error: missing --expect value")
			return 2
		case arg == "--expect-file":
			fmt.Fprintln(stderr, "error: missing --expect-file value")
			return 2
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "unknown option: %s\n", arg)
			printUsage(stderr)
			return 2
		default:
			if reportPath != "" {
				fmt.Fprintln(stderr, "error: verdict command requires exactly one report argument")
				printUsage(stderr)
				return 2
			}
			reportPath = arg
		}
	}

	if reportPath == "" {
		fmt.Fprintln(stderr, "error: verdict command requires exactly one report argument")
		printUsage(stderr)
		return 2
	}
	if !expectSpecified {
		fmt.Fprintln(stderr, "error: verdict command requires --expect=pass|fail or --expect-file=<path>")
		printUsage(stderr)
		return 2
	}
	if expectFile != "" {
		var err error
		expect, err = artifactio.ReadVerdictGoldenFile(expectFile)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}

	persisted, err := artifactio.ReadReportFile(reportPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	actual := persisted.Verdict
	if actual != expect {
		fmt.Fprintf(stderr, "error: report %q verdict mismatch: expected %s, got %s\n", reportPath, expect, actual)
		return 1
	}
	fmt.Fprintf(stdout, "verdict %s matches expectation\n", actual)
	return 0
}
