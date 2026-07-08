package keyforms

// Name is a value-receiver method (key form: func (T).M).
func (w Widget) Name() string { return w.name }

// SetName is a pointer-receiver method (key form: func (*T).M).
func (w *Widget) SetName(n string) { w.name = n }

// Circle is a concrete implementation of the Shape interface.
type Circle struct {
	R float64
}

// Area implements Shape with a value receiver.
func (c Circle) Area() float64 { return 3.14159 * c.R * c.R }

// MapInts is a generic function; its instantiation key must normalize
// (bracket-stripping) back to the generic origin.
func MapInts[T any](xs []T, f func(T) int) []int {
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// Demo forces a concrete instantiation of MapInts and exercises the methods and
// interface dispatch so SSA materializes instantiations and interface wrappers.
func Demo() (string, float64, []int) {
	w := &Widget{}
	w.SetName("hi")
	var s Shape = Circle{R: 2}
	b := Boxed{Widget{name: "boxed"}}
	nums := MapInts([]string{"a", "bb"}, func(v string) int { return len(v) })
	return b.Name(), s.Area(), nums
}
