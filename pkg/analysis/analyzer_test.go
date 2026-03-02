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
		"unhandled instruction type falls through to Top": {
			// -x is a *ssa.UnOp, which transferInstruction doesn't handle.
			// So when lookupInterval is called for -x as a divisor,
			// it's not a const and not in the state map → returns Top().
			// Top contains zero, so division warns.
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
