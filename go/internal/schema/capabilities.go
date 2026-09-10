// Package schema holds the vocabulary shared by every consumer of the
// persisted archcontracts schemas, alongside the generated protobuf types in
// the schema/gen package.
//
// Component Contract (FR10):
//   - What it does: Declares the known Capslock capability taxonomy that
//     validates declared-authority entries in component manifests and persisted
//     artifacts.
//   - What it requires: Nothing; the taxonomy is a fixed value set.
//   - What it provides: KnownCapabilities, the single source of truth for the
//     capability names accepted by the manifest model and the artifact-I/O
//     shell, both of which consume it through the schema component dependency.
//   - Ambient Authority: None; this package is pure data.
package schema

// KnownCapabilities is the set of known Capslock capabilities. It previously
// lived in the manifest package; ownership moved to the schema component so
// the manifest model and the artifact-I/O shell consume one shared taxonomy
// through their declared schema component dependency instead of manifest
// exporting a set the shell cannot reach without an impossible component
// edge (the protobuf runtime is owned by one package-surface component, and
// the checker's member-overlap rule forbids dependency-related components from
// sharing those foreign members).
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
