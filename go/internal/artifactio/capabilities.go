package artifactio

// knownCapabilities is artifactio's local spelling of the known Capslock
// capability taxonomy. The artifact-I/O shell consumes the taxonomy value set
// without a component dependency on the manifest component, so the set is
// replicated here; capabilitySetDriftTest guards the two spellings against
// drift. Keys and values mirror manifest.KnownCapabilities.
var knownCapabilities = map[string]bool{
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
