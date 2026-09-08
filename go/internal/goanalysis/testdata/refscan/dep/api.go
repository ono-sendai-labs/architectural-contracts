package dep

// api.go is the interface-file analog: the declared interface surface of the
// dependency fixture. NewGreeter hands out the unexported implementation, so
// member code can only dispatch through the declared interface type.

type Greeter interface {
	Greet() string
}

func Hello() string {
	return "hello"
}

func NewGreeter() Greeter {
	return greeterImpl{}
}
