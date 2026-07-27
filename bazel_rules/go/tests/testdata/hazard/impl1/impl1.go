package impl1

import "example.com/hazard/impl2"

func Call() string {
	return "impl1 -> " + impl2.Call()
}
