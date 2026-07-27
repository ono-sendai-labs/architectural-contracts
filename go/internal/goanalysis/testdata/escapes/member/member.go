package member

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/escapes/absorbed"
)

// Fixture 1: struct-literal field escape (detected)
func EscapeInStruct() absorbed.Handlers {
	return absorbed.Handlers{
		Open: absorbed.Save,
	}
}

// Fixture 2: bound method value escape (detected)
func EscapeBoundMethod() func() {
	b := &absorbed.Backend{}
	return b.Read
}

// Fixture 3: plain static call into absorbed code (not an escape, ignored)
func DirectCall() {
	absorbed.Load()
}

// Fixture 4: local indirection (suppressed since call graph reaches it)
func LocalIndirection() {
	f := absorbed.Load
	f()
}

// Fixture 5: member-defined function values (ignored)
func MemberCallback() {
	_ = func() {}
}
