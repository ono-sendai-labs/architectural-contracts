package artifactio

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

// Persisted report artifact contract (task req 1, 2): the canonical
// `<name>.report.json` is the completed ConformanceReport plus an explicit
// verdict derived by report.VerdictOf. MarshalReport is the only encode path
// and derives the verdict itself; DecodeReport re-derives and validates it,
// so a caller cannot stamp a contradictory verdict. ReportFormatVersion is
// the only supported major version; MaxReportBytes caps reads before
// decoding (DR-15).
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
// ConformanceReport plus its explicitly derived verdict. Constructed only by
// MarshalReport (encode) and DecodeReport (decode), keeping the verdict
// derivation authoritative in report.VerdictOf.
type PersistedReport struct {
	FormatVersion int                      `json:"format_version"`
	Verdict       report.Verdict           `json:"verdict"`
	Report        report.ConformanceReport `json:"report"`
}

// MarshalReport returns the canonical JSON bytes of r with its verdict
// derived by report.VerdictOf. The findings collections are canonically
// sorted before encoding, so logically identical reports produce
// byte-identical output regardless of the order the findings were observed
// in.
func MarshalReport(r report.ConformanceReport) ([]byte, error) {
	persisted := PersistedReport{
		FormatVersion: ReportFormatVersion,
		Verdict:       report.VerdictOf(r),
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
	case report.VerdictPass, report.VerdictFail:
	default:
		return PersistedReport{}, fmt.Errorf("%w: %q", ErrUnknownVerdict, string(persisted.Verdict))
	}
	if actual := report.VerdictOf(persisted.Report); actual != persisted.Verdict {
		return PersistedReport{}, fmt.Errorf("%w: verdict %q but report has %d violations", ErrVerdictMismatch, persisted.Verdict, len(persisted.Report.Violations))
	}
	return persisted, nil
}

// canonicalReport returns a deterministic copy of r: findings sorted by kind,
// then message, then file, then line; nil slices preserved as nil.
func canonicalReport(r report.ConformanceReport) report.ConformanceReport {
	c := r
	c.Violations = sortFindings(r.Violations)
	c.Warnings = sortFindings(r.Warnings)
	return c
}

func sortFindings(findings []report.Finding) []report.Finding {
	if findings == nil {
		return nil
	}
	sorted := make([]report.Finding, len(findings))
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
