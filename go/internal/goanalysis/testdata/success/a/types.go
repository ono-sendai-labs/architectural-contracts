package a

// Grouped declarations
const (
	ExportedConst1  = "C1"
	ExportedConst2  = "C2"
	unexportedConst = "C3"
)

var (
	ExportedVar1  = "V1"
	ExportedVar2  = "V2"
	unexportedVar = "V3"
)

// Exported interface plus concrete implementation
type Greeter interface {
	Greet() string
}

type GreeterImpl struct {
	Greeting string
}

// Concrete implementation method (value receiver)
func (g GreeterImpl) Greet() string {
	return g.Greeting
}

// Embedded structure (promoted method behavior)
type Base struct {
	Value int
}

func (b *Base) GetValue() int {
	return b.Value
}

type Wrapper struct {
	Base // embedding Base, which promotes GetValue()
	Name string
}

// Generic Type
type Box[T any] struct {
	Val T
}

// Method on generic type with pointer receiver
func (b *Box[T]) Get() T {
	return b.Val
}

// Method on generic type with value receiver
func (b Box[T]) GetVal() T {
	return b.Val
}
