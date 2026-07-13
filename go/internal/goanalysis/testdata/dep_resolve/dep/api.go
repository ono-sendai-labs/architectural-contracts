package dep

import "fmt"

func Hello() {
	fmt.Println("hello")
}

func (b *Box[T]) Get() T {
	return b.Val
}

func init() {
	// explicit init
}
