package analysis

import (
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

func TestBuildCallGraph_HasEdges(t *testing.T) {
	t.Parallel()
	prog, pkgs, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	cg := BuildCallGraph(prog)

	// DivByDecrement calls Decrement — should have an outgoing edge.
	var callerFn *ssa.Function
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == "DivByDecrement" {
			callerFn = fn
			break
		}
	}
	require.NotNil(t, callerFn, "DivByDecrement not found")

	node := cg.Nodes[callerFn]
	require.NotNil(t, node, "DivByDecrement should be in call graph")
	require.NotEmpty(t, node.Out, "DivByDecrement should have outgoing edges")
}

func TestBuildCallGraph_HasNodes(t *testing.T) {
	t.Parallel()
	prog, _, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	cg := BuildCallGraph(prog)
	require.NotNil(t, cg)
	require.NotEmpty(t, cg.Nodes, "call graph should have nodes")
}

func TestCallgraph(t *testing.T) {
	prog, _, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)
	cg := BuildCallGraph(prog)
	printCallgraph(cg, prog)
}

// TestCHAResolver_Resolve_NilCallerNode tests Resolve when the caller
// function is not in the call graph (node == nil at line 32).
func TestCHAResolver_Resolve_NilCallerNode(t *testing.T) {
	t.Parallel()
	prog, pkgs, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	resolver := NewCHAResolver(prog)

	// Find any function that makes an interface call.
	// DivByMixedImpl calls v.Result() which is a dynamic dispatch.
	var targetFn *ssa.Function
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == "DivByMixedImpl" {
			targetFn = fn
			break
		}
	}
	require.NotNil(t, targetFn)

	// Remove the function from the graph to trigger the nil path.
	delete(resolver.graph.Nodes, targetFn)

	// Find a call instruction in the function.
	for _, block := range targetFn.Blocks {
		for _, instr := range block.Instrs {
			call, ok := instr.(ssa.CallInstruction)
			if !ok {
				continue
			}
			// If it's a static call, skip — we want the dynamic dispatch.
			if call.Common().StaticCallee() != nil {
				continue
			}
			// This should hit the nil node path and return nil.
			result := resolver.Resolve(call)
			require.Nil(t, result, "expected nil when caller not in graph")
			return
		}
	}
	t.Fatal("no dynamic call instruction found in DivByMixedImpl")
}

// ---------------------------------------------------------------------------
// CHAResolver tests
// ---------------------------------------------------------------------------
func TestCHAResolver_String(t *testing.T) {
	t.Parallel()
	prog, _, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	resolver := NewCHAResolver(prog)
	// Should not panic.
	resolver.String()
}

// TestCHAResolver_String_MissingNode exercises the !ok branch in String().
func TestCHAResolver_String_MissingNode(t *testing.T) {
	t.Parallel()
	prog, pkgs, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	resolver := NewCHAResolver(prog)

	// Delete a node from the graph to trigger the !ok branch.
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if ok {
			delete(resolver.graph.Nodes, fn)
			break
		}
	}
	// Should not panic.
	resolver.String()
}

// TestCHAResolver_String_NilNode exercises the node == nil branch in String().
func TestCHAResolver_String_NilNode(t *testing.T) {
	t.Parallel()
	prog, pkgs, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	resolver := NewCHAResolver(prog)

	// Set a node to nil to trigger the nil-node branch.
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if ok {
			resolver.graph.Nodes[fn] = nil
			break
		}
	}
	// Should not panic.
	resolver.String()
}

// TestPrintCallgraph_MissingNode exercises the !ok branch in printCallgraph.
func TestPrintCallgraph_MissingNode(t *testing.T) {
	t.Parallel()
	prog, _, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	// Empty call graph — no nodes at all.
	cg := &callgraph.Graph{
		Nodes: make(map[*ssa.Function]*callgraph.Node),
	}
	// Should not panic, and should hit the !ok branch for every function.
	printCallgraph(cg, prog)
}

// TestPrintCallgraph_NilNode exercises the node == nil branch in printCallgraph.
func TestPrintCallgraph_NilNode(t *testing.T) {
	t.Parallel()
	prog, pkgs, err := loader.Load("../../pkg/testdata/")
	require.NoError(t, err)

	cg := BuildCallGraph(prog)

	// Insert a nil node for a function to trigger the nil branch.
	for _, member := range pkgs[0].Members {
		fn, ok := member.(*ssa.Function)
		if ok {
			cg.Nodes[fn] = nil
			break // one is enough
		}
	}
	// Should not panic.
	printCallgraph(cg, prog)
}
