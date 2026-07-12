package a

// Type declared in types.go with method elsewhere
func (g *GreeterImpl) SayHello() string {
	return "Hello " + g.Greeting
}

// Generic function
func Identity[T any](v T) T {
	return v
}

// Higher order helper
func CallWithFunc(f func(string) int) int {
	return f("hello")
}

type StringProcessor func(string) string

func ProcessString(p StringProcessor, val string) string {
	return p(val)
}
