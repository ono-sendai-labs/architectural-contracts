package manifest

import (
	"errors"
	"fmt"
	"io"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
	"google.golang.org/protobuf/encoding/prototext"
)

var (
	ErrEmptyName           = errors.New("manifest name cannot be empty")
	ErrEmptyInterfaceFiles = errors.New("manifest must declare at least one interface file")
)

// DuplicateDeclarationError represents a duplicate declaration error.
type DuplicateDeclarationError struct {
	Kind  string // "interface file", "component dependency", "absorbed dependency", "declared authority"
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
	Name                  string
	InterfaceFiles        []string
	ComponentDependencies []ComponentDependency
	AbsorbedDependencies  []AbsorbedDependency
	DeclaredAuthority     []string
}

// ComponentDependency represents a dependency on a first-class component.
type ComponentDependency struct {
	Name     string
	Manifest string
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

	m := Manifest{
		Name:              pbComponent.GetName(),
		InterfaceFiles:    pbComponent.GetInterfaceFiles(),
		DeclaredAuthority: pbComponent.GetDeclaredAuthority(),
	}

	if pbDeps := pbComponent.GetComponentDependencies(); len(pbDeps) > 0 {
		m.ComponentDependencies = make([]ComponentDependency, len(pbDeps))
		for i, pbDep := range pbDeps {
			m.ComponentDependencies[i] = ComponentDependency{
				Name:     pbDep.GetName(),
				Manifest: pbDep.GetManifest(),
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

// validate is a seam for task-03 to add syntactic validation rules.
func validate(m Manifest) error {
	if m.Name == "" {
		return ErrEmptyName
	}
	if len(m.InterfaceFiles) == 0 {
		return ErrEmptyInterfaceFiles
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
