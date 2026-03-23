package analysis_test

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

// ===========================================================================
// Final coverage tests — targeting the last uncovered code paths.
// ===========================================================================

// ---------------------------------------------------------------------------
// Analyze: receiver init when paramNilStates is nil (line 71-74)
// A method analyzed with nil paramNilStates → state[blocks[0]] is nil
// when receiver init runs.
// ---------------------------------------------------------------------------

func TestCovFinal_ReceiverInitWithoutParamStates(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	// Find a method with a pointer receiver.
	var method *ssa.Function
	for _, member := range pkgs[0].Members {
		if tp, ok := member.(*ssa.Type); ok {
			mset := prog.MethodSets.MethodSet(tp.Type())
			for i := 0; i < mset.Len(); i++ {
				fn := prog.MethodValue(mset.At(i))
				if fn != nil && fn.Blocks != nil && fn.Signature.Recv() != nil {
					// Check receiver is a pointer (nillable).
					if analysis.IsNillableExported(fn.Params[0]) {
						method = fn
						break
					}
				}
			}
		}
		if method != nil {
			break
		}
	}
	if method == nil {
		t.Skip("no pointer-receiver method found in testdata")
	}

	// Analyze with nil paramNilStates → exercises receiver init
	// creating state[blocks[0]] from scratch.
	analyzer := analysis.NewNilAnalyzer(nil, nil)
	findings := analyzer.Analyze(method)
	_ = findings
}

// ---------------------------------------------------------------------------
// resolveCallees: with resolver (line 431)
// ---------------------------------------------------------------------------

func TestCovFinal_ResolveCalleesWithCHAResolver(t *testing.T) {
	t.Parallel()

	prog, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)

	resolver := analysis.NewCHAResolver(prog)
	analyzer := analysis.NewNilAnalyzer(resolver, nil)

	// Find a function that makes interface calls — exercises resolver path.
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if !ok || fn.Blocks == nil {
			continue
		}
		// Analyze all functions — some will have Call instructions
		// that go through the resolver.
		analyzer.Analyze(fn)
	}
}

// ---------------------------------------------------------------------------
// ComputeParamNilStates: nParams == 0 skip (line 127)
// ---------------------------------------------------------------------------

func TestCovFinal_ZeroParamFunction(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	// CallerOfZeroParam calls ZeroParamFunc which has 0 params.
	// ComputeParamNilStates should hit the nParams == 0 continue.
	states := analysis.ComputeParamNilStates(nil, pkgs)
	require.NotNil(t, states)

	// ZeroParamFunc should NOT be in the param states map.
	for fn, s := range states.States() {
		if fn.Name() == "ZeroParamFunc" {
			require.Empty(t, s, "zero-param function should have no param states")
		}
	}
}

// ---------------------------------------------------------------------------
// collectCallSites: go + interface dispatch (line 178)
// ---------------------------------------------------------------------------

func TestCovFinal_GoInterfaceDispatchNilCallee(t *testing.T) {
	t.Parallel()

	_, pkgs, err := loader.Load("../../pkg/testdata")
	require.NoError(t, err)

	// LaunchWorker does `go w.Work()` where w is an interface.
	// StaticCallee() returns nil for interface dispatch.
	// collectCallSites should hit the nil callee continue in the Go branch.
	states := analysis.ComputeParamNilStates(nil, pkgs)
	require.NotNil(t, states)
}

// ---------------------------------------------------------------------------
// ComputeParamNilStatesAnalysis: callerState == nil fallback (line 66-70)
// This happens when a caller has no convergedStates entry — e.g., it's
// an init function or was not analyzed.
// ---------------------------------------------------------------------------

func TestCovFinal_CallerNotInConvergedStates(t *testing.T) {
	t.Parallel()

	// Build a package where a function calls another, but the caller
	// might not be in convergedStates on the first iteration.
	ssaPkg := buildSSA(t, `
		package example

		func helper(p *int) int { return *p }

		func init() {
			x := 42
			helper(&x)
		}
	`)

	analyzer := analysis.NewNilAnalyzer(nil, nil)
	states := analysis.ComputeParamNilStatesAnalysis(
		[]*ssa.Package{ssaPkg}, analyzer,
	)
	require.NotNil(t, states)
}
