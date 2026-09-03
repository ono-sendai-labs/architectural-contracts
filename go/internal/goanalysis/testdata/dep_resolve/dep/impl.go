package dep

type GreeterImpl struct{}

func (g *GreeterImpl) Greet() string {
	return "hello"
}

// Exported but not in interface_files, so it is architecture-private
func PrivateFunc() {}

// statusErr implements the universal stdlib error interface without being
// named in any interface file.
type statusErr struct{ code int }

func (statusErr) Error() string { return "status" }

// Code is a second method on the error-implementing type; it must stay outside
// the derived dependency surface.
func (statusErr) Code() int { return 0 }
