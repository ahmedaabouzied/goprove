package analysis

import (
	"go/constant"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestLookupInterval_nilConst(t *testing.T) {
	t.Parallel()

	// In real Go SSA, nil literals and zero-value pointers are represented
	// as *ssa.Const with Value == nil. Calling c.Value.Kind() on these
	// without a nil guard causes a panic. lookupInterval must handle this
	// gracefully and return Top (unknown).
	block := &ssa.BasicBlock{}
	nilConst := &ssa.Const{} // Value is nil — simulates a nil pointer literal

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {},
		},
	}

	got := a.lookupInterval(block, nilConst)
	if !got.Equals(Top()) {
		t.Errorf("expected Top for nil-valued *ssa.Const, got %+v", got)
	}
}

func TestRefineFromCondition_nilConst(t *testing.T) {
	t.Parallel()

	// When refineFromCondition encounters a comparison like `x < nil`,
	// the *ssa.Const has Value == nil. The function must not panic and
	// should leave the state unchanged (early return).
	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {param: Top()},
		},
	}

	// cond.Y is a nil-valued Const (simulates comparison with nil)
	cond := &ssa.BinOp{
		Op: token.LSS,
	}
	cond.X = param
	cond.Y = &ssa.Const{} // Value == nil

	a.refineFromCondition(block, cond, true)

	// State should be unchanged — nil Const is not an int, early return.
	got := a.state[block][param]
	if !got.Equals(Top()) {
		t.Errorf("expected Top (unchanged), got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// refineFromEquality precision: Meet against current interval
// ---------------------------------------------------------------------------
// These tests verify that EQL/true and NEQ/false meet the constant against
// the current interval. Without this, unreachable branches aren't detected.
// e.g. if x=[7,7] and condition is x==0, the true branch should be Bottom
// (unreachable), not [0,0].

func TestRefineFromEquality_EQL_true_const_outside_range(t *testing.T) {
	t.Parallel()

	// x is known to be [7, 7]. Condition: x == 0, true branch.
	// Since 0 is outside [7, 7], the true branch is unreachable → Bottom.
	a := &Analyzer{}
	current := NewInterval(7, 7)
	refined := a.refineFromEquality(token.EQL, current, 0, true)

	expected := current.Meet(NewInterval(0, 0)) // Should be Bottom
	if !expected.IsBottom {
		t.Fatalf("precondition: Meet([7,7], [0,0]) should be Bottom, got %+v", expected)
	}
	if !refined.IsBottom {
		t.Errorf("EQL true with const outside range: expected Bottom, got %+v", refined)
	}
}

func TestRefineFromEquality_EQL_true_const_inside_range(t *testing.T) {
	t.Parallel()

	// x is known to be [-5, 10]. Condition: x == 3, true branch.
	// 3 is inside [-5, 10], so refined should be [3, 3].
	a := &Analyzer{}
	current := NewInterval(-5, 10)
	refined := a.refineFromEquality(token.EQL, current, 3, true)

	expected := NewInterval(3, 3)
	if !refined.Equals(expected) {
		t.Errorf("EQL true with const inside range: expected %+v, got %+v", expected, refined)
	}
}

func TestRefineFromEquality_NEQ_false_const_outside_range(t *testing.T) {
	t.Parallel()

	// x is known to be [10, 20]. Condition: x != 5, false branch (x == 5).
	// Since 5 is outside [10, 20], the false branch is unreachable → Bottom.
	a := &Analyzer{}
	current := NewInterval(10, 20)
	refined := a.refineFromEquality(token.NEQ, current, 5, false)

	expected := current.Meet(NewInterval(5, 5)) // Should be Bottom
	if !expected.IsBottom {
		t.Fatalf("precondition: Meet([10,20], [5,5]) should be Bottom, got %+v", expected)
	}
	if !refined.IsBottom {
		t.Errorf("NEQ false with const outside range: expected Bottom, got %+v", refined)
	}
}

func TestRefineFromEquality_NEQ_false_const_inside_range(t *testing.T) {
	t.Parallel()

	// x is known to be [0, 100]. Condition: x != 50, false branch (x == 50).
	// 50 is inside [0, 100], so refined should be [50, 50].
	a := &Analyzer{}
	current := NewInterval(0, 100)
	refined := a.refineFromEquality(token.NEQ, current, 50, false)

	expected := NewInterval(50, 50)
	if !refined.Equals(expected) {
		t.Errorf("NEQ false with const inside range: expected %+v, got %+v", expected, refined)
	}
}

func TestRefineFromEquality_EQL_true_wide_range_excludes_const(t *testing.T) {
	t.Parallel()

	// x is known to be [100, 200]. Condition: x == 0, true branch.
	// 0 is far outside [100, 200] → Bottom.
	a := &Analyzer{}
	current := NewInterval(100, 200)
	refined := a.refineFromEquality(token.EQL, current, 0, true)

	if !refined.IsBottom {
		t.Errorf("EQL true with const far outside range: expected Bottom, got %+v", refined)
	}
}

// ---------------------------------------------------------------------------
// isBlockReachable: defensive paths
// ---------------------------------------------------------------------------

func TestIsBlockReachable_UnvisitedBlock(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	a := &Analyzer{
		state: make(map[*ssa.BasicBlock]map[ssa.Value]Interval),
		// block NOT in state — unvisited
	}

	require.False(t, a.isBlockReachable(block),
		"unvisited block should not be reachable")
}

func TestIsBlockReachable_BlockWithBottomValue(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {param: Bottom()},
		},
	}

	require.False(t, a.isBlockReachable(block),
		"block with Bottom interval should not be reachable")
}

func TestIsBlockReachable_VisitedBlockWithValues(t *testing.T) {
	t.Parallel()

	block := &ssa.BasicBlock{}
	param := &ssa.Parameter{}

	a := &Analyzer{
		state: map[*ssa.BasicBlock]map[ssa.Value]Interval{
			block: {param: NewInterval(1, 10)},
		},
	}

	require.True(t, a.isBlockReachable(block),
		"block with non-Bottom interval should be reachable")
}

// flagOverflow non-basic type path: cannot be tested synthetically because
// ssa.BinOp has unexported type fields. Go's type system prevents arithmetic
// on non-basic types, so this path is unreachable in practice. The defensive
// check exists for safety but cannot be unit tested.

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
