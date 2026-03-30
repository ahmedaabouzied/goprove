package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// False positive regression tests
//
// These tests document known false positives and patterns found by running
// goprove on real-world code (go-redis v9.7.3, net/http, production codebases).
//
// Tests marked "KNOWN FALSE POSITIVE" are expected to produce warnings
// that shouldn't exist. When a fix is implemented, update the assertion.
//
// Tests marked "CORRECTLY HANDLED" verify patterns that work today.
// ===========================================================================

// ---------------------------------------------------------------------------
// Category 1: Multi-return value patterns (error check guarantees non-nil)
// ---------------------------------------------------------------------------

func TestFalsePositive_MultiReturn_ErrorCheckGuaranteesNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func parse(s string) (*int, error) {
			if s == "" {
				return nil, nil
			}
			x := 42
			return &x, nil
		}

		func useParse(s string) int {
			p, err := parse(s)
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useParse")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: parse() can return (nil, nil) — so when
	// err == nil, p can still be nil. This is actually a correct warning!
	// The multi-return pattern is only safe when the function guarantees
	// non-nil on the first return when err is nil.
	// For now, this warning is legitimate.
	require.NotEmpty(t, findings,
		"parse can return (nil, nil) — warning is correct")
}

// ---------------------------------------------------------------------------
// Category 2: Double pointer load after nil check
//
// When *pp is nil-checked, the deref of pp itself triggers a warning
// because pp is a pointer parameter (MaybeNil).
// ---------------------------------------------------------------------------

func TestFalsePositive_DoublePtrLoadWarnsOnOuterDeref(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useDoublePtr(pp **int) int {
			p := *pp
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useDoublePtr")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: *pp dereferences pp (the outer pointer),
	// which is an unchecked parameter. The warning is on pp, not on p.
	// Technically correct — pp could be nil — but in practice callers
	// rarely pass nil **int.
	require.Len(t, findings, 1,
		"pp deref produces a warning — pp is an unchecked param")
	require.Contains(t, findings[0].Message, "possible nil dereference")
}

// ---------------------------------------------------------------------------
// Category 3: Type assertion ok pattern — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_TypeAssertionOkPattern(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		type Doer interface {
			Do()
		}

		type MyDoer struct{}
		func (MyDoer) Do() {}

		func tryDoer(x interface{}) {
			if d, ok := x.(*MyDoer); ok {
				d.Do()
			}
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "tryDoer")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: the analyzer produces no false positive here.
	// The type assertion + ok check works because SSA represents this
	// in a way the nil analyzer can track.
	require.Empty(t, findings,
		"type assertion with ok check should be safe")
}

// ---------------------------------------------------------------------------
// Category 4: Map lookup ok pattern — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_MapLookupOkPattern(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func mapLookup(m map[string]*int, key string) int {
			v, ok := m[key]
			if ok && v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "mapLookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: the v != nil check refines v in the true branch.
	// The && creates sequential blocks and the refinement propagates.
	require.Empty(t, findings,
		"map lookup with ok && v != nil should be safe")
}

// ---------------------------------------------------------------------------
// Category 5: Stdlib return value guarantees — CORRECTLY HANDLED
//
// The interprocedural analysis analyzes stdlib function bodies and
// computes return nil summaries. Functions like time.NewTimer and
// bytes.NewBuffer always return non-nil, and the analyzer proves this.
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_StdlibNewTimer(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "time"

		func useTimer() {
			t := time.NewTimer(time.Second)
			t.Stop()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useTimer")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: interprocedural analysis resolves time.NewTimer
	// return value as DefinitelyNotNil by analyzing the stdlib source.
	require.Empty(t, findings,
		"time.NewTimer always returns non-nil — interprocedural analysis proves this")
}

func TestCorrectlyHandled_StdlibBytesNewBuffer(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "bytes"

		func useBuffer() int {
			b := bytes.NewBuffer(nil)
			return b.Len()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useBuffer")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: bytes.NewBuffer always returns non-nil.
	require.Empty(t, findings,
		"bytes.NewBuffer always returns non-nil — interprocedural analysis proves this")
}

// ---------------------------------------------------------------------------
// Category 6: String indexing — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_StringIndexNotNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func charAt(s string, i int) byte {
			return s[i]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "charAt")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: strings are value types in Go — cannot be nil.
	require.Empty(t, findings,
		"string indexing should not produce nil dereference warnings")
}

// ---------------------------------------------------------------------------
// Category 7: Unsafe pointer operations
// ---------------------------------------------------------------------------

func TestFalsePositive_UnsafePointerConversion(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "unsafe"

		func unsafeConvert(b []byte) string {
			return *(*string)(unsafe.Pointer(&b))
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "unsafeConvert")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// FIXED: *ssa.Convert now propagates source nil state.
	// unsafe.Pointer(&b) → &b is Alloc (DefinitelyNotNil) → Convert preserves it.
	require.Empty(t, findings,
		"unsafe.Pointer(&b) should be non-nil — Convert propagates source state")
}

// ---------------------------------------------------------------------------
// Category 8: Stdlib multi-return with error check (url.Parse pattern)
//
// Cannot use buildSSA for this pattern because struct field access
// (u.Scheme) panics in the inline SSA builder. This pattern is tested
// via the real go-redis codebase instead.
// Documented here for reference:
//
//   u, err := url.Parse(s)
//   if err != nil { return nil, err }
//   return u.Scheme  // u is non-nil when err is nil, but analyzer warns
//
// This is a false positive because url.Parse guarantees non-nil *URL
// when err is nil.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Category 9: Nested nil checks with sequential ifs — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_NestedNilCheckSequential(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func nestedCheck(pp **int) int {
			if pp != nil {
				p := *pp
				if p != nil {
					return *p
				}
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "nestedCheck")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: nested nil checks with sequential ifs work.
	require.Empty(t, findings,
		"nested nil checks with sequential ifs should be safe")
}

// ---------------------------------------------------------------------------
// Category 10: Early return guard — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_EarlyReturnGuard(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func guarded(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "guarded")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"early return guard should prove p non-nil after guard")
}

// ---------------------------------------------------------------------------
// Category 11: Callee return nil checked by caller — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_CalleeReturnNilChecked(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeNil(cond bool) *int {
			if cond {
				return new(int)
			}
			return nil
		}

		func checked() int {
			p := maybeNil(true)
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "checked")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"callee return checked before use should be safe")
}

// ===========================================================================
// Regression tests for NilBottom leak and variadic parameter false positives
//
// These tests cover bugs found by running goprove against a large production
// codebase (Adjust backend). The bugs were:
//
// 1. computeReturnNilStates returned NilBottom for return positions the
//    analysis couldn't observe, which downstream was treated as DefinitelyNil
//    instead of "unknown". Fixed by promoting NilBottom → MaybeNil.
//
// 2. Variadic parameters (e.g., opts ...Option) were classified as
//    DefinitelyNil when all visible callers passed no variadic args.
//    A nil variadic slice is idiomatic Go — range over nil is a no-op.
//    Fixed by capping variadic params at MaybeNil.
// ===========================================================================

// ---------------------------------------------------------------------------
// Category 12: NilBottom leak — external method chain returns
// ---------------------------------------------------------------------------

func TestRegression_NilBottomLeak_MethodChainReturn(t *testing.T) {
	t.Parallel()

	// Simulates the pattern: bitset.MustNew(n).SetAll()
	// SetAll returns the receiver (non-nil), but the analyzer previously
	// failed to track that and let NilBottom leak as DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		type BitSet struct{}

		func NewBitSet() *BitSet {
			return &BitSet{}
		}

		func (b *BitSet) SetAll() *BitSet {
			return b
		}

		func useChain() {
			bs := NewBitSet().SetAll()
			_ = bs
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useChain")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"method chain returning receiver should not be flagged as nil")
}

func TestRegression_NilBottomLeak_ReturnFromCallee(t *testing.T) {
	t.Parallel()

	// Simulates the pattern: states := src.ToStateSliceUpTo(max)
	// where the callee builds and returns a slice.
	ssaPkg := buildSSA(t, `
		package example

		type StateBitSet struct{}

		func (s *StateBitSet) ToSliceUpTo(max int) []int {
			result := make([]int, 0)
			for i := 0; i < max; i++ {
				result = append(result, i)
			}
			return result
		}

		func useSlice(s *StateBitSet) {
			states := s.ToSliceUpTo(10)
			if len(states) == 0 {
				return
			}
			_ = states[0]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useSlice")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Should not flag states as DefinitelyNil — the callee returns
	// a slice built with make+append.
	for _, f := range findings {
		require.NotContains(t, f.Message, "always nil",
			"callee return should not be flagged as DefinitelyNil")
	}
}

// ---------------------------------------------------------------------------
// Category 13: Variadic parameters — nil variadic is safe
// ---------------------------------------------------------------------------

func TestRegression_VariadicParam_RangeOverNilIsSafe(t *testing.T) {
	t.Parallel()

	// Simulates: func Process(opts ...func(int)) where callers pass no opts.
	// range over nil variadic is a no-op, not a crash.
	ssaPkg := buildSSA(t, `
		package example

		func Process(name string, opts ...func(int)) int {
			total := 0
			for _, o := range opts {
				o(total)
			}
			return total
		}

		func caller() int {
			return Process("test")
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "Process")
	findings := analyzer.Analyze(fn)

	// No Bug-severity findings for opts. range over nil is safe.
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug finding for variadic param: %s", f.Message)
		}
	}
}

func TestRegression_VariadicParam_WithCallerPassingArgs(t *testing.T) {
	t.Parallel()

	// When some callers DO pass variadic args, should also be fine.
	ssaPkg := buildSSA(t, `
		package example

		func add(x int) int { return x + 1 }

		func Process(name string, transforms ...func(int) int) int {
			total := 0
			for _, t := range transforms {
				total = t(total)
			}
			return total
		}

		func caller1() int {
			return Process("test")
		}

		func caller2() int {
			return Process("test", add)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "Process")
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug finding for variadic param: %s", f.Message)
		}
	}
}

func TestRegression_VariadicParam_DirectIndexWithLenGuard(t *testing.T) {
	t.Parallel()

	// Pattern: if len(hashFuncsArg) > 0 { use hashFuncsArg[0] }
	// Even if hashFuncsArg is DefinitelyNil, the len guard makes it safe.
	// The variadic cap ensures this isn't a Bug regardless.
	ssaPkg := buildSSA(t, `
		package example

		func derive(seed int, extras ...[]int) int {
			defaults := []int{1, 2, 3}
			if len(extras) > 0 {
				defaults = extras[0]
			}
			return defaults[seed%len(defaults)]
		}

		func caller() int {
			return derive(1)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "derive")
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug finding for variadic param: %s", f.Message)
		}
	}
}

// ---------------------------------------------------------------------------
// Category 14: NilBottom leak via transferCall
//
// When transferCall resolves zero callees (builtins like append, len, cap)
// or callees whose summaries have no return values, the result stayed at
// NilBottom — which downstream is treated as DefinitelyNil.
// Fixed by falling back to MaybeNil in both cases.
// ---------------------------------------------------------------------------

func TestRegression_NilBottomLeak_BuiltinAppend(t *testing.T) {
	t.Parallel()

	// append is a builtin with no StaticCallee. The CHA resolver returns
	// an empty callee list. transferCall must not leave the result at NilBottom.
	ssaPkg := buildSSA(t, `
		package example

		func buildSlice(n int) []int {
			var s []int
			for i := 0; i < n; i++ {
				s = append(s, i)
			}
			return s
		}

		func use() {
			s := buildSlice(10)
			if len(s) > 0 {
				_ = s[0]
			}
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_AppendInLoop(t *testing.T) {
	t.Parallel()

	// Pattern from ToStateSliceUpTo: var s []uint, append in loop, return s.
	// Both return paths (early nil, loop result) should not produce DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		type Set struct{}

		func (s *Set) Next(start int) (int, bool) {
			return start + 1, start < 10
		}

		func (s *Set) ToSlice(cap int) []int {
			if s == nil || cap < 0 {
				return nil
			}
			var result []int
			for i, ok := s.Next(0); ok && i <= cap; i, ok = s.Next(i + 1) {
				result = append(result, i)
			}
			return result
		}

		func caller(s *Set) {
			states := s.ToSlice(10)
			if len(states) == 0 {
				return
			}
			_ = states[0]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "caller")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for append-in-loop pattern: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_SliceAppendThenDeref(t *testing.T) {
	t.Parallel()

	// Direct dereference of append result without len guard.
	// Should be Warning (MaybeNil) at worst, never Bug (DefinitelyNil).
	ssaPkg := buildSSA(t, `
		package example

		func collect(items ...int) []int {
			var result []int
			for _, item := range items {
				result = append(result, item*2)
			}
			return result
		}

		func use() {
			s := collect(1, 2, 3)
			_ = s[0]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for slice from append: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_MakeAndAppend(t *testing.T) {
	t.Parallel()

	// make([]T, 0) + append — the make ensures non-nil, but the append
	// result should also not be flagged.
	ssaPkg := buildSSA(t, `
		package example

		func buildList(n int) []int {
			result := make([]int, 0, n)
			for i := 0; i < n; i++ {
				result = append(result, i)
			}
			return result
		}

		func use() {
			list := buildList(5)
			_ = list[0]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for make+append pattern: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_MultiReturnBuiltin(t *testing.T) {
	t.Parallel()

	// Builtins that return multiple values (e.g., map lookup with ok).
	// The call result should not be DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		func lookup(m map[string]*int, key string) int {
			v, ok := m[key]
			if ok && v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for map lookup pattern: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_VarSliceNeverAppended(t *testing.T) {
	t.Parallel()

	// var s []int with no append — s IS nil. But accessing it should be
	// a Warning (MaybeNil via Phi), not a Bug, because the control flow
	// path may not reach the access.
	ssaPkg := buildSSA(t, `
		package example

		func emptyOrFull(fill bool) []int {
			var s []int
			if fill {
				s = append(s, 1, 2, 3)
			}
			return s
		}

		func use(fill bool) {
			s := emptyOrFull(fill)
			if len(s) > 0 {
				_ = s[0]
			}
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for conditionally-filled slice: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_CalleeReturnsNilOnAllPaths(t *testing.T) {
	t.Parallel()

	// A function that genuinely returns nil on all paths SHOULD be DefinitelyNil.
	// This test verifies the fix doesn't suppress true positives.
	ssaPkg := buildSSA(t, `
		package example

		func alwaysNil() *int {
			return nil
		}

		func deref() int {
			p := alwaysNil()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "deref")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.NotEmpty(t, findings, "should flag dereference of always-nil return")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "always-nil return deref should be Bug severity")
}

func TestRegression_NilBottomLeak_CalleeReturnsNilOnSomePaths(t *testing.T) {
	t.Parallel()

	// Function returns nil on one path, non-nil on another.
	// Summary should be MaybeNil, not DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		func maybePtr(flag bool) *int {
			if flag {
				x := 42
				return &x
			}
			return nil
		}

		func use(flag bool) int {
			p := maybePtr(flag)
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"nil-checked callee return should not produce any findings")
}

func TestRegression_NilBottomLeak_UnresolvedCall(t *testing.T) {
	t.Parallel()

	// Function pointer call — resolveCallees returns nil (no static callee).
	// The result should be MaybeNil, not NilBottom/DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		func consume(fn func() *int) int {
			p := fn()
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "consume")

	// No resolver — simulates unresolved dispatch.
	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for unresolved call result: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_ChainedCallResults(t *testing.T) {
	t.Parallel()

	// result of a().b().c() — each method returns an object used by the next.
	// None should be flagged as DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		type Builder struct{}

		func NewBuilder() *Builder {
			return &Builder{}
		}

		func (b *Builder) WithOption(v int) *Builder {
			return b
		}

		func (b *Builder) Result() int {
			return 42
		}

		func use() int {
			return NewBuilder().WithOption(1).Result()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"chained builder calls should not produce nil findings")
}

func TestRegression_NilBottomLeak_TransferCallNilBottomGuard(t *testing.T) {
	t.Parallel()

	// Callee with void return (no return values) — summary.Returns is empty.
	// transferCall's loop produces no joins, res stays NilBottom.
	// Must fall back to MaybeNil.
	ssaPkg := buildSSA(t, `
		package example

		func sideEffect(x int) {}

		func use() *int {
			sideEffect(42)
			return nil
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// sideEffect returns void — its call result is not used as a pointer.
	// This should not crash or produce false positives.
	// No assertions on findings count — just verifying no panic and no Bug.
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug for void-return call: %s", f.Message)
		}
	}
}

func TestRegression_NilBottomLeak_ComputeReturnNilStates_NoReturnInstructions(t *testing.T) {
	t.Parallel()

	// Function with no explicit return — e.g., infinite loop or panic.
	// computeReturnNilStates returns nil (no returns found).
	// This should not cause downstream issues.
	ssaPkg := buildSSA(t, `
		package example

		func neverReturns() *int {
			panic("boom")
		}

		func caller() int {
			p := neverReturns()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "caller")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// neverReturns panics — summary has nil returns.
	// The dereference of p is technically unreachable.
	// We accept either no findings or a warning — but never a Bug
	// claiming "always nil" when the function never actually returns.
	for _, f := range findings {
		if f.Severity == analysis.Bug && contains(f.Message, "always nil") {
			t.Errorf("unexpected Bug for panic-path return: %s", f.Message)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRegression_VariadicParam_NoCallersAtAll(t *testing.T) {
	t.Parallel()

	// Exported function with no visible callers — variadic param
	// should not be flagged as DefinitelyNil.
	ssaPkg := buildSSA(t, `
		package example

		func NewData(x int, options ...func(int) int) int {
			v := x
			for _, option := range options {
				v = option(v)
			}
			return v
		}
	`)

	fn := findSSAFunc(t, ssaPkg, "NewData")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	for _, f := range findings {
		if f.Severity == analysis.Bug {
			t.Errorf("unexpected Bug finding for variadic param with no callers: %s", f.Message)
		}
	}
}
