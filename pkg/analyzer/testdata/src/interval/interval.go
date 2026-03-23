package interval

// Interval analyzer only tests.

func divByZeroLiteral(x int) int {
	zero := 0
	return x / zero // want "division by zero"
}

func divByParam(x, y int) int {
	return x / y // want "possible division by zero"
}

func divByConst(x int) int {
	return x / 10 // safe — no diagnostic expected
}

func safeDivGuarded(x, y int) int {
	if y != 0 {
		return x / y
	}
	return 0
}
