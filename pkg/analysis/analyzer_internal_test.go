package analysis

import (
	"go/constant"
	"go/token"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestLookupInterval_unknownValueInVisitedBlock(t *testing.T) {
	t.Parallel()

	// When a block has been visited (exists in state) but a specific
	// value is not in its state map, lookupInterval returns Top.
	// This happens for values produced by unhandled instructions (e.g. *ssa.Call).
	block := &ssa.BasicBlock{}
	knownParam := &ssa.Parameter{}
	unknownParam := &ssa.Parameter{}

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {knownParam: NewInterval(1, 5)},
			// unknownParam is NOT in the state map
		},
	}

	got := a.lookupInterval(block, unknownParam)
	if !got.Equals(Top()) {
		t.Errorf("expected Top for unknown value in visited block, got %+v", got)
	}
}

func TestLookupInterval_unvisitedBlock(t *testing.T) {
	t.Parallel()

	// When a block has no entry in the state map (unvisited),
	// lookupInterval should return Bottom — not Top.
	// This is critical for Phi nodes reading from unvisited back-edge predecessors.
	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &Analyzer{
		state: make(map[*ssa.BasicBlock]map[ssa.Value]Interval),
		// block is NOT in state — simulates an unvisited predecessor
	}

	got := a.lookupInterval(block, param)
	if !got.Equals(Bottom()) {
		t.Errorf("expected Bottom for unvisited block, got %+v", got)
	}
}

func TestTransferUnOp_unsupportedOp(t *testing.T) {
	t.Parallel()

	// Construct a synthetic UnOp with token.XOR (bitwise complement ^x)
	// to exercise the default: Top() branch in transferUnOp.
	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {param: NewInterval(1, 5)},
		},
	}

	unOp := &ssa.UnOp{
		Op: token.XOR, // bitwise complement — not negation
	}
	unOp.X = param

	a.transferUnOp(block, unOp)

	got := a.state[block][unOp]
	if !got.Equals(Top()) {
		t.Errorf("expected Top for unsupported UnOp, got %+v", got)
	}
}

func TestRefineFromCondition_unsupportedOp(t *testing.T) {
	t.Parallel()

	// Construct a synthetic BinOp with token.AND — an operator that
	// can't appear as an *ssa.If condition in real Go SSA, but exercises
	// the default: return branch in refineFromCondition.
	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {param: Top()},
		},
	}

	cond := &ssa.BinOp{
		Op: token.AND, // bitwise AND — not a comparison operator
	}
	cond.X = param
	cond.Y = &ssa.Const{}
	cond.Y.(*ssa.Const).Value = constant.MakeInt64(1)

	a.refineFromCondition(block, cond, true)

	// State should be unchanged — the default branch returns without writing.
	got := a.state[block][param]
	if !got.Equals(Top()) {
		t.Errorf("expected Top, got %+v", got)
	}
}
