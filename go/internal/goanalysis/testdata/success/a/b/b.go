package b

import (
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/testdata/success/a"
)

func Greet() {
	a.Hello()
}

// Test dynamic dispatch
func CallGreet(greeter a.Greeter) string {
	return greeter.Greet()
}

func TriggerDynamicDispatch() string {
	impl := a.GreeterImpl{Greeting: "Hi"}
	return CallGreet(impl)
}

// Test generic normalization
func TriggerGenericFunc() string {
	_ = a.Identity[func()](func() {})
	return a.Identity[string]("generic")
}

func TriggerGenericMethods() int {
	box := &a.Box[int]{Val: 42}
	return box.Get() + box.GetVal()
}

// Test higher-order calls
func callback(s string) int {
	return len(s)
}

func TriggerHigherOrder() int {
	return a.CallWithFunc(callback)
}

func stringCallback(s string) string {
	return s + "!"
}

func TriggerHigherOrderNamed() string {
	return a.ProcessString(stringCallback, "hello")
}
