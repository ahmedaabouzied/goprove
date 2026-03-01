package analysis

import (
	"go/constant"
	"go/token"
	"testing"

	"golang.org/x/tools/go/ssa"
)

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
