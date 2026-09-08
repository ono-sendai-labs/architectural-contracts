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
	// Findings runs Capslock once over the complete importable batch. When
	// nil, the native Capslock runner is used, bound to the same target
	// environment as the loader and oracle (round-1 finding: the analysis
	// must describe the target, not the host).
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
	findingsSource := in.Findings
	if findingsSource == nil {
		env := (&NativeLoader{Env: TargetEnv(in.Target)}).Environment()
		findingsSource = FindingSourceFunc(func(paths []string) ([]capslockadapter.GenerationFinding, error) {
			return capslockadapter.GenerationFindingsForEnv(env, paths)
		})
	}
	findings, err := findingsSource.Findings(importable)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	// Provenance for SAFE roots comes from the generation classifier itself:
	// curated-safe and minting-reclassified roots produce NO findings, so
	// every inventoried function and method (plus the observed roots, and the
	// aggregate inits defensively) is queried against the classifier once.
	// The classifier's spellings include the pointer and value receiver
	// forms; CuratedSafeKeys marks the ones the classifier curates SAFE
	// (round-1 finding: querying only observed findings lost curated roots
	// such as os.Exit entirely).
	names := rootDisplayNames(findings)
	for _, id := range inv.Symbols {
		kind, err := resolveSymbol(inv, id)
		if err != nil {
			return nil, fmt.Errorf("generating the stdlib map: %w", err)
		}
		if kind == kindFunc || kind == kindMethod {
			names = append(names, classifierSpellings(id)...)
		}
	}
	for _, id := range inv.Inits {
		names = append(names, id.String())
	}
	curated, err := capslockadapter.CuratedSafeKeys(names)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	m, err := BuildAuthorityMap(inv, findings, curated)
	if err != nil {
		return nil, err
	}
	m.FormatVersion = artifactio.MapFormatVersion
	key, err := sdkKeyProto(in)
	if err != nil {
		return nil, fmt.Errorf("generating the stdlib map: %w", err)
	}
	m.Key = key
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

// classifierSpellings returns the Capslock display spellings of an inventoried
// function or method ID — the keys the generation classifier's FunctionCategory
// consults. Methods are spelled with both pointer and value receivers: the
// classifier's reclassification keys use the pointer form, but a curated entry
// may be spelled either way.
func classifierSpellings(id symbol.SymbolID) []string {
	text := id.String()
	if !strings.HasPrefix(text, "(") {
		return []string{text}
	}
	// "(pkg.T).M" -> "(*pkg.T).M" (pointer form, the classifier's
	// reclassification key spelling) and the canonical value form itself
	// "(pkg.T).M" (round-2 finding: the previous construction dropped the
	// receiver type name, so value-receiver curations such as
	// "(net.Flags).String" were never found).
	end := strings.IndexByte(text, ')')
	return []string{"(*" + text[1:end] + ")" + text[end+1:], text}
}

// rootDisplayNames returns each root's distinct display spelling, the keys
// the generation classifier's FunctionCategory consults.
func rootDisplayNames(findings []capslockadapter.GenerationFinding) []string {
	seen := map[string]bool{}
	var names []string
	for _, f := range findings {
		if !seen[f.RootName] {
			seen[f.RootName] = true
			names = append(names, f.RootName)
		}
	}
	return names
}

// GenerationClassifierRules renders the generation classifier's hashable
// rule descriptor from the semantics generation actually executes (task reqs
// 1–3, design I2): Capslock's exact builtin classifier content pin, the exact
// project overlay text NewGenerationClassifier loads, and the canonical
// project completion rules (the unsafe-builtin hardcoding and the full
// variable/minting policy — the same canonical instance BuildAuthorityMap
// executes). There is no independently maintained description that can drift
// from the classifier execution path (review: assembly-to-execution drift).
func GenerationClassifierRules() ([]ClassifierRule, error) {
	pin, err := capslockadapter.BuiltinClassifierPin()
	if err != nil {
		return nil, fmt.Errorf("fingerprinting the Capslock builtins: %w", err)
	}
	overlay, err := capslockadapter.GenerationClassifierText()
	if err != nil {
		return nil, fmt.Errorf("reading the generation classifier overlay: %w", err)
	}
	return classifierFingerprintRules(pin, overlay, projectCompletionRules())
}

// renderUnsafeRule renders the executed unsafe-completion rule's canonical
// machine-readable effect: the classifications and provenance the execution
// path applies, key=value form, deterministic.
func renderUnsafeRule(u UnsafeRuleSemantics) string {
	return fmt.Sprintf("builtin-classification=%s type-classification=%s provenance=%s",
		u.BuiltinClassification, u.TypeClassification, u.Provenance)
}

// renderVarRule renders the executed variable-completion rule's canonical
// machine-readable effect: the full policy plus the minting table, sorted for
// order independence.
func renderVarRule(v VarRuleSemantics) string {
	var minted []string
	for recv, cap := range v.Minting {
		minted = append(minted, recv+" → "+cap)
	}
	sort.Strings(minted)
	return fmt.Sprintf("pointer-dereference=%t method-set=%s exported-only=%t interface-safe=%t named-safe=%t minting=%s",
		v.PointerDereference, v.MethodSet, v.ExportedOnly, v.InterfaceSafe, v.NamedSafe, strings.Join(minted, ","))
}

// classifierFingerprintRules assembles the canonical fingerprint rule set
// from its semantic inputs: the Capslock builtin content pin, the exact
// project overlay text, and the canonical project completion rules. Rule
// effects carry semantic material, not prose summaries;
// CanonicalClassifierText renders them order-independently.
func classifierFingerprintRules(builtinPin, overlayText string, rules completionRules) ([]ClassifierRule, error) {
	return []ClassifierRule{
		{
			// The effect IS the content pin: a cryptographic digest of
			// Capslock's embedded builtin classifier, so any builtin
			// classification change necessarily changes the classifier_hash
			// (review F1, task req 2).
			Name:   "capslock.builtins.pin",
			Effect: builtinPin,
		},
		{
			// The effect IS the exact overlay text the generation classifier
			// loads (capslockadapter.GenerationClassifierText, the minting
			// reclassification lines).
			Name:   "minting.reclassification",
			Effect: overlayText,
		},
		{
			// The effect IS the executed var-rule policy (renderVarRule): the
			// pointer/deref, method-set, exported/interface/named decisions
			// and the minting table classifyVar actually consumes.
			Name:   "var.completion",
			Effect: renderVarRule(rules.Var),
		},
		{
			// The effect IS the executed unsafe rule (renderUnsafeRule): the
			// classifications and provenance classifyBuiltin and the
			// unsafe.* type branch actually apply.
			Name:   "unsafe.builtins",
			Effect: renderUnsafeRule(rules.Unsafe),
		},
	}, nil
}

// sdkKeyProto derives the target SDK key from the generation descriptor and
// converts it to its persisted proto form (task req 10). Fingerprint assembly
// errors fail generation closed — never a silently unhashed key.
func sdkKeyProto(in GenerationInput) (*gen.SDKKey, error) {
	rules, err := GenerationClassifierRules()
	if err != nil {
		return nil, err
	}
	text, err := CanonicalClassifierText(rules)
	if err != nil {
		// The rule set is a fixed, duplicate-free set; a canonicalization
		// failure is a programming error and must fail generation, not
		// produce an unhashed key.
		return nil, fmt.Errorf("canonicalizing the classifier rules: %w", err)
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
	}, nil
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
	pkg := f.RootPackage
	// Drops only apply inside a known package: Capslock roots come from the
	// queried batch, so an unknown package is an accounting failure, never a
	// drop (task req 9). The guard precedes every drop rule.
	if pkg != "" && !inv.KnownPackage(pkg) {
		return "", "", fmt.Errorf("capslock root %q names an unknown package %q", name, pkg)
	}
	if strings.Contains(name, "$") {
		return "", dropClosure, nil
	}
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
		if pkg == "" {
			return "", "", fmt.Errorf("capslock root %q names no package", name)
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
	// A bracketed spelling is a generic instantiation: the Step 3 structured
	// parser (symbol.ParseCapslockFunction) validates the type-argument
	// grammar — including receiver instantiations — and confirms the
	// uninstantiated declaration through the inventory. A validated
	// instantiation is discarded, not merged into the origin's findings (task
	// req 3). Anything the parser rejects — misplaced brackets, empty or
	// non-type arguments, an unknown exported origin — fails closed rather
	// than being silently dropped (round-4 finding). Only an unexported
	// origin is a sanctioned drop: a helper folded into its exported
	// callers.
	if strings.ContainsAny(name, "[]") {
		if _, err := symbol.ParseCapslockFunction(symbol.CapslockFunction{Name: name, Package: pkg}, inv); err == nil {
			return "", dropInstantiation, nil
		} else if !instantiationOriginExported(name) {
			return "", dropUnexported, nil
		} else {
			return "", "", fmt.Errorf("capslock root %q: %w", name, err)
		}
	}
	id, err := symbol.ParseCapslockFunction(symbol.CapslockFunction{Name: name, Package: pkg}, inv)
	if err == nil {
		return id, "", nil
	}
	// Sanctioned non-roots. A receiver type or trailing identifier that is
	// unexported is a helper folded into its exported callers. Everything
	// else is an exported root the inventory cannot account for, which fails
	// generation rather than guessing (task req 3).
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
		// SSA promotes embedded methods under the embedding type's spelling
		// (a synthetic wrapper "(*pkg.T).M" for the declaring "(pkg.Base).M").
		// Resolve through the full method set: when the declaring object is
		// itself inventoried, the wrapper folds into it.
		if id, ok := promotedMethodOwner(inv, pkg, typeName, method); ok {
			return id, "", nil
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

// instantiationOriginExported reports whether the generic origin a bracketed
// Capslock spelling instantiates names an exported declaration. The scan is
// purely textual and only feeds the unexported-helper drop rule after
// symbol.ParseCapslockFunction has already rejected the spelling; the
// spelling's validity is judged solely by the parser.
func instantiationOriginExported(name string) bool {
	if strings.HasPrefix(name, "(") {
		end := strings.Index(name, ").")
		if end < 0 {
			return false
		}
		recv := strings.TrimPrefix(name[1:end], "*")
		if open := strings.IndexByte(recv, '['); open >= 0 {
			recv = recv[:open]
		}
		dot := strings.LastIndexByte(recv, '.')
		if dot < 0 {
			return false
		}
		return tokenIsExported(recv[dot+1:]) && tokenIsExported(name[end+2:])
	}
	base := name
	if open := strings.IndexByte(base, '['); open >= 0 {
		base = base[:open]
	}
	dot := strings.LastIndexByte(base, '.')
	if dot < 0 {
		return false
	}
	return tokenIsExported(base[dot+1:])
}

// tokenIsExported reports whether a Go identifier is exported.
func tokenIsExported(name string) bool {
	return name != "" && name[0] != '_' && name[0] < 0x80 && name[0] >= 'A' && name[0] <= 'Z'
}

// promotedMethodOwner resolves a method spelled against pkg.TypeName through
// the type's FULL method set (including promoted methods) to the declaring
// *types.Func's inventoried SymbolID. ok=false unless the declaring object is
// itself inventoried.
func promotedMethodOwner(inv *Inventory, pkg, typeName, method string) (symbol.SymbolID, bool) {
	p := inv.loaded[pkg]
	if p == nil {
		return "", false
	}
	obj := p.Scope().Lookup(typeName)
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return "", false
	}
	found, _, _ := types.LookupFieldOrMethod(tn.Type(), true, p, method)
	fn, ok := found.(*types.Func)
	if !ok || fn == nil {
		return "", false
	}
	id, err := symbol.FromObject(fn)
	if err != nil || !inv.hasSymbol(id) {
		return "", false
	}
	return id, true
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
		// Defensive spelling; curated-safe roots produce no findings at all
		// (see the SAFE handling below and capslockadapter.CuratedSafeKeys).
		return "", capSafe, nil
	case "UNANALYZED", "CAPABILITY_UNANALYZED":
		// Capslock's unanalyzable control name (spike finding 5): terminal,
		// never a taxonomy capability despite the taxonomy listing it.
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
// hardcoded UNANALYZED as a project override. The classification and
// provenance come from the canonical completion rules
// (projectCompletionRules), the same source the classifier fingerprint
// hashes (review: assembly-to-execution drift).
func classifyBuiltin(rules UnsafeRuleSemantics) (gen.Classification, string, error) {
	classification, err := classificationFor(rules.BuiltinClassification)
	if err != nil {
		return 0, "", fmt.Errorf("the unsafe-builtin completion rule: %w", err)
	}
	return classification, rules.Provenance, nil
}

// classificationFor parses a canonical classification spelling into its
// persisted enum value. Only the design's terminal classifications are
// representable; anything else fails closed.
func classificationFor(name string) (gen.Classification, error) {
	switch name {
	case "SAFE":
		return gen.Classification_SAFE, nil
	case "CAPABILITIES":
		return gen.Classification_CAPABILITIES, nil
	case "UNANALYZED":
		return gen.Classification_UNANALYZED, nil
	}
	return 0, fmt.Errorf("classification %q is not a known classification spelling", name)
}

// UnsafeRuleSemantics is the executed unsafe-completion rule's
// machine-readable semantics (review: the fingerprint must carry the executed
// rule, not prose). Both the execution paths (classifyBuiltin and the
// unsafe.* named-type branch of BuildAuthorityMap) and the canonical
// classifier fingerprint derive from one canonical instance
// (projectCompletionRules).
type UnsafeRuleSemantics struct {
	// BuiltinClassification is the terminal classification applied to
	// exported unsafe.* compiler builtins (*types.Builtin with no SSA roots).
	BuiltinClassification string
	// TypeClassification is the terminal classification applied to unsafe.*
	// named types (compiler-magic types in a package of compiler builtins,
	// e.g. unsafe.Pointer).
	TypeClassification string
	// Provenance is the SAFE-provenance vocabulary entry the rule records
	// (ProvenanceProjectOverride); the rule is this project's overriding
	// trust decision, not Capslock's curation or proved purity.
	Provenance string
}

// VarRuleSemantics is the executed variable-completion rule's
// machine-readable policy (DR-05, task req 5; review: the fingerprint must
// carry the executed policy). classifyVar derives from the same canonical
// instance the classifier fingerprint hashes.
type VarRuleSemantics struct {
	// PointerDereference dereferences a variable's static pointer type before
	// classification (a *File variable is classified via File's method set).
	PointerDereference bool
	// MethodSet is the receiver type whose method set the rule unions:
	// "pointer" (the full static pointer method set, promoted methods and
	// alias-exposed receivers included) or "value".
	MethodSet string
	// ExportedOnly restricts the union to exported methods.
	ExportedOnly bool
	// InterfaceSafe classifies interface-typed variables SAFE.
	InterfaceSafe bool
	// NamedSafe classifies methodless non-interface named variables SAFE.
	NamedSafe bool
	// Minting maps a receiver type ("pkg.T") to the ambient capability its
	// minting site moves to itself; a variable of that handle type inherits
	// the minted capability (capslockadapter.MintingAuthority is the source).
	Minting map[string]string
}

// completionRules is the canonical, machine-readable source of the project
// completion-rule semantics executed by BuildAuthorityMap and hashed into the
// SDK key's classifier fingerprint. There is exactly one production instance
// (projectCompletionRules); no independently maintained description may drift
// from it (review: project completion rules must not remain prose-only).
type completionRules struct {
	Unsafe UnsafeRuleSemantics
	Var    VarRuleSemantics
}

// projectCompletionRules returns the canonical instance of the executed
// project completion rules: the unsafe-builtin hardcoding and the full
// variable/minting policy (task reqs 1, 5, 7).
func projectCompletionRules() completionRules {
	return completionRules{
		Unsafe: UnsafeRuleSemantics{
			BuiltinClassification: "UNANALYZED",
			TypeClassification:    "UNANALYZED",
			Provenance:            ProvenanceProjectOverride,
		},
		Var: VarRuleSemantics{
			PointerDereference: true,
			MethodSet:          "pointer",
			ExportedOnly:       true,
			InterfaceSafe:      true,
			NamedSafe:          true,
			Minting:            capslockadapter.MintingAuthority(),
		},
	}
}

// classifyVar applies the var rule (task req 5, DR-05): the union of the
// pointer-dereferenced static type's exported method classifications under
// the map's own classifier, plus the handle minting-authority clause.
// Interface-typed and methodless vars are SAFE. provenance is the inherited
// SAFE provenance when the union is empty ("" otherwise, a capability record).
// Every branch derives from the canonical completion rules, which the
// classifier fingerprint also hashes (review: assembly-to-execution drift).
func classifyVar(inv *Inventory, varID symbol.SymbolID, obj *types.Var, records map[symbol.SymbolID]*symbolResult, rules VarRuleSemantics) (*symbolResult, error) {
	t := obj.Type()
	if rules.PointerDereference {
		if ptr, ok := t.(*types.Pointer); ok {
			t = ptr.Elem()
		}
	}
	if rules.InterfaceSafe {
		if _, ok := t.Underlying().(*types.Interface); ok {
			return &symbolResult{classification: gen.Classification_SAFE, provenance: provenanceStructural}, nil
		}
	}
	named, _ := types.Unalias(t).(*types.Named)
	if named == nil {
		if rules.NamedSafe {
			return &symbolResult{classification: gen.Classification_SAFE, provenance: provenanceStructural}, nil
		}
		return &symbolResult{}, nil
	}
	result := &symbolResult{}
	// The full static method set of the rule's receiver type: promoted
	// methods from embedded types and alias-exposed receivers carry authority
	// too, so the walk uses the complete method set rather than only the
	// explicitly declared methods (round-1 finding).
	var recv types.Type = named
	if rules.MethodSet == "pointer" {
		recv = types.NewPointer(named)
	}
	mset := types.NewMethodSet(recv)
	for i := 0; i < mset.Len(); i++ {
		m, ok := mset.At(i).Obj().(*types.Func)
		if rules.ExportedOnly && (!ok || !m.Exported()) {
			continue
		}
		if m == nil {
			continue
		}
		id, err := symbol.FromObject(m)
		if err != nil {
			return nil, fmt.Errorf("classifying var %q: %w", obj.Name(), err)
		}
		// Only inventory-visible methods carry a classification; an exported
		// method declared on an unexported type that no exported alias
		// exposes is structurally outside the inventory and is skipped.
		// (Exact reconciliation guarantees every inventoried method has a
		// record, so a missing record means the method is not inventoried.)
		method, ok := records[id]
		if !ok {
			continue
		}
		result.mergeFrom(method)
	}
	// The handle minting-authority clause: a pre-minted handle variable of a
	// type owning reclassified use-methods inherits the capability the
	// minting site moved to itself (DR-05; os.Stdin is FILES, not merely
	// CHDIR). The minting table comes from the canonical completion rules —
	// the same table the fingerprint hashes.
	pkgPath := ""
	if p := named.Obj().Pkg(); p != nil {
		pkgPath = p.Path()
	}
	if cap, ok := rules.Minting[pkgPath+"."+named.Obj().Name()]; ok {
		result.addCapability(cap)
		// The minted capability has no analyzed call path — it is the
		// pre-minted handle's inherited authority. Its evidence is a
		// synthesized single frame naming the handle var, deterministic and
		// auditably non-analyzed (empty file/line).
		if result.evidence == nil {
			result.evidence = map[string][]stdlibauthority.Frame{}
		}
		result.evidence[cap] = append([]stdlibauthority.Frame(nil), result.evidence[cap]...)
		result.evidence[cap] = append(result.evidence[cap], stdlibauthority.Frame{Function: varID.String()})
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

// provenanceRank encodes the SAFE provenance precedence explicitly (project
// override, then curated, then proved) so the inherited annotation is
// independent of the method-set iteration order (round-1 finding).
func provenanceRank(p string) int {
	switch p {
	case ProvenanceProjectOverride:
		return 3
	case ProvenanceCapslockCurated:
		return 2
	case ProvenanceProvedPure:
		return 1
	}
	return 0
}

// mergeFrom unions a method's classification into a var's result, tracking
// the inherited SAFE provenance: the highest-ranked incoming annotation wins,
// regardless of visit order.
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
		if provenanceRank(method.provenance) > provenanceRank(r.provenance) {
			r.provenance = method.provenance
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

// isCuratedSafe reports whether the generation classifier curates the
// inventoried function or method CAPABILITY_SAFE, consulting both receiver
// spellings for methods and the observed root name as a fallback.
func isCuratedSafe(curatedSafe map[string]bool, id symbol.SymbolID) bool {
	if len(curatedSafe) == 0 {
		return false
	}
	for _, name := range classifierSpellings(id) {
		if curatedSafe[name] {
			return true
		}
	}
	return curatedSafe[id.String()]
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
func BuildAuthorityMap(inv *Inventory, findings []capslockadapter.GenerationFinding, curatedSafe map[string]bool) (*gen.StdlibMap, error) {
	if inv == nil {
		return nil, fmt.Errorf("building the authority map: the inventory is required")
	}

	// 1. Group findings by root, normalizing through the structured API and
	// tracking every root's disposition (mapped or dropped-with-reason).
	type rootKey struct{ pkg, name string }
	aggregates := map[symbol.SymbolID]*rootAggregate{}
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
	// records == inventory, task req 9). The completion rules come from the
	// canonical projectCompletionRules — the same source the classifier
	// fingerprint hashes — so a rule-semantics change necessarily moves the
	// SDK key (review: assembly-to-execution drift).
	rules := projectCompletionRules()
	unsafeBuiltinClass, _, err := classifyBuiltin(rules.Unsafe)
	if err != nil {
		return nil, fmt.Errorf("building the authority map: %w", err)
	}
	unsafeTypeClass, err := classificationFor(rules.Unsafe.TypeClassification)
	if err != nil {
		return nil, fmt.Errorf("building the authority map: the unsafe-type completion rule: %w", err)
	}
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
				// Zero-finding inventoried root (curated-safe and
				// minting-reclassified roots produce no findings either):
				// explicitly SAFE. Provenance: this project's overriding
				// minting rule, Capslock's curation, or analysis-proved
				// purity — visibly distinct trust decisions (task req 8).
				result.classification = gen.Classification_SAFE
				switch {
				case reclassified[id]:
					result.provenance = ProvenanceProjectOverride
				case isCuratedSafe(curatedSafe, id):
					result.provenance = ProvenanceCapslockCurated
				default:
					result.provenance = ProvenanceProvedPure
				}
			}
			records[id] = result
		case kindVar:
			continue // second pass, after methods exist
		case kindConst, kindType:
			// Consts and plain types are SAFE by construction (task req 5) —
			// except unsafe.Pointer, a compiler-magic type in a package of
			// compiler builtins with no SSA roots (task req 7). The
			// classification comes from the canonical unsafe rule.
			if recordPackage(id.String()) == "unsafe" {
				records[id] = &symbolResult{classification: unsafeTypeClass, provenance: rules.Unsafe.Provenance}
				continue
			}
			records[id] = &symbolResult{classification: gen.Classification_SAFE, provenance: provenanceStructural}
		case kindBuiltin:
			records[id] = &symbolResult{classification: unsafeBuiltinClass, provenance: rules.Unsafe.Provenance}
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
		result, err := classifyVar(inv, id, obj, records, rules.Var)
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
					// Capslock curates some aggregate inits SAFE (e.g.
					// "func os.init CAPABILITY_SAFE" in interesting.cm); the
					// classifier decides between its curation and
					// analysis-proved purity, and the InitRecord's provenance
					// field keeps the distinction visible in the artifact
					// (round-2 finding).
					if isCuratedSafe(curatedSafe, id) {
						result.provenance = ProvenanceCapslockCurated
					} else {
						result.provenance = ProvenanceProvedPure
					}
				}
			}
		} else {
			result.classification = gen.Classification_SAFE
			if isCuratedSafe(curatedSafe, id) {
				result.provenance = ProvenanceCapslockCurated
			} else {
				result.provenance = ProvenanceProvedPure
			}
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
			Provenance:     r.provenance,
		})
		// Capability-bearing init records carry the same deterministic
		// evidence as symbols (task req 8; round-1 finding).
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
