package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// Correctness tests
//
// These tests define the CORRECT expected behavior. If a test fails, it
// indicates a bug in the implementation that needs to be fixed.
// Tests are organized by category of potential bug.
// ===========================================================================

// ---------------------------------------------------------------------------
// Soundness: Bug findings must be real
// If we say "Bug", the code MUST crash at runtime.
// ---------------------------------------------------------------------------

func TestSoundness_NilLiteralDerefIsBug(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() int {
			var p *int
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Len(t, findings, 1)
	require.Equal(t, analysis.Bug, findings[0].Severity,
		"dereferencing nil literal MUST be Bug, not Warning")
}

func TestSoundness_ExplicitNilAssignDerefIsBug(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() int {
			var p *int = nil
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Len(t, findings, 1)
	require.Equal(t, analysis.Bug, findings[0].Severity)
}

func TestSoundness_DivByZeroLiteralIsBug(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(x int) int {
			zero := 0
			return x / zero
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewAnalyzer(nil).Analyze(fn)
	require.NotEmpty(t, findings)
	require.Equal(t, analysis.Bug, findings[0].Severity,
		"division by proven zero MUST be Bug")
}

// ---------------------------------------------------------------------------
// Safety: no false bugs
// If we say "Safe" (no finding), the code must NOT crash for the tracked patterns.
// ---------------------------------------------------------------------------

func TestSafety_NilCheckGuardIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(p *int) int {
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "nil-checked deref MUST be safe")
}

func TestSafety_EarlyReturnGuardIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(p *int) int {
			if p == nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "early return guard MUST be safe")
}

func TestSafety_NewIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() int {
			p := new(int)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "new() always returns non-nil")
}

func TestSafety_AddressOfIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() int {
			x := 42
			p := &x
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "&x always returns non-nil")
}

func TestSafety_MakeSliceIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(n int) []int {
			s := make([]int, n)
			return s
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "make([]T) always returns non-nil")
}

func TestSafety_MakeMapIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() map[string]int {
			return make(map[string]int)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "make(map) always returns non-nil")
}

func TestSafety_MakeChanIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() chan int {
			return make(chan int)
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "make(chan) always returns non-nil")
}

func TestSafety_DivByNonZeroConstIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(x int) int {
			return x / 10
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewAnalyzer(nil).Analyze(fn)
	require.Empty(t, findings, "division by non-zero constant MUST be safe")
}

func TestSafety_DivAfterNonZeroCheckIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(x, y int) int {
			if y != 0 {
				return x / y
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewAnalyzer(nil).Analyze(fn)
	require.Empty(t, findings, "division after != 0 guard MUST be safe")
}

// ---------------------------------------------------------------------------
// Interprocedural correctness
// ---------------------------------------------------------------------------

func TestInterprocedural_CalleeReturnsNonNilIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func makePtr() *int {
			return new(int)
		}
		func f() int {
			p := makePtr()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings,
		"callee that always returns new() → deref is safe")
}

func TestInterprocedural_CalleeReturnsNilIsBugOrWarning(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func returnNil() *int {
			return nil
		}
		func f() int {
			p := returnNil()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.NotEmpty(t, findings,
		"callee that returns nil → deref MUST produce finding")
}

func TestInterprocedural_CalleeReturnCheckedIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func maybeNil(b bool) *int {
			if b { return new(int) }
			return nil
		}
		func f() int {
			p := maybeNil(true)
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings,
		"callee return checked before use → safe")
}

// ---------------------------------------------------------------------------
// Phi node correctness
// ---------------------------------------------------------------------------

func TestPhi_BothBranchesNonNilIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(b bool) int {
			var p *int
			if b {
				x := 1
				p = &x
			} else {
				y := 2
				p = &y
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings,
		"phi with both edges non-nil → safe")
}

func TestPhi_OneBranchNilIsWarning(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(b bool) int {
			var p *int
			if b {
				x := 1
				p = &x
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.NotEmpty(t, findings)
	require.Equal(t, analysis.Warning, findings[0].Severity,
		"phi with one nil edge → Warning, not Bug")
}

func TestPhi_AllBranchesNilIsBug(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(b bool) int {
			var p *int
			if b {
				p = nil
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.NotEmpty(t, findings)
	require.Equal(t, analysis.Bug, findings[0].Severity,
		"phi with all nil edges → Bug")
}

// ---------------------------------------------------------------------------
// Address model correctness
// ---------------------------------------------------------------------------

func TestAddr_FieldReloadAfterNilCheckIsSafe(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "AddrFieldReload" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	// Only the param 'o' should warn. o.In after nil check should NOT.
	for _, f := range findings {
		require.NotContains(t, f.Message, "nil dereference of nil pointer",
			"field reload after nil check must not be Bug")
	}
}

func TestAddr_GlobalNilCheckIsSafe(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "AddrGlobalFieldReload" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	// Global is nil-checked. No Bug findings should exist.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"nil-checked global must not produce Bug")
	}
}

// ---------------------------------------------------------------------------
// Interface invoke correctness
// ---------------------------------------------------------------------------

func TestInvoke_UncheckedInterfaceIsWarning(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "MethodCallOnParam" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.NotEmpty(t, findings,
		"interface invoke on unchecked param MUST produce warning")
	require.Equal(t, analysis.Warning, findings[0].Severity)
}

func TestInvoke_CheckedInterfaceIsSafe(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "RefineInterface" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings,
		"interface invoke after nil check MUST be safe")
}

// ---------------------------------------------------------------------------
// Convert correctness
// ---------------------------------------------------------------------------

func TestConvert_PropagatesNilState(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		import "unsafe"
		func f(b []byte) string {
			return *(*string)(unsafe.Pointer(&b))
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings,
		"unsafe.Pointer(&b) → Convert chain should be non-nil")
}

// ---------------------------------------------------------------------------
// Method receiver correctness
// ---------------------------------------------------------------------------

func TestReceiver_AssumedNonNil(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	// Find any method with a pointer receiver that dereferences self.
	prog, _, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	_ = prog

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if !ok || fn.Signature.Recv() == nil {
			continue
		}
		findings := analyzer.Analyze(fn)
		for _, f := range findings {
			require.NotContains(t, f.Message, "nil dereference",
				"method receiver should never produce nil warning: %s", fn.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// Deduplication correctness
// ---------------------------------------------------------------------------

func TestDedup_SameVarOnce(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(p *int) int {
			a := *p
			b := *p
			c := *p
			return a + b + c
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Len(t, findings, 1,
		"same variable dereferenced 3 times → 1 finding only")
}

func TestDedup_DifferentVarsEach(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(a, b *int) int {
			return *a + *b
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Len(t, findings, 2,
		"two different variables → 2 findings")
}

// ---------------------------------------------------------------------------
// Loop correctness: must converge and be sound
// ---------------------------------------------------------------------------

func TestLoop_NilCheckInLoopIsSafe(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(ptrs []*int) int {
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
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings, "nil check in loop body → safe")
}

func TestLoop_UncheckedDerefInLoopIsWarning(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(ptrs []*int) int {
			sum := 0
			for i := 0; i < len(ptrs); i++ {
				p := ptrs[i]
				sum += *p
			}
			return sum
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.NotEmpty(t, findings, "unchecked deref in loop → warning")
}

// ---------------------------------------------------------------------------
// Edge cases: should not panic
// ---------------------------------------------------------------------------

func TestEdge_EmptyFunction(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f() {}
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings)
}

func TestEdge_NoPointerOps(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(x, y int) int { return x + y }
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings)
}

func TestEdge_AnalyzerReuse(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func safe() int { p := new(int); return *p }
		func warn(p *int) int { return *p }
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil)

	// First: safe function.
	f1 := analyzer.Analyze(findSSAFunc(t, ssaPkg, "safe"))
	require.Empty(t, f1)

	// Second: warning function. Previous state must not leak.
	f2 := analyzer.Analyze(findSSAFunc(t, ssaPkg, "warn"))
	require.Len(t, f2, 1)

	// Third: safe again. Must reset.
	f3 := analyzer.Analyze(findSSAFunc(t, ssaPkg, "safe"))
	require.Empty(t, f3)
}

func TestEdge_StringIndexNotNilWarning(t *testing.T) {
	t.Parallel()
	ssaPkg := buildSSA(t, `
		package example
		func f(s string, i int) byte { return s[i] }
	`)
	fn := findSSAFunc(t, ssaPkg, "f")
	findings := analysis.NewNilAnalyzer(nil, nil).Analyze(fn)
	require.Empty(t, findings,
		"string indexing must never produce nil dereference warning")
}
