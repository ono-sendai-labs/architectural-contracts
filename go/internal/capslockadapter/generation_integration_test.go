//go:build integration

package capslockadapter

import (
	"testing"
)

// TestGenerationClassifierPreservesUnanalyzed is the I4 regression pin: the
// generation classifier keeps CAPABILITY_UNANALYZED (io.ReadAll is a
// curated-unanalyzed helper), while the check-time classifier strips it. The
// map generator must run under the generation classifier only.
func TestGenerationClassifierPreservesUnanalyzed(t *testing.T) {
	gen, err := NewGenerationClassifier()
	if err != nil {
		t.Fatalf("NewGenerationClassifier: %v", err)
	}
	if got := gen.FunctionCategory("", "io.ReadAll"); got != "UNANALYZED" {
		t.Fatalf("generation classifier io.ReadAll = %q; want UNANALYZED (I4)", got)
	}
	check, err := buildClassifier(nil)
	if err != nil {
		t.Fatalf("buildClassifier: %v", err)
	}
	if got := check.FunctionCategory("", "io.ReadAll"); got != "" {
		t.Fatalf("check-time classifier io.ReadAll = %q; want empty (excluded)", got)
	}
}
