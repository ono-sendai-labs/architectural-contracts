// Package report defines the conformance report data models and rendering logic.
//
// Component Contract (FR10):
// - What it does: Defines the representation of architectural checker findings and renders them into deterministic, human-readable text.
// - What it requires: Receives a ConformanceReport struct populated with violations and warnings from the checker.
// - What it provides: RenderText for plain-text formatting. Avoids JSON marshaling internally to keep the package free of reflection.
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
	AnalysisLimitation      Kind = "ANALYSIS_LIMITATION"
	AllowedWithWarning      Kind = "ALLOWED_WITH_WARNING"
	HigherOrderBoundaryCall Kind = "HIGHER_ORDER_BOUNDARY_CALL"
	UnusedDependency        Kind = "UNUSED_DEPENDENCY"
	InterfaceFileExcluded   Kind = "INTERFACE_FILE_EXCLUDED"
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

// ConformanceReport is the overall result of analyzing a component against its manifest.
type ConformanceReport struct {
	Component  string    `json:"component"`
	Violations []Finding `json:"violations"`
	Warnings   []Finding `json:"warnings"`
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
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Component: %s\n", r.Component))

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
