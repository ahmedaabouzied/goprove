package testdata

// ---------------------------------------------------------------------------
// Address model test fixtures
// These test patterns that require tracking nil state per memory address
// rather than per SSA register.
// ---------------------------------------------------------------------------

// AddrFieldReload: if o.In != nil { o.In.Val } — two loads from same field.
func AddrFieldReload(o *Outer) int {
	if o.In != nil {
		return o.In.Val // safe — o.In was just checked
	}
	return 0
}

// AddrFieldReloadMultiple: multiple accesses after nil check.
func AddrFieldReloadMultiple(o *Outer) int {
	if o.In != nil {
		a := o.In.Val  // safe
		b := o.In.Val  // safe
		return a + b
	}
	return 0
}

// AddrNestedFieldCheck: if o.In != nil { if o.In.Val > 0 { use(o.In.Val) } }
func AddrNestedFieldCheck(o *Outer) int {
	if o.In != nil {
		if o.In.Val > 0 {
			return o.In.Val // safe — o.In checked above
		}
	}
	return 0
}

// AddrGlobalNilCheck: if globalOuter.In != nil { globalOuter.In.Val }
var globalOuter *Outer

func AddrGlobalFieldReload() int {
	if globalOuter != nil {
		if globalOuter.In != nil {
			return globalOuter.In.Val // safe — both checked
		}
	}
	return 0
}

// AddrFieldNotChecked: o.In used without nil check — should warn.
func AddrFieldNotChecked(o *Outer) int {
	return o.In.Val // want "possible nil dereference"
}
