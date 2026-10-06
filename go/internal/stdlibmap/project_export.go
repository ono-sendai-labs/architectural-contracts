package stdlibmap

import (
	"fmt"
	"io"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap/internal/hosttoolchain"
)

const bazelExecrootPlaceholder = "__BAZEL_EXECROOT__/"

// projectExportFiles copies exactly the compiler export artifacts named by a
// rules_go stdlib metadata stream into an output tree. The input tree may be a
// rules_go build-cache-shaped artifact, but the result is deliberately
// export-only: action IDs, dependency records, SDK files and tool binaries are
// never copied across the producer boundary (design I5/N2).
func projectExportFiles(metadataPath, sourceRoot, outputRoot string) error {
	if metadataPath == "" || sourceRoot == "" || outputRoot == "" {
		return fmt.Errorf("projecting standard-library export files requires metadata, source root and output root")
	}

	sourceRoot = filepath.Clean(sourceRoot)
	sourceRootSlash := filepath.ToSlash(sourceRoot)
	if err := hosttoolchain.MkdirAll(outputRoot, 0o755); err != nil {
		return fmt.Errorf("creating projected standard-library export tree %q: %w", outputRoot, err)
	}

	metadata, err := hosttoolchain.OpenFile(metadataPath)
	if err != nil {
		return fmt.Errorf("opening standard-library export metadata %q: %w", metadataPath, err)
	}
	defer metadata.Close()

	exportFiles, err := packagelayout.DecodeStdlibExportFiles(metadata)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	projected := 0
	for recordNumber, exportFile := range exportFiles {
		relative, err := projectedExportRelativePath(exportFile, sourceRootSlash)
		if err != nil {
			return fmt.Errorf("standard-library export metadata record %d: %w", recordNumber, err)
		}
		if seen[relative] {
			continue
		}
		seen[relative] = true

		sourcePath, mode, err := hosttoolchain.DeclaredExportSourcePath(sourceRoot, relative)
		if err != nil {
			return fmt.Errorf("standard-library export file %q: %w", exportFile, err)
		}

		destinationPath := filepath.Join(outputRoot, filepath.FromSlash(relative))
		if err := hosttoolchain.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return fmt.Errorf("creating projected export directory for %q: %w", relative, err)
		}
		if err := hosttoolchain.CopyProjectedExport(sourcePath, destinationPath, mode); err != nil {
			return fmt.Errorf("projecting standard-library export file %q: %w", exportFile, err)
		}
		projected++
	}
	if projected == 0 {
		return fmt.Errorf("standard-library export metadata names no compiler export artifacts")
	}
	return nil
}

func projectedExportRelativePath(value, sourceRoot string) (string, error) {
	if !strings.HasPrefix(value, bazelExecrootPlaceholder) {
		return "", fmt.Errorf("standard-library export path %q has no %s prefix", value, bazelExecrootPlaceholder)
	}
	pathValue := strings.TrimPrefix(value, bazelExecrootPlaceholder)
	prefix := sourceRoot + "/"
	if !strings.HasPrefix(pathValue, prefix) {
		return "", fmt.Errorf("standard-library export path %q is outside declared export root %q", value, sourceRoot)
	}
	relative := strings.TrimPrefix(pathValue, prefix)
	if relative == "" || strings.HasPrefix(relative, "/") || filepath.VolumeName(filepath.FromSlash(relative)) != "" {
		return "", fmt.Errorf("standard-library export path %q has an invalid relative path", value)
	}
	clean := pathpkg.Clean(relative)
	if clean != relative || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("standard-library export path %q escapes its declared export root", value)
	}
	return clean, nil
}

func runProjectExportCommand(args []string, stdout, stderr io.Writer) int {
	values := map[string]string{}
	for _, arg := range args {
		matched := false
		for _, name := range []string{"--metadata", "--source-root", "--output"} {
			prefix := name + "="
			if !strings.HasPrefix(arg, prefix) {
				continue
			}
			matched = true
			if values[name] != "" {
				fmt.Fprintf(stderr, "error: duplicate option: %s\n", name)
				return 2
			}
			values[name] = strings.TrimPrefix(arg, prefix)
			if values[name] == "" {
				fmt.Fprintf(stderr, "error: empty option value: %s\n", name)
				return 2
			}
			break
		}
		if !matched {
			fmt.Fprintf(stderr, "error: unknown option: %s\n", arg)
			return 2
		}
	}
	for _, name := range []string{"--metadata", "--source-root", "--output"} {
		if values[name] == "" {
			fmt.Fprintf(stderr, "error: project requires %s=<path>\n", name)
			return 2
		}
	}
	if err := projectExportFiles(values["--metadata"], values["--source-root"], values["--output"]); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "projected standard-library export files into %s\n", values["--output"])
	return 0
}
