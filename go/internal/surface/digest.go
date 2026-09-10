package surface

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"strconv"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

// DigestForSources computes the producer digest for an already assembled
// surface and a byte-only source/manifest input set. The native dependency
// consumer uses this same framing without loading or parsing Go packages, so
// its freshness result is comparable to the digest emitted by Derive.
func DigestForSources(sources []SourceFile, manifestBytes []byte, m *gen.SurfaceManifest) (string, error) {
	if m == nil {
		return "", fmt.Errorf("surface digest: manifest is missing")
	}
	return computeDigest(sources, manifestBytes, m)
}

// computeDigest returns the lowercase hex SHA-256 over the semantic producer
// inputs (DR-03): member source bytes sorted by canonical path, the component
// manifest bytes, the surface format version, the namespace, the SDK key and
// the producer version. Length-prefixed field framing makes the encoding
// unambiguous, and sorting the sources before hashing makes the result
// independent of file enumeration order.
func computeDigest(sources []SourceFile, manifestBytes []byte, m *gen.SurfaceManifest) (string, error) {
	h := sha256.New()
	writeField := func(b []byte) {
		var lenBuf [8]byte
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(b)))
		h.Write(lenBuf[:])
		h.Write(b)
	}

	sorted := slices.Clone(sources)
	slices.SortFunc(sorted, func(a, b SourceFile) int { return slices.Compare([]string{a.Path}, []string{b.Path}) })
	for _, src := range sorted {
		writeField([]byte(src.Path))
		writeField(src.Bytes)
	}
	writeField(manifestBytes)
	writeField([]byte(strconv.Itoa(int(m.FormatVersion))))
	writeField([]byte(m.Namespace))
	if m.SdkKey == nil {
		return "", fmt.Errorf("surface digest: SDK key is missing")
	}
	writeField(sdkKeyDigestBytes(m.SdkKey))
	writeField([]byte(m.ProducerVersion))

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// sdkKeyDigestBytes renders the SDK key's fields in a fixed order with sorted
// build tags, so the key contributes identically regardless of how the caller
// enumerated it.
func sdkKeyDigestBytes(k *gen.SDKKey) []byte {
	tags := slices.Clone(k.BuildTags)
	slices.Sort(tags)
	return []byte(k.ToolchainVersion + "\x00" + k.Goos + "\x00" + k.Goarch + "\x00" +
		boolByte(k.CgoEnabled) + "\x00" + joinNull(tags) + "\x00" + k.Goexperiment + "\x00" +
		k.ClassifierHash + "\x00" + strconv.Itoa(int(k.MapFormatVersion)))
}

func boolByte(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func joinNull(values []string) string {
	out := ""
	for i, v := range values {
		if i > 0 {
			out += "\x00"
		}
		out += v
	}
	return out
}
