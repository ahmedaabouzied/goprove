package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// Edge case tests for NilAnalyzer.Analyze
// These tests target specific patterns, corner cases, and known limitations.
// ===========================================================================

// ---------------------------------------------------------------------------
// Early return guard: if p == nil { return }; *p
// Tests initBlockState propagation across blocks.
// ---------------------------------------------------------------------------

func TestNilAnalyze_EarlyReturnGuard(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
	}{
		"early return on nil is safe": {
			src: `
				package example

				func earlyReturn(p *int) int {
					if p == nil {
						return 0
					}
					return *p
				}
			`,
			fnName:  "earlyReturn",
			wantLen: 0,
		},
		"early return on neq nil with else is safe": {
			src: `
				package example

				func earlyReturnNeq(p *int) int {
					if p != nil {
						return *p
					}
					return 0
				}
			`,
			fnName:  "earlyReturnNeq",
			wantLen: 0,
		},
		"early return guard with multiple uses after guard": {
			src: `
				package example

				func multiUseAfterGuard(p *int) int {
					if p == nil {
						return 0
					}
					a := *p
					b := *p
					return a + b
				}
			`,
			fnName:  "multiUseAfterGuard",
			wantLen: 0,
		},
		"double nil check both params": {
			src: `
				package example

				func doubleGuard(a, b *int) int {
					if a == nil {
						return 0
					}
					if b == nil {
						return 0
					}
					return *a + *b
				}
			`,
			fnName:  "doubleGuard",
			wantLen: 0,
		},
		"guard one param but not the other": {
			src: `
				package example

				func partialGuard(a, b *int) int {
					if a == nil {
						return 0
					}
					return *a + *b
				}
			`,
			fnName:  "partialGuard",
			wantLen: 1, // b is unchecked
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen, "findings mismatch")
		})
	}
}

// ---------------------------------------------------------------------------
// Interprocedural: callee return nil state
// ---------------------------------------------------------------------------

func TestNilAnalyze_InterproceduralReturns(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
	}{
		"callee always returns new — safe": {
			src: `
				package example

				func makePtr() *int {
					return new(int)
				}

				func useNew() int {
					p := makePtr()
					return *p
				}
			`,
			fnName:  "useNew",
			wantLen: 0,
		},
		"callee always returns nil — bug": {
			src: `
				package example

				func returnNil() *int {
					return nil
				}

				func useNil() int {
					p := returnNil()
					return *p
				}
			`,
			fnName:  "useNil",
			wantLen: 1,
		},
		"callee may return nil — warning": {
			src: `
				package example

				func maybeNil(cond bool) *int {
					if cond {
						return new(int)
					}
					return nil
				}

				func useMaybe() int {
					p := maybeNil(true)
					return *p
				}
			`,
			fnName:  "useMaybe",
			wantLen: 1,
		},
		"callee return checked before use — safe": {
			src: `
				package example

				func maybeNil2(cond bool) *int {
					if cond {
						return new(int)
					}
					return nil
				}

				func checkReturn() int {
					p := maybeNil2(true)
					if p != nil {
						return *p
					}
					return 0
				}
			`,
			fnName:  "checkReturn",
			wantLen: 0,
		},
		"callee returns addr-of — safe": {
			src: `
				package example

				func addrOf() *int {
					x := 42
					return &x
				}

				func useAddrOf() int {
					p := addrOf()
					return *p
				}
			`,
			fnName:  "useAddrOf",
			wantLen: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen, "findings mismatch")
		})
	}
}

// ---------------------------------------------------------------------------
// Method receiver: assumed non-nil
// ---------------------------------------------------------------------------

func TestNilAnalyze_MethodReceiver(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	_ = prog

	// Collect all methods (functions with receivers) from the testdata package.
	var methods []*ssa.Function
	for _, member := range pkgs[0].Members {
		// Types have methods attached via NamedType.
		if tp, ok := member.(*ssa.Type); ok {
			// Get methods of the pointer type (covers both value and pointer receivers).
			mset := prog.MethodSets.MethodSet(tp.Type())
			for i := 0; i < mset.Len(); i++ {
				fn := prog.MethodValue(mset.At(i))
				if fn != nil && fn.Blocks != nil {
					methods = append(methods, fn)
				}
			}
		}
	}
	require.NotEmpty(t, methods, "should find some methods in testdata")

	// All methods should have no nil findings on the receiver.
	analyzer := analysis.NewNilAnalyzer(nil, nil)
	for _, fn := range methods {
		t.Run(fn.String(), func(t *testing.T) {
			findings := analyzer.Analyze(fn)
			for _, f := range findings {
				require.NotContains(t, f.Message, "nil dereference",
					"method %s should not warn on receiver deref", fn.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Deduplication: same variable only reported once
// ---------------------------------------------------------------------------

func TestNilAnalyze_Dedup(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
	}{
		"same param deref three times — one warning": {
			src: `
				package example

				func triple(p *int) int {
					a := *p
					b := *p
					c := *p
					return a + b + c
				}
			`,
			fnName:  "triple",
			wantLen: 1,
		},
		"two different params — two warnings": {
			src: `
				package example

				func twoParams(a, b *int) int {
					return *a + *b
				}
			`,
			fnName:  "twoParams",
			wantLen: 2,
		},
		"nil literal deref twice — one bug": {
			src: `
				package example

				func nilTwice() int {
					var p *int
					a := *p
					b := *p
					return a + b
				}
			`,
			fnName:  "nilTwice",
			wantLen: 1,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen, "dedup mismatch")
		})
	}
}

// ---------------------------------------------------------------------------
// Message format: named vs unnamed variables
// ---------------------------------------------------------------------------

func TestNilAnalyze_MessageFormat(t *testing.T) {
	t.Parallel()

	t.Run("parameter name in message", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example

			func withName(config *int) int {
				return *config
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "withName")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		require.Len(t, findings, 1)
		require.Contains(t, findings[0].Message, "config")
		require.Contains(t, findings[0].Message, "add a nil check")
	})

	t.Run("nil literal message", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example

			func nilLit() int {
				var p *int
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "nilLit")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		require.Len(t, findings, 1)
		require.Contains(t, findings[0].Message, "nil pointer")
		require.Contains(t, findings[0].Message, "always nil")
	})

	t.Run("call result name in message", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example

			func getPtr() *int {
				return nil
			}

			func callResult() int {
				p := getPtr()
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "callResult")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		require.Len(t, findings, 1)
		require.Contains(t, findings[0].Message, "result of getPtr()")
	})
}

// ---------------------------------------------------------------------------
// Recursive functions: should not stack overflow
// ---------------------------------------------------------------------------

func TestNilAnalyze_Recursive(t *testing.T) {
	t.Parallel()

	t.Run("direct recursion", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example

			func recurse(n int) *int {
				if n <= 0 {
					return new(int)
				}
				return recurse(n - 1)
			}

			func useRecurse() int {
				p := recurse(5)
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "useRecurse")

		// Should not panic or stack overflow.
		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		// Result depends on depth cap — may be 0 or 1 findings.
		_ = findings
	})

	t.Run("mutual recursion", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example

			func ping(n int) *int {
				if n <= 0 {
					return new(int)
				}
				return pong(n - 1)
			}

			func pong(n int) *int {
				return ping(n - 1)
			}

			func usePing() int {
				p := ping(3)
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "usePing")

		// Should not panic or stack overflow.
		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		_ = findings
	})
}

// ---------------------------------------------------------------------------
// Slice patterns
// ---------------------------------------------------------------------------

func TestNilAnalyze_SlicePatterns(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
	}{
		"nil slice index is not warned (deferred to bounds)": {
			src: `
				package example

				func sliceParam(s []int) int {
					return s[0]
				}
			`,
			fnName:  "sliceParam",
			wantLen: 0,
		},
		"make slice then index is safe": {
			src: `
				package example

				func makeAndIndex(n int) int {
					s := make([]int, n)
					return s[0]
				}
			`,
			fnName:  "makeAndIndex",
			wantLen: 0,
		},
		"slice from param checked then index is safe": {
			src: `
				package example

				func checkedSlice(s []int) int {
					if s != nil {
						return s[0]
					}
					return 0
				}
			`,
			fnName:  "checkedSlice",
			wantLen: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen)
		})
	}
}

// ---------------------------------------------------------------------------
// Map patterns
// ---------------------------------------------------------------------------

func TestNilAnalyze_MapPatterns(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
	}{
		"make map then read is safe": {
			src: `
				package example

				func makeMapRead() int {
					m := make(map[string]int)
					return m["key"]
				}
			`,
			fnName:  "makeMapRead",
			wantLen: 0,
		},
		"make map then write is safe": {
			src: `
				package example

				func makeMapWrite() {
					m := make(map[string]int)
					m["key"] = 42
				}
			`,
			fnName:  "makeMapWrite",
			wantLen: 0,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen)
		})
	}
}

// ---------------------------------------------------------------------------
// Channel patterns
// ---------------------------------------------------------------------------

func TestNilAnalyze_ChanPatterns(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func makeChanUse() {
			ch := make(chan int, 1)
			ch <- 42
		}
	`)
	fn := findSSAFunc(t, ssaPkg, "makeChanUse")

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(fn)
	require.Empty(t, findings)
}

// ---------------------------------------------------------------------------
// Non-pointer operations: should never produce nil findings
// ---------------------------------------------------------------------------

func TestNilAnalyze_NonPointerOps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
	}{
		"pure arithmetic": {
			src: `
				package example
				func arith(a, b int) int { return a + b * 2 }
			`,
			fnName: "arith",
		},
		"string operations": {
			src: `
				package example
				func str(s string) string { return s + "!" }
			`,
			fnName: "str",
		},
		"bool operations": {
			src: `
				package example
				func boolOp(a, b bool) bool { return a && b }
			`,
			fnName: "boolOp",
		},
		"multiple returns no pointers": {
			src: `
				package example
				func multiRet(x int) (int, bool) {
					if x > 0 {
						return x, true
					}
					return 0, false
				}
			`,
			fnName: "multiRet",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Empty(t, findings)
		})
	}
}

// ---------------------------------------------------------------------------
// Loop patterns: convergence and nil checks in loops
// ---------------------------------------------------------------------------

func TestNilAnalyze_LoopPatterns(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src     string
		fnName  string
		wantLen int
	}{
		"nil check in while loop body is safe": {
			src: `
				package example

				func loopCheck(ptrs []*int) int {
					sum := 0
					for i := 0; i < len(ptrs); i++ {
						p := ptrs[i]
						if p != nil {
							sum += *p
						}
					}
					return sum
				}
			`,
			fnName:  "loopCheck",
			wantLen: 0,
		},
		"new inside loop is safe": {
			src: `
				package example

				func loopNew(n int) int {
					sum := 0
					for i := 0; i < n; i++ {
						p := new(int)
						*p = i
						sum += *p
					}
					return sum
				}
			`,
			fnName:  "loopNew",
			wantLen: 0,
		},
		"unchecked deref in loop is a warning": {
			src: `
				package example

				func loopUnchecked(ptrs []*int) int {
					sum := 0
					for i := 0; i < len(ptrs); i++ {
						p := ptrs[i]
						sum += *p
					}
					return sum
				}
			`,
			fnName:  "loopUnchecked",
			wantLen: 1,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findSSAFunc(t, ssaPkg, tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen)
		})
	}
}

// ---------------------------------------------------------------------------
// Severity classification
// ---------------------------------------------------------------------------

func TestNilAnalyze_SeverityClassification(t *testing.T) {
	t.Parallel()

	t.Run("DefinitelyNil is Bug", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example
			func definiteNil() int {
				var p *int
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "definiteNil")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		require.Len(t, findings, 1)
		require.Equal(t, analysis.Bug, findings[0].Severity)
	})

	t.Run("MaybeNil is Warning", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example
			func maybeNil(p *int) int {
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "maybeNil")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		require.Len(t, findings, 1)
		require.Equal(t, analysis.Warning, findings[0].Severity)
	})

	t.Run("DefinitelyNotNil is Safe", func(t *testing.T) {
		t.Parallel()
		ssaPkg := buildSSA(t, `
			package example
			func notNil() int {
				p := new(int)
				return *p
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "notNil")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		require.Empty(t, findings)
	})
}

// ---------------------------------------------------------------------------
// Analyzer reuse: multiple Analyze calls on same analyzer
// ---------------------------------------------------------------------------

func TestNilAnalyze_Reuse(t *testing.T) {
	t.Parallel()

	ssaPkg := buildSSA(t, `
		package example

		func safe() int {
			p := new(int)
			return *p
		}

		func unsafe(p *int) int {
			return *p
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil)

	// First call — safe function.
	fn1 := findSSAFunc(t, ssaPkg, "safe")
	findings1 := analyzer.Analyze(fn1)
	require.Empty(t, findings1)

	// Second call — unsafe function.
	// Previous findings should not leak.
	fn2 := findSSAFunc(t, ssaPkg, "unsafe")
	findings2 := analyzer.Analyze(fn2)
	require.Len(t, findings2, 1)

	// Third call — safe again. Should reset.
	findings3 := analyzer.Analyze(fn1)
	require.Empty(t, findings3)
}

// ---------------------------------------------------------------------------
// Known limitations: document behavior, don't assert correctness
// These tests document what the analyzer DOES in known-limitation scenarios.
// They catch regressions if behavior changes unexpectedly.
// ---------------------------------------------------------------------------

func TestNilAnalyze_KnownLimitations(t *testing.T) {
	t.Parallel()

	t.Run("store-load not tracked — false warning expected", func(t *testing.T) {
		t.Parallel()
		// errors.As writes through a pointer — not tracked.
		// The analyzer sees p as MaybeNil after the store.
		ssaPkg := buildSSA(t, `
			package example

			func storeLoad() int {
				var x int
				p := &x   // p = &x, DefinitelyNotNil
				*p = 42   // Store through p
				return *p // Load through p — should be safe
			}
		`)
		fn := findSSAFunc(t, ssaPkg, "storeLoad")

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		// p is &x which is Alloc → DefinitelyNotNil.
		// *p dereferences p, which IS tracked. Should be 0 findings.
		require.Empty(t, findings, "alloc addr-of should be tracked as non-nil")
	})

	t.Run("interface invoke now checked", func(t *testing.T) {
		t.Parallel()
		// Interface method calls use ssa.Call with IsInvoke — now checked.
		_, pkgs, err := loader.Load("../../pkg/testdata")
		require.NoError(t, err)
		require.NotEmpty(t, pkgs)

		var fn *ssa.Function
		for _, member := range pkgs[0].Members {
			f, ok := member.(*ssa.Function)
			if ok && f.Name() == "MethodCallOnParam" {
				fn = f
				break
			}
		}
		require.NotNil(t, fn)

		analyzer := analysis.NewNilAnalyzer(nil, nil)
		findings := analyzer.Analyze(fn)
		// FIXED: interface invoke is now checked via IsInvoke().
		require.Len(t, findings, 1,
			"interface invoke on unchecked param should warn")
		require.Contains(t, findings[0].Message, "possible nil dereference")
	})
}

// ---------------------------------------------------------------------------
// Helper: find function in SSA package
// ---------------------------------------------------------------------------

func findSSAFunc(t *testing.T, pkg *ssa.Package, name string) *ssa.Function {
	t.Helper()
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == name {
			return fn
		}
	}
	t.Fatalf("function %s not found in package", name)
	return nil
}
