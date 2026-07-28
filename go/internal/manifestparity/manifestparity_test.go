package manifestparity_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifestparity"
)

type spyTB struct {
	testing.TB
	errors []string
}

func (s *spyTB) Errorf(format string, args ...any) {
	s.errors = append(s.errors, fmt.Sprintf(format, args...))
}

func (s *spyTB) Helper() {}

func TestRun_NoArgs_Skips(t *testing.T) {
	manifestparity.Run(t, nil)
}

func TestRun_MissingGeneratedCounterpart(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "mycomp")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chkPath := filepath.Join(compDir, "component.textproto")
	if err := os.WriteFile(chkPath, []byte("name: \"mycomp\"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spy := &spyTB{TB: t}
	manifestparity.Run(spy, []string{chkPath})

	if len(spy.errors) == 0 {
		t.Fatalf("expected Run to report error when generated counterpart is missing, got none")
	}
	found := false
	for _, e := range spy.errors {
		if strings.Contains(e, "checked-in manifest has no generated counterpart") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error containing 'checked-in manifest has no generated counterpart', got %v", spy.errors)
	}
}

func TestRun_MissingCheckedInCounterpart(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "mycomp")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	genPath := filepath.Join(compDir, "mycomp_component.component.textproto")
	if err := os.WriteFile(genPath, []byte("name: \"mycomp_component\"\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spy := &spyTB{TB: t}
	manifestparity.Run(spy, []string{genPath})

	if len(spy.errors) == 0 {
		t.Fatalf("expected Run to report error when checked-in counterpart is missing, got none")
	}
	found := false
	for _, e := range spy.errors {
		if strings.Contains(e, "generated manifest has no checked-in counterpart") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error containing 'generated manifest has no checked-in counterpart', got %v", spy.errors)
	}
}

func TestRun_MatchingPair(t *testing.T) {
	tmpDir := t.TempDir()
	compDir := filepath.Join(tmpDir, "mycomp")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	chkPath := filepath.Join(compDir, "component.textproto")
	manifestContent := []byte("name: \"mycomp\"\ninterface_files: \"mycomp.go\"\n")
	if err := os.WriteFile(chkPath, manifestContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	genContent := []byte("name: \"mycomp_component\"\ninterface_files: \"mycomp.go\"\n")
	genPath := filepath.Join(compDir, "mycomp_component.component.textproto")
	if err := os.WriteFile(genPath, genContent, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	spy := &spyTB{TB: t}
	manifestparity.Run(spy, []string{chkPath, genPath})

	if len(spy.errors) > 0 {
		t.Errorf("unexpected errors for matching pair: %v", spy.errors)
	}
}
