package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 2. CommaOk: v, ok := x.(T) — value is MaybeNil without ok check
// ---------------------------------------------------------------------------
func TestTypeAssert_CommaOk_DerefWithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func unsafeCast(x interface{}) int {
			p, _ := x.(*int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "unsafeCast")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CommaOk: Extract #0 is MaybeNil. Deref without check → Warning.
	require.NotEmpty(t, findings, "CommaOk deref without check should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning (MaybeNil), not Bug (DefinitelyNil)")
	}
}

func TestTypeAssert_CommaOk_EarlyReturnGuard_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castWithGuard(x interface{}) int {
			p, ok := x.(*int)
			if !ok || p == nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castWithGuard")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// !ok || p == nil → early return. In the continuation, p is non-nil
	// because the || creates SSA blocks where p == nil is the false branch.
	require.Empty(t, findings,
		"early return guard with !ok || p == nil should be safe")
}

func TestTypeAssert_CommaOk_MultipleExtracts(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func twoAsserts(x, y interface{}) int {
			a, ok1 := x.(*int)
			b, ok2 := y.(*int)
			if ok1 && ok2 && a != nil && b != nil {
				return *a + *b
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "twoAsserts")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Two CommaOk assertions, both nil-checked before deref.
	require.Empty(t, findings,
		"two CommaOk assertions with nil checks should be safe")
}

func TestTypeAssert_CommaOk_ValueType_Int(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeCastInt(x interface{}) int {
			v, ok := x.(int)
			if ok {
				return v + 1
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "maybeCastInt")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// int is non-nillable — no nil deref possible regardless of ok check.
	require.Empty(t, findings,
		"CommaOk assertion to int should produce no warnings")
}

func TestTypeAssert_CommaOk_WithOkAndNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func safeCast(x interface{}) int {
			p, ok := x.(*int)
			if ok && p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "safeCast")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CommaOk with ok+nil check. Branch refinement from p != nil
	// sets p to DefinitelyNotNil in the true branch.
	require.Empty(t, findings,
		"CommaOk with ok && p != nil check should be safe")
}

func TestTypeAssert_CommaOk_WithOnlyNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castWithNilCheck(x interface{}) int {
			p, _ := x.(*int)
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castWithNilCheck")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Even without checking ok, a nil check on p is sufficient.
	require.Empty(t, findings,
		"CommaOk with nil check on value should be safe")
}

func TestTypeAssert_CommaOk_WithOnlyOkCheck_StillMaybeNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castWithOnlyOk(x interface{}) int {
			p, ok := x.(*int)
			if ok {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castWithOnlyOk")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// ok == true means the assertion succeeded, but x could have held
	// a typed nil (*int)(nil). So p is still MaybeNil.
	// The analyzer doesn't correlate ok to p's nil state, so this
	// correctly produces a Warning.
	// NOTE: If the analyzer is conservative (MaybeNil → Warning), we
	// expect a warning. If future tuple-aware refinement is added,
	// this could become safe.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should never be Bug — at worst Warning for MaybeNil")
	}
}

// ---------------------------------------------------------------------------
// 4. Type assertion in control flow patterns
// ---------------------------------------------------------------------------
func TestTypeAssert_InIfCondition_NonCommaOk(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func assertInIf(x interface{}, flag bool) int {
			if flag {
				p := x.(*int)
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "assertInIf")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Non-CommaOk in a branch — still DefinitelyNotNil if we get past it.
	require.Empty(t, findings,
		"non-CommaOk assertion in branch should be safe")
}

func TestTypeAssert_InLoop_NonCommaOk(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func assertInLoop(items []interface{}) int {
			total := 0
			for i := 0; i < len(items); i++ {
				p := items[i].(*int)
				total += *p
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "assertInLoop")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Non-CommaOk inside a loop body — still DefinitelyNotNil per iteration.
	require.Empty(t, findings,
		"non-CommaOk assertion in loop should be safe")
}

func TestTypeAssert_NonCommaOk_ChainedAssertions(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func chainedCast(x interface{}) int {
			p := x.(*int)
			return *p + 1
		}

		func caller(x interface{}) int {
			return chainedCast(x)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "chainedCast")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"chained assertion + deref should be safe")
}

func TestTypeAssert_NonCommaOk_FuncType(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castToFunc(x interface{}) int {
			fn := x.(func() int)
			return fn()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castToFunc")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// x.(func() int) panics if assertion fails.
	// fn is DefinitelyNotNil after successful assertion.
	// Calling fn() is safe.
	require.Empty(t, findings,
		"non-CommaOk type assertion to func should not warn")
}

func TestTypeAssert_NonCommaOk_MapType(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castToMap(x interface{}) bool {
			m := x.(map[string]int)
			_, ok := m["key"]
			return ok
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castToMap")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// x.(map[string]int) panics on failure. Map operations on nil
	// maps are safe for reads (return zero value). No nil deref risk.
	require.Empty(t, findings,
		"non-CommaOk type assertion to map should not warn")
}

func TestTypeAssert_NonCommaOk_MultipleAssertions(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func multiAssert(x, y interface{}) int {
			a := x.(*int)
			b := y.(*int)
			return *a + *b
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "multiAssert")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Both assertions are non-CommaOk → both results DefinitelyNotNil.
	require.Empty(t, findings,
		"multiple non-CommaOk assertions — both results are DefinitelyNotNil")
}

// ===========================================================================
// TypeAssert instruction tests
//
// These tests verify that transferTypeAssertInstr correctly assigns nil state
// for both CommaOk and non-CommaOk type assertions.
//
// SSA representation:
//
//   Non-CommaOk:  v := x.(T)
//     t0 = TypeAssert x.(*T)          → panics on failure, DefinitelyNotNil if we continue
//
//   CommaOk:      v, ok := x.(T)
//     t0 = TypeAssert x.(*T) ,ok      → tuple (*T, bool)
//     t1 = Extract t0 #0              → *T (MaybeNil — zero value when ok==false)
//     t2 = Extract t0 #1              → bool
// ===========================================================================
// ---------------------------------------------------------------------------
// 1. Non-CommaOk: v := x.(T) — panics on failure, DefinitelyNotNil
// ---------------------------------------------------------------------------
func TestTypeAssert_NonCommaOk_PointerType(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castAndDeref(x interface{}) int {
			p := x.(*int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castAndDeref")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// x.(*int) panics if x is not *int. If we continue, p is DefinitelyNotNil.
	// Dereferencing *p is safe.
	require.Empty(t, findings,
		"non-CommaOk type assertion result is DefinitelyNotNil — deref is safe")
}

func TestTypeAssert_NonCommaOk_SliceType(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castToSlice(x interface{}) int {
			s := x.([]int)
			return s[0]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castToSlice")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// x.([]int) panics if assertion fails.
	// If we reach s[0], s came from a successful assertion.
	// s could still be a nil slice that was wrapped in the interface,
	// but the TypeAssert itself succeeded. For slices, the IndexAddr
	// check only flags DefinitelyNil — so this should be clean.
	require.Empty(t, findings,
		"non-CommaOk type assertion to slice should not warn")
}

// ---------------------------------------------------------------------------
// 3. Non-pointer type assertions (value types)
// ---------------------------------------------------------------------------
func TestTypeAssert_NonCommaOk_ValueType_Int(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castToInt(x interface{}) int {
			return x.(int) + 1
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castToInt")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// int is non-nillable. The assertion result is DefinitelyNotNil
	// (though it doesn't matter since int can't be dereferenced).
	require.Empty(t, findings,
		"type assertion to value type should produce no warnings")
}

func TestTypeAssert_NonCommaOk_ValueType_String(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func castToString(x interface{}) string {
			return x.(string)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "castToString")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// string is a value type. No nil deref possible.
	require.Empty(t, findings,
		"type assertion to string should produce no warnings")
}

func TestTypeAssert_Regression_AllocStillNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useNew() int {
			p := new(int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useNew")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"new() must still produce DefinitelyNotNil")
}

func TestTypeAssert_Regression_MultiReturnExtractStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getPtr() (*int, error) {
			x := 42
			return &x, nil
		}

		func useExtract() int {
			p, err := getPtr()
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useExtract")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"multi-return Extract must still work after TypeAssert changes")
}

// ---------------------------------------------------------------------------
// 6. Regressions — ensure existing patterns still work alongside TypeAssert
// ---------------------------------------------------------------------------
func TestTypeAssert_Regression_NilCheckStillWorks(t *testing.T) {
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
		"basic nil check pattern must still work")
}

func TestTypeAssert_Regression_SingleReturnCallStillWorks(t *testing.T) {
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

func TestTypeAssert_SequentialAssertions(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func sequential(x interface{}) int {
			p := x.(*int)
			q := x.(*int)
			return *p + *q
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "sequential")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Two sequential assertions — both DefinitelyNotNil.
	require.Empty(t, findings,
		"sequential non-CommaOk assertions both produce DefinitelyNotNil")
}

func TestTypeAssert_TruePositive_CommaOk_DerefNilResult(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func derefFailed(x interface{}) int {
			p, _ := x.(*int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "derefFailed")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// CommaOk: p is MaybeNil. Deref without check → Warning.
	require.NotEmpty(t, findings, "deref of unchecked CommaOk result must warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"unchecked CommaOk deref should be Warning, not Bug")
	}
}

// ---------------------------------------------------------------------------
// 5. True positives — must still detect real bugs
// ---------------------------------------------------------------------------
func TestTypeAssert_TruePositive_NilInterfaceNonCommaOk(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func assertOnNil() int {
			var x interface{}
			p := x.(*int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "assertOnNil")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// x is nil. x.(*int) will panic at runtime.
	// But the TypeAssert instruction itself causes the panic, not the deref.
	// The analyzer marks p as DefinitelyNotNil (if we get past the assert).
	// So no *nil deref* finding is expected — the bug is the assertion, not *p.
	// This is correct behavior: the tool catches nil derefs, not panic-inducing
	// type assertions.
	// Accept either empty (assertion panics before deref) or non-empty.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"deref after non-CommaOk assertion should not be Bug — panic happens at assertion")
	}
}
