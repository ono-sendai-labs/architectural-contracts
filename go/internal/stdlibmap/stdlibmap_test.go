package stdlibmap

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizePackageList(t *testing.T) {
	got, err := NormalizePackageList([]string{
		"os", "internal/secret", "strings", "os", "crypto/internal/boring", "", "net/http", "internal", " os ",
	})
	if err == nil {
		t.Fatalf("NormalizePackageList: want error for malformed paths, got nil")
	}
	if !strings.Contains(err.Error(), `""`) && !strings.Contains(err.Error(), `" os "`) {
		t.Fatalf("NormalizePackageList error %q: want it to name the malformed path", err)
	}

	got, err = NormalizePackageList([]string{
		"os", "internal/secret", "strings", "os", "crypto/internal/boring", "net/http", "internal", "strings",
	})
	if err != nil {
		t.Fatalf("NormalizePackageList: %v", err)
	}
	want := []PackageEntry{
		{Path: "crypto/internal/boring", Importable: false},
		{Path: "internal", Importable: false},
		{Path: "internal/secret", Importable: false},
		{Path: "net/http", Importable: true},
		{Path: "os", Importable: true},
		{Path: "strings", Importable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizePackageList = %+v, want %+v", got, want)
	}
}

func TestNormalizePackageListMalformed(t *testing.T) {
	for _, p := range []string{"", " os", "os/", "/os", "a//b", "a/../b", "./a", "a\\b", "a:b", "example..com/pkg", "a~b"} {
		if _, err := NormalizePackageList([]string{p}); err == nil {
			t.Fatalf("NormalizePackageList(%q): want error, got nil", p)
		} else if !strings.Contains(err.Error(), "package path") && !strings.Contains(err.Error(), "import path") {
			t.Fatalf("NormalizePackageList(%q): error %q lacks actionable context", p, err)
		}
	}
}

func TestIsInternalPath(t *testing.T) {
	for p, want := range map[string]bool{
		"os": false, "internal": true, "internal/secret": true,
		"crypto/internal/boring": true, "internalx/os": false, "x/osinternal": false,
	} {
		if got := IsInternalPath(p); got != want {
			t.Fatalf("IsInternalPath(%q) = %t, want %t", p, got, want)
		}
	}
}

func TestParseGoListOutputWhitespacePortable(t *testing.T) {
	got, err := parseGoListOutput("os\r\nstrings\tfmt\n\nos\r\n")
	if err != nil {
		t.Fatalf("parseGoListOutput: %v", err)
	}
	want := []PackageEntry{
		{Path: "fmt", Importable: true},
		{Path: "os", Importable: true},
		{Path: "strings", Importable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseGoListOutput = %+v, want %+v (CRLF and tab separators must not corrupt paths)", got, want)
	}
}

func TestExplicitPackageList(t *testing.T) {
	oracle := ExplicitPackageList([]string{"b", "internal/x", "a", "b"})
	raw, err := oracle.Packages()
	if err != nil {
		t.Fatalf("ExplicitPackageList.Packages: %v", err)
	}
	if len(raw) != 4 {
		t.Fatalf("raw oracle entries = %d, want the unnormalized 4", len(raw))
	}
	got, err := NormalizePackageList(entriesToPaths(raw))
	if err != nil {
		t.Fatalf("NormalizePackageList: %v", err)
	}
	want := []PackageEntry{
		{Path: "a", Importable: true},
		{Path: "b", Importable: true},
		{Path: "internal/x", Importable: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized = %+v, want %+v", got, want)
	}
}

func entriesToPaths(entries []PackageEntry) []string {
	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.Path
	}
	return paths
}
