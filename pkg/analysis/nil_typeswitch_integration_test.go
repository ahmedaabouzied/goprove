package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// TypeAssert CommaOk refinement tests
//
// These tests verify that type switches and comma-ok type assertions
// refine the extracted value to DefinitelyNotNil in the true branch.
//
// SSA lowering for both patterns:
//
//   Block 0:
//     t0 = TypeAssert x.(*Foo) ,ok
//     t1 = Extract t0 #0          (*Foo)
//     t2 = Extract t0 #1          (ok bool)
//     if t2 goto Block1 else Block2
//   Block 1:                      (true branch — assertion succeeded)
//     &t1.Field                   (safe — t1 is DefinitelyNotNil)
//
// All tests use testdata/typeswitch.go fixtures.
// ===========================================================================

func TestTypeSwitch_Testdata(t *testing.T) {
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
		// --- Safe: type switch proves non-nil ---

		// switch v := a.(type) { case *Dog: v.Name }
		{"TypeSwitchDeref", 0, 0, ""},

		// if v, ok := a.(*Dog); ok { v.Name }
		{"CommaOkDeref", 0, 0, ""},

		// switch v := a.(type) { case *Dog: v.Name; case *Cat: v.Lives }
		{"TypeSwitchMultiCase", 0, 0, ""},

		// v, ok := a.(*Dog); if !ok { return }; v.Name
		{"CommaOkEarlyReturn", 0, 0, ""},

		// --- Unsafe: should warn ---

		// v, _ := a.(*Dog); v.Name — ok not checked
		{"CommaOkNoCheck", 1, analysis.Warning, "possible nil dereference"},

		// default: a.Sound() — interface param, no type narrowing
		{"TypeSwitchDefault", 1, analysis.Warning, "possible nil dereference"},
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
// Inline tests for patterns that don't need testdata fixtures
// ===========================================================================

func TestTypeSwitch_Inline(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src      string
		fnName   string
		wantLen  int
		severity analysis.Severity
		message  string
	}{
		// Comma-ok on interface param, ok checked — safe
		"comma ok checked deref safe": {
			src: `
				package example

				func safe(x interface{}) int {
					if p, ok := x.(*int); ok {
						return *p
					}
					return 0
				}
			`,
			fnName:  "safe",
			wantLen: 0,
		},

		// Comma-ok on interface param, ok NOT checked — warn
		"comma ok unchecked deref warns": {
			src: `
				package example

				func unsafe(x interface{}) int {
					p, _ := x.(*int)
					return *p
				}
			`,
			fnName:   "unsafe",
			wantLen:  1,
			severity: analysis.Warning,
			message:  "possible nil dereference",
		},

		// Non-CommaOk type assertion — panics on failure, so safe
		"bare type assertion safe": {
			src: `
				package example

				func bareAssert(x interface{}) int {
					p := x.(*int)
					return *p
				}
			`,
			fnName:  "bareAssert",
			wantLen: 0,
		},

		// Comma-ok early return pattern — safe
		"comma ok early return safe": {
			src: `
				package example

				func earlyReturn(x interface{}) int {
					p, ok := x.(*int)
					if !ok {
						return 0
					}
					return *p
				}
			`,
			fnName:  "earlyReturn",
			wantLen: 0,
		},

		// Regression: nil check refinement still works
		"nil check still works": {
			src: `
				package example

				func nilCheck(p *int) int {
					if p == nil {
						return 0
					}
					return *p
				}
			`,
			fnName:  "nilCheck",
			wantLen: 0,
		},

		// Regression: neq nil check still works
		"neq nil check still works": {
			src: `
				package example

				func neqCheck(p *int) int {
					if p != nil {
						return *p
					}
					return 0
				}
			`,
			fnName:  "neqCheck",
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

			if tt.wantLen > 0 {
				require.Equal(t, tt.severity, findings[0].Severity)
				require.Contains(t, findings[0].Message, tt.message)
			}
		})
	}
}
