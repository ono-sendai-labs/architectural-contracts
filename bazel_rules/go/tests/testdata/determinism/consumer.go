package consumer

import (
	"os"

	"example.com/arcc/determinism/dependency"
	"example.com/arcc/determinism/unknown"
)

func First() string {
	data, _ := os.ReadFile("first")
	return string(data) + dependency.Alpha() + unknown.Value()
}

func Second() string {
	data, _ := os.ReadFile("second")
	return string(data) + dependency.Beta()
}

func Third() string {
	data, _ := os.ReadFile("third")
	return string(data) + dependency.Gamma()
}
