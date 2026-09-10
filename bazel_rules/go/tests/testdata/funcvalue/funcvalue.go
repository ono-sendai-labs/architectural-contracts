// Package funcvalue exposes FILES ambient authority through a function *value*
// rather than a directly-called exported function. The authority is reachable
// only by invoking the exported variable Open, whose dynamic value is a closure
// that calls os.Open. The component declares no authority, so its check must
// fail.
//
// This is the subtler companion to the `violation` fixture: `violation` uses
// os.Open inside an exported function body (a direct call), whereas here the
// typed reference analysis has to retain an exported function *value* and its
// underlying authority. A scan that only inspected direct calls from exported
// functions would miss it — the negative control for that class.
package funcvalue

import "os"

// Open is an exported function value carrying FILES authority: calling it opens
// a file. Exposing it hands a consumer the authority without the component ever
// naming it in a declared_authority.
var Open = func(name string) (*os.File, error) {
	return os.Open(name)
}
