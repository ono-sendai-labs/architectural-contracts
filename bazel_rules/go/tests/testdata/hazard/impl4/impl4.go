package impl4

import (
	"example.com/hazard/util1"
	"example.com/hazard/util2"
	"example.com/hazard/util3"
)

func Call() string {
	return "impl4 -> (" + util1.Call() + ", " + util2.Call() + ", " + util3.Call() + ")"
}
