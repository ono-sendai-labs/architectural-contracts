package unsafe_member

import "unsafe"

func Use(items []int, p *int, text string) []int {
	items = append(items, 1)
	_ = len(items)
	_ = cap(items)
	_ = unsafe.Sizeof(p)
	_ = unsafe.StringData(text)
	return unsafe.Slice(p, 1)
}
