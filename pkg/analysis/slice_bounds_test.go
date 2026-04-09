package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

func TestSliceBounds(t *testing.T) {
	t.Parallel()

	type check struct {
		severity analysis.Severity
		message  string
	}

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
		checks  []check
	}{
		// ── Proven OOB (Bug) ──────────────────────────────────────────

		"constant index exceeds make length": {
			src: `
				package example

				func f() int {
					s := make([]int, 5)
					return s[10]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},
		"constant index exceeds literal length": {
			src: `
				package example

				func f() int {
					s := []int{1, 2, 3}
					return s[5]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},
		"index equals length exactly is OOB": {
			src: `
				package example

				func f() int {
					s := make([]int, 3)
					return s[3]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},
		"make length 1 index 1 is OOB": {
			src: `
				package example

				func f() int {
					s := make([]int, 1)
					return s[1]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},
		"reslice then OOB index": {
			src: `
				package example

				func f() int {
					s := []int{1, 2, 3, 4, 5}
					t := s[1:4]
					return t[3]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},
		"make via variable then OOB": {
			src: `
				package example

				func f() int {
					n := 10
					s := make([]int, n)
					return s[10]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},

		// ── Safe accesses (no findings) ───────────────────────────────

		"constant index within make length": {
			src: `
				package example

				func f() int {
					s := make([]int, 5)
					return s[2]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"index zero on make length 1": {
			src: `
				package example

				func f() int {
					s := make([]int, 1)
					return s[0]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"last index of make": {
			src: `
				package example

				func f() int {
					s := make([]int, 5)
					return s[4]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"constant index within literal length": {
			src: `
				package example

				func f() int {
					s := []int{10, 20, 30, 40, 50}
					return s[4]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"index zero on literal": {
			src: `
				package example

				func f() int {
					s := []int{1, 2, 3}
					return s[0]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"last index of literal": {
			src: `
				package example

				func f() int {
					s := []int{1, 2, 3}
					return s[2]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"reslice then safe index": {
			src: `
				package example

				func f() int {
					s := []int{1, 2, 3, 4, 5}
					t := s[1:4]
					return t[2]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"reslice index zero": {
			src: `
				package example

				func f() int {
					s := []int{1, 2, 3, 4, 5}
					t := s[1:4]
					return t[0]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"make via variable then safe index": {
			src: `
				package example

				func f() int {
					n := 10
					s := make([]int, n)
					return s[9]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},

		// ── Append tracking ───────────────────────────────────────────

		"append then index last via len": {
			src: `
				package example

				func f(s []int, v int) int {
					s = append(s, v)
					return s[len(s)-1]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"append single then index zero": {
			src: `
				package example

				func f() int {
					var s []int
					s = append(s, 42)
					return s[0]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"double append then index one": {
			src: `
				package example

				func f() int {
					var s []int
					s = append(s, 1)
					s = append(s, 2)
					return s[1]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"double append from nil then OOB is silent": {
			// After append to nil slice, upper bound is MaxInt64
			// so we can't prove index 5 is OOB.
			src: `
				package example

				func f() int {
					var s []int
					s = append(s, 1)
					s = append(s, 2)
					return s[5]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"append to known length then OOB": {
			src: `
				package example

				func f() int {
					s := make([]int, 0)
					s = append(s, 1)
					s = append(s, 2)
					return s[5]
				}
			`,
			fnName:  "f",
			wantLen: 1,
			checks:  []check{{analysis.Bug, "slice out of bound access"}},
		},

		// ── Range loop safety ─────────────────────────────────────────

		"range loop index is always safe": {
			src: `
				package example

				func f(s []int) int {
					total := 0
					for i := range s {
						total += s[i]
					}
					return total
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"range loop with value is safe": {
			src: `
				package example

				func f(s []int) int {
					total := 0
					for _, v := range s {
						total += v
					}
					return total
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},

		// ── Bounds-checked access ─────────────────────────────────────

		"if i >= 0 && i < len(s) then safe": {
			src: `
				package example

				func f(s []int, i int) int {
					if i >= 0 && i < len(s) {
						return s[i]
					}
					return 0
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},

		// ── Unknown / suppressed warnings ─────────────────────────────

		"unchecked param index on param slice is silent": {
			src: `
				package example

				func f(s []int, i int) int {
					return s[i]
				}
			`,
			fnName:  "f",
			wantLen: 0,
		},
		"param index on make slice is silent": {
			src: `
				package example

				func f(i int) int {
					s := make([]int, 10)
					return s[i]
				}
			`,
			fnName:  "f",
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

			for i, c := range tt.checks {
				require.Equal(t, c.severity, findings[i].Severity,
					"finding[%d] severity mismatch", i)
				require.Equal(t, c.message, findings[i].Message,
					"finding[%d] message mismatch", i)
			}
		})
	}
}
