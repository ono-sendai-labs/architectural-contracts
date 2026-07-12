package a

import (
	"fmt"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
)

func Hello() {
	fmt.Println("Hello")
	_ = facts.PackageFacts{}
}
