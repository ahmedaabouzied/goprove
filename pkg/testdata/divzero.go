package testdata

// DivAfterComputation divides by a value that is always zero (x - x).
func DivAfterComputation(x int) int {
	d := x - x
	return 100 / d // want "proven division by zero"
}

// DivByConstant divides by a nonzero constant.
func DivByConstant(x int) int {
	return x / 10 // safe — divisor is 10
}

// DivByParam divides by an unchecked parameter.
func DivByParam(x, y int) int {
	return x / y // want "possible division by zero"
}

// DivByZeroLiteral divides by a variable that is always zero.
func DivByZeroLiteral(x int) int {
	zero := 0
	return x / zero // want "proven division by zero"
}

// DivInLoop divides inside a loop with a decrementing divisor.
func DivInLoop(n int) int {
	total := 0
	for i := n; i >= 0; i-- {
		total += 100 / i // want "possible division by zero"
	}
	return total
}

// DivSafe divides only after checking the divisor is nonzero.
func DivSafe(x, y int) int {
	if y != 0 {
		return x / y // safe — y is proven nonzero here
	}
	return 0
}

// ModByZero uses the remainder operator with a variable that is always zero.
func ModByZero(x int) int {
	zero := 0
	return x % zero // want "proven division by zero"
}
