package testdata

// Sum returns the sum of integers from 1 to n.
func Sum(n int) int {
	total := 0
	for i := 1; i <= n; i++ {
		total += i
	}
	return total
}

// Countdown counts down from n to 0, returning the number of steps.
func Countdown(n int) int {
	steps := 0
	for n > 0 {
		n--
		steps++
	}
	return steps
}

// SumSlice returns the sum of all elements using a range loop.
func SumSlice(s []int) int {
	total := 0
	for _, v := range s {
		total += v
	}
	return total
}

// Nested has a nested loop structure.
func Nested(n int) int {
	count := 0
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			count++
		}
	}
	return count
}
