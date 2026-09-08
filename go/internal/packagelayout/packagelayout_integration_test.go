//go:build integration

package packagelayout

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// goListPackage is the subset of `go list -json` output the oracle comparison
// reads.
type goListPackage struct {
	ImportPath string   `json:"ImportPath"`
	GoFiles    []string `json:"GoFiles"`
}

// TestStdlibLayoutMatchesGoListStd is the whole-stdlib fidelity oracle check
// (task AC 2, design I3): against the host SDK root, the layout builder's
// package set is identical to `go list std` and every package's file list
// equals `go list`'s GoFiles.
func TestStdlibLayoutMatchesGoListStd(t *testing.T) {
	goroot := commandOutputValue(t, "go", "env", "GOROOT")
	version := commandOutputValue(t, "go", "env", "GOVERSION")
	sdkRoot := filepath.Join(goroot, "src")
	layout, err := StdlibLayout(sdkRoot, &Platform{
		GOOS:             "linux",
		GOARCH:           "amd64",
		ToolchainVersion: &version,
	})
	if err != nil {
		t.Fatalf("StdlibLayout() error = %v", err)
	}

	discovered := map[string][]string{}
	for _, p := range layout.Packages {
		discovered[p.PkgPath] = p.GoFiles
	}

	var listed []goListPackage
	dec := json.NewDecoder(strings.NewReader(commandOutput(t, "go", "list", "-json", "std")))
	for {
		var lp goListPackage
		if err := dec.Decode(&lp); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("decoding `go list -json std`: %v", err)
		}
		listed = append(listed, lp)
	}
	if len(listed) == 0 {
		t.Fatal("`go list -json std` returned no packages")
	}
	listedPaths := map[string]bool{}
	for _, lp := range listed {
		listedPaths[lp.ImportPath] = true
		files, ok := discovered[lp.ImportPath]
		if !ok {
			t.Errorf("`go list std` package %q was not discovered by the layout builder", lp.ImportPath)
			continue
		}
		// go list reports GoFiles as base names in the package's Dir; the
		// layout resolves them under the SDK root. Compare base-name sets.
		got := make([]string, len(files))
		for i, f := range files {
			got[i] = filepath.Base(f)
		}
		sort.Strings(got)
		want := append([]string{}, lp.GoFiles...)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("package %q file list %v does not equal go list's GoFiles %v", lp.ImportPath, got, want)
		}
	}
	for path := range discovered {
		if !listedPaths[path] {
			t.Errorf("layout package %q is absent from `go list std`", path)
		}
	}
}

func commandOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return string(out)
}

func commandOutputValue(t *testing.T, name, sub, key string) string {
	t.Helper()
	value := strings.TrimSpace(commandOutput(t, name, sub, key))
	if value == "" {
		t.Fatalf("%s %s %s returned no value", name, sub, key)
	}
	return value
}
