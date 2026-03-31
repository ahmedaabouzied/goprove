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

func TestGlobalAddr_InitNonNil_SetterCalledNonNil_Safe(t *testing.T) {
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
	// (non-nil). The fixed-point loop tracks both param and global state
	// changes, so after param analysis refines setG's v to DefinitelyNotNil,
	// the global state is re-collected and g becomes DefinitelyNotNil.
	require.Empty(t, findings,
		"global only stored with non-nil values should not warn")
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

// ---------------------------------------------------------------------------
// 13. Convergence: setter with nil guard — param refined then global refined
// ---------------------------------------------------------------------------

func TestGlobalAddr_Convergence_SetterWithNilGuard(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func setG(v *int) {
			if v != nil {
				g = v
			}
		}

		func caller() {
			setG(nil)
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

	// setG guards with if v != nil before storing. Even though caller
	// passes nil, the store only executes when v is non-nil.
	// init stores non-nil, setG stores non-nil (guarded). g is safe.
	require.Empty(t, findings,
		"global stored only through nil-guarded setter should be safe")
}

// ---------------------------------------------------------------------------
// 14. Multiple setters — one nil, one non-nil
// ---------------------------------------------------------------------------

func TestGlobalAddr_MultipleSetters_MixedStores(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func setNonNil() {
			x := 1
			g = &x
		}

		func setNil() {
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

	// One function stores non-nil, another stores nil.
	// Join = MaybeNil → should warn.
	require.NotEmpty(t, findings,
		"global with mixed nil/non-nil stores should warn")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity)
	}
}

// ---------------------------------------------------------------------------
// 15. Global read in multiple functions — all should see same state
// ---------------------------------------------------------------------------

func TestGlobalAddr_MultipleReaders(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func read1() int { return *g }
		func read2() int { return *g }
		func read3() int { return *g }
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	for _, name := range []string{"read1", "read2", "read3"} {
		fn := findSSAFunc(t, ssaPkg, name)
		findings := analyzer.Analyze(fn)
		require.Empty(t, findings,
			"%s: all readers of non-nil global should be safe", name)
	}
}

// ---------------------------------------------------------------------------
// 16. Global stored conditionally — both branches store
// ---------------------------------------------------------------------------

func TestGlobalAddr_ConditionalStore_BothBranches(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int

		func initG(cond bool) {
			if cond {
				x := 1
				g = &x
			} else {
				y := 2
				g = &y
			}
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

	// Both branches of initG store non-nil. All stores are non-nil.
	// But g starts as zero-value nil and readG could be called before initG.
	// The analysis sees stores from initG (both non-nil) but no init() store.
	// Since there's no init() store, g's default is MaybeNil (no entry).
	// initG stores are non-nil, so global state = DefinitelyNotNil.
	// However readG could be called when g hasn't been set yet.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug")
	}
}

// ---------------------------------------------------------------------------
// 17. Global stored in a loop — non-nil every iteration
// ---------------------------------------------------------------------------

func TestGlobalAddr_StoreInLoop(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func updateLoop(vals []*int) {
			for _, v := range vals {
				g = v
			}
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

	// updateLoop stores v (slice element, MaybeNil) to g in a loop.
	// init stores non-nil. Join = MaybeNil → warn.
	require.NotEmpty(t, findings,
		"global stored with MaybeNil slice elements should warn")
}

// ---------------------------------------------------------------------------
// 18. Two globals — one read safe, other read after clear
// ---------------------------------------------------------------------------

func TestGlobalAddr_TwoGlobals_IndependentTracking(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var a *int = new(int)
		var b *int = new(int)

		func clearB() {
			b = nil
		}

		func readA() int { return *a }
		func readB() int { return *b }
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	aFn := findSSAFunc(t, ssaPkg, "readA")
	aFindings := analyzer.Analyze(aFn)
	require.Empty(t, aFindings,
		"a is never cleared — should be safe")

	bFn := findSSAFunc(t, ssaPkg, "readB")
	bFindings := analyzer.Analyze(bFn)
	require.NotEmpty(t, bFindings,
		"b is cleared by clearB — should warn")
}

// ---------------------------------------------------------------------------
// 19. Chain: init → setter → reader with param convergence
// ---------------------------------------------------------------------------

func TestGlobalAddr_Convergence_ChainedCalls(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int

		func initG() {
			x := 42
			setG(&x)
		}

		func setG(v *int) {
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

	// initG calls setG(&x) — non-nil. setG's param v is DefinitelyNotNil
	// after param analysis. Store to g is non-nil.
	// g has no zero-value init store (var g *int has no init store in SSA).
	// Only store is from setG with non-nil → g = DefinitelyNotNil.
	// But readG could be called before initG. No init() store to g.
	// So the default for g (no entry in globalStates) → MaybeNil.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"should not be Bug — g might not be initialized yet")
	}
}

// ---------------------------------------------------------------------------
// 20. Regression: deref after nil check on param (not global) still works
// ---------------------------------------------------------------------------

func TestGlobalAddr_Regression_ParamNilCheckSafe(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func process(p *int) int {
			if p == nil {
				return *g
			}
			return *p + *g
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)

	fn := findSSAFunc(t, ssaPkg, "process")
	findings := analyzer.Analyze(fn)

	// g is non-nil (init), p is checked before deref.
	// Both paths should be safe.
	require.Empty(t, findings,
		"param nil check + non-nil global should produce no findings")
}

// ---------------------------------------------------------------------------
// 21. Regression: global used as function argument
// ---------------------------------------------------------------------------

func TestGlobalAddr_PassedAsArg(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		var g *int = new(int)

		func helper(p *int) int {
			return *p
		}

		func caller() int {
			return helper(g)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "helper")
	findings := analyzer.Analyze(fn)

	// g is non-nil, passed to helper. The UnOp transfer writes the
	// global's addr state into the value state for the load register,
	// making it visible to param analysis at the call site.
	require.Empty(t, findings,
		"passing non-nil global as argument should make param safe")
}
