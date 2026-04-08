package analysis

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"math"

	"golang.org/x/tools/go/ssa"
)

const (
	Safe Severity = iota
	Warning
	Bug
)

func NewAnalyzer(resolver *CHAResolver) *Analyzer {
	return &Analyzer{
		resolver: resolver,
	}
}

type Analyzer struct {
	state          map[*ssa.BasicBlock]map[ssa.Value]Interval
	summaries      map[*ssa.Function][]FunctionSummary
	sliceLens      map[*ssa.BasicBlock]map[ssa.Value]Interval
	paramOverrides []Interval // If set, use these instead of type bounds.

	callDepth    int
	maxCallDepth int
	resolver     *CHAResolver // nil = fallback to StaticCallee only resolver
	targetPkgs   map[*ssa.Package]bool

	findings []Finding
	err      error
}

func (a *Analyzer) Analyze(fn *ssa.Function) []Finding {
	a.state = make(map[*ssa.BasicBlock]map[ssa.Value]Interval)
	a.sliceLens = make(map[*ssa.BasicBlock]map[ssa.Value]Interval)
	if a.summaries == nil {
		a.summaries = make(map[*ssa.Function][]FunctionSummary)
	}

	if a.maxCallDepth == 0 {
		a.maxCallDepth = 3
	}

	blocks, err := ReversePostOrder(fn)
	if err != nil {
		a.err = err
		return nil
	}

	rpoIndex := make(map[*ssa.BasicBlock]int)

	workQueue := []*ssa.BasicBlock{}
	for i, block := range blocks {
		workQueue = append(workQueue, block)
		rpoIndex[block] = i
	}

	// Iterate through the queue
	iterations := 0
	for len(workQueue) > 0 && iterations < maxIterations {
		iterations += 1
		block := workQueue[0]
		workQueue = workQueue[1:]
		// Copy blocks current state before initializing it.
		oldState := a.copyBlockState(block)
		oldSliceLen := a.copyBlockSliceLen(block)
		a.initBlockState(rpoIndex, block, oldState, oldSliceLen, fn)
		a.refineFromPredecessor(block)
		for _, instr := range block.Instrs {
			a.transferInstruction(block, instr)
		}

		// Compare old state with the current state.
		// If they're not the same, loop has not ended.
		// we need to loop again.
		if !stateEqual(oldState, a.state[block]) || !stateEqual(oldSliceLen, a.sliceLens[block]) {
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
	// Deduplicate findings by position + message.
	// CHA interface dispatch can produce the same finding once per callee.
	seen := make(map[string]bool)
	deduped := make([]Finding, 0, len(a.findings))
	for _, f := range a.findings {
		key := fmt.Sprintf("%d:%s", f.Pos, f.Message)
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, f)
		}
	}
	return deduped
}

// SetTargetPackages limits interprocedural analysis to the given packages.
// Calls to functions outside these packages return conservative results
// (Top) instead of recursively analyzing the callee body.
func (a *Analyzer) SetTargetPackages(pkgs []*ssa.Package) {
	a.targetPkgs = make(map[*ssa.Package]bool, len(pkgs))
	for _, pkg := range pkgs {
		if pkg != nil {
			a.targetPkgs[pkg] = true
		}
	}
}

func (a *Analyzer) checkConvertOp(block *ssa.BasicBlock, v *ssa.Convert) {
	sourceInterval := a.lookupInterval(block, v.X)
	targetKind, ok := v.Type().Underlying().(*types.Basic)
	if ok {
		targetInterval, covered := IntervalForType(targetKind.Kind())
		if covered {
			a.checkOverflow(targetInterval, sourceInterval, v, "conversion")
		}
	}
}

func (a *Analyzer) checkInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.BinOp:
		if v.Op == token.QUO || v.Op == token.REM {
			// We can check only X (one side of the operation)
			// since Go doesn't allow math operations on
			// different types.
			if basic, ok := v.X.Type().
				Underlying().(*types.Basic); ok &&
				basic.Info()&types.IsInteger != 0 {
				y := a.lookupInterval(block, v.Y)
				a.flagDivisionByZero(v, y)
			}
		}
		a.flagOverflow(block, v)
	case *ssa.Convert:
		a.checkConvertOp(block, v)
	case *ssa.UnOp:
		a.checkUnOp(block, v)
	case *ssa.IndexAddr:
		a.checkIndexAddrOp(block, v)
	}
}

func (a *Analyzer) checkIndexAddrOp(block *ssa.BasicBlock, v *ssa.IndexAddr) {
	indexInterval := a.lookupInterval(block, v.Index)
	sliceLen := a.lookupSliceLen(block, v.X)
	a.checkBoundsViolation(v, indexInterval, sliceLen)
}

func (a *Analyzer) checkBoundsViolation(v *ssa.IndexAddr, indexI Interval, sliceL Interval) {
	if sliceL.IsBottom || indexI.IsBottom {
		return
	}

	if sliceL.IsTop || indexI.IsTop {
		// Unknown length of slice or interval or index. Can't be proven save or unsafe
		// Could warn here, but we choose less noise.
		return
	}

	validRange := NewInterval(0, sliceL.Lo-1)
	if validRange.Contains(indexI) {
		// safe
		return
	}

	if indexI.Lo >= sliceL.Hi || indexI.Hi < 0 {
		// Proven OOB (index always >= length) --> Bug
		a.findings = append(a.findings, Finding{
			v.Pos(),
			"slice out of bound access",
			Bug,
		})
		return
	}
	// We can warn here, but we stay silent to reduce noise.
}

func (a *Analyzer) checkOverflow(bound, current Interval, v ssa.Instruction, context string) {
	if bound.Contains(current) {
		return
	}

	proven := bound.Meet(current).IsBottom
	msg := "possible integer overflow"
	severity := Warning
	if proven {
		msg = "proven integer overflow"
		severity = Bug
	}
	if context != "" {
		msg += " in " + context
	}

	a.findings = append(a.findings, Finding{
		v.Pos(),
		msg,
		severity,
	})
}

func (a *Analyzer) checkUnOp(block *ssa.BasicBlock, v *ssa.UnOp) {
	if current, ok := a.state[block][v]; ok {
		if targetKind, ok := v.Type().Underlying().(*types.Basic); ok {
			if bound, covered := IntervalForType(targetKind.Kind()); covered {
				a.checkOverflow(bound, current, v, "negation")
			}
		}
	}
}

func (a *Analyzer) computeReturnIntervals(fn *ssa.Function) []Interval {
	var returns []Interval
	for _, block := range fn.Blocks {
		if !a.isBlockReachable(block) {
			continue
		}
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			// First return instruction — initialize the slice
			if returns == nil {
				returns = make([]Interval, len(ret.Results))
				for i := range returns {
					returns[i] = Bottom()
				}
			}
			// Join each return value's interval
			for i, result := range ret.Results {
				returns[i] = returns[i].Join(a.lookupInterval(block,
					result))
			}
		}
	}
	return returns
}

func (a *Analyzer) copyBlockState(block *ssa.BasicBlock) map[ssa.Value]Interval {
	cpy := make(map[ssa.Value]Interval)
	if currState, ok := a.state[block]; ok {
		maps.Copy(cpy, currState)
	}
	return cpy
}

func (a *Analyzer) copyBlockSliceLen(block *ssa.BasicBlock) map[ssa.Value]Interval {
	cpy := make(map[ssa.Value]Interval)
	if currSliceLens, ok := a.sliceLens[block]; ok {
		maps.Copy(cpy, currSliceLens)
	}
	return cpy
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

func (a *Analyzer) flagOverflow(block *ssa.BasicBlock, v *ssa.BinOp) {
	// Check for type bounds
	basic, ok := v.Type().Underlying().(*types.Basic)
	if !ok {
		// DEFENSIVE: unreachable in valid Go. All BinOp result types are *types.Basic:
		// - Arithmetic (+, -, *, /, %, &, |, ^, <<, >>) → integer types
		// - Comparison (==, !=, <, >, <=, >=) → bool
		// - String concatenation (+) → string
		// Go's type system prevents BinOp on non-basic types (structs, slices, etc.).
		// This guard exists for safety against future SSA changes.
		// Cannot be unit tested because ssa.BinOp has unexported type fields.
		return
	}
	bound, covered := IntervalForType(basic.Kind())
	if !covered {
		return // Not a type we cover the boundary check of. Skip.
	}
	currentInterval := a.state[block][v]
	a.checkOverflow(bound, currentInterval, v, "")
}

func (a *Analyzer) initBlockState(rpoIndex map[*ssa.BasicBlock]int, block *ssa.BasicBlock, oldState map[ssa.Value]Interval, oldSliceLen map[ssa.Value]Interval, fn *ssa.Function) {
	// Initialize the initial state
	a.state[block] = make(map[ssa.Value]Interval)
	a.sliceLens[block] = make(map[ssa.Value]Interval)

	if len(block.Preds) == 0 {
		// Entry block. Initialize params with top
		for i, param := range fn.Params {
			if a.paramOverrides != nil && i < len(a.paramOverrides) {
				a.state[block][param] = a.paramOverrides[i]
			} else {
				// get the param type
				kind, ok := param.Type().Underlying().(*types.Basic)
				if ok {
					a.state[block][param], ok = IntervalForType(kind.Kind())
					if !ok {
						a.state[block][param] = Top()
					}
				} else if _, ok := param.Type().Underlying().(*types.Slice); ok {
					// A slice. Init the value of len in the sliceLens state
					a.sliceLens[block][param] = NewInterval(0, math.MaxInt64)
				} else {
					// Fallback to Top in case we couldn't get the basic type.
					a.state[block][param] = Top()
				}
			}
		}
	}

	for _, pred := range block.Preds {
		predState := a.state[pred]
		predSliceLens := a.sliceLens[pred]

		for v, interval := range predState {
			if existing, ok := a.state[block][v]; ok {
				a.state[block][v] = existing.Join(interval)
			} else {
				a.state[block][v] = interval
			}
		}

		for v, interval := range predSliceLens {
			if existing, ok := a.sliceLens[block][v]; ok {
				a.sliceLens[block][v] = existing.Join(interval)
			} else {
				a.sliceLens[block][v] = interval
			}
		}
	}
	// Widen the loop header interval to the max interval
	// the loop will have.
	if isLoopHeader(block, rpoIndex) {
		for v, newVal := range a.state[block] {
			if oldVal, ok := oldState[v]; ok {
				a.state[block][v] = oldVal.Widen(newVal)
			}
		}

		for v, newVal := range a.sliceLens[block] {
			if oldVal, ok := oldSliceLen[v]; ok {
				a.sliceLens[block][v] = oldVal.Widen(newVal)
			}
		}
	}
}

func (a *Analyzer) isBlockReachable(block *ssa.BasicBlock) bool {
	blockState, ok := a.state[block]
	if !ok {
		return false
	}

	for _, iv := range blockState {
		if iv.IsBottom {
			return false
		}
	}
	return true
}

func (a *Analyzer) lookupInterval(block *ssa.BasicBlock, v ssa.Value) Interval {
	if c, ok := v.(*ssa.Const); ok {
		// Extract int64 from the cosnt value
		if c.Value == nil || c.Value.Kind() != constant.Int {
			a.err = fmt.Errorf("parsing non int const into an interval")
			return Top()
		}
		val := c.Int64() // This will not panic because of the check above
		return NewInterval(val, val)
	}
	blockState, visited := a.state[block]
	if !visited {
		return Bottom()
	}
	if iv, ok := blockState[v]; ok {
		return iv
	}
	return Top() // Value is unknown
}

func (a *Analyzer) lookupOrComputeSummary(callee *ssa.Function, args []Interval) FunctionSummary {
	// Skip functions outside the target packages — return conservative Top.
	if a.targetPkgs != nil && !a.targetPkgs[callee.Package()] {
		return FunctionSummary{Params: args, Returns: []Interval{Top()}}
	}

	if a.callDepth >= a.maxCallDepth {
		return FunctionSummary{Params: args, Returns: []Interval{Top()}}
	}

	summaries := a.summaries[callee]
	// No need to handle not found here. Will simply not loop.
	for _, summary := range summaries {
		if summary.ArgsMatch(args) {
			return summary
		}
	}
	child := Analyzer{
		resolver:       a.resolver,
		summaries:      a.summaries, // Inherit summaries from callee
		paramOverrides: args,
		callDepth:      a.callDepth + 1,
		maxCallDepth:   a.maxCallDepth,
		targetPkgs:     a.targetPkgs,
	}
	child.Analyze(callee)

	returns := child.computeReturnIntervals(callee)
	summary := FunctionSummary{Params: args, Returns: returns}
	a.summaries[callee] = append(a.summaries[callee], summary)
	return summary
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

func (a *Analyzer) refineFromCondition(block *ssa.BasicBlock, cond *ssa.BinOp, isTrueBranch bool) {
	var variable ssa.Value
	var constVal int64
	if c, ok := cond.Y.(*ssa.Const); ok && c.Value != nil && c.Value.Kind() == constant.Int {
		variable = cond.X
		constVal = c.Int64()
	} else if c, ok := cond.X.(*ssa.Const); ok && c.Value != nil && c.Value.Kind() == constant.Int {
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
			refined = current.Meet(NewInterval(constVal, constVal))
		}
	case token.EQL: // y == 0
		if isTrueBranch {
			refined = current.Meet(NewInterval(constVal, constVal))
		} else {
			refined = current.ExcludeZero()
		}
	}
	return refined
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

func (a *Analyzer) resolveCallees(v *ssa.Call) []*ssa.Function {
	if a.resolver != nil {
		return a.resolver.Resolve(v)
	}
	if callee := v.Call.StaticCallee(); callee != nil {
		return []*ssa.Function{callee}
	}
	return nil
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
	case token.QUO:
		result = x.Div(y)
	case token.REM:
		result = x.Rem(y)
	default:
		result = Top()
	}
	a.state[block][v] = result

}

func (a *Analyzer) lookupSliceLen(block *ssa.BasicBlock, v ssa.Value) Interval {
	if sl, ok := a.sliceLens[block][v]; ok {
		return sl
	}
	return NewInterval(0, math.MaxInt64)
}

func (a *Analyzer) transferCall(block *ssa.BasicBlock, v *ssa.Call) {
	// Get the callee
	callees := a.resolveCallees(v)
	if builtin, ok := v.Call.Value.(*ssa.Builtin); ok {
		if builtin.Name() == "len" {
			if len(v.Call.Args) == 1 {
				arg := v.Call.Args[0]
				if sliceLen, ok := a.sliceLens[block][arg]; ok {
					a.state[block][v] = sliceLen
				} else {
					a.state[block][v] = NewInterval(0, math.MaxInt64)
				}
				return
			}
		}
		if builtin.Name() == "append" {
			if len(v.Call.Args) >= 2 {
				oldSlice := v.Call.Args[0]
				addedSlice := v.Call.Args[1]

				oldLen := a.lookupSliceLen(block, oldSlice)
				addedLen := a.lookupSliceLen(block, addedSlice)
				a.sliceLens[block][v] = oldLen.Add(addedLen)
			}
			a.state[block][v] = Top()
			return
		}
	}
	if len(callees) == 0 {
		a.state[block][v] = Top()
		return
	}

	args := make([]Interval, len(v.Call.Args))
	for i, arg := range v.Call.Args {
		args[i] = a.lookupInterval(block, arg)
	}

	result := Bottom()
	for _, callee := range callees {
		summary := a.lookupOrComputeSummary(callee, args)
		if len(summary.Returns) > 0 {
			result = result.Join(summary.Returns[0])
		}
	}
	a.state[block][v] = result
}

func (a *Analyzer) transferConvert(block *ssa.BasicBlock, v *ssa.Convert) {
	a.state[block][v] = a.lookupInterval(block, v.X)
}

func (a *Analyzer) transferInstruction(block *ssa.BasicBlock, instr ssa.Instruction) {
	switch v := instr.(type) {
	case *ssa.BinOp:
		a.transferBinOp(block, v)
	case *ssa.Phi:
		a.transferPhi(block, v)
	case *ssa.UnOp:
		a.transferUnOp(block, v)
	case *ssa.Convert:
		a.transferConvert(block, v)
	case *ssa.Call:
		a.transferCall(block, v)
	case *ssa.MakeSlice:
		a.transferMakeSlice(block, v)
	case *ssa.Slice:
		a.transferSlice(block, v)
	}
}

func (a *Analyzer) transferMakeSlice(block *ssa.BasicBlock, v *ssa.MakeSlice) {
	lenInterval := a.lookupInterval(block, v.Len)
	a.sliceLens[block][v] = lenInterval
}

func (a *Analyzer) transferSlice(block *ssa.BasicBlock, v *ssa.Slice) {
	var lowBoundInterval Interval
	if v.Low == nil {
		// no lower bound. [0,0]
		lowBoundInterval = NewInterval(0, 0)
	} else {
		lowBoundInterval = a.lookupInterval(block, v.Low)
	}

	highBoundInterval := Top()
	if v.High == nil {
		// No explicit high bound. We need to figure out the length of v.X
		ptr, ok := v.X.Type().Underlying().(*types.Pointer)
		if ok {
			arr, ok := ptr.Elem().Underlying().(*types.Array)
			if ok {
				length := arr.Len()
				highBoundInterval = NewInterval(length, length)
			}
		}
		if highBoundInterval.IsTop {
			if sliceLen, ok := a.sliceLens[block][v.X]; ok {
				highBoundInterval = sliceLen
			}
		}
	} else {
		highBoundInterval = a.lookupInterval(block, v.High)
	}

	a.sliceLens[block][v] = highBoundInterval.Sub(lowBoundInterval)
}

func (a *Analyzer) transferPhi(block *ssa.BasicBlock, v *ssa.Phi) {
	result := Bottom()
	for i, edge := range v.Edges {
		pred := block.Preds[i]
		result = result.Join(a.lookupInterval(pred, edge))
	}

	a.state[block][v] = result
}

func (a *Analyzer) transferUnOp(block *ssa.BasicBlock, v *ssa.UnOp) {
	x := a.lookupInterval(block, v.X)
	var result Interval
	switch v.Op {
	case token.SUB:
		result = x.Neg()
	default:
		result = Top()
	}

	a.state[block][v] = result
}

type Finding struct {
	Pos      token.Pos
	Message  string
	Severity Severity
}

type Severity uint8

const maxIterations = 1000

func isLoopHeader(block *ssa.BasicBlock, rpoIndex map[*ssa.BasicBlock]int) bool {
	blockRepoIndex := rpoIndex[block]
	for _, pred := range block.Preds {
		if rpoIndex[pred] > blockRepoIndex {
			return true
		}
	}
	return false
}

func stateEqual(s1, s2 map[ssa.Value]Interval) bool {
	return maps.Equal(s1, s2)
}
