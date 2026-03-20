package analysis

import (
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/ssa"
)

type NilAnalyzer struct {
	state    map[*ssa.BasicBlock]map[ssa.Value]NilState
	resolver *CHAResolver
	findings []Finding
	err      error
}

func (a *NilAnalyzer) Analyze(fn *ssa.Function) []Finding {
	a.state = make(map[*ssa.BasicBlock]map[ssa.Value]NilState)
	a.findings = make([]Finding, 0)

	blocks, err := ReversePostOrder(fn)
	if err != nil {
		a.err = err
		return nil
	}
	workQueue := []*ssa.BasicBlock{}
	for _, block := range blocks {
		workQueue = append(workQueue, block)
	}

	// Iterate through the queue
	iterations := 0
	for len(workQueue) > 0 && iterations < maxIterations {
		iterations += 1
		block := workQueue[0]
		workQueue = workQueue[1:]

		// Initialize block state on first block visit
		if a.state[block] == nil {
			a.state[block] = make(map[ssa.Value]NilState)
		}

		// Save old state for change detection
		oldState := a.copyBlockState(block)

		a.refineFromPredecessor(block)
		for _, instr := range block.Instrs {
			a.transferInstruction(block, instr)
		}

		if !a.stateEqual(oldState, a.state[block]) {
			for _, succ := range block.Succs {
				workQueue = append(workQueue, succ)
			}
		}
	}
	for _, block := range blocks {
		for _, instr := range block.Instrs {
			a.checkInstruction(block, instr)
		}
	}
	return a.findings
}

func (a *NilAnalyzer) checkInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.UnOp:
		// Only pointer dereference: *p
		if v.Op == token.MUL {
			a.flagNilDeref(block, v.X, v.Pos())
		}
	case *ssa.FieldAddr:
		// p.Field — v.X is the struct pointer
		a.flagNilDeref(block, v.X, v.Pos())
	case *ssa.IndexAddr:
		// p[i] — v.X is the slice/array pointer
		a.flagNilDeref(block, v.X, v.Pos())
	}
}

func (a *NilAnalyzer) flagNilDeref(block *ssa.BasicBlock, v ssa.Value, pos token.Pos) {
	state := a.lookupNilState(block, v)
	switch state {
	case DefinitelyNil:
		a.findings = append(a.findings, Finding{
			Pos:      pos,
			Message:  "nil dereference",
			Severity: Bug,
		})
	case MaybeNil:
		a.findings = append(a.findings, Finding{
			Pos:      pos,
			Message:  "possible nil dereference",
			Severity: Warning,
		})
	}
}

func (a *NilAnalyzer) stateEqual(s1, s2 map[ssa.Value]NilState) bool {
	return maps.Equal(s1, s2)
}

func (a *NilAnalyzer) copyBlockState(block *ssa.BasicBlock) map[ssa.Value]NilState {
	cpy := make(map[ssa.Value]NilState)
	if currState, ok := a.state[block]; ok {
		maps.Copy(cpy, currState)
	}
	return cpy
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
