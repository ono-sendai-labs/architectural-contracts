package capanalyzer_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
)

func TestCoreTypes(t *testing.T) {
	// 1. Verify Class constants
	if capanalyzer.TrueAuthority.String() != "TrueAuthority" {
		t.Errorf("expected TrueAuthority, got %q", capanalyzer.TrueAuthority.String())
	}
	if capanalyzer.AnalysisDefeating.String() != "AnalysisDefeating" {
		t.Errorf("expected AnalysisDefeating, got %q", capanalyzer.AnalysisDefeating.String())
	}

	// 2. Class constants carry their string forms
	if capanalyzer.Class("TrueAuthority") != capanalyzer.TrueAuthority {
		t.Error("Class spelling changed")
	}
}

func TestStrictPolicy(t *testing.T) {
	policy := capanalyzer.StrictPolicy()
	if len(policy.Allowed) != 0 {
		t.Errorf("expected empty Allowed map in StrictPolicy, got %v", policy.Allowed)
	}
	if len(policy.Warn) != 0 {
		t.Errorf("expected empty Warn map in StrictPolicy, got %v", policy.Warn)
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		policy     capanalyzer.CapabilityPolicy
		capability string
		want       capanalyzer.Decision
	}{
		{
			name: "In Allowed map only",
			policy: capanalyzer.CapabilityPolicy{
				Allowed: map[string]bool{"FILES": true},
			},
			capability: "FILES",
			want:       capanalyzer.DecisionAllowed,
		},
		{
			name: "In Warn map only",
			policy: capanalyzer.CapabilityPolicy{
				Warn: map[string]bool{"NETWORK": true},
			},
			capability: "NETWORK",
			want:       capanalyzer.DecisionWarn,
		},
		{
			name:       "In neither map",
			policy:     capanalyzer.CapabilityPolicy{},
			capability: "FILES",
			want:       capanalyzer.DecisionViolation,
		},
		{
			name: "In both maps (allow wins)",
			policy: capanalyzer.CapabilityPolicy{
				Allowed: map[string]bool{"FILES": true},
				Warn:    map[string]bool{"FILES": true},
			},
			capability: "FILES",
			want:       capanalyzer.DecisionAllowed,
		},
		{
			name:       "StrictPolicy (everything violates)",
			policy:     capanalyzer.StrictPolicy(),
			capability: "ANY_CAP",
			want:       capanalyzer.DecisionViolation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := capanalyzer.Classify(tc.capability, tc.policy)
			if got != tc.want {
				t.Errorf("Classify(%q) = %s, want %s", tc.capability, got, tc.want)
			}
		})
	}
}
