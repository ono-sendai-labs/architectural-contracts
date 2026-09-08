package capslockadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/google/capslock/analyzer"
	"github.com/google/capslock/interesting"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"golang.org/x/tools/go/packages"
)

// The generation classifier (design I4, DR-05) is the classifier map
// generation runs under: Capslock's builtins merged with the object-capability
// minting-site reclassification, and — unlike the check-time buildClassifier —
// deliberately NOT wrapped in interesting.ClassifierExcludingUnanalyzed. The
// wrapper deletes CAPABILITY_UNANALYZED findings, which would launder
// unanalysable stdlib bodies such as sort.Slice into apparent purity (spike
// finding 5); generation must preserve them as terminal UNANALYZED records.

// GenerationClassifierText returns the generation classifier's source text:
// exactly the minting-site reclassification lines. It shares the check-time
// builder so the two classifiers cannot drift, and its rules material is the
// source hashed into the SDK key's classifier_hash (I2, task req 2).
func GenerationClassifierText() (string, error) {
	return buildClassifierText(nil)
}

// builtinPinSeparator/entry delimit the canonical builtin-content rendering
// hashed by classifierContentDigest; the separators never appear in the
// extracted keys or values.
const (
	builtinPinEntry     = "\x1f"
	builtinPinSeparator = "\x1e"
)

// BuiltinClassifierPin returns a cryptographically stable content pin for
// Capslock's embedded builtin classifier (design I2, task req 2): the hex
// SHA-256 of a canonical, order-independent rendering of the classifier's
// exact semantic content — every function and package classification, every
// unanalyzed declaration, every ignored edge and cgo suffix. Any change to
// the builtin classifier content (for example through a Capslock dependency
// update) necessarily changes the pin, and with it the SDK key's
// classifier_hash and the native cache identity.
func BuiltinClassifierPin() (string, error) {
	return classifierContentDigest(interesting.DefaultClassifier())
}

// classifierContentDigest renders one classifier's full semantic content
// canonically and digests it. Capslock's Classifier exposes its content only
// through unexported map fields, so the rendering reads those fields with
// reflection; read-only reflection is sufficient (the read-only flag permits
// Len, MapKeys/MapIndex, Index and String). The extraction fails loudly — an
// error, never a partial pin — if a Capslock upgrade changes the classifier's
// field structure, so generation fails closed rather than fingerprinting a
// silently weakened description.
func classifierContentDigest(c *interesting.Classifier) (string, error) {
	if c == nil {
		return "", fmt.Errorf("digesting the builtin classifier: the classifier is nil")
	}
	v := reflect.ValueOf(c).Elem()
	var lines []string
	addMap := func(name string, renderKey func(reflect.Value) string, hasValue bool) error {
		f := v.FieldByName(name)
		if !f.IsValid() || f.Kind() != reflect.Map {
			return fmt.Errorf("digesting the builtin classifier: field %q is missing or not a map (Capslock's Classifier structure changed)", name)
		}
		// Capslock's own loader panics when the embedded builtin map is
		// empty (parseInternalMapOrDie), so no empty-classifier guard is
		// needed here; the digest is simply faithful to the content.
		for _, k := range f.MapKeys() {
			line := name + builtinPinEntry + renderKey(k)
			if hasValue {
				line += builtinPinEntry + f.MapIndex(k).String()
			}
			lines = append(lines, line)
		}
		return nil
	}
	if err := addMap("functionCategory", func(k reflect.Value) string { return k.String() }, true); err != nil {
		return "", err
	}
	if err := addMap("unanalyzedCategory", func(k reflect.Value) string { return k.String() }, true); err != nil {
		return "", err
	}
	if err := addMap("packageCategory", func(k reflect.Value) string { return k.String() }, true); err != nil {
		return "", err
	}
	if err := addMap("ignoredEdges", func(k reflect.Value) string {
		return k.Index(0).String() + builtinPinEntry + k.Index(1).String()
	}, false); err != nil {
		return "", err
	}
	sfx := v.FieldByName("cgoSuffixes")
	if !sfx.IsValid() || sfx.Kind() != reflect.Slice {
		return "", fmt.Errorf("digesting the builtin classifier: field %q is missing or not a slice (Capslock's Classifier structure changed)", "cgoSuffixes")
	}
	for i := 0; i < sfx.Len(); i++ {
		lines = append(lines, "cgoSuffix"+builtinPinEntry+sfx.Index(i).String())
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, builtinPinSeparator)))
	return hex.EncodeToString(h[:]), nil
}

// NewGenerationClassifier constructs the generation classifier: Capslock's
// builtins plus the minting-site reclassification, without the
// UNANALYZED-excluding wrapper (design I4).
func NewGenerationClassifier() (analyzer.Classifier, error) {
	text, err := GenerationClassifierText()
	if err != nil {
		return nil, err
	}
	merged, err := interesting.LoadClassifier("arcc-ocap-generation", strings.NewReader(text), false /* excludeBuiltin */)
	if err != nil {
		return nil, fmt.Errorf("failed to load generation classifier: %w", err)
	}
	return merged, nil
}

// mintingAuthority maps the receiver type of the minting-site rule's
// reclassified handle-use methods to the ambient capability the minting site
// moves to itself. Under the ocap model, a method operating on an
// already-obtained handle ((*os.File).Read, ...) is capability use, not
// ambient authority; the ambient authority it exercises was minted where the
// handle was created (os.Open, os.ReadFile, ...). A variable of such a handle
// type (os.Stdin) is a pre-minted handle: it inherits the minted capability
// (FILES) so a pre-minted handle is not misread as pure, while ordinary
// handle *use* stays SAFE (DR-05 var rule).
//
// The table is derived from fileHandleUseMethods, all of which Capslock's
// builtin classifier assigns FILES (spike finding 3); if a reclassification
// is ever added for another receiver or capability, this table must be
// extended — the generation_test.go cross-check enforces the correspondence.
func MintingAuthority() map[string]string {
	return map[string]string{
		"os.File": "FILES",
	}
}

// GenerationFinding is one Capslock root finding prepared for map
// generation: the root function's structured identity (display name plus
// declaring package — Path[0] under GranularityFunction), the capability
// name (base category, a /sub-category, or the CAPABILITY_SAFE /
// CAPABILITY_UNANALYZED control names), and the ordered root-to-use path.
type GenerationFinding struct {
	// RootName is the Capslock display spelling of Path[0] (proto.Function.Name).
	RootName string
	// RootPackage is the structured package field of Path[0]
	// (proto.Function.Package); the import path declaring the root.
	RootPackage string
	// Capability is the finding's capability name (proto.CapabilityInfo.capability_name).
	Capability string
	// Path is the finding's ordered caller-to-use path (Path[0] is the root).
	Path []stdlibauthority.Frame
}

// GenerationFindings runs Capslock ONCE over the complete batch of package
// paths in the host environment (GenerationFindingsForEnv with nil env).
func GenerationFindings(packagePaths []string) ([]GenerationFinding, error) {
	return GenerationFindingsForEnv(nil, packagePaths)
}

// GenerationFindingsForEnv is GenerationFindings bound to a complete target
// environment: env is the COMPLETE environment the toolchain runs in (the
// merged host + target overrides, NativeLoader.Environment), so the Capslock
// analysis describes the same target configuration as the inventory loader
// and package oracle — never the host (round-1 finding). Grouping by Path[0]
// is the caller's (stdlibmap's) responsibility. packagePaths must be
// non-empty. Any package load error fails the whole batch.
func GenerationFindingsForEnv(env []string, packagePaths []string) ([]GenerationFinding, error) {
	if len(packagePaths) == 0 {
		return nil, fmt.Errorf("generation findings: the package batch is empty")
	}
	classifier, err := NewGenerationClassifier()
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{Mode: analyzer.PackagesLoadModeNeeded, Env: env}
	pkgs, err := packages.Load(cfg, packagePaths...)
	if err != nil {
		return nil, fmt.Errorf("loading %d SDK packages for generation: %w", len(packagePaths), err)
	}
	var errs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("generation findings: package load errors:\n%s", strings.Join(errs, "\n"))
	}
	queried := analyzer.GetQueriedPackages(pkgs)
	cil := analyzer.GetCapabilityInfo(pkgs, queried, &analyzer.Config{
		Classifier:  classifier,
		Granularity: analyzer.GranularityFunction,
	})
	out := make([]GenerationFinding, 0, len(cil.GetCapabilityInfo()))
	for _, ci := range cil.GetCapabilityInfo() {
		root := ci.GetPath()[0]
		f := GenerationFinding{
			RootName:    root.GetName(),
			RootPackage: root.GetPackage(),
			Capability:  ci.GetCapabilityName(),
		}
		for _, fr := range ci.GetPath() {
			site := fr.GetSite()
			f.Path = append(f.Path, stdlibauthority.Frame{
				Function: normalizeFrameName(fr.GetName()),
				File:     site.GetFilename(),
				Line:     int(site.GetLine()),
			})
		}
		out = append(out, f)
	}
	return out, nil
}

// normalizeFrameName strips every balanced type-argument bracket group from a
// Capslock/SSA function spelling for persistence. The design never persists
// generic brackets ("generic brackets are never persisted", DR-04): the same
// instantiation prints with different equivalent type spellings between runs
// (e.g. "[]io/fs.DirEntry" vs "[]os.DirEntry" — the same type reached through
// different alias spellings), which would make evidence bytes depend on
// analyzer iteration order. Go identifiers cannot contain brackets, so every
// bracket group in a display name is a type-argument group and safe to drop.
func normalizeFrameName(fn string) string {
	for {
		open := strings.IndexByte(fn, '[')
		if open < 0 {
			return fn
		}
		depth, close := 0, -1
		for i := open; i < len(fn); i++ {
			switch fn[i] {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					close = i
				}
			}
			if close >= 0 {
				break
			}
		}
		if close < 0 {
			return fn
		}
		fn = fn[:open] + fn[close+1:]
	}
}

// ReclassifiedHandleUseMethods returns the method keys the minting-site rule
// reclassifies CAPABILITY_SAFE, in the classifier's spelling ("(*os.File).Read").
// It is the source material for the generation rule descriptor and for the
// map's project-override provenance.
func ReclassifiedHandleUseMethods() []string {
	return append([]string(nil), fileHandleUseMethods...)
}

// CuratedSafeKeys loads the generation classifier once and reports which of
// the given classifier spellings (Capslock/SSA display names, e.g.
// "os.Exit", "(*os.File).Read") are classified CAPABILITY_SAFE. Curated-safe
// and minting-reclassified roots produce no Capslock findings, so their SAFE
// classification and provenance must come from the classifier itself; the
// caller distinguishes the minting override via the reclassified-key list.
func CuratedSafeKeys(names []string) (map[string]bool, error) {
	cl, err := NewGenerationClassifier()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		if cl.FunctionCategory("", name) == "SAFE" {
			out[name] = true
		}
	}
	return out, nil
}
