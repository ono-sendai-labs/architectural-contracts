package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

func (r *Runner) runPackageImports(args []string, _, stderr io.Writer) int {
	configPath, outputPath, err := parsePackageImportsOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if err := packagelayout.WriteImportGraph(configPath, outputPath); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	return 0
}

func parsePackageImportsOptions(args []string) (string, string, error) {
	var configPath, outputPath string
	for _, arg := range args {
		var destination *string
		var name string
		switch {
		case strings.HasPrefix(arg, "--config="):
			destination, name = &configPath, "--config"
		case strings.HasPrefix(arg, "--output="):
			destination, name = &outputPath, "--output"
		default:
			return "", "", fmt.Errorf("unknown option: %s", arg)
		}
		if *destination != "" {
			return "", "", fmt.Errorf("duplicate option: %s", name)
		}
		value := strings.SplitN(arg, "=", 2)[1]
		if value == "" {
			return "", "", fmt.Errorf("empty %s value", name)
		}
		*destination = value
	}
	if configPath == "" || outputPath == "" {
		return "", "", errorsMissingPackageImportsOption(configPath, outputPath)
	}
	return configPath, outputPath, nil
}

func errorsMissingPackageImportsOption(configPath, outputPath string) error {
	if configPath == "" {
		return fmt.Errorf("missing --config value")
	}
	if outputPath == "" {
		return fmt.Errorf("missing --output value")
	}
	return nil
}
