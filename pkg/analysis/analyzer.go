package analysis

import (
	"fmt"
	"go/constant"
	"go/token"
	"maps"
	"math"

	"golang.org/x/tools/go/ssa"
)

type Analyzer struct {
	state    map[*ssa.BasicBlock]map[ssa.Value]Interval
	findings []Finding
	err      error
}

type Finding struct {
	Pos      token.Pos
	Message  string
	Severity Severity
}

type Severity uint8

const (
	Safe Severity = iota
	Warning
	Bug
)

const maxIterations = 1000

func (a *Analyzer) Analyze(fn *ssa.Function) []Finding {
	a.state = make(map[*ssa.BasicBlock]map[ssa.Value]Interval)

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
		// Copy blocks current state before initializing it.
		oldState := a.copyBlockState(block)
		a.initBlockState(block, fn)
		a.refineFromPredecessor(block)
		for _, instr := range block.Instrs {
			a.transferInstruction(block, instr)
		}

		// Compare old state with the current state.
		// If they're not the same, loop has not ended.
		// we need to loop again.
		if !stateEqual(oldState, a.state[block]) {
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

func (a *Analyzer) copyBlockState(block *ssa.BasicBlock) map[ssa.Value]Interval {
	cpy := make(map[ssa.Value]Interval)
	if currState, ok := a.state[block]; ok {
		maps.Copy(cpy, currState)
	}
	return cpy
}

func stateEqual(s1, s2 map[ssa.Value]Interval) bool {
	return maps.Equal(s1, s2)
}

func (a *Analyzer) initBlockState(block *ssa.BasicBlock, fn *ssa.Function) {
	// Initialize the initial state
	a.state[block] = make(map[ssa.Value]Interval)
	if len(block.Preds) == 0 {
		// Entry block. Initialize params with top
		for _, param := range fn.Params {
			a.state[block][param] = Top()
		}
	}

	for _, pred := range block.Preds {
		predState := a.state[pred]
		for v, interval := range predState {
			if existing, ok := a.state[block][v]; ok {
				a.state[block][v] = existing.Join(interval)
			} else {
				a.state[block][v] = interval
			}
		}
	}
}

// refineFromPredecessor narrows the block interval by walking through the predecessors
func (a *Analyzer) refineFromPredecessor(block *ssa.BasicBlock) {
	for _, pred := range block.Preds {
		// Check if predecessor ended with an If
		lastInstr := pred.Instrs[len(pred.Instrs)-1]
		ifInstr, ok := lastInstr.(*ssa.If)
		if !ok {
			continue
		}

		// Figure out: are we in the true or false successor
		isTrueBranch := pred.Succs[0] == block

		binOp, ok := ifInstr.Cond.(*ssa.BinOp)
		if !ok {
			continue
		}

		a.refineFromCondition(block, binOp, isTrueBranch)
	}
}

func (a *Analyzer) refineFromCondition(block *ssa.BasicBlock, cond *ssa.BinOp, isTrueBranch bool) {
	var variable ssa.Value
	var constVal int64
	if c, ok := cond.Y.(*ssa.Const); ok && c.Value.Kind() == constant.Int {
		variable = cond.X
		constVal = c.Int64()
	} else if c, ok := cond.X.(*ssa.Const); ok && c.Value.Kind() == constant.Int {
		variable = cond.Y
		constVal = c.Int64()
	} else {
		return
	}

	current := a.lookupInterval(block, variable)
	var refined Interval

	switch cond.Op {
	case token.EQL, token.NEQ:
		refined = a.refineFromEquality(cond.Op, current, constVal, isTrueBranch)
	case token.GTR, token.GEQ, token.LSS, token.LEQ:
		refined = a.refineFromComparison(cond.Op, current, constVal, isTrueBranch)
	default:
		return
	}

	a.state[block][variable] = refined
}

func (a *Analyzer) refineFromEquality(op token.Token, current Interval, constVal int64, isTrueBranch bool) Interval {
	var refined Interval
	switch op {
	case token.NEQ: // y != 0
		if isTrueBranch {
			refined = current.ExcludeZero()
		} else {
			refined = NewInterval(constVal, constVal)
		}
	case token.EQL: // y == 0
		if isTrueBranch {
			refined = NewInterval(constVal, constVal)
		} else {
			refined = current.ExcludeZero()
		}
	}
	return refined
}

func (a *Analyzer) transferInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.BinOp:
		a.transferBinOp(block, v)
	case *ssa.Phi:
		a.transferPhi(block, v)
	}
}

func (a *Analyzer) checkInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.BinOp:
		if v.Op == token.QUO || v.Op == token.REM {
			y := a.lookupInterval(block, v.Y)
			a.flagDivisionByZero(v, y)
		}
	}
}

func (a *Analyzer) refineFromComparison(op token.Token, current Interval, constVal int64, isTrueBranch bool) Interval {
	var refined Interval
	switch op {
	case token.LSS: // x < C
		if isTrueBranch {
			refined = current.Meet(NewInterval(math.MinInt64, constVal-1))
		} else {
			refined = current.Meet(NewInterval(constVal, math.MaxInt64))
		}
	case token.LEQ: // x <= c
		if isTrueBranch {
			refined = current.Meet(NewInterval(math.MinInt64, constVal))
		} else {
			refined = current.Meet(NewInterval(constVal+1, math.MaxInt64))
		}
	case token.GTR: // x > c
		if isTrueBranch {
			refined = current.Meet(NewInterval(constVal+1, math.MaxInt64))
		} else {
			refined = current.Meet(NewInterval(math.MinInt64, constVal))
		}
	case token.GEQ: // x >= c
		if isTrueBranch {
			refined = current.Meet(NewInterval(constVal, math.MaxInt64))
		} else {
			refined = current.Meet(NewInterval(math.MinInt64, constVal-1))
		}
	}
	return refined
}

func (a *Analyzer) transferBinOp(block *ssa.BasicBlock, v *ssa.BinOp) {
	x := a.lookupInterval(block, v.X)
	y := a.lookupInterval(block, v.Y)

	var result Interval
	switch v.Op {
	case token.ADD:
		result = x.Add(y)
	case token.SUB:
		result = x.Sub(y)
	case token.MUL:
		result = x.Mul(y)
	case token.QUO, token.REM:
		result = x.Div(y)
	default:
		result = Top()
	}
	a.state[block][v] = result
}

func (a *Analyzer) transferPhi(block *ssa.BasicBlock, v *ssa.Phi) {
	result := Bottom()

	for _, edge := range v.Edges {
		result = result.Join(a.lookupInterval(block, edge))
	}

	a.state[block][v] = result
}

func (a *Analyzer) flagDivisionByZero(v *ssa.BinOp, divisor Interval) {
	if !divisor.ContainsZero() {
		return
	}

	if divisor.Equals(NewInterval(0, 0)) {
		a.findings = append(a.findings, Finding{
			Pos:      v.Pos(),
			Message:  "division by zero",
			Severity: Bug,
		})
		return
	}

	a.findings = append(a.findings, Finding{
		Pos:      v.Pos(),
		Message:  "possible division by zero",
		Severity: Warning,
	})
}

func (a *Analyzer) lookupInterval(block *ssa.BasicBlock, v ssa.Value) Interval {
	if c, ok := v.(*ssa.Const); ok {
		// Extract int64 from the cosnt value
		if c.Value.Kind() != constant.Int {
			a.err = fmt.Errorf("parsing non int const into an interval")
			return Top()
		}
		val := c.Int64() // This will not panic because of the check above
		return NewInterval(val, val)
	}
	if iv, ok := a.state[block][v]; ok {
		return iv
	}
	return Top() // Value is unknown
}
