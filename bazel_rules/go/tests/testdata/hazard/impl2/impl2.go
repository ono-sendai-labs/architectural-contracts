package impl2

import "example.com/hazard/impl3"

func Call() string {
	return "impl2 -> " + impl3.Call()
}
