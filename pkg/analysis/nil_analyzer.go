package analysis

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

type NilAnalyzer struct {
	state    map[*ssa.BasicBlock]map[ssa.Value]NilState
	resolver *CHAResolver
	findings []Finding
}

func (a *NilAnalyzer) lookupNilState(block *ssa.BasicBlock, v ssa.Value) NilState {
	if c, ok := v.(*ssa.Const); ok {
		if c.IsNil() {
			return DefinitelyNil
		}
		return DefinitelyNotNil
	}

	if !isNillable(v) {
		// Non-nillable values. Like int, bools, etc..
		return DefinitelyNotNil
	}

	// Check if block has state for v, if so, return it.
	if m, ok := a.state[block]; ok {
		if s, ok := m[v]; ok {
			return s
		}
	}
	return MaybeNil
}

// Checks for if something == nil or something != nil
func (a *NilAnalyzer) refineFromPredecessor(block *ssa.BasicBlock) {
	// Check if predecessor ends with an if
	for _, pred := range block.Preds {
		lastInstr := pred.Instrs[len(pred.Instrs)-1]
		ifInstr, ok := lastInstr.(*ssa.If)
		if !ok {
			continue
		}
		// IF statement

		// Figure out if we're in a true or false branch
		isTrueBranch := pred.Succs[0] == block

		binOp, ok := ifInstr.Cond.(*ssa.BinOp)
		if !ok {
			continue
		}
		a.refineFromCondition(block, binOp, isTrueBranch)
	}
}

func (a *NilAnalyzer) refineFromCondition(block *ssa.BasicBlock, cond *ssa.BinOp, isTrueBranch bool) {
	var variable ssa.Value
	var res NilState
	if c, ok := cond.X.(*ssa.Const); ok && c.IsNil() {
		variable = cond.Y
	} else if c, ok := cond.Y.(*ssa.Const); ok && c.IsNil() {
		variable = cond.X
	} else {
		return
	}

	switch cond.Op {
	case token.EQL:
		if isTrueBranch {
			res = DefinitelyNil
		} else {
			res = DefinitelyNotNil
		}
	case token.NEQ:
		if isTrueBranch {
			res = DefinitelyNotNil
		} else {
			res = DefinitelyNil
		}
	default:
		return
	}
	a.state[block][variable] = res
}

func (a *NilAnalyzer) transferInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.Alloc:
		a.state[block][v] = DefinitelyNotNil
	case *ssa.MakeInterface:
		a.state[block][v] = DefinitelyNotNil
	case *ssa.MakeSlice:
		a.state[block][v] = DefinitelyNotNil
	case *ssa.MakeMap:
		a.state[block][v] = DefinitelyNotNil
	case *ssa.MakeChan:
		a.state[block][v] = DefinitelyNotNil
	case *ssa.Phi:
		a.transferPhi(block, v)
	}
}

func (a *NilAnalyzer) transferPhi(block *ssa.BasicBlock, instr *ssa.Phi) {
	res := NilBottom
	for i, edge := range instr.Edges {
		pred := block.Preds[i]
		res = res.Join(a.lookupNilState(pred, edge))
	}
	a.state[block][instr] = res
}

func isNillable(v ssa.Value) bool {
	switch v.Type().Underlying().(type) {
	case *types.Pointer,
		*types.Interface,
		*types.Slice,
		*types.Map,
		*types.Chan,
		*types.Signature:
		return true
	default:
		return false
	}
}
