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

// InvalidMemberError reports a member that is empty or is not a valid import-path pattern.
type InvalidMemberError struct {
	Member string
	Reason string
}

func (e *InvalidMemberError) Error() string {
	return fmt.Sprintf("invalid member %q: %s", e.Member, e.Reason)
}

// MemberAbsorbedDependencyError reports a member declared again as an absorbed dependency.
type MemberAbsorbedDependencyError struct {
	ImportPath string
}

func (e *MemberAbsorbedDependencyError) Error() string {
	return fmt.Sprintf("import path %q is both a member and an absorbed dependency", e.ImportPath)
}

// DuplicateDeclarationError represents a duplicate declaration error.
type DuplicateDeclarationError struct {
	Kind  string // "interface file", "component dependency", "absorbed dependency", "member", "declared authority"
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
	AbsorbedDependencies   []AbsorbedDependency
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

// AbsorbedDependency represents an implementation-detail dependency (e.g. internal or third-party package).
type AbsorbedDependency struct {
	ImportPath string
	Reason     *string
}

// Parse reads textproto content from the provided reader, unmarshals it into the
// generated message, and maps it onto the native model. It is ambient-authority-free.
func Parse(r io.Reader) (Manifest, error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading manifest: %w", err)
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

	if pbAbsDeps := pbComponent.GetAbsorbedDependencies(); len(pbAbsDeps) > 0 {
		m.AbsorbedDependencies = make([]AbsorbedDependency, len(pbAbsDeps))
		for i, pbAbsDep := range pbAbsDeps {
			var reasonPtr *string
			if pbAbsDep.Reason != nil {
				val := *pbAbsDep.Reason
				reasonPtr = &val
			}
			m.AbsorbedDependencies[i] = AbsorbedDependency{
				ImportPath: pbAbsDep.GetImportPath(),
				Reason:     reasonPtr,
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

// validate is a seam for task-03 to add syntactic validation rules.
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

	seenAbsDeps := make(map[string]bool)
	for _, absDep := range m.AbsorbedDependencies {
		if seenAbsDeps[absDep.ImportPath] {
			return &DuplicateDeclarationError{Kind: "absorbed dependency", Value: absDep.ImportPath}
		}
		seenAbsDeps[absDep.ImportPath] = true
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
		if seenAbsDeps[member] {
			return &MemberAbsorbedDependencyError{ImportPath: member}
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
