package testdata

// AppendAndIndex appends to a slice and indexes the result.
func AppendAndIndex(s []int, v int) int {
	s = append(s, v)
	return s[len(s)-1] // safe — just appended
}

// IndexAfterCheck indexes after a bounds check.
func IndexAfterCheck(s []int, i int) int {
	if i >= 0 && i < len(s) {
		return s[i] // safe — i is within bounds
	}
	return 0
}

// IndexConstant indexes with a constant on a known-length slice.
func IndexConstant() int {
	s := []int{1, 2, 3, 4, 5}
	return s[2] // safe — index 2, length 5
}

// IndexDirect indexes a slice with an unchecked value.
func IndexDirect(s []int, i int) int {
	return s[i] // want "possible index out of bounds"
}

// IndexOutOfBounds uses an index that exceeds the slice length.
func IndexOutOfBounds() int {
	s := make([]int, 5)
	return s[10] // want "proven index out of bounds"
}

// RangeLoop iterates with range — always safe.
func RangeLoop(s []int) int {
	total := 0
	for i := range s {
		total += s[i] // safe — range guarantees i is in bounds
	}
	return total
}

// SliceOp performs a slice operation with potentially bad bounds.
func SliceOp(s []int, lo, hi int) []int {
	return s[lo:hi] // want "possible index out of bounds"
}
