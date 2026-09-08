//go:build integration

package capslockadapter

import (
	"testing"
)

// TestGenerationClassifierPreservesUnanalyzed is the I4 regression pin: the
// generation classifier keeps CAPABILITY_UNANALYZED (io.ReadAll is a
// curated-unanalyzed helper). The check-time classifier — and with it the
// ClassifierExcludingUnanalyzed wrapper — is deleted with the Capslock check
// path; only the generation classifier remains.
func TestGenerationClassifierPreservesUnanalyzed(t *testing.T) {
	gen, err := NewGenerationClassifier()
	if err != nil {
		t.Fatalf("NewGenerationClassifier: %v", err)
	}
	if got := gen.FunctionCategory("", "io.ReadAll"); got != "UNANALYZED" {
		t.Fatalf("generation classifier io.ReadAll = %q; want UNANALYZED (I4)", got)
	}
}
