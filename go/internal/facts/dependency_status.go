package facts

import "fmt"

// DependencyProvenance identifies the structural source of a dependency
// conformance claim. It is intentionally separate from freshness and
// authority knowledge: an asserted surface can be fresh or stale, and an
// authority declaration can be known or unknown, without changing how the
// surface was produced.
type DependencyProvenance string

const (
	DependencyProvenanceCheckedPass DependencyProvenance = "CHECKED_PASS"
	DependencyProvenanceCheckedFail DependencyProvenance = "CHECKED_FAIL"
	DependencyProvenanceAsserted    DependencyProvenance = "ASSERTED"
	ProvenanceCheckedPass                                = DependencyProvenanceCheckedPass
	ProvenanceCheckedFail                                = DependencyProvenanceCheckedFail
	ProvenanceAsserted                                   = DependencyProvenanceAsserted
)

// String returns the persisted vocabulary spelling.
func (p DependencyProvenance) String() string { return string(p) }

// Validate rejects a value outside the dependency provenance vocabulary.
func (p DependencyProvenance) Validate() error {
	switch p {
	case DependencyProvenanceCheckedPass, DependencyProvenanceCheckedFail, DependencyProvenanceAsserted:
		return nil
	default:
		return fmt.Errorf("unknown dependency provenance %q", p)
	}
}

// DependencyFreshness identifies how the resolver established that a surface
// corresponds to its producer inputs. It is independent of provenance and
// authority: BUILD_GRAPH is a structural fact, while the native values are
// best-effort byte observations.
type DependencyFreshness string

const (
	DependencyFreshnessBuildGraph DependencyFreshness = "BUILD_GRAPH"
	DependencyFreshnessVerified   DependencyFreshness = "VERIFIED"
	DependencyFreshnessStale      DependencyFreshness = "STALE"
	DependencyFreshnessUnknown    DependencyFreshness = "UNKNOWN"
	FreshnessBuildGraph                               = DependencyFreshnessBuildGraph
	FreshnessVerified                                 = DependencyFreshnessVerified
	FreshnessStale                                    = DependencyFreshnessStale
	FreshnessUnknown                                  = DependencyFreshnessUnknown
)

// String returns the persisted vocabulary spelling.
func (f DependencyFreshness) String() string { return string(f) }

// Validate rejects a value outside the dependency freshness vocabulary.
func (f DependencyFreshness) Validate() error {
	switch f {
	case DependencyFreshnessBuildGraph, DependencyFreshnessVerified, DependencyFreshnessStale, DependencyFreshnessUnknown:
		return nil
	default:
		return fmt.Errorf("unknown dependency freshness %q", f)
	}
}
