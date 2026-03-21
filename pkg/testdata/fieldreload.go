package testdata

type Inner struct {
	Val int
}

type Outer struct {
	In *Inner
}

// CheckFieldReload tests the pattern: if o.In != nil { o.In.Val }
// SSA produces two separate loads of o.In — the nil check refines the
// first but the second is a new SSA value.
func CheckFieldReload(o *Outer) int {
	if o.In != nil {
		return o.In.Val
	}
	return 0
}
