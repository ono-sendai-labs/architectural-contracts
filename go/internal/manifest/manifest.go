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
	"path"
	"regexp"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
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
	return fmt.Sprintf(
		"unsupported manifest field %q: this field was removed; migrate to explicit members or component_dependencies",
		e.Field,
	)
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
}

// checkRemovedFields rejects textproto content naming fields that no longer
// exist in the schema. It runs before unmarshaling because the protobuf
// unmarshaler silently ignores unknown fields, which would drop a stale
// declaration without a trace.
func checkRemovedFields(text []byte) error {
	for _, rf := range removedFields {
		if rf.pattern.Match(text) {
			return &RemovedFieldError{Field: rf.field}
		}
	}
	return nil
}

// InvalidMemberError reports a member that is empty or is not a valid import-path pattern.
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

// KnownCapabilities is the set of known Capslock capabilities.
var KnownCapabilities = map[string]bool{
	"FILES":               true,
	"NETWORK":             true,
	"READ_SYSTEM_STATE":   true,
	"MODIFY_SYSTEM_STATE": true,
	"OPERATING_SYSTEM":    true,
	"SYSTEM_CALLS":        true,
	"EXEC":                true,
	"RUNTIME":             true,
	"ARBITRARY_EXECUTION": true,
	"CGO":                 true,
	"UNSAFE_POINTER":      true,
	"REFLECT":             true,
	"UNANALYZED":          true,
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
	Name                   string
	InterfaceFiles         []string
	ComponentDependencies  []ComponentDependency
	DeclaredAuthority      []string
	Members                []string
	InterfaceStyle         InterfaceStyle
	OwnCheckRuns           bool
	CertificationReference string
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
	bytes, err := io.ReadAll(r)
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

	m := Manifest{
		Name:                   pbComponent.GetName(),
		InterfaceFiles:         copyStrings(pbComponent.GetInterfaceFiles()),
		DeclaredAuthority:      copyStrings(pbComponent.GetDeclaredAuthority()),
		Members:                copyStrings(pbComponent.GetMembers()),
		InterfaceStyle:         interfaceStyle,
		OwnCheckRuns:           pbComponent.GetOwnCheckRuns(),
		CertificationReference: pbComponent.GetCertificationReference(),
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
		if _, err := path.Match(member, ""); err != nil {
			return &InvalidMemberError{Member: member, Reason: fmt.Sprintf("malformed import-path pattern: %v", err)}
		}
		if m.InterfaceStyle == InterfaceStyleUnspecified && strings.ContainsAny(member, "*?[]\\") {
			return &InvalidMemberError{Member: member, Reason: "declared-style members must be literal import paths"}
		}
	}

	seenAuth := make(map[string]bool)
	for _, auth := range m.DeclaredAuthority {
		if seenAuth[auth] {
			return &DuplicateDeclarationError{Kind: "declared authority", Value: auth}
		}
		seenAuth[auth] = true
		if !KnownCapabilities[auth] {
			return &UnknownCapabilityError{Capability: auth}
		}
	}

	return nil
}
