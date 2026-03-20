package testdata

// DerefParam dereferences a pointer parameter without checking.
func DerefParam(p *int) int {
	return *p // want "possible nil dereference"
}

// DerefAfterCheck dereferences only after a nil check.
func DerefAfterCheck(p *int) int {
	if p != nil {
		return *p // safe — p is proven non-nil here
	}
	return 0
}

// DerefNew dereferences a freshly allocated pointer.
func DerefNew() int {
	p := new(int)
	*p = 42
	return *p // safe — p was just allocated
}

// DerefNilLiteral assigns nil and dereferences.
func DerefNilLiteral() int {
	var p *int
	return *p // want "proven nil dereference"
}

// FieldAccessOnNil accesses a field on a nil struct pointer.
func FieldAccessOnNil() int {
	type S struct{ X int }
	var s *S
	return s.X // want "proven nil dereference"
}

// MethodCallOnParam calls a method on a possibly-nil receiver.
func MethodCallOnParam(s fmt_Stringer) string {
	return s.String() // want "possible nil dereference"
}

// fmt_Stringer mimics fmt.Stringer to avoid importing fmt in testdata.
type fmt_Stringer interface {
	String() string
}

// MapLookupOk uses the ok pattern for safe map access.
func MapLookupOk(m map[string]*int, key string) int {
	v, ok := m[key]
	if ok && v != nil {
		return *v // safe — v is proven non-nil
	}
	return 0
}

// ---------------------------------------------------------------------------
// Fixtures for transfer function tests
// ---------------------------------------------------------------------------

// AllocNew tests that new(T) produces a non-nil pointer.
func AllocNew() *int {
	p := new(int)
	return p
}

// AllocAddr tests that &x produces a non-nil pointer.
func AllocAddr() *int {
	x := 42
	return &x
}

// MakeSliceFixture tests that make([]T, n) is non-nil.
// Uses a parameter for length to prevent SSA from optimizing away the MakeSlice.
func MakeSliceFixture(n int) []int {
	return make([]int, n)
}

// MakeSliceCapFixture tests that make([]T, n, cap) is non-nil.
// Uses parameters for length and cap to prevent SSA from optimizing away the MakeSlice.
func MakeSliceCapFixture(n, cap int) []byte {
	return make([]byte, n, cap)
}

// MakeMapFixture tests that make(map[K]V) is non-nil.
func MakeMapFixture() map[string]int {
	return make(map[string]int)
}

// MakeMapHintFixture tests that make(map[K]V, hint) is non-nil.
func MakeMapHintFixture() map[string]int {
	return make(map[string]int, 100)
}

// MakeChanFixture tests that make(chan T) is non-nil.
func MakeChanFixture() chan int {
	return make(chan int)
}

// MakeChanBufFixture tests that make(chan T, size) is non-nil.
func MakeChanBufFixture() chan string {
	return make(chan string, 10)
}

// PhiBothNotNil tests a Phi where both branches produce non-nil.
func PhiBothNotNil(cond bool) *int {
	var p *int
	if cond {
		x := 1
		p = &x
	} else {
		y := 2
		p = &y
	}
	return p
}

// PhiOneBranchNil tests a Phi where one branch is nil (default var)
// and the other is non-nil.
func PhiOneBranchNil(cond bool) *int {
	var p *int
	if cond {
		x := 1
		p = &x
	}
	return p
}

// PhiAllNil tests a Phi where all branches are nil.
func PhiAllNil(cond bool) *int {
	var p *int
	if cond {
		p = nil
	}
	return p
}

// ---------------------------------------------------------------------------
// Fixtures for nil branch refinement tests
// ---------------------------------------------------------------------------

// RefineNotNil tests that p != nil narrows the true branch to DefinitelyNotNil.
func RefineNotNil(p *int) int {
	if p != nil {
		return *p // safe — p is proven non-nil
	}
	return 0
}

// RefineEqlNil tests that p == nil narrows the true branch to DefinitelyNil.
func RefineEqlNil(p *int) int {
	if p == nil {
		return 0
	}
	return *p // safe — p is proven non-nil in the else branch
}

// RefineNilOnLeft tests nil == p (nil constant on the left side).
func RefineNilOnLeft(p *int) int {
	if nil == p {
		return 0
	}
	return *p
}

// RefineInterface tests nil check on an interface parameter.
func RefineInterface(s fmt_Stringer) string {
	if s != nil {
		return s.String()
	}
	return ""
}

// RefineSlice tests nil check on a slice parameter.
func RefineSlice(s []int) int {
	if s != nil {
		return s[0]
	}
	return 0
}

// MakeInterfaceFixture wraps a concrete value into an interface.
type MakeIfaceImpl struct{}

func (MakeIfaceImpl) String() string { return "hello" }

func MakeInterfaceFixture() fmt_Stringer {
	return MakeIfaceImpl{}
}

// MakeInterfaceNilPtr wraps a nil pointer into an interface.
// The interface itself is non-nil even though the underlying pointer is nil.
type MakeIfacePtr struct{}

func (*MakeIfacePtr) String() string { return "hi" }

func MakeInterfaceNilPtrFixture() fmt_Stringer {
	var p *MakeIfacePtr
	return p
}
