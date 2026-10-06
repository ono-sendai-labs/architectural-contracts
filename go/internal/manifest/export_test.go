package manifest

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

var (
	ErrEmptyName           = errEmptyName
	ErrEmptyInterfaceFiles = errEmptyInterfaceFiles
)

type (
	RemovedFieldError         = removedFieldError
	DuplicateDeclarationError = duplicateDeclarationError
	UnknownCapabilityError    = unknownCapabilityError
)

func FromPersistedAnalysisDefeatingPolicy(policy gen.AnalysisDefeatingPolicy) (AnalysisDefeatingPolicy, error) {
	return fromPersistedAnalysisDefeatingPolicy(policy)
}

func ToPersistedAnalysisDefeatingPolicy(policy AnalysisDefeatingPolicy) (gen.AnalysisDefeatingPolicy, error) {
	return toPersistedAnalysisDefeatingPolicy(policy)
}
