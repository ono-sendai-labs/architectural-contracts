package artifactio

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteSeams injects the filesystem operations of atomic replacement so tests
// can simulate deterministic failures at each stage (task req 7). A nil
// function field uses the real operation; production callers pass nil seams.
type WriteSeams struct {
	CreateTemp func(dir, pattern string) (*os.File, error)
	Write      func(f *os.File, data []byte) error
	Sync       func(f *os.File) error
	Close      func(f *os.File) error
	Rename     func(oldPath, newPath string) error
}

// WriteFileAtomically replaces path with data atomically: the data is written
// to a uniquely named temporary file in path's directory, synced, closed, and
// renamed over path, so an interrupted write leaves the previous contents
// intact. On any failure the temporary file is closed and removed and the
// previous target is untouched.
func WriteFileAtomic(path string, data []byte, mode os.FileMode, seams *WriteSeams) error {
	if seams == nil {
		seams = &WriteSeams{}
	}
	dir := filepath.Dir(path)

	create := seams.CreateTemp
	if create == nil {
		create = os.CreateTemp
	}
	write := seams.Write
	if write == nil {
		write = func(f *os.File, data []byte) error {
			_, err := f.Write(data)
			return err
		}
	}
	sync := seams.Sync
	if sync == nil {
		sync = func(f *os.File) error { return f.Sync() }
	}
	close := seams.Close
	if close == nil {
		close = func(f *os.File) error { return f.Close() }
	}
	rename := seams.Rename
	if rename == nil {
		rename = os.Rename
	}

	tmp, err := create(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary sibling for %s: %w", path, err)
	}
	if werr := write(tmp, data); werr != nil {
		err = werr
	} else if serr := sync(tmp); serr != nil {
		err = serr
	}
	if closeErr := close(tmp); err == nil {
		err = closeErr
	}
	if err == nil {
		err = rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("atomic write of %s: %w", path, err)
	}
	return nil
}
