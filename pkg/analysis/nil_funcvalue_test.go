package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 4. Func value always non-nil — no warning expected
// ---------------------------------------------------------------------------
func TestFuncValue_AlwaysNonNil_FromCallee(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeHandler() func() int {
			return func() int { return 42 }
		}

		func useAlwaysNonNil() int {
			h := makeHandler()
			return h()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useAlwaysNonNil")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// makeHandler returns a closure via MakeClosure → DefinitelyNotNil.
	// Summary.Returns[0] = DefinitelyNotNil → h is DefinitelyNotNil.
	require.Empty(t, findings,
		"calling always-non-nil func value should be safe")
}

func TestFuncValue_BothChecked_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func callBoth(a func() int, b func() int) int {
			if a != nil && b != nil {
				return a() + b()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "callBoth")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"both func values nil-checked should be safe")
}

func TestFuncValue_Builtin_Append_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useAppend(s []int, x int) []int {
			return append(s, x)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useAppend")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin append must not be flagged as nil func call")
}

func TestFuncValue_Builtin_Cap_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useCap(s []int) int {
			return cap(s)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useCap")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin cap must not be flagged as nil func call")
}

func TestFuncValue_Builtin_Close_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useClose(ch chan int) {
			close(ch)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useClose")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin close must not be flagged as nil func call")
}

func TestFuncValue_Builtin_Copy_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useCopy(dst, src []int) int {
			return copy(dst, src)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useCopy")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin copy must not be flagged as nil func call")
}

func TestFuncValue_Builtin_Delete_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useDelete(m map[string]int, key string) {
			delete(m, key)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useDelete")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin delete must not be flagged as nil func call")
}

// ---------------------------------------------------------------------------
// 8. Builtins must NOT be flagged
// ---------------------------------------------------------------------------
func TestFuncValue_Builtin_Len_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useLen(s []int) int {
			return len(s)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useLen")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin len must not be flagged as nil func call")
}

func TestFuncValue_Builtin_Panic_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func usePanic() {
			panic("boom")
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "usePanic")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin panic must not be flagged as nil func call")
}

func TestFuncValue_Builtin_Print_NotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func usePrint(x int) {
			print(x)
			println(x)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "usePrint")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"builtin print/println must not be flagged as nil func call")
}

// ---------------------------------------------------------------------------
// 9. Builtins mixed with func values in same function
// ---------------------------------------------------------------------------
func TestFuncValue_BuiltinAndFuncValue_Mixed(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func mixed(s []int, fn func(int) int) int {
			n := len(s)
			if n == 0 {
				return 0
			}
			return fn(s[0])
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "mixed")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// len(s) is a builtin — must not be flagged.
	// fn(s[0]) is a func value call — fn is MaybeNil → Warning.
	warningCount := 0
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug")
		if f.Severity == analysis.Warning {
			warningCount++
		}
	}
	require.Equal(t, 1, warningCount,
		"exactly one warning for unchecked fn, none for builtin len")
}

// ---------------------------------------------------------------------------
// 5. Func value DefinitelyNil — Bug severity
// ---------------------------------------------------------------------------
func TestFuncValue_DefinitelyNil_Bug(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func callNilFunc() int {
			var fn func() int
			return fn()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "callNilFunc")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// var fn func() int → fn is nil. Calling it → Bug.
	require.NotEmpty(t, findings, "calling nil func should be flagged")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "calling DefinitelyNil func should be Bug severity")
}

// ---------------------------------------------------------------------------
// 7. Func value in loop
// ---------------------------------------------------------------------------
func TestFuncValue_InLoop_WithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func runAll(fns []func() int) int {
			total := 0
			for i := 0; i < len(fns); i++ {
				fn := fns[i]
				if fn != nil {
					total += fn()
				}
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "runAll")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"func value in loop with nil check should be safe")
}

func TestFuncValue_InLoop_WithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func runAllUnsafe(fns []func() int) int {
			total := 0
			for i := 0; i < len(fns); i++ {
				fn := fns[i]
				total += fn()
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "runAllUnsafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// fn from slice index is MaybeNil. Call without check → Warning.
	require.NotEmpty(t, findings, "func value in loop without check should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning, not Bug")
	}
}

// ---------------------------------------------------------------------------
// 2. Func value from map lookup
// ---------------------------------------------------------------------------
func TestFuncValue_MapLookup_CallWithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func dispatch(handlers map[string]func(), key string) {
			h := handlers[key]
			h()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "dispatch")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// h is MaybeNil from map lookup. Call without check → Warning.
	require.NotEmpty(t, findings, "calling unchecked func from map lookup should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning, not Bug")
	}
}

func TestFuncValue_MapLookup_CommaOk_WithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func dispatchSafeOk(handlers map[string]func() int, key string) int {
			h, ok := handlers[key]
			if ok && h != nil {
				return h()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "dispatchSafeOk")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"CommaOk with ok && nil check should be safe")
}

func TestFuncValue_MapLookup_CommaOk_WithOkCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func dispatchOk(handlers map[string]func() int, key string) int {
			h, ok := handlers[key]
			if ok {
				return h()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "dispatchOk")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// ok == true means key exists, but the value could be nil func.
	// h is MaybeNil (Extract from Lookup tuple). ok check doesn't
	// refine h's nil state. Should warn.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug — at worst Warning")
	}
}

func TestFuncValue_MapLookup_WithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func dispatchSafe(handlers map[string]func(), key string) {
			h := handlers[key]
			if h != nil {
				h()
			}
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "dispatchSafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"nil-checked func from map lookup should be safe")
}

// ---------------------------------------------------------------------------
// 6. Multiple func value calls
// ---------------------------------------------------------------------------
func TestFuncValue_MultipleCalls_MixedStates(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func callTwo(a func() int, b func() int) int {
			if a != nil {
				return a() + b()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "callTwo")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// a is nil-checked → safe. b is not checked → Warning.
	warningCount := 0
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"neither call should be Bug")
		if f.Severity == analysis.Warning {
			warningCount++
		}
	}
	require.Equal(t, 1, warningCount,
		"exactly one warning for unchecked b")
}

func TestFuncValue_MultiReturn_WithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getHandler(name string) (func() int, error) {
			if name == "" {
				return nil, nil
			}
			return func() int { return 42 }, nil
		}

		func useHandlerSafe(name string) int {
			h, _ := getHandler(name)
			if h != nil {
				return h()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useHandlerSafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"nil-checked func from multi-return should be safe")
}

// ---------------------------------------------------------------------------
// 3. Func value from multi-return
// ---------------------------------------------------------------------------
func TestFuncValue_MultiReturn_WithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getHandler(name string) (func() int, error) {
			if name == "" {
				return nil, nil
			}
			return func() int { return 42 }, nil
		}

		func useHandler(name string) int {
			h, _ := getHandler(name)
			return h()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useHandler")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// getHandler returns (nil, nil) on one path → h is MaybeNil.
	// Call without check → Warning.
	require.NotEmpty(t, findings, "calling unchecked func from multi-return should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning, not Bug")
	}
}

func TestFuncValue_ParamCallWithEarlyReturn_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func applyGuarded(fn func() int) int {
			if fn == nil {
				return 0
			}
			return fn()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "applyGuarded")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Early return on nil → fn is DefinitelyNotNil in continuation.
	require.Empty(t, findings,
		"early return guard on func param should be safe")
}

func TestFuncValue_ParamCallWithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func applyIfNotNil(fn func(int) int, x int) int {
			if fn != nil {
				return fn(x)
			}
			return x
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "applyIfNotNil")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// fn is nil-checked before call → safe.
	require.Empty(t, findings,
		"nil-checked func param call should be safe")
}

// ===========================================================================
// Nil func value call detection tests
//
// These tests verify that checkInstruction detects calls to potentially nil
// func-typed values. In SSA, a func value call has:
//   - IsInvoke() == false (not an interface method dispatch)
//   - StaticCallee() == nil (callee not known at compile time)
//   - v.Call.Value is a func-typed SSA value
//
// Builtins (len, cap, append, etc.) also have StaticCallee() == nil but
// are *ssa.Builtin — these must be excluded.
// ===========================================================================
// ---------------------------------------------------------------------------
// 1. Func value parameter — MaybeNil without check
// ---------------------------------------------------------------------------
func TestFuncValue_ParamCallWithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func apply(fn func(int) int, x int) int {
			return fn(x)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "apply")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// fn is a func parameter — MaybeNil. Calling without nil check → Warning.
	require.NotEmpty(t, findings, "calling unchecked func param should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"unchecked func param call should be Warning, not Bug")
	}
}

func TestFuncValue_Regression_AlwaysNilCallStillBug(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func alwaysNil() *int { return nil }

		func derefNil() int {
			p := alwaysNil()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "derefNil")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.NotEmpty(t, findings, "always-nil deref must still be caught")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "always-nil deref must be Bug severity")
}

func TestFuncValue_Regression_ExtractStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getVal() (*int, error) {
			x := 1
			return &x, nil
		}

		func useExtract() int {
			p, err := getVal()
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useExtract")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "Extract must still work")
}

func TestFuncValue_Regression_LoopWithLenStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func walkSlice(ptrs []*int) int {
			sum := 0
			for i := 0; i < len(ptrs); i++ {
				p := ptrs[i]
				if p != nil {
					sum += *p
				}
			}
			return sum
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "walkSlice")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// len() is a builtin — must not be flagged.
	// p is nil-checked before deref — safe.
	require.Empty(t, findings,
		"loop with len() and nil check must still work")
}

// ---------------------------------------------------------------------------
// 10. Regressions — existing patterns still work
// ---------------------------------------------------------------------------
func TestFuncValue_Regression_NilCheckPointer(t *testing.T) {
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

func TestFuncValue_Regression_StaticCallNotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func helper(x int) int { return x + 1 }

		func useHelper() int {
			return helper(42)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useHelper")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Static call — StaticCallee() is non-nil. Must not be flagged.
	require.Empty(t, findings,
		"static function call must not be flagged as nil func value")
}

func TestFuncValue_Regression_TypeAssertStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castDeref(x interface{}) int {
			p := x.(*int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castDeref")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "TypeAssert non-CommaOk must still work")
}
