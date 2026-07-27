package consumer

import "example.com/infra/runtime"

func ConsumeRuntime() ([]byte, error) {
	return runtime.InjectedReadFile("test.txt")
}
