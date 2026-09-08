package capslockadapter

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/capslock/interesting"
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
		end := strings.IndexByte(recv, ')')
		if end < 0 {
			t.Fatalf("reclassified method %q has no closing receiver bracket", m)
		}
		recv = recv[:end]
		if _, ok := minting[recv]; !ok {
			t.Fatalf("reclassified method %q names receiver %q, absent from MintingAuthority %v", m, recv, minting)
		}
	}
	if cap, ok := minting["os.File"]; !ok || cap != "FILES" {
		t.Fatalf("MintingAuthority[os.File] = %q, %v; want FILES", cap, ok)
	}
}

// TestBuiltinClassifierPin pins the builtin content pin (review F1, task req
// 2): it is a stable 64-hex SHA-256 digest of Capslock's exact builtin
// classifier content, so a builtin classification change necessarily changes
// the SDK key's classifier_hash.
func TestBuiltinClassifierPin(t *testing.T) {
	first, err := BuiltinClassifierPin()
	if err != nil {
		t.Fatalf("BuiltinClassifierPin: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(first) {
		t.Fatalf("BuiltinClassifierPin = %q; want a 64-hex digest", first)
	}
	for i := 0; i < 5; i++ {
		again, err := BuiltinClassifierPin()
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if again != first {
			t.Fatalf("builtin classifier pin is not deterministic:\n%s\n%s", first, again)
		}
	}
}

// TestClassifierContentDigestSensitivity proves the content digest tracks the
// builtin classifier's semantics: the bare builtin classifier, an empty
// classifier, and the builtin merged with one extra overlay line all digest
// differently — a change anywhere in the classifier content moves the pin.
func TestClassifierContentDigestSensitivity(t *testing.T) {
	builtin := interesting.DefaultClassifier()
	bare, err := classifierContentDigest(builtin)
	if err != nil {
		t.Fatalf("classifierContentDigest(builtin): %v", err)
	}
	empty, err := interesting.LoadClassifier("test", strings.NewReader(""), true /* excludeBuiltin */)
	if err != nil {
		t.Fatalf("LoadClassifier(empty): %v", err)
	}
	emptyDigest, err := classifierContentDigest(empty)
	if err != nil {
		t.Fatalf("classifierContentDigest(empty): %v", err)
	}
	if bare == emptyDigest {
		t.Fatalf("the builtin classifier and an empty classifier digest identically")
	}
	merged, err := interesting.LoadClassifier("test", strings.NewReader("func os.Open CAPABILITY_SAFE\n"), false)
	if err != nil {
		t.Fatalf("LoadClassifier(overlay): %v", err)
	}
	mergedDigest, err := classifierContentDigest(merged)
	if err != nil {
		t.Fatalf("classifierContentDigest(merged): %v", err)
	}
	if mergedDigest == bare {
		t.Fatalf("adding one overlay classification did not change the digest")
	}
	if pin, err := BuiltinClassifierPin(); err != nil || pin != bare {
		t.Fatalf("BuiltinClassifierPin = %q, %v; want the bare builtin digest %q", pin, err, bare)
	}
}
