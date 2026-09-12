package stdlibmap

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

const bazelExecrootPlaceholder = "__BAZEL_EXECROOT__/"

// ProjectExportFiles copies exactly the compiler export artifacts named by a
// rules_go stdlib metadata stream into an output tree. The input tree may be a
// rules_go build-cache-shaped artifact, but the result is deliberately
// export-only: action IDs, dependency records, SDK files and tool binaries are
// never copied across the producer boundary (design I5/N2).
func ProjectExportFiles(metadataPath, sourceRoot, outputRoot string) error {
	if metadataPath == "" || sourceRoot == "" || outputRoot == "" {
		return fmt.Errorf("projecting standard-library export files requires metadata, source root and output root")
	}

	sourceRoot = filepath.Clean(sourceRoot)
	sourceRootSlash := filepath.ToSlash(sourceRoot)
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		return fmt.Errorf("creating projected standard-library export tree %q: %w", outputRoot, err)
	}

	metadata, err := os.Open(metadataPath)
	if err != nil {
		return fmt.Errorf("opening standard-library export metadata %q: %w", metadataPath, err)
	}
	defer metadata.Close()

	decoder := json.NewDecoder(metadata)
	seen := map[string]bool{}
	projected := 0
	for recordNumber := 0; ; recordNumber++ {
		var record struct {
			ExportFile string `json:"ExportFile"`
		}
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding standard-library export metadata record %d: %w", recordNumber, err)
		}
		if record.ExportFile == "" {
			continue
		}

		relative, err := projectedExportRelativePath(record.ExportFile, sourceRootSlash)
		if err != nil {
			return fmt.Errorf("standard-library export metadata record %d: %w", recordNumber, err)
		}
		if seen[relative] {
			continue
		}
		seen[relative] = true

		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(relative))
		info, err := os.Stat(sourcePath)
		if err != nil {
			return fmt.Errorf("standard-library export file %q is not available below declared root %q: %w", record.ExportFile, sourceRoot, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("standard-library export file %q below declared root %q is not a regular file", record.ExportFile, sourceRoot)
		}

		destinationPath := filepath.Join(outputRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return fmt.Errorf("creating projected export directory for %q: %w", relative, err)
		}
		if err := copyProjectedExport(sourcePath, destinationPath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("projecting standard-library export file %q: %w", record.ExportFile, err)
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

func copyProjectedExport(sourcePath, destinationPath string, mode os.FileMode) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	buffer := make([]byte, 32*1024)
	for {
		readCount, readErr := source.Read(buffer)
		if readCount > 0 {
			written, writeErr := destination.Write(buffer[:readCount])
			if writeErr != nil {
				destination.Close()
				return writeErr
			}
			if written != readCount {
				destination.Close()
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			destination.Close()
			return readErr
		}
	}
	return destination.Close()
}

// RunProjectExportCommand implements the explicit-input stdlib export
// projection used by the single shared Bazel producer. It accepts only paths
// supplied by the action; there is no toolchain or host discovery fallback.
func RunProjectExportCommand(args []string, stdout, stderr io.Writer) int {
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
	if err := ProjectExportFiles(values["--metadata"], values["--source-root"], values["--output"]); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "projected standard-library export files into %s\n", values["--output"])
	return 0
}
