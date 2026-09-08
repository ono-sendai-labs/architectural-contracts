// Package capanalyzer defines the capability classes, policies, and policy
// classification shared by the stdlib authority classification and the report.
//
// Component Contract (FR10):
// - What it does: Defines the capability Class taxonomy (TrueAuthority, AnalysisDefeating) and the CapabilityPolicy decision model.
// - What it requires: Nothing; pure data and pure classification.
// - What it provides: Class, CapabilityPolicy, StrictPolicy, and Classify.
// - Ambient Authority: This component is guaranteed-pure and holds no ambient authority (no I/O, no filesystem, and no environment access).
package capanalyzer

// Class represents the taxonomy of a capability finding (TrueAuthority or AnalysisDefeating).
type Class string

const (
	TrueAuthority     Class = "TrueAuthority"
	AnalysisDefeating Class = "AnalysisDefeating"
)

// String returns the string representation of Class.
func (c Class) String() string {
	return string(c)
}

// CapabilityPolicy decides, per capability, whether a finding is allowed, a warning, or a violation.
type CapabilityPolicy struct {
	Allowed map[string]bool // permitted capabilities (e.g. from declared_authority)
	Warn    map[string]bool // capabilities downgraded to a non-fatal warning
}

// StrictPolicy returns the empty policy (both maps empty/nil), which is the MVP default.
func StrictPolicy() CapabilityPolicy {
	return CapabilityPolicy{}
}

// Decision represents the result of classifying a capability against a policy.
type Decision int

const (
	DecisionViolation Decision = iota
	DecisionWarn
	DecisionAllowed
)

// String returns the string representation of Decision.
func (d Decision) String() string {
	switch d {
	case DecisionAllowed:
		return "allowed"
	case DecisionWarn:
		return "warn"
	case DecisionViolation:
		return "violation"
	default:
		return "unknown"
	}
}

// Classify determines if a capability name is allowed, a warn, or a violation against the given policy.
// Rule precedence:
// 1. ∈ Allowed -> allowed (allow wins).
// 2. ∈ Warn -> warn.
// 3. otherwise -> violation.
func Classify(capability string, policy CapabilityPolicy) Decision {
	if policy.Allowed != nil && policy.Allowed[capability] {
		return DecisionAllowed
	}
	if policy.Warn != nil && policy.Warn[capability] {
		return DecisionWarn
	}
	return DecisionViolation
}
