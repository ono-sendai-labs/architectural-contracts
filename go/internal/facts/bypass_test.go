package facts_test

import (
	"reflect"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func TestBypassObservationValidate(t *testing.T) {
	valid := facts.BypassObservation{
		Kind: facts.BypassLinkname,
		Site: facts.SourceSite{File: "member/byp/link.go", Line: 5},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
	for name, bad := range map[string]facts.BypassObservation{
		"unknown kind": {Kind: facts.BypassKind("code"), Site: facts.SourceSite{File: "a.go", Line: 1}},
		"empty site":   {Kind: facts.BypassCgo, Site: facts.SourceSite{File: "", Line: 1}},
		"zero line":    {Kind: facts.BypassAssembly, Site: facts.SourceSite{File: "a.s", Line: 0}},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestSortAndDedupBypassObservations(t *testing.T) {
	mk := func(kind facts.BypassKind, file string, line int) facts.BypassObservation {
		return facts.BypassObservation{Kind: kind, Site: facts.SourceSite{File: file, Line: line}}
	}
	in := []facts.BypassObservation{
		mk(facts.BypassCgo, "b/b.go", 3),
		mk(facts.BypassLinkname, "a/a.go", 9),
		mk(facts.BypassAssembly, "a/a.s", 1),
		mk(facts.BypassLinkname, "a/a.go", 2),
		mk(facts.BypassLinkname, "a/a.go", 9), // exact duplicate
	}
	want := []facts.BypassObservation{
		mk(facts.BypassLinkname, "a/a.go", 2),
		mk(facts.BypassLinkname, "a/a.go", 9),
		mk(facts.BypassAssembly, "a/a.s", 1),
		mk(facts.BypassCgo, "b/b.go", 3),
	}
	got := facts.DedupBypassObservations(facts.SortBypassObservations(in))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sorted/deduped = %+v, want %+v", got, want)
	}
}
