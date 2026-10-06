package symbol

import "go/types"

const GrammarVersion = grammarVersion

func FromSelection(sel *types.Selection) (SymbolID, error) {
	return fromSelection(sel)
}
