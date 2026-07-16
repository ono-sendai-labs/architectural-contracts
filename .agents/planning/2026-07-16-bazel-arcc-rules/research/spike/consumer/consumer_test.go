package consumer_test

import (
	"testing"

	"example.com/spike/svc"
)

// Depends on //svc:svc_component directly — validates provider forwarding.
func TestViaComponent(t *testing.T) {
	if svc.Do() == "" {
		t.Fatal("empty")
	}
}
