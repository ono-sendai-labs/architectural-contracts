// Package teststdlibmap provides the one checked-in stdlib authority map used
// by routine integration tests. It is deliberately a test-only package: the
// production native path continues to generate and cache maps on demand.
package teststdlibmap

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

const (
	PinnedToolchain = "go1.26.4"
	PinnedGOOS      = "linux"
	PinnedGOARCH    = "amd64"
)

//go:embed testdata/linux_amd64.stdlib-map.json
var pinnedMap []byte

// PinnedTarget is the exact routine integration target. Returning a value
// prevents callers from changing the package-level pin.
func PinnedTarget() stdlibmap.TargetConfig {
	return stdlibmap.TargetConfig{ToolchainVersion: PinnedToolchain, GOOS: PinnedGOOS, GOARCH: PinnedGOARCH}
}

// ExpectedKey derives every SDK-key field from the current production
// classifier descriptor. The artifact's self-reported key is never trusted
// as the source of this expectation.
func ExpectedKey() (stdlibauthority.SDKKey, error) {
	rules, err := stdlibmap.GenerationClassifierRules()
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("reading generation classifier rules: %w", err)
	}
	text, err := stdlibmap.CanonicalClassifierText(rules)
	if err != nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("canonicalizing generation classifier rules: %w", err)
	}
	return stdlibmap.DeriveSDKKey(stdlibmap.GenerationDescriptor{
		Target:           PinnedTarget(),
		ClassifierText:   text,
		RuleVersion:      stdlibmap.RuleVersion,
		MapFormatVersion: artifactio.MapFormatVersion,
	}), nil
}

// OpenPinned validates and opens the checked artifact through the production
// bounded decoder and immutable reader. It is intended for integration tests.
func OpenPinned(t testing.TB) stdlibauthority.StdlibAuthority {
	t.Helper()
	key, err := ExpectedKey()
	if err != nil {
		t.Fatalf("derive pinned stdlib-map key: %v", err)
	}
	authority, err := artifactio.OpenStdlibMap(bytes.NewReader(pinnedMap), &key)
	if err != nil {
		t.Fatalf("open pinned stdlib map: %v", err)
	}
	return authority
}

// PinnedBytes returns a defensive copy of the checked artifact bytes.
func PinnedBytes() []byte { return append([]byte(nil), pinnedMap...) }

// WritePinned copies the checked artifact to a temporary path for subprocess
// tests. It never uses the production cache.
func WritePinned(t testing.TB) string {
	t.Helper()
	// The returned path may be retained by several integration tests in one
	// process after the creating test's cleanup phase. Keep this independent
	// temporary directory alive for the process; it is never the user cache.
	dir, err := os.MkdirTemp("", "arcc-pinned-stdlib-map-")
	if err != nil {
		t.Fatalf("create pinned stdlib map directory: %v", err)
	}
	path := filepath.Join(dir, "linux_amd64.stdlib-map.json")
	if err := os.WriteFile(path, pinnedMap, 0o644); err != nil {
		t.Fatalf("write pinned stdlib map: %v", err)
	}
	return path
}

// WriteSynthetic writes a small, semantically valid map for tests whose
// target-selection assertions do not require the real SDK inventory. It is
// keyed with the supplied target but still uses the live production
// classifier descriptor, so it cannot accidentally exercise a stale key.
func WriteSynthetic(t testing.TB, target stdlibmap.TargetConfig, packages, symbols []string) string {
	t.Helper()
	key, err := expectedKey(target)
	if err != nil {
		t.Fatalf("derive synthetic stdlib-map key: %v", err)
	}
	m := &gen.StdlibMap{FormatVersion: artifactio.MapFormatVersion, Key: &gen.SDKKey{
		ToolchainVersion: key.ToolchainVersion,
		Goos:             key.GOOS,
		Goarch:           key.GOARCH,
		CgoEnabled:       key.CgoEnabled,
		BuildTags:        key.BuildTags,
		Goexperiment:     key.GOEXPERIMENT,
		ClassifierHash:   key.ClassifierHash,
		MapFormatVersion: key.MapFormatVersion,
	}}
	for _, pkg := range packages {
		m.Packages = append(m.Packages, &gen.PackageInventory{Path: pkg, Importable: true})
		m.Inits = append(m.Inits, &gen.InitRecord{Package: pkg, Classification: gen.Classification_SAFE})
	}
	for _, id := range symbols {
		parsed, err := symbol.Parse(id)
		if err != nil {
			t.Fatalf("parse synthetic symbol %q: %v", id, err)
		}
		m.Symbols = append(m.Symbols, &gen.SymbolRecord{Package: packageOf(parsed), Id: id, Classification: gen.Classification_SAFE})
	}
	data, err := artifactio.MarshalMap(m)
	if err != nil {
		t.Fatalf("marshal synthetic stdlib map: %v", err)
	}
	dir, err := os.MkdirTemp("", "arcc-synthetic-stdlib-map-")
	if err != nil {
		t.Fatalf("create synthetic stdlib map directory: %v", err)
	}
	path := filepath.Join(dir, "synthetic.stdlib-map.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write synthetic stdlib map: %v", err)
	}
	return path
}

func expectedKey(target stdlibmap.TargetConfig) (stdlibauthority.SDKKey, error) {
	rules, err := stdlibmap.GenerationClassifierRules()
	if err != nil {
		return stdlibauthority.SDKKey{}, err
	}
	text, err := stdlibmap.CanonicalClassifierText(rules)
	if err != nil {
		return stdlibauthority.SDKKey{}, err
	}
	return stdlibmap.DeriveSDKKey(stdlibmap.GenerationDescriptor{
		Target: target, ClassifierText: text, RuleVersion: stdlibmap.RuleVersion,
		MapFormatVersion: artifactio.MapFormatVersion,
	}), nil
}

func packageOf(id symbol.SymbolID) string {
	text := id.Format()
	if strings.HasPrefix(text, "(") {
		receiver := strings.TrimPrefix(strings.SplitN(text, ")", 2)[0], "(")
		return receiver[:strings.LastIndexByte(receiver, '.')]
	}
	return text[:strings.LastIndexByte(text, '.')]
}

// OpenPinnedReader is the non-testing variant for helpers that already own a
// reader lifecycle; it applies the same complete live-key validation.
func OpenPinnedReader() (stdlibauthority.StdlibAuthority, error) {
	key, err := ExpectedKey()
	if err != nil {
		return nil, err
	}
	return artifactio.OpenStdlibMap(bytes.NewReader(pinnedMap), &key)
}

// PinnedReader returns a fresh reader over the embedded artifact.
func PinnedReader() io.Reader { return bytes.NewReader(pinnedMap) }

// MustSymbolID is a test convenience for asserting a real map lookup.
func MustSymbolID(t testing.TB, text string) symbol.SymbolID {
	t.Helper()
	id, err := symbol.Parse(text)
	if err != nil {
		t.Fatalf("parse symbol %q: %v", text, err)
	}
	return id
}
