// Deterministic authority aggregation and policy application for the DR-17
// finding model: one authority finding per (capability, class) per component,
// carrying every sorted source site, the classification class, the stdlib
// map's SDK key and the first sorted site's evidence.
//
// Component Contract (FR10):
//   - What it does: Aggregates policy-ready AuthorityObservations and
//     analysis-defeating BypassObservations into AuthorityFindings (sites
//     sorted by file, line, referenced SymbolID; exact duplicates removed;
//     evidence selected for the first sorted site), then applies a capability
//     policy: strict/default is a violation for every class, an explicit warn
//     policy downgrades to ANALYSIS_LIMITATION (AnalysisDefeating) or
//     ALLOWED_WITH_WARNING (TrueAuthority), and an allowed capability
//     disappears. Never downgrades automatically.
//   - What it requires: Observations carrying a single common SDKKey (the
//     target configuration the map decided under), bypass observations in
//     canonical form, and the expected SDKKey; a policy.
//   - What it provides: AggregateAuthority, AuthorityFinding, AuthoritySite,
//     ApplyAuthorityPolicy. Pure and deterministic: identical inputs produce
//     identical results or identical errors; emitted collections are sorted.
//   - Ambient Authority: This component is guaranteed-pure and holds no
//     ambient authority (no filesystem I/O, network, process execution or
//     reflection).
package checker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// AuthoritySite is one sorted member source site of an aggregated authority
// finding: the component-relative site and the referenced SymbolID (empty for
// analysis-defeating constructs, which name no declaring object).
type AuthoritySite struct {
	Site     facts.SourceSite
	Referent facts.SymbolID
}

// AuthorityFinding is one aggregated authority finding (DR-17): exactly one
// per (capability, class) per component. Sites carries every member source
// site sorted and duplicate-free; Evidence carries the map's canned path for
// the first sorted site's (symbol, capability) and is nil for
// AnalysisDefeating findings.
type AuthorityFinding struct {
	Class      capanalyzer.Class
	Capability string
	// SDKKey is the map's own target-configuration key (N3), carried on the
	// finding so reports pin the configuration that decided.
	SDKKey   stdlibauthority.SDKKey
	Sites    []AuthoritySite
	Evidence []stdlibauthority.Frame
}

// AggregateAuthority merges stdlib classification observations and member
// bypass observations into one AuthorityFinding per (capability, class). All
// observations must carry the expected SDKKey; a conflicting key is a tool
// error, since one report must pin exactly one target configuration.
func AggregateAuthority(
	obs []AuthorityObservation,
	bypasses []facts.BypassObservation,
	expected stdlibauthority.SDKKey,
) ([]AuthorityFinding, error) {
	type key struct {
		class      capanalyzer.Class
		capability string
	}
	findings := make(map[key]*AuthorityFinding)
	evidence := make(map[string][]stdlibauthority.Frame)

	addSite := func(k key, s AuthoritySite) {
		f, ok := findings[k]
		if !ok {
			f = &AuthorityFinding{Class: k.class, Capability: k.capability, SDKKey: expected}
			findings[k] = f
		}
		f.Sites = append(f.Sites, s)
	}

	for _, o := range obs {
		if fields := stdlibauthority.EqualKeys(o.SDKKey, expected); len(fields) > 0 {
			return nil, fmt.Errorf("authority observation at %s carries a conflicting SDK key: mismatched %v", o.Site.File, fields)
		}
		k := key{class: o.Class, capability: o.Capability}
		addSite(k, AuthoritySite{Site: o.Site, Referent: o.Referent})
		if o.Class == capanalyzer.TrueAuthority && len(o.Evidence) > 0 {
			evidence[o.Referent.Format()+"\x00"+o.Capability] = o.Evidence
		}
	}
	for _, b := range bypasses {
		if err := b.Validate(); err != nil {
			return nil, err
		}
		addSite(key{class: capanalyzer.AnalysisDefeating}, AuthoritySite{Site: b.Site})
	}

	out := make([]AuthorityFinding, 0, len(findings))
	for k, f := range findings {
		f.Sites = sortAuthoritySites(f.Sites)
		if k.class == capanalyzer.TrueAuthority && len(f.Sites) > 0 {
			first := f.Sites[0]
			f.Evidence = evidence[first.Referent.Format()+"\x00"+k.capability]
		}
		out = append(out, *f)
	}
	slices.SortFunc(out, func(a, b AuthorityFinding) int {
		if a.Class != b.Class {
			return strings.Compare(string(a.Class), string(b.Class))
		}
		return strings.Compare(a.Capability, b.Capability)
	})
	return out, nil
}

// sortAuthoritySites orders sites by file, line, then referenced SymbolID,
// removing exact duplicates from the already-sorted list (req 4).
func sortAuthoritySites(sites []AuthoritySite) []AuthoritySite {
	slices.SortStableFunc(sites, func(a, b AuthoritySite) int {
		if c := facts.CompareSourceSite(a.Site, b.Site); c != 0 {
			return c
		}
		return symbol.Compare(a.Referent, b.Referent)
	})
	out := sites[:0:0]
	for i, s := range sites {
		if i > 0 && sites[i-1].Site == s.Site && sites[i-1].Referent == s.Referent {
			continue
		}
		out = append(out, s)
	}
	return out
}

// ApplyAuthorityPolicy applies the effective capability policy to aggregated
// authority findings (DR-11, DR-17): a violation for every class under
// strict/default policy, an ANALYSIS_LIMITATION warning for AnalysisDefeating
// and an ALLOWED_WITH_WARNING warning for TrueAuthority only when the policy
// explicitly says so, and nothing for an allowed capability. The downgrade is
// always explicit; it is never inferred from anything else.
func ApplyAuthorityPolicy(findings []AuthorityFinding, policy capanalyzer.CapabilityPolicy) ([]report.Finding, []report.Finding) {
	var violations, warnings []report.Finding
	for _, f := range findings {
		switch capanalyzer.Classify(f.Capability, policy) {
		case capanalyzer.DecisionViolation:
			violations = append(violations, authorityReportFinding(f, report.UndeclaredAuthority, violationMessage(f)))
		case capanalyzer.DecisionWarn:
			kind, msg := warnKindAndMessage(f)
			warnings = append(warnings, authorityReportFinding(f, kind, msg))
		case capanalyzer.DecisionAllowed:
		}
	}
	return violations, warnings
}

func violationMessage(f AuthorityFinding) string {
	if f.Class == capanalyzer.AnalysisDefeating {
		return "analysis-defeating construct(s) bypass typed reference analysis"
	}
	return fmt.Sprintf("use of undeclared authority %q", f.Capability)
}

func warnKindAndMessage(f AuthorityFinding) (report.Kind, string) {
	if f.Class == capanalyzer.AnalysisDefeating {
		return report.AnalysisLimitation, "analysis-defeating construct(s) escape typed reference analysis"
	}
	return report.AllowedWithWarning, fmt.Sprintf("capability %q allowed with warning", f.Capability)
}

// authorityReportFinding converts one aggregated finding to its report
// representation: every sorted site, the class, the SDK key and the
// first-site evidence; Location stays empty because the site list owns the
// source positions.
func authorityReportFinding(f AuthorityFinding, kind report.Kind, message string) report.Finding {
	sites := make([]report.AuthoritySite, len(f.Sites))
	for i, s := range f.Sites {
		sites[i] = report.AuthoritySite{File: s.Site.File, Line: s.Site.Line, Symbol: string(s.Referent)}
	}
	evidence := make([]string, 0, len(f.Evidence))
	for _, frame := range f.Evidence {
		evidence = append(evidence, fmt.Sprintf("%s at %s:%d", frame.Function, frame.File, frame.Line))
	}
	return report.Finding{
		Kind:     kind,
		Message:  message,
		Evidence: evidence,
		Class:    string(f.Class),
		SDKKey:   f.SDKKey.String(),
		Sites:    sites,
	}
}
