package consumer

import (
	"example.com/aspect/shared"
	"example.com/reportboundary/manual"
)

func Values() string {
	return shared.Tag("") + manual.Value()
}
