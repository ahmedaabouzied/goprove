package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// Lookup instruction tests
//
// These tests verify that transferMapLookup correctly assigns nil state
// for both CommaOk and non-CommaOk map lookups.
//
// SSA representation:
//
//   Non-CommaOk:  v := m[key]
//     t0 = Lookup m key              → type: V (zero value if missing, never panics)
//
//   CommaOk:      v, ok := m[key]
//     t0 = Lookup m key ,ok          → type: (V, bool)
//     t1 = Extract t0 #0             → type: V
//     t2 = Extract t0 #1             → type: bool
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. Non-CommaOk with nillable value types
// ---------------------------------------------------------------------------

func TestLookup_NonCommaOk_PointerValue_DerefWithoutCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getPtr(m map[string]*int, key string) int {
			v := m[key]
			return *v
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getPtr")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// m[key] returns *int which is nillable → MaybeNil.
	// Deref without nil check → Warning.
	require.NotEmpty(t, findings, "deref of unchecked map lookup should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"unchecked nillable map lookup deref should be Warning, not Bug")
	}
}

func TestLookup_NonCommaOk_PointerValue_WithNilCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getPtrSafe(m map[string]*int, key string) int {
			v := m[key]
			if v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getPtrSafe")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// m[key] → MaybeNil, but nil check refines to DefinitelyNotNil in branch.
	require.Empty(t, findings,
		"nil-checked map lookup value should be safe")
}

func TestLookup_NonCommaOk_SliceValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getSlice(m map[string][]int, key string) int {
			v := m[key]
			if len(v) > 0 {
				return v[0]
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getSlice")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// []int is nillable → MaybeNil. But len guard protects access.
	// Slice IndexAddr only flags DefinitelyNil, not MaybeNil.
	require.Empty(t, findings,
		"map lookup returning slice with len guard should be safe")
}

func TestLookup_NonCommaOk_MapValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getNestedMap(m map[string]map[string]int, key string) int {
			inner := m[key]
			v := inner["nested"]
			return v
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getNestedMap")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// inner is map type → nillable → MaybeNil.
	// But reading from a nil map is safe in Go (returns zero value).
	// v is int → non-nillable. No dereference of inner happens.
	require.Empty(t, findings,
		"nested map lookup with non-nillable final value should be safe")
}

func TestLookup_NonCommaOk_FuncValue_CallWithoutCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getFunc(m map[string]func() int, key string) int {
			fn := m[key]
			return fn()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getFunc")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// func() int is nillable → MaybeNil from transferMapLookup.
	// However, calling a nil func value (fn()) is a *ssa.Call, not a
	// dereference (*ssa.UnOp/FieldAddr/IndexAddr). checkInstruction only
	// checks nil on interface Invoke calls, not direct func value calls.
	// This is a known limitation — the nil state is correct (MaybeNil),
	// but no finding is emitted because the call isn't flagged.
	// TODO: Add func value nil call detection to checkInstruction.
	require.Empty(t, findings,
		"func value call not yet detected — known limitation")
}

func TestLookup_NonCommaOk_InterfaceValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getIface(m map[string]interface{}, key string) interface{} {
			v := m[key]
			return v
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getIface")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// interface{} is nillable → MaybeNil. But v is only returned, not deref'd.
	require.Empty(t, findings,
		"returning map lookup value without deref should be safe")
}

func TestLookup_NonCommaOk_ChanValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getChan(m map[string]chan int, key string) chan int {
			ch := m[key]
			return ch
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getChan")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// chan is nillable → MaybeNil. But ch is only returned, not used.
	require.Empty(t, findings,
		"returning channel from map lookup without send/recv should be safe")
}

// ---------------------------------------------------------------------------
// 2. Non-CommaOk with non-nillable value types
// ---------------------------------------------------------------------------

func TestLookup_NonCommaOk_IntValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getInt(m map[string]int, key string) int {
			return m[key]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getInt")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// int is non-nillable → DefinitelyNotNil. No deref possible.
	require.Empty(t, findings,
		"map lookup returning int should produce no warnings")
}

func TestLookup_NonCommaOk_StringValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getString(m map[int]string, key int) string {
			return m[key]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getString")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"map lookup returning string should produce no warnings")
}

func TestLookup_NonCommaOk_BoolValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getBool(m map[string]bool, key string) bool {
			return m[key]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getBool")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"map lookup returning bool should produce no warnings")
}

func TestLookup_NonCommaOk_StructValue(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		type Pair struct{}

		func getStruct(m map[string]Pair, key string) Pair {
			return m[key]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "getStruct")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Struct value type — non-nillable.
	require.Empty(t, findings,
		"map lookup returning struct value should produce no warnings")
}

// ---------------------------------------------------------------------------
// 3. CommaOk patterns
// ---------------------------------------------------------------------------

func TestLookup_CommaOk_PointerValue_WithOkAndNilCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func safeLookup(m map[string]*int, key string) int {
			v, ok := m[key]
			if ok && v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "safeLookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CommaOk + ok check + nil check → safe.
	require.Empty(t, findings,
		"CommaOk lookup with ok && nil check should be safe")
}

func TestLookup_CommaOk_PointerValue_WithOnlyNilCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func nilCheckOnly(m map[string]*int, key string) int {
			v, _ := m[key]
			if v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "nilCheckOnly")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Nil check alone is sufficient to guard the deref.
	require.Empty(t, findings,
		"CommaOk lookup with only nil check should be safe")
}

func TestLookup_CommaOk_PointerValue_DerefWithoutCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func unsafeLookup(m map[string]*int, key string) int {
			v, _ := m[key]
			return *v
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "unsafeLookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CommaOk but no nil check on v → Warning.
	require.NotEmpty(t, findings, "CommaOk deref without check should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"unchecked CommaOk deref should be Warning, not Bug")
	}
}

func TestLookup_CommaOk_IntValue_WithOkCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookupInt(m map[string]int, key string) int {
			v, ok := m[key]
			if ok {
				return v
			}
			return -1
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookupInt")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// int is non-nillable. No deref possible regardless of ok check.
	require.Empty(t, findings,
		"CommaOk lookup of int value should produce no warnings")
}

func TestLookup_CommaOk_EarlyReturnGuard(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookupWithGuard(m map[string]*int, key string) int {
			v, ok := m[key]
			if !ok || v == nil {
				return 0
			}
			return *v
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookupWithGuard")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// !ok || v == nil → early return. After guard, v is non-nil.
	require.Empty(t, findings,
		"CommaOk with early return guard should be safe")
}

// ---------------------------------------------------------------------------
// 4. Nil map lookups (safe in Go — returns zero value)
// ---------------------------------------------------------------------------

func TestLookup_NilMap_NonCommaOk(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookupNilMap() int {
			var m map[string]int
			return m["key"]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookupNilMap")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Lookup on nil map returns zero value. int is non-nillable.
	// Go does not panic on nil map read.
	require.Empty(t, findings,
		"lookup on nil map with non-nillable value should be safe")
}

func TestLookup_NilMap_CommaOk(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookupNilMapCommaOk() int {
			var m map[string]int
			v, ok := m["key"]
			if ok {
				return v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookupNilMapCommaOk")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Lookup on nil map: ok is false, v is zero value. No panic.
	require.Empty(t, findings,
		"CommaOk lookup on nil map should be safe")
}

// ---------------------------------------------------------------------------
// 5. Multiple lookups and combined patterns
// ---------------------------------------------------------------------------

func TestLookup_MultipleLookups_DifferentMaps(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func multiLookup(m1 map[string]*int, m2 map[string]*int, k1, k2 string) int {
			a := m1[k1]
			b := m2[k2]
			if a != nil && b != nil {
				return *a + *b
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "multiLookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Both lookups are MaybeNil, both nil-checked before deref.
	require.Empty(t, findings,
		"multiple lookups both nil-checked should be safe")
}

func TestLookup_LookupThenCallMethod(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookupAndUse(m map[string]*int, key string) int {
			v := m[key]
			if v == nil {
				return 0
			}
			return *v
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookupAndUse")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Nil check via == nil early return.
	require.Empty(t, findings,
		"lookup with == nil early return should be safe")
}

func TestLookup_LookupInLoop(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookupAll(m map[string]*int, keys []string) int {
			total := 0
			for i := 0; i < len(keys); i++ {
				v := m[keys[i]]
				if v != nil {
					total += *v
				}
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookupAll")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// Lookup in loop with nil check per iteration.
	require.Empty(t, findings,
		"lookup in loop with nil check should be safe")
}

// ---------------------------------------------------------------------------
// 6. True positives — must detect real bugs
// ---------------------------------------------------------------------------

func TestLookup_TruePositive_DerefWithoutAnyCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func unsafeGet(m map[string]*int) int {
			return *m["key"]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "unsafeGet")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// m["key"] is MaybeNil (pointer value), deref without check → Warning.
	require.NotEmpty(t, findings, "direct deref of map lookup must warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"unchecked map value deref should be Warning")
	}
}

// ---------------------------------------------------------------------------
// 7. Regressions — existing patterns still work alongside Lookup
// ---------------------------------------------------------------------------

func TestLookup_Regression_NilCheckStillWorks(t *testing.T) {
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

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "basic nil check must still work")
}

func TestLookup_Regression_ExtractStillWorks(t *testing.T) {
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

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "Extract from multi-return must still work")
}

func TestLookup_Regression_TypeAssertStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func assertDeref(x interface{}) int {
			p := x.(*int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "assertDeref")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"non-CommaOk TypeAssert must still produce DefinitelyNotNil")
}

func TestLookup_Regression_AlwaysNilCallStillBug(t *testing.T) {
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

	analyzer := analysis.NewNilAnalyzer(nil, nil)
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

func TestLookup_Regression_AllocStillNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useNew() int {
			p := new(int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useNew")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings, "new() must still produce DefinitelyNotNil")
}
