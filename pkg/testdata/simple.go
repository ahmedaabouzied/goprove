package testdata

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}

// Constant returns a constant value.
func Constant() int {
	return 42
}

// LocalVar uses a local variable.
func LocalVar(x int) int {
	y := x + 10
	z := y * 2
	return z
}

// Multiply returns the product of two integers.
func Multiply(a, b int) int {
	return a * b
}
