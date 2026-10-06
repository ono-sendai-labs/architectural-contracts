package stdlibmap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/atomicfile"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap/internal/hosttoolchain"
)

// cacheKeyDigest returns the cache filename stem for an SDK key: the
// hex-encoded SHA-256 digest of a canonical, NUL-separated encoding of every
// key field (tags sorted), so the digest is collision-resistant and total
// over the complete key (task req 1). Two keys that differ in any field —
// toolchain version, GOOS, GOARCH, cgo, build tags, GOEXPERIMENT,
// classifier_hash or format version — digest differently.
func cacheKeyDigest(k stdlibauthority.SDKKey) string {
	tags := append([]string(nil), k.BuildTags...)
	sortStringsInPlace(tags)
	var material bytes.Buffer
	material.WriteString(k.ToolchainVersion)
	material.WriteByte(0)
	material.WriteString(k.GOOS)
	material.WriteByte(0)
	material.WriteString(k.GOARCH)
	material.WriteByte(0)
	material.WriteString(strconv.FormatBool(k.CgoEnabled))
	material.WriteByte(0)
	for _, tag := range tags {
		material.WriteString(tag)
		material.WriteByte(0)
	}
	material.WriteString(k.GOEXPERIMENT)
	material.WriteByte(0)
	material.WriteString(k.ClassifierHash)
	material.WriteByte(0)
	material.WriteString(strconv.FormatInt(int64(k.MapFormatVersion), 10))
	sum := sha256.Sum256(material.Bytes())
	return hex.EncodeToString(sum[:])
}

func sortStringsInPlace(s []string) { sort.Strings(s) }

// defaultCacheRoot returns the native cache root: the platform user cache
// directory's arcc-specific subtree (task req 1). It is the parent of the
// stdlibmap artifact directory; the on-disk layout is
// `<root>/stdlibmap/<cacheKeyDigest>.json`.
func defaultCacheRoot() (string, error) {
	base, err := hosttoolchain.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving the native stdlib-map cache root: %w", err)
	}
	return filepath.Join(base, "arcc"), nil
}

// cacheArtifactPath renders the artifact path for a key under root.
func cacheArtifactPath(root string, key stdlibauthority.SDKKey) string {
	return filepath.Join(root, "stdlibmap", cacheKeyDigest(key)+".json")
}

// cacheSeams injects the native cache's environment and filesystem
// operations so tests neither mutate the real user cache nor run a real SDK
// generation (task req 4). A nil field uses the real operation; production
// callers pass nil seams.
type cacheSeams struct {
	// UserCacheDir resolves the default cache root (defaultCacheRoot's base).
	UserCacheDir func() (string, error)
	// ReadFile reads the artifact bytes at a path (hosttoolchain.OpenFile plus
	// a read bounded by artifactio.MaxMapBytes, so an oversized cache entry
	// cannot allocate unbounded memory before the decoder rejects it).
	ReadFile func(path string) ([]byte, error)
	// MkdirAll creates the artifact directory (hosttoolchain.MkdirAll).
	MkdirAll func(path string, perm fs.FileMode) error
	// WriteAtomic replaces the artifact path with bytes atomically
	// (atomicfile.WriteFileAtomic semantics).
	WriteAtomic func(path string, data []byte, mode fs.FileMode) error
	// Generate runs map generation on a cache miss (stdlibmap.Generate).
	Generate func(in GenerationInput) (*GeneratedMap, error)
}

func (s *cacheSeams) userCacheDir() func() (string, error) {
	if s != nil && s.UserCacheDir != nil {
		return s.UserCacheDir
	}
	return hosttoolchain.UserCacheDir
}

func (s *cacheSeams) readFile() func(string) ([]byte, error) {
	if s != nil && s.ReadFile != nil {
		return s.ReadFile
	}
	// The bounded default: at most MaxMapBytes plus one sentinel byte is
	// read, so an oversized or runaway cache entry is rejected by the
	// decoder's own bound without the whole file ever being loaded (DR-15;
	// review-round-3 finding).
	return func(path string) ([]byte, error) {
		f, err := hosttoolchain.OpenFile(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return readBounded(f, int64(artifactio.MaxMapBytes)+1)
	}
}

// readBounded copies up to limit bytes and stops at that boundary without
// asking the source for an additional byte. The extra byte used by the cache
// reader is the decoder's oversize sentinel: an oversized artifact remains a
// cache miss, matching the former LimitReader behavior. The explicit loop
// preserves io.ReadAll's EOF and reader-error results while bounding no-progress
// readers instead of allowing them to loop forever; it also keeps this member
// package free of the stdlib map's UNANALYZED io.ReadAll reference.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("reading cached stdlib map: negative byte limit %d", limit)
	}
	const chunkSize = 32 * 1024
	capacity := int64(chunkSize)
	if limit < capacity {
		capacity = limit
	}
	data := make([]byte, 0, int(capacity))
	buf := make([]byte, chunkSize)
	noProgress := 0
	for int64(len(data)) < limit {
		remaining := limit - int64(len(data))
		readBuf := buf
		if remaining < int64(len(readBuf)) {
			readBuf = readBuf[:int(remaining)]
		}
		n, err := r.Read(readBuf)
		if n < 0 || n > len(readBuf) {
			return data, fmt.Errorf("reading cached stdlib map: invalid reader count %d", n)
		}
		if n > 0 {
			data = append(data, readBuf[:n]...)
			noProgress = 0
		} else if err == nil {
			noProgress++
			if noProgress >= 100 {
				return data, fmt.Errorf("reading cached stdlib map: %w", io.ErrNoProgress)
			}
		}
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return data, err
		}
	}
	return data, nil
}

func (s *cacheSeams) mkdirAll() func(string, fs.FileMode) error {
	if s != nil && s.MkdirAll != nil {
		return s.MkdirAll
	}
	return hosttoolchain.MkdirAll
}

func (s *cacheSeams) writeAtomic() func(string, []byte, fs.FileMode) error {
	if s != nil && s.WriteAtomic != nil {
		return s.WriteAtomic
	}
	return func(path string, data []byte, mode fs.FileMode) error {
		return atomicfile.WriteFileAtomic(path, data, mode, nil)
	}
}

func (s *cacheSeams) generate() func(GenerationInput) (*GeneratedMap, error) {
	if s != nil && s.Generate != nil {
		return s.Generate
	}
	return Generate
}

func openCachedMap(in CachedMapInput) (stdlibauthority.StdlibAuthority, error) {
	seams := in.Seams
	root := in.Root
	if root == "" {
		base, err := seams.userCacheDir()()
		if err != nil {
			return nil, fmt.Errorf("resolving the native stdlib-map cache root: %w", err)
		}
		root = filepath.Join(base, "arcc")
	}
	path := cacheArtifactPath(root, in.Key)

	if data, err := seams.readFile()(path); err == nil {
		auth, openErr := artifactio.OpenStdlibMap(bytes.NewReader(data), &in.Key)
		if openErr == nil {
			return auth, nil
		}
		// Corrupt or mismatched cache content is a miss: fall through to
		// regeneration (task req 3).
	} else if !isNotExist(err) {
		// Only absence is a miss. A permission failure, I/O error, or other
		// filesystem fault is not corrupt content — it is a broken cache, and
		// silently regenerating on top of it would hide the fault (review:
		// cache read errors must surface, not trigger generation).
		return nil, fmt.Errorf("reading the cached stdlib map %s: %w", path, err)
	}

	// A miss generates through Generate; the fresh bytes are validated
	// against the requested key before anything is written, so a generator
	// that produces a different key's map fails closed instead of poisoning
	// the cache entry under Key's digest.
	out, err := seams.generate()(in.Generation)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map for the cache: %w", err)
	}
	auth, err := artifactio.OpenStdlibMap(bytes.NewReader(out.Bytes), &in.Key)
	if err != nil {
		return nil, fmt.Errorf("validating the freshly generated stdlib map: %w", err)
	}
	dir := filepath.Dir(path)
	if err := seams.mkdirAll()(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating the stdlib-map cache directory %s: %w", dir, err)
	}
	if err := seams.writeAtomic()(path, out.Bytes, 0o644); err != nil {
		return nil, fmt.Errorf("storing the stdlib map in the cache: %w", err)
	}
	return auth, nil
}

func isNotExist(err error) bool {
	return isNotExistDepth(err, 0)
}

// isNotExistDepth preserves errors.Is-style matching without adding a direct
// reference to errors.Is to the checked stdlibmap member package. The stdlib
// map intentionally classifies errors.Is as UNANALYZED; this cache still needs
// its prior recursive missing-artifact behavior for wrapped filesystem errors,
// so the bounded walk handles both single- and multi-error Unwrap contracts.
func isNotExistDepth(err error, depth int) bool {
	if err == nil || depth > 100 {
		return false
	}
	if hosttoolchain.IsNotExist(err) || err == fs.ErrNotExist {
		return true
	}
	switch unwrapped := err.(type) {
	case interface{ Is(error) bool }:
		if unwrapped.Is(fs.ErrNotExist) {
			return true
		}
		return isNotExistUnwrap(err, depth)
	case interface{ Unwrap() error }:
		return isNotExistDepth(unwrapped.Unwrap(), depth+1)
	case interface{ Unwrap() []error }:
		for _, child := range unwrapped.Unwrap() {
			if isNotExistDepth(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func isNotExistUnwrap(err error, depth int) bool {
	switch unwrapped := err.(type) {
	case interface{ Unwrap() error }:
		return isNotExistDepth(unwrapped.Unwrap(), depth+1)
	case interface{ Unwrap() []error }:
		for _, child := range unwrapped.Unwrap() {
			if isNotExistDepth(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func writeArtifactAtomic(path string, data []byte, mode fs.FileMode) error {
	return atomicfile.WriteFileAtomic(path, data, mode, nil)
}

func decodeMapArtifact(r io.Reader) (*gen.StdlibMap, error) {
	return artifactio.DecodeMap(r)
}

func nativeTargetConfig() (TargetConfig, error) {
	version, err := nativeToolchainVersion(context.Background())
	if err != nil {
		return TargetConfig{}, err
	}
	out, err := hosttoolchain.GoEnv(context.Background(), "GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT")
	if err != nil {
		return TargetConfig{}, fmt.Errorf("discovering the target build configuration with `go env`: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) != 3 && len(fields) != 4 {
		return TargetConfig{}, fmt.Errorf("discovering the target build configuration: `go env` returned %d values, want GOOS, GOARCH, CGO_ENABLED and GOEXPERIMENT", len(fields))
	}
	goexperiment := ""
	if len(fields) == 4 {
		goexperiment = fields[3]
	}
	return TargetConfig{
		ToolchainVersion: version,
		GOOS:             fields[0],
		GOARCH:           fields[1],
		CgoEnabled:       fields[2] == "1",
		GOEXPERIMENT:     goexperiment,
	}, nil
}
