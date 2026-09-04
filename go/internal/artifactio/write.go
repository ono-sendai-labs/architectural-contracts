package artifactio

import (
	"errors"
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
	Chmod      func(path string, mode os.FileMode) error
	Rename     func(oldPath, newPath string) error
	Remove     func(path string) error
}

// WriteFileAtomic replaces path with data atomically: the data is written
// to a uniquely named temporary file in path's directory, synced, closed,
// given the requested permission mode, and renamed over path, so an
// interrupted write leaves the previous contents intact. On any failure the
// temporary file is closed and removed — a removal failure is surfaced
// together with the original error — and the previous target is untouched.
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
	chmod := seams.Chmod
	if chmod == nil {
		chmod = os.Chmod
	}
	rename := seams.Rename
	if rename == nil {
		rename = os.Rename
	}
	remove := seams.Remove
	if remove == nil {
		remove = os.Remove
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
	if cerr := close(tmp); err == nil {
		err = cerr
	}
	if err == nil {
		err = chmod(tmp.Name(), mode)
	}
	if err == nil {
		err = rename(tmp.Name(), path)
	}
	if err != nil {
		// Failure cleanup: the descriptor is already closed above; drop the
		// temporary sibling and surface a removal failure alongside the
		// original error rather than discarding it.
		if rerr := remove(tmp.Name()); rerr != nil {
			err = errors.Join(err, fmt.Errorf("remove temporary sibling %s: %w", tmp.Name(), rerr))
		}
		return fmt.Errorf("atomic write of %s: %w", path, err)
	}
	return nil
}
