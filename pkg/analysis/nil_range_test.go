package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// Range over nil slice / map — false positive tests
//
// In Go, ranging over a nil slice or nil map is a no-op — it does NOT panic.
// The loop body simply never executes. The analyzer should not flag these
// as nil dereferences.
//
// Tests marked "KNOWN FALSE POSITIVE" are expected to produce findings
// that shouldn't exist. When the fix is implemented, flip the assertion.
// ===========================================================================

// ---------------------------------------------------------------------------
// Category 15: Range over maybe-nil slice — no panic
// ---------------------------------------------------------------------------

func TestFalsePositive_RangeOverMaybeNilSlice_IntElements(t *testing.T) {
	t.Parallel()

	// range over a nil []int is a no-op. No panic, no dereference.
	ssaPkg := buildSSA(t, `
		package example

		func sum(items []int) int {
			total := 0
			for _, v := range items {
				total += v
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "sum")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Range over nil slice is safe — should produce no findings.
	require.Empty(t, findings,
		"range over maybe-nil []int should not be flagged — it's a no-op")
}

func TestFalsePositive_RangeOverMaybeNilSlice_PtrElements_NilChecked(t *testing.T) {
	t.Parallel()

	// range over nil []*int is a no-op. Elements are nil-checked inside.
	ssaPkg := buildSSA(t, `
		package example

		func process(items []*int) int {
			total := 0
			for _, item := range items {
				if item != nil {
					total += *item
				}
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "process")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// The slice may be nil (no-op range) and elements are nil-checked.
	require.Empty(t, findings,
		"range over maybe-nil []*int with nil check on elements should be safe")
}

func TestFalsePositive_RangeOverMaybeNilSlice_FromCallee(t *testing.T) {
	t.Parallel()

	// Callee may return nil slice. Ranging over it is safe.
	ssaPkg := buildSSA(t, `
		package example

		func getItems(cond bool) []*int {
			if cond {
				x := 1
				return []*int{&x}
			}
			return nil
		}

		func use(cond bool) int {
			items := getItems(cond)
			total := 0
			for _, item := range items {
				total += *item
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: the range itself is safe (nil → no iterations),
	// but *item inside is also safe because if items is nil the loop body
	// never executes. The analyzer may not understand this yet.
	require.Empty(t, findings,
		"range over maybe-nil slice from callee should not be flagged")
}

func TestFalsePositive_RangeOverMaybeNilSlice_OnlyIndex(t *testing.T) {
	t.Parallel()

	// Using only the index, not the element. Still safe if slice is nil.
	ssaPkg := buildSSA(t, `
		package example

		func countItems(items []string) int {
			count := 0
			for range items {
				count++
			}
			return count
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "countItems")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"range over maybe-nil []string using only index should not be flagged")
}

func TestFalsePositive_RangeOverMaybeNilSlice_Param(t *testing.T) {
	t.Parallel()

	// Slice parameter that is maybe-nil. Range is safe.
	ssaPkg := buildSSA(t, `
		package example

		type Handler func(int)

		func runHandlers(handlers []Handler, val int) {
			for _, h := range handlers {
				h(val)
			}
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "runHandlers")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: handlers may be nil but range is a no-op.
	// Individual handler h could also be nil but that's a different issue
	// (element nullability, not slice nullability).
	// For now: no findings expected for the RANGE itself.
	for _, f := range findings {
		require.NotContains(t, f.Message, "nil dereference of handlers",
			"should not flag the slice itself as nil deref in range")
	}
}

// ---------------------------------------------------------------------------
// Category 16: Range over nil map — no panic
// ---------------------------------------------------------------------------

func TestFalsePositive_RangeOverMaybeNilMap(t *testing.T) {
	t.Parallel()

	// range over a nil map is a no-op.
	ssaPkg := buildSSA(t, `
		package example

		func sumMap(m map[string]int) int {
			total := 0
			for _, v := range m {
				total += v
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "sumMap")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"range over maybe-nil map should not be flagged — it's a no-op")
}

func TestFalsePositive_RangeOverMaybeNilMap_PtrValues(t *testing.T) {
	t.Parallel()

	// range over nil map[string]*int — no-op. Values nil-checked inside.
	ssaPkg := buildSSA(t, `
		package example

		func sumPtrMap(m map[string]*int) int {
			total := 0
			for _, v := range m {
				if v != nil {
					total += *v
				}
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "sumPtrMap")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"range over maybe-nil map with nil-checked ptr values should be safe")
}

func TestFalsePositive_RangeOverMaybeNilMap_FromCallee(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func getConfig(ok bool) map[string]string {
			if ok {
				return map[string]string{"a": "b"}
			}
			return nil
		}

		func readConfig(ok bool) string {
			cfg := getConfig(ok)
			result := ""
			for k, v := range cfg {
				result += k + "=" + v + ";"
			}
			return result
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "readConfig")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"range over maybe-nil map from callee should not be flagged")
}

// ---------------------------------------------------------------------------
// Category 17: Regression — nil slice ranging should still flag element derefs
// ---------------------------------------------------------------------------

func TestCorrect_RangeOverSlice_ElementDerefWithoutCheck(t *testing.T) {
	t.Parallel()

	// The range itself is safe, but dereferencing elements without a nil
	// check IS a real issue. We should flag *item, not the range.
	ssaPkg := buildSSA(t, `
		package example

		func derefAll(items []*int) int {
			total := 0
			for _, item := range items {
				total += *item
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "derefAll")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// *item is a legitimate warning — elements of []*int can be nil.
	// But the warning should be about item, not about the slice itself.
	for _, f := range findings {
		require.NotContains(t, f.Message, "nil dereference of items",
			"should flag element deref, not the range slice itself")
	}
}

// ---------------------------------------------------------------------------
// Category 18: Definitely-nil slice ranging — still safe (no-op)
// ---------------------------------------------------------------------------

func TestCorrect_RangeOverDefinitelyNilSlice_IsNoOp(t *testing.T) {
	t.Parallel()

	// Even a definitely-nil slice is safe to range over. It's a no-op.
	ssaPkg := buildSSA(t, `
		package example

		func rangeNilSlice() int {
			var items []int
			total := 0
			for _, v := range items {
				total += v
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "rangeNilSlice")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"range over definitely-nil slice is a no-op — should not be flagged")
}

func TestCorrect_RangeOverDefinitelyNilMap_IsNoOp(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func rangeNilMap() int {
			var m map[string]int
			total := 0
			for _, v := range m {
				total += v
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "rangeNilMap")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"range over definitely-nil map is a no-op — should not be flagged")
}
