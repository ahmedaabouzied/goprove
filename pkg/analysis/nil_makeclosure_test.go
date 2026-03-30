package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// MakeClosure and Function reference tests
//
// Two fixes are tested here:
//
// 1. *ssa.MakeClosure in transferInstruction → DefinitelyNotNil
//    When a closure captures free variables, SSA emits MakeClosure.
//    This is an allocation — always non-nil.
//
// 2. *ssa.Function in lookupNilState → DefinitelyNotNil
//    When a closure has no captures, SSA optimizes away MakeClosure
//    and uses the *ssa.Function directly. Function references are
//    compile-time constants — always non-nil.
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. No-capture closures (*ssa.Function — no MakeClosure emitted)
// ---------------------------------------------------------------------------

func TestClosure_NoCaptureReturn_NonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeHandler() func() int {
			return func() int { return 42 }
		}

		func use() int {
			h := makeHandler()
			return h()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// No captures → SSA returns *ssa.Function directly.
	// lookupNilState returns DefinitelyNotNil for *ssa.Function.
	// Summary.Returns[0] = DefinitelyNotNil → h is safe to call.
	require.Empty(t, findings,
		"no-capture closure return should be DefinitelyNotNil")
}

func TestClosure_NoCaptureReturn_DerefResult(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeGetter() func() *int {
			return func() *int {
				x := 42
				return &x
			}
		}

		func use() int {
			getter := makeGetter()
			p := getter()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// getter is non-nil (no-capture closure) — MakeClosure/Function fix works.
	// However, getter() calls the anonymous function. The interprocedural
	// analysis computes the closure's return summary, but the closure is
	// called indirectly (func value call, StaticCallee == nil), so the
	// summary isn't resolved — result defaults to MaybeNil.
	// The Warning is on *p (result of getter()), not on getter itself.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug — at worst Warning for closure return")
	}
}

func TestClosure_NoCaptureAssignment_CallSafe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func double(x int) int { return x * 2 }

		func use() int {
			fn := double
			return fn(21)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// fn = double → *ssa.Function reference. Always non-nil.
	require.Empty(t, findings,
		"function reference assignment should be DefinitelyNotNil")
}

func TestClosure_NoCapturePassedAsArg(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func apply(fn func(int) int, x int) int {
			if fn != nil {
				return fn(x)
			}
			return x
		}

		func double(x int) int { return x * 2 }

		func use() int {
			return apply(double, 21)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// double is *ssa.Function → non-nil. apply checks fn != nil → safe.
	require.Empty(t, findings,
		"passing function reference as arg should be safe")
}

// ---------------------------------------------------------------------------
// 2. Closures with captures (*ssa.MakeClosure)
// ---------------------------------------------------------------------------

func TestClosure_WithCapture_ReturnNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeAdder(x int) func(int) int {
			return func(y int) int { return x + y }
		}

		func use() int {
			add5 := makeAdder(5)
			return add5(10)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Captures x → MakeClosure emitted → DefinitelyNotNil.
	// Summary.Returns[0] = DefinitelyNotNil → add5 safe to call.
	require.Empty(t, findings,
		"closure with capture should be DefinitelyNotNil")
}

func TestClosure_WithCapture_MultipleCaptures(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeFormatter(prefix string, suffix string) func(string) string {
			return func(s string) string { return prefix + s + suffix }
		}

		func use() string {
			fmt := makeFormatter("[", "]")
			return fmt("hello")
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Captures prefix and suffix → MakeClosure → DefinitelyNotNil.
	require.Empty(t, findings,
		"closure with multiple captures should be DefinitelyNotNil")
}

func TestClosure_WithCapture_DerefResult(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeCounter(start int) func() *int {
			count := start
			return func() *int {
				count++
				return &count
			}
		}

		func use() int {
			counter := makeCounter(0)
			p := counter()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// counter is non-nil (MakeClosure) — correctly tracked.
	// counter() calls the closure indirectly (func value call,
	// StaticCallee == nil), so the return summary isn't resolved —
	// result defaults to MaybeNil. Warning on *p is expected.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug — at worst Warning for closure return")
	}
}

// ---------------------------------------------------------------------------
// 3. Closure returned via multi-return
// ---------------------------------------------------------------------------

func TestClosure_MultiReturn_NoCaptureNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getHandler(name string) (func() int, error) {
			return func() int { return 42 }, nil
		}

		func use() int {
			h, err := getHandler("test")
			if err != nil {
				return 0
			}
			return h()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// getHandler always returns (non-nil closure, nil).
	// Extract #0 → DefinitelyNotNil from summary.
	require.Empty(t, findings,
		"no-capture closure from multi-return should be DefinitelyNotNil")
}

func TestClosure_MultiReturn_WithCaptureNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getAdder(x int) (func(int) int, error) {
			return func(y int) int { return x + y }, nil
		}

		func use() int {
			add, err := getAdder(5)
			if err != nil {
				return 0
			}
			return add(10)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Captures x → MakeClosure → DefinitelyNotNil in summary.
	require.Empty(t, findings,
		"closure with capture from multi-return should be DefinitelyNotNil")
}

func TestClosure_MultiReturn_MaybeClosure(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeHandler(name string) (func() int, error) {
			if name == "" {
				return nil, nil
			}
			return func() int { return 42 }, nil
		}

		func useUnsafe(name string) int {
			h, _ := maybeHandler(name)
			return h()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useUnsafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// maybeHandler returns nil on one path → MaybeNil.
	// h() without check → Warning.
	require.NotEmpty(t, findings, "unchecked maybe-nil closure should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning, not Bug")
	}
}

func TestClosure_MultiReturn_MaybeClosureWithCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeHandler(name string) (func() int, error) {
			if name == "" {
				return nil, nil
			}
			return func() int { return 42 }, nil
		}

		func useSafe(name string) int {
			h, _ := maybeHandler(name)
			if h != nil {
				return h()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useSafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// h is nil-checked before call → safe.
	require.Empty(t, findings,
		"nil-checked maybe-nil closure should be safe")
}

// ---------------------------------------------------------------------------
// 4. Closure stored in variable then called
// ---------------------------------------------------------------------------

func TestClosure_StoredInVar_NilCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func process(transform func(int) int, x int) int {
			if transform == nil {
				return x
			}
			return transform(x)
		}

		func use() int {
			double := func(x int) int { return x * 2 }
			return process(double, 21)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"closure assigned to var and passed to nil-checking func should be safe")
}

// ---------------------------------------------------------------------------
// 5. True positives — nil func values must still be detected
// ---------------------------------------------------------------------------

func TestClosure_TruePositive_NilVar(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func callNil() int {
			var fn func() int
			return fn()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "callNil")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.NotEmpty(t, findings, "calling nil func var must be flagged")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "nil func var call must be Bug")
}

func TestClosure_TruePositive_ParamUnchecked(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func apply(fn func() int) int {
			return fn()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "apply")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// fn is a param → MaybeNil. Call without check → Warning.
	require.NotEmpty(t, findings, "unchecked func param must warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning, not Bug")
	}
}

// ---------------------------------------------------------------------------
// 6. Regressions
// ---------------------------------------------------------------------------

func TestClosure_Regression_PointerNilCheck(t *testing.T) {
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

	require.Empty(t, findings, "pointer nil check must still work")
}

func TestClosure_Regression_ExtractStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getVal() (*int, error) {
			x := 1
			return &x, nil
		}

		func use() int {
			p, err := getVal()
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "Extract must still work")
}

func TestClosure_Regression_BuiltinNotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useBuiltins(s []int) int {
			n := len(s)
			s = append(s, 42)
			return n + cap(s)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useBuiltins")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "builtins must not be flagged")
}

func TestClosure_Regression_AlwaysNilDeref(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func alwaysNil() *int { return nil }

		func deref() int {
			return *alwaysNil()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "deref")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.NotEmpty(t, findings, "always-nil deref must still be caught")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "must be Bug severity")
}
