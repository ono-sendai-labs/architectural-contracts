// Package keyforms exercises the Go/SSA constructs whose Capslock key form the
// design's normalization contract (review A4) and init handling (review A2)
// depend on: methods with both receiver forms, a type split across files, an
// exported interface + concrete impl, promoted (embedded) methods, and a generic
// function instantiation. The spike dumps ssa.Function.String() for this package
// to confirm the exact emitted key strings.
package keyforms

// Widget is a concrete type declared here (types.go); its methods live in
// methods.go — the "type in one interface file, methods in another" case that
// drives the FR4 well-formedness rule and the boundary-symbol key set.
type Widget struct {
	name string
}

// Shape is an exported interface type. Circle (methods.go) implements it; the
// FR5 boundary rule treats a concrete impl of an interface-file interface type
// as part of the contract.
type Shape interface {
	Area() float64
}

// Boxed embeds Widget, so it promotes Widget's methods — the promoted-method
// wrapper key form the design must match.
type Boxed struct {
	Widget
}
