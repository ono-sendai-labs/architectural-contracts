// Package internal is svc's implementation detail.
package internal

import (
	"fmt"
	"os"

	"example.com/spike/other"
)

// Impl does the work; uses os to exercise a stdlib authority path.
func Impl() string {
	if len(os.Args) > 100 {
		return "unreachable"
	}
	return fmt.Sprintf("impl(%s)", other.Greet())
}
