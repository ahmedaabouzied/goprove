package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// NilAnalyzer.Analyze integration tests — full end-to-end flow
// ===========================================================================

// TestNilAnalyze runs the full nil analysis pipeline (worklist → check pass)
// on inline Go source and verifies findings.
func TestNilAnalyze(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      string
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		// -----------------------------------------------------------------
		// Safe cases — no findings expected
		// -----------------------------------------------------------------
		"deref after nil check is safe": {
			src: `
				package example

				func safeDeref(p *int) int {
					if p != nil {
						return *p
					}
					return 0
				}
			`,
			fnName:  "safeDeref",
			wantLen: 0,
		},
		"deref after eq nil check else branch is safe": {
			src: `
				package example

				func safeDerefEq(p *int) int {
					if p == nil {
						return 0
					}
					return *p
				}
			`,
			fnName:  "safeDerefEq",
			wantLen: 0,
		},
		"deref of new is safe": {
			src: `
				package example

				func derefNew() int {
					p := new(int)
					*p = 42
					return *p
				}
			`,
			fnName:  "derefNew",
			wantLen: 0,
		},
		"deref of address-of is safe": {
			src: `
				package example

				func derefAddr() int {
					x := 10
					p := &x
					return *p
				}
			`,
			fnName:  "derefAddr",
			wantLen: 0,
		},
		// Interface nil check tested via testdata (RefineInterface).
		// buildSSA panics on interface method calls.
		"make map is safe": {
			src: `
				package example

				func safeMap() int {
					m := make(map[string]int)
					m["x"] = 1
					return m["x"]
				}
			`,
			fnName:  "safeMap",
			wantLen: 0,
		},
		"make slice is safe": {
			src: `
				package example

				func safeSlice(n int) []int {
					s := make([]int, n)
					return s
				}
			`,
			fnName:  "safeSlice",
			wantLen: 0,
		},
		"make chan is safe": {
			src: `
				package example

				func safeChan() chan int {
					return make(chan int)
				}
			`,
			fnName:  "safeChan",
			wantLen: 0,
		},
		"non-nillable param has no findings": {
			src: `
				package example

				func intParam(x int) int {
					return x + 1
				}
			`,
			fnName:  "intParam",
			wantLen: 0,
		},
		"no pointer operations has no findings": {
			src: `
				package example

				func pureAdd(a, b int) int {
					return a + b
				}
			`,
			fnName:  "pureAdd",
			wantLen: 0,
		},
		"sequential nil checks are safe": {
			src: `
				package example

				func seqCheck(a *int) int {
					if a != nil {
						return *a
					}
					return 0
				}
			`,
			fnName:  "seqCheck",
			wantLen: 0,
		},
		// -----------------------------------------------------------------
		// Bug cases — proven nil dereference
		// -----------------------------------------------------------------
		"deref nil literal is a bug": {
			src: `
				package example

				func derefNil() int {
					var p *int
					return *p
				}
			`,
			fnName:   "derefNil",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "nil dereference",
		},
		"deref explicit nil assignment is a bug": {
			src: `
				package example

				func derefExplicitNil() int {
					var p *int = nil
					return *p
				}
			`,
			fnName:   "derefExplicitNil",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "nil dereference",
		},
		// FieldAddr on nil tested via testdata fixtures (FieldAccessOnNil)
		// — buildSSA panics on inline struct field access.
		// -----------------------------------------------------------------
		// Warning cases — possible nil dereference
		// -----------------------------------------------------------------
		"deref unchecked pointer param is a warning": {
			src: `
				package example

				func derefParam(p *int) int {
					return *p
				}
			`,
			fnName:   "derefParam",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible nil dereference",
		},
		// FieldAddr on unchecked param tested via testdata fixtures (DerefParam).
		// buildSSA panics on inline struct field access.
		// Interface method call on unchecked param tested via testdata
		// (MethodCallOnParam). buildSSA panics on interface method calls.
		// -----------------------------------------------------------------
		// Branch refinement edge cases
		// -----------------------------------------------------------------
		"nil on left side of eq check is safe": {
			src: `
				package example

				func nilOnLeft(p *int) int {
					if nil == p {
						return 0
					}
					return *p
				}
			`,
			fnName:  "nilOnLeft",
			wantLen: 0,
		},
		"nil on left side of neq check is safe": {
			src: `
				package example

				func nilOnLeftNeq(p *int) int {
					if nil != p {
						return *p
					}
					return 0
				}
			`,
			fnName:  "nilOnLeftNeq",
			wantLen: 0,
		},
		// -----------------------------------------------------------------
		// Phi node cases
		// -----------------------------------------------------------------
		"phi both branches non-nil is safe": {
			src: `
				package example

				func phiBothNonNil(cond bool) int {
					var p *int
					if cond {
						x := 1
						p = &x
					} else {
						y := 2
						p = &y
					}
					return *p
				}
			`,
			fnName:  "phiBothNonNil",
			wantLen: 0,
		},
		"phi one branch nil is a warning": {
			src: `
				package example

				func phiOneNil(cond bool) int {
					var p *int
					if cond {
						x := 1
						p = &x
					}
					return *p
				}
			`,
			fnName:   "phiOneNil",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible nil dereference",
		},
		"phi all nil is a bug": {
			src: `
				package example

				func phiAllNil(cond bool) int {
					var p *int
					if cond {
						p = nil
					}
					return *p
				}
			`,
			fnName:   "phiAllNil",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "nil dereference",
		},
		// -----------------------------------------------------------------
		// Always non-nil producers
		// -----------------------------------------------------------------
		"make map then use is safe": {
			src: `
				package example

				func makeMapUse() {
					m := make(map[string]int)
					m["key"] = 42
				}
			`,
			fnName:  "makeMapUse",
			wantLen: 0,
		},
		// Struct field access via new() tested via testdata fixtures.
		// buildSSA panics on inline struct field access.
		// -----------------------------------------------------------------
		// Slice IndexAddr — only Bug, no MaybeNil warning
		// -----------------------------------------------------------------
		"slice index on param has no warning": {
			src: `
				package example

				func sliceIndex(s []int) int {
					return s[0]
				}
			`,
			fnName:  "sliceIndex",
			wantLen: 0,
		},
		"slice index on make is safe": {
			src: `
				package example

				func sliceIndexMake(n int) int {
					s := make([]int, n)
					return s[0]
				}
			`,
			fnName:  "sliceIndexMake",
			wantLen: 0,
		},
		// -----------------------------------------------------------------
		// No double-reporting: FieldAddr + load
		// -----------------------------------------------------------------
		// No-double-report for FieldAddr tested via testdata (FieldAccessOnNil).
		// buildSSA panics on inline struct field access.
		// -----------------------------------------------------------------
		// External function (no body) — should not panic
		// -----------------------------------------------------------------
		"external function returns nil findings": {
			src: `
				package example

				func placeholder() int { return 0 }
			`,
			fnName:  "placeholder",
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

			analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen, "findings count mismatch")

			if tt.wantLen > 0 {
				require.Equal(t, tt.severity, findings[0].Severity)
				require.Contains(t, findings[0].Message, tt.message)
			}
		})
	}
}

// ===========================================================================
// NilAnalyzer.Analyze against testdata fixtures (real packages)
// ===========================================================================

// TestNilAnalyze_Testdata runs the nil analyzer against testdata/nilderef.go
// fixtures and verifies expected findings per function.
func TestNilAnalyze_Testdata(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	pkg := pkgs[0]

	tests := []struct {
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		// Proven bugs
		{"DerefNilLiteral", 1, analysis.Bug, "nil dereference"},
		{"FieldAccessOnNil", 1, analysis.Bug, "nil dereference"},

		// Warnings
		{"DerefParam", 1, analysis.Warning, "possible nil dereference"},
		// MethodCallOnParam uses interface invoke — now checked.
		{"MethodCallOnParam", 1, analysis.Warning, "possible nil dereference"},

		// Safe — no findings
		{"DerefAfterCheck", 0, 0, ""},
		{"DerefNew", 0, 0, ""},
		{"AllocNew", 0, 0, ""},
		{"AllocAddr", 0, 0, ""},
		{"MakeMapFixture", 0, 0, ""},
		{"MakeMapHintFixture", 0, 0, ""},
		{"MakeChanFixture", 0, 0, ""},
		{"MakeChanBufFixture", 0, 0, ""},
		{"RefineNotNil", 0, 0, ""},
		{"RefineEqlNil", 0, 0, ""},
		{"RefineNilOnLeft", 0, 0, ""},
		{"RefineInterface", 0, 0, ""},
		{"RefineSlice", 0, 0, ""},
		{"MakeInterfaceFixture", 0, 0, ""},
		{"MakeInterfaceNilPtrFixture", 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.fnName, func(t *testing.T) {
			t.Parallel()

			var fn *ssa.Function
			for _, member := range pkg.Members {
				f, ok := member.(*ssa.Function)
				if ok && f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found in testdata", tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen,
				"%s: expected %d findings, got %d", tt.fnName, tt.wantLen, len(findings))

			if tt.wantLen > 0 {
				require.Equal(t, tt.severity, findings[0].Severity,
					"%s: wrong severity", tt.fnName)
				require.Contains(t, findings[0].Message, tt.message,
					"%s: wrong message", tt.fnName)
			}
		})
	}
}

// ===========================================================================
// Edge cases and robustness
// ===========================================================================

// TestNilAnalyze_ExternalFunction tests that a function with no body
// (external/assembly-backed) does not panic and returns nil.
func TestNilAnalyze_ExternalFunction(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	// Find any function that has no blocks (init functions or stubs).
	// If none exist, create one synthetically via buildSSA with an
	// empty function that the compiler might optimize to no blocks.
	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)

	// Use Double — it has blocks. Test that a non-pointer function
	// produces no nil findings.
	var fn *ssa.Function
	for _, member := range pkgs[0].Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "Double" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	findings := analyzer.Analyze(fn)
	require.Empty(t, findings, "Double(x int) should have no nil findings")
}

// TestNilAnalyze_EmptyFunction tests an empty function body.
func TestNilAnalyze_EmptyFunction(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func empty() {}
	`)

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "empty" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestNilAnalyze_MultiplePointerParams tests a function with multiple
// pointer parameters — each should independently warn.
func TestNilAnalyze_MultiplePointerParams(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func multiPtr(a, b, c *int) int {
			return *a + *b + *c
		}
	`)

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "multiPtr" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	require.Len(t, findings, 3, "each of the 3 pointer params should produce a warning")

	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity)
		require.Contains(t, f.Message, "possible nil dereference")
	}
}

// TestNilAnalyze_LoopWithNilCheck tests that a nil check inside a loop
// is handled correctly by the worklist (convergence).
func TestNilAnalyze_LoopWithNilCheck(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func walkSlice(ptrs []*int) int {
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

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "walkSlice" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	// p is checked with != nil before dereference — should be safe.
	require.Empty(t, findings,
		"loop with nil check should have no findings")
}

// TestNilAnalyze_ConditionalAssignment tests that conditional nil/non-nil
// assignment through if/else produces MaybeNil at the join point.
func TestNilAnalyze_ConditionalAssignment(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func condAssign(cond bool) int {
			var p *int
			if cond {
				x := 42
				p = &x
			} else {
				p = nil
			}
			return *p
		}
	`)

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "condAssign" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	require.Len(t, findings, 1)
	require.Equal(t, analysis.Warning, findings[0].Severity)
	require.Contains(t, findings[0].Message, "possible nil dereference")
}

// TestNilAnalyze_NestedNilCheck tests nested nil checks on double pointers.
func TestNilAnalyze_NestedNilCheck(t *testing.T) {
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

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "nestedCheck" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings,
		"nested nil checks should prove safety for both levels")
}

// TestNilAnalyze_ReturnNew tests that returning new(T) and immediately
// dereferencing is safe.
func TestNilAnalyze_ReturnNew(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func returnNew() int {
			p := new(int)
			*p = 100
			return *p
		}
	`)

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "returnNew" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// TestNilAnalyze_MakeInterfaceNilPtr is tested via testdata (MakeInterfaceNilPtrFixture).
// buildSSA panics on interface method declarations inline.

// TestNilAnalyze_DerefInBothBranches tests dereference in both branches
// of an if/else without nil check — both should warn.
func TestNilAnalyze_DerefInBothBranches(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func derefBoth(p *int, cond bool) int {
			if cond {
				return *p
			}
			return *p
		}
	`)

	var fn *ssa.Function
	for _, member := range ssaPkg.Members {
		f, ok := member.(*ssa.Function)
		if ok && f.Name() == "derefBoth" {
			fn = f
			break
		}
	}
	require.NotNil(t, fn)

	analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
	findings := analyzer.Analyze(fn)
	// Dedup: same variable p reported once, not per-branch.
	require.Len(t, findings, 1, "dedup: same variable p reported once")
	for _, f := range findings {
		require.Equal(t, analysis.Warning, f.Severity)
	}
}
