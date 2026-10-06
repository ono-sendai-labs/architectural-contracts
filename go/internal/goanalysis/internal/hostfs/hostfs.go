// Package hostfs confines host filesystem and working-directory operations for goanalysis.
// It is deliberately not an fs.FS because sources and bazel-out export data live under
// different roots; it is the only package in goanalysis that names os.
package hostfs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func ReadFile(path string) ([]byte, error)              { return os.ReadFile(path) }
func ReadDir(path string) ([]fs.DirEntry, error)        { return os.ReadDir(path) }
func Stat(path string) (fs.FileInfo, error)             { return os.Stat(path) }
func OpenExportData(path string) (io.ReadCloser, error) { return os.Open(path) }
func Abs(path string) (string, error)                   { return filepath.Abs(path) }
