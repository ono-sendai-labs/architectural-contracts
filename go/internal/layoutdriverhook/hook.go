package layoutdriverhook

import (
	"fmt"
	"os"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

func init() {
	ok, layoutPath, workspaceDir := packagelayout.DriverRequestFromEnv()
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
	if err := packagelayout.RunDriver(layout, layoutPath, workspaceDir, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "GOPACKAGESDRIVER error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
