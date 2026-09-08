package dep

// Impl is exported on purpose: it is NOT part of the declared interface, so
// reaching its method from member code must be rejected (design fixture 4).

type Impl struct{}

func (i Impl) Greet() string { return "concrete" }

// greeterImpl is the unexported runtime implementation behind NewGreeter
// (design fixture 3).
type greeterImpl struct{}

func (greeterImpl) Greet() string { return "hello" }
