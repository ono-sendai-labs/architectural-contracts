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

	// 2. Instantiate and check InterfaceSymbol type
	var sym capanalyzer.InterfaceSymbol = "example.com/store.Read"
	if string(sym) != "example.com/store.Read" {
		t.Errorf("expected example.com/store.Read, got %q", sym)
	}

	// 3. Instantiate Frame
	frame := capanalyzer.Frame{
		Func: "main",
		File: "main.go",
		Line: 42,
	}
	if frame.Func != "main" || frame.File != "main.go" || frame.Line != 42 {
		t.Errorf("frame fields mismatch: %+v", frame)
	}

	// 4. Instantiate CapabilityFinding
	finding := capanalyzer.CapabilityFinding{
		Package:    "os",
		Capability: "FILES",
		Class:      capanalyzer.TrueAuthority,
		CallPath:   []capanalyzer.Frame{frame},
	}
	if finding.Package != "os" || finding.Capability != "FILES" || finding.Class != capanalyzer.TrueAuthority || len(finding.CallPath) != 1 {
		t.Errorf("finding fields mismatch: %+v", finding)
	}

	// 5. Instantiate AnalyzeRequest
	req := capanalyzer.AnalyzeRequest{
		Packages: []string{"example.com/foo"},
		PruneAt:  []capanalyzer.InterfaceSymbol{sym},
	}
	if len(req.Packages) != 1 || req.Packages[0] != "example.com/foo" || len(req.PruneAt) != 1 || req.PruneAt[0] != sym {
		t.Errorf("request fields mismatch: %+v", req)
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
