package a

// Type declared in types.go with method elsewhere
func (g *GreeterImpl) SayHello() string {
	return "Hello " + g.Greeting
}

// Generic function
func Identity[T any](v T) T {
	return v
}
