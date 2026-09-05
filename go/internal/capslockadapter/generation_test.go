package capslockadapter

import (
	"slices"
	"strings"
	"testing"
)

// TestGenerationClassifierText pins the generation classifier's source text:
// exactly the 22 minting-site reclassification lines, deterministic, and
// nothing else (no check-time prune symbols, no package-granularity rules).
func TestGenerationClassifierText(t *testing.T) {
	txt, err := GenerationClassifierText()
	if err != nil {
		t.Fatalf("GenerationClassifierText: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(txt, "\n"), "\n")
	if len(lines) != len(fileHandleUseMethods) {
		t.Fatalf("generation classifier text has %d lines, want %d (the minting reclassifications):\n%s", len(lines), len(fileHandleUseMethods), txt)
	}
	for _, m := range fileHandleUseMethods {
		want := "func " + m + " CAPABILITY_SAFE"
		if !slices.Contains(lines, want) {
			t.Fatalf("generation classifier text missing reclassification %q:\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "package ") {
		t.Fatalf("generation classifier text contains a package-granularity rule:\n%s", txt)
	}
	for i := 0; i < 5; i++ {
		again, err := GenerationClassifierText()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if again != txt {
			t.Fatalf("generation classifier text is not deterministic")
		}
	}
}

// TestMintingAuthority pins the var-rule minting table: the receiver types
// whose reclassified handle-use methods mint ambient authority, and the
// capability each mints. Every receiver named here must own reclassified
// methods, and every reclassified method's receiver must appear here.
func TestMintingAuthority(t *testing.T) {
	minting := MintingAuthority()
	if len(minting) == 0 {
		t.Fatalf("MintingAuthority is empty; the var rule has no minting sites")
	}
	for _, m := range fileHandleUseMethods {
		recv, ok := strings.CutPrefix(m, "(*")
		if !ok {
			t.Fatalf("reclassified method %q is not a pointer-receiver spelling", m)
		}
		recv = strings.TrimSuffix(recv, ")")
		dot := strings.LastIndexByte(recv, '.')
		if dot < 0 {
			t.Fatalf("reclassified method %q has no package-qualified receiver", m)
		}
		recv = recv[:dot]
		if _, ok := minting[recv]; !ok {
			t.Fatalf("reclassified method %q names receiver %q, absent from MintingAuthority %v", m, recv, minting)
		}
	}
	if cap, ok := minting["os.File"]; !ok || cap != "FILES" {
		t.Fatalf("MintingAuthority[os.File] = %q, %v; want FILES", cap, ok)
	}
}
