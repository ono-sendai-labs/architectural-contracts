package manifest

import (
	"fmt"
	"io"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest/gen"
	"google.golang.org/protobuf/encoding/prototext"
)

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
	return nil
}
