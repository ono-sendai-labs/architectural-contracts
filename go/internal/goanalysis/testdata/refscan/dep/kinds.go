package dep

// kinds.go exercises every design reference kind (design fixture 5).

type Base struct {
	Name   string
	hidden int
}

type Outer struct {
	Base
	Own string
}

const ExportedConst = "const_val"

var ExportedVar = "var_val"

type Plain int

type Alias = Base

type Box[T any] struct {
	Val T
}

func (b Box[T]) Get() T {
	return b.Val
}

type Ptr struct {
	n int
}

func (p *Ptr) Touch() {
	p.n++
}

func Free() int {
	return 42
}
