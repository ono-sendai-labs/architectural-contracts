package manifest

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
)

// Capability names a Capslock ambient-authority capability, e.g. FILES.
type Capability = string

// AuthorityDeclaration is the structural authority element (design §Data
// Models, R10, R11): either a known, sorted set of declared capabilities, or
// the unknown value meaning "not analysed; could be anything".
//
// UNKNOWN is deliberately not representable as an empty known set: empty means
// "checked, uses nothing", which only holds for a verified component.
type AuthorityDeclaration struct {
	Known bool         // false ⇒ UNKNOWN; Set must be empty and is ignored
	Set   []Capability // meaningful only when Known; sorted, duplicate-free
}

// UnknownAuthority returns the lattice top: an unanalysed component whose
// authority is unknown rather than empty.
func UnknownAuthority() AuthorityDeclaration {
	return AuthorityDeclaration{Known: false}
}

// NewDeclared validates and returns a known declaration carrying the given
// capabilities. Duplicate or unknown capability names are rejected, and the
// stored set is sorted and duplicate-free.
func NewDeclared(capabilities ...Capability) (AuthorityDeclaration, error) {
	seen := make(map[Capability]bool, len(capabilities))
	for _, c := range capabilities {
		if seen[c] {
			return AuthorityDeclaration{}, &DuplicateDeclarationError{Kind: "declared authority", Value: c}
		}
		seen[c] = true
		if !KnownCapabilities[c] {
			return AuthorityDeclaration{}, &UnknownCapabilityError{Capability: c}
		}
	}
	set := slices.Clone(capabilities)
	sort.Strings(set)
	return AuthorityDeclaration{Known: true, Set: set}, nil
}

// DeclaredAuthority returns a known declaration without validation, for tests
// and internal call sites that have already validated their capabilities.
func DeclaredAuthority(capabilities ...Capability) AuthorityDeclaration {
	return AuthorityDeclaration{Known: true, Set: capabilities}
}

// Join is the only way to combine declarations. Unknown absorbs: joining an
// unknown operand with anything yields unknown. Two known declarations join by
// unioning their capability sets; the result is sorted, duplicate-free, and
// never aliases either input slice.
func Join(a, b AuthorityDeclaration) AuthorityDeclaration {
	if !a.Known || !b.Known {
		return UnknownAuthority()
	}
	merged := make([]Capability, 0, len(a.Set)+len(b.Set))
	merged = append(merged, a.Set...)
	merged = append(merged, b.Set...)
	slices.Sort(merged)
	merged = slices.Compact(merged)
	if len(merged) == 0 {
		merged = nil
	}
	return AuthorityDeclaration{Known: true, Set: merged}
}

// Equal reports whether two declarations are the same lattice element. Slice
// emptiness alone is never the test: DECLARED{} and UNKNOWN are distinct even
// though both have empty sets.
func Equal(a, b AuthorityDeclaration) bool {
	if a.Known != b.Known {
		return false
	}
	if !a.Known {
		return true
	}
	return slices.Equal(a.Set, b.Set)
}

// ToPersisted converts a native declaration to the persisted representation:
// a generated authority enum value plus the declared capability list. It
// rejects a non-canonical unknown-with-capabilities value instead of
// normalizing it away (R11).
func ToPersisted(d AuthorityDeclaration) (gen.Authority, []Capability, error) {
	if !d.Known {
		if len(d.Set) > 0 {
			return gen.Authority_UNKNOWN, nil, fmt.Errorf("unknown authority declaration carries %d declared capabilities; declared_authority must be empty when authority is UNKNOWN", len(d.Set))
		}
		return gen.Authority_UNKNOWN, nil, nil
	}
	return gen.Authority_DECLARED, slices.Clone(d.Set), nil
}

// FromPersisted converts the persisted authority-plus-capabilities
// representation to a native declaration. It rejects an unknown value paired
// with a non-empty capability list, an unrecognized enum value, and unknown
// capability names; nothing is normalized away.
func FromPersisted(authority gen.Authority, capabilities []Capability) (AuthorityDeclaration, error) {
	switch authority {
	case gen.Authority_UNKNOWN:
		if len(capabilities) > 0 {
			return AuthorityDeclaration{}, fmt.Errorf("authority is UNKNOWN but declared_authority is non-empty (%s); declared_authority must be empty when authority is UNKNOWN", strings.Join(capabilities, ", "))
		}
		return UnknownAuthority(), nil
	case gen.Authority_DECLARED:
		return NewDeclared(capabilities...)
	default:
		return AuthorityDeclaration{}, fmt.Errorf("unknown authority value: %d", int32(authority))
	}
}
