package artifactio

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
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
	if err := validateSurface(&m); err != nil {
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
	if err := validateSurface(m); err != nil {
		return nil, err
	}
	c := proto.Clone(m).(*gen.SurfaceManifest)
	slices.Sort(c.Packages)
	sortStrings(c.Symbols)
	if c.Authority != nil {
		c.Authority.DeclaredAuthority = slices.Clone(c.Authority.DeclaredAuthority)
		sortStrings(c.Authority.DeclaredAuthority)
	}
	if c.SdkKey != nil {
		c.SdkKey.BuildTags = slices.Clone(c.SdkKey.BuildTags)
		slices.Sort(c.SdkKey.BuildTags)
	}
	return c, nil
}

// validateSurface enforces the surface's semantic invariants. It reads only
// the caller's values and mutates nothing.
func validateSurface(m *gen.SurfaceManifest) error {
	switch m.InterfaceStyle {
	case gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED, gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE:
	default:
		return fmt.Errorf("unknown surface interface_style: %d", int32(m.InterfaceStyle))
	}
	if m.InterfaceStyle == gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE && len(m.Symbols) > 0 {
		return fmt.Errorf("PACKAGE_SURFACE surface must not declare symbols (schema reserves symbols for declared-interface style)")
	}
	if m.FormatVersion != SurfaceFormatVersion {
		return fmt.Errorf("surface %w: got %d, supported: %d", ErrUnsupportedVersion, m.FormatVersion, SurfaceFormatVersion)
	}
	if err := rejectDuplicates("surface package", m.Packages); err != nil {
		return err
	}
	for _, pkg := range m.Packages {
		if err := validPackagePath(pkg); err != nil {
			return fmt.Errorf("surface package %q: %w", pkg, err)
		}
	}
	if err := rejectDuplicates("surface symbol", m.Symbols); err != nil {
		return err
	}
	for _, sym := range m.Symbols {
		if _, err := symbol.Parse(sym); err != nil {
			return fmt.Errorf("surface symbol %q: %w", sym, err)
		}
	}
	if m.SdkKey != nil {
		if err := rejectDuplicates("SDK key build tag", m.SdkKey.BuildTags); err != nil {
			return err
		}
	}
	return validateAuthority(m.Authority)
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
			if !knownCapabilities[cap] {
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

// validPackagePath enforces the concrete import-path grammar for surface
// packages: non-empty slash-separated elements of Go identifier characters,
// '-', and '.', with no wildcards or patterns — surfaces never carry patterns
// (DR-16). The grammar mirrors symbol's package-path check so that package
// entries and the packages embedded in symbol IDs agree.
func validPackagePath(pkg string) error {
	if pkg == "" {
		return errors.New("package path must not be empty")
	}
	if strings.HasPrefix(pkg, "/") || strings.HasSuffix(pkg, "/") || strings.Contains(pkg, "//") {
		return errors.New("not a concrete import path: empty path element")
	}
	for _, elem := range strings.Split(pkg, "/") {
		if elem == "." || elem == ".." {
			return fmt.Errorf("path element %q is not a package element", elem)
		}
		for _, part := range strings.Split(elem, ".") {
			if part == "" {
				return fmt.Errorf("empty dot-separated part in %q", elem)
			}
			for _, r := range part {
				if r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
					continue
				}
				return fmt.Errorf("character %q is not allowed in a concrete import path", r)
			}
		}
	}
	return nil
}
