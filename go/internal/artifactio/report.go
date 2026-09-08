package artifactio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

// Persisted report artifact contract (task req 1, 2): the canonical
// `<name>.report.json` is the completed ConformanceReport plus an explicit
// verdict derived by report.VerdictOf. The verdict is derived at every
// boundary — MarshalJSON derives it regardless of what a caller stamped into
// the struct, and UnmarshalJSON re-derives and validates it — so a
// contradictory verdict can be neither written nor read. ReportFormatVersion
// is the only supported major version; MaxReportBytes caps both direct
// decoding and file reads before parsing (DR-15).
const (
	ReportFormatVersion = 1
	MaxReportBytes      = 16 << 20 // 16 MiB
)

// Sentinels classifying persisted-report failures for `arcc verdict` and
// other callers: malformed input, an unknown verdict string, a verdict
// contradicting the embedded report, and an unsupported major format version.
var (
	ErrMalformedReport          = errors.New("malformed report artifact")
	ErrUnknownVerdict           = errors.New("unknown report verdict")
	ErrVerdictMismatch          = errors.New("persisted verdict contradicts the report")
	ErrUnsupportedReportVersion = errors.New("unsupported report format version")
)

// PersistedReport is the canonical report artifact: the completed
// ConformanceReport plus its explicitly derived verdict. Its JSON methods are
// the only serialization path, and they always derive the verdict from the
// embedded report (MarshalJSON) and validate it against the derivation
// (UnmarshalJSON), so callers cannot stamp a contradictory verdict into
// persisted bytes.
type PersistedReport struct {
	FormatVersion int
	Verdict       report.Verdict
	Report        report.ConformanceReport
}

// wirePersistedReport is the private JSON representation; unexported so no
// caller can serialize around the derived verdict.
type wirePersistedReport struct {
	FormatVersion int                      `json:"format_version"`
	Verdict       string                   `json:"verdict"`
	Report        report.ConformanceReport `json:"report"`
}

// MarshalJSON implements json.Marshaler: the verdict is always re-derived
// from the embedded report, and the findings and dependency collections are
// canonically sorted, so logically identical reports marshal to byte-identical
// output regardless of observation order or a caller-stamped Verdict field.
func (p PersistedReport) MarshalJSON() ([]byte, error) {
	wire := wirePersistedReport{
		FormatVersion: ReportFormatVersion,
		Verdict:       string(report.VerdictOf(p.Report)),
		Report:        canonicalReport(p.Report),
	}
	return json.Marshal(wire)
}

// UnmarshalJSON implements json.Unmarshaler by delegating to
// decodePersistedReport; see that function for the validation contract.
func (p *PersistedReport) UnmarshalJSON(data []byte) error {
	return p.unmarshal(data)
}

// unmarshal parses and validates the envelope; see decodePersistedReport.
func (p *PersistedReport) unmarshal(data []byte) error {
	parsed, err := decodePersistedReport(data)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// decodePersistedReport parses and validates the envelope: the fields are
// required (a missing or null report is malformed, never an implicit passing
// report), the format version and verdict vocabulary are validated, and the
// verdict is checked against the re-derived one. Unknown fields are ignored
// for forward compatibility (DR-15).
func decodePersistedReport(data []byte) (PersistedReport, error) {
	// The shared parser is the single size gate: it covers DecodeReport and
	// the public json.Unmarshal path (UnmarshalJSON), so no decoding
	// invocation escapes MaxReportBytes.
	if len(data) > MaxReportBytes {
		return PersistedReport{}, fmt.Errorf("decode report artifact: size %d exceeds the %d byte limit", len(data), MaxReportBytes)
	}
	var raw struct {
		FormatVersion *int             `json:"format_version"`
		Verdict       *string          `json:"verdict"`
		Report        *json.RawMessage `json:"report"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return PersistedReport{}, fmt.Errorf("%w: parse report artifact: %v", ErrMalformedReport, err)
	}
	if raw.FormatVersion == nil {
		return PersistedReport{}, fmt.Errorf("report %w: format_version is missing", ErrUnsupportedReportVersion)
	}
	if *raw.FormatVersion != ReportFormatVersion {
		return PersistedReport{}, fmt.Errorf("report %w: got %d, supported: %d", ErrUnsupportedReportVersion, *raw.FormatVersion, ReportFormatVersion)
	}
	if raw.Verdict == nil {
		return PersistedReport{}, fmt.Errorf("%w: verdict field is missing", ErrUnknownVerdict)
	}
	var verdict report.Verdict
	switch verdict = report.Verdict(*raw.Verdict); verdict {
	case report.VerdictPass, report.VerdictFail:
	default:
		return PersistedReport{}, fmt.Errorf("%w: %q", ErrUnknownVerdict, *raw.Verdict)
	}
	if raw.Report == nil {
		return PersistedReport{}, fmt.Errorf("%w: report field is missing", ErrMalformedReport)
	}
	if bytes.Equal(bytes.TrimSpace(*raw.Report), []byte("null")) {
		return PersistedReport{}, fmt.Errorf("%w: report field is null", ErrMalformedReport)
	}
	var parsed report.ConformanceReport
	if err := json.Unmarshal(*raw.Report, &parsed); err != nil {
		return PersistedReport{}, fmt.Errorf("%w: parse embedded report: %v", ErrMalformedReport, err)
	}
	if actual := report.VerdictOf(parsed); actual != verdict {
		return PersistedReport{}, fmt.Errorf("%w: verdict %q but report has %d violations", ErrVerdictMismatch, verdict, len(parsed.Violations))
	}
	return PersistedReport{
		FormatVersion: *raw.FormatVersion,
		Verdict:       verdict,
		Report:        parsed,
	}, nil
}

// MarshalReport returns the canonical JSON bytes of r with its verdict
// derived by report.VerdictOf. Every order-insensitive collection is
// canonically sorted, so logically identical reports produce byte-identical
// output regardless of the order the findings were observed in.
func MarshalReport(r report.ConformanceReport) ([]byte, error) {
	data, err := json.MarshalIndent(PersistedReport{Report: r}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal report artifact: %w", err)
	}
	return data, nil
}

// DecodeReport parses and validates canonical report artifact bytes. Input
// over MaxReportBytes is rejected before any JSON parsing; malformed JSON, a
// missing/null embedded report, an unsupported format_version, a verdict
// outside the pass|fail vocabulary, or a verdict that contradicts the
// embedded report each fail with a specific sentinel-wrapped error.
func DecodeReport(data []byte) (PersistedReport, error) {
	if len(data) > MaxReportBytes {
		return PersistedReport{}, fmt.Errorf("decode report artifact: size %d exceeds the %d byte limit", len(data), MaxReportBytes)
	}
	// Parse directly rather than through json.Unmarshal on the value: the
	// standard library wraps custom-unmarshaler errors in a form that loses
	// errors.Is identity, and callers classify these sentinels.
	persisted, err := decodePersistedReport(data)
	if err != nil {
		return PersistedReport{}, fmt.Errorf("decode report artifact: %w", err)
	}
	return persisted, nil
}

// canonicalReport returns a deterministic copy of r: dependency boundaries
// sorted by component, findings sorted by kind, message, file, line, and
// evidence (a full tie-break order over all compared fields); nil slices
// preserved as nil.
func canonicalReport(r report.ConformanceReport) report.ConformanceReport {
	c := r
	c.Dependencies = sortDependencies(r.Dependencies)
	c.Violations = sortFindings(r.Violations)
	c.Warnings = sortFindings(r.Warnings)
	return c
}

func sortDependencies(deps []report.DependencyBoundary) []report.DependencyBoundary {
	if deps == nil {
		return nil
	}
	sorted := make([]report.DependencyBoundary, len(deps))
	copy(sorted, deps)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Component < sorted[j].Component
	})
	return sorted
}

func sortFindings(findings []report.Finding) []report.Finding {
	if findings == nil {
		return nil
	}
	sorted := make([]report.Finding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if c := compareStrings(string(a.Kind), string(b.Kind)); c != 0 {
			return c < 0
		}
		if c := compareStrings(a.Message, b.Message); c != 0 {
			return c < 0
		}
		if c := compareStrings(a.Location.File, b.Location.File); c != 0 {
			return c < 0
		}
		if a.Location.Line != b.Location.Line {
			return a.Location.Line < b.Location.Line
		}
		return compareStringSlices(a.Evidence, b.Evidence) < 0
	})
	return sorted
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// compareStringSlices compares element-wise; a prefix sorts first and nil is
// treated as empty, giving equal-key findings a deterministic evidence order.
func compareStringSlices(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if c := compareStrings(a[i], b[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// ReadReportFile reads and validates the persisted report artifact at path.
// The read is bounded by MaxReportBytes so an oversized input fails before
// any unbounded allocation; decoding and verdict validation are delegated to
// DecodeReport. Filesystem access stays in this shell package — the report
// package itself holds no filesystem authority (task req 3). Eventual atomic
// writes reuse WriteFileAtomic in the caller (`--report-out`, Task 3).
func ReadReportFile(path string) (PersistedReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return PersistedReport{}, fmt.Errorf("open report artifact %q: %w", path, err)
	}
	defer f.Close()

	// One sentinel byte beyond the cap distinguishes "too large" from
	// "exactly at the cap" without buffering the whole excess.
	data, err := boundedRead(f, MaxReportBytes)
	if err != nil {
		return PersistedReport{}, fmt.Errorf("read report artifact %q: %w", path, err)
	}

	persisted, err := DecodeReport(data)
	if err != nil {
		return PersistedReport{}, fmt.Errorf("decode report artifact %q: %w", path, err)
	}
	return persisted, nil
}
