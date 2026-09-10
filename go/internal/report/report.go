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
	AnalysisLimitation     Kind = "ANALYSIS_LIMITATION"
	AllowedWithWarning     Kind = "ALLOWED_WITH_WARNING"
	UnusedDependency       Kind = "UNUSED_DEPENDENCY"
	InterfaceFileExcluded  Kind = "INTERFACE_FILE_EXCLUDED"
	DependencyCheckFailed  Kind = "DEPENDENCY_CHECK_FAILED"
	DependencySurfaceStale Kind = "DEPENDENCY_SURFACE_STALE"
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

// AuthoritySite is one member source site contributing to an aggregated
// authority finding (DR-17): the component-relative file, the 1-based line,
// and the referenced symbol's canonical identity (empty for analysis-defeating
// constructs, which name no declaring object).
type AuthoritySite struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol,omitempty"`
}

// Finding represents a single violation or warning discovered during checker analysis.
//
// An aggregated authority finding (one per (capability, class), DR-17) leaves
// Location empty and carries its full sorted site collection in Sites, the
// classification class and the stdlib map's SDK key, with Evidence holding the
// map's evidence frames for the first site's (symbol, capability). Boundary
// findings keep their exact single Location and no Sites.
type Finding struct {
	Kind     Kind            `json:"kind"`
	Message  string          `json:"message"`
	Location Location        `json:"location"`
	Evidence []string        `json:"evidence,omitempty"`
	Class    string          `json:"class,omitempty"`
	SDKKey   string          `json:"sdk_key,omitempty"`
	Sites    []AuthoritySite `json:"sites,omitempty"`
}

// DependencyProvenance identifies the structural producer and conformance
// result behind a dependency surface. It is separate from freshness and
// authority so a report cannot collapse distinct trust states into one label.
type DependencyProvenance string

const (
	DependencyProvenanceCheckedPass DependencyProvenance = "CHECKED_PASS"
	DependencyProvenanceCheckedFail DependencyProvenance = "CHECKED_FAIL"
	DependencyProvenanceAsserted    DependencyProvenance = "ASSERTED"
)

// DependencyFreshness identifies how a dependency surface was tied to its
// producer inputs.
type DependencyFreshness string

const (
	DependencyFreshnessBuildGraph DependencyFreshness = "BUILD_GRAPH"
	DependencyFreshnessVerified   DependencyFreshness = "VERIFIED"
	DependencyFreshnessStale      DependencyFreshness = "STALE"
	DependencyFreshnessUnknown    DependencyFreshness = "UNKNOWN"
)

// DependencyAuthority identifies the structural authority declaration carried
// by a dependency surface. DeclaredAuthority is meaningful for DECLARED and
// remains empty for UNKNOWN.
type DependencyAuthority string

const (
	DependencyAuthorityDeclared DependencyAuthority = "DECLARED"
	DependencyAuthorityUnknown  DependencyAuthority = "UNKNOWN"
)

// DependencyBoundary represents a component dependency boundary annotation
// in a report. The three status axes are deliberately persisted separately;
// the text renderer only adds familiar summary words for their combinations.
type DependencyBoundary struct {
	Component         string               `json:"component"`
	Provenance        DependencyProvenance `json:"provenance,omitempty"`
	Freshness         DependencyFreshness  `json:"freshness,omitempty"`
	Authority         DependencyAuthority  `json:"authority,omitempty"`
	DeclaredAuthority []string             `json:"declared_authority,omitempty"`
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
			renderFinding(&sb, v)
		}
	}

	if len(r.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, w := range r.Warnings {
			renderFinding(&sb, w)
		}
	}

	return sb.String()
}

// renderFinding renders one finding as a single text entry. A finding with
// sites (an aggregated authority finding, DR-17) prints the first sorted site
// and the total site count — never one entry per site; a boundary finding
// prints its exact location.
func renderFinding(sb *strings.Builder, f Finding) {
	sb.WriteString(fmt.Sprintf("- [%s] %s\n", f.Kind, f.Message))
	if len(f.Sites) > 0 {
		sb.WriteString(fmt.Sprintf("  at %s:%d (%d sites)\n", f.Sites[0].File, f.Sites[0].Line, len(f.Sites)))
	} else if f.Location.File != "" {
		sb.WriteString(fmt.Sprintf("  at %s:%d\n", f.Location.File, f.Location.Line))
	}
	if len(f.Evidence) > 0 {
		sb.WriteString("  Evidence:\n")
		for _, ev := range f.Evidence {
			sb.WriteString(fmt.Sprintf("    - %s\n", ev))
		}
	}
}

func formatDependencyBoundary(dep DependencyBoundary) string {
	words := dependencyStatusWords(dep)
	if len(words) == 0 {
		return fmt.Sprintf("- %s", dep.Component)
	}
	return fmt.Sprintf("- %s (%s)", dep.Component, strings.Join(words, ", "))
}

func dependencyStatusWords(dep DependencyBoundary) []string {
	var words []string
	switch dep.Provenance {
	case DependencyProvenanceCheckedPass:
		if dep.Freshness != DependencyFreshnessStale {
			words = append(words, "certified")
		}
	case DependencyProvenanceCheckedFail:
		words = append(words, "check failed")
	case DependencyProvenanceAsserted:
		words = append(words, "asserted")
	}
	if dep.Freshness == DependencyFreshnessStale {
		words = append(words, "stale")
	}
	if dep.Authority == DependencyAuthorityUnknown {
		words = append(words, "untrusted")
	}
	return words
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
