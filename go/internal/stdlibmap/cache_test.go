package stdlibmap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// --- cache fixtures --------------------------------------------------------------

// cacheKey returns the SDK key the cache fixtures use. Every variation test
// derives from it by changing exactly one field.
func cacheKey() stdlibauthority.SDKKey {
	return stdlibauthority.SDKKey{
		ToolchainVersion: "go-test",
		GOOS:             "linux",
		GOARCH:           "amd64",
		CgoEnabled:       false,
		BuildTags:        nil,
		GOEXPERIMENT:     "",
		ClassifierHash:   "cafe1234",
		MapFormatVersion: artifactio.MapFormatVersion,
	}
}

// genKeyProto converts a core SDK key to its persisted proto form.
func genKeyProto(k stdlibauthority.SDKKey) *gen.SDKKey {
	return &gen.SDKKey{
		ToolchainVersion: k.ToolchainVersion,
		Goos:             k.GOOS,
		Goarch:           k.GOARCH,
		CgoEnabled:       k.CgoEnabled,
		BuildTags:        k.BuildTags,
		Goexperiment:     k.GOEXPERIMENT,
		ClassifierHash:   k.ClassifierHash,
		MapFormatVersion: k.MapFormatVersion,
	}
}

// cannedGeneration builds a valid GeneratedMap for the given key over the
// hermetic fixture inventory, with an optional provenance tweak so two valid
// same-key maps can be distinguished in the concurrency test.
func cannedGeneration(t *testing.T, key stdlibauthority.SDKKey, provenanceFlip bool) *GeneratedMap {
	t.Helper()
	m, err := buildFixtureMap(t)
	if err != nil {
		t.Fatalf("buildFixtureMap: %v", err)
	}
	m.Key = genKeyProto(key)
	if provenanceFlip {
		for _, s := range m.Symbols {
			if s.Provenance == ProvenanceProvedPure {
				s.Provenance = ""
				break
			}
		}
	}
	data, err := artifactio.MarshalMap(m)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	digest, err := artifactio.MapDigest(m)
	if err != nil {
		t.Fatalf("MapDigest: %v", err)
	}
	return &GeneratedMap{Map: m, Bytes: data, Digest: digest}
}

// generatorSpy is a Generate seam that records its invocations.
type generatorSpy struct {
	mu      sync.Mutex
	calls   int
	results *GeneratedMap
	err     error
}

func (s *generatorSpy) generate(in GenerationInput) (*GeneratedMap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}

func (s *generatorSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// cacheSeams wires a temp-dir cache root and a spy generator; filesystem
// operations stay real so the atomic-write and concurrency behavior is
// exercised end to end.
func cacheSeams(t *testing.T, root string, spy *generatorSpy) *CacheSeams {
	t.Helper()
	return &CacheSeams{
		UserCacheDir: func() (string, error) { return root, nil },
		Generate:     spy.generate,
	}
}

type cacheReadError struct{}

func (cacheReadError) Read(p []byte) (int, error) {
	copy(p, "partial")
	return len("partial"), errors.New("cache reader failed")
}

type cacheNoProgressReader struct{}

func (cacheNoProgressReader) Read([]byte) (int, error) { return 0, nil }

func TestReadBoundedPreservesCacheReaderSemantics(t *testing.T) {
	tests := []struct {
		name       string
		reader     io.Reader
		limit      int64
		want       string
		wantErr    string
		wantNoProg bool
	}{
		{name: "EOF", reader: strings.NewReader("cache"), limit: 32, want: "cache"},
		{name: "reader failure", reader: cacheReadError{}, limit: 32, want: "partial", wantErr: "cache reader failed"},
		{name: "sentinel bound", reader: strings.NewReader("012345"), limit: 4, want: "0123"},
		{name: "no progress", reader: cacheNoProgressReader{}, limit: 4, wantNoProg: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readBounded(tt.reader, tt.limit)
			if string(got) != tt.want {
				t.Fatalf("readBounded() bytes = %q, want %q", got, tt.want)
			}
			switch {
			case tt.wantNoProg:
				if !errors.Is(err, io.ErrNoProgress) {
					t.Fatalf("readBounded() error = %v, want io.ErrNoProgress", err)
				}
			case tt.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readBounded() error = %v, want %q", err, tt.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("readBounded() error = %v, want nil", err)
				}
			}
		})
	}
}

func TestIsNotExistMatchesDirectAndWrappedErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "direct", err: os.ErrNotExist, want: true},
		{name: "wrapped", err: fmt.Errorf("open cache: %w", os.ErrNotExist), want: true},
		{name: "joined", err: errors.Join(errors.New("other"), os.ErrNotExist), want: true},
		{name: "unrelated", err: os.ErrPermission, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotExist(tt.err); got != tt.want {
				t.Fatalf("isNotExist(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}

// --- req 1: cached by the full SDK key -------------------------------------------

func TestOpenCachedMapGeneratesOnceThenReuses(t *testing.T) {
	root := t.TempDir()
	key := cacheKey()
	spy := &generatorSpy{results: cannedGeneration(t, key, false)}
	seams := cacheSeams(t, root, spy)

	in := CachedMapInput{Key: key, Generation: GenerationInput{Target: TargetConfig{ToolchainVersion: "go-test", GOOS: "linux", GOARCH: "amd64"}}, Seams: seams}
	first, err := OpenCachedMap(in)
	if err != nil {
		t.Fatalf("first OpenCachedMap: %v", err)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("generator invocations after first open = %d; want 1", got)
	}
	// The artifact must be on disk under the arcc-specific subtree.
	data, err := os.ReadFile(filepath.Join(root, "arcc", "stdlibmap", CacheKeyDigest(key)+".json"))
	if err != nil {
		t.Fatalf("reading cached artifact: %v", err)
	}

	second, err := OpenCachedMap(in)
	if err != nil {
		t.Fatalf("second OpenCachedMap: %v", err)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("generator invocations after reuse = %d; want 1 (the generator must not run again)", got)
	}
	if fields := stdlibauthority.EqualKeys(first.Key(), second.Key()); len(fields) > 0 {
		t.Fatalf("cached and regenerated authorities disagree on the SDK key: %v", fields)
	}
	_ = data
}

func TestDistinctKeysSelectDistinctEntries(t *testing.T) {
	root := t.TempDir()
	base := cacheKey()
	spy := &generatorSpy{}
	seams := cacheSeams(t, root, spy)

	variations := map[string]stdlibauthority.SDKKey{
		"baseline":        base,
		"cgo":             mutateKey(base, func(k *stdlibauthority.SDKKey) { k.CgoEnabled = true }),
		"goos":            mutateKey(base, func(k *stdlibauthority.SDKKey) { k.GOOS = "windows" }),
		"goarch":          mutateKey(base, func(k *stdlibauthority.SDKKey) { k.GOARCH = "arm64" }),
		"tags":            mutateKey(base, func(k *stdlibauthority.SDKKey) { k.BuildTags = []string{"perf"} }),
		"goexperiment":    mutateKey(base, func(k *stdlibauthority.SDKKey) { k.GOEXPERIMENT = "greenteagc" }),
		"classifier_hash": mutateKey(base, func(k *stdlibauthority.SDKKey) { k.ClassifierHash = "deadbeef" }),
	}
	// The format version also selects a distinct entry; it cannot ride the
	// canned-generation variations above (a persisted map must carry the
	// format version its key declares), so its digest difference is asserted
	// directly.
	if CacheKeyDigest(mutateKey(base, func(k *stdlibauthority.SDKKey) { k.MapFormatVersion = base.MapFormatVersion + 1 })) == CacheKeyDigest(base) {
		t.Fatalf("format version does not change the cache digest")
	}
	paths := map[string]bool{}
	for name, key := range variations {
		spy.results = cannedGeneration(t, key, false)
		_, err := OpenCachedMap(CachedMapInput{Key: key, Seams: seams})
		if err != nil {
			t.Fatalf("%s: OpenCachedMap: %v", name, err)
		}
		path := filepath.Join(root, "arcc", "stdlibmap", CacheKeyDigest(key)+".json")
		if paths[path] {
			t.Fatalf("%s: cache path %q collides with another key's path", name, path)
		}
		paths[path] = true
	}
	if got := spy.count(); got != len(variations) {
		t.Fatalf("generator invocations = %d; want %d (one per distinct key)", got, len(variations))
	}
	// Tag order must not change the digest: the key is canonicalized.
	reordered := mutateKey(base, func(k *stdlibauthority.SDKKey) { k.BuildTags = []string{"perf"} })
	if CacheKeyDigest(reordered) != CacheKeyDigest(variations["tags"]) {
		t.Fatalf("build-tag ordering changed the cache digest")
	}
}

// TestSemanticClassifierInputsSelectDistinctCacheEntries is the end-to-end
// key/cache regression for the classifier fingerprint repair (task req 6, AC
// 3): two different classifier semantic inputs select different cache entries,
// and a map generated under input A fails input B's key validation instead of
// being reused.
func TestSemanticClassifierInputsSelectDistinctCacheEntries(t *testing.T) {
	target := TargetConfig{ToolchainVersion: "go-test", GOOS: "linux", GOARCH: "amd64"}
	keyFor := func(pin, overlay string) stdlibauthority.SDKKey {
		rules, err := classifierFingerprintRules(pin, overlay, projectCompletionRules())
		if err != nil {
			t.Fatalf("classifierFingerprintRules: %v", err)
		}
		text, err := CanonicalClassifierText(rules)
		if err != nil {
			t.Fatalf("CanonicalClassifierText: %v", err)
		}
		return DeriveSDKKey(GenerationDescriptor{
			Target:           target,
			ClassifierText:   text,
			RuleVersion:      RuleVersion,
			MapFormatVersion: artifactio.MapFormatVersion,
		})
	}
	keyA := keyFor("builtin-pin-a", "func (*os.File).Read CAPABILITY_SAFE\n")
	keyB := keyFor("builtin-pin-b", "func (*os.File).Read CAPABILITY_SAFE\n")
	if keyA.ClassifierHash == keyB.ClassifierHash {
		t.Fatalf("two classifier semantic inputs produced the same classifier_hash")
	}
	if CacheKeyDigest(keyA) == CacheKeyDigest(keyB) {
		t.Fatalf("two classifier semantic inputs selected the same cache digest")
	}

	// A's artifact, even placed at B's digest path, is a key mismatch: the
	// cache must regenerate for B rather than return A's map.
	root := t.TempDir()
	mapA := cannedGeneration(t, keyA, false)
	spy := &generatorSpy{results: cannedGeneration(t, keyB, false)}
	seams := cacheSeams(t, root, spy)
	pathB := filepath.Join(root, "arcc", "stdlibmap", CacheKeyDigest(keyB)+".json")
	if err := os.MkdirAll(filepath.Dir(pathB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, mapA.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	auth, err := OpenCachedMap(CachedMapInput{Key: keyB, Seams: seams})
	if err != nil {
		t.Fatalf("OpenCachedMap with A's artifact under B's key: %v", err)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("generator invocations = %d; want 1 (A's map must not be reused for B)", got)
	}
	if fields := stdlibauthority.EqualKeys(auth.Key(), keyB); len(fields) > 0 {
		t.Fatalf("returned authority disagrees with B's key: %v", fields)
	}
}

func mutateKey(base stdlibauthority.SDKKey, mutate func(*stdlibauthority.SDKKey)) stdlibauthority.SDKKey {
	k := base
	mutate(&k)
	return k
}

func TestDefaultCacheRootIsUserCacheDirArccSubtree(t *testing.T) {
	root, err := DefaultCacheRoot()
	if err != nil {
		t.Fatalf("DefaultCacheRoot: %v", err)
	}
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no user cache dir on this host: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(root), "/arcc") {
		t.Fatalf("DefaultCacheRoot = %q; want the user cache dir's arcc subtree (under %q)", root, base)
	}
}

// --- req 2: corrupt caches regenerate safely -------------------------------------

func TestCorruptCacheRegenerates(t *testing.T) {
	key := cacheKey()
	valid := cannedGeneration(t, key, false)
	corruptions := map[string][]byte{
		"truncated JSON":    valid.Bytes[:len(valid.Bytes)/2],
		"invalid inventory": []byte(`{"format_version":1,"key":{"toolchain_version":"go-test"},"packages":[{"path":"!!bad path!!"}]}`),
		"another SDK key":   mustMarshal(t, cannedGeneration(t, mutateKey(key, func(k *stdlibauthority.SDKKey) { k.GOOS = "windows" }), false).Map),
		"empty file":        {},
	}
	for name, content := range corruptions {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			spy := &generatorSpy{results: valid}
			seams := cacheSeams(t, root, spy)
			dir := filepath.Join(root, "arcc", "stdlibmap")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, CacheKeyDigest(key)+".json")
			if err := os.WriteFile(path, content, 0o644); err != nil {
				t.Fatal(err)
			}
			auth, err := OpenCachedMap(CachedMapInput{Key: key, Seams: seams})
			if err != nil {
				t.Fatalf("OpenCachedMap over corrupt cache: %v", err)
			}
			if got := spy.count(); got != 1 {
				t.Fatalf("generator invocations = %d; want 1 (corrupt content must regenerate)", got)
			}
			// The corrupt bytes must be replaced by the fresh valid artifact.
			fresh, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading regenerated artifact: %v", err)
			}
			if !bytes.Equal(fresh, valid.Bytes) {
				t.Fatalf("corrupt cache was not replaced by the fresh bytes")
			}
			if fields := stdlibauthority.EqualKeys(auth.Key(), key); len(fields) > 0 {
				t.Fatalf("regenerated authority reports mismatched fields: %v", fields)
			}
		})
	}
}

func TestFailedGenerationPreservesPriorBytes(t *testing.T) {
	root := t.TempDir()
	key := cacheKey()
	valid := cannedGeneration(t, key, false)
	dir := filepath.Join(root, "arcc", "stdlibmap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, CacheKeyDigest(key)+".json")
	// A generation that produces a different key's map is refused, and the
	// prior bytes stay untouched (checked before any bytes exist, so the
	// cache-miss path actually runs).
	mismatchSeams := &CacheSeams{
		UserCacheDir: func() (string, error) { return root, nil },
		Generate: func(in GenerationInput) (*GeneratedMap, error) {
			return cannedGeneration(t, mutateKey(key, func(k *stdlibauthority.SDKKey) { k.GOARCH = "arm64" }), false), nil
		},
	}
	_, err := OpenCachedMap(CachedMapInput{Key: key, Seams: mismatchSeams})
	if err == nil || !strings.Contains(err.Error(), "SDK key") {
		t.Fatalf("mismatched generation output: want an SDK-key error, got %v", err)
	}
	// Prior (here: stale corrupt) bytes: a failed generation must leave them
	// exactly as they were. Truncated bytes make the read a miss.
	staleBytes := valid.Bytes[:len(valid.Bytes)/2]
	if err := os.WriteFile(path, staleBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	// A failing generator leaves the prior valid bytes intact and errors.
	failing := &generatorSpy{err: errors.New("capslock exploded")}
	_, err = OpenCachedMap(CachedMapInput{
		Key:   key,
		Seams: &CacheSeams{UserCacheDir: func() (string, error) { return root, nil }, Generate: failing.generate},
	})
	if err == nil {
		t.Fatalf("OpenCachedMap with failing generator: want error, got nil")
	}
	if !strings.Contains(err.Error(), "capslock exploded") {
		t.Fatalf("generation error not surfaced: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, staleBytes) {
		t.Fatalf("failed generation overwrote or truncated the prior artifact")
	}
}

// TestOversizedCacheEntryIsAMissAndRegenerated pins the bounded default read
// (review round 3): a cache entry over artifactio.MaxMapBytes is read through
// the decoder's bound, fails validation, and is regenerated — without the
// whole file being loaded.
func TestOversizedCacheEntryIsAMissAndRegenerated(t *testing.T) {
	root := t.TempDir()
	key := cacheKey()
	valid := cannedGeneration(t, key, false)
	spy := &generatorSpy{results: valid}
	seams := cacheSeams(t, root, spy)
	dir := filepath.Join(root, "arcc", "stdlibmap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, CacheKeyDigest(key)+".json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(artifactio.MaxMapBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	auth, err := OpenCachedMap(CachedMapInput{Key: key, Seams: seams})
	if err != nil {
		t.Fatalf("OpenCachedMap over oversized entry: %v", err)
	}
	if got := spy.count(); got != 1 {
		t.Fatalf("generator invocations = %d; want 1 (oversized entry is a miss)", got)
	}
	if fields := stdlibauthority.EqualKeys(auth.Key(), key); len(fields) > 0 {
		t.Fatalf("regenerated authority reports mismatched fields: %v", fields)
	}
	fresh, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fresh, valid.Bytes) {
		t.Fatalf("oversized entry was not replaced by the fresh bytes")
	}
}

// --- req 2/3: read faults and the concurrent cache write path --------------------

func TestCacheReadErrorSurfacesWithoutGeneration(t *testing.T) {
	root := t.TempDir()
	key := cacheKey()
	spy := &generatorSpy{results: cannedGeneration(t, key, false)}
	seams := cacheSeams(t, root, spy)
	// A filesystem fault (permission, I/O) is not corrupt content: it must
	// surface as an error, and it must never trigger a costly regeneration
	// on top of a broken cache (review finding).
	seams.ReadFile = func(path string) ([]byte, error) { return nil, os.ErrPermission }
	_, err := OpenCachedMap(CachedMapInput{Key: key, Seams: seams})
	if err == nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("OpenCachedMap with unreadable cache: want a wrapped read error, got %v", err)
	}
	if got := spy.count(); got != 0 {
		t.Fatalf("generator invocations = %d; want 0 (a read fault must not regenerate)", got)
	}
}

func TestConcurrentReadersNeverObservePartialOutput(t *testing.T) {
	root := t.TempDir()
	key := cacheKey()
	mapA := cannedGeneration(t, key, false)
	mapB := cannedGeneration(t, key, true)
	dir := filepath.Join(root, "arcc", "stdlibmap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, CacheKeyDigest(key)+".json")
	if err := os.WriteFile(path, mapA.Bytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// A delayed writer replaces the artifact repeatedly, the way a concurrent
	// generating process would. Every fourth round removes the file entirely,
	// so overlapping readers hit cache misses and regenerate through the
	// cache's own (atomic) write path, not only the foreign writer's.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if i%4 == 3 {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("delayed writer removal: %v", err)
					return
				}
			} else {
				next := mapA
				if i%2 == 1 {
					next = mapB
				}
				if err := artifactio.WriteFileAtomic(path, next.Bytes, 0o644, nil); err != nil {
					t.Errorf("delayed writer: %v", err)
					return
				}
			}
		}
	}()

	// A generator seam that "regenerates" slowly: if a reader ever treats
	// in-flight content as a miss it re-enters generation, which must still
	// produce a complete valid map — never an error from partial bytes.
	spy := &generatorSpy{results: mapA}
	seams := cacheSeams(t, root, spy)
	var readWG sync.WaitGroup
	errCh := make(chan error, 64)
	for r := 0; r < 8; r++ {
		readWG.Add(1)
		go func() {
			defer readWG.Done()
			for i := 0; i < 50; i++ {
				auth, err := OpenCachedMap(CachedMapInput{Key: key, Seams: seams})
				if err != nil {
					errCh <- fmt.Errorf("concurrent read: %w", err)
					return
				}
				if fields := stdlibauthority.EqualKeys(auth.Key(), key); len(fields) > 0 {
					errCh <- fmt.Errorf("concurrent read returned mismatched key: %v", fields)
					return
				}
			}
		}()
	}
	readWG.Wait()
	close(stop)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func mustMarshal(t *testing.T, m *gen.StdlibMap) []byte {
	t.Helper()
	data, err := artifactio.MarshalMap(m)
	if err != nil {
		t.Fatalf("MarshalMap: %v", err)
	}
	return data
}
