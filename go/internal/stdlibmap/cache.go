package stdlibmap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// RuleVersion is the production version of the generation classification
// rules (GenerationInput.RuleVersion): bumped whenever the rules' semantics
// change, so the stamped classifier_hash cannot mask a rule change (I2).
const RuleVersion = "stdlibmap-generation-rules-v2"

// CacheKeyDigest returns the cache filename stem for an SDK key: the
// hex-encoded SHA-256 digest of a canonical, NUL-separated encoding of every
// key field (tags sorted), so the digest is collision-resistant and total
// over the complete key (task req 1). Two keys that differ in any field —
// toolchain version, GOOS, GOARCH, cgo, build tags, GOEXPERIMENT,
// classifier_hash or format version — digest differently.
func CacheKeyDigest(k stdlibauthority.SDKKey) string {
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

// DefaultCacheRoot returns the native cache root: the platform user cache
// directory's arcc-specific subtree (task req 1). It is the parent of the
// stdlibmap artifact directory; the on-disk layout is
// `<root>/stdlibmap/<CacheKeyDigest>.json`.
func DefaultCacheRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving the native stdlib-map cache root: %w", err)
	}
	return filepath.Join(base, "arcc"), nil
}

// cacheArtifactPath renders the artifact path for a key under root.
func cacheArtifactPath(root string, key stdlibauthority.SDKKey) string {
	return filepath.Join(root, "stdlibmap", CacheKeyDigest(key)+".json")
}

// CacheSeams injects the native cache's environment and filesystem
// operations so tests neither mutate the real user cache nor run a real SDK
// generation (task req 4). A nil field uses the real operation; production
// callers pass nil seams.
type CacheSeams struct {
	// UserCacheDir resolves the default cache root (DefaultCacheRoot's base).
	UserCacheDir func() (string, error)
	// ReadFile reads the artifact bytes at a path (os.Open plus a read
	// bounded by artifactio.MaxMapBytes, so an oversized cache entry cannot
	// allocate unbounded memory before the decoder rejects it).
	ReadFile func(path string) ([]byte, error)
	// MkdirAll creates the artifact directory (os.MkdirAll).
	MkdirAll func(path string, perm os.FileMode) error
	// WriteAtomic replaces the artifact path with bytes atomically
	// (artifactio.WriteFileAtomic semantics).
	WriteAtomic func(path string, data []byte, mode os.FileMode) error
	// Generate runs map generation on a cache miss (stdlibmap.Generate).
	Generate func(in GenerationInput) (*GeneratedMap, error)
}

func (s *CacheSeams) userCacheDir() func() (string, error) {
	if s != nil && s.UserCacheDir != nil {
		return s.UserCacheDir
	}
	return os.UserCacheDir
}

func (s *CacheSeams) readFile() func(string) ([]byte, error) {
	if s != nil && s.ReadFile != nil {
		return s.ReadFile
	}
	// The bounded default: at most MaxMapBytes plus one sentinel byte is
	// read, so an oversized or runaway cache entry is rejected by the
	// decoder's own bound without the whole file ever being loaded (DR-15;
	// review-round-3 finding).
	return func(path string) ([]byte, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, artifactio.MaxMapBytes+1))
	}
}

func (s *CacheSeams) mkdirAll() func(string, os.FileMode) error {
	if s != nil && s.MkdirAll != nil {
		return s.MkdirAll
	}
	return os.MkdirAll
}

func (s *CacheSeams) writeAtomic() func(string, []byte, os.FileMode) error {
	if s != nil && s.WriteAtomic != nil {
		return s.WriteAtomic
	}
	return func(path string, data []byte, mode os.FileMode) error {
		return artifactio.WriteFileAtomic(path, data, mode, nil)
	}
}

func (s *CacheSeams) generate() func(GenerationInput) (*GeneratedMap, error) {
	if s != nil && s.Generate != nil {
		return s.Generate
	}
	return Generate
}

// CachedMapInput is one native-cache open request: the target SDK key the
// map must match (N3) and, for a cache miss, the full generation
// configuration. Root selects an explicit cache root (tests, Bazel-style
// pinned locations); the empty value resolves the platform user cache dir's
// arcc subtree.
type CachedMapInput struct {
	// Key is the requested target SDK configuration. The opened authority is
	// guaranteed to agree with it on every field, classifier_hash and
	// format version included.
	Key stdlibauthority.SDKKey
	// Root is an explicit cache root; empty means DefaultCacheRoot.
	Root string
	// Generation is the generation configuration used on a cache miss; the
	// freshly generated bytes are validated against Key before they are
	// written, so a generation producing a different key fails closed.
	Generation GenerationInput
	// Seams inject the cache's environment and filesystem operations.
	Seams *CacheSeams
}

// OpenCachedMap returns the stdlib authority map for the requested SDK key,
// generating and caching it on demand (task reqs 1–3, design "native
// on-demand generator with cache"):
//
//   - A cache hit is validated end to end against the requested key —
//     decode, semantic validation, and exact SDK-key agreement — via
//     artifactio.OpenStdlibMap. Truncated, malformed, invalid, or
//     mismatched content is treated as a miss, never as an answer.
//   - A miss generates through Generate and validates the fresh bytes the
//     same way before they are written, then replaces the artifact
//     atomically, so concurrent readers only ever observe a complete valid
//     map and a failed generation leaves any previous artifact intact.
func OpenCachedMap(in CachedMapInput) (stdlibauthority.StdlibAuthority, error) {
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
	} else if !errors.Is(err, os.ErrNotExist) {
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

// WriteArtifactAtomic replaces path with persisted stdlib-map artifact bytes
// using the artifact I/O boundary's atomic-write semantics (Step 3): a synced
// temporary sibling renamed over the target, so an interrupted write leaves
// the previous bytes intact. This re-export exists because the cli component
// consumes the map workflow through this component: artifactio is a
// package-surface wrapper over foreign protobuf-runtime members, which the
// legacy native dependency resolution cannot load beneath another component's
// root (design §schema, migration interval).
func WriteArtifactAtomic(path string, data []byte, mode os.FileMode) error {
	return artifactio.WriteFileAtomic(path, data, mode, nil)
}

// DecodeMapArtifact reads and validates a persisted stdlib map from r: the
// bounded decode, the semantic invariants, and the format-version check, in
// one call (same re-export rationale as WriteArtifactAtomic).
func DecodeMapArtifact(r io.Reader) (*gen.StdlibMap, error) {
	return artifactio.DecodeMap(r)
}

// NativeTargetConfig discovers the host toolchain's current target
// configuration for native map generation (task req 5's default native
// discovery): the toolchain version via `go env GOVERSION` and GOOS, GOARCH,
// CGO_ENABLED and GOEXPERIMENT via one `go env` call. Callers with an
// explicit target configuration (the later Bazel rule) construct
// TargetConfig directly instead.
func NativeTargetConfig() (TargetConfig, error) {
	version, err := NativeToolchainVersion(context.Background())
	if err != nil {
		return TargetConfig{}, err
	}
	out, err := exec.Command("go", "env", "GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT").Output()
	if err != nil {
		return TargetConfig{}, fmt.Errorf("discovering the target build configuration with `go env`: %w", err)
	}
	fields := strings.Fields(string(out))
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
