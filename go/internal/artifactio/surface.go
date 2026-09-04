package artifactio

import (
	"fmt"
	"io"
	"slices"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// MarshalSurface returns the canonical bytes of a surface manifest. The input
// message is defensively copied before normalization, so the caller's message
// is never mutated. Duplicate entries, contradictory classifications, and
// other non-canonical states are rejected rather than silently repaired.
func MarshalSurface(m *gen.SurfaceManifest) ([]byte, error) {
	canonical, err := normalizeSurface(m)
	if err != nil {
		return nil, err
	}
	return canonicalJSON(canonical)
}

// DecodeSurface reads at most MaxSurfaceBytes bytes from r, parses them as a
// surface manifest, and validates its semantic invariants. Unknown JSON fields
// are ignored (forward-compatible minor additions); malformed JSON, an
// unsupported major format_version, or a non-canonical semantic state fails
// with a specific error.
func DecodeSurface(r io.Reader) (*gen.SurfaceManifest, error) {
	data, err := boundedRead(r, MaxSurfaceBytes)
	if err != nil {
		return nil, err
	}
	var m gen.SurfaceManifest
	if err := decodeInto(data, &m); err != nil {
		return nil, err
	}
	if err := validateSurface(m.FormatVersion, m.Packages, m.Symbols, m.Authority); err != nil {
		return nil, err
	}
	return &m, nil
}

// SurfaceDigest returns the lowercase hex SHA-256 digest of the canonical
// encoding of m. Hashing a decoded artifact operates on its re-canonicalized
// representation, not on the source's whitespace or entry ordering.
func SurfaceDigest(m *gen.SurfaceManifest) (string, error) {
	canonical, err := normalizeSurface(m)
	if err != nil {
		return "", err
	}
	data, err := canonicalJSON(canonical)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

// normalizeSurface validates m and returns a canonically sorted clone.
func normalizeSurface(m *gen.SurfaceManifest) (*gen.SurfaceManifest, error) {
	if m == nil {
		return nil, fmt.Errorf("surface manifest is nil")
	}
	if err := validateSurface(m.FormatVersion, m.Packages, m.Symbols, m.Authority); err != nil {
		return nil, err
	}
	c := proto.Clone(m).(*gen.SurfaceManifest)
	slices.Sort(c.Packages)
	sortStrings(c.Symbols)
	if c.Authority != nil {
		c.Authority.DeclaredAuthority = slices.Clone(c.Authority.DeclaredAuthority)
		sortStrings(c.Authority.DeclaredAuthority)
	}
	return c, nil
}

// validateSurface enforces the surface's semantic invariants. It reads only
// the caller's values and mutates nothing.
func validateSurface(formatVersion int32, packages, symbols []string, authority *gen.AuthorityDeclaration) error {
	if formatVersion != SurfaceFormatVersion {
		return fmt.Errorf("surface %w: got %d, supported: %d", ErrUnsupportedVersion, formatVersion, SurfaceFormatVersion)
	}
	if err := rejectDuplicates("surface package", packages); err != nil {
		return err
	}
	for _, pkg := range packages {
		if err := validPackagePath(pkg); err != nil {
			return fmt.Errorf("surface package %q: %w", pkg, err)
		}
	}
	if err := rejectDuplicates("surface symbol", symbols); err != nil {
		return err
	}
	for _, sym := range symbols {
		if _, err := symbol.Parse(sym); err != nil {
			return fmt.Errorf("surface symbol %q: %w", sym, err)
		}
	}
	return validateAuthority(authority)
}

// validateAuthority rejects the forbidden empty-set spelling of UNKNOWN and
// any UNKNOWN carrying declared capabilities, plus unknown or duplicate
// capability names on DECLARED authorities.
func validateAuthority(a *gen.AuthorityDeclaration) error {
	if a == nil {
		return nil
	}
	switch a.Authority {
	case gen.Authority_UNKNOWN:
		if len(a.DeclaredAuthority) > 0 {
			return fmt.Errorf("authority is UNKNOWN but declared_authority is non-empty (%s); declared_authority must be empty when authority is UNKNOWN", a.DeclaredAuthority[0])
		}
		return nil
	case gen.Authority_DECLARED:
		if err := rejectDuplicates("declared capability", a.DeclaredAuthority); err != nil {
			return err
		}
		for _, cap := range a.DeclaredAuthority {
			if !manifest.KnownCapabilities[cap] {
				return fmt.Errorf("declared capability %q is not a known capability", cap)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown authority value: %d", int32(a.Authority))
	}
}

// boundedRead reads at most max bytes plus one sentinel byte, so an oversized
// input fails before any unbounded read or allocation.
func boundedRead(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("artifact size %d exceeds the %d byte limit", len(data), max)
	}
	return data, nil
}

// decodeInto parses canonical or non-canonical (but well-formed) artifact
// bytes, ignoring unknown JSON fields for forward compatibility.
func decodeInto(data []byte, msg proto.Message) error {
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, msg); err != nil {
		return fmt.Errorf("parse artifact: %w", err)
	}
	return nil
}

// sortStrings sorts in place and compacts nothing: duplicates are the caller's
// error to reject.
func sortStrings(s []string) {
	slices.Sort(s)
}

// rejectDuplicates reports a duplicate entry in a repeated string collection.
func rejectDuplicates(kind string, values []string) error {
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if seen[v] {
			return fmt.Errorf("duplicate %s entry: %q", kind, v)
		}
		seen[v] = true
	}
	return nil
}

// validPackagePath rejects empty package paths; the SymbolID grammar performs
// the authoritative per-symbol check.
func validPackagePath(pkg string) error {
	if pkg == "" {
		return fmt.Errorf("package path must not be empty")
	}
	return nil
}
