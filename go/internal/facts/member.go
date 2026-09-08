package facts

import (
	"fmt"
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// MemberSet is the minimal effective-member set of a component: the exact
// set of canonical import paths whose packages are component members. It is
// a core value with no shell data: membership is exact string equality on
// canonical paths, never a prefix or pattern match (pattern membership is
// removed, DR-16).
//
// Construction is deterministic: duplicates and noncanonical paths are
// rejected, and the stored paths are sorted.
type MemberSet struct {
	pkgs []string // sorted, unique, canonical
}

// NewMemberSet builds a MemberSet from canonical package import paths. It
// rejects empty paths, duplicates and paths that are not fixed points of the
// host canonicalizer, naming the offending path.
func NewMemberSet(paths ...string) (MemberSet, error) {
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)
	ms := MemberSet{pkgs: sorted[:0:0]}
	prev := ""
	for i, p := range sorted {
		if p == "" {
			return MemberSet{}, fmt.Errorf("member set: empty package path at index %d", i)
		}
		if i > 0 && p == prev {
			return MemberSet{}, fmt.Errorf("member set: duplicate package path %q", p)
		}
		if err := symbol.ValidateCanonicalPath(p); err != nil {
			return MemberSet{}, fmt.Errorf("member set: %w", err)
		}
		ms.pkgs = append(ms.pkgs, p)
		prev = p
	}
	return ms, nil
}

// Contains reports whether pkg is exactly one of the member packages. It
// performs no canonicalization: a non-exact spelling is simply not a member.
func (m MemberSet) Contains(pkg string) bool {
	i := sort.SearchStrings(m.pkgs, pkg)
	return i < len(m.pkgs) && m.pkgs[i] == pkg
}

// Packages returns the member package paths as a sorted copy.
func (m MemberSet) Packages() []string {
	out := make([]string, len(m.pkgs))
	copy(out, m.pkgs)
	return out
}

// Len returns the number of member packages.
func (m MemberSet) Len() int {
	return len(m.pkgs)
}
