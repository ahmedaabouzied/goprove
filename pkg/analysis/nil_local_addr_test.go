package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ---------------------------------------------------------------------------
// Inline tests for address-taken local patterns
// ---------------------------------------------------------------------------
func TestLocalAddr_Inline(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      string
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		// Named return assigned non-nil then used — safe
		"named return assigned non-nil": {
			src: `
				package example

				func create() (p *int) {
					x := 42
					p = &x
					_ = *p
					return
				}
			`,
			fnName:  "create",
			wantLen: 0,
		},

		// Named return not assigned — proven nil, should be Bug
		"named return zero value": {
			src: `
				package example

				func create() (p *int) {
					_ = *p
					return
				}
			`,
			fnName:   "create",
			wantLen:  1,
			severity: analysis.Bug,
			message:  "nil dereference",
		},

		// Named return assigned nil then used — should warn or bug
		"named return assigned nil": {
			src: `
				package example

				func create() (p *int) {
					p = nil
					_ = *p
					return
				}
			`,
			fnName:  "create",
			wantLen: 1,
		},

		// Address-taken local via & — assigned non-nil then loaded
		"address taken local non-nil": {
			src: `
				package example

				func process() int {
					var p *int
					x := 42
					p = &x
					return *p
				}
			`,
			fnName:  "process",
			wantLen: 0,
		},

		// Multiple stores to named return — last store is non-nil
		"named return multiple stores last non-nil": {
			src: `
				package example

				func create(cond bool) (p *int) {
					p = nil
					if cond {
						x := 1
						p = &x
					}
					return
				}
			`,
			fnName:  "create",
			wantLen: 0,
		},

		// Named return used after conditional assignment — may be nil
		"named return conditional use": {
			src: `
				package example

				func create(cond bool) int {
					var p *int
					if cond {
						x := 1
						p = &x
					}
					return *p
				}
			`,
			fnName:   "create",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible nil dereference",
		},

		// Regression: regular param deref still works
		"param deref still warns": {
			src: `
				package example

				func deref(p *int) int {
					return *p
				}
			`,
			fnName:   "deref",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible nil dereference",
		},

		// Regression: nil check on param still safe
		"param nil check still safe": {
			src: `
				package example

				func safe(p *int) int {
					if p == nil {
						return 0
					}
					return *p
				}
			`,
			fnName:  "safe",
			wantLen: 0,
		},

		// Regression: new(T) deref still safe
		"new deref still safe": {
			src: `
				package example

				func newDeref() int {
					p := new(int)
					return *p
				}
			`,
			fnName:  "newDeref",
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
				if ok && f.Name() == tt.fnName {
					fn = f
					break
				}
			}
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
			findings := analyzer.Analyze(fn)

			require.Len(t, findings, tt.wantLen,
				"%s: expected %d findings, got %d", name, tt.wantLen, len(findings))

			if tt.wantLen > 0 && tt.severity != 0 {
				require.Equal(t, tt.severity, findings[0].Severity)
				require.Contains(t, findings[0].Message, tt.message)
			}
		})
	}
}

// ===========================================================================
// Address-taken local variable (addrLocal) tests
//
// When a local variable has its address taken (named return, &x),
// SSA uses Store/UnOp(MUL) pairs instead of direct value flow.
// The resolveAddress function now handles *ssa.Alloc, so stores
// to address-taken locals are tracked in addrState and loads
// see the stored nil state via transferUnOpInstr.
//
// SSA pattern for named return:
//
//   func New() (e *T) {
//       e = &T{}            // Store &T{} to e's alloc
//       e.Field = x         // Load e (UnOp MUL), then FieldAddr
//       return
//   }
//
//   entry:
//     t0 = local *T (e)    ← Alloc
//     t1 = &T{}            ← Alloc
//     *t0 = t1             ← Store: non-nil to t0
//     t2 = *t0             ← UnOp MUL: load e — now DefinitelyNotNil
//     &t2.Field            ← FieldAddr: safe
// ===========================================================================
// ---------------------------------------------------------------------------
// Testdata fixtures (use real packages for struct field patterns)
// ---------------------------------------------------------------------------
func TestLocalAddr_Testdata(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	pkg := pkgs[0]

	tests := []struct {
		fnName  string
		wantLen int
	}{
		// CheckFieldReload: if o.In != nil { o.In.Val } — field reload safe
		{"CheckFieldReload", 1}, // 1 for param o, 0 for field reload
	}

	for _, tt := range tests {
		t.Run(tt.fnName, func(t *testing.T) {
			t.Parallel()
			fn := findSSAFunc(t, pkg, tt.fnName)
			require.NotNil(t, fn)

			analyzer := analysis.NewNilAnalyzer(nil, nil, nil)
			findings := analyzer.Analyze(fn)
			require.Len(t, findings, tt.wantLen,
				"%s: wrong finding count", tt.fnName)
		})
	}
}
