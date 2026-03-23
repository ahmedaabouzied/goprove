package nilcheck

// Nil analyzer only tests.

func derefNil() int {
	var p *int
	return *p // want "nil dereference of nil pointer — value is always nil"
}

func derefParam(p *int) int {
	return *p // want "possible nil dereference of p — add a nil check before use"
}

func derefNew() int {
	p := new(int)
	return *p // safe — no diagnostic expected
}

func earlyReturn(p *int) int {
	if p == nil {
		return 0
	}
	return *p // safe — no diagnostic expected
}
