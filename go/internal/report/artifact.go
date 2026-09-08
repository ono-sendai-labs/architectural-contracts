package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// Verdict is the explicit pass/fail outcome recorded in a persisted report.
// It is derived from the completed report's violations; there is no API that
// accepts a caller-supplied verdict, so a contradictory verdict cannot be
// persisted (task req 1).
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

// ReportFormatVersion is the only supported major version of the persisted
// report artifact; readers reject any other major version (DR-15).
const ReportFormatVersion = 1

// MaxReportBytes caps a persisted report read before decoding (DR-15 size
// caps); the filesystem-backed reader enforces it.
const MaxReportBytes = 16 << 20 // 16 MiB

// Persisted errors are sentinels so callers (and `arcc verdict`) can classify
// failures into tool errors versus assertion mismatches.
var (
	// ErrMalformedReport reports input that is not a well-formed report artifact.
	ErrMalformedReport = errors.New("malformed report artifact")
	// ErrUnknownVerdict reports a persisted verdict outside the pass|fail vocabulary.
	ErrUnknownVerdict = errors.New("unknown report verdict")
	// ErrVerdictMismatch reports a persisted verdict that contradicts the
	// embedded report's violations.
	ErrVerdictMismatch = errors.New("persisted verdict contradicts the report")
	// ErrUnsupportedReportVersion reports a report artifact whose major
	// format_version is not supported by this binary.
	ErrUnsupportedReportVersion = errors.New("unsupported report format version")
)

// PersistedReport is the canonical report artifact: the completed
// ConformanceReport plus its explicitly derived verdict. Constructed only by
// MarshalReport (encode) and DecodeReport (decode), which re-derive and
// validate the verdict, keeping the derivation authoritative in one place.
type PersistedReport struct {
	FormatVersion int               `json:"format_version"`
	Verdict       Verdict           `json:"verdict"`
	Report        ConformanceReport `json:"report"`
}

// VerdictOf derives the verdict of a completed report from its violations:
// non-empty Violations is fail, otherwise pass. This is the single
// authoritative derivation shared by encoding, decoding, and exit codes.
func VerdictOf(r ConformanceReport) Verdict {
	if len(r.Violations) > 0 {
		return VerdictFail
	}
	return VerdictPass
}

// MarshalReport returns the canonical JSON bytes of r with its verdict
// derived by VerdictOf. The findings collections are canonically sorted
// before encoding, so logically identical reports produce byte-identical
// output regardless of the order the findings were observed in.
func MarshalReport(r ConformanceReport) ([]byte, error) {
	persisted := PersistedReport{
		FormatVersion: ReportFormatVersion,
		Verdict:       VerdictOf(r),
		Report:        canonicalReport(r),
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report artifact: %w", err)
	}
	return data, nil
}

// DecodeReport parses and validates canonical report artifact bytes. Malformed
// JSON, an unsupported format_version, a verdict outside the pass|fail
// vocabulary, or a verdict that contradicts the embedded report each fail with
// a specific sentinel-wrapped error.
func DecodeReport(data []byte) (PersistedReport, error) {
	var persisted PersistedReport
	// Unknown fields are ignored for forward compatibility (DR-15); a reader
	// must not fail on artifacts written by a newer minor writer.
	if err := json.Unmarshal(data, &persisted); err != nil {
		return PersistedReport{}, fmt.Errorf("%w: parse report artifact: %v", ErrMalformedReport, err)
	}
	if persisted.FormatVersion != ReportFormatVersion {
		return PersistedReport{}, fmt.Errorf("report %w: got %d, supported: %d", ErrUnsupportedReportVersion, persisted.FormatVersion, ReportFormatVersion)
	}
	switch persisted.Verdict {
	case VerdictPass, VerdictFail:
	default:
		return PersistedReport{}, fmt.Errorf("%w: %q", ErrUnknownVerdict, string(persisted.Verdict))
	}
	if actual := VerdictOf(persisted.Report); actual != persisted.Verdict {
		return PersistedReport{}, fmt.Errorf("%w: verdict %q but report has %d violations", ErrVerdictMismatch, persisted.Verdict, len(persisted.Report.Violations))
	}
	return persisted, nil
}

// canonicalReport returns a deterministic copy of r: findings sorted by kind,
// then message, then file, then line; nil slices normalized to empty.
func canonicalReport(r ConformanceReport) ConformanceReport {
	c := r
	c.Violations = sortFindings(r.Violations)
	c.Warnings = sortFindings(r.Warnings)
	return c
}

func sortFindings(findings []Finding) []Finding {
	sorted := make([]Finding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Message != b.Message {
			return a.Message < b.Message
		}
		if a.Location.File != b.Location.File {
			return a.Location.File < b.Location.File
		}
		return a.Location.Line < b.Location.Line
	})
	return sorted
}
