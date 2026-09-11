// Package manifest defines the component manifest parser and native model.
//
// Component Contract (FR10):
// - What it does: Parses textproto component manifests into a hand-written Go-native Manifest model and validates its syntax.
// - What it requires: An io.Reader representing the textproto manifest source (as an object capability).
// - What it provides: A validated Manifest native model with syntactic correctness guaranteed.
// - Ambient Authority: This component is clean of filesystem/network ambient authority but leverages REFLECT, RUNTIME, SYSTEM_CALLS, and UNSAFE_POINTER internally via protobuf unmarshaling.
package manifest

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"google.golang.org/protobuf/encoding/prototext"
)

var (
	ErrEmptyName                     = errors.New("manifest name cannot be empty")
	ErrEmptyInterfaceFiles           = errors.New("manifest must declare at least one interface file")
	ErrPackageSurfaceRequiresMembers = errors.New("package-surface component requires at least one member")
	ErrPackageSurfaceInterfaceFiles  = errors.New("package-surface component must not declare interface files")
)

// InterfaceStyle controls how a component's interface is determined.
type InterfaceStyle uint8

const (
	// InterfaceStyleUnspecified preserves the declared interface_files behavior.
	InterfaceStyleUnspecified InterfaceStyle = iota
	// InterfaceStylePackageSurface exposes every exported symbol in each member package.
	InterfaceStylePackageSurface
)

// UnknownInterfaceStyleError reports an interface style value not understood by this version.
type UnknownInterfaceStyleError struct {
	Value int32
}

func (e *UnknownInterfaceStyleError) Error() string {
	return fmt.Sprintf("unknown interface style: %d", e.Value)
}

// RemovedFieldError reports a manifest field that existed in an earlier schema
// version but has been removed. Stale manifests must fail loudly rather than
// have the removed declaration silently ignored by the lenient textproto
// unmarshaler.
type RemovedFieldError struct {
	Field string
}

func (e *RemovedFieldError) Error() string {
	hint, ok := removedFieldHints[e.Field]
	if !ok {
		hint = "this field was removed"
	}
	return fmt.Sprintf("unsupported manifest field %q: %s", e.Field, hint)
}

// removedFieldHints give each retired field an actionable migration message.
var removedFieldHints = map[string]string{
	"absorbed_dependencies":   "this field was removed; migrate to explicit members or component_dependencies",
	"own_check_runs":          "this field was removed; verification provenance is derived from check artifacts, not author declarations",
	"certification_reference": "this field was removed; verification provenance is derived from check artifacts, not author declarations",
}

// removedFields are schema fields that must be rejected at parse time, with a
// textual match at a field-name position (followed by ':' or '{').
var removedFields = []struct {
	field   string
	pattern *regexp.Regexp
}{
	{
		field:   "absorbed_dependencies",
		pattern: regexp.MustCompile(`(^|[^\w.])absorbed_dependencies\s*[:{]`),
	},
	{
		field:   "own_check_runs",
		pattern: regexp.MustCompile(`(^|[^\w.])own_check_runs\s*[:{]`),
	},
	{
		field:   "certification_reference",
		pattern: regexp.MustCompile(`(^|[^\w.])certification_reference\s*[:{]`),
	},
}

// checkRemovedFields rejects textproto content naming fields that no longer
// exist in the schema. It runs before unmarshaling because the protobuf
// unmarshaler silently ignores unknown fields, which would drop a stale
// declaration without a trace. Comments and quoted string values are stripped
// first so the removed-field names are only matched at field-name positions,
// never inside prose or data.
func checkRemovedFields(text []byte) error {
	scrubbed := scrubCommentsAndStrings(text)
	for _, rf := range removedFields {
		if rf.pattern.Match(scrubbed) {
			return &RemovedFieldError{Field: rf.field}
		}
	}
	return nil
}

// scrubCommentsAndStrings removes '#' comments and replaces the contents of
// quoted string literals so removed-field detection cannot be tripped by
// occurrences inside comments or data values.
func scrubCommentsAndStrings(text []byte) []byte {
	scrubbed := make([]byte, len(text))
	copy(scrubbed, text)
	var quote byte // 0 when not inside a string literal
	for i := 0; i < len(scrubbed); i++ {
		c := scrubbed[i]
		switch {
		case quote != 0:
			if c == '\\' && i+1 < len(scrubbed) {
				scrubbed[i+1] = ' '
				i++
				continue
			}
			if c == quote {
				quote = 0
			} else {
				scrubbed[i] = ' '
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			for i < len(scrubbed) && scrubbed[i] != '\n' {
				scrubbed[i] = ' '
				i++
			}
		}
	}
	return scrubbed
}

// InvalidMemberError reports a member that is empty or is not a literal
// import path (it contains glob metacharacters).
type InvalidMemberError struct {
	Member string
	Reason string
}

func (e *InvalidMemberError) Error() string {
	return fmt.Sprintf("invalid member %q: %s", e.Member, e.Reason)
}

// DuplicateDeclarationError represents a duplicate declaration error.
type DuplicateDeclarationError struct {
	Kind  string // "interface file", "component dependency", "member", "declared authority"
	Value string
}

func (e *DuplicateDeclarationError) Error() string {
	return fmt.Sprintf("duplicate %s: %s", e.Kind, e.Value)
}

// UnknownCapabilityError represents an unknown capability error.
type UnknownCapabilityError struct {
	Capability string
}

func (e *UnknownCapabilityError) Error() string {
	return fmt.Sprintf("unknown capability: %s", e.Capability)
}

// Manifest represents the native hand-written Go model for a component manifest,
// shielding the rest of the application from protobuf definitions.
type Manifest struct {
	Name                  string
	InterfaceFiles        []string
	ComponentDependencies []ComponentDependency
	DeclaredAuthority     []string
	// Authority is the structural declaration over the persisted axis: a
	// known, verified declaration (default) or the unknown value for a
	// component that has not been analysed. DeclaredAuthority is retained as
	// the declared capability list the checker consumes during the transition.
	Authority      AuthorityDeclaration
	Members        []string
	InterfaceStyle InterfaceStyle
}

// ComponentDependency represents a dependency on a first-class component.
type ComponentDependency struct {
	Name         string
	Manifest     string
	AutoAttached bool
}

// Parse reads textproto content from the provided reader, unmarshals it into the
// generated message, and maps it onto the native model. It is ambient-authority-free.
func Parse(r io.Reader) (Manifest, error) {
	bytes, err := readAll(r)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading manifest: %w", err)
	}

	if err := checkRemovedFields(bytes); err != nil {
		return Manifest{}, err
	}

	var pbComponent gen.Component
	if err := prototext.Unmarshal(bytes, &pbComponent); err != nil {
		return Manifest{}, fmt.Errorf("unmarshaling textproto: %w", err)
	}

	interfaceStyle, err := nativeInterfaceStyle(pbComponent.GetInterfaceStyle())
	if err != nil {
		return Manifest{}, err
	}

	// The persisted axis defaults to a known declaration, and an UNKNOWN
	// component must not claim declared capabilities (R10, R11).
	authority, err := FromPersisted(pbComponent.GetAuthority(), pbComponent.GetDeclaredAuthority())
	if err != nil {
		return Manifest{}, err
	}

	m := Manifest{
		Name:              pbComponent.GetName(),
		InterfaceFiles:    copyStrings(pbComponent.GetInterfaceFiles()),
		DeclaredAuthority: copyStrings(pbComponent.GetDeclaredAuthority()),
		Authority:         authority,
		Members:           copyStrings(pbComponent.GetMembers()),
		InterfaceStyle:    interfaceStyle,
	}

	if pbDeps := pbComponent.GetComponentDependencies(); len(pbDeps) > 0 {
		m.ComponentDependencies = make([]ComponentDependency, len(pbDeps))
		for i, pbDep := range pbDeps {
			m.ComponentDependencies[i] = ComponentDependency{
				Name:         pbDep.GetName(),
				Manifest:     pbDep.GetManifest(),
				AutoAttached: pbDep.GetAutoAttached(),
			}
		}
	}

	if err := validate(m); err != nil {
		return Manifest{}, err
	}

	return m, nil
}

// readAll copies the supplied reader without routing through io.ReadAll. The
// stdlib authority map intentionally keeps io.ReadAll UNANALYZED because its
// implementation dispatches through an arbitrary reader; spelling the one
// interface read here lets the component boundary and the reader capability
// remain visible to the typed scan (Step 7 AC8b migration).
func readAll(r io.Reader) ([]byte, error) {
	const chunkSize = 32 * 1024
	var data []byte
	buf := make([]byte, chunkSize)
	noProgress := 0
	for {
		n, err := r.Read(buf)
		if n < 0 || n > len(buf) {
			return nil, fmt.Errorf("invalid reader count %d", n)
		}
		if n > 0 {
			data = append(data, buf[:n]...)
			noProgress = 0
		} else if err == nil {
			noProgress++
			if noProgress >= 100 {
				return nil, io.ErrNoProgress
			}
		}
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func copyStrings(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func nativeInterfaceStyle(style gen.InterfaceStyle) (InterfaceStyle, error) {
	switch style {
	case gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED:
		return InterfaceStyleUnspecified, nil
	case gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE:
		return InterfaceStylePackageSurface, nil
	default:
		return InterfaceStyleUnspecified, &UnknownInterfaceStyleError{Value: int32(style)}
	}
}

// validate enforces manifest shape and membership invariants before checking.
func validate(m Manifest) error {
	if m.Name == "" {
		return ErrEmptyName
	}

	switch m.InterfaceStyle {
	case InterfaceStyleUnspecified:
		if len(m.InterfaceFiles) == 0 {
			return ErrEmptyInterfaceFiles
		}
	case InterfaceStylePackageSurface:
		if len(m.Members) == 0 {
			return ErrPackageSurfaceRequiresMembers
		}
		if len(m.InterfaceFiles) > 0 {
			return ErrPackageSurfaceInterfaceFiles
		}
	default:
		return &UnknownInterfaceStyleError{Value: int32(m.InterfaceStyle)}
	}

	seenFiles := make(map[string]bool)
	for _, file := range m.InterfaceFiles {
		if seenFiles[file] {
			return &DuplicateDeclarationError{Kind: "interface file", Value: file}
		}
		seenFiles[file] = true
	}

	seenDeps := make(map[string]bool)
	for _, dep := range m.ComponentDependencies {
		if seenDeps[dep.Name] {
			return &DuplicateDeclarationError{Kind: "component dependency", Value: dep.Name}
		}
		seenDeps[dep.Name] = true
	}

	seenMembers := make(map[string]bool)
	for _, member := range m.Members {
		if seenMembers[member] {
			return &DuplicateDeclarationError{Kind: "member", Value: member}
		}
		seenMembers[member] = true

		if member == "" {
			return &InvalidMemberError{Member: member, Reason: "import path cannot be empty"}
		}
		if strings.ContainsAny(member, "*?[]\\") {
			return &InvalidMemberError{Member: member, Reason: "members must be literal import paths; glob patterns are not accepted"}
		}
	}

	seenAuth := make(map[string]bool)
	for _, auth := range m.DeclaredAuthority {
		if seenAuth[auth] {
			return &DuplicateDeclarationError{Kind: "declared authority", Value: auth}
		}
		seenAuth[auth] = true
		if !schema.KnownCapabilities[auth] {
			return &UnknownCapabilityError{Capability: auth}
		}
	}

	return nil
}
