package core

import "example.com/aspect/extradep"

func extraHelper() string { return extradep.Extra() }
