package dep

type Greeter interface {
	Greet() string
}

type Box[T any] struct {
	Val T
}

const ExportedConst = "const_val"

var ExportedVar = "var_val"

type Base struct {
	Value int
}

func (b *Base) GetValue() int {
	return b.Value
}
