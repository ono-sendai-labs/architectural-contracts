package api

import (
	"example.com/aspect/core"
	"example.com/aspect/shared"
)

func Greet(s string) string { return shared.Tag(core.Run(s)) }
