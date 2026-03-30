package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// param_nil.go tests
// Tests for ComputeParamNilStates, ComputeParamNilStatesAnalysis,
// collectCallSites, classifyArg, nilStatesEqual, and States.
// ===========================================================================

// ---------------------------------------------------------------------------
// classifyArg tests (via ComputeParamNilStates)
// ---------------------------------------------------------------------------

func TestParamNilStates_SingleCallerNonNil(t *testing.T) {
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

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	helperFn := findSSAFunc(t, ssaPkg, "helper")
	paramStates := states.States()[helperFn]

	// &x is Alloc → DefinitelyNotNil. Single caller passes non-nil.
	require.NotEmpty(t, paramStates, "should have param states for helper")
	// NilState 2 = DefinitelyNotNil
	require.Equal(t, analysis.DefinitelyNotNil, paramStates[0],
		"single caller passes &x which is DefinitelyNotNil")
}

func TestParamNilStates_SingleCallerNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func helper(p *int) int {
			if p != nil {
				return *p
			}
			return 0
		}

		func caller() int {
			return helper(nil)
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	helperFn := findSSAFunc(t, ssaPkg, "helper")
	paramStates := states.States()[helperFn]

	require.NotEmpty(t, paramStates)
	require.Equal(t, analysis.DefinitelyNil, paramStates[0],
		"single caller passes nil literal")
}

func TestParamNilStates_MultipleCallersMixed(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func helper(p *int) int {
			if p != nil {
				return *p
			}
			return 0
		}

		func caller1() int {
			x := 42
			return helper(&x)
		}

		func caller2() int {
			return helper(nil)
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	helperFn := findSSAFunc(t, ssaPkg, "helper")
	paramStates := states.States()[helperFn]

	require.NotEmpty(t, paramStates)
	// Join(DefinitelyNotNil, DefinitelyNil) = MaybeNil
	require.Equal(t, analysis.MaybeNil, paramStates[0],
		"mixed callers: one passes non-nil, one passes nil → MaybeNil")
}

func TestParamNilStates_MultipleCallersAllNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func helper(p *int) int {
			return *p
		}

		func caller1() int {
			x := 1
			return helper(&x)
		}

		func caller2() int {
			y := 2
			return helper(&y)
		}

		func caller3() int {
			z := new(int)
			return helper(z)
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	helperFn := findSSAFunc(t, ssaPkg, "helper")
	paramStates := states.States()[helperFn]

	require.NotEmpty(t, paramStates)
	require.Equal(t, analysis.DefinitelyNotNil, paramStates[0],
		"all callers pass non-nil → DefinitelyNotNil")
}

func TestParamNilStates_NoCallers(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func NoCaller(p *int) int {
			return *p
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	fn := findSSAFunc(t, ssaPkg, "NoCaller")
	paramStates := states.States()[fn]

	// No call sites → not in the map → params default to MaybeNil.
	require.Empty(t, paramStates,
		"no callers → no param states in map")
}

func TestParamNilStates_NonNillableParam(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func intHelper(x int) int {
			return x + 1
		}

		func caller() int {
			return intHelper(42)
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	fn := findSSAFunc(t, ssaPkg, "intHelper")
	paramStates := states.States()[fn]

	// int param → classifyArg returns DefinitelyNotNil (non-nillable).
	if len(paramStates) > 0 {
		require.Equal(t, analysis.DefinitelyNotNil, paramStates[0])
	}
}

func TestParamNilStates_MakeProducers(t *testing.T) {
	t.Parallel()

	// Use parameter-based sizes to prevent SSA from optimizing away MakeSlice.
	ssaPkg := buildSSA(t, `
		package example

		func useSlice(s []int) int { return len(s) }
		func useMap(m map[string]int) int { return len(m) }
		func useChan(ch chan int) { ch <- 1 }

		func caller(n int) {
			useSlice(make([]int, n))
			useMap(make(map[string]int))
			useChan(make(chan int))
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	for _, name := range []string{"useSlice", "useMap", "useChan"} {
		fn := findSSAFunc(t, ssaPkg, name)
		paramStates := states.States()[fn]
		if len(paramStates) > 0 {
			require.Equal(t, analysis.DefinitelyNotNil, paramStates[0],
				"%s: make() argument should be DefinitelyNotNil", name)
		}
	}
}

func TestParamNilStates_MultipleParams(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func twoParams(a *int, b *int) int {
			return *a + *b
		}

		func caller() int {
			x := 1
			return twoParams(&x, nil)
		}
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	require.NotNil(t, states)

	fn := findSSAFunc(t, ssaPkg, "twoParams")
	paramStates := states.States()[fn]

	require.Len(t, paramStates, 2)
	require.Equal(t, analysis.DefinitelyNotNil, paramStates[0],
		"first param: &x is DefinitelyNotNil")
	require.Equal(t, analysis.DefinitelyNil, paramStates[1],
		"second param: nil literal is DefinitelyNil")
}

// ---------------------------------------------------------------------------
// ComputeParamNilStatesAnalysis (iterative, uses converged state)
// ---------------------------------------------------------------------------

func TestParamNilStatesAnalysis_NilGuardPropagation(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func Init(config *int) {
			if config == nil {
				return
			}
			helper(config)
		}

		func helper(config *int) int {
			return *config
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	require.NotNil(t, states)

	fn := findSSAFunc(t, ssaPkg, "helper")
	paramStates := states.States()[fn]

	// Init checks config != nil before calling helper(config).
	// The iterative analysis should see config as DefinitelyNotNil at the call site.
	require.NotEmpty(t, paramStates)
	require.Equal(t, analysis.DefinitelyNotNil, paramStates[0],
		"caller nil-guards config before passing → DefinitelyNotNil")
}

func TestParamNilStatesAnalysis_WithParamStates(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func Init(config *int) {
			if config == nil {
				return
			}
			helper(config)
		}

		func helper(config *int) int {
			return *config
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	// Now analyze helper with param states — should have 0 findings.
	fn := findSSAFunc(t, ssaPkg, "helper")
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings,
		"helper with non-nil param state should have no warnings")
}

func TestParamNilStatesAnalysis_NilPackageSkipped(t *testing.T) {
	t.Parallel()

	// Should not panic on nil packages.
	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{nil, nil}, analyzer,
	)
	require.NotNil(t, states)
}

func TestParamNilStatesAnalysis_EmptyPackage(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	require.NotNil(t, states)
	require.Empty(t, states.States())
}

func TestParamNilStatesAnalysis_RecursiveFunction(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func recurse(n int, p *int) int {
			if n <= 0 {
				return *p
			}
			return recurse(n-1, p)
		}

		func caller() int {
			x := 42
			return recurse(5, &x)
		}
	`)

	// Should not stack overflow or hang.
	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	require.NotNil(t, states)
}

// ---------------------------------------------------------------------------
// States() getter
// ---------------------------------------------------------------------------

func TestParamNilStates_StatesGetter(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func f(p *int) int { return *p }
		func g() int { return f(new(int)) }
	`)

	states := analysis.ComputeParamNilStates(nil, []*ssa.Package{ssaPkg}, nil)
	m := states.States()
	require.NotNil(t, m, "States() should return non-nil map")
}
