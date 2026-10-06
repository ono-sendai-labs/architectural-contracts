// Package hosttoolchain confines the host Go toolchain subprocesses (`go list
// std`, `go env`), environment/cache-directory queries, and host filesystem
// probes and file operations used by stdlibmap so that stdlibmap itself is
// pure computation.
//
// Component Contract (FR10):
//   - What it does: Runs `go list std` and `go env` on the host Go toolchain,
//     queries the user cache directory and process environment, opens and
//     creates cache/input files and directories, writes temporary layout files
//     for the self-exec layout driver, and validates/copies projected stdlib
//     export artifacts.
//   - What it requires: Nothing beyond the Go standard library (`context`,
//     `fmt`, `io`, `io/fs`, `os`, `os/exec`, `path/filepath`, `strings`).
//   - What it provides: GoEnv, GoListStd, GoVersion, UserCacheDir, Environ,
//     Getenv, SDKFS, OpenFile, MkdirAll, IsNotExist, DirectoryExists,
//     WriteTempLayout, DeclaredExportSourcePath, and CopyProjectedExport.
//   - Ambient Authority: Holds EXEC (`os/exec`), FILES (`os`,
//     `path/filepath.EvalSymlinks`), and READ_SYSTEM_STATE (`os.UserCacheDir`,
//     `os.Environ`, `os.Getenv`, `path/filepath.Abs`).
package hosttoolchain

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GoEnv runs `go env <vars...>` and returns the raw stdout text.
func GoEnv(ctx context.Context, vars ...string) (string, error) {
	args := append([]string{"env"}, vars...)
	cmd := exec.CommandContext(ctx, "go", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// GoListStd runs `go list std` in env (nil inherits the host environment) and
// returns stdout or an error including trimmed stderr.
func GoListStd(ctx context.Context, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "std")
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("discovering the standard library with `go list std`: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// GoVersion runs `go env GOVERSION` and returns the trimmed version string.
func GoVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "GOVERSION")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("discovering the toolchain version with `go env GOVERSION`: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", fmt.Errorf("`go env GOVERSION` returned no version")
	}
	return version, nil
}

// UserCacheDir returns the default root directory to use for user-specific
// cached data.
func UserCacheDir() (string, error) {
	return os.UserCacheDir()
}

// Environ returns a copy of strings representing the environment, in the form
// "key=value".
func Environ() []string {
	return os.Environ()
}

// Getenv retrieves the value of the environment variable named by the key.
func Getenv(key string) string {
	return os.Getenv(key)
}

// SDKFS returns an fs.FS rooted at the host directory root.
func SDKFS(root string) fs.FS {
	return os.DirFS(root)
}

// OpenFile opens the named host file for reading.
func OpenFile(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

// IsNotExist reports whether an error is known to report that a file or
// directory does not exist.
func IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

// DirectoryExists reports whether path exists on the host filesystem and is a
// directory.
func DirectoryExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// WriteTempLayout writes data to a new temporary file for the layout driver and
// returns its path and a cleanup function that removes it.
func WriteTempLayout(data []byte) (string, func(), error) {
	f, err := os.CreateTemp("", "arcc-stdlib-layout-*.json")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

// DeclaredExportSourcePath resolves and validates that relative under
// sourceRoot is a regular file within the declared export root (or a Bazel
// execroot symlink preserving the logical root and relative export path).
func DeclaredExportSourcePath(sourceRoot, relative string) (string, fs.FileMode, error) {
	rootAbs, err := filepath.Abs(sourceRoot)
	if err != nil {
		return "", 0, fmt.Errorf("resolving declared export root %q: %w", sourceRoot, err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", 0, fmt.Errorf("resolving declared export root %q: %w", sourceRoot, err)
	}
	sourcePath := filepath.Join(rootAbs, filepath.FromSlash(relative))
	entry, err := os.Lstat(sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("is not available below declared root %q: %w", sourceRoot, err)
	}
	if !entry.Mode().IsRegular() && entry.Mode()&os.ModeSymlink == 0 {
		return "", 0, fmt.Errorf("below declared root %q is not a regular file", sourceRoot)
	}
	resolved, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return "", 0, fmt.Errorf("cannot resolve below declared root %q: %w", sourceRoot, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", 0, fmt.Errorf("resolving export target below declared root %q: %w", sourceRoot, err)
	}
	relativeResolved, err := filepath.Rel(rootReal, resolved)
	if err != nil || relativeResolved == ".." || strings.HasPrefix(relativeResolved, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeResolved) {
		// Bazel may materialize a tree-artifact entry as an absolute symlink to
		// the same declared tree in the execroot outside this action's sandbox.
		// Permit that representation only when the target preserves the exact
		// declared logical root and relative export path; an arbitrary symlink
		// escape still fails closed.
		logicalExportPath := filepath.ToSlash(filepath.Join(sourceRoot, filepath.FromSlash(relative)))
		if !strings.HasSuffix(filepath.ToSlash(resolved), "/"+logicalExportPath) {
			return "", 0, fmt.Errorf("resolves outside declared root %q", sourceRoot)
		}
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		return "", 0, fmt.Errorf("checking resolved export below declared root %q: %w", sourceRoot, err)
	}
	if !resolvedInfo.Mode().IsRegular() {
		return "", 0, fmt.Errorf("resolved target below declared root %q is not a regular file", sourceRoot)
	}
	return resolved, resolvedInfo.Mode().Perm(), nil
}

// CopyProjectedExport copies a validated export file from sourcePath to
// destinationPath with the given mode.
func CopyProjectedExport(sourcePath, destinationPath string, mode fs.FileMode) error {
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
				_ = destination.Close()
				return writeErr
			}
			if written != readCount {
				_ = destination.Close()
				return io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = destination.Close()
			return readErr
		}
	}
	return destination.Close()
}
