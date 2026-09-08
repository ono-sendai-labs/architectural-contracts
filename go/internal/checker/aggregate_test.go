package checker_test

import (
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/checker"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

var aggKey = stdlibauthority.SDKKey{ToolchainVersion: "go1.26.4", GOOS: "linux", GOARCH: "amd64", MapFormatVersion: 1}

func aggSite(file string, line int) facts.SourceSite {
	return facts.SourceSite{File: file, Line: line}
}

func trueObs(file string, line int, sym string, caps ...string) checker.AuthorityObservation {
	id := symbol.SymbolID(sym)
	return checker.AuthorityObservation{
		Class:      capanalyzer.TrueAuthority,
		Capability: caps[0],
		Referent:   facts.SymbolID(id),
		Site:       aggSite(file, line),
		SDKKey:     aggKey,
		Evidence: []stdlibauthority.Frame{
			{Function: sym, File: sym + ".go", Line: 10},
		},
	}
}

// TestAggregateAuthorityOnePerCapabilityClass pins req 4: three member
// references contributing the same (capability, class) in shuffled order
// aggregate to exactly one finding whose sites are fully sorted and free of
// exact duplicates.
func TestAggregateAuthorityOnePerCapabilityClass(t *testing.T) {
	obs := []checker.AuthorityObservation{
		trueObs("member/c.go", 11, "os.Create", "FILES"),
		trueObs("member/a.go", 3, "os.ReadFile", "FILES"),
		trueObs("member/b.go", 7, "os.Stdin", "CHDIR"),
		trueObs("member/b.go", 7, "os.Stdin", "FILES"),    // same site, other capability
		trueObs("member/a.go", 3, "os.ReadFile", "FILES"), // exact duplicate
	}
	got, err := checker.AggregateAuthority(obs, nil, aggKey)
	if err != nil {
		t.Fatalf("unexpected aggregation error: %v", err)
	}
	var files *checker.AuthorityFinding
	for i := range got {
		if got[i].Capability == "FILES" {
			if files != nil {
				t.Fatalf("want one FILES finding, got several: %+v", got)
			}
			files = &got[i]
		}
	}
	if files == nil {
		t.Fatalf("no FILES finding in %+v", got)
	}
	if files.Class != capanalyzer.TrueAuthority || len(stdlibauthority.EqualKeys(files.SDKKey, aggKey)) != 0 {
		t.Errorf("FILES finding class/key wrong: %+v", files)
	}
	wantSites := []checker.AuthoritySite{
		{Site: aggSite("member/a.go", 3), Referent: facts.SymbolID(symbol.SymbolID("os.ReadFile"))},
		{Site: aggSite("member/b.go", 7), Referent: facts.SymbolID(symbol.SymbolID("os.Stdin"))},
		{Site: aggSite("member/c.go", 11), Referent: facts.SymbolID(symbol.SymbolID("os.Create"))},
	}
	if !reflect.DeepEqual(files.Sites, wantSites) {
		t.Errorf("sites = %+v, want %+v", files.Sites, wantSites)
	}
	if !reflect.DeepEqual(files.Evidence, []stdlibauthority.Frame{{Function: "os.ReadFile", File: "os.ReadFile.go", Line: 10}}) {
		t.Errorf("evidence must be the first sorted site's symbol's evidence, got %+v", files.Evidence)
	}
}

// TestAggregateAuthorityAnalysisDefeating pins reqs 2-3: UNANALYZED stdlib
// observations and member bypass constructs merge into one AnalysisDefeating
// finding carrying every site (referent only for the stdlib ones) and the map
// SDK key, with no evidence.
func TestAggregateAuthorityAnalysisDefeating(t *testing.T) {
	unanalyzed := checker.AuthorityObservation{
		Class:    capanalyzer.AnalysisDefeating,
		Referent: facts.SymbolID(symbol.SymbolID("go/ast.init")),
		Site:     aggSite("member/rows/rows.go", 12),
		SDKKey:   aggKey,
	}
	bypasses := []facts.BypassObservation{
		{Kind: facts.BypassLinkname, Site: aggSite("member/byp/link.go", 3)},
		{Kind: facts.BypassAssembly, Site: aggSite("member/byp/stub.s", 1)},
		{Kind: facts.BypassCgo, Site: aggSite("member/byp/cgo.go", 4)},
	}
	got, err := checker.AggregateAuthority([]checker.AuthorityObservation{unanalyzed}, bypasses, aggKey)
	if err != nil {
		t.Fatalf("unexpected aggregation error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one AnalysisDefeating finding, got %+v", got)
	}
	f := got[0]
	if f.Class != capanalyzer.AnalysisDefeating || f.Capability != "" || len(stdlibauthority.EqualKeys(f.SDKKey, aggKey)) != 0 || len(f.Evidence) != 0 {
		t.Errorf("finding header wrong: %+v", f)
	}
	wantSites := []checker.AuthoritySite{
		{Site: aggSite("member/byp/cgo.go", 4)},
		{Site: aggSite("member/byp/link.go", 3)},
		{Site: aggSite("member/byp/stub.s", 1)},
		{Site: aggSite("member/rows/rows.go", 12), Referent: facts.SymbolID(symbol.SymbolID("go/ast.init"))},
	}
	if !reflect.DeepEqual(f.Sites, wantSites) {
		t.Errorf("sites = %+v, want %+v", f.Sites, wantSites)
	}
}

func TestAggregateAuthorityKeyMismatchFailsClosed(t *testing.T) {
	other := aggKey
	other.GOARCH = "arm64"
	obs := []checker.AuthorityObservation{trueObs("member/a.go", 1, "os.ReadFile", "FILES")}
	obs[0].SDKKey = other
	if _, err := checker.AggregateAuthority(obs, nil, aggKey); err == nil {
		t.Fatalf("mixed SDK keys must fail closed")
	}
}

// TestApplyAuthorityPolicyStrictViolates pins req 1: strict/default policy is
// a violation for both classes, carrying the sites, class, SDK key and
// first-site evidence into the finding.
func TestApplyAuthorityPolicyStrictViolates(t *testing.T) {
	finding := checker.AuthorityFinding{
		Class:      capanalyzer.TrueAuthority,
		Capability: "FILES",
		SDKKey:     aggKey,
		Sites: []checker.AuthoritySite{
			{Site: aggSite("member/a.go", 3), Referent: facts.SymbolID(symbol.SymbolID("os.ReadFile"))},
			{Site: aggSite("member/b.go", 7), Referent: facts.SymbolID(symbol.SymbolID("os.Stdin"))},
		},
		Evidence: []stdlibauthority.Frame{{Function: "os.ReadFile", File: "os/file.go", Line: 331}},
	}
	defeating := checker.AuthorityFinding{
		Class:  capanalyzer.AnalysisDefeating,
		SDKKey: aggKey,
		Sites:  []checker.AuthoritySite{{Site: aggSite("member/byp/link.go", 3)}},
	}
	violations, warnings := checker.ApplyAuthorityPolicy(
		[]checker.AuthorityFinding{finding, defeating}, capanalyzer.StrictPolicy())
	if len(warnings) != 0 {
		t.Errorf("strict policy must produce no warnings, got %+v", warnings)
	}
	if len(violations) != 2 {
		t.Fatalf("want two violations, got %+v", violations)
	}
	for _, v := range violations {
		if v.Kind != report.UndeclaredAuthority {
			t.Errorf("violation kind = %q, want UNDECLARED_AUTHORITY", v.Kind)
		}
	}
	v := violations[0]
	if v.Class != "TrueAuthority" || v.SDKKey != aggKey.String() || len(v.Sites) != 2 {
		t.Errorf("violation must carry class, SDK key and every site: %+v", v)
	}
	if v.Location.File != "" {
		t.Errorf("aggregated violation must leave Location to the site list, got %+v", v.Location)
	}
	var defeatingV *report.Finding
	for i := range violations {
		if violations[i].Class == "AnalysisDefeating" {
			defeatingV = &violations[i]
		}
	}
	if defeatingV == nil {
		t.Fatalf("AnalysisDefeating violation missing: %+v", violations)
	}
	if len(defeatingV.Sites) != 1 || defeatingV.Sites[0].File != "member/byp/link.go" {
		t.Errorf("AnalysisDefeating violation must carry its bypass site: %+v", defeatingV)
	}
}

// TestApplyAuthorityPolicyExplicitWarn pins req 2: an explicit warn policy for
// the analysis-defeating capability renders exactly one ANALYSIS_LIMITATION
// warning and no violation.
func TestApplyAuthorityPolicyExplicitWarn(t *testing.T) {
	defeating := checker.AuthorityFinding{
		Class:  capanalyzer.AnalysisDefeating,
		SDKKey: aggKey,
		Sites: []checker.AuthoritySite{
			{Site: aggSite("member/byp/link.go", 3)},
			{Site: aggSite("member/byp/stub.s", 1)},
			{Site: aggSite("member/byp/cgo.go", 4)},
		},
	}
	policy := capanalyzer.CapabilityPolicy{Warn: map[string]bool{"": true}}
	violations, warnings := checker.ApplyAuthorityPolicy([]checker.AuthorityFinding{defeating}, policy)
	if len(violations) != 0 {
		t.Errorf("explicit warn policy must remove the violation, got %+v", violations)
	}
	if len(warnings) != 1 {
		t.Fatalf("want exactly one warning, got %+v", warnings)
	}
	if warnings[0].Kind != report.AnalysisLimitation || warnings[0].Class != "AnalysisDefeating" || len(warnings[0].Sites) != 3 {
		t.Errorf("warning must be ANALYSIS_LIMITATION with all sites: %+v", warnings[0])
	}
}

func TestApplyAuthorityPolicyTrueAuthorityWarn(t *testing.T) {
	finding := checker.AuthorityFinding{
		Class:      capanalyzer.TrueAuthority,
		Capability: "FILES",
		SDKKey:     aggKey,
		Sites:      []checker.AuthoritySite{{Site: aggSite("member/a.go", 3), Referent: facts.SymbolID(symbol.SymbolID("os.ReadFile"))}},
	}
	policy := capanalyzer.CapabilityPolicy{Warn: map[string]bool{"FILES": true}}
	violations, warnings := checker.ApplyAuthorityPolicy([]checker.AuthorityFinding{finding}, policy)
	if len(violations) != 0 || len(warnings) != 1 {
		t.Fatalf("want one warning, no violations: %d/%d", len(violations), len(warnings))
	}
	if warnings[0].Kind != report.AllowedWithWarning {
		t.Errorf("TrueAuthority warn kind = %q, want ALLOWED_WITH_WARNING", warnings[0].Kind)
	}
}
