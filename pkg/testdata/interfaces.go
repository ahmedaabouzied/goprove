package testdata

// CallComputerWithFive calls Compute(5) through the interface.
// CHA resolves to SafeComputer.Compute with arg [5,5].
// Inside, x != 0 is true, so division is safe. Return is [20,20].
func CallComputerWithFive(c Computer) int {
	return c.Compute(5) // safe — concrete arg 5 propagated
}

// DivByDualZero divides by an interface where both implementations return 0.
// CHA joins [0,0] and [0,0] → [0,0]. Proven division by zero.
func DivByDualZero(v DualZero) int {
	return 100 / v.Val() // want "division by zero"
}

// DivByEmbeddedIface divides by a method from an embedded interface.
// CHA should resolve through the embedding. Return is [7,7]. Safe.
func DivByEmbeddedIface(e ExtendedIface) int {
	return 100 / e.BaseVal() // safe — implementation returns 7
}

// DivByMixedImpl divides by an interface with mixed returns.
// CHA joins [5,5] and [0,0] → [0,5]. Contains zero. Warning.
func DivByMixedImpl(v MixedValuer) int {
	return 100 / v.Result() // want "possible division by zero"
}

// DivByMultiNonzero divides by an interface with two implementations.
// CHA joins [5,5] and [7,7] → [5,7]. Does not contain zero. Safe.
func DivByMultiNonzero(v MultiValuer) int {
	return 100 / v.Amount() // safe — both implementations return nonzero
}

// DivByPhantom calls an interface method with no known implementations.
// CHA finds no callees → resolveCallees returns nil → result is Top → warning.
func DivByPhantom(p Phantom) int {
	return 100 / p.Ghost() // want "possible division by zero"
}

// DivByPtrReceiver divides by an interface implemented with pointer receiver.
// CHA should still resolve this. Return is [42,42]. Safe.
func DivByPtrReceiver(v PtrValuer) int {
	return 100 / v.PtrVal() // safe — pointer receiver returns 42
}

// DivBySingleImpl divides by a single interface implementation that returns 10.
// CHA should resolve the interface call to ConstTen.Value → [10,10].
// Division should be proven safe.
func DivBySingleImpl(v SingleValuer) int {
	return 100 / v.Value() // safe — only implementation returns 10
}

// DivBySingleZeroImpl divides by an interface call that always returns 0.
// CHA resolves to AlwaysZero.Get → [0,0]. Proven division by zero.
func DivBySingleZeroImpl(v ZeroValuer) int {
	return 100 / v.Get() // want "division by zero"
}

// DivByTriMixed divides by interface with three implementations.
// CHA joins [3,3], [0,0], [-2,-2] → [-2,3]. Contains zero. Warning.
func DivByTriMixed(v TriValuer) int {
	return 100 / v.Tri() // want "possible division by zero"
}

type AlwaysZero struct{}

func (z AlwaysZero) Get() int { return 0 }

// --- Embedded interface ---
type BaseIface interface {
	BaseVal() int
}

// --- Interface with param propagation ---
type Computer interface {
	Compute(x int) int
}

type ConstTen struct{}

func (c ConstTen) Value() int { return 10 }

// --- Two implementations, both zero ---
type DualZero interface {
	Val() int
}

type EmbedImpl struct{}

func (e EmbedImpl) BaseVal() int { return 7 }

func (e EmbedImpl) Extra() int { return 3 }

type ExtendedIface interface {
	BaseIface
	Extra() int
}

type FiveVal struct{}

func (f FiveVal) Amount() int { return 5 }

// --- Two implementations, one zero one nonzero ---
type MixedValuer interface {
	Result() int
}

// --- Two implementations, both nonzero ---
type MultiValuer interface {
	Amount() int
}

// --- No implementations (empty interface method set) ---
type Phantom interface {
	Ghost() int
}

type PtrImpl struct{ x int }

func (p *PtrImpl) PtrVal() int { return 42 }

// --- Pointer receiver ---
type PtrValuer interface {
	PtrVal() int
}

type SafeComputer struct{}

func (s SafeComputer) Compute(x int) int {
	if x != 0 {
		return 100 / x
	}
	return 1
}

type SafeResult struct{}

func (s SafeResult) Result() int { return 5 }

type SevenVal struct{}

func (s SevenVal) Amount() int { return 7 }

// ---------------------------------------------------------------------------
// Interface dispatch testdata for CHA resolver tests
// ---------------------------------------------------------------------------
// --- Single implementation, nonzero return ---
type SingleValuer interface {
	Value() int
}

type TriA struct{}

func (a TriA) Tri() int { return 3 }

type TriB struct{}

func (b TriB) Tri() int { return 0 }

type TriC struct{}

func (c TriC) Tri() int { return -2 }

// --- Three implementations with mixed returns ---
type TriValuer interface {
	Tri() int
}

type UnsafeResult struct{}

func (u UnsafeResult) Result() int { return 0 }

type ZeroA struct{}

func (z ZeroA) Val() int { return 0 }

type ZeroB struct{}

func (z ZeroB) Val() int { return 0 }

// --- Single implementation, zero return ---
type ZeroValuer interface {
	Get() int
}
