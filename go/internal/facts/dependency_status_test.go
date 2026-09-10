package facts_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func TestDependencyStatusVocabularies(t *testing.T) {
	provenance := []struct {
		name  string
		value facts.DependencyProvenance
	}{
		{name: "checked pass", value: facts.DependencyProvenanceCheckedPass},
		{name: "checked fail", value: facts.DependencyProvenanceCheckedFail},
		{name: "asserted", value: facts.DependencyProvenanceAsserted},
	}
	for _, tt := range provenance {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.value.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}

	freshness := []struct {
		name  string
		value facts.DependencyFreshness
	}{
		{name: "build graph", value: facts.DependencyFreshnessBuildGraph},
		{name: "verified", value: facts.DependencyFreshnessVerified},
		{name: "stale", value: facts.DependencyFreshnessStale},
		{name: "unknown", value: facts.DependencyFreshnessUnknown},
	}
	for _, tt := range freshness {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.value.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestDependencyInterfaceCarriesIndependentStatusAxes(t *testing.T) {
	dep := facts.DependencyInterface{
		Component:  "dep",
		Provenance: facts.DependencyProvenanceCheckedFail,
		Freshness:  facts.DependencyFreshnessStale,
	}
	if dep.Provenance != facts.DependencyProvenanceCheckedFail {
		t.Errorf("provenance = %q, want CHECKED_FAIL", dep.Provenance)
	}
	if dep.Freshness != facts.DependencyFreshnessStale {
		t.Errorf("freshness = %q, want STALE", dep.Freshness)
	}
}

func TestDependencyStatusValidationRejectsUnknownValues(t *testing.T) {
	tests := []struct {
		name  string
		check func() error
	}{
		{name: "provenance", check: func() error { return facts.DependencyProvenance("bogus").Validate() }},
		{name: "freshness", check: func() error { return facts.DependencyFreshness("bogus").Validate() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.check(); err == nil {
				t.Fatal("Validate() = nil, want an invalid-vocabulary error")
			}
		})
	}
}
