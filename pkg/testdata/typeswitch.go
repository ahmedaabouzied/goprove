package testdata

// CommaOkDeref: v, ok pattern with ok check.
func CommaOkDeref(a Animal) string {
	if v, ok := a.(*Dog); ok {
		return v.Name
	}
	return ""
}

// CommaOkEarlyReturn: ok check via early return, then use.
func CommaOkEarlyReturn(a Animal) string {
	v, ok := a.(*Dog)
	if !ok {
		return ""
	}
	return v.Name
}

// --- Unsafe patterns (findings expected) ---
// CommaOkNoCheck: v, ok without checking ok — should warn.
func CommaOkNoCheck(a Animal) string {
	v, _ := a.(*Dog)
	return v.Name
}

// TypeSwitchDefault: default case uses original interface — should warn.
func TypeSwitchDefault(a Animal) string {
	switch a.(type) {
	case *Dog:
		return "dog"
	default:
		return a.Sound()
	}
}

// --- Safe patterns (no findings expected) ---
// TypeSwitchDeref: in case *Dog, v is proven non-nil.
func TypeSwitchDeref(a Animal) string {
	switch v := a.(type) {
	case *Dog:
		return v.Name
	}
	return ""
}

// TypeSwitchMultiCase: multiple cases, each proven non-nil.
func TypeSwitchMultiCase(a Animal) string {
	switch v := a.(type) {
	case *Dog:
		return v.Name
	case *Cat:
		_ = v.Lives
		return "cat"
	}
	return ""
}

type Animal interface {
	Sound() string
}

type Cat struct {
	Lives int
}

func (c *Cat) Sound() string { return "meow" }

type Dog struct {
	Name string
}

func (d *Dog) Sound() string { return "woof" }
