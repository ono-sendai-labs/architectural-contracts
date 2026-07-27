// Package capanalyzer defines the port/interface and types for analyzing capabilities of Go packages.
//
// Component Contract (FR10):
// - What it does: Defines the abstract CapabilityAnalyzer port, finding structures, and capability policies.
// - What it requires: The caller must provide valid AnalyzeRequest configurations (packages and prune symbols).
// - What it provides: An injection seam for capability analysis and policy classification of identified findings.
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

// Frame represents a single call-stack frame in a capanalyzer finding.
type Frame struct {
	Func string
	File string
	Line int
}

// CapabilityFinding is one capability reached transitively by a component's code.
type CapabilityFinding struct {
	Package    string  // import path where the capability is incurred
	Capability string  // e.g. "FILES", "NETWORK", "REFLECT"
	Class      Class   // TrueAuthority | AnalysisDefeating
	CallPath   []Frame // example path: caller -> ... -> privileged callee
}

// InterfaceSymbol identifies one declared-interface symbol of a component, in the
// key form Capslock/go-types use, e.g. "example.com/store.Read" or
// "(*example.com/store.DB).Get". It is the shared primitive behind both the
// boundary check (Pillar 1) and capability pruning (Pillar 3).
//
// Normalization Contract (A4):
// - Generic type-argument brackets are stripped from SSA names before comparison.
// - Methods are emitted in BOTH pointer- and value-receiver key forms.
// - Examples of valid key formats: "pkg.Name" and "(*pkg.Type).Method".
type InterfaceSymbol string

// AnalyzeRequest asks the analyzer for the capabilities of a component's packages.
//
// Scope Semantics:
//   - Analyzes every function in Packages (Capslock's native behavior).
//   - "_test.go" files are excluded by construction.
//   - Traversal is pruned per-symbol at PruneAt and at package granularity at
//     PruneAtPackages. Every function in a package listed in PruneAtPackages is
//     treated as capability-safe, with PruneAt's per-symbol keys taking
//     precedence. Package pruning is weaker than symbol pruning and is what a
//     PACKAGE_SURFACE dependency gets.
type AnalyzeRequest struct {
	Packages        []string
	PruneAt         []InterfaceSymbol
	PruneAtPackages []string
}

// CapabilityAnalyzer is the port the checker depends on; the Capslock adapter
// (in the shell) implements it. Injecting it keeps the checker pure and testable.
type CapabilityAnalyzer interface {
	Analyze(req AnalyzeRequest) ([]CapabilityFinding, error)
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
