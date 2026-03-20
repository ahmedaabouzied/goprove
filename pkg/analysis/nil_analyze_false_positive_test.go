package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// False positive regression tests
//
// These tests document known false positives and patterns found by running
// goprove on real-world code (go-redis v9.7.3, net/http, production codebases).
//
// Tests marked "KNOWN FALSE POSITIVE" are expected to produce warnings
// that shouldn't exist. When a fix is implemented, update the assertion.
//
// Tests marked "CORRECTLY HANDLED" verify patterns that work today.
// ===========================================================================

// ---------------------------------------------------------------------------
// Category 1: Multi-return value patterns (error check guarantees non-nil)
// ---------------------------------------------------------------------------

func TestFalsePositive_MultiReturn_ErrorCheckGuaranteesNonNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func parse(s string) (*int, error) {
			if s == "" {
				return nil, nil
			}
			x := 42
			return &x, nil
		}

		func useParse(s string) int {
			p, err := parse(s)
			if err != nil {
				return 0
			}
			return *p
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useParse")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: parse() can return (nil, nil) — so when
	// err == nil, p can still be nil. This is actually a correct warning!
	// The multi-return pattern is only safe when the function guarantees
	// non-nil on the first return when err is nil.
	// For now, this warning is legitimate.
	require.NotEmpty(t, findings,
		"parse can return (nil, nil) — warning is correct")
}

// ---------------------------------------------------------------------------
// Category 2: Double pointer load after nil check
//
// When *pp is nil-checked, the deref of pp itself triggers a warning
// because pp is a pointer parameter (MaybeNil).
// ---------------------------------------------------------------------------

func TestFalsePositive_DoublePtrLoadWarnsOnOuterDeref(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func useDoublePtr(pp **int) int {
			p := *pp
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useDoublePtr")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: *pp dereferences pp (the outer pointer),
	// which is an unchecked parameter. The warning is on pp, not on p.
	// Technically correct — pp could be nil — but in practice callers
	// rarely pass nil **int.
	require.Len(t, findings, 1,
		"pp deref produces a warning — pp is an unchecked param")
	require.Contains(t, findings[0].Message, "possible nil dereference")
}

// ---------------------------------------------------------------------------
// Category 3: Type assertion ok pattern — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_TypeAssertionOkPattern(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		type Doer interface {
			Do()
		}

		type MyDoer struct{}
		func (MyDoer) Do() {}

		func tryDoer(x interface{}) {
			if d, ok := x.(*MyDoer); ok {
				d.Do()
			}
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "tryDoer")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: the analyzer produces no false positive here.
	// The type assertion + ok check works because SSA represents this
	// in a way the nil analyzer can track.
	require.Empty(t, findings,
		"type assertion with ok check should be safe")
}

// ---------------------------------------------------------------------------
// Category 4: Map lookup ok pattern — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_MapLookupOkPattern(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func mapLookup(m map[string]*int, key string) int {
			v, ok := m[key]
			if ok && v != nil {
				return *v
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "mapLookup")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: the v != nil check refines v in the true branch.
	// The && creates sequential blocks and the refinement propagates.
	require.Empty(t, findings,
		"map lookup with ok && v != nil should be safe")
}

// ---------------------------------------------------------------------------
// Category 5: Stdlib return value guarantees — CORRECTLY HANDLED
//
// The interprocedural analysis analyzes stdlib function bodies and
// computes return nil summaries. Functions like time.NewTimer and
// bytes.NewBuffer always return non-nil, and the analyzer proves this.
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_StdlibNewTimer(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "time"

		func useTimer() {
			t := time.NewTimer(time.Second)
			t.Stop()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useTimer")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: interprocedural analysis resolves time.NewTimer
	// return value as DefinitelyNotNil by analyzing the stdlib source.
	require.Empty(t, findings,
		"time.NewTimer always returns non-nil — interprocedural analysis proves this")
}

func TestCorrectlyHandled_StdlibBytesNewBuffer(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "bytes"

		func useBuffer() int {
			b := bytes.NewBuffer(nil)
			return b.Len()
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "useBuffer")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: bytes.NewBuffer always returns non-nil.
	require.Empty(t, findings,
		"bytes.NewBuffer always returns non-nil — interprocedural analysis proves this")
}

// ---------------------------------------------------------------------------
// Category 6: String indexing — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_StringIndexNotNil(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func charAt(s string, i int) byte {
			return s[i]
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "charAt")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: strings are value types in Go — cannot be nil.
	require.Empty(t, findings,
		"string indexing should not produce nil dereference warnings")
}

// ---------------------------------------------------------------------------
// Category 7: Unsafe pointer operations
// ---------------------------------------------------------------------------

func TestFalsePositive_UnsafePointerConversion(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		import "unsafe"

		func unsafeConvert(b []byte) string {
			return *(*string)(unsafe.Pointer(&b))
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "unsafeConvert")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// KNOWN FALSE POSITIVE: unsafe.Pointer(&b) is always non-nil because
	// &b takes the address of a local variable. But the analyzer sees
	// the Convert instruction as producing MaybeNil.
	// When fixed (handle *ssa.Convert for unsafe.Pointer), change to:
	//   require.Empty(t, findings)
	require.NotEmpty(t, findings,
		"EXPECTED FALSE POSITIVE: unsafe.Pointer conversion")
}

// ---------------------------------------------------------------------------
// Category 8: Stdlib multi-return with error check (url.Parse pattern)
//
// Cannot use buildSSA for this pattern because struct field access
// (u.Scheme) panics in the inline SSA builder. This pattern is tested
// via the real go-redis codebase instead.
// Documented here for reference:
//
//   u, err := url.Parse(s)
//   if err != nil { return nil, err }
//   return u.Scheme  // u is non-nil when err is nil, but analyzer warns
//
// This is a false positive because url.Parse guarantees non-nil *URL
// when err is nil.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Category 9: Nested nil checks with sequential ifs — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_NestedNilCheckSequential(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func nestedCheck(pp **int) int {
			if pp != nil {
				p := *pp
				if p != nil {
					return *p
				}
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "nestedCheck")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	// CORRECTLY HANDLED: nested nil checks with sequential ifs work.
	require.Empty(t, findings,
		"nested nil checks with sequential ifs should be safe")
}

// ---------------------------------------------------------------------------
// Category 10: Early return guard — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_EarlyReturnGuard(t *testing.T) {
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

	require.Empty(t, findings,
		"early return guard should prove p non-nil after guard")
}

// ---------------------------------------------------------------------------
// Category 11: Callee return nil checked by caller — CORRECTLY HANDLED
// ---------------------------------------------------------------------------

func TestCorrectlyHandled_CalleeReturnNilChecked(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func maybeNil(cond bool) *int {
			if cond {
				return new(int)
			}
			return nil
		}

		func checked() int {
			p := maybeNil(true)
			if p != nil {
				return *p
			}
			return 0
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "checked")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)

	require.Empty(t, findings,
		"callee return checked before use should be safe")
}
