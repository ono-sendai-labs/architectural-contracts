// Package stdlibauthority defines the core-side port for standard-library
// authority lookup (design R5–R6, DR-05): the terminal classification model,
// the SDK identity key, and the fail-closed lookup contract the reference
// scan consumes. The port is defined here, in the pure decision-side code, and
// implemented shell-side (artifactio's map reader), mirroring
// capanalyzer.CapabilityAnalyzer.
//
// Component Contract (FR10):
//   - What it does: Defines the StdlibAuthority port (total package-membership
//     query, fail-closed symbol and package-init authority, evidence paths,
//     SDK-key identity), the terminal Classification value (exactly one of
//     SAFE, a non-empty capability set, or UNANALYZED), the Frame evidence hop,
//     the SDKKey target-configuration identity with deterministic equality and
//     rendering, and the inventory-gap and key-mismatch errors.
//   - What it requires: Symbol and init lookups name enumerated packages and
//     grammar-valid SymbolIDs; callers compare the returned SDKKey against
//     their target configuration via EqualKeys and fail closed on mismatch.
//   - What it provides: StdlibAuthority, Classification (with Validate),
//     Capability, Frame, SDKKey (with String), EqualKeys, ErrInventoryGap,
//     InventoryGapError, ErrKeyMismatch, KeyMismatchError. All pure and
//     deterministic; returned capability and evidence slices are defensive
//     copies. Nothing here imports protobuf, Capslock, or any I/O.
//   - Ambient Authority: This component is guaranteed-pure and holds no ambient
//     authority (no filesystem I/O, network, process execution or reflection).
package stdlibauthority

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// Capability names one entry of the Capslock capability taxonomy shared by
// manifests, surfaces and maps (schema.KnownCapabilities). It is a plain
// string here so the port stays free of any persisted-artifact dependency.
type Capability = string

// Frame is one hop of an evidence path from a symbol down to the capability
// use (DR-17): the ordered caller-to-capability chain the report renders.
type Frame struct {
	// Fully qualified function or method name of the hop.
	Function string
	// File the call site lives in.
	File string
	// Line within File of the call site.
	Line int
}

// Classification is the terminal classification of a symbol or aggregate init
// (DR-05): exactly one of Safe, a non-empty capability set, or Unanalyzed.
// The zero value is not a valid terminal state — absence of a record is an
// inventory gap, never an implicit SAFE (R6).
type Classification struct {
	// Safe marks the record analysed and capability-free.
	Safe bool
	// Capabilities holds the reached capabilities, sorted, non-empty exactly
	// when Safe and Unanalyzed are false. Callers must not mutate it.
	Capabilities []Capability
	// Unanalyzed marks the record unanalysable (assembly, cgo, linkname,
	// builtins); rendered as an AnalysisDefeating finding at check time (I4).
	Unanalyzed bool
}

// Validate reports whether c is exactly one terminal state. An empty value,
// any combination of states, or a capability set alongside Safe/Unanalyzed is
// rejected; a capability set must additionally be sorted and duplicate-free
// (the persisted canonical form), so the zero value can never read as an
// (empty) SAFE.
func (c Classification) Validate() error {
	states := 0
	if c.Safe {
		states++
	}
	if c.Unanalyzed {
		states++
	}
	if len(c.Capabilities) > 0 {
		states++
	}
	if states == 0 {
		return errors.New("classification names no terminal state; a record must be SAFE, capability-bearing, or UNANALYZED")
	}
	if states != 1 {
		return fmt.Errorf("classification must name exactly one terminal state (SAFE, capabilities, or UNANALYZED); got %d", states)
	}
	for i, cap := range c.Capabilities {
		if i > 0 && cap <= c.Capabilities[i-1] {
			if cap == c.Capabilities[i-1] {
				return fmt.Errorf("classification capabilities must be duplicate-free; %q appears twice", cap)
			}
			return fmt.Errorf("classification capabilities must be sorted: %q after %q", cap, c.Capabilities[i-1])
		}
	}
	return nil
}

// SDKKey identifies the target SDK configuration a stdlib map (or surface)
// was produced against (DR-09). Every field describes the TARGET
// configuration used to compile member code — never the host. Two
// configurations that differ in any field select distinct maps. BuildTags is
// stored sorted; producers must sort before comparing.
type SDKKey struct {
	// Exact target toolchain version including patch (e.g. "go1.26.4").
	ToolchainVersion string
	// Target operating system (GOOS), not the host's.
	GOOS string
	// Target architecture (GOARCH), not the host's.
	GOARCH string
	// Whether cgo is enabled in the target configuration.
	CgoEnabled bool
	// Build tags active in the target configuration, sorted, duplicates
	// rejected at the artifact boundary.
	BuildTags []string
	// GOEXPERIMENT setting of the target toolchain (empty when default).
	GOEXPERIMENT string
	// Hash of the generation classifier text, so generation-time and
	// check-time assumptions cannot drift silently (I2).
	ClassifierHash string
	// Map format version the key was computed for; must equal the map's own
	// format_version, enforced at load (N3).
	MapFormatVersion int32
}

// EqualKeys compares two SDK keys field by field and returns the sorted list
// of mismatched field names (proto field names: toolchain_version, goos,
// goarch, cgo_enabled, build_tags, goexperiment, classifier_hash,
// map_format_version). Build tags are compared as sets after canonical
// sorting, so only genuine content differences are reported. An empty result
// means the keys are equal.
func EqualKeys(a, b SDKKey) []string {
	var fields []string
	add := func(name string) { fields = append(fields, name) }
	if a.ToolchainVersion != b.ToolchainVersion {
		add("toolchain_version")
	}
	if a.GOOS != b.GOOS {
		add("goos")
	}
	if a.GOARCH != b.GOARCH {
		add("goarch")
	}
	if a.CgoEnabled != b.CgoEnabled {
		add("cgo_enabled")
	}
	if !slicesEqualIgnoreOrder(a.BuildTags, b.BuildTags) {
		add("build_tags")
	}
	if a.GOEXPERIMENT != b.GOEXPERIMENT {
		add("goexperiment")
	}
	if a.ClassifierHash != b.ClassifierHash {
		add("classifier_hash")
	}
	if a.MapFormatVersion != b.MapFormatVersion {
		add("map_format_version")
	}
	sort.Strings(fields)
	return fields
}

// slicesEqualIgnoreOrder compares two string slices as multisets after
// canonical sorting: equal sets in any order are equal.
func slicesEqualIgnoreOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := sortStrings(a), sortStrings(b)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func sortStrings(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// String renders the key deterministically: fields in a fixed order, build
// tags sorted, regardless of how the caller stored them.
func (k SDKKey) String() string {
	tags := append([]string(nil), k.BuildTags...)
	sort.Strings(tags)
	var sb strings.Builder
	fmt.Fprintf(&sb, "sdk{toolchain_version:%q goos:%q goarch:%q cgo_enabled:%t build_tags:[%s] goexperiment:%q classifier_hash:%q map_format_version:%d}",
		k.ToolchainVersion, k.GOOS, k.GOARCH, k.CgoEnabled, strings.Join(tags, ","), k.GOEXPERIMENT, k.ClassifierHash, k.MapFormatVersion)
	return sb.String()
}

// StdlibAuthority is the port the analysis consumes; defined core-side,
// implemented shell-side over the persisted map artifact.
type StdlibAuthority interface {
	// IsStdlibPackage reports whether pkgPath is in the map's package
	// enumeration. The enumeration is total; a false answer means "not
	// standard library". Non-importable (internal) packages are members.
	IsStdlibPackage(pkgPath string) bool

	// SymbolAuthority returns the terminal classification of a symbol in an
	// enumerated package. It fails closed: a symbol absent from the package's
	// total inventory is ErrInventoryGap (a tool error), never an empty
	// result (R6).
	SymbolAuthority(id symbol.SymbolID) (Classification, error)

	// PackageInitAuthority returns the classification of pkgPath's aggregate
	// init (R3). It fails closed: a package absent from the enumeration, or
	// enumerated without an init record, is ErrInventoryGap.
	PackageInitAuthority(pkgPath string) (Classification, error)

	// Evidence returns the generator's canned path explaining why id reaches
	// cap. The returned slice is a defensive copy.
	Evidence(id symbol.SymbolID, cap Capability) []Frame

	// Key identifies the target SDK configuration this authority describes
	// (N3). Callers compare it against their target with EqualKeys and fail
	// closed on mismatch.
	Key() SDKKey
}

// ErrInventoryGap is the sentinel for a lookup that names a package or symbol
// absent from the map's total inventory (a tool error, never purity; R6).
var ErrInventoryGap = errors.New("stdlib map inventory gap")

// InventoryGapError is the typed inventory-gap error: it names the absent
// package and, when the gap is a symbol, the absent symbol, so diagnostics
// can point at the map entry that is missing.
type InventoryGapError struct {
	// Package is the package path of the failed lookup.
	Package string
	// Symbol is the SymbolID text of the failed symbol lookup; empty when the
	// gap is a package or init lookup.
	Symbol string
}

// Error implements error, rendering the absent entry.
func (e *InventoryGapError) Error() string {
	if e.Symbol == "" {
		return fmt.Sprintf("stdlib map has no record for package %q", e.Package)
	}
	return fmt.Sprintf("stdlib map has no record for symbol %q in package %q", e.Symbol, e.Package)
}

// Unwrap ties the typed error to ErrInventoryGap so callers can fail closed
// with errors.Is.
func (e *InventoryGapError) Unwrap() error { return ErrInventoryGap }

// ErrKeyMismatch is the sentinel for an SDK-key mismatch between the reader's
// map and the caller's required target configuration (N3).
var ErrKeyMismatch = errors.New("stdlib map SDK key mismatch")

// KeyMismatchError is the typed key-mismatch error: it lists the mismatched
// key field names so diagnostics can show why the map does not apply.
type KeyMismatchError struct {
	// Fields names the mismatched SDK-key fields (EqualKeys's output).
	Fields []string
}

// Error implements error, rendering the mismatched fields.
func (e *KeyMismatchError) Error() string {
	return fmt.Sprintf("stdlib map SDK key does not match the required configuration; mismatched fields: %s", strings.Join(e.Fields, ", "))
}

// Unwrap ties the typed error to ErrKeyMismatch so callers can fail closed
// with errors.Is.
func (e *KeyMismatchError) Unwrap() error { return ErrKeyMismatch }
