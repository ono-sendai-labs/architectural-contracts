// Package report defines the conformance report data models and rendering logic.
//
// Component Contract (FR10):
// - What it does: Defines the representation of architectural checker findings, renders them into deterministic, human-readable text, and owns the authoritative pass/fail verdict derivation for the persisted report artifact.
// - What it requires: Receives a ConformanceReport struct populated with violations and warnings from the checker.
// - What it provides: RenderText for plain-text formatting and the Verdict/VerdictOf derivation consumed by the persisted-artifact codec in the artifactio shell. JSON marshaling, filesystem access, and artifact file I/O stay in the shell (artifactio); this package remains free of reflection.
// - Ambient Authority: This component is guaranteed-pure and holds no ambient authority (performs no filesystem I/O, network, process execution, or reflection).
package report

import (
	"fmt"
	"strings"
)

// Kind represents the category of a conformance finding (violation or warning).
type Kind string

const (
	// Violations
	UndeclaredDependency     Kind = "UNDECLARED_DEPENDENCY"
	CallsUndeclaredInterface Kind = "CALLS_UNDECLARED_INTERFACE"
	UndeclaredAuthority      Kind = "UNDECLARED_AUTHORITY"
	MethodOutsideInterface   Kind = "METHOD_OUTSIDE_INTERFACE"
	MemberOverlap            Kind = "MEMBER_OVERLAP"

	// Warnings
	AnalysisLimitation    Kind = "ANALYSIS_LIMITATION"
	AllowedWithWarning    Kind = "ALLOWED_WITH_WARNING"
	UnusedDependency      Kind = "UNUSED_DEPENDENCY"
	InterfaceFileExcluded Kind = "INTERFACE_FILE_EXCLUDED"
)

// String returns the string representation of the Kind.
func (k Kind) String() string {
	return string(k)
}

// Location represents a source locator (file and line number) for a finding.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Finding represents a single violation or warning discovered during checker analysis.
type Finding struct {
	Kind     Kind     `json:"kind"`
	Message  string   `json:"message"`
	Location Location `json:"location"`
	Evidence []string `json:"evidence,omitempty"`
}

// DependencyBoundary represents a component dependency boundary annotation in a report.
type DependencyBoundary struct {
	Component string `json:"component"`
}

// ConformanceReport is the overall result of analyzing a component against its manifest.
type ConformanceReport struct {
	Component    string               `json:"component"`
	Dependencies []DependencyBoundary `json:"dependencies,omitempty"`
	Violations   []Finding            `json:"violations"`
	Warnings     []Finding            `json:"warnings"`
}

// RenderText returns a deterministic, human-readable string representation of the ConformanceReport.
func RenderText(r ConformanceReport) string {
	return r.RenderText()
}

// RenderText returns a deterministic, human-readable string representation of the ConformanceReport.
func (r ConformanceReport) RenderText() string {
	var sb strings.Builder
	if len(r.Violations) == 0 && len(r.Warnings) == 0 {
		sb.WriteString(fmt.Sprintf("Component %q conforms; does not exceed declared authority\n", r.Component))
		if len(r.Dependencies) > 0 {
			sb.WriteString("\nDependencies:\n")
			for _, dep := range r.Dependencies {
				sb.WriteString(formatDependencyBoundary(dep))
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Component: %s\n", r.Component))

	if len(r.Dependencies) > 0 {
		sb.WriteString("\nDependencies:\n")
		for _, dep := range r.Dependencies {
			sb.WriteString(formatDependencyBoundary(dep))
			sb.WriteString("\n")
		}
	}

	if len(r.Violations) > 0 {
		sb.WriteString("\nViolations:\n")
		for _, v := range r.Violations {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", v.Kind, v.Message))
			if v.Location.File != "" {
				sb.WriteString(fmt.Sprintf("  at %s:%d\n", v.Location.File, v.Location.Line))
			}
			if len(v.Evidence) > 0 {
				sb.WriteString("  Evidence:\n")
				for _, ev := range v.Evidence {
					sb.WriteString(fmt.Sprintf("    - %s\n", ev))
				}
			}
		}
	}

	if len(r.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range r.Warnings {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", w.Kind, w.Message))
			if w.Location.File != "" {
				sb.WriteString(fmt.Sprintf("  at %s:%d\n", w.Location.File, w.Location.Line))
			}
			if len(w.Evidence) > 0 {
				sb.WriteString("  Evidence:\n")
				for _, ev := range w.Evidence {
					sb.WriteString(fmt.Sprintf("    - %s\n", ev))
				}
			}
		}
	}

	return sb.String()
}

func formatDependencyBoundary(dep DependencyBoundary) string {
	return fmt.Sprintf("- %s", dep.Component)
}

// Verdict is the explicit pass/fail outcome recorded in a persisted report
// artifact. It is derived from the completed report's violations; no
// caller-supplied verdict is accepted anywhere in the artifact pipeline, so a
// contradictory verdict cannot be persisted (task req 1).
type Verdict string

const (
	// VerdictPass records a completed analysis with zero violations.
	VerdictPass Verdict = "pass"
	// VerdictFail records a successful analysis with at least one violation.
	VerdictFail Verdict = "fail"
)

// String returns the string representation of the Verdict.
func (v Verdict) String() string {
	return string(v)
}

// VerdictOf derives the verdict of a completed report from its violations:
// non-empty Violations is fail, otherwise pass. This is the single
// authoritative derivation shared by artifact encoding, decoding, and exit
// codes; it stays in the pure report package because it is a property of the
// report data model itself. The canonical JSON codec that consumes it lives
// in the shell (artifactio), which holds the reflection authority JSON
// encoding requires.
func VerdictOf(r ConformanceReport) Verdict {
	if len(r.Violations) > 0 {
		return VerdictFail
	}
	return VerdictPass
}
