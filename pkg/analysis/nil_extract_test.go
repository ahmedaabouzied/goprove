package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// Extract instruction tests
//
// These tests verify that transferExtractInstr correctly propagates nil state
// from multi-return function summaries to extracted values. This is the fix
// for the largest category of false positives (nil_check_pattern, ~59% of FPs).
//
// SSA representation of multi-return:
//   t0 = Call foo(args)          → tuple type (T1, T2, ...)
//   t1 = Extract t0 #0          → T1
//   t2 = Extract t0 #1          → T2
//
// The analyzer must index into the callee's summary.Returns[] using v.Index
// to assign the correct nil state to each extracted value.
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. Two-return function: (*T, error) — the most common Go pattern
// ---------------------------------------------------------------------------

func TestExtract_TwoReturn_NonNilFirstReturn(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func newInt(x int) (*int, error) {
			v := x
			return &v, nil
		}

		func useNewInt() int {
			p, err := newInt(42)
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useNewInt")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// newInt always returns (&v, nil).
	// Summary.Returns[0] = DefinitelyNotNil.
	// Extract #0 (p) should be DefinitelyNotNil → no warning.
	require.Empty(t, findings,
		"p is always non-nil — Extract #0 should propagate DefinitelyNotNil from summary")
}

func TestExtract_TwoReturn_AlwaysNilFirstReturn(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "errors"

		func alwaysFails() (*int, error) {
			return nil, errors.New("always fails")
		}

		func useAlwaysFails() int {
			p, _ := alwaysFails()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useAlwaysFails")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// alwaysFails always returns (nil, err).
	// Extract #0 (p) should be DefinitelyNil → Bug.
	require.NotEmpty(t, findings, "should flag deref of always-nil return")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "deref of always-nil first return should be Bug severity")
}

func TestExtract_TwoReturn_MaybeNilFirstReturn(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeParse(s string) (*int, error) {
			if s == "" {
				return nil, nil
			}
			x := 42
			return &x, nil
		}

		func useMaybeParse(s string) int {
			p, _ := maybeParse(s)
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useMaybeParse")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// maybeParse returns (nil, nil) or (&x, nil).
	// Summary.Returns[0] = MaybeNil.
	// Extract #0 (p) should be MaybeNil → Warning, not Bug.
	require.NotEmpty(t, findings, "should warn about possibly-nil return")
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"MaybeNil return should be Warning, not Bug")
	}
}

func TestExtract_TwoReturn_MaybeNilWithNilCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeParse(s string) (*int, error) {
			if s == "" {
				return nil, nil
			}
			x := 42
			return &x, nil
		}

		func useSafely(s string) int {
			p, _ := maybeParse(s)
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useSafely")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// MaybeNil return + nil check before use → safe.
	require.Empty(t, findings,
		"nil-checked MaybeNil extract should produce no findings")
}

// ---------------------------------------------------------------------------
// 2. Three-return function: (T1, T2, error)
// ---------------------------------------------------------------------------

func TestExtract_ThreeReturn_AllPositions(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func compute(x int) (*int, *int, error) {
			a := x
			b := x * 2
			return &a, &b, nil
		}

		func useCompute() int {
			a, b, err := compute(10)
			if err != nil {
				return 0
			}
			return *a + *b
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useCompute")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// compute returns (&a, &b, nil) — all three returns known.
	// Extract #0 (a) = DefinitelyNotNil
	// Extract #1 (b) = DefinitelyNotNil
	// Extract #2 (err) = DefinitelyNil
	require.Empty(t, findings,
		"all three extracted values should have correct nil state from summary")
}

func TestExtract_ThreeReturn_MixedNilStates(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "errors"

		func connect(addr string) (*int, *int, error) {
			if addr == "" {
				return nil, nil, errors.New("empty addr")
			}
			a := 1
			return &a, nil, nil
		}

		func useConnect(addr string) {
			conn, meta, err := connect(addr)
			if err != nil {
				return
			}
			_ = conn
			_ = meta
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useConnect")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// connect returns (&a, nil, nil) or (nil, nil, err).
	// Extract #0 (conn) = MaybeNil (non-nil on success, nil on error)
	// Extract #1 (meta) = DefinitelyNil (always nil)
	// conn and meta are not dereferenced, so no findings expected.
	require.Empty(t, findings,
		"no dereferences → no findings regardless of nil state")
}

func TestExtract_ThreeReturn_DerefMixedStates(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "errors"

		func connect(addr string) (*int, *int, error) {
			if addr == "" {
				return nil, nil, errors.New("empty addr")
			}
			a := 1
			b := 2
			return &a, &b, nil
		}

		func useUnsafely(addr string) int {
			a, b, _ := connect(addr)
			return *a + *b
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useUnsafely")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// connect: Returns[0] = MaybeNil, Returns[1] = MaybeNil.
	// Both are dereferenced without nil check → Warning (not Bug).
	require.NotEmpty(t, findings, "should warn about unchecked MaybeNil extracts")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity,
			"MaybeNil extract deref should be Warning, not Bug")
	}
}

// ---------------------------------------------------------------------------
// 3. Single-return function — Extract should not appear, but verify no panic
// ---------------------------------------------------------------------------

func TestExtract_SingleReturn_NoExtract(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func single() *int {
			x := 42
			return &x
		}

		func useSingle() int {
			p := single()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useSingle")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Single-return uses direct Call value, not Extract.
	// transferCall handles this. Verify it still works.
	require.Empty(t, findings,
		"single non-nil return should produce no warnings")
}

// ---------------------------------------------------------------------------
// 4. Non-call tuple producers (TypeAssert CommaOk, Lookup CommaOk)
// ---------------------------------------------------------------------------

func TestExtract_TypeAssertCommaOk_FallsBackToMaybeNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func stringify(x interface{}) string {
			if s, ok := x.(*int); ok && s != nil {
				return "got int"
			}
			return ""
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "stringify")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// TypeAssert with CommaOk produces a tuple.
	// Extract #0 is the asserted value, Extract #1 is the bool.
	// The ok+nil check guards the use of s — should be safe.
	require.Empty(t, findings,
		"type assertion with ok check should not produce warnings")
}

func TestExtract_MapLookupCommaOk_FallsBackToMaybeNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookup(m map[string]*int, key string) int {
			if v, ok := m[key]; ok && v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "lookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Lookup with CommaOk produces a tuple, not a Call.
	// transferExtractInstr falls back to MaybeNil.
	// The ok && v != nil guard makes it safe via branch refinement.
	require.Empty(t, findings,
		"map lookup with ok+nil check should not produce warnings")
}

// ---------------------------------------------------------------------------
// 5. Interface dispatch (nil callee) — indirect call through interface
// ---------------------------------------------------------------------------

func TestExtract_IndirectCall_NilCallee(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func doCall(fn func(string) (*int, error), s string) int {
			val, err := fn(s)
			if err != nil {
				return 0
			}
			if val != nil {
				return *val
			}
			return -1
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "doCall")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Indirect call via function pointer — StaticCallee() is nil.
	// Extract falls back to MaybeNil for both #0 and #1.
	// val is nil-checked before deref → safe.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"indirect call extract should not produce Bug")
	}
}

// ---------------------------------------------------------------------------
// 6. Stdlib multi-return functions
// ---------------------------------------------------------------------------

func TestExtract_StdlibMultiReturn_OsOpen(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "os"

		func readFile(path string) ([]byte, error) {
			f, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			buf := make([]byte, 1024)
			n, err := f.Read(buf)
			if err != nil {
				f.Close()
				return nil, err
			}
			f.Close()
			return buf[:n], nil
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "readFile")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// os.Open returns (*File, error).
	// With Extract handling, f gets its nil state from the summary.
	// After err != nil guard, f is used — should be safe or at worst Warning.
	for _, f := range findings {
		require.NotEqual(t, analysis.Bug, f.Severity,
			"os.Open result after error check should not be Bug")
	}
}

func TestExtract_StdlibMultiReturn_FmtSscanf(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "fmt"

		func parseNum(s string) (int, bool) {
			var x int
			n, err := fmt.Sscanf(s, "%d", &x)
			if err != nil || n != 1 {
				return 0, false
			}
			return x, true
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "parseNum")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// fmt.Sscanf returns (int, error) — int is non-nillable.
	// err is checked. No dereferences of nillable values.
	require.Empty(t, findings,
		"fmt.Sscanf multi-return with error check should be clean")
}

// ---------------------------------------------------------------------------
// 7. Multi-return with callee in same package (interprocedural)
// ---------------------------------------------------------------------------

func TestExtract_SamePackageCallee_TwoReturns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		type DB struct{}

		func openDB(dsn string) (*DB, error) {
			if dsn == "" {
				return nil, nil
			}
			return &DB{}, nil
		}

		func setup(dsn string) *DB {
			db, err := openDB(dsn)
			if err != nil {
				return nil
			}
			if db == nil {
				return nil
			}
			return db
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "setup")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// openDB returns MaybeNil for both positions.
	// setup checks both err and db before using db → safe.
	require.Empty(t, findings,
		"double-checked multi-return from same-package callee should be safe")
}

func TestExtract_SamePackageCallee_ChainedMultiReturn(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func step1(x int) (*int, error) {
			v := x
			return &v, nil
		}

		func step2(p *int) (*int, error) {
			v := *p * 2
			return &v, nil
		}

		func pipeline(x int) int {
			a, err := step1(x)
			if err != nil {
				return 0
			}
			b, err := step2(a)
			if err != nil {
				return 0
			}
			return *b
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "pipeline")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Both step1 and step2 always return non-nil first values.
	// Extract #0 from each call gets DefinitelyNotNil from summary.
	require.Empty(t, findings,
		"chained multi-return calls with always-non-nil returns should be clean")
}

// ---------------------------------------------------------------------------
// 8. Edge cases
// ---------------------------------------------------------------------------

func TestExtract_CalleePanics_NoReturnValues(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func mustParse(s string) (*int, error) {
			if s == "" {
				panic("empty string")
			}
			x := 42
			return &x, nil
		}

		func use(s string) int {
			p, err := mustParse(s)
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "use")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// mustParse has one return path: (&x, nil). The panic path has no Return.
	// Summary.Returns[0] should be DefinitelyNotNil.
	require.Empty(t, findings,
		"callee with panic+single non-nil return should be safe")
}

func TestExtract_RecursiveCallee(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func recParse(s string, depth int) (*int, error) {
			if depth > 10 {
				return nil, nil
			}
			if s == "" {
				return recParse("default", depth+1)
			}
			x := 42
			return &x, nil
		}

		func useRecursive(s string) int {
			p, err := recParse(s, 0)
			if err != nil {
				return 0
			}
			if p != nil {
				return *p
			}
			return -1
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useRecursive")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Recursive callee — sentinel summary (MaybeNil) breaks the cycle.
	// p is nil-checked before deref → safe.
	require.Empty(t, findings,
		"recursive callee with nil check should produce no findings")
}

func TestExtract_MultipleCallSitesSameFunction(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func lookup(key string) (*int, bool) {
			if key == "magic" {
				x := 42
				return &x, true
			}
			return nil, false
		}

		func useMultiple() int {
			a, ok1 := lookup("magic")
			b, ok2 := lookup("other")
			total := 0
			if ok1 && a != nil {
				total += *a
			}
			if ok2 && b != nil {
				total += *b
			}
			return total
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useMultiple")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Same function called twice. Each Extract gets the same summary.
	// Both values nil-checked before deref → safe.
	require.Empty(t, findings,
		"multiple calls to same function, both nil-checked, should be clean")
}

// ---------------------------------------------------------------------------
// 9. Regression: existing single-return patterns still work
// ---------------------------------------------------------------------------

func TestExtract_Regression_SingleReturnCallStillWorks(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func alwaysNil() *int {
			return nil
		}

		func derefNil() int {
			p := alwaysNil()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "derefNil")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	// Single return — handled by transferCall, not transferExtractInstr.
	// Must still detect the Bug.
	require.NotEmpty(t, findings, "single-return always-nil should still be detected")
	hasBug := false
	for _, f := range findings {
		if f.Severity == analysis.Bug {
			hasBug = true
		}
	}
	require.True(t, hasBug, "single-return always-nil deref must be Bug")
}

func TestExtract_Regression_NonNilSingleReturn(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func newInt() *int {
			x := 0
			return &x
		}

		func useNewInt() int {
			p := newInt()
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useNewInt")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"single non-nil return deref should still be clean")
}

func TestExtract_Regression_BranchRefinementStillWorks(t *testing.T) {
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
		"branch refinement regression — early return guard must still work")
}

func TestExtract_Regression_AllocNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useAlloc() int {
			p := new(int)
			*p = 42
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useAlloc")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"new() produces DefinitelyNotNil — must still work")
}

func TestExtract_Regression_MethodReceiverNonNil(t *testing.T) {
	t.Parallel()

	// Method receiver regression is already covered by existing tests
	// in nil_analyze_false_positive_test.go. Here we verify that the
	// Extract changes didn't break the guarded-param pattern.
	ssaPkg := buildSSA(t, `
		package example

		func guarded(p *int) int {
			if p == nil {
				return -1
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "guarded")

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"guarded param deref must still work after Extract changes")
}

// ---------------------------------------------------------------------------
// 10. Param analysis + Extract interaction
// ---------------------------------------------------------------------------

func TestExtract_WithParamAnalysis_CallerPassesNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func find(id int) (*int, error) {
			x := id
			return &x, nil
		}

		func handler(p *int) int {
			val, err := find(*p)
			if err != nil {
				return 0
			}
			return *val
		}

		func main() {
			x := 1
			_ = handler(&x)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	analyzer.SetParamNilStates(states)

	fn := findSSAFunc(t, ssaPkg, "handler")
	findings := analyzer.Analyze(fn)

	// main passes &x (DefinitelyNotNil) to handler.
	// find always returns (&x, nil).
	// val = Extract #0 = DefinitelyNotNil.
	require.Empty(t, findings,
		"param analysis + extract should combine to eliminate warnings")
}
