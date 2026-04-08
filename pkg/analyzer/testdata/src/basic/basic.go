package basic

func divByZero(x int) int {
	zero := 0
	return x / zero // want "division by zero"
}

// Combined analyzer test — both nil and interval findings.
func nilDeref() int {
	var p *int
	return *p // want "nil dereference of nil pointer — value is always nil"
}

func safe(x, y int) int {
	if y != 0 {
		return x / y
	}
	return 0
}

func safeNil(p *int) int {
	if p != nil {
		return *p
	}
	return 0
}
