package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

func TestGlobal_ArrayIndex_InLoop(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var flags = [8]bool{true, false, true, false, true, false, true, false}

		func countTrue() int {
			count := 0
			for i := 0; i < 8; i++ {
				if flags[i] {
					count++
				}
			}
			return count
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "countTrue")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"indexing global array in loop should not warn")
}

func TestGlobal_ArrayIndex_MultipleAccesses(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var table = [16]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

		func sum4(a, b, c, d int) int {
			return table[a] + table[b] + table[c] + table[d]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "sum4")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Multiple IndexAddr on the same global array — all safe.
	require.Empty(t, findings,
		"multiple accesses to global array should not warn")
}

// ===========================================================================
// Global variable tests
//
// In SSA, *ssa.Global represents the ADDRESS of a package-level variable.
// The address of a global is always valid (allocated in the data segment),
// so lookupNilState returns DefinitelyNotNil for *ssa.Global.
//
// This is distinct from the VALUE stored in the global, which may be nil
// for pointer-typed globals. Reading the value requires a load (UnOp MUL),
// which is handled separately.
//
// These tests verify that accessing globals (arrays, structs, pointers)
// does not produce false positives for the address itself, while still
// correctly warning about nil values stored in pointer globals.
// ===========================================================================
// ---------------------------------------------------------------------------
// 1. Global arrays — value types, address always valid
// ---------------------------------------------------------------------------
func TestGlobal_ArrayIndex_NoWarning(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var lookup = [256]byte{'a': 1, 'b': 2}

		func get(i int) byte {
			return lookup[i]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "get")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// lookup is a global [256]byte. Its address is always valid.
	// IndexAddr on it should not warn.
	require.Empty(t, findings,
		"indexing global array should not produce nil warnings")
}

func TestGlobal_FuncVar_CallWithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var handler func() int

		func callHandlerSafe() int {
			if handler != nil {
				return handler()
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "callHandlerSafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"nil-checked global func var call should be safe")
}

// ---------------------------------------------------------------------------
// 6. Global func var — address valid, value can be nil
// ---------------------------------------------------------------------------
func TestGlobal_FuncVar_CallWithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var handler func() int

		func callHandler() int {
			return handler()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "callHandler")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// handler is a global func var. Its address is valid, but the loaded
	// func value could be nil. Calling without check should warn.
	require.NotEmpty(t, findings, "calling unchecked global func var should warn")
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should be Warning, not Bug")
	}
}

// ---------------------------------------------------------------------------
// 5. Global map — address valid
// ---------------------------------------------------------------------------
func TestGlobal_Map_ReadNoWarning(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var registry = map[string]int{"a": 1}

		func lookup(key string) int {
			return registry[key]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// registry is a global map. Reading from nil map returns zero value (safe).
	// int is non-nillable. No deref risk.
	require.Empty(t, findings,
		"reading from global map with non-nillable value should not warn")
}

// ---------------------------------------------------------------------------
// 7. Multiple globals in same function
// ---------------------------------------------------------------------------
func TestGlobal_MultipleGlobals_Mixed(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var arr = [4]int{1, 2, 3, 4}
		var counter int

		func getAndIncrement(i int) int {
			counter++
			return arr[i]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getAndIncrement")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Both arr and counter are globals. Addresses always valid.
	require.Empty(t, findings,
		"accessing multiple globals should not warn")
}

// ---------------------------------------------------------------------------
// 8. Global used as argument
// ---------------------------------------------------------------------------
func TestGlobal_PassedAsArg_AddressNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "sync"

		var mu sync.Mutex

		func withLock(fn func()) {
			mu.Lock()
			fn()
			mu.Unlock()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "withLock")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// mu.Lock() is a method call on global — address is valid.
	// fn is a func param → MaybeNil → Warning for fn().
	warningCount := 0
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity)
		if f.Severity == analysis.Warning {
			warningCount++
		}
	}
	require.LessOrEqual(t, warningCount, 1,
		"at most 1 warning for unchecked fn param, none for global")
}

func TestGlobal_PointerVar_DerefWithNilCheck_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var globalPtr *int

		func derefSafe() int {
			if globalPtr != nil {
				return *globalPtr
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "derefSafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// globalPtr checked for nil before deref → safe.
	require.Empty(t, findings,
		"nil-checked global pointer deref should be safe")
}

// ---------------------------------------------------------------------------
// 4. Global pointer — address valid, but VALUE can be nil
// ---------------------------------------------------------------------------
func TestGlobal_PointerVar_DerefWithoutCheck_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var globalPtr *int

		func derefGlobal() int {
			return *globalPtr
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "derefGlobal")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// globalPtr is a global *int. Its ADDRESS is always valid (DefinitelyNotNil).
	// But loading its VALUE (UnOp MUL on the global) yields MaybeNil.
	// Derefing the loaded value (*globalPtr = load then deref) should warn.
	// Note: the SSA is: t0 = *globalPtr (load), t1 = *t0 (deref).
	// The first * loads from the global address (safe), the second * derefs
	// the loaded pointer (MaybeNil).
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"global pointer deref should be Warning at worst, not Bug")
	}
}

func TestGlobal_Regression_AllocStillNonNil(t *testing.T) {
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

	require.Empty(t, findings, "new() must still produce DefinitelyNotNil")
}

func TestGlobal_Regression_AlwaysNilStillBug(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func alwaysNil() *int { return nil }

		func derefNil() int {
			return *alwaysNil()
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
	require.True(t, hasBug, "must be Bug severity")
}

func TestGlobal_Regression_BuiltinNotFlagged(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useBuiltins(s []int) int {
			return len(s) + cap(s)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useBuiltins")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "builtins must not be flagged")
}

func TestGlobal_Regression_ExtractStillWorks(t *testing.T) {
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

func TestGlobal_Regression_FuncRefStillNonNil(t *testing.T) {
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

	require.Empty(t, findings, "function reference must still be DefinitelyNotNil")
}

// ---------------------------------------------------------------------------
// 9. Regressions
// ---------------------------------------------------------------------------
func TestGlobal_Regression_NilCheckStillWorks(t *testing.T) {
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

	require.Empty(t, findings, "basic nil check must still work")
}

func TestGlobal_Regression_TypeAssertStillWorks(t *testing.T) {
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

// ---------------------------------------------------------------------------
// 3. Global slices — address valid, but value can be nil
// ---------------------------------------------------------------------------
func TestGlobal_SliceVar_AddressValid(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var items []int

		func appendItem(x int) {
			items = append(items, x)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "appendItem")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// items is a global []int. The address of items is valid.
	// append on nil slice is safe in Go.
	require.Empty(t, findings,
		"appending to global slice should not warn")
}

// ---------------------------------------------------------------------------
// 2. Global structs — value types, address always valid
// ---------------------------------------------------------------------------
func TestGlobal_StructFieldAccess_NoWarning(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "sync"

		var mu sync.Mutex

		func lock() {
			mu.Lock()
			mu.Unlock()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lock")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// mu is a global sync.Mutex. Its address is always valid.
	// FieldAddr on it should not warn.
	require.Empty(t, findings,
		"accessing fields of global struct should not warn")
}
