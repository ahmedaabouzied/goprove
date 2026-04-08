package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
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
		"mod by constant is safe": {
			src: `
				package example

				func modByConst(x int) int {
					return x % 10
				}
			`,
			fnName:  "modByConst",
			wantLen: 0,
		},
		"mod by negative constant is safe": {
			src: `
				package example

				func modByNegConst(x int) int {
					return x % -3
				}
			`,
			fnName:  "modByNegConst",
			wantLen: 0,
		},
		"mod by one is safe": {
			src: `
				package example

				func modByOne(x int) int {
					return x % 1
				}
			`,
			fnName:  "modByOne",
			wantLen: 0,
		},
		"safe mod with neq check": {
			src: `
				package example

				func safeMod(x, y int) int {
					if y != 0 {
						return x % y
					}
					return 0
				}
			`,
			fnName:  "safeMod",
			wantLen: 0,
		},
		"safe mod with eq check": {
			src: `
				package example

				func safeModEq(x, y int) int {
					if 0 == y {
						return 0
					}
					return x % y
				}
			`,
			fnName:  "safeModEq",
			wantLen: 0,
		},
		"safe mod with greater than zero": {
			src: `
				package example

				func safeModGt(x, y int) int {
					if y > 0 {
						return x % y
					}
					return 0
				}
			`,
			fnName:  "safeModGt",
			wantLen: 0,
		},
		"safe mod with less than zero": {
			src: `
				package example

				func safeModLt(x, y int) int {
					if y < 0 {
						return x % y
					}
					return 0
				}
			`,
			fnName:  "safeModLt",
			wantLen: 0,
		},
		"mod by const zero minus zero is a bug": {
			src: `
				package example

				func modByConstZero(x int) int {
					d := 5 - 5
					return x % d
				}
			`,
			fnName:   "modByConstZero",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "division by zero",
		},
		"mod by constant product is safe": {
			src: `
				package example

				func modByProduct(x int) int {
					d := 3 * 7
					return x % d
				}
			`,
			fnName:  "modByProduct",
			wantLen: 0,
		},
		"mod by constant sum is safe": {
			src: `
				package example

				func modBySum(x int) int {
					d := 5 + 3
					return x % d
				}
			`,
			fnName:  "modBySum",
			wantLen: 0,
		},
		"chained mod both warn": {
			src: `
				package example

				func chainedMod(a, b, c int) int {
					return a % b % c
				}
			`,
			fnName:   "chainedMod",
			wantLen:  2,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"phi node merges intervals for mod": {
			src: `
				package example

				func phiMod(x int, flag bool) int {
					d := 1
					if flag {
						d = 2
					}
					return x % d
				}
			`,
			fnName:  "phiMod",
			wantLen: 0,
		},
		"mod after computation x-x is a warning": {
			src: `
				package example

				func modAfterComputation(x int) int {
					d := x - x
					return 100 % d
				}
			`,
			fnName:   "modAfterComputation",
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
		"float division by constant is not flagged": {
			// Float division by zero doesn't panic in Go (IEEE 754).
			// The analyzer should only flag integer division by zero.
			src: `
				package example

				func floatDiv(x float64) float64 {
					return x / 2.0
				}
			`,
			fnName:  "floatDiv",
			wantLen: 0,
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
		// ── Float division: no panic (IEEE 754) ──────────────────────
		"float64 division by zero literal not flagged": {
			src: `
				package example

				func floatDivZero(x float64) float64 {
					return x / 0.0
				}
			`,
			fnName:  "floatDivZero",
			wantLen: 0,
		},
		"float64 division by param not flagged": {
			src: `
				package example

				func floatDivParam(x, y float64) float64 {
					return x / y
				}
			`,
			fnName:  "floatDivParam",
			wantLen: 0,
		},
		"float32 division by zero not flagged": {
			src: `
				package example

				func float32DivZero(x float32) float32 {
					return x / 0.0
				}
			`,
			fnName:  "float32DivZero",
			wantLen: 0,
		},
		"float32 division by param not flagged": {
			src: `
				package example

				func float32DivParam(x, y float32) float32 {
					return x / y
				}
			`,
			fnName:  "float32DivParam",
			wantLen: 0,
		},
		"float64 division by zero var not flagged": {
			src: `
				package example

				func floatQuoByZeroVar() float64 {
					var y float64
					return 1.0 / y
				}
			`,
			fnName:  "floatQuoByZeroVar",
			wantLen: 0,
		},
		"float64 division in expression chain not flagged": {
			src: `
				package example

				func floatChain(a, b, c float64) float64 {
					return (a / b) / c
				}
			`,
			fnName:  "floatChain",
			wantLen: 0,
		},
		"mixed float result still flags int division": {
			// The division itself is integer; float64() conversion is after.
			src: `
				package example

				func mixedDiv(x, y int) float64 {
					return float64(x / y)
				}
			`,
			fnName:   "mixedDiv",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"int division by zero literal still flagged after float fix": {
			src: `
				package example

				func intDivZero(x int) int {
					zero := 0
					return x / zero
				}
			`,
			fnName:   "intDivZero",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "division by zero",
		},
		"int remainder by zero still flagged after float fix": {
			src: `
				package example

				func intModZero(x int) int {
					zero := 0
					return x % zero
				}
			`,
			fnName:   "intModZero",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "division by zero",
		},
		"int div by param still warns after float fix": {
			src: `
				package example

				func intDivParam(x, y int) int {
					return x / y
				}
			`,
			fnName:   "intDivParam",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"uint division by param still warns": {
			src: `
				package example

				func uintDiv(x, y uint) uint {
					return x / y
				}
			`,
			fnName:   "uintDiv",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"int8 division by param still warns": {
			// Also produces an overflow warning for int8, so expect 2.
			src: `
				package example

				func int8Div(x, y int8) int8 {
					return x / y
				}
			`,
			fnName:   "int8Div",
			wantLen:  2,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"int64 division by param still warns": {
			src: `
				package example

				func int64Div(x, y int64) int64 {
					return x / y
				}
			`,
			fnName:   "int64Div",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible division by zero",
		},
		"float and int division in same function": {
			// Only the integer division should be flagged.
			src: `
				package example

				func bothDivs(a int, b float64) float64 {
					_ = a / a
					return b / b
				}
			`,
			fnName:   "bothDivs",
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

// TestAnalyze_BooleanBinOp exercises transferBinOp's default case
// for comparison operators that produce bool results (not int intervals).
func TestAnalyze_BooleanBinOp(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func compare(a, b int) bool {
			return a == b
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "compare")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_BothSidesVariable tests that comparisons where both sides
// are variables (not constants) are handled gracefully.
func TestAnalyze_BothSidesVariable(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func bothVars(x, y int) int {
			if x < y {
				return x / 1
			}
			return y / 1
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "bothVars")
	require.NotNil(t, fn)

	analyzer := analysis.NewAnalyzer(nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_ConditionNotBinOp exercises refineFromPredecessor when
// the If condition is not a BinOp (e.g., a plain bool variable).
func TestAnalyze_ConditionNotBinOp(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func condBool(flag bool, x int) int {
			if flag {
				return x / 1
			}
			return x / 2
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "condBool")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_EmptyFunction exercises Analyze on a function with
// no instructions beyond the implicit return.
func TestAnalyze_EmptyFunction(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func empty() {}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "empty")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_ExternalFunction tests that calling a function with no
// body (no blocks) returns Top and doesn't crash.
func TestAnalyze_ExternalFunction(t *testing.T) {
	t.Parallel()
	src := `
		package example

		import "math"

		func callExternal(x float64) float64 {
			return math.Abs(x)
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "callExternal")
	require.NotNil(t, fn)

	analyzer := analysis.NewAnalyzer(nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_MultipleReturnResults verifies that multi-value returns
// don't crash the analyzer. The Extract instruction is not yet tracked,
// so the destructured value is Top → warns about division.
func TestAnalyze_MultipleReturnResults(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func twoReturns(x int) (int, int) {
			return x + 1, x - 1
		}

		func callTwoReturns(x int) int {
			a, _ := twoReturns(5)
			return 100 / a
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "callTwoReturns")
	require.NotNil(t, fn)

	analyzer := analysis.NewAnalyzer(nil)
	findings := analyzer.Analyze(fn)
	// Extract from multi-value calls yields Top → warns about possible div by zero.
	// This is a known limitation — the analyzer doesn't track Extract yet.
	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Message, "possible division by zero")
}

// TestAnalyze_MultipleReturnValues exercises computeReturnIntervals
// with functions that return multiple values.
func TestAnalyze_MultipleReturnValues(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func divmod(x, y int) (int, int) {
			return x / y, x % y
		}

		func caller() (int, int) {
			return divmod(10, 3)
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "caller")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	_ = analyzer.Analyze(fn)
	// No crash is the assertion — multiple return intervals handled correctly.
}

// TestAnalyze_MutualRecursion tests that mutually recursive functions
// don't infinite-loop due to the maxCallDepth limit.
func TestAnalyze_MutualRecursion(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func ping(x int) int {
			if x <= 0 {
				return x
			}
			return pong(x - 1)
		}

		func pong(x int) int {
			if x <= 0 {
				return x
			}
			return ping(x - 1)
		}
	`
	pkg := buildSSA(t, src)

	fn := findFunctionInPkg(pkg, "ping")
	require.NotNil(t, fn)

	analyzer := analysis.NewAnalyzer(nil)
	// Should terminate due to maxCallDepth, not hang.
	_ = analyzer.Analyze(fn)
}

// ---------------------------------------------------------------------------
// Edge case tests: NEQ refinement, mutual recursion, external functions, etc.
// ---------------------------------------------------------------------------
// TestAnalyze_NEQRefinement tests that x != constVal correctly refines in both branches.
func TestAnalyze_NEQRefinement(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
		message string
	}{
		"neq zero true branch excludes zero": {
			src: `
				package example

				func neqZero(x, y int) int {
					if y != 0 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "neqZero",
			wantLen: 0,
		},
		"eql zero false branch excludes zero": {
			src: `
				package example

				func eqlZero(x, y int) int {
					if y == 0 {
						return 0
					}
					return x / y
				}
			`,
			fnName:  "eqlZero",
			wantLen: 0,
		},
		"eql zero true branch narrows to zero": {
			src: `
				package example

				func eqlZeroTrue(x, y int) int {
					if y == 0 {
						return x / y
					}
					return 0
				}
			`,
			fnName:  "eqlZeroTrue",
			wantLen: 1,
			message: "division by zero",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn)

			analyzer := analysis.NewAnalyzer(nil)
			findings := analyzer.Analyze(fn)

			if tt.wantLen == 0 {
				require.Empty(t, findings)
			} else {
				require.Len(t, findings, tt.wantLen)
				require.Contains(t, findings[0].Message, tt.message)
			}
		})
	}
}

// TestAnalyze_NonIntConst exercises lookupInterval's handling of
// non-integer constants (nil, bool, string consts).
func TestAnalyze_NonIntConst(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func boolConst() bool {
			x := true
			return !x
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "boolConst")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_ShiftOp exercises transferBinOp's default case for
// shift operations (SHL, SHR) which are not handled explicitly.
func TestAnalyze_ShiftOp(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func shift(x int) int {
			return x << 2
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "shift")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// ---------------------------------------------------------------------------
// Coverage gap tests
// ---------------------------------------------------------------------------
// TestAnalyze_StringConcat exercises flagOverflow's early return for
// non-basic types (BinOp on string type → !ok at line 430).
func TestAnalyze_StringConcat(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func concat(a, b string) string {
			return a + b
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "concat")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_SwitchStatement tests switch compiled to If chains in SSA.
func TestAnalyze_SwitchStatement(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func switchDiv(x int) int {
			var d int
			switch x {
			case 1:
				d = 10
			case 2:
				d = 20
			default:
				d = 5
			}
			return 100 / d
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "switchDiv")
	require.NotNil(t, fn)

	analyzer := analysis.NewAnalyzer(nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestAnalyze_UnreachableBlock exercises isBlockReachable returning false
// for blocks that the worklist never visited (not in state map).
func TestAnalyze_UnreachableBlock(t *testing.T) {
	t.Parallel()
	src := `
		package example

		func unreachable(x int) int {
			if x > 0 {
				return x / 1
			}
			return x / 1
		}
	`
	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "unreachable")
	require.NotNil(t, fn)

	analyzer := &analysis.Analyzer{}
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

func TestAnalyzeCHA_InterfaceDispatch(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"single implementation returns nonzero, division safe": {
			fnName: "DivBySingleImpl",
			checks: nil,
		},

		"single implementation returns zero, division is bug": {
			fnName: "DivBySingleZeroImpl",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"two implementations both return nonzero, division safe": {
			fnName: "DivByMultiNonzero",
			checks: nil,
		},

		"two implementations both return zero, division is bug": {
			fnName: "DivByDualZero",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"two implementations, one returns zero one returns nonzero, division is warning": {
			fnName: "DivByMixedImpl",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"three implementations with mixed returns, join produces warning": {
			fnName: "DivByTriMixed",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"interface method with param propagation": {
			fnName: "CallComputerWithFive",
			checks: nil,
		},

		"pointer receiver, CHA resolves it": {
			fnName: "DivByPtrReceiver",
			checks: nil,
		},

		"embedded interface implementation": {
			fnName: "DivByEmbeddedIface",
			checks: nil,
		},

		"no implementations, call returns Top, division warns": {
			fnName: "DivByPhantom",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver, pkg := loadCHATestdata(t)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := analysis.NewAnalyzer(resolver)
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// TestAnalyzeCHA_StaticCallsStillWork verifies that the CHA resolver
// correctly handles direct (non-interface) function calls.
func TestAnalyzeCHA_StaticCallsStillWork(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"static call to nonzero callee is safe": {
			fnName: "DivBySafeCall",
			checks: nil,
		},
		"static call to zero callee is bug": {
			fnName: "DivByDecrement",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},
		"static call to constant callee is safe": {
			fnName: "DivByAlwaysTen",
			checks: nil,
		},
		"static call with possible zero return warns": {
			fnName: "DivByAbsOrZero",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolver, pkg := loadCHATestdata(t)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := analysis.NewAnalyzer(resolver)
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// TestAnalyzeConvertOverflow tests integer overflow detection on type
// conversion (narrowing) instructions.
func TestAnalyzeConvertOverflow(t *testing.T) {
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
		// === PROVEN OVERFLOW — source entirely outside target bounds ===

		"int16 300 to int8 proven overflow": {
			// [300, 300] entirely outside [-128, 127]
			src: `
				package example

				func convert300(x int16) int8 {
					x = 300
					return int8(x)
				}
			`,
			fnName:  "convert300",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},
		"int16 negative to int8 proven overflow": {
			// [-300, -300] entirely outside [-128, 127]
			src: `
				package example

				func convertNeg300(x int16) int8 {
					x = -300
					return int8(x)
				}
			`,
			fnName:  "convertNeg300",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},
		"int32 large to int16 proven overflow": {
			// [100000, 100000] entirely outside [-32768, 32767]
			src: `
				package example

				func convertLargeToInt16(x int32) int16 {
					x = 100000
					return int16(x)
				}
			`,
			fnName:  "convertLargeToInt16",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},
		"int32 large to int8 proven overflow": {
			// [1000, 1000] entirely outside [-128, 127]
			src: `
				package example

				func convertInt32ToInt8(x int32) int8 {
					x = 1000
					return int8(x)
				}
			`,
			fnName:  "convertInt32ToInt8",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},

		// === POSSIBLE OVERFLOW — source partially exceeds target bounds ===

		"int16 param to int8 possible overflow": {
			// int16 param is [-32768, 32767], which partially exceeds [-128, 127]
			src: `
				package example

				func convertParamToInt8(x int16) int8 {
					return int8(x)
				}
			`,
			fnName:  "convertParamToInt8",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},
		"int32 param to int16 possible overflow": {
			// int32 param is [-2147483648, 2147483647], partially exceeds int16
			src: `
				package example

				func convertInt32ParamToInt16(x int32) int16 {
					return int16(x)
				}
			`,
			fnName:  "convertInt32ParamToInt16",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},
		"int32 param to int8 possible overflow": {
			// int32 param partially exceeds int8
			src: `
				package example

				func convertInt32ParamToInt8(x int32) int8 {
					return int8(x)
				}
			`,
			fnName:  "convertInt32ParamToInt8",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},
		"int param to int8 possible overflow": {
			// int param is Top, doesn't fit int8
			src: `
				package example

				func convertIntToInt8(x int) int8 {
					return int8(x)
				}
			`,
			fnName:  "convertIntToInt8",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},
		"int64 param to int8 possible overflow": {
			// int64 param is Top, doesn't fit int8
			src: `
				package example

				func convertInt64ToInt8(x int64) int8 {
					return int8(x)
				}
			`,
			fnName:  "convertInt64ToInt8",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},

		// === SAFE — source fits within target bounds ===

		"int8 to int16 widening safe": {
			// int8 [-128, 127] fits within int16 [-32768, 32767]
			src: `
				package example

				func widenInt8ToInt16(x int8) int16 {
					return int16(x)
				}
			`,
			fnName:  "widenInt8ToInt16",
			wantLen: 0,
		},
		"int8 to int32 widening safe": {
			// int8 [-128, 127] fits within int32
			src: `
				package example

				func widenInt8ToInt32(x int8) int32 {
					return int32(x)
				}
			`,
			fnName:  "widenInt8ToInt32",
			wantLen: 0,
		},
		"int16 to int32 widening safe": {
			// int16 [-32768, 32767] fits within int32
			src: `
				package example

				func widenInt16ToInt32(x int16) int32 {
					return int32(x)
				}
			`,
			fnName:  "widenInt16ToInt32",
			wantLen: 0,
		},
		"int8 constant to int8 same width safe": {
			// [50, 50] fits within [-128, 127]
			src: `
				package example

				func sameWidthSafe(x int8) int8 {
					x = 50
					return int8(x)
				}
			`,
			fnName:  "sameWidthSafe",
			wantLen: 0,
		},
		"int16 small constant to int8 safe": {
			// [50, 50] fits within [-128, 127]
			src: `
				package example

				func smallConstToInt8(x int16) int8 {
					x = 50
					return int8(x)
				}
			`,
			fnName:  "smallConstToInt8",
			wantLen: 0,
		},
		"int16 negative constant to int8 safe": {
			// [-100, -100] fits within [-128, 127]
			src: `
				package example

				func negConstToInt8(x int16) int8 {
					x = -100
					return int8(x)
				}
			`,
			fnName:  "negConstToInt8",
			wantLen: 0,
		},
		"int16 at int8 max boundary safe": {
			// [127, 127] fits within [-128, 127]
			src: `
				package example

				func boundaryMaxSafe(x int16) int8 {
					x = 127
					return int8(x)
				}
			`,
			fnName:  "boundaryMaxSafe",
			wantLen: 0,
		},
		"int16 at int8 min boundary safe": {
			// [-128, -128] fits within [-128, 127]
			src: `
				package example

				func boundaryMinSafe(x int16) int8 {
					x = -128
					return int8(x)
				}
			`,
			fnName:  "boundaryMinSafe",
			wantLen: 0,
		},

		// === UNTRACKED TARGET — no finding ===

		"int8 to int widening untracked": {
			// int is untracked. No overflow check.
			src: `
				package example

				func int8ToInt(x int8) int {
					return int(x)
				}
			`,
			fnName:  "int8ToInt",
			wantLen: 0,
		},
		"int8 to int64 widening untracked": {
			// int64 is untracked. No overflow check.
			src: `
				package example

				func int8ToInt64(x int8) int64 {
					return int64(x)
				}
			`,
			fnName:  "int8ToInt64",
			wantLen: 0,
		},
		"int16 to int widening untracked": {
			src: `
				package example

				func int16ToInt(x int16) int {
					return int(x)
				}
			`,
			fnName:  "int16ToInt",
			wantLen: 0,
		},

		// === GUARDED CONVERSION — branch narrows source before convert ===

		"int16 guarded upper then convert to int8 safe": {
			// x < 100 refines int16 to [-32768, 99].
			// Hmm, that still partially exceeds int8 on the low end.
			// Need both guards.
			src: `
				package example

				func guardedConvert(x int16) int8 {
					if x > -128 {
						if x < 128 {
							return int8(x)
						}
					}
					return 0
				}
			`,
			fnName:  "guardedConvert",
			wantLen: 0,
		},
		"int16 guarded one side still warns": {
			// x < 100 refines to [-32768, 99]. Low end exceeds int8 [-128, 127].
			// Partial overlap → Warning.
			src: `
				package example

				func guardedOneSide(x int16) int8 {
					if x < 100 {
						return int8(x)
					}
					return 0
				}
			`,
			fnName:  "guardedOneSide",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},
		"int32 tightly guarded to int8 safe": {
			// x >= -128 && x <= 127 refines to [-128, 127]. Fits int8 exactly.
			src: `
				package example

				func tightGuardToInt8(x int32) int8 {
					if x >= -128 {
						if x <= 127 {
							return int8(x)
						}
					}
					return 0
				}
			`,
			fnName:  "tightGuardToInt8",
			wantLen: 0,
		},
		"int32 guarded to int16 safe": {
			// x >= -32768 && x <= 32767 refines to int16 range. Fits.
			src: `
				package example

				func guardedToInt16(x int32) int16 {
					if x >= -32768 {
						if x <= 32767 {
							return int16(x)
						}
					}
					return 0
				}
			`,
			fnName:  "guardedToInt16",
			wantLen: 0,
		},

		// === BOUNDARY — one past the limit ===

		"int16 one past int8 max proven overflow": {
			// [128, 128] entirely outside [-128, 127]
			src: `
				package example

				func onePastMax(x int16) int8 {
					x = 128
					return int8(x)
				}
			`,
			fnName:  "onePastMax",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},
		"int16 one past int8 min proven overflow": {
			// [-129, -129] entirely outside [-128, 127]
			src: `
				package example

				func onePastMin(x int16) int8 {
					x = -129
					return int8(x)
				}
			`,
			fnName:  "onePastMin",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},

		// === CHAINED CONVERSIONS ===

		"int32 to int16 to int8 double narrowing": {
			// int32 param → int16: possible overflow.
			// Then int16 result (still wide) → int8: possible overflow.
			src: `
				package example

				func doubleNarrow(x int32) int8 {
					y := int16(x)
					return int8(y)
				}
			`,
			fnName:  "doubleNarrow",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},

		// === CONVERT AFTER ARITHMETIC ===

		"arithmetic then narrow proven overflow": {
			// int16: 100 + 100 = [200, 200]. Convert to int8: [200, 200]
			// entirely outside [-128, 127]. Bug.
			src: `
				package example

				func arithThenNarrow() int8 {
					var a int16 = 100
					var b int16 = 100
					c := a + b
					return int8(c)
				}
			`,
			fnName:  "arithThenNarrow",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},
		"arithmetic then narrow safe": {
			// int16: 30 + 40 = [70, 70]. Convert to int8: fits [-128, 127]. Safe.
			src: `
				package example

				func arithThenNarrowSafe() int8 {
					var a int16 = 30
					var b int16 = 40
					c := a + b
					return int8(c)
				}
			`,
			fnName:  "arithThenNarrowSafe",
			wantLen: 0,
		},
		"arithmetic overflow then narrow both flag": {
			// int8: 100 + 100 = [200, 200]. Overflow on the add (Bug).
			// Convert int8 [200, 200] to int8 — also proven overflow in conversion (Bug).
			// Wait — converting int8 to int8 won't generate a Convert instruction.
			// Let's use int16 arithmetic that overflows int16, then narrow to int8.
			// int16: 30000 + 30000 = [60000, 60000]. Overflow on add for int16 (Bug).
			// Convert to int8: [60000, 60000] outside [-128, 127]. Bug.
			src: `
				package example

				func arithOverflowThenNarrow() int8 {
					var a int16 = 30000
					var b int16 = 30000
					c := a + b
					return int8(c)
				}
			`,
			fnName:  "arithOverflowThenNarrow",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},

		// === PHI NODE THEN CONVERT ===

		"phi merge then convert safe": {
			// Both branches assign int16 values within int8 range.
			// Phi joins [10, 10] and [20, 20] → [10, 20]. Fits int8. Safe.
			src: `
				package example

				func phiThenConvert(flag bool) int8 {
					var x int16 = 10
					if flag {
						x = 20
					}
					return int8(x)
				}
			`,
			fnName:  "phiThenConvert",
			wantLen: 0,
		},
		"phi merge then convert warns": {
			// Branches: [10, 10] and [200, 200] → [10, 200].
			// Partially exceeds int8 [-128, 127]. Warning.
			src: `
				package example

				func phiThenConvertWarns(flag bool) int8 {
					var x int16 = 10
					if flag {
						x = 200
					}
					return int8(x)
				}
			`,
			fnName:  "phiThenConvertWarns",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},

		// === LOOP THEN CONVERT ===

		"loop accumulator then convert warns": {
			// Loop accumulates int16 values. After widening, s is [0, MaxInt64].
			// The s += i triggers possible int16 overflow.
			// Convert to int8: partially exceeds. Warning.
			src: `
				package example

				func loopThenConvert(n int16) int8 {
					var s int16 = 0
					for i := int16(0); i < n; i++ {
						s += i
					}
					return int8(s)
				}
			`,
			fnName:  "loopThenConvert",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
				{analysis.Warning, "possible integer overflow in conversion"},
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

// ---------------------------------------------------------------------------
// Interprocedural analysis tests
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		// --- Proven bugs via interprocedural analysis ---

		"callee returns zero, caller divides by it": {
			src: `
				package example

				func decrement(x int) int {
					return x - 1
				}

				func caller() int {
					d := decrement(1)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"callee always returns zero literal": {
			src: `
				package example

				func zero() int {
					return 0
				}

				func caller(x int) int {
					return x / zero()
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		// --- Proven safe via interprocedural analysis ---

		"callee returns nonzero constant, division is safe": {
			src: `
				package example

				func ten() int {
					return 10
				}

				func caller(x int) int {
					return x / ten()
				}
			`,
			fnName: "caller",
			checks: nil, // no findings
		},

		"callee adds one to zero, result is 1, division safe": {
			src: `
				package example

				func addOne(x int) int {
					return x + 1
				}

				func caller() int {
					d := addOne(0)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: nil,
		},

		"callee with branch, known arg makes safe path": {
			src: `
				package example

				func isPositive(x int) int {
					if x > 0 {
						return 1
					}
					return 0
				}

				func caller() int {
					d := isPositive(5)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: nil, // isPositive(5) returns [1,1] — dead path pruned
		},

		"double call result is nonzero": {
			src: `
				package example

				func double(x int) int {
					return x * 2
				}

				func caller() int {
					d := double(5)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: nil, // double(5) = 10
		},

		// --- Warnings: callee may return zero ---

		"callee with branch, unknown arg, may return zero": {
			src: `
				package example

				func isPositive(x int) int {
					if x > 0 {
						return 1
					}
					return 0
				}

				func caller(x int) int {
					d := isPositive(x)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"callee with multiple return paths including zero": {
			src: `
				package example

				func absOrZero(x int) int {
					if x > 0 {
						return x
					}
					if x < 0 {
						return -x
					}
					return 0
				}

				func caller(x int) int {
					d := absOrZero(x)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"identity function passes through param, warns": {
			src: `
				package example

				func identity(x int) int {
					return x
				}

				func caller(x int) int {
					return 100 / identity(x)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		// --- Multi-level call chains ---

		"two-level chain: caller -> wrapper -> leaf": {
			src: `
				package example

				func leaf() int {
					return 7
				}

				func wrapper() int {
					return leaf()
				}

				func caller(x int) int {
					return x / wrapper()
				}
			`,
			fnName: "caller",
			checks: nil, // wrapper() -> leaf() = 7
		},

		"three-level chain returning zero": {
			src: `
				package example

				func bottom() int {
					return 0
				}

				func middle() int {
					return bottom()
				}

				func top() int {
					return middle()
				}

				func caller(x int) int {
					return x / top()
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		// --- Multiple calls in same function ---

		"two calls, one safe one unsafe": {
			src: `
				package example

				func alwaysOne() int {
					return 1
				}

				func alwaysZero() int {
					return 0
				}

				func caller(x int) int {
					a := x / alwaysOne()
					b := x / alwaysZero()
					return a + b
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"same function called with different args": {
			src: `
				package example

				func sub(a, b int) int {
					return a - b
				}

				func caller() int {
					safe := sub(10, 5)
					return 100 / safe
				}
			`,
			fnName: "caller",
			checks: nil, // sub(10, 5) = 5
		},

		"same function called with args producing zero": {
			src: `
				package example

				func sub(a, b int) int {
					return a - b
				}

				func caller() int {
					zero := sub(5, 5)
					return 100 / zero
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		// --- Call result used in arithmetic then division ---

		"call result used in further arithmetic": {
			src: `
				package example

				func five() int {
					return 5
				}

				func caller(x int) int {
					d := five() - 5
					return x / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"call result added to constant makes safe divisor": {
			src: `
				package example

				func zero() int {
					return 0
				}

				func caller(x int) int {
					d := zero() + 1
					return x / d
				}
			`,
			fnName: "caller",
			checks: nil, // 0 + 1 = 1
		},

		// --- Callee with no return value used as divisor ---

		"callee return not used as divisor, no finding": {
			src: `
				package example

				func helper() int {
					return 0
				}

				func caller(x, y int) int {
					_ = helper()
					return x + y
				}
			`,
			fnName: "caller",
			checks: nil,
		},

		// --- Context sensitivity: same callee, different contexts ---

		"context sensitive: same fn called with 0 and 5": {
			src: `
				package example

				func inc(x int) int {
					return x + 1
				}

				func caller() int {
					a := 100 / inc(0)
					b := 100 / inc(5)
					return a + b
				}
			`,
			fnName: "caller",
			checks: nil, // inc(0)=1, inc(5)=6, both safe
		},

		// --- Callee with loop ---

		"callee with loop returns nonzero": {
			// Widening overapproximates total to include 0, so a warning is sound.
			// Narrowing (not yet implemented) could recover precision here.
			src: `
				package example

				func sumTo(n int) int {
					total := 0
					for i := 1; i <= n; i++ {
						total += i
					}
					return total
				}

				func caller() int {
					return 100 / sumTo(10)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		// --- Callee with negation ---

		"callee negates input": {
			src: `
				package example

				func negate(x int) int {
					return -x
				}

				func caller() int {
					d := negate(0)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"callee negates nonzero is safe": {
			src: `
				package example

				func negate(x int) int {
					return -x
				}

				func caller() int {
					d := negate(3)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: nil, // negate(3) = -3
		},

		// --- Callee that the analyzer can't resolve (interface) ---

		// Note: interface method calls can't be tested with buildSSA
		// because the test helper uses Importer: nil which doesn't
		// support interface dispatch. Tested via cmd/goprove integration tests.

		// --- Multiple return values (only first used) ---

		// Note: Go SSA uses Extract for multi-return. This tests that
		// the analyzer doesn't crash on call instructions that return tuples.
		// The division uses a separate value, not the call result directly.
		"function with no division but calls another function": {
			src: `
				package example

				func helper(x int) int {
					return x * 2
				}

				func caller(x int) int {
					return helper(x) + 1
				}
			`,
			fnName: "caller",
			checks: nil,
		},

		// --- Guard against crashes ---

		"callee with no blocks (external function)": {
			src: `
				package example

				func caller(x int) int {
					return x + 1
				}
			`,
			fnName: "caller",
			checks: nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: caller guards call result with branch
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_CallerGuards(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"caller checks callee result != 0 before dividing": {
			src: `
				package example

				func maybeZero(x int) int {
					if x > 0 {
						return x
					}
					return 0
				}

				func caller(x int) int {
					d := maybeZero(x)
					if d != 0 {
						return 100 / d
					}
					return 0
				}
			`,
			fnName: "caller",
			checks: nil, // guarded by d != 0
		},

		"caller checks callee result > 0 before dividing": {
			src: `
				package example

				func compute(x int) int {
					return x - 5
				}

				func caller(x int) int {
					d := compute(x)
					if d > 0 {
						return 100 / d
					}
					return -1
				}
			`,
			fnName: "caller",
			checks: nil, // guarded by d > 0
		},

		"caller checks but divides in wrong branch": {
			src: `
				package example

				func compute(x int) int {
					return x - 5
				}

				func caller(x int) int {
					d := compute(x)
					if d == 0 {
						return 100 / d
					}
					return 0
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: callee with multiple return paths (complex control flow)
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_ComplexCalleeControlFlow(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"callee with 4 branches, all nonzero for known input": {
			src: `
				package example

				func classify(x int) int {
					if x > 100 {
						return 4
					}
					if x > 50 {
						return 3
					}
					if x > 0 {
						return 2
					}
					return 1
				}

				func caller() int {
					return 100 / classify(75)
				}
			`,
			fnName: "caller",
			checks: nil, // classify(75): x>100 false, x>50 true → returns 3
		},

		"callee with 4 branches, unknown input includes zero path": {
			src: `
				package example

				func risky(x int) int {
					if x > 100 {
						return 3
					}
					if x > 50 {
						return 2
					}
					if x > 0 {
						return 1
					}
					return 0
				}

				func caller(x int) int {
					return 100 / risky(x)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"callee with early return guard": {
			src: `
				package example

				func safeDiv(x, y int) int {
					if y == 0 {
						return 0
					}
					return x / y
				}

				func caller() int {
					return safeDiv(100, 0)
				}
			`,
			fnName: "caller",
			// safeDiv(100, 0): y=0, enters y==0 branch, returns 0.
			// The false branch (y!=0) is unreachable since ExcludeZero([0,0])=Bottom.
			// Caller doesn't divide by the result, so no finding.
			checks: nil,
		},

		"callee switches on parameter to return different values": {
			// pick(1): x=[1,1], x==1 true branch returns 10.
			// But the false branch of x==1 calls ExcludeZero (not Exclude(1)),
			// so x=[1,1] stays reachable in the false path. The "return 0"
			// path is not pruned. Known limitation: equality refinement
			// only excludes zero, not arbitrary constants.
			src: `
				package example

				func pick(x int) int {
					if x == 1 {
						return 10
					}
					if x == 2 {
						return 20
					}
					return 0
				}

				func caller() int {
					return 100 / pick(1)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: isBlockReachable tests
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_DeadBranchPruning(t *testing.T) {
	t.Parallel()

	// This specifically tests that unreachable branches in callees
	// don't pollute the return interval.
	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"true branch always taken, false return pruned": {
			src: `
				package example

				func check(x int) int {
					if x > 0 {
						return 1
					}
					return 0
				}

				func caller() int {
					return 100 / check(10)
				}
			`,
			fnName: "caller",
			checks: nil,
		},

		"false branch always taken, true return pruned": {
			src: `
				package example

				func check(x int) int {
					if x > 100 {
						return 0
					}
					return 1
				}

				func caller() int {
					return 100 / check(5)
				}
			`,
			fnName: "caller",
			checks: nil, // check(5): x=5, not > 100, returns 1
		},

		"equality check, true branch pruned": {
			// check(7): x=[7,7], condition x==0 → true branch gets
			// Meet([7,7],[0,0])=Bottom (unreachable). Only false branch
			// returns x=[7,7]. Division by 7 is safe.
			src: `
				package example

				func check(x int) int {
					if x == 0 {
						return 0
					}
					return x
				}

				func caller() int {
					return 100 / check(7)
				}
			`,
			fnName: "caller",
			checks: nil, // safe — dead branch pruned
		},

		"both branches reachable, returns joined": {
			src: `
				package example

				func check(x int) int {
					if x > 0 {
						return 1
					}
					return 0
				}

				func caller(x int) int {
					return 100 / check(x)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"nested branches, only deepest return reachable": {
			src: `
				package example

				func classify(x int) int {
					if x > 10 {
						if x > 20 {
							return 2
						}
						return 1
					}
					return 0
				}

				func caller() int {
					return 100 / classify(25)
				}
			`,
			fnName: "caller",
			checks: nil, // classify(25): x>10 true, x>20 true, returns 2
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: deep call chains (4+ levels)
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_DeepChains(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"three-level chain returning constant 3": {
			src: `
				package example

				func c() int { return 3 }
				func b() int { return c() }
				func a() int { return b() }

				func caller(x int) int {
					return x / a()
				}
			`,
			fnName: "caller",
			checks: nil, // a()->b()->c()=3, within maxCallDepth=3
		},

		"three-level chain returning zero": {
			src: `
				package example

				func c() int { return 0 }
				func b() int { return c() }
				func a() int { return b() }

				func caller(x int) int {
					return x / a()
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"three-level chain with arithmetic at each level": {
			src: `
				package example

				func c(x int) int { return x + 1 }
				func b(x int) int { return c(x) + 1 }
				func a(x int) int { return b(x) + 1 }

				func caller() int {
					return 100 / a(0)
				}
			`,
			fnName: "caller",
			checks: nil, // a(0)=b(0)+1=c(0)+2=0+1+2=3, within maxCallDepth=3
		},

		"deep chain where middle function zeroes out": {
			src: `
				package example

				func leaf(x int) int { return x }
				func middle(x int) int { return leaf(x) - x }
				func top(x int) int { return middle(x) }

				func caller() int {
					return 100 / top(5)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: callee findings should not leak into caller
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_FindingsIsolation(t *testing.T) {
	t.Parallel()

	// When a child analyzer finds issues inside the callee, those findings
	// should NOT appear in the caller's findings list. Each Analyze() call
	// produces findings only for the function being analyzed.
	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"callee has internal div-by-zero, caller is clean": {
			src: `
				package example

				func buggyCallee(x int) int {
					y := x - x
					_ = x / y
					return 1
				}

				func caller() int {
					d := buggyCallee(5)
					return 100 / d
				}
			`,
			fnName: "caller",
			// Only the caller's own findings. The callee's div-by-zero
			// should not appear here. d=1 so caller's division is safe.
			checks: nil,
		},

		"callee has overflow, caller only sees own issues": {
			src: `
				package example

				func overflowCallee() int8 {
					var x int8 = 127
					_ = x + 1
					return 10
				}

				func caller() int {
					d := int(overflowCallee())
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: nil, // caller is safe, callee overflow is callee's problem
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: function value calls (StaticCallee nil → Top)
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_FunctionValues(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"function passed as value, StaticCallee nil, returns Top": {
			src: `
				package example

				func apply(f func(int) int, x int) int {
					return f(x)
				}

				func double(x int) int {
					return x * 2
				}

				func caller() int {
					d := apply(double, 5)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: multiple parameters
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_MultipleParams(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"three-param function, known args, safe result": {
			src: `
				package example

				func combine(a, b, c int) int {
					return a + b + c
				}

				func caller() int {
					return 100 / combine(1, 2, 3)
				}
			`,
			fnName: "caller",
			checks: nil, // 1+2+3=6
		},

		"three-param function, args cancel to zero": {
			src: `
				package example

				func combine(a, b, c int) int {
					return a + b - c
				}

				func caller() int {
					return 100 / combine(3, 2, 5)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"param ordering matters": {
			src: `
				package example

				func sub(a, b int) int {
					return a - b
				}

				func caller() int {
					d := sub(3, 3)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"param ordering: reversed args are safe": {
			src: `
				package example

				func sub(a, b int) int {
					return a - b
				}

				func caller() int {
					d := sub(10, 3)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: nil, // 10-3=7
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: nested/chained calls (f(g(x)))
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_NestedCalls(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"f(g(x)) where g returns 0 and f adds 1": {
			src: `
				package example

				func g(x int) int { return 0 }
				func f(x int) int { return x + 1 }

				func caller() int {
					return 100 / f(g(5))
				}
			`,
			fnName: "caller",
			checks: nil, // f(g(5)) = f(0) = 1
		},

		"f(g(x)) where composition yields zero": {
			src: `
				package example

				func g(x int) int { return x + 1 }
				func f(x int) int { return x - 1 }

				func caller() int {
					return 100 / f(g(0))
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"triple nesting: f(g(h(x)))": {
			src: `
				package example

				func h(x int) int { return x * 2 }
				func g(x int) int { return x + 3 }
				func f(x int) int { return x - 1 }

				func caller() int {
					return 100 / f(g(h(1)))
				}
			`,
			fnName: "caller",
			checks: nil, // h(1)=2, g(2)=5, f(5)=4
		},

		"nested call as both arguments to a binary op": {
			src: `
				package example

				func inc(x int) int { return x + 1 }
				func dec(x int) int { return x - 1 }

				func caller() int {
					d := inc(2) - dec(4)
					return 100 / d
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: does not crash on edge cases
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_NoCrash(t *testing.T) {
	t.Parallel()

	// These tests verify the analyzer doesn't panic on tricky SSA patterns.
	tests := map[string]struct {
		src    string
		fnName string
	}{
		"callee with panic path": {
			src: `
				package example

				func mustPositive(x int) int {
					if x <= 0 {
						panic("must be positive")
					}
					return x
				}

				func caller() int {
					return 100 / mustPositive(5)
				}
			`,
			fnName: "caller",
		},

		"callee that calls builtin len": {
			src: `
				package example

				func caller(x int) int {
					return x + 1
				}
			`,
			fnName: "caller",
		},

		"deeply nested if-else in callee": {
			src: `
				package example

				func deep(a, b, c int) int {
					if a > 0 {
						if b > 0 {
							if c > 0 {
								return a + b + c
							}
							return a + b
						}
						return a
					}
					return 0
				}

				func caller() int {
					return 100 / deep(1, 2, 3)
				}
			`,
			fnName: "caller",
		},

		"callee with multiple params, some unused": {
			src: `
				package example

				func onlyFirst(a, b, c int) int {
					return a
				}

				func caller() int {
					return 100 / onlyFirst(5, 0, 0)
				}
			`,
			fnName: "caller",
		},

		"self-recursive with no base case that returns useful value": {
			src: `
				package example

				func infinite(x int) int {
					return infinite(x + 1)
				}

				func caller() int {
					return 100 / infinite(0)
				}
			`,
			fnName: "caller",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			// Must not panic — that's the only assertion here.
			analyzer := &analysis.Analyzer{}
			_ = analyzer.Analyze(fn)
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: overflow detection across call boundaries
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_OverflowAcrossCalls(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"callee returns near-max value, caller adds to it causing overflow": {
			src: `
				package example

				func nearMax() int8 {
					return 126
				}

				func caller() int8 {
					return nearMax() + 2
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},

		"callee returns safe value, caller adds within bounds": {
			src: `
				package example

				func small() int8 {
					return 10
				}

				func caller() int8 {
					return small() + 5
				}
			`,
			fnName: "caller",
			checks: nil,
		},

		"callee returns near-min value, caller subtracts causing underflow": {
			src: `
				package example

				func nearMin() int8 {
					return -127
				}

				func caller() int8 {
					return nearMin() - 2
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow"},
			},
		},

		"callee returns value, conversion to narrower type overflows": {
			src: `
				package example

				func big() int {
					return 200
				}

				func caller() int8 {
					return int8(big())
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in conversion"},
			},
		},

		"callee returns value within narrow type bounds, conversion safe": {
			src: `
				package example

				func small() int {
					return 50
				}

				func caller() int8 {
					return int8(small())
				}
			`,
			fnName: "caller",
			checks: nil,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: recursive / depth-limit tests
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_Recursion(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"direct recursion with concrete arg within depth limit": {
			// factorial(2) recurses 2 levels (within maxCallDepth=3).
			// Each level has a concrete arg, base case n<=1 is reached.
			// Result is precise: 2. Division is safe.
			src: `
				package example

				func factorial(n int) int {
					if n <= 1 {
						return 1
					}
					return n * factorial(n - 1)
				}

				func caller() int {
					return 100 / factorial(2)
				}
			`,
			fnName: "caller",
			checks: nil, // factorial(2)=2, no zero
		},

		"direct recursion exceeding depth limit warns": {
			// factorial(5) recurses 5 levels, exceeding maxCallDepth=3.
			// Hits depth limit → returns Top → includes zero → warns.
			src: `
				package example

				func factorial(n int) int {
					if n <= 1 {
						return 1
					}
					return n * factorial(n - 1)
				}

				func caller() int {
					return 100 / factorial(5)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},

		"mutual recursion with concrete arg within depth limit": {
			// pingPong(1)→pong(0)→2.
			// Only 2 levels deep. Terminates before depth limit.
			src: `
				package example

				func pingPong(n int) int {
					if n <= 0 {
						return 1
					}
					return pong(n - 1)
				}

				func pong(n int) int {
					if n <= 0 {
						return 2
					}
					return pingPong(n - 1)
				}

				func caller() int {
					return 100 / pingPong(1)
				}
			`,
			fnName: "caller",
			checks: nil, // terminates naturally, returns nonzero
		},

		"tail recursive countdown with concrete arg within depth limit": {
			// countdown(2)→countdown(1)→countdown(0)→42.
			// Only 3 levels. Terminates within depth limit.
			src: `
				package example

				func countdown(n int) int {
					if n <= 0 {
						return 42
					}
					return countdown(n - 1)
				}

				func caller() int {
					return 100 / countdown(2)
				}
			`,
			fnName: "caller",
			checks: nil, // countdown(2)=42
		},

		"recursion with unknown arg hits depth limit, warns": {
			// With a wide-range param, both branches are reachable
			// at every level. Eventually hits depth limit → Top.
			src: `
				package example

				func recur(n int) int {
					if n <= 0 {
						return 1
					}
					return recur(n - 1)
				}

				func caller(x int) int {
					return 100 / recur(x)
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: summary caching
// ---------------------------------------------------------------------------
func TestAnalyzeInterprocedural_SummaryCaching(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
		checks []struct {
			severity analysis.Severity
			message  string
		}
	}{
		"same function called 3 times with same args, reuses summary": {
			src: `
				package example

				func ten() int { return 10 }

				func caller(x int) int {
					a := x / ten()
					b := x / ten()
					c := x / ten()
					return a + b + c
				}
			`,
			fnName: "caller",
			checks: nil, // all safe, ten()=10
		},

		"same function called with different args producing different summaries": {
			src: `
				package example

				func sub1(x int) int { return x - 1 }

				func caller() int {
					safe := sub1(5)    // = 4
					zero := sub1(1)    // = 0
					a := 100 / safe
					b := 100 / zero
					return a + b
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},

		"callee called from two different call sites, each with different constant": {
			src: `
				package example

				func double(x int) int { return x * 2 }

				func caller() int {
					a := double(0)   // = 0
					b := double(3)   // = 6
					return b / a
				}
			`,
			fnName: "caller",
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "division by zero"},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(pkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := &analysis.Analyzer{}
			findings := analyzer.Analyze(fn)

			if tt.checks == nil {
				require.Empty(t, findings,
					"expected no findings, got %d: %+v", len(findings), findings)
				return
			}

			require.Len(t, findings, len(tt.checks),
				"expected %d findings, got %d: %+v", len(tt.checks), len(findings), findings)

			for i, check := range tt.checks {
				require.Equal(t, check.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Contains(t, findings[i].Message, check.message,
					"finding[%d] message mismatch", i)
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

func TestAnalyzeNegationOverflow(t *testing.T) {
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
		// === Proven overflow ===

		"negate int8 min constant proven overflow": {
			// -(-128) = 128, but int8 max is 127. Entirely outside bounds.
			src: `
				package example

				func negInt8Min() int8 {
					var x int8 = -128
					return -x
				}
			`,
			fnName:  "negInt8Min",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in negation"},
			},
		},
		"negate int16 min constant proven overflow": {
			// -(-32768) = 32768, but int16 max is 32767.
			src: `
				package example

				func negInt16Min() int16 {
					var x int16 = -32768
					return -x
				}
			`,
			fnName:  "negInt16Min",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in negation"},
			},
		},
		"negate int32 min constant proven overflow": {
			// -(-2147483648) = 2147483648, but int32 max is 2147483647.
			src: `
				package example

				func negInt32Min() int32 {
					var x int32 = -2147483648
					return -x
				}
			`,
			fnName:  "negInt32Min",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Bug, "proven integer overflow in negation"},
			},
		},

		// === Possible overflow (param full range) ===

		"negate int8 param possible overflow": {
			// x is [-128, 127]. -x gives [-127, 128]. 128 > 127. Partial overlap.
			src: `
				package example

				func negInt8Param(x int8) int8 {
					return -x
				}
			`,
			fnName:  "negInt8Param",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},
		"negate int16 param possible overflow": {
			// x is [-32768, 32767]. -x gives [-32767, 32768]. Partial overlap.
			src: `
				package example

				func negInt16Param(x int16) int16 {
					return -x
				}
			`,
			fnName:  "negInt16Param",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},
		"negate int32 param possible overflow": {
			// x is [-2147483648, 2147483647]. -x gives [-2147483647, 2147483648]. Partial overlap.
			src: `
				package example

				func negInt32Param(x int32) int32 {
					return -x
				}
			`,
			fnName:  "negInt32Param",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},

		// === Safe: constants that fit ===

		"negate positive int8 constant safe": {
			// -(127) = -127. Fits in [-128, 127].
			src: `
				package example

				func negPos127() int8 {
					var x int8 = 127
					return -x
				}
			`,
			fnName:  "negPos127",
			wantLen: 0,
		},
		"negate negative int8 constant safe": {
			// -(-127) = 127. Fits in [-128, 127].
			src: `
				package example

				func negNeg127() int8 {
					var x int8 = -127
					return -x
				}
			`,
			fnName:  "negNeg127",
			wantLen: 0,
		},
		"negate zero int8 safe": {
			// -(0) = 0. Always safe.
			src: `
				package example

				func negZero() int8 {
					var x int8 = 0
					return -x
				}
			`,
			fnName:  "negZero",
			wantLen: 0,
		},
		"negate one int8 safe": {
			// -(1) = -1. Safe.
			src: `
				package example

				func negOne() int8 {
					var x int8 = 1
					return -x
				}
			`,
			fnName:  "negOne",
			wantLen: 0,
		},
		"negate positive int16 constant safe": {
			// -(100) = -100. Fits in int16.
			src: `
				package example

				func negPos100() int16 {
					var x int16 = 100
					return -x
				}
			`,
			fnName:  "negPos100",
			wantLen: 0,
		},
		"negate int32 small constant safe": {
			// -(42) = -42. Fits in int32.
			src: `
				package example

				func negSmall32() int32 {
					var x int32 = 42
					return -x
				}
			`,
			fnName:  "negSmall32",
			wantLen: 0,
		},

		// === Safe: guarded range ===

		"negate int8 param guarded above min safe": {
			// x > -128 narrows to [-127, 127]. -x gives [-127, 127]. Safe.
			src: `
				package example

				func negGuarded(x int8) int8 {
					if x > -128 {
						return -x
					}
					return 0
				}
			`,
			fnName:  "negGuarded",
			wantLen: 0,
		},
		"negate int8 param guarded positive safe": {
			// x > 0 narrows to [1, 127]. -x gives [-127, -1]. Safe.
			src: `
				package example

				func negGuardedPos(x int8) int8 {
					if x > 0 {
						return -x
					}
					return 0
				}
			`,
			fnName:  "negGuardedPos",
			wantLen: 0,
		},
		"negate int16 param guarded above min safe": {
			// x > -32768 narrows to [-32767, 32767]. -x gives [-32767, 32767]. Safe.
			src: `
				package example

				func negGuarded16(x int16) int16 {
					if x > -32768 {
						return -x
					}
					return 0
				}
			`,
			fnName:  "negGuarded16",
			wantLen: 0,
		},
		"negate int32 param guarded above min safe": {
			// x > -2147483648 narrows to [-2147483647, 2147483647]. -x gives [-2147483647, 2147483647]. Safe.
			src: `
				package example

				func negGuarded32(x int32) int32 {
					if x > -2147483648 {
						return -x
					}
					return 0
				}
			`,
			fnName:  "negGuarded32",
			wantLen: 0,
		},

		// === Untracked types: no findings ===

		"negate int param untracked": {
			// int is not tracked for overflow. No findings.
			src: `
				package example

				func negInt(x int) int {
					return -x
				}
			`,
			fnName:  "negInt",
			wantLen: 0,
		},
		"negate int64 param untracked": {
			// int64 is not tracked. No findings.
			src: `
				package example

				func negInt64(x int64) int64 {
					return -x
				}
			`,
			fnName:  "negInt64",
			wantLen: 0,
		},

		// === Boundary: one past min ===

		"negate int8 one past min safe": {
			// x = -127. -(-127) = 127. Fits in int8.
			src: `
				package example

				func negOnePastMin8() int8 {
					var x int8 = -127
					return -x
				}
			`,
			fnName:  "negOnePastMin8",
			wantLen: 0,
		},
		"negate int16 one past min safe": {
			// x = -32767. -(-32767) = 32767. Fits in int16.
			src: `
				package example

				func negOnePastMin16() int16 {
					var x int16 = -32767
					return -x
				}
			`,
			fnName:  "negOnePastMin16",
			wantLen: 0,
		},

		// === Negation then arithmetic ===

		"negate then add overflow": {
			// x is int8 param [-128, 127]. -x gives [-127, 128].
			// Negation warns. Then -x + 1 gives [-126, 129]. Addition also warns.
			src: `
				package example

				func negThenAdd(x int8) int8 {
					return -x + 1
				}
			`,
			fnName:  "negThenAdd",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
				{analysis.Warning, "possible integer overflow"},
			},
		},
		"negate guarded then add safe": {
			// x > -128 → [-127, 127]. -x → [-127, 127]. Safe negation.
			// -x + 1 → [-126, 128]. Partially exceeds. Addition warns.
			src: `
				package example

				func negGuardedThenAdd(x int8) int8 {
					if x > -128 {
						return -x + 1
					}
					return 0
				}
			`,
			fnName:  "negGuardedThenAdd",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow"},
			},
		},

		// === Negation then conversion ===

		"negate int16 param then convert to int8 both warn": {
			// x is int16 [-32768, 32767]. -x gives [-32767, 32768].
			// Negation warns (partial overlap with int16).
			// Convert to int8: [-32767, 32768] vs [-128, 127]. Partial overlap. Warns.
			src: `
				package example

				func negThenConvert(x int16) int8 {
					return int8(-x)
				}
			`,
			fnName:  "negThenConvert",
			wantLen: 2,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
				{analysis.Warning, "possible integer overflow in conversion"},
			},
		},

		// === Double negation ===

		"double negate int8 param warns once": {
			// x is [-128, 127]. -x → [-127, 128]. First negation warns.
			// -(-x) → [-128, 127]. Second negation is safe (fits in int8).
			src: `
				package example

				func doubleNeg(x int8) int8 {
					y := -x
					return -y
				}
			`,
			fnName:  "doubleNeg",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},

		// === Phi then negate ===

		"phi merge then negate safe": {
			// if cond: x = 10, else: x = -10. Phi gives [-10, 10].
			// -x gives [-10, 10]. Safe for int8.
			src: `
				package example

				func phiThenNeg(cond bool) int8 {
					var x int8
					if cond {
						x = 10
					} else {
						x = -10
					}
					return -x
				}
			`,
			fnName:  "phiThenNeg",
			wantLen: 0,
		},
		"phi merge then negate warns": {
			// if cond: x = 127, else: x = -128. Phi gives [-128, 127].
			// -x gives [-127, 128]. Partial overlap. Warns.
			src: `
				package example

				func phiThenNegWarn(cond bool) int8 {
					var x int8
					if cond {
						x = 127
					} else {
						x = -128
					}
					return -x
				}
			`,
			fnName:  "phiThenNegWarn",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},

		// === Negation in branch ===

		"negate in true branch guarded safe": {
			// In true branch: x >= 0 → [0, 127]. -x → [-127, 0]. Safe.
			src: `
				package example

				func negInBranch(x int8) int8 {
					if x >= 0 {
						return -x
					}
					return x
				}
			`,
			fnName:  "negInBranch",
			wantLen: 0,
		},
		"negate in false branch unguarded warns": {
			// In false branch: x < 0 → [-128, -1]. -x → [1, 128]. 128 > 127. Warns.
			src: `
				package example

				func negFalseBranch(x int8) int8 {
					if x >= 0 {
						return x
					}
					return -x
				}
			`,
			fnName:  "negFalseBranch",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},

		// === Absolute value pattern ===

		"abs pattern on int8 warns": {
			// if x < 0: return -x (x is [-128, -1], -x is [1, 128]. Warns.)
			// else: return x
			src: `
				package example

				func absInt8(x int8) int8 {
					if x < 0 {
						return -x
					}
					return x
				}
			`,
			fnName:  "absInt8",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
		},
		"abs pattern on int8 guarded safe": {
			// if x < 0 && x > -128: return -x (x is [-127, -1], -x is [1, 127]. Safe.)
			// Needs two guards. Our analyzer refines per-predecessor, so
			// x < 0 gives [-128, -1], then x > -128 gives [-127, -1].
			// -x gives [1, 127]. Safe.
			src: `
				package example

				func absInt8Safe(x int8) int8 {
					if x > -128 {
						if x < 0 {
							return -x
						}
					}
					return x
				}
			`,
			fnName:  "absInt8Safe",
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
		"int8 param negation warns": {
			// x is [-128, 127]. -x gives [-127, 128].
			// 128 exceeds int8 max (127). Partial overlap — Warning.
			src: `
				package example

				func int8Neg(x int8) int8 {
					return -x
				}
			`,
			fnName:  "int8Neg",
			wantLen: 1,
			checks: []struct {
				severity analysis.Severity
				message  string
			}{
				{analysis.Warning, "possible integer overflow in negation"},
			},
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

// ---------------------------------------------------------------------------
// buildSSA importer: verify that imported types resolve
// ---------------------------------------------------------------------------
// Note: interface dispatch through imported packages (e.g. fmt.Stringer)
// does NOT work with the low-level SSA construction in buildSSA, because
// imported packages are created as stubs (nil AST). The production loader
// (packages.Load) handles this correctly. These tests verify that imported
// constants and type checks work.
func TestBuildSSA_MathImportResolves(t *testing.T) {
	t.Parallel()

	// Uses math.MaxInt32 — verifies that importing "math" works.
	// With Importer: nil, type-checking would fail.
	src := `
		package example

		import "math"

		func clamp(x int) int {
			if x > math.MaxInt32 {
				return math.MaxInt32
			}
			return x
		}

		func caller() int {
			return 100 / clamp(5)
		}
	`

	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "caller")
	require.NotNil(t, fn, "function caller not found")

	analyzer := &analysis.Analyzer{}
	_ = analyzer.Analyze(fn)
}

func TestBuildSSA_MultipleImportsResolve(t *testing.T) {
	t.Parallel()

	// Uses both math and strconv — verifies multiple imports resolve.
	src := `
		package example

		import "math"

		func bounded(x int) int {
			if x > math.MaxInt8 {
				return math.MaxInt8
			}
			if x < math.MinInt8 {
				return math.MinInt8
			}
			return x
		}

		func caller() int {
			return 100 / bounded(5)
		}
	`

	pkg := buildSSA(t, src)
	fn := findFunctionInPkg(pkg, "caller")
	require.NotNil(t, fn, "function caller not found")

	analyzer := &analysis.Analyzer{}
	_ = analyzer.Analyze(fn)
}

// ---------------------------------------------------------------------------
// CHA Resolver tests — interface dispatch and multi-callee join
// ---------------------------------------------------------------------------
// These tests use loader.Load with testdata/interfaces.go because the
// low-level buildSSA helper panics on method calls (SSA builder can't
// handle method wrappers without the full packages.Load pipeline).
func loadCHATestdata(t *testing.T) (*analysis.CHAResolver, *ssa.Package) {
	t.Helper()
	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	resolver := analysis.NewCHAResolver(prog)
	return resolver, pkgs[0]
}
