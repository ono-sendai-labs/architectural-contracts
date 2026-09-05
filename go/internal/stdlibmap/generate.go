package stdlibmap

import (
	"bytes"
	"fmt"
	"go/types"
	"regexp"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capslockadapter"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// Provenance vocabulary for SAFE records (task req 8, DR-05): Capslock's
// curated-safe curation, this project's own overriding rules, and
// analysis-proved purity are visibly distinct trust decisions. Capability and
// UNANALYZED records carry no provenance — their classification is not a
// trust decision.
const (
	ProvenanceCapslockCurated = "capslock-curated"
	ProvenanceProjectOverride = "project-override"
	ProvenanceProvedPure      = "proved-pure"
)

// provenanceStructural is the empty provenance of structurally safe records
// (consts, plain types, interface-typed and methodless vars): they can never
// hold authority, so no trust decision is recorded.
const provenanceStructural = ""

// FindingSource is the injectable Capslock seam behind Generate: one batched
// GranularityFunction run over the complete importable package batch (task
// req 1). capslockadapter.GenerationFindings is the native implementation.
type FindingSource interface {
	Findings(paths []string) ([]capslockadapter.GenerationFinding, error)
}

// FindingSourceFunc adapts a function to the FindingSource seam.
type FindingSourceFunc func(paths []string) ([]capslockadapter.GenerationFinding, error)

// Findings implements FindingSource.
func (f FindingSourceFunc) Findings(paths []string) ([]capslockadapter.GenerationFinding, error) {
	return f(paths)
}

// GeneratedMap is one completed generation: the canonical map message, its
// canonical bytes, and the digest over those bytes (DR-15).
type GeneratedMap struct {
	Map    *gen.StdlibMap
	Bytes  []byte
	Digest string
}

// GenerationInput carries every seam and configuration Generate needs: the
// target configuration and rule version for the SDK key, the package oracle,
// the batch loader, and the Capslock findings source.
type GenerationInput struct {
	// Target describes the target build configuration (DR-09).
	Target TargetConfig
	// RuleVersion is the generation classification rules' explicit version.
	RuleVersion string
	// Oracle enumerates the SDK's packages (total, any order).
	Oracle PackageOracle
	// Loader type-loads the importable packages for the target.
	Loader Loader
	// Findings runs Capslock once over the complete importable batch.
	Findings FindingSource
}

// Generate produces the total stdlib authority map (task reqs 1–10): it
// inventories the SDK independently, runs the Capslock batch once, groups the
// findings by root, completes every inventoried symbol and init with a
// terminal classification under the design's object-kind rules, reconciles
// the exact inventory equality, and emits canonical bytes through the
// artifact I/O boundary. It fails closed: any inventory gap, unaccounted
// analyzer root, or validation error means no map.
func Generate(in GenerationInput) (*GeneratedMap, error) {
	if in.Oracle == nil {
		return nil, fmt.Errorf("generating the stdlib map: the package oracle is required")
	}
	if in.Loader == nil {
		return nil, fmt.Errorf("generating the stdlib map: the package loader is required")
	}
	if in.Findings == nil {
		return nil, fmt.Errorf("generating the stdlib map: the Capslock findings source is required")
	}
	entries, err := in.Oracle.Packages()
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	inv, err := BuildInventory(entries, in.Loader)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	var importable []string
	for _, p := range inv.Packages {
		if p.Importable {
			importable = append(importable, p.Path)
		}
	}
	findings, err := in.Findings.Findings(importable)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	m, err := BuildAuthorityMap(inv, findings)
	if err != nil {
		return nil, err
	}
	m.FormatVersion = artifactio.MapFormatVersion
	m.Key = sdkKeyProto(in)
	data, err := artifactio.MarshalMap(m)
	if err != nil {
		return nil, fmt.Errorf("emitting the stdlib map: %w", err)
	}
	// Round-trip the result through the canonical reader before returning
	// (task approach step 4): the bytes must validate as the persisted form.
	decoded, err := artifactio.DecodeMap(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("round-tripping the generated map: %w", err)
	}
	if decoded.Key == nil || decoded.Key.ToolchainVersion != in.Target.ToolchainVersion ||
		decoded.Key.Goos != in.Target.GOOS || decoded.Key.Goarch != in.Target.GOARCH {
		return nil, fmt.Errorf("round-tripping the generated map: the decoded key does not match the target")
	}
	digest, err := artifactio.MapDigest(m)
	if err != nil {
		return nil, fmt.Errorf("digesting the generated map: %w", err)
	}
	return &GeneratedMap{Map: m, Bytes: data, Digest: digest}, nil
}

// GenerationClassifierRules renders the generation classifier's hashable rule
// descriptor (task req 2): the Capslock builtins, the minting-site
// reclassification, the var-rule minting authority, and the unsafe-builtin
// hardcoding. The reclassification effect is derived from
// capslockadapter's shared source material, so the descriptor cannot drift
// from the classifier actually applied (I2).
func GenerationClassifierRules() []ClassifierRule {
	methods := capslockadapter.ReclassifiedHandleUseMethods()
	var minted []string
	for recv, cap := range capslockadapter.MintingAuthority() {
		minted = append(minted, recv+" → "+cap)
	}
	sort.Strings(minted)
	return []ClassifierRule{
		{
			Name: "capslock.builtins",
			Effect: "Capslock's builtin capability map is included (excludeBuiltin=false); " +
				"CAPABILITY_UNANALYZED findings are preserved as terminal UNANALYZED records (I4); " +
				"the check-time ClassifierExcludingUnanalyzed wrapper is NOT applied.",
		},
		{
			Name: "minting.reclassification",
			Effect: "The minting-site rule reclassifies the following handle-use methods CAPABILITY_SAFE: " +
				strings.Join(methods, ", "),
		},
		{
			Name: "minting.authority",
			Effect: "A variable whose pointer-dereferenced static type owns a reclassified handle-use " +
				"method inherits the ambient capability the minting site moved to itself: " + strings.Join(minted, ", "),
		},
		{
			Name: "unsafe.builtins",
			Effect: "Exported unsafe.* compiler builtins (*types.Builtin values with no SSA roots) " +
				"are hardcoded UNANALYZED.",
		},
	}
}

// sdkKeyProto derives the target SDK key from the generation descriptor and
// converts it to its persisted proto form (task req 10).
func sdkKeyProto(in GenerationInput) *gen.SDKKey {
	rules := GenerationClassifierRules()
	text, err := CanonicalClassifierText(rules)
	if err != nil {
		// The rule set is a fixed, duplicate-free literal; a derivation
		// failure is a programming error and must not become a silent
		// unhashed key.
		panic(fmt.Sprintf("generating the stdlib map: canonicalizing the classifier rules: %v", err))
	}
	key := DeriveSDKKey(GenerationDescriptor{
		Target:           in.Target,
		ClassifierText:   text,
		RuleVersion:      in.RuleVersion,
		MapFormatVersion: artifactio.MapFormatVersion,
	})
	return &gen.SDKKey{
		ToolchainVersion: key.ToolchainVersion,
		Goos:             key.GOOS,
		Goarch:           key.GOARCH,
		CgoEnabled:       key.CgoEnabled,
		BuildTags:        key.BuildTags,
		Goexperiment:     key.GOEXPERIMENT,
		ClassifierHash:   key.ClassifierHash,
		MapFormatVersion: key.MapFormatVersion,
	}
}

// --- root normalization (task req 3) --------------------------------------------

// dropReason records why an analyzer root carries no map record. Only the
// design's sanctioned causes may drop a root; anything else fails generation
// (task req 9).
type dropReason string

const (
	dropClosure         dropReason = "closure"
	dropInitSubFunction dropReason = "init#N"
	dropUnexported      dropReason = "unexported helper"
	dropInstantiation   dropReason = "instantiation"
)

var initSubPattern = regexp.MustCompile(`\.init#\d+$`)

// normalizeRoot resolves one Capslock root finding to the inventoried
// SymbolID it declares, or returns a sanctioned drop reason. It runs through
// the structured package/inventory API (symbol.ParseCapslockFunction over the
// Step 3 Inventory), so normalization fails closed unless the inventory
// confirms exactly the one canonical declaration.
func normalizeRoot(inv *Inventory, f capslockadapter.GenerationFinding) (symbol.SymbolID, dropReason, error) {
	name := f.RootName
	if strings.Contains(name, "$") {
		return "", dropClosure, nil
	}
	pkg := f.RootPackage
	// Aggregate init and source-level init#N: the aggregate's findings union
	// every init#N already (spike finding 4), so the aggregate is the only
	// record and the sub-functions are dropped.
	seg := name
	if dot := strings.LastIndexByte(seg, '.'); dot >= 0 {
		seg = seg[dot+1:]
	}
	if seg == "init" || initSubPattern.MatchString(name) {
		if seg != "init" {
			return "", dropInitSubFunction, nil
		}
		if pkg == "" || !inv.KnownPackage(pkg) {
			return "", "", fmt.Errorf("capslock root %q names an unknown package %q", name, pkg)
		}
		id, err := initID(pkg)
		if err != nil {
			return "", "", fmt.Errorf("normalizing capslock root %q: %w", name, err)
		}
		if !inv.hasInit(pkg) {
			return "", "", fmt.Errorf("capslock root %q: package %q has no inventoried aggregate init", name, pkg)
		}
		return id, "", nil
	}
	id, err := symbol.ParseCapslockFunction(symbol.CapslockFunction{Name: name, Package: pkg}, inv)
	if err == nil {
		return id, "", nil
	}
	// Sanctioned non-roots. A receiver type or trailing identifier that is
	// unexported is a helper folded into its exported callers; a bracket
	// group anywhere is an instantiation (uninstantiated generic origins are
	// their own roots and normalize above). Everything else is an exported
	// root the inventory cannot account for, which fails generation rather
	// than guessing (task req 3).
	if strings.ContainsAny(name, "[]") {
		return "", dropInstantiation, nil
	}
	declared := name
	if strings.HasPrefix(name, "(") {
		// Method spelling "(*pkg.T).M" or "(pkg.T).M": the receiver type name
		// and the method name must both be exported to be a persisted root.
		end := strings.IndexByte(name, ')')
		if end < 0 {
			return "", "", fmt.Errorf("normalizing capslock root %q: %w", name, err)
		}
		recv := strings.TrimPrefix(name[1:end], "*")
		typeName := recv
		if dot := strings.LastIndexByte(recv, '.'); dot >= 0 {
			typeName = recv[dot+1:]
		}
		method := name[end+2:]
		if !tokenIsExported(typeName) || !tokenIsExported(method) {
			return "", dropUnexported, nil
		}
		return "", "", fmt.Errorf("capslock root %q: the inventory does not confirm the declaration (%w)", name, err)
	}
	if dot := strings.LastIndexByte(declared, '.'); dot >= 0 {
		declared = declared[dot+1:]
	}
	if !tokenIsExported(declared) {
		return "", dropUnexported, nil
	}
	return "", "", fmt.Errorf("capslock root %q: the inventory does not confirm the declaration (%w)", name, err)
}

// tokenIsExported reports whether a Go identifier is exported.
func tokenIsExported(name string) bool {
	return name != "" && name[0] != '_' && name[0] < 0x80 && name[0] >= 'A' && name[0] <= 'Z'
}

// --- classification --------------------------------------------------------------

type capKind int

const (
	capAuthority  capKind = iota // a base capability in the shared taxonomy
	capSafe                      // Capslock's CAPABILITY_SAFE curation
	capUnanalyzed                // Capslock's CAPABILITY_UNANALYZED
)

// normalizeCapability classifies one Capslock capability name: the control
// names CAPABILITY_SAFE and CAPABILITY_UNANALYZED, or a taxonomy capability
// normalized to its base category ("MODIFY_SYSTEM_STATE/CHDIR" →
// "MODIFY_SYSTEM_STATE").
func normalizeCapability(capName string) (string, capKind, error) {
	switch capName {
	case "CAPABILITY_SAFE":
		return "", capSafe, nil
	case "CAPABILITY_UNANALYZED":
		return "", capUnanalyzed, nil
	}
	base := capName
	if idx := strings.Index(capName, "/"); idx != -1 {
		base = capName[:idx]
	}
	if !schema.KnownCapabilities[base] {
		return "", capAuthority, fmt.Errorf("capability %q is not a known capability", capName)
	}
	return base, capAuthority, nil
}

// rootAggregate accumulates one root's grouped findings (task req 4).
type rootAggregate struct {
	unanalyzed bool
	caps       map[string]bool
	evidence   map[string][]stdlibauthority.Frame
	safe       bool // a CAPABILITY_SAFE finding was seen
}

func newRootAggregate() *rootAggregate {
	return &rootAggregate{caps: map[string]bool{}, evidence: map[string][]stdlibauthority.Frame{}}
}

// add folds one finding into the root's aggregate. UNANALYZED is terminal and
// wins over every other finding (conservative); SAFE findings mark the
// curated-safe curation without contributing a capability.
func (a *rootAggregate) add(f capslockadapter.GenerationFinding) error {
	base, kind, err := normalizeCapability(f.Capability)
	if err != nil {
		return fmt.Errorf("capslock root %q: %w", f.RootName, err)
	}
	switch kind {
	case capUnanalyzed:
		a.unanalyzed = true
	case capSafe:
		a.safe = true
	case capAuthority:
		a.caps[base] = true
		if prev, ok := a.evidence[base]; !ok || compareFramePaths(f.Path, prev) < 0 {
			a.evidence[base] = f.Path
		}
	}
	return nil
}

// symbolKind is the object kind an inventoried SymbolID resolves to; the
// object-kind completion rules dispatch on it (task reqs 4–7).
type symbolKind int

const (
	kindFunc symbolKind = iota
	kindMethod
	kindVar
	kindConst
	kindType
	kindBuiltin
	kindInit
)

// resolveSymbol maps an inventoried ID to its object kind (and, for vars and
// methods, the declaring object) using the inventory's loaded typed packages.
func resolveSymbol(inv *Inventory, id symbol.SymbolID) (symbolKind, error) {
	text := id.String()
	if strings.HasSuffix(text, ".init") {
		return kindInit, nil
	}
	if strings.HasPrefix(text, "(") {
		return kindMethod, nil
	}
	dot := strings.LastIndexByte(text, '.')
	obj := inv.loaded[text[:dot]].Scope().Lookup(text[dot+1:])
	if obj == nil {
		return kindFunc, fmt.Errorf("resolving inventoried symbol %q: not found in the loaded package", text)
	}
	switch obj.(type) {
	case *types.Func:
		return kindFunc, nil
	case *types.Var:
		return kindVar, nil
	case *types.Const:
		return kindConst, nil
	case *types.TypeName:
		return kindType, nil
	case *types.Builtin:
		return kindBuiltin, nil
	default:
		return kindFunc, fmt.Errorf("resolving inventoried symbol %q: unsupported object kind %T", text, obj)
	}
}

// classifyBuiltin is the unsafe-builtin rule (task req 7): exported
// compiler builtins are *types.Builtin values with no SSA roots, so they are
// hardcoded UNANALYZED as a project override.
func classifyBuiltin() (gen.Classification, string) {
	return gen.Classification_UNANALYZED, ProvenanceProjectOverride
}

// classifyVar applies the var rule (task req 5, DR-05): the union of the
// pointer-dereferenced static type's exported method classifications under
// the map's own classifier, plus the handle minting-authority clause.
// Interface-typed and methodless vars are SAFE. provenance is the inherited
// SAFE provenance when the union is empty ("" otherwise, a capability record).
func classifyVar(inv *Inventory, obj *types.Var, records map[symbol.SymbolID]*symbolResult) (*symbolResult, error) {
	t := obj.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if _, ok := t.Underlying().(*types.Interface); ok {
		return &symbolResult{classification: gen.Classification_SAFE, provenance: provenanceStructural}, nil
	}
	named, _ := types.Unalias(t).(*types.Named)
	if named == nil {
		return &symbolResult{classification: gen.Classification_SAFE, provenance: provenanceStructural}, nil
	}
	result := &symbolResult{}
	for i := 0; i < named.NumMethods(); i++ {
		m := named.Method(i)
		if !m.Exported() {
			continue
		}
		id, err := symbol.FromObject(m)
		if err != nil {
			return nil, fmt.Errorf("classifying var %q: %w", obj.Name(), err)
		}
		method, ok := records[id]
		if !ok {
			return nil, fmt.Errorf("classifying var %q: method %q has no classification; the map would be incomplete", obj.Name(), id)
		}
		result.mergeFrom(method)
	}
	// The handle minting-authority clause: a pre-minted handle variable of a
	// type owning reclassified use-methods inherits the capability the
	// minting site moved to itself (DR-05; os.Stdin is FILES, not merely
	// CHDIR).
	minting := capslockadapter.MintingAuthority()
	pkgPath := ""
	if p := named.Obj().Pkg(); p != nil {
		pkgPath = p.Path()
	}
	if cap, ok := minting[pkgPath+"."+named.Obj().Name()]; ok {
		result.addCapability(cap)
		// The minted capability has no analyzed call path — it is the
		// pre-minted handle's inherited authority. Its evidence is a
		// synthesized single frame naming the handle var, deterministic and
		// auditably non-analyzed (empty file/line).
		if result.evidence == nil {
			result.evidence = map[string][]stdlibauthority.Frame{}
		}
		result.evidence[cap] = append([]stdlibauthority.Frame(nil), result.evidence[cap]...)
		result.evidence[cap] = append(result.evidence[cap], stdlibauthority.Frame{Function: obj.Name()})
	}
	result.finish()
	return result, nil
}

// symbolResult is one terminal classification being assembled.
type symbolResult struct {
	classification gen.Classification
	caps           []string
	evidence       map[string][]stdlibauthority.Frame
	provenance     string
}

func (r *symbolResult) addCapability(cap string) {
	if r.caps == nil {
		r.caps = []string{}
	}
	if !containsString(r.caps, cap) {
		r.caps = append(r.caps, cap)
	}
}

// mergeFrom unions a method's classification into a var's result, tracking
// the inherited SAFE provenance (the strongest trust annotation wins:
// override, then curated, then proved).
func (r *symbolResult) mergeFrom(method *symbolResult) {
	switch {
	case method.classification == gen.Classification_UNANALYZED:
		r.classification = gen.Classification_UNANALYZED
	case method.classification == gen.Classification_CAPABILITIES:
		for _, c := range method.caps {
			r.addCapability(c)
		}
		if method.evidence != nil {
			if r.evidence == nil {
				r.evidence = map[string][]stdlibauthority.Frame{}
			}
			for c, frames := range method.evidence {
				r.evidence[c] = frames
			}
		}
	case method.classification == gen.Classification_SAFE:
		if method.provenance == ProvenanceProjectOverride {
			r.provenance = ProvenanceProjectOverride
		} else if r.provenance == "" && method.provenance == ProvenanceCapslockCurated {
			r.provenance = ProvenanceCapslockCurated
		} else if r.provenance == "" && method.provenance == ProvenanceProvedPure {
			r.provenance = ProvenanceProvedPure
		}
	}
}

// finish settles the terminal classification once all merging is done.
func (r *symbolResult) finish() {
	if r.classification == gen.Classification_UNANALYZED {
		r.caps, r.evidence = nil, nil
		r.provenance = ""
		return
	}
	if len(r.caps) > 0 {
		sort.Strings(r.caps)
		r.classification = gen.Classification_CAPABILITIES
		r.provenance = ""
		return
	}
	r.classification = gen.Classification_SAFE
}

// hasInit reports whether the inventory holds an aggregate init for pkgPath.
func (inv *Inventory) hasInit(pkgPath string) bool {
	for _, id := range inv.Inits {
		if strings.TrimSuffix(id.String(), ".init") == pkgPath {
			return true
		}
	}
	return false
}

// containsString is a small linear membership helper.
func containsString(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

// compareFramePaths orders two evidence paths lexicographically frame by
// frame, then by length, so evidence selection is deterministic regardless of
// analyzer iteration order (task req 10).
func compareFramePaths(a, b []stdlibauthority.Frame) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareFrames(a[i], b[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

func compareFrames(a, b stdlibauthority.Frame) int {
	if c := strings.Compare(a.Function, b.Function); c != 0 {
		return c
	}
	if c := strings.Compare(a.File, b.File); c != 0 {
		return c
	}
	switch {
	case a.Line < b.Line:
		return -1
	case a.Line > b.Line:
		return 1
	}
	return 0
}

// BuildAuthorityMap converts one batched Capslock response into the
// classified map message (task reqs 3–8): findings are grouped by their
// `Path[0]` root, normalized through the structured inventory, and every
// inventoried symbol and init is completed with a terminal classification.
// Reconciliation is exact: every inventory symbol carries exactly one record,
// every analyzer root is accounted for (mapped or dropped for a sanctioned
// cause), and every capability carries deterministic evidence.
func BuildAuthorityMap(inv *Inventory, findings []capslockadapter.GenerationFinding) (*gen.StdlibMap, error) {
	if inv == nil {
		return nil, fmt.Errorf("building the authority map: the inventory is required")
	}

	// 1. Group findings by root, normalizing through the structured API and
	// tracking every root's disposition (mapped or dropped-with-reason).
	type rootKey struct{ pkg, name string }
	aggregates := map[symbol.SymbolID]*rootAggregate{}
	accounted := map[rootKey]bool{}
	dropped := map[rootKey]dropReason{}
	for _, f := range findings {
		key := rootKey{pkg: f.RootPackage, name: f.RootName}
		id, drop, err := normalizeRoot(inv, f)
		if err != nil {
			return nil, fmt.Errorf("building the authority map: %w", err)
		}
		if drop != "" {
			if prev, ok := dropped[key]; ok && prev != drop {
				return nil, fmt.Errorf("building the authority map: root %q was dropped for conflicting reasons %q and %q", f.RootName, prev, drop)
			}
			dropped[key] = drop
			continue
		}
		accounted[key] = true
		agg, ok := aggregates[id]
		if !ok {
			agg = newRootAggregate()
			aggregates[id] = agg
		}
		if err := agg.add(f); err != nil {
			return nil, fmt.Errorf("building the authority map: %w", err)
		}
	}

	// 2. Classify every inventoried symbol and init (exact reconciliation:
	// records == inventory, task req 9).
	reclassified := map[symbol.SymbolID]bool{}
	for _, m := range capslockadapter.ReclassifiedHandleUseMethods() {
		id, err := symbol.ParseCapslock(m)
		if err != nil {
			return nil, fmt.Errorf("building the authority map: reclassified method %q: %w", m, err)
		}
		reclassified[id] = true
	}

	records := map[symbol.SymbolID]*symbolResult{}
	// Funcs and methods first: the var rule unions their classifications.
	for _, id := range inv.Symbols {
		kind, err := resolveSymbol(inv, id)
		if err != nil {
			return nil, fmt.Errorf("building the authority map: %w", err)
		}
		switch kind {
		case kindFunc, kindMethod:
			result := &symbolResult{}
			if agg := aggregates[id]; agg != nil {
				if agg.unanalyzed {
					result.classification = gen.Classification_UNANALYZED
				} else {
					for cap := range agg.caps {
						result.addCapability(cap)
						if result.evidence == nil {
							result.evidence = map[string][]stdlibauthority.Frame{}
						}
						result.evidence[cap] = agg.evidence[cap]
					}
					result.finish()
					if result.classification == gen.Classification_SAFE {
						switch {
						case reclassified[id]:
							result.provenance = ProvenanceProjectOverride
						case agg.safe:
							result.provenance = ProvenanceCapslockCurated
						default:
							result.provenance = ProvenanceProvedPure
						}
					}
				}
			} else {
				// Zero-finding inventoried root: explicitly SAFE (proved
				// pure under a classifier that preserves UNANALYZED, I4).
				result.classification = gen.Classification_SAFE
				result.provenance = ProvenanceProvedPure
			}
			records[id] = result
		case kindVar:
			continue // second pass, after methods exist
		case kindConst, kindType:
			// Consts and plain types are SAFE by construction (task req 5).
			records[id] = &symbolResult{classification: gen.Classification_SAFE, provenance: provenanceStructural}
		case kindBuiltin:
			records[id] = &symbolResult{classification: gen.Classification_UNANALYZED, provenance: ProvenanceProjectOverride}
		default:
			return nil, fmt.Errorf("building the authority map: inventoried symbol %q has unexpected kind %d", id, kind)
		}
	}
	// Vars second.
	for _, id := range inv.Symbols {
		kind, err := resolveSymbol(inv, id)
		if err != nil {
			return nil, fmt.Errorf("building the authority map: %w", err)
		}
		if kind != kindVar {
			continue
		}
		dot := strings.LastIndexByte(id.String(), '.')
		obj, ok := inv.loaded[id.String()[:dot]].Scope().Lookup(id.String()[dot+1:]).(*types.Var)
		if !ok || obj == nil {
			return nil, fmt.Errorf("building the authority map: inventoried var %q did not resolve to a types.Var", id)
		}
		result, err := classifyVar(inv, obj, records)
		if err != nil {
			return nil, fmt.Errorf("building the authority map: %w", err)
		}
		records[id] = result
	}
	// Inits: one record per importable package, keyed on the aggregate (task
	// req 6).
	for _, id := range inv.Inits {
		agg := aggregates[id]
		result := &symbolResult{}
		if agg != nil {
			if agg.unanalyzed {
				result.classification = gen.Classification_UNANALYZED
			} else {
				for cap := range agg.caps {
					result.addCapability(cap)
					if result.evidence == nil {
						result.evidence = map[string][]stdlibauthority.Frame{}
					}
					result.evidence[cap] = agg.evidence[cap]
				}
				result.finish()
				if result.classification == gen.Classification_SAFE {
					result.provenance = ProvenanceProvedPure
				}
			}
		} else {
			result.classification = gen.Classification_SAFE
			result.provenance = ProvenanceProvedPure
		}
		records[id] = result
	}

	// 3. Every analyzer root must be accounted for: mapped or dropped for a
	// sanctioned cause (task req 9).
	seenRoots := map[rootKey]bool{}
	for _, f := range findings {
		key := rootKey{pkg: f.RootPackage, name: f.RootName}
		if seenRoots[key] {
			continue
		}
		seenRoots[key] = true
		id, drop, err := normalizeRoot(inv, f)
		if err != nil {
			return nil, fmt.Errorf("building the authority map: unaccounted analyzer root: %w", err)
		}
		if drop != "" {
			continue
		}
		if _, ok := records[id]; !ok {
			return nil, fmt.Errorf("building the authority map: analyzer root %q resolved to %q, which is not an inventoried symbol or init", f.RootName, id)
		}
	}

	// 4. Assemble the message. Collections are sorted by the canonical I/O
	// boundary as well, but they are built sorted here so the in-memory
	// message is already canonical (task req 10).
	m := &gen.StdlibMap{}
	for _, p := range inv.Packages {
		m.Packages = append(m.Packages, &gen.PackageInventory{Path: p.Path, Importable: p.Importable})
	}
	var evidence []*gen.Evidence
	for _, id := range inv.Symbols {
		r := records[id]
		m.Symbols = append(m.Symbols, &gen.SymbolRecord{
			Package:        recordPackage(id.String()),
			Id:             id.String(),
			Classification: r.classification,
			Capabilities:   r.caps,
			Provenance:     r.provenance,
		})
		for _, c := range r.caps {
			frames := r.evidence[c]
			if len(frames) == 0 {
				return nil, fmt.Errorf("building the authority map: capability %q of %q has no evidence", c, id)
			}
			ev := &gen.Evidence{SymbolId: id.String(), Capability: c}
			for _, f := range frames {
				ev.Frames = append(ev.Frames, &gen.Frame{Function: f.Function, File: f.File, Line: int32(f.Line)})
			}
			evidence = append(evidence, ev)
		}
	}
	for _, id := range inv.Inits {
		r := records[id]
		m.Inits = append(m.Inits, &gen.InitRecord{
			Package:        strings.TrimSuffix(id.String(), ".init"),
			Classification: r.classification,
			Capabilities:   r.caps,
		})
	}
	m.Evidence = evidence
	return m, nil
}

// recordPackage extracts the package path from a top-level or method ID
// (grammar: "(pkg.T).M" or "pkg.Name").
func recordPackage(id string) string {
	if strings.HasPrefix(id, "(") {
		recv := id[1:strings.IndexByte(id, ')')]
		return recv[:strings.LastIndexByte(recv, '.')]
	}
	return id[:strings.LastIndexByte(id, '.')]
}
