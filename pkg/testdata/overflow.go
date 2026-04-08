package testdata

import "math"

// OverflowAdd overflows int8 bounds.
func OverflowAdd() int8 {
	var x int8 = math.MaxInt8
	return x + 1 // want "proven integer overflow"
}

// OverflowMul multiplies two large values.
func OverflowMul(x int32) int32 {
	return x * x // want "possible integer overflow"
}

// SafeAdd adds small known values.
func SafeAdd() int {
	a := 10
	b := 20
	return a + b // safe — result is 30
}

// SafeSmallArithmetic does arithmetic that stays in bounds.
func SafeSmallArithmetic(x int) int {
	if x > 0 && x < 100 {
		return x * 2 // safe — result is in [2, 198]
	}
	return 0
}

// ShiftOverflow shifts beyond the type width.
func ShiftOverflow(x int32) int32 {
	return x << 40 // want "possible integer overflow"
}

// UnderflowSub underflows int8 bounds.
func UnderflowSub() int8 {
	var x int8 = math.MinInt8
	return x - 1 // want "proven integer overflow"
}
