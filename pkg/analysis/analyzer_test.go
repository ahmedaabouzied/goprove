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
