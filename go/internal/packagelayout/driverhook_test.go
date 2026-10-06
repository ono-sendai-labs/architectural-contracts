package packagelayout

import (
	"fmt"
	"os"
)

// In packagelayout tests, self-exec GOPACKAGESDRIVER interception is hooked
// directly in the test package to avoid an import cycle with layoutdriverhook
// (which imports packagelayout). Production packagelayout has no init() hook;
// that responsibility belongs to layoutdriverhook.
func init() {
	ok, layoutPath, workspaceDir := DriverRequestFromEnv()
	if !ok {
		return
	}
	if layoutPath == "" {
		fmt.Fprintln(os.Stderr, "GOPACKAGESDRIVER error: package-layout file path is required")
		os.Exit(1)
	}
	layout, err := os.ReadFile(layoutPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GOPACKAGESDRIVER error: opening package-layout: %v\n", err)
		os.Exit(1)
	}
	if err := RunDriver(layout, layoutPath, workspaceDir, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "GOPACKAGESDRIVER error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
