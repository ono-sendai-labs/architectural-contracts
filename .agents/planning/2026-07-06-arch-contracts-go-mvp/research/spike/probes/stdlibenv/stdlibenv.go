// Package stdlibenv exercises the standard-library entry points the pure core
// and the examples want to use, one exported function each, so the spike can
// measure the "strict-safe stdlib envelope": which of these come out with no
// Capslock capability (usable by an authority-free component) and which do not.
package stdlibenv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
)

// UseFmt exercises fmt formatting (no authority expected).
func UseFmt(x int) string { return fmt.Sprintf("value=%d", x) }

// intSlice is a concrete sort.Interface so UseSortSort can call sort.Sort
// without reaching reflect.
type intSlice []int

func (s intSlice) Len() int           { return len(s) }
func (s intSlice) Less(i, j int) bool { return s[i] < s[j] }
func (s intSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// UseSortSort sorts via sort.Sort with a concrete sort.Interface.
func UseSortSort(xs []int) { sort.Sort(intSlice(xs)) }

// UseSortSlice sorts via sort.Slice (reaches reflect.Swapper — the envelope
// question of interest; review B8).
func UseSortSlice(xs []int) { sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] }) }

// UseSortInts sorts via the monomorphic sort.Ints helper.
func UseSortInts(xs []int) { sort.Ints(xs) }

// UseSlicesSortFunc sorts via the generic slices.SortFunc.
func UseSlicesSortFunc(xs []int) {
	slices.SortFunc(xs, func(a, b int) int { return a - b })
}

// UseStrconv exercises strconv (no authority expected).
func UseStrconv(s string) (int, string) {
	n, _ := strconv.Atoi(s)
	return n, strconv.Itoa(n + 1)
}

// UseErrors exercises errors/fmt.Errorf (no authority expected).
func UseErrors() error {
	base := errors.New("base")
	wrapped := fmt.Errorf("context: %w", base)
	if errors.Is(wrapped, base) {
		return wrapped
	}
	return nil
}

// UseIoReadAll reads an in-memory bytes.Reader via io.ReadAll — the exact shape
// manifest.Parse(io.Reader) uses. Expected authority-free because only a
// bytes.Reader flows in.
func UseIoReadAll(data []byte) ([]byte, error) {
	var r io.Reader = bytes.NewReader(data)
	return io.ReadAll(r)
}
