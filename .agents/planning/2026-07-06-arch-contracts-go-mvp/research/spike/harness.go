// Package spike is a preserved, re-runnable harness that drives the Capslock
// library (github.com/google/capslock/analyzer) the same way the future
// capslockadapter will, so we can *execute* — not just read — the load-bearing
// Capslock assumptions the Architectural Contracts design rests on (plan Step 0).
//
// Run it with:  go test -v ./...   (from this module's directory)
//
// It is intentionally a separate Go module, pinned to a released Capslock version
// (see go.mod), so the product module under go/ does not depend on Capslock until
// Step 7.
package spike

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/capslock/analyzer"
	"github.com/google/capslock/interesting"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Finding is a flattened Capslock CapabilityInfo: which function in a queried
// package can reach which capability, plus the example call path as evidence.
type Finding struct {
	PackageDir string
	Function   string // the queried function (path frame 0)
	Capability string
	Type       string // DIRECT | TRANSITIVE
	Path       []string
}

func (f Finding) String() string {
	return fmt.Sprintf("%-22s %-14s %s", f.Capability, f.Type, f.Function)
}

// loadPackages loads the given patterns as a single program rooted at dir, using
// the exact load mode Capslock needs. Errors in loaded packages are returned.
func loadPackages(dir string, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{Mode: analyzer.PackagesLoadModeNeeded, Dir: dir}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
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
	return pkgs, nil
}

// analyze runs Capslock over pkgs (queried = the loaded top-level packages) with
// the given classifier and returns the findings, sorted for stable output.
func analyze(pkgs []*packages.Package, classifier analyzer.Classifier) []Finding {
	queried := analyzer.GetQueriedPackages(pkgs)
	cfg := &analyzer.Config{Classifier: classifier, Granularity: analyzer.GranularityFunction}
	cil := analyzer.GetCapabilityInfo(pkgs, queried, cfg)
	var out []Finding
	for _, ci := range cil.GetCapabilityInfo() {
		f := Finding{
			PackageDir: ci.GetPackageDir(),
			Capability: ci.GetCapabilityName(),
			Type:       strings.TrimPrefix(ci.GetCapabilityType().String(), "CAPABILITY_TYPE_"),
		}
		for i, fr := range ci.GetPath() {
			if i == 0 {
				f.Function = fr.GetName()
			}
			site := fr.GetSite()
			f.Path = append(f.Path, fmt.Sprintf("%s (%s:%d)", fr.GetName(), filepath.Base(site.GetFilename()), site.GetLine()))
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Function != out[j].Function {
			return out[i].Function < out[j].Function
		}
		return out[i].Capability < out[j].Capability
	})
	return out
}

// strictClassifier is the classifier the design should use for the MVP
// StrictPolicy (spike finding): Capslock's builtins with UNANALYZED *excluded*.
// Excluding UNANALYZED makes Capslock descend through functions like io.ReadAll,
// errors.Is and the bufio readers instead of treating them as analysis-defeating
// leaves. That (a) keeps genuinely-clean code's capability set empty and (b)
// unmasks the io.Reader→*os.File VTA flow the design's §11 attribution relies on.
func strictClassifier() analyzer.Classifier { return analyzer.GetClassifier(true) }

// unanalyzedVisibleClassifier keeps UNANALYZED visible — used only for the
// spike's contrast, to show what the builtin default reports.
func unanalyzedVisibleClassifier() analyzer.Classifier { return analyzer.GetClassifier(false) }

// fileHandleUseMethods are the (*os.File) methods Capslock's builtin map
// classifies CAPABILITY_FILES but which, under the object-capability model, are
// *capability use* (operating on an already-obtained handle) rather than ambient
// authority. Reclassifying them SAFE attributes filesystem authority to the
// *minting* site (os.Open/os.ReadFile/... which take an ambient path) instead of
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

// ocapClassifier realizes the "ambient authority = capability minting, not
// capability use" refinement: it starts from the strict builtins and reclassifies
// the file-handle *use* methods SAFE. Minting functions (os.Open, os.ReadFile,
// os.Create, os.NewFile, ...) keep their FILES classification, so authority is
// attributed at the site that turns an ambient designator into a capability.
func ocapClassifier() (analyzer.Classifier, error) {
	var b strings.Builder
	for _, k := range fileHandleUseMethods {
		fmt.Fprintf(&b, "func %s CAPABILITY_SAFE\n", k)
	}
	merged, err := interesting.LoadClassifier("spike-ocap", strings.NewReader(b.String()), false /* excludeBuiltin */)
	if err != nil {
		return nil, err
	}
	return interesting.ClassifierExcludingUnanalyzed(merged), nil
}

// pruneClassifier builds a per-run custom capability map marking each key
// CAPABILITY_SAFE (terminating traversal), merges it with the builtins, and then
// excludes UNANALYZED — exactly the StrictPolicy + boundary-pruning combination
// the capslockadapter will use for component dependencies (FR5b).
func pruneClassifier(safeKeys ...string) (analyzer.Classifier, error) {
	var b strings.Builder
	for _, k := range safeKeys {
		fmt.Fprintf(&b, "func %s CAPABILITY_SAFE\n", k)
	}
	merged, err := interesting.LoadClassifier("spike-prune", strings.NewReader(b.String()), false /* excludeBuiltin */)
	if err != nil {
		return nil, err
	}
	return interesting.ClassifierExcludingUnanalyzed(merged), nil
}

// ssaKeys returns the sorted ssa.Function.String() keys for the loaded packages —
// the exact key form Capslock's classifier matches against, so we can confirm the
// A4 normalization contract (method receiver forms, generic bracket-stripping,
// promoted-method wrappers, init).
func ssaKeys(pkgs []*packages.Package) []string {
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	seen := map[string]struct{}{}
	for fn := range ssautil.AllFunctions(prog) {
		seen[fn.String()] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// rawGraphHasEdge builds a plain VTA call graph over pkgs (without Capslock's
// sort/Once call-site rewrites) and reports whether a function whose name
// contains callerSub calls the function whose ssa key is exactly calleeKey. Used
// as evidence that Capslock's clean result for sort.Sort/sort.Slice is due to the
// rewrite, not to a missing edge.
func rawGraphHasEdge(pkgs []*packages.Package, callerSub, calleeKey string) bool {
	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	all := ssautil.AllFunctions(prog)
	graph := vta.CallGraph(all, nil)
	for fn := range all {
		if !strings.Contains(fn.Name(), callerSub) {
			continue
		}
		node := graph.Nodes[fn]
		if node == nil {
			continue
		}
		for _, e := range node.Out {
			if e.Callee != nil && e.Callee.Func != nil && e.Callee.Func.String() == calleeKey {
				return true
			}
		}
	}
	return false
}

// hasCapability reports whether any finding carries the given capability.
func hasCapability(fs []Finding, cap string) bool {
	for _, f := range fs {
		if f.Capability == cap {
			return true
		}
	}
	return false
}

// functionHasCapability reports whether some finding whose function name contains
// fnSubstr carries cap.
func functionHasCapability(fs []Finding, fnSubstr, cap string) bool {
	for _, f := range fs {
		if strings.Contains(f.Function, fnSubstr) && f.Capability == cap {
			return true
		}
	}
	return false
}

// anyFunctionContains reports whether any finding's function name contains sub.
func anyFunctionContains(fs []Finding, sub string) bool {
	for _, f := range fs {
		if strings.Contains(f.Function, sub) {
			return true
		}
	}
	return false
}
