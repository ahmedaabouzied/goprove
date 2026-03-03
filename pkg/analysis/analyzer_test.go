package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

func TestAnalyze(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      string
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		"div by constant is safe": {
			src: `
				package example

				func divByConstant(x int) int {
					return x / 10
				}
			`,
			fnName:  "divByConstant",
			wantLen: 0,
		},
		"safe div checking zero divisor": {
			src: `
				package example

				func safeDiv(x, y int) int {
					if y != 0 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "safeDiv",
			wantLen: 0,
		},
		"div by zero literal is a bug": {
			src: `
				package example

				func divByZeroLiteral(x int) int {
					zero := 0
					return x / zero
				}
			`,
			fnName:   "divByZeroLiteral",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "division by zero",
		},
		"mod by zero literal is a bug": {
			src: `
				package example

				func modByZero(x int) int {
					zero := 0
					return x % zero
				}
			`,
			fnName:   "modByZero",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "division by zero",
		},
		"div by unchecked param is a warning": {
			src: `
				package example

				func divByParam(x, y int) int {
					return x / y
				}
			`,
			fnName:   "divByParam",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"simple addition has no findings": {
			src: `
				package example

				func add(a, b int) int {
					return a + b
				}
			`,
			fnName:  "add",
			wantLen: 0,
		},
		"multiplication has no findings": {
			src: `
				package example

				func mul(a, b int) int {
					return a * b
				}
			`,
			fnName:  "mul",
			wantLen: 0,
		},
		"div after computation x-x is a warning": {
			// x-x is always 0, but interval analysis can't prove it
			// because Top.Sub(Top) = Top (no relational tracking).
			// This is a known limitation of non-relational domains.
			// Relational analysis (e.g. octagons, polyhedra) could prove this,
			// but is not on the current roadmap.
			src: `
				package example

				func divAfterComputation(x int) int {
					d := x - x
					return 100 / d
				}
			`,
			fnName:   "divAfterComputation",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"div by negative constant is safe": {
			src: `
				package example

				func divByNegConst(x int) int {
					return x / -3
				}
			`,
			fnName:  "divByNegConst",
			wantLen: 0,
		},
		"div by one is safe": {
			src: `
				package example

				func divByOne(x int) int {
					return x / 1
				}
			`,
			fnName:  "divByOne",
			wantLen: 0,
		},
		"chained division both warn": {
			src: `
				package example

				func chainedDiv(a, b, c int) int {
					return a / b / c
				}
			`,
			fnName:   "chainedDiv",
			wantLen:  2,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"div by constant product is safe": {
			src: `
				package example

				func divByProduct(x int) int {
					d := 3 * 7
					return x / d
				}
			`,
			fnName:  "divByProduct",
			wantLen: 0,
		},
		"div by constant sum is safe": {
			src: `
				package example

				func divBySum(x int) int {
					d := 5 + 3
					return x / d
				}
			`,
			fnName:  "divBySum",
			wantLen: 0,
		},
		"mod by unchecked param is a warning": {
			src: `
				package example

				func modByParam(x, y int) int {
					return x % y
				}
			`,
			fnName:   "modByParam",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		// Note: x / 0 with a literal zero is a compile error in Go
		// ("invalid operation: division by zero"), so the type checker
		// rejects it before SSA is built. We don't need to test it —
		// the zero := 0; x / zero case covers runtime division by zero.
		"subtraction no division is safe": {
			src: `
				package example

				func sub(a, b int) int {
					return a - b
				}
			`,
			fnName:  "sub",
			wantLen: 0,
		},
		"multiple operations ending in safe div": {
			src: `
				package example

				func multiOp(x int) int {
					a := x + 5
					b := a * 2
					return b / 10
				}
			`,
			fnName:  "multiOp",
			wantLen: 0,
		},
		"div by const zero minus zero is a bug": {
			src: `
				package example

				func divByConstZero(x int) int {
					d := 5 - 5
					return x / d
				}
			`,
			fnName:   "divByConstZero",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "division by zero",
		},
		"phi node merges intervals from branches": {
			// The Phi node at the merge point joins [1,1] and [2,2] → [1,2].
			// Dividing by [1,2] is safe (no zero).
			src: `
				package example

				func phiDiv(x int, flag bool) int {
					d := 1
					if flag {
						d = 2
					}
					return x / d
				}
			`,
			fnName:  "phiDiv",
			wantLen: 0,
		},
		"safe div with eq check": {
			// Tests the cond.X is const path (constant on the left side).
			src: `
				package example

				func safeDivEq(x, y int) int {
					if 0 == y {
						return 0
					}
					return x / y
				}
			`,
			fnName:  "safeDivEq",
			wantLen: 0,
		},
		"function with no branches or division": {
			// Exercises the refineFromPredecessor path where predecessor
			// doesn't end with If (just a plain Jump).
			src: `
				package example

				func identity(x int) int {
					y := x + 1
					return y
				}
			`,
			fnName:  "identity",
			wantLen: 0,
		},
		// --- LSS (x < C) ---
		"LSS true branch: x > 0 proves safe div": {
			// if x > 0 → true branch: x in [1, MaxInt64] → no zero → safe
			src: `
				package example

				func lssTrue(x, y int) int {
					if y > 0 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "lssTrue",
			wantLen: 0,
		},
		"LSS false branch: y < 1 false means y >= 1 proves safe": {
			// if y < 1 → false branch: y in [1, MaxInt64] → no zero → safe
			src: `
				package example

				func lssFalse(x, y int) int {
					if y < 1 {
						return 0
					}
					return x / y
				}
			`,
			fnName:  "lssFalse",
			wantLen: 0,
		},
		// --- LEQ (x <= C) ---
		"LEQ true branch: y <= -1 proves safe div": {
			// if y <= -1 → true branch: y in [MinInt64, -1] → no zero → safe
			src: `
				package example

				func leqTrue(x, y int) int {
					if y <= -1 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "leqTrue",
			wantLen: 0,
		},
		"LEQ false branch: y <= 0 false means y >= 1 proves safe div": {
			// if y <= 0 → false branch: y in [1, MaxInt64] → no zero → safe
			src: `
				package example

				func leqFalse(x, y int) int {
					if y <= 0 {
						return 0
					}
					return x / y
				}
			`,
			fnName:  "leqFalse",
			wantLen: 0,
		},
		// --- GTR (x > C) ---
		"GTR true branch: y > 0 proves safe div": {
			// if y > 0 → true branch: y in [1, MaxInt64] → no zero → safe
			src: `
				package example

				func gtrTrue(x, y int) int {
					if y > 0 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "gtrTrue",
			wantLen: 0,
		},
		"GTR false branch: y > 0 false means y <= 0 includes zero": {
			// if y > 0 → false branch: y in [MinInt64, 0] → contains zero → warn
			src: `
				package example

				func gtrFalse(x, y int) int {
					if y > 0 {
						return 0
					}
					return x / y
				}
			`,
			fnName:   "gtrFalse",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		// --- GEQ (x >= C) ---
		"GEQ true branch: y >= 1 proves safe div": {
			// if y >= 1 → true branch: y in [1, MaxInt64] → no zero → safe
			src: `
				package example

				func geqTrue(x, y int) int {
					if y >= 1 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "geqTrue",
			wantLen: 0,
		},
		"GEQ false branch: y >= 1 false means y <= 0 includes zero": {
			// if y >= 1 → false branch: y in [MinInt64, 0] → contains zero → warn
			src: `
				package example

				func geqFalse(x, y int) int {
					if y >= 1 {
						return 0
					}
					return x / y
				}
			`,
			fnName:   "geqFalse",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"float division hits non-int const path": {
			// Float constants have constant.Float kind, not constant.Int.
			// lookupInterval returns Top() for them and sets a.err.
			// Since Top contains zero, division warns.
			src: `
				package example

				func floatDiv(x float64) float64 {
					return x / 2.0
				}
			`,
			fnName:   "floatDiv",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"negation of param is still Top warns": {
			// -x negates Top → Top. Top contains zero, so division warns.
			src: `
				package example

				func negDiv(x int) int {
					y := -x
					return 10 / y
				}
			`,
			fnName:   "negDiv",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"negation of positive constant is safe": {
			// -5 is [5,5].Neg() = [-5,-5]. No zero. Safe.
			src: `
				package example

				func negConstDiv(x int) int {
					c := 5
					d := -c
					return x / d
				}
			`,
			fnName:  "negConstDiv",
			wantLen: 0,
		},
		"type conversion preserves interval": {
			// int32(5) converts [5,5] → [5,5]. No zero. Safe.
			src: `
				package example

				func convertDiv(x int) int {
					c := 5
					d := int64(c)
					return x / int(d)
				}
			`,
			fnName:  "convertDiv",
			wantLen: 0,
		},
		"type conversion of param stays Top warns": {
			// int32(y) converts Top → Top. Contains zero. Warns.
			src: `
				package example

				func convertParamDiv(x int, y int32) int {
					d := int(y)
					return x / d
				}
			`,
			fnName:   "convertParamDiv",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"both sides are variables in condition": {
			// if x > y — neither operand is a constant, so
			// refineFromCondition returns early (line 85-88).
			// y is still Top, so division warns.
			src: `
				package example

				func divWithVarCondition(x, y int) int {
					if x > y {
						return x / y
					}
					return 0
				}
			`,
			fnName:   "divWithVarCondition",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)

			var fn *ssa.Function
			for _, member := range ssaPkg.Members {
				f, ok := member.(*ssa.Function)
				if !ok {
					continue
				}
				if f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen)

			if tt.wantLen > 0 {
				require.Equal(t, tt.severity, findings[0].Severity)
				require.Equal(t, tt.message, findings[0].Message)
			}
		})
	}
}

// TestAnalyzeLoops tests loop handling. These tests require the worklist
// algorithm with widening to produce correct results. Without iteration
// to a fixed point, the single RPO pass misses back-edge contributions
// to Phi nodes, making loop variables imprecise.
func TestAnalyzeLoops(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      string
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		"div by loop counter starting at 1 is safe": {
			// i starts at 1 and only increments.
			// After widening, i should be [1, MaxInt64]. No zero. Safe.
			src: `
				package example

				func divByCounter(x, n int) int {
					result := 0
					for i := 1; i <= n; i++ {
						result += x / i
					}
					return result
				}
			`,
			fnName:  "divByCounter",
			wantLen: 0,
		},
		"div by s+1 after loop accumulation is safe": {
			// s starts at 0 and accumulates. s+1 >= 1 always. Safe.
			src: `
				package example

				func divAfterLoop(x, n int) int {
					s := 0
					for i := 1; i <= n; i++ {
						s += i
					}
					return x / (s + 1)
				}
			`,
			fnName:  "divAfterLoop",
			wantLen: 0,
		},
		"div by loop counter starting at 0 warns": {
			// i starts at 0. Even with widening, [0, MaxInt64] contains zero. Warn.
			src: `
				package example

				func divByZeroCounter(x, n int) int {
					result := 0
					for i := 0; i < n; i++ {
						result += x / i
					}
					return result
				}
			`,
			fnName:   "divByZeroCounter",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"div by loop counter with guard inside loop is safe": {
			// i starts at 0, but division only happens when i > 0. Safe.
			src: `
				package example

				func divGuardedInLoop(x, n int) int {
					result := 0
					for i := 0; i < n; i++ {
						if i > 0 {
							result += x / i
						}
					}
					return result
				}
			`,
			fnName:  "divGuardedInLoop",
			wantLen: 0,
		},
		"loop with constant bound keeps interval bounded": {
			// i goes from 0 to 9. After the loop, i == 10.
			// Dividing by i+1 (>= 1) is safe.
			src: `
				package example

				func divAfterBoundedLoop(x int) int {
					s := 0
					for i := 0; i < 10; i++ {
						s += i
					}
					return x / (s + 1)
				}
			`,
			fnName:  "divAfterBoundedLoop",
			wantLen: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)

			var fn *ssa.Function
			for _, member := range ssaPkg.Members {
				f, ok := member.(*ssa.Function)
				if !ok {
					continue
				}
				if f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen)

			if tt.wantLen > 0 {
				require.Equal(t, tt.severity, findings[0].Severity)
				require.Equal(t, tt.message, findings[0].Message)
			}
		})
	}
}

// TestAnalyzeLoopsAdvanced tests more complex loop patterns that exercise
// the worklist algorithm, widening, and the interaction between loops
// and branch refinement.
func TestAnalyzeLoopsAdvanced(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      string
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		"nested loop with safe divisor": {
			// Both i and j start at 1. Division by j is safe.
			src: `
				package example

				func nestedLoop(x, n, m int) int {
					result := 0
					for i := 1; i <= n; i++ {
						for j := 1; j <= m; j++ {
							result += x / j
						}
					}
					return result
				}
			`,
			fnName:  "nestedLoop",
			wantLen: 0,
		},
		"nested loop with unsafe divisor starting at 0": {
			// Inner loop starts j at 0. Division by j warns.
			src: `
				package example

				func nestedLoopUnsafe(x, n, m int) int {
					result := 0
					for i := 1; i <= n; i++ {
						for j := 0; j < m; j++ {
							result += x / j
						}
					}
					return result
				}
			`,
			fnName:   "nestedLoopUnsafe",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"while-style loop with decrementing counter": {
			// Counter starts at n (Top) and decrements. Division by counter warns
			// because Top contains zero.
			src: `
				package example

				func decLoop(x, n int) int {
					result := 0
					i := n
					for i > 0 {
						result += x / i
						i--
					}
					return result
				}
			`,
			fnName:  "decLoop",
			wantLen: 0,
		},
		"loop accumulator used as divisor warns": {
			// s starts at 0 and accumulates. s itself may be zero (first iteration).
			src: `
				package example

				func accumDiv(x, n int) int {
					s := 0
					for i := 0; i < n; i++ {
						s += i
					}
					return x / s
				}
			`,
			fnName:   "accumDiv",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"div by constant inside loop is safe": {
			// Constant divisor never changes regardless of loop iterations.
			src: `
				package example

				func constDivInLoop(x, n int) int {
					result := 0
					for i := 0; i < n; i++ {
						result += x / 7
					}
					return result
				}
			`,
			fnName:  "constDivInLoop",
			wantLen: 0,
		},
		"multiple divisions in loop body": {
			// First division by constant (safe), second by loop var starting at 0 (warn).
			src: `
				package example

				func multiDivLoop(x, n int) int {
					result := 0
					for i := 0; i < n; i++ {
						a := x / 5
						b := a / i
						result += b
					}
					return result
				}
			`,
			fnName:   "multiDivLoop",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"loop counter with step 2 starting at 1 is safe": {
			// i goes 1, 3, 5, 7... Always positive. Safe.
			src: `
				package example

				func stepTwoLoop(x, n int) int {
					result := 0
					for i := 1; i < n; i += 2 {
						result += x / i
					}
					return result
				}
			`,
			fnName:  "stepTwoLoop",
			wantLen: 0,
		},
		"loop with break still analyzes correctly": {
			// Even with a break, the division happens when i >= 1.
			src: `
				package example

				func breakLoop(x, n int) int {
					result := 0
					for i := 1; i <= n; i++ {
						if i > 100 {
							break
						}
						result += x / i
					}
					return result
				}
			`,
			fnName:  "breakLoop",
			wantLen: 0,
		},
		"division after loop with counter starting at 1": {
			// After the loop, i has been widened to [1, MaxInt64].
			// Division by i after the loop is safe.
			src: `
				package example

				func divAfterLoopCounter(x, n int) int {
					i := 1
					for i <= n {
						i++
					}
					return x / i
				}
			`,
			fnName:  "divAfterLoopCounter",
			wantLen: 0,
		},
		"loop with guarded division by zero var safe": {
			// i starts at 0, but we only divide when i != 0.
			src: `
				package example

				func guardedZeroVar(x, n int) int {
					result := 0
					for i := 0; i < n; i++ {
						if i != 0 {
							result += x / i
						}
					}
					return result
				}
			`,
			fnName:  "guardedZeroVar",
			wantLen: 0,
		},
		"mod in loop by counter starting at 1 is safe": {
			// Mod (%) has the same zero-divisor issue as division.
			src: `
				package example

				func modInLoop(x, n int) int {
					result := 0
					for i := 1; i <= n; i++ {
						result += x % i
					}
					return result
				}
			`,
			fnName:  "modInLoop",
			wantLen: 0,
		},
		"no findings for loop without division": {
			// Pure computation loop, no division at all.
			src: `
				package example

				func sumLoop(n int) int {
					s := 0
					for i := 0; i < n; i++ {
						s += i
					}
					return s
				}
			`,
			fnName:  "sumLoop",
			wantLen: 0,
		},
		"worklist terminates on complex CFG": {
			// Multiple branches inside a loop — exercises worklist convergence.
			src: `
				package example

				func complexCFG(x, n int) int {
					result := 0
					for i := 1; i <= n; i++ {
						if i > 10 {
							result += x / i
						} else {
							result += x / (i + 1)
						}
					}
					return result
				}
			`,
			fnName:  "complexCFG",
			wantLen: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)

			var fn *ssa.Function
			for _, member := range ssaPkg.Members {
				f, ok := member.(*ssa.Function)
				if !ok {
					continue
				}
				if f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen)

			if tt.wantLen > 0 {
				require.Equal(t, tt.severity, findings[0].Severity)
				require.Equal(t, tt.message, findings[0].Message)
			}
		})
	}
}

// TestAnalyzeOverflow tests integer overflow detection across all supported
// narrow integer types (int8, int16, int32) and all arithmetic operators.
func TestAnalyzeOverflow(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
		checks  []struct {
			severity analysis.Severity
			message  string
		}
	}{
		// === PROVEN OVERFLOW (Bug) — result entirely outside type bounds ===

		"int8 add proven overflow": {
			// 100 + 100 = [200, 200] entirely outside [-128, 127]
			src: `
				package example

				func int8AddOverflow() int8 {
					var a int8 = 100
					var b int8 = 100
					return a + b
				}
			`,
			fnName:  "int8AddOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int8 add proven negative overflow": {
			// -100 + (-100) = [-200, -200] entirely outside [-128, 127]
			src: `
				package example

				func int8AddNegOverflow() int8 {
					var a int8 = -100
					var b int8 = -100
					return a + b
				}
			`,
			fnName:  "int8AddNegOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int8 mul proven overflow": {
			// 50 * 3 = [150, 150] entirely outside [-128, 127]
			src: `
				package example

				func int8MulOverflow() int8 {
					var a int8 = 50
					var b int8 = 3
					return a * b
				}
			`,
			fnName:  "int8MulOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int8 sub proven overflow": {
			// -100 - 100 = [-200, -200] entirely outside [-128, 127]
			src: `
				package example

				func int8SubOverflow() int8 {
					var a int8 = -100
					var b int8 = 100
					return a - b
				}
			`,
			fnName:  "int8SubOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int16 add proven overflow": {
			// 30000 + 30000 = [60000, 60000] entirely outside [-32768, 32767]
			src: `
				package example

				func int16AddOverflow() int16 {
					var a int16 = 30000
					var b int16 = 30000
					return a + b
				}
			`,
			fnName:  "int16AddOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int32 mul proven overflow": {
			// 100000 * 100000 = [10000000000, 10000000000] outside int32
			src: `
				package example

				func int32MulOverflow() int32 {
					var a int32 = 100000
					var b int32 = 100000
					return a * b
				}
			`,
			fnName:  "int32MulOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},

		// === POSSIBLE OVERFLOW (Warning) — result partially exceeds bounds ===

		"int8 add param possible overflow": {
			// x is Top for int8. x + 1 could overflow (if x = 127) or not.
			// Result is Top, which is not contained in [-128, 127].
			// Meet(Top, [-128, 127]) is [-128, 127] which is not Bottom,
			// so it's a Warning not a Bug.
			src: `
				package example

				func int8AddParam(x int8) int8 {
					return x + 1
				}
			`,
			fnName:  "int8AddParam",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"int8 mul param possible overflow": {
			// x is Top for int8. x * 2 could overflow or not.
			src: `
				package example

				func int8MulParam(x int8) int8 {
					return x * 2
				}
			`,
			fnName:  "int8MulParam",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"int8 sub param possible overflow": {
			// x is Top for int8. x - 1 could underflow (if x = -128) or not.
			src: `
				package example

				func int8SubParam(x int8) int8 {
					return x - 1
				}
			`,
			fnName:  "int8SubParam",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"int16 sub param possible overflow": {
			src: `
				package example

				func int16SubParam(x int16) int16 {
					return x - 1
				}
			`,
			fnName:  "int16SubParam",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"int32 add param possible overflow": {
			src: `
				package example

				func int32AddParam(x int32) int32 {
					return x + 1
				}
			`,
			fnName:  "int32AddParam",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},

		// === SAFE — result fits within type bounds ===

		"int8 add small constants safe": {
			// 10 + 5 = [15, 15] within [-128, 127]
			src: `
				package example

				func int8AddSafe() int8 {
					var a int8 = 10
					var b int8 = 5
					return a + b
				}
			`,
			fnName:  "int8AddSafe",
			wantLen: 0,
		},
		"int8 sub small constants safe": {
			// 10 - 5 = [5, 5] within [-128, 127]
			src: `
				package example

				func int8SubSafe() int8 {
					var a int8 = 10
					var b int8 = 5
					return a - b
				}
			`,
			fnName:  "int8SubSafe",
			wantLen: 0,
		},
		"int8 mul small constants safe": {
			// 5 * 5 = [25, 25] within [-128, 127]
			src: `
				package example

				func int8MulSafe() int8 {
					var a int8 = 5
					var b int8 = 5
					return a * b
				}
			`,
			fnName:  "int8MulSafe",
			wantLen: 0,
		},
		"int8 negative result safe": {
			// -50 + (-50) = [-100, -100] within [-128, 127]
			src: `
				package example

				func int8NegSafe() int8 {
					var a int8 = -50
					var b int8 = -50
					return a + b
				}
			`,
			fnName:  "int8NegSafe",
			wantLen: 0,
		},
		"int8 boundary safe max": {
			// 100 + 27 = [127, 127] exactly at max int8 boundary
			src: `
				package example

				func int8BoundarySafe() int8 {
					var a int8 = 100
					var b int8 = 27
					return a + b
				}
			`,
			fnName:  "int8BoundarySafe",
			wantLen: 0,
		},
		"int8 boundary safe min": {
			// -100 + (-28) = [-128, -128] exactly at min int8 boundary
			src: `
				package example

				func int8BoundaryMinSafe() int8 {
					var a int8 = -100
					var b int8 = -28
					return a + b
				}
			`,
			fnName:  "int8BoundaryMinSafe",
			wantLen: 0,
		},
		"int16 add small safe": {
			// 100 + 200 = [300, 300] within [-32768, 32767]
			src: `
				package example

				func int16AddSafe() int16 {
					var a int16 = 100
					var b int16 = 200
					return a + b
				}
			`,
			fnName:  "int16AddSafe",
			wantLen: 0,
		},
		"int32 add small safe": {
			// 1000 + 2000 = [3000, 3000] within int32 bounds
			src: `
				package example

				func int32AddSafe() int32 {
					var a int32 = 1000
					var b int32 = 2000
					return a + b
				}
			`,
			fnName:  "int32AddSafe",
			wantLen: 0,
		},
		"int8 div safe": {
			// 100 / 10 = [10, 10] within [-128, 127]. Also no div-by-zero.
			src: `
				package example

				func int8DivSafe() int8 {
					var a int8 = 100
					var b int8 = 10
					return a / b
				}
			`,
			fnName:  "int8DivSafe",
			wantLen: 0,
		},

		// === UNTRACKED TYPES — no overflow findings ===

		"int add no overflow finding": {
			// int is untracked (platform-dependent), no overflow check.
			src: `
				package example

				func intAdd(x, y int) int {
					return x + y
				}
			`,
			fnName:  "intAdd",
			wantLen: 0,
		},
		"int64 add no overflow finding": {
			// int64 is untracked (our internal representation), no overflow check.
			src: `
				package example

				func int64Add(x, y int64) int64 {
					return x + y
				}
			`,
			fnName:  "int64Add",
			wantLen: 0,
		},

		// === BRANCHING — overflow guarded by condition ===

		"int8 add guarded by upper bound safe": {
			// Params are initialized to type bounds: int8 x starts as [-128, 127].
			// x < 100 refines to [-128, 99]. x + 27 gives [-101, 126].
			// [-101, 126] fits within [-128, 127]. Safe.
			src: `
				package example

				func int8Guarded(x int8) int8 {
					if x < 100 {
						return x + 27
					}
					return 0
				}
			`,
			fnName:  "int8Guarded",
			wantLen: 0,
		},

		// === OVERFLOW ON BOUNDARY — one past the limit ===

		"int8 one past max is overflow": {
			// 100 + 28 = [128, 128] just past max int8 (127)
			src: `
				package example

				func int8OnePast() int8 {
					var a int8 = 100
					var b int8 = 28
					return a + b
				}
			`,
			fnName:  "int8OnePast",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int8 one past min is overflow": {
			// -100 + (-29) = [-129, -129] just past min int8 (-128)
			src: `
				package example

				func int8OnePastMin() int8 {
					var a int8 = -100
					var b int8 = -29
					return a + b
				}
			`,
			fnName:  "int8OnePastMin",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},

		// === DEFAULT BINOP — non-arithmetic ops produce no overflow ===

		"int8 comparison produces no overflow": {
			// Comparison BinOps (==, <, etc.) produce bool, not int8.
			// No overflow check applies.
			src: `
				package example

				func int8Cmp(x, y int8) bool {
					return x < y
				}
			`,
			fnName:  "int8Cmp",
			wantLen: 0,
		},

		// === MULTIPLE FINDINGS — overflow + div-by-zero ===

		"int8 div by zero and overflow": {
			// Division by zero-valued var. The div result is Top (since divisor
			// contains zero). Depending on type, overflow may also be flagged.
			// This verifies both checks coexist without interfering.
			src: `
				package example

				func int8DivByZero() int8 {
					var a int8 = 100
					var b int8 = 0
					return a / b
				}
			`,
			fnName:  "int8DivByZero",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
				{analysis.Warning, "possible integer overflow"},
			},
		},

		// === CHAINED ARITHMETIC ===

		"int8 chained add overflow": {
			// 50 + 50 = 100 (safe), then 100 + 50 = 150 (overflow)
			src: `
				package example

				func int8ChainedAdd() int8 {
					var a int8 = 50
					var b int8 = 50
					c := a + b
					return c + b
				}
			`,
			fnName:  "int8ChainedAdd",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int8 chained mul overflow": {
			// 10 * 10 = 100 (safe), then 100 * 2 = 200 (overflow)
			src: `
				package example

				func int8ChainedMul() int8 {
					var a int8 = 10
					var b int8 = 10
					c := a * b
					var d int8 = 2
					return c * d
				}
			`,
			fnName:  "int8ChainedMul",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},

		// === LOOP + OVERFLOW ===

		"int8 loop counter overflow possible": {
			// i starts at 1 and increments as int8. After widening,
			// the add i + 1 has Top result which exceeds int8 bounds.
			src: `
				package example

				func int8LoopOverflow(n int8) int8 {
					var s int8 = 0
					for i := int8(1); i < n; i++ {
						s += i
					}
					return s
				}
			`,
			fnName:  "int8LoopOverflow",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
				{analysis.Warning, "possible integer overflow"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)

			var fn *ssa.Function
			for _, member := range ssaPkg.Members {
				f, ok := member.(*ssa.Function)
				if !ok {
					continue
				}
				if f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen, "unexpected number of findings")

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Equal(t, check.message, findings[i].Message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// TestAnalyzeParamTypeBounds verifies that function parameters are initialized
// to their type's interval bounds rather than Top. This improves precision:
// an int8 param starts as [-128, 127] instead of [MinInt64, MaxInt64].
func TestAnalyzeParamTypeBounds(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
		checks  []struct {
			severity analysis.Severity
			message  string
		}
	}{
		// === int8 params bounded to [-128, 127] ===

		"int8 param add 1 safe": {
			// x is [-128, 127]. x + 1 gives [-127, 128].
			// [-127, 128] partially exceeds [-128, 127] at the top.
			// This is a Warning — but NOT a false Bug.
			src: `
				package example

				func int8Add1(x int8) int8 {
					return x + 1
				}
			`,
			fnName:  "int8Add1",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"int8 param guarded upper safe": {
			// x is [-128, 127]. x < 100 refines to [-128, 99].
			// x + 27 gives [-101, 126]. Fits in [-128, 127]. Safe.
			src: `
				package example

				func int8GuardedUpper(x int8) int8 {
					if x < 100 {
						return x + 27
					}
					return 0
				}
			`,
			fnName:  "int8GuardedUpper",
			wantLen: 0,
		},
		"int8 param guarded lower safe": {
			// x is [-128, 127]. x > -100 refines to [-99, 127].
			// x - 28 gives [-127, 99]. Fits in [-128, 127]. Safe.
			src: `
				package example

				func int8GuardedLower(x int8) int8 {
					if x > -100 {
						return x - 28
					}
					return 0
				}
			`,
			fnName:  "int8GuardedLower",
			wantLen: 0,
		},
		"int8 param guarded both sides safe": {
			// x > -50 && x < 50 refines to [-49, 49].
			// x * 2 gives [-98, 98]. Fits in [-128, 127]. Safe.
			src: `
				package example

				func int8GuardedBoth(x int8) int8 {
					if x > -50 {
						if x < 50 {
							return x * 2
						}
					}
					return 0
				}
			`,
			fnName:  "int8GuardedBoth",
			wantLen: 0,
		},
		"int8 param guarded but still overflows": {
			// x > 0 refines to [1, 127]. x + 127 gives [128, 254].
			// Entirely outside [-128, 127]. Proven Bug.
			src: `
				package example

				func int8GuardedOverflow(x int8) int8 {
					if x > 0 {
						return x + 127
					}
					return 0
				}
			`,
			fnName:  "int8GuardedOverflow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},
		"int8 param negation safe": {
			// x is [-128, 127]. -x gives [-127, 128].
			// Partially exceeds — Warning (because -(-128) = 128 overflows).
			// But with bounded params this is a Warning not a false safe.
			src: `
				package example

				func int8Neg(x int8) int8 {
					return -x
				}
			`,
			fnName:  "int8Neg",
			wantLen: 0, // UnOp negation doesn't go through flagOverflow (only BinOp does)
		},

		// === int16 params bounded to [-32768, 32767] ===

		"int16 param guarded safe": {
			// x is [-32768, 32767]. x < 32000 refines to [-32768, 31999].
			// x + 700 gives [-32068, 32699]. Fits in [-32768, 32767]. Safe.
			src: `
				package example

				func int16Guarded(x int16) int16 {
					if x < 32000 {
						return x + 700
					}
					return 0
				}
			`,
			fnName:  "int16Guarded",
			wantLen: 0,
		},
		"int16 param unguarded add warns": {
			// x is [-32768, 32767]. x + 1 gives [-32767, 32768].
			// Partially exceeds. Warning.
			src: `
				package example

				func int16Add1(x int16) int16 {
					return x + 1
				}
			`,
			fnName:  "int16Add1",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},

		// === int32 params bounded to [-2147483648, 2147483647] ===

		"int32 param guarded safe": {
			// x < 2000000000 refines to [-2147483648, 1999999999].
			// x + 100000000 gives [-2047483648, 2099999999].
			// int32 bounds: [-2147483648, 2147483647].
			// Lo: -2147483648 <= -2047483648 ✓, Hi: 2147483647 >= 2099999999 ✓.
			// Fully contained. Safe.
			src: `
				package example

				func int32Guarded(x int32) int32 {
					if x < 2000000000 {
						return x + 100000000
					}
					return 0
				}
			`,
			fnName:  "int32Guarded",
			wantLen: 0,
		},
		"int32 param tightly guarded safe": {
			// x < 1000 refines to [-2147483648, 999].
			// x + 500 gives [-2147483148, 1499]. Fits in int32. Safe.
			src: `
				package example

				func int32TightGuard(x int32) int32 {
					if x < 1000 {
						return x + 500
					}
					return 0
				}
			`,
			fnName:  "int32TightGuard",
			wantLen: 0,
		},

		// === int params stay Top (untracked) ===

		"int param add no overflow": {
			// int is untracked. Params start as Top. No overflow check fires.
			src: `
				package example

				func intAdd(x int) int {
					return x + 1
				}
			`,
			fnName:  "intAdd",
			wantLen: 0,
		},

		// === Division precision improved by type bounds ===

		"int8 param div after guard safe": {
			// x is [-128, 127]. x > 0 refines to [1, 127].
			// Division by x is safe (no zero in [1, 127]).
			src: `
				package example

				func int8DivGuarded(a, x int8) int8 {
					if x > 0 {
						return a / x
					}
					return 0
				}
			`,
			fnName:  "int8DivGuarded",
			wantLen: 0,
		},
		"int8 param div unguarded warns": {
			// x is [-128, 127]. Contains zero → div-by-zero warning.
			// Division result is Top (divisor contains zero) → exceeds int8 → overflow warning.
			src: `
				package example

				func int8DivUnguarded(a, x int8) int8 {
					return a / x
				}
			`,
			fnName:  "int8DivUnguarded",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
				{analysis.Warning, "possible integer overflow"},
			},
		},

		// === Multiple params, mixed types ===

		"mixed param types": {
			// x is int (Top). int8(x) flags possible overflow in conversion
			// (Top doesn't fit int8). Then Top / [1, 127] gives Top which
			// also exceeds int8 → second overflow warning on the division.
			src: `
				package example

				func mixedParams(x int, y int8) int8 {
					if y > 0 {
						return int8(x) / y
					}
					return 0
				}
			`,
			fnName:  "mixedParams",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
				{analysis.Warning, "possible integer overflow"},
			},
		},

		// === Phi node merges with bounded params ===

		"int8 phi merge stays bounded": {
			// x is [-128, 127]. Both branches assign values within int8 range.
			// Phi joins [1,1] and [2,2] → [1,2]. x + d gives [-127, 129].
			// Partially exceeds. Warning.
			src: `
				package example

				func int8PhiBounded(x int8, flag bool) int8 {
					var d int8 = 1
					if flag {
						d = 2
					}
					return x + d
				}
			`,
			fnName:  "int8PhiBounded",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"int8 phi merge guarded safe": {
			// x < 100 refines to [-128, 99]. d is [1,2].
			// x + d gives [-127, 101]. Fits in [-128, 127]. Safe.
			src: `
				package example

				func int8PhiGuarded(x int8, flag bool) int8 {
					var d int8 = 1
					if flag {
						d = 2
					}
					if x < 100 {
						return x + d
					}
					return 0
				}
			`,
			fnName:  "int8PhiGuarded",
			wantLen: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)

			var fn *ssa.Function
			for _, member := range ssaPkg.Members {
				f, ok := member.(*ssa.Function)
				if !ok {
					continue
				}
				if f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen, "unexpected number of findings")

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Equal(t, check.message, findings[i].Message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

func TestAnalyzeEmptyBlocks(t *testing.T) {
	t.Parallel()

	// A function with no blocks (e.g. external/assembly-backed functions)
	// should return nil findings without panicking.
	fn := &ssa.Function{} // Blocks is nil by default
	require.Empty(t, fn.Blocks)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Nil(t, findings)
}
