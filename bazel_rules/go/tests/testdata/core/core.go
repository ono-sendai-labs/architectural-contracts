package core

import (
	"example.com/aspect/lowlevel"
	"example.com/aspect/shared"
)

func Run(s string) string { return shared.Tag(lowlevel.Norm(s)) }
