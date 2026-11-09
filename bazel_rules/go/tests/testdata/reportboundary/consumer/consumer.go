package consumer

import (
	"example.com/reportboundary/manual"

	"example.com/aspect/shared"
)

func Values() string {
	return shared.Tag("") + manual.Value()
}
