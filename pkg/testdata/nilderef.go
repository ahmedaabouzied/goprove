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
