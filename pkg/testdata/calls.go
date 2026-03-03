package testdata

// --- Direct calls: simple chains ---

// Double returns x * 2.
func Double(x int) int {
	return x * 2
}

// Quadruple calls Double twice: Double(Double(x)).
func Quadruple(x int) int {
	return Double(Double(x))
}

// --- Call chain affecting division safety ---

// Decrement returns x - 1.
func Decrement(x int) int {
	return x - 1
}

// DivByDecrement divides by Decrement(1), which is 0.
// With interprocedural analysis, this should be a proven division by zero.
// Without it, the analyzer sees only Top for the call result.
func DivByDecrement() int {
	d := Decrement(1)
	return 100 / d // want "division by zero" (interprocedural)
}

// --- Safe call chain ---

// AddOne returns x + 1.
func AddOne(x int) int {
	return x + 1
}

// DivBySafeCall divides by AddOne(0), which is 1.
// With interprocedural analysis, this should be proven safe.
func DivBySafeCall() int {
	d := AddOne(0)
	return 100 / d // safe — d is 1 (interprocedural)
}

// --- Multi-level call chain ---

// Identity returns its argument unchanged.
func Identity(x int) int {
	return x
}

// WrapIdentity calls Identity.
func WrapIdentity(x int) int {
	return Identity(x)
}

// DeepChain calls WrapIdentity, which calls Identity.
// Tests that summaries propagate through multiple call levels.
func DeepChain(x int) int {
	return WrapIdentity(x)
}

// --- Multiple callers of the same function ---

// Square returns x * x.
func Square(x int) int {
	return x * x
}

// SumOfSquares calls Square twice with different arguments.
func SumOfSquares(a, b int) int {
	return Square(a) + Square(b)
}

// --- Call result used in a branch ---

// IsPositive returns 1 if x > 0, else 0.
func IsPositive(x int) int {
	if x > 0 {
		return 1
	}
	return 0
}

// DivByIsPositive divides by IsPositive(x).
// Without knowing x, the result could be 0 or 1 — should warn.
func DivByIsPositive(x int) int {
	d := IsPositive(x)
	return 100 / d // want "possible division by zero"
}

// DivByIsPositiveKnown divides by IsPositive(5).
// With interprocedural analysis, d is 1 — should be safe.
func DivByIsPositiveKnown() int {
	d := IsPositive(5)
	return 100 / d // safe — d is 1 (interprocedural)
}

// --- Function returning a constant ---

// AlwaysTen always returns 10.
func AlwaysTen() int {
	return 10
}

// DivByAlwaysTen divides by AlwaysTen(), which is always 10.
func DivByAlwaysTen(x int) int {
	return x / AlwaysTen() // safe — divisor is 10
}

// --- Multiple return paths ---

// AbsOrZero returns |x| if x != 0, else 0.
func AbsOrZero(x int) int {
	if x > 0 {
		return x
	}
	if x < 0 {
		return -x
	}
	return 0
}

// DivByAbsOrZero divides by AbsOrZero, which can return 0.
func DivByAbsOrZero(x int) int {
	d := AbsOrZero(x)
	return 100 / d // want "possible division by zero"
}
