package facts

import (
	"fmt"
	"slices"
)

// BypassKind classifies the analysis-defeating construct of a bypass
// observation (DR-11): a `//go:linkname` directive, an assembly source file,
// or a cgo use. None of these constructs is visible to typed reference
// analysis, so each must be observed directly from member file metadata and
// source.
type BypassKind string

const (
	// BypassLinkname marks a `//go:linkname` directive in member source.
	BypassLinkname BypassKind = "linkname"
	// BypassAssembly marks an assembly source file of a member package.
	BypassAssembly BypassKind = "assembly"
	// BypassCgo marks a cgo use (`import "C"`) in member source.
	BypassCgo BypassKind = "cgo"
)

// BypassObservation records one analysis-defeating construct at an exact
// component-relative site. It carries no referenced symbol: bypass constructs
// create edges typed analysis cannot see, so there is no declaring-object
// identity to name.
//
// Identity is structural: two observations are the same exactly when kind and
// site match; anything else is a distinct observation.
type BypassObservation struct {
	Kind BypassKind
	Site SourceSite
}

// Validate checks that the observation is well-formed: a known kind and a
// component-relative 1-based site.
func (o BypassObservation) Validate() error {
	switch o.Kind {
	case BypassLinkname, BypassAssembly, BypassCgo:
	default:
		return fmt.Errorf("bypass observation: unknown kind %q", o.Kind)
	}
	if err := o.Site.Validate(); err != nil {
		return fmt.Errorf("bypass observation of kind %q: %w", o.Kind, err)
	}
	return nil
}

// CompareBypassObservations totally orders two observations by site (file,
// then line — the finding-site order), then kind.
func CompareBypassObservations(a, b BypassObservation) int {
	if c := CompareSourceSite(a.Site, b.Site); c != 0 {
		return c
	}
	return stringCompare(string(a.Kind), string(b.Kind))
}

// SortBypassObservations returns a new slice in deterministic order.
func SortBypassObservations(obs []BypassObservation) []BypassObservation {
	out := make([]BypassObservation, len(obs))
	copy(out, obs)
	slices.SortStableFunc(out, CompareBypassObservations)
	return out
}

// DedupBypassObservations removes exact duplicates from an already-sorted
// observation list, keeping one observation per distinct identity.
func DedupBypassObservations(obs []BypassObservation) []BypassObservation {
	out := obs[:0:0]
	for i, o := range obs {
		if i > 0 && CompareBypassObservations(obs[i-1], o) == 0 {
			continue
		}
		out = append(out, o)
	}
	return out
}

func stringCompare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// BypassKey is the structural identity of a bypass observation.
type BypassKey struct {
	Kind BypassKind
	Site SourceSite
}
