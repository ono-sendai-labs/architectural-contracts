package dep

type GreeterImpl struct{}

func (g *GreeterImpl) Greet() string {
	return "hello"
}

// Exported but not in interface_files, so it is architecture-private
func PrivateFunc() {}
