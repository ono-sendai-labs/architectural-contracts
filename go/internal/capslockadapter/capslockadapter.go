package capslockadapter

import (
	"fmt"
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
	for _, k := range fileHandleUseMethods {
		fmt.Fprintf(&b, "func %s CAPABILITY_SAFE\n", k)
	}
	for _, sym := range pruneAt {
		fmt.Fprintf(&b, "func %s CAPABILITY_SAFE\n", string(sym))
	}
	merged, err := interesting.LoadClassifier("arcc-ocap", strings.NewReader(b.String()), false /* excludeBuiltin */)
	if err != nil {
		return nil, fmt.Errorf("failed to load custom capslock classifier: %w", err)
	}
	return interesting.ClassifierExcludingUnanalyzed(merged), nil
}

// mapClass maps a Capslock capability name to its corresponding capanalyzer.Class.
func mapClass(capName string) capanalyzer.Class {
	switch capName {
	case "ARBITRARY_EXECUTION", "CGO", "UNSAFE_POINTER", "REFLECT", "UNANALYZED":
		return capanalyzer.AnalysisDefeating
	default:
		return capanalyzer.TrueAuthority
	}
}

// Adapter implements the capanalyzer.CapabilityAnalyzer interface using Capslock.
type Adapter struct{}

// NewAdapter creates a new Capslock-backed capability analyzer adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// Analyze loads the requested packages and performs capability analysis.
func (a *Adapter) Analyze(req capanalyzer.AnalyzeRequest) ([]capanalyzer.CapabilityFinding, error) {
	if len(req.Packages) == 0 {
		return nil, nil
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

	var findings []capanalyzer.CapabilityFinding
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

		findings = append(findings, capanalyzer.CapabilityFinding{
			Package:    ci.GetPackageDir(),
			Capability: ci.GetCapabilityName(),
			Class:      mapClass(ci.GetCapabilityName()),
			CallPath:   callPath,
		})
	}

	return findings, nil
}
