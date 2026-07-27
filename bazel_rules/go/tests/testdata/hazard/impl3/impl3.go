package impl3

import "example.com/hazard/impl4"

func Call() string {
	return "impl3 -> " + impl4.Call()
}
