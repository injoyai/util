package slice

import (
	"math"
	"slices"
	"testing"
)

func TestBoundaries(t *testing.T) {
	s := []int{1, 2, 3, 4}
	if got, ok := Get(s, -1); got != 4 || !ok {
		t.Fatal(got, ok)
	}
	if _, ok := Get(s, math.MinInt); ok {
		t.Fatal("overflowed index")
	}
	for _, tc := range []struct {
		size, offset int
		want         []int
	}{{2, -10, []int{1, 2}}, {math.MaxInt, 2, []int{3, 4}}, {1, math.MaxInt, nil}, {-1, 0, nil}} {
		if got := Limit(s, tc.size, tc.offset); !slices.Equal(got, tc.want) {
			t.Fatal(got)
		}
	}
	got := Common([]int{3, 1, 3, 2}, []int{2, 3}, []int{3, 4})
	if !slices.Equal(got, []int{3}) {
		t.Fatal(got)
	}
	original := []int{2, 2, 1}
	clone := Common(original)
	clone[0] = 9
	if original[0] != 2 {
		t.Fatal("shared storage")
	}
}
