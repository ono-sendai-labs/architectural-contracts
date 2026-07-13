// Package capslockadapter provides a Capslock-backed implementation of the CapabilityAnalyzer port.
//
// Component Contract (FR10):
// - What it does: Runs capability analysis over Go packages and extracts findings pruned at component boundaries.
// - What it requires: A request specifying packages to analyze and interface symbols to prune at.
// - What it provides: Deterministic lists of CapabilityFindings associated with their transitive call paths.
// - Ambient Authority: This component is a shell component and holds FILES, EXEC, READ_SYSTEM_STATE, OPERATING_SYSTEM, REFLECT, RUNTIME, SYSTEM_CALLS, and UNSAFE_POINTER.
package capslockadapter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/capslock/analyzer"
	"github.com/google/capslock/interesting"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"golang.org/x/tools/go/packages"
)

// fileHandleUseMethods are the (*os.File) methods Capslock's builtin map
// classifies CAPABILITY_FILES but which, under the object-capability model, are
// capability use (operating on an already-obtained handle) rather than ambient
// authority. Reclassifying them SAFE attributes filesystem authority to the
// minting site (os.Open/os.ReadFile/... which take an ambient path) instead of
// to every downstream consumer of the handle.
//
// Deliberately EXCLUDES (*os.File).Chdir, which Capslock classifies as
// MODIFY_SYSTEM_STATE: it mutates process-global cwd (genuine ambient authority),
// not the designated file, so it stays classified.
var fileHandleUseMethods = []string{
	"(*os.File).Chmod", "(*os.File).Chown", "(*os.File).Close", "(*os.File).Fd",
	"(*os.File).Name", "(*os.File).Read", "(*os.File).ReadAt", "(*os.File).ReadDir",
	"(*os.File).ReadFrom", "(*os.File).Readdir", "(*os.File).Readdirnames",
	"(*os.File).Seek", "(*os.File).SetDeadline", "(*os.File).SetReadDeadline",
	"(*os.File).SetWriteDeadline", "(*os.File).Stat", "(*os.File).Sync",
	"(*os.File).SyscallConn", "(*os.File).Truncate", "(*os.File).Write",
	"(*os.File).WriteAt", "(*os.File).WriteString",
}

// buildClassifier constructs a per-run custom capability classifier that:
// 1. Reclassifies the 22 (*os.File) handle-use methods as CAPABILITY_SAFE.
// 2. Adds boundary-prune safe keys to the custom capability map (FR5b).
// 3. Merges the custom map with Capslock's built-ins (excludeBuiltin=false).
// 4. Wraps the result to exclude UNANALYZED helper leaves.
func buildClassifier(pruneAt []capanalyzer.InterfaceSymbol) (analyzer.Classifier, error) {
	var b strings.Builder
	seen := make(map[string]bool)

	for _, k := range fileHandleUseMethods {
		if seen[k] {
			continue
		}
		seen[k] = true
		fmt.Fprintf(&b, "func %s CAPABILITY_SAFE\n", k)
	}

	for _, sym := range pruneAt {
		s := string(sym)
		if s == "" {
			return nil, fmt.Errorf("empty prune symbol key")
		}
		if strings.ContainsAny(s, "\r\n") {
			return nil, fmt.Errorf("prune symbol key %q contains newline characters", s)
		}

		var funcKey string
		if strings.HasPrefix(s, "func ") {
			funcKey = strings.TrimPrefix(s, "func ")
		} else {
			funcKey = s
		}

		if seen[funcKey] {
			continue
		}
		seen[funcKey] = true

		fmt.Fprintf(&b, "func %s CAPABILITY_SAFE\n", funcKey)
	}
	merged, err := interesting.LoadClassifier("arcc-ocap", strings.NewReader(b.String()), false /* excludeBuiltin */)
	if err != nil {
		return nil, fmt.Errorf("failed to load custom capslock classifier: %w", err)
	}
	return interesting.ClassifierExcludingUnanalyzed(merged), nil
}

// mapClass maps a Capslock capability name to its corresponding capanalyzer.Class.
// It returns an error if the capability name is unknown or a control value.
func mapClass(capName string) (capanalyzer.Class, error) {
	// Normalize to documented base category (e.g. "MODIFY_SYSTEM_STATE/ENV" -> "MODIFY_SYSTEM_STATE")
	baseCap := capName
	if idx := strings.Index(capName, "/"); idx != -1 {
		baseCap = capName[:idx]
	}

	switch baseCap {
	case "ARBITRARY_EXECUTION", "CGO", "UNSAFE_POINTER", "REFLECT", "UNANALYZED":
		return capanalyzer.AnalysisDefeating, nil
	case "FILES", "NETWORK", "READ_SYSTEM_STATE", "MODIFY_SYSTEM_STATE", "OPERATING_SYSTEM", "SYSTEM_CALLS", "EXEC", "RUNTIME":
		return capanalyzer.TrueAuthority, nil
	default:
		return "", fmt.Errorf("unsupported or unknown capability name: %q", capName)
	}
}

// Adapter implements the capanalyzer.CapabilityAnalyzer interface using Capslock.
type Adapter struct{}

var _ capanalyzer.CapabilityAnalyzer = (*Adapter)(nil)

// NewAdapter creates a new Capslock-backed capability analyzer adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Analyze loads the requested packages and performs capability analysis.
func (a *Adapter) Analyze(req capanalyzer.AnalyzeRequest) ([]capanalyzer.CapabilityFinding, error) {
	if len(req.Packages) == 0 {
		return nil, fmt.Errorf("package request is empty; at least one package path must be provided")
	}

	classifier, err := buildClassifier(req.PruneAt)
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Mode: analyzer.PackagesLoadModeNeeded,
	}

	pkgs, err := packages.Load(cfg, req.Packages...)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	var errs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("package load errors:\n%s", strings.Join(errs, "\n"))
	}

	queried := analyzer.GetQueriedPackages(pkgs)
	analyzerCfg := &analyzer.Config{
		Classifier:  classifier,
		Granularity: analyzer.GranularityFunction,
	}

	cil := analyzer.GetCapabilityInfo(pkgs, queried, analyzerCfg)

	findings := []capanalyzer.CapabilityFinding{}
	for _, ci := range cil.GetCapabilityInfo() {
		var callPath []capanalyzer.Frame
		for _, fr := range ci.GetPath() {
			site := fr.GetSite()
			callPath = append(callPath, capanalyzer.Frame{
				Func: fr.GetName(),
				File: site.GetFilename(),
				Line: int(site.GetLine()),
			})
		}

		rawCapName := ci.GetCapabilityName()
		class, err := mapClass(rawCapName)
		if err != nil {
			return nil, err
		}

		// Normalize to base category name for the Capability finding field
		normalizedCapName := rawCapName
		if idx := strings.Index(rawCapName, "/"); idx != -1 {
			normalizedCapName = rawCapName[:idx]
		}

		findings = append(findings, capanalyzer.CapabilityFinding{
			Package:    ci.GetPackageDir(),
			Capability: normalizedCapName,
			Class:      class,
			CallPath:   callPath,
		})
	}

	sortFindings(findings)
	return findings, nil
}

// sortFindings sorts the slice of capability findings deterministically.
func sortFindings(findings []capanalyzer.CapabilityFinding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.Capability != b.Capability {
			return a.Capability < b.Capability
		}
		if string(a.Class) != string(b.Class) {
			return string(a.Class) < string(b.Class)
		}
		return compareCallPaths(a.CallPath, b.CallPath) < 0
	})
}

// compareFrames compares two Frames and returns -1 if a < b, 1 if a > b, and 0 if a == b.
func compareFrames(a, b capanalyzer.Frame) int {
	if a.Func != b.Func {
		if a.Func < b.Func {
			return -1
		}
		return 1
	}
	if a.File != b.File {
		if a.File < b.File {
			return -1
		}
		return 1
	}
	if a.Line != b.Line {
		if a.Line < b.Line {
			return -1
		}
		return 1
	}
	return 0
}

// compareCallPaths compares two CallPaths and returns -1 if a < b, 1 if a > b, and 0 if a == b.
func compareCallPaths(a, b []capanalyzer.Frame) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		cmp := compareFrames(a[i], b[i])
		if cmp != 0 {
			return cmp
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
