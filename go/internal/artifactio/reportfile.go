package artifactio

import (
	"fmt"
	"io"
	"os"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

// ReadReportFile reads and validates the persisted report artifact at path.
// The read is bounded by report.MaxReportBytes so an oversized input fails
// before any unbounded allocation; decoding and verdict validation are
// delegated to report.DecodeReport. Filesystem access stays in this shell
// package — the report package itself holds no filesystem authority (task
// req 3). Eventual atomic writes reuse WriteFileAtomic in the caller
// (`--report-out`, Task 3).
func ReadReportFile(path string) (report.PersistedReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return report.PersistedReport{}, fmt.Errorf("open report artifact %q: %w", path, err)
	}
	defer f.Close()

	// One sentinel byte beyond the cap distinguishes "too large" from
	// "exactly at the cap" without buffering the whole excess.
	data, err := io.ReadAll(io.LimitReader(f, report.MaxReportBytes+1))
	if err != nil {
		return report.PersistedReport{}, fmt.Errorf("read report artifact %q: %w", path, err)
	}
	if len(data) > report.MaxReportBytes {
		return report.PersistedReport{}, fmt.Errorf("read report artifact %q: size %d exceeds the %d byte limit", path, len(data), report.MaxReportBytes)
	}

	persisted, err := report.DecodeReport(data)
	if err != nil {
		return report.PersistedReport{}, fmt.Errorf("decode report artifact %q: %w", path, err)
	}
	return persisted, nil
}
