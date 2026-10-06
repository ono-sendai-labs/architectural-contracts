package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/atomicfile"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

func (r *Runner) runPackageImports(args []string, _, stderr io.Writer) int {
	configPath, outputPath, err := parsePackageImportsOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", fmt.Errorf("reading import graph request %q: %w", configPath, err))
		return 2
	}
	out, err := packagelayout.WriteImportGraph(configBytes)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if err := atomicfile.WriteFileAtomic(outputPath, out, 0o666, nil); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", fmt.Errorf("writing import graph %q: %w", outputPath, err))
		return 2
	}
	return 0
}

func (r *Runner) runPackageLayoutMerge(args []string, _, stderr io.Writer) int {
	baseLayout, imports, output, err := parsePackageLayoutMergeOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	baseBytes, err := os.ReadFile(baseLayout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", fmt.Errorf("reading base package layout %q: %w", baseLayout, err))
		return 2
	}
	graphBytes, err := os.ReadFile(imports)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", fmt.Errorf("reading ordinary import graph %q: %w", imports, err))
		return 2
	}
	out, err := packagelayout.MergeImportGraphLayout(baseBytes, graphBytes)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if err := atomicfile.WriteFileAtomic(output, out, 0o666, nil); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", fmt.Errorf("writing merged package layout %q: %w", output, err))
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

func parsePackageLayoutMergeOptions(args []string) (string, string, string, error) {
	values := map[string]*string{
		"--layout":  nil,
		"--imports": nil,
		"--output":  nil,
	}
	var layoutPath, importsPath, outputPath string
	values["--layout"] = &layoutPath
	values["--imports"] = &importsPath
	values["--output"] = &outputPath
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok {
			return "", "", "", fmt.Errorf("missing value for %s", arg)
		}
		destination, ok := values[name]
		if !ok {
			return "", "", "", fmt.Errorf("unknown option: %s", arg)
		}
		if *destination != "" {
			return "", "", "", fmt.Errorf("duplicate option: %s", name)
		}
		if value == "" {
			return "", "", "", fmt.Errorf("empty %s value", name)
		}
		*destination = value
	}
	if layoutPath == "" {
		return "", "", "", fmt.Errorf("missing --layout value")
	}
	if importsPath == "" {
		return "", "", "", fmt.Errorf("missing --imports value")
	}
	if outputPath == "" {
		return "", "", "", fmt.Errorf("missing --output value")
	}
	return layoutPath, importsPath, outputPath, nil
}
