package goanalysis

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

func TestNormalizeStdlibImports(t *testing.T) {
	got := normalizeStdlibImports(map[string]bool{
		"z/canonical": true,
		"a/canonical": true,
	})
	if got == nil {
		t.Fatal("normalizeStdlibImports returned nil")
	}
	if want := []string{"a/canonical", "z/canonical"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeStdlibImports = %v, want %v", got, want)
	}
}

func TestNormalizeStdlibImportsEmptyIsNonNil(t *testing.T) {
	got := normalizeStdlibImports(nil)
	if got == nil {
		t.Fatal("normalizeStdlibImports returned nil for empty input")
	}
	if len(got) != 0 {
		t.Fatalf("normalizeStdlibImports = %v, want empty", got)
	}
}

func TestNormalizeStdlibImportsCanonicalizesAndDeduplicates(t *testing.T) {
	original := hostpolicy.CanonicalizePath
	t.Cleanup(func() { hostpolicy.CanonicalizePath = original })
	hostpolicy.CanonicalizePath = func(path string) string {
		if strings.HasPrefix(path, "host/") {
			return "canonical/" + strings.TrimPrefix(path, "host/")
		}
		return path
	}

	got := normalizeStdlibImports(map[string]bool{
		"host/fmt":      true,
		"canonical/fmt": true,
	})
	if want := []string{"canonical/fmt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeStdlibImports = %v, want %v", got, want)
	}
}
