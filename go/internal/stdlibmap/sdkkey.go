package stdlibmap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// TargetConfig describes the target build configuration a map is generated
// for (task reqs 7–8): every field describes the TARGET, never the host, so a
// cross-compile derives the target's key while running on any host.
type TargetConfig struct {
	// ToolchainVersion is the exact target toolchain version including patch
	// (e.g. "go1.26.4").
	ToolchainVersion string
	// GOOS is the target operating system.
	GOOS string
	// GOARCH is the target architecture.
	GOARCH string
	// CgoEnabled is the target cgo state.
	CgoEnabled bool
	// BuildTags are the target build tags, in any order; DeriveSDKKey sorts
	// them into the canonical key form.
	BuildTags []string
	// GOEXPERIMENT is the target GOEXPERIMENT setting (empty when default).
	GOEXPERIMENT string
}

// ClassifierRule is one entry of the generation classifier's source material
// (design I2, "Centralize generation classifier/rule source material in a
// hashable descriptor"): a stable rule name and its effect text. Task 3's
// classifier implementation renders its builtins, the minting-site
// reclassification and the var/handle rule through this type so the stamped
// classifier_hash cannot drift from the rules actually applied.
type ClassifierRule struct {
	// Name is the stable rule identifier (e.g. "unsafe.builtins").
	Name string
	// Effect is the rule's deterministic description.
	Effect string
}

// CanonicalClassifierText renders the complete generation classifier text in
// a canonical form: rules sorted by name, so non-semantic iteration order
// never changes the text or its hash (task AC 5). Duplicate rule names fail
// with an actionable error rather than producing an ambiguous descriptor.
func CanonicalClassifierText(rules []ClassifierRule) (string, error) {
	sorted := append([]ClassifierRule(nil), rules...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var sb strings.Builder
	prev := ""
	for i, r := range sorted {
		if i > 0 && r.Name == prev {
			return "", fmt.Errorf("classifier rule %q appears twice; the descriptor must name each rule exactly once", r.Name)
		}
		prev = r.Name
		fmt.Fprintf(&sb, "%s\x1f%s\x1e", r.Name, r.Effect)
	}
	return sb.String(), nil
}

// GenerationDescriptor centralizes every input the map's SDK key depends on
// (task req 7): the target configuration and the hashable classifier source
// material — the complete generation classifier text plus an explicit
// rule-version string — so a later classifier change cannot silently escape
// the stamped classifier_hash (I2).
type GenerationDescriptor struct {
	// Target is the target build configuration (task req 8).
	Target TargetConfig
	// ClassifierText is the complete generation classifier text, canonically
	// rendered (CanonicalClassifierText).
	ClassifierText string
	// RuleVersion is the explicit version string of the classification rules
	// themselves, bumped when the rules change even if the text spelling does
	// not.
	RuleVersion string
	// MapFormatVersion is the map format version the key is derived for; it
	// must equal the map's own format_version (N3).
	MapFormatVersion int32
}

// classifierHashSeparator delimits the classifier text from the rule version
// inside the hashed material; the separators never appear in either field.
const classifierHashSeparator = "\x00"

// ClassifierHash returns the deterministic classifier fingerprint (task req
// 7): SHA-256 over the complete generation classifier text plus the explicit
// rule-version string, hex-encoded. Any change to either input changes the
// hash.
func ClassifierHash(d GenerationDescriptor) string {
	h := sha256.New()
	h.Write([]byte(d.ClassifierText))
	h.Write([]byte(classifierHashSeparator))
	h.Write([]byte(d.RuleVersion))
	return hex.EncodeToString(h.Sum(nil))
}

// DeriveSDKKey derives the target's SDKKey from the generation descriptor
// (task reqs 7–8): the target's toolchain version, GOOS/GOARCH, cgo state,
// sorted build tags and GOEXPERIMENT, the map format version, and the
// classifier_hash. Repeated derivation over identical descriptors is
// byte-for-byte stable.
func DeriveSDKKey(d GenerationDescriptor) stdlibauthority.SDKKey {
	tags := append([]string(nil), d.Target.BuildTags...)
	sort.Strings(tags)
	return stdlibauthority.SDKKey{
		ToolchainVersion: d.Target.ToolchainVersion,
		GOOS:             d.Target.GOOS,
		GOARCH:           d.Target.GOARCH,
		CgoEnabled:       d.Target.CgoEnabled,
		BuildTags:        tags,
		GOEXPERIMENT:     d.Target.GOEXPERIMENT,
		ClassifierHash:   ClassifierHash(d),
		MapFormatVersion: d.MapFormatVersion,
	}
}
