package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// Global address state tracking tests
//
// These tests verify that stores to package-level globals in init()
// and other functions are tracked across the fixed-point iteration
// and visible when analyzing functions that read those globals.
//
// Note: buildSSA panics on method calls on inline struct types, so
// these tests use pointer dereference (*g) instead of method calls.
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. Global initialized to non-nil — read in another function
// ---------------------------------------------------------------------------

func TestGlobalAddr_InitNonNil_DerefSafe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g is initialized to new(int) in init(). No function sets it to nil.
	// readG() should see g as DefinitelyNotNil → no findings.
	require.Empty(t, findings,
		"global initialized to non-nil in init() should not warn on dereference")
}

// ---------------------------------------------------------------------------
// 2. Global initialized but setter takes MaybeNil param — should warn
// ---------------------------------------------------------------------------

func TestGlobalAddr_InitNonNil_SetterWithParam_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func SetG(v *int) {
			g = v
		}

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g initialized to non-nil, but SetG(v) stores v which is MaybeNil
	// (exported param, no callers visible). Join = MaybeNil → warn.
	require.NotEmpty(t, findings,
		"global with setter that accepts MaybeNil param should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning, not Bug")
	}
}

// ---------------------------------------------------------------------------
// 3. Global initialized, unexported setter called with non-nil only
// ---------------------------------------------------------------------------

func TestGlobalAddr_InitNonNil_SetterCalledNonNil_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func setG(v *int) {
			g = v
		}

		func setup() {
			x := 99
			setG(&x)
		}

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g initialized to non-nil. setG is unexported, only called with &x
	// (non-nil). Ideally all stores to g are non-nil → safe.
	// However, the fixed-point loop collects global states before param
	// states converge, so on iteration 1 setG's param v is MaybeNil,
	// making the store MaybeNil. The changed flag only tracks param
	// changes, not global state changes, so a second pass doesn't
	// re-collect globals with the refined param state.
	// TODO: track global state changes in the convergence check to
	// resolve this interaction between param and global analysis.
	require.NotEmpty(t, findings,
		"currently warns due to param/global convergence ordering")
}

// ---------------------------------------------------------------------------
// 4. Global zero value (nil pointer) — should warn
// ---------------------------------------------------------------------------

func TestGlobalAddr_ZeroValue_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g is zero-value nil. No stores set it to non-nil. Should warn.
	require.NotEmpty(t, findings,
		"global with zero value (nil) should warn on dereference")
}

// ---------------------------------------------------------------------------
// 5. Multiple globals — mixed nil and non-nil
// ---------------------------------------------------------------------------

func TestGlobalAddr_MultipleGlobals_Mixed(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var good *int = new(int)
		var bad *int

		func readGood() int {
			return *good
		}

		func readBad() int {
			return *bad
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	goodFn := findSSAFunc(t, ssaPkg, "readGood")
	goodFindings := analyzer.Analyze(goodFn)
	require.Empty(t, goodFindings,
		"good global initialized to non-nil should not warn")

	badFn := findSSAFunc(t, ssaPkg, "readBad")
	badFindings := analyzer.Analyze(badFn)
	require.NotEmpty(t, badFindings,
		"bad global zero value should warn")
}

// ---------------------------------------------------------------------------
// 6. Global reassigned to nil in another function — should warn
// ---------------------------------------------------------------------------

func TestGlobalAddr_ReassignedToNil_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func clearG() {
			g = nil
		}

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// init() sets g = new(int) (non-nil), but clearG() sets g = nil.
	// Join = MaybeNil. readG() should warn.
	require.NotEmpty(t, findings,
		"global reassigned to nil in another function should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"should be Warning (MaybeNil from join), not Bug")
	}
}

// ---------------------------------------------------------------------------
// 7. Global with nil check before use — should be safe
// ---------------------------------------------------------------------------

func TestGlobalAddr_NilCheckBeforeUse_Safe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int

		func readG() int {
			if g != nil {
				return *g
			}
			return 0
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g is nil by default, but readG() checks before use.
	// Branch refinement should prove the dereference safe.
	require.Empty(t, findings,
		"nil-checked global dereference should be safe")
}

// ---------------------------------------------------------------------------
// 8. Global set in non-init function — no init store
// ---------------------------------------------------------------------------

func TestGlobalAddr_SetInSetup_NoInit(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int

		func setup() {
			x := 42
			g = &x
		}

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g starts as nil (zero value). setup() stores non-nil.
	// But readG() could be called before setup().
	// The analysis should not produce Bug — at worst Warning.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug — global might be set before use")
	}
}

// ---------------------------------------------------------------------------
// 9. Regression: single-pred refinement still works
// ---------------------------------------------------------------------------

func TestGlobalAddr_Regression_SinglePredRefinement(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func earlyReturn(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "earlyReturn")
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"single-pred refinement must still work with global addr state")
}

// ---------------------------------------------------------------------------
// 10. Regression: param analysis still works
// ---------------------------------------------------------------------------

func TestGlobalAddr_Regression_ParamAnalysis(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func helper(p *int) int {
			return *p
		}

		func caller() int {
			x := 42
			return helper(&x)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "helper")
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"param analysis should still work alongside global addr state")
}

// ---------------------------------------------------------------------------
// 11. Global and param interaction — function reads global and takes param
// ---------------------------------------------------------------------------

func TestGlobalAddr_GlobalAndParam_Combined(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func combine(p *int) int {
			return *g + *p
		}

		func caller() int {
			x := 10
			return combine(&x)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "combine")
	findings := analyzer.Analyze(fn)

	// g is non-nil (init), p is non-nil (caller passes &x). No warnings.
	require.Empty(t, findings,
		"both global (non-nil) and param (non-nil from caller) should be safe")
}

// ---------------------------------------------------------------------------
// 12. Global explicit nil init — should warn or bug
// ---------------------------------------------------------------------------

func TestGlobalAddr_ExplicitNilInit_Warns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = nil

		func readG() int {
			return *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "readG")
	findings := analyzer.Analyze(fn)

	// g is explicitly nil. Should produce a finding.
	require.NotEmpty(t, findings,
		"global explicitly set to nil should produce a finding")
}
