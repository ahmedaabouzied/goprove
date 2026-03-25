package analysis

import (
	"fmt"
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/ssa"
)

type NilAnalyzer struct {
	state           map[*ssa.BasicBlock]map[ssa.Value]NilState
	summaries       map[*ssa.Function]NilFunctionSummary
	callDepth       int
	maxCallDepth    int
	resolver        *CHAResolver
	paramNilStates  *ParamNilStates
	targetPkgs      map[*ssa.Package]bool
	findings        []Finding
	convergedStates map[*ssa.Function]map[*ssa.BasicBlock]map[ssa.Value]NilState
	addrState       map[*ssa.BasicBlock]map[addressKey]NilState
	err             error
}

type NilFunctionSummary struct {
	Returns []NilState
}

func NewNilAnalyzer(resolver *CHAResolver, paramStates *ParamNilStates) *NilAnalyzer {
	return &NilAnalyzer{
		resolver:       resolver,
		paramNilStates: paramStates,
		summaries:      make(map[*ssa.Function]NilFunctionSummary),
		maxCallDepth:   3,
	}
}

func (a *NilAnalyzer) SetParamNilStates(states *ParamNilStates) {
	a.paramNilStates = states
}

// SetTargetPackages limits interprocedural analysis to the given packages.
// Calls to functions outside these packages return conservative results
// (MaybeNil) instead of recursively analyzing the callee body.
func (a *NilAnalyzer) SetTargetPackages(pkgs []*ssa.Package) {
	a.targetPkgs = make(map[*ssa.Package]bool, len(pkgs))
	for _, pkg := range pkgs {
		if pkg != nil {
			a.targetPkgs[pkg] = true
		}
	}
}

func (a *NilAnalyzer) Analyze(fn *ssa.Function) []Finding {
	a.state = make(map[*ssa.BasicBlock]map[ssa.Value]NilState)
	a.addrState = make(map[*ssa.BasicBlock]map[addressKey]NilState)
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

	if a.paramNilStates != nil {
		if states, ok := a.paramNilStates.paramsStates[fn]; ok {
			if a.state[blocks[0]] == nil {
				a.state[blocks[0]] = make(map[ssa.Value]NilState)
			}
			for i, param := range fn.Params {
				if i < len(states) && isNillable(param) {
					a.state[blocks[0]][param] = states[i]
				}
			}
		}
	}

	if fn.Signature.Recv() != nil && len(fn.Params) > 0 && isNillable(fn.Params[0]) {
		if a.state[blocks[0]] == nil {
			a.state[blocks[0]] = make(map[ssa.Value]NilState)
		}
		a.state[blocks[0]][fn.Params[0]] = DefinitelyNotNil
	}

	// Variadic parameters: a nil variadic slice is idiomatic Go.
	// `for _, v := range opts` on a nil slice is a no-op, not a crash.
	// Cap at MaybeNil so it never triggers a Bug-severity finding.
	if fn.Signature.Variadic() && len(fn.Params) > 0 {
		lastParam := fn.Params[len(fn.Params)-1]
		if a.state[blocks[0]] != nil && a.state[blocks[0]][lastParam] == DefinitelyNil {
			a.state[blocks[0]][lastParam] = MaybeNil
		}
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
		a.initBlockState(block)

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
	reported := make(map[string]bool)
	for _, block := range blocks {
		for _, instr := range block.Instrs {
			a.checkInstruction(block, instr, reported)
		}
	}

	if a.convergedStates == nil {
		a.convergedStates = make(map[*ssa.Function]map[*ssa.BasicBlock]map[ssa.Value]NilState)
	}
	a.convergedStates[fn] = a.state

	return a.findings
}

func (a *NilAnalyzer) initBlockState(block *ssa.BasicBlock) {
	if len(block.Preds) == 0 {
		// Entry block. Keep existing state (receiver init, etc)
		return
	}

	// Start fresh for this block
	newState := make(map[ssa.Value]NilState)

	// Join all predecessor states
	for _, pred := range block.Preds {
		predState, ok := a.state[pred]
		if !ok {
			continue
		}

		for v, s := range predState {
			if existing, ok := newState[v]; ok {
				newState[v] = existing.Join(s)
			} else {
				newState[v] = s
			}
		}
	}
	a.state[block] = newState

	newAddrState := make(map[addressKey]NilState)
	for _, pred := range block.Preds {
		predAddr, ok := a.addrState[pred]
		if !ok {
			continue
		}

		for k, s := range predAddr {
			if existing, ok := newAddrState[k]; ok {
				newAddrState[k] = existing.Join(s)
			} else {
				newAddrState[k] = s
			}
		}
	}

	if len(newAddrState) > 0 {
		a.addrState[block] = newAddrState
	}
}

func (a *NilAnalyzer) checkInstruction(block *ssa.BasicBlock, instr ssa.Instruction, reported map[string]bool) {
	switch v := instr.(type) {
	case *ssa.Call:
		// When we call s.Method() on an interface, the ssa represents
		// this as *ssa.Call with IsInvoke() true. The receiver is
		// then v.Call.Value. We should check the nil state of
		// v.Call.Value.
		if v.Call.IsInvoke() {
			a.flagNilDeref(block, v.Call.Value, v.Pos(), reported)
		}
	case *ssa.UnOp:
		// Only pointer dereference: *p
		if v.Op == token.MUL {
			if _, isGlobal := v.X.(*ssa.Global); isGlobal {
				break
			}
			a.flagNilDeref(block, v.X, v.Pos(), reported)
		}
	case *ssa.FieldAddr:
		// p.Field — v.X is the struct pointer
		a.flagNilDeref(block, v.X, v.Pos(), reported)
	case *ssa.IndexAddr:
		if isSliceType(v.X) {
			// Only flag proven nil for slices. MaybeNil is too noisy. It's the job of the bound checker to check slice bound access panics.
			if a.lookupNilState(block, v.X) == DefinitelyNil {
				name := nilValueName(v.X)
				if reported[name] {
					break
				}
				reported[name] = true
				a.findings = append(a.findings, Finding{
					Pos:      v.Pos(),
					Message:  fmt.Sprintf("nil dereference of %s — slice is always nil", name),
					Severity: Bug,
				})
			}
		} else {
			// p[i] — v.X is the slice/array pointer
			a.flagNilDeref(block, v.X, v.Pos(), reported)
		}
	}
}

func isSliceType(v ssa.Value) bool {
	_, isSlice := v.Type().Underlying().(*types.Slice)
	return isSlice
}

// nilValueName returns a human-readable name for the value being dereferenced.
// For parameters it returns the source name (e.g., "config").
// For global loads it returns the global name (e.g., "globalPtr").
// For nil constants it returns "nil pointer".
// For call results it returns "result of funcName()".
// For SSA register names (t0, t1, ...) it returns "" to avoid confusing users.
func nilValueName(v ssa.Value) string {
	switch val := v.(type) {
	case *ssa.Const:
		if val.IsNil() {
			return "nil pointer"
		}
		return ""
	case *ssa.Parameter:
		return val.Name()
	case *ssa.UnOp:
		if g, ok := val.X.(*ssa.Global); ok {
			return g.Name()
		}
		return ""
	case *ssa.Call:
		if callee := val.Call.StaticCallee(); callee != nil {
			return fmt.Sprintf("result of %s()", callee.Name())
		}
		return ""
	case *ssa.Alloc:
		if val.Comment != "" {
			return val.Comment
		}
		return ""
	case *ssa.Phi:
		// Don't recurse into Phi edges — they can form cycles (loop back-edges).
		return ""
	default:
		return ""
	}
}

func (a *NilAnalyzer) flagNilDeref(block *ssa.BasicBlock, v ssa.Value, pos token.Pos, reported map[string]bool) {
	name := nilValueName(v)
	state := a.lookupNilState(block, v)

	// Dedup key: use name if meaningful, otherwise fall back to pos.
	dedupKey := name
	if dedupKey == "" {
		dedupKey = fmt.Sprintf("pos:%d", pos)
	}

	switch state {
	case DefinitelyNil:
		if reported[dedupKey] {
			return
		}
		reported[dedupKey] = true
		if name != "" {
			a.findings = append(a.findings, Finding{
				Pos:      pos,
				Message:  fmt.Sprintf("nil dereference of %s — value is always nil", name),
				Severity: Bug,
			})
		} else {
			a.findings = append(a.findings, Finding{
				Pos:      pos,
				Message:  "nil dereference — value is always nil",
				Severity: Bug,
			})
		}
	case MaybeNil:
		if reported[dedupKey] {
			return
		}
		reported[dedupKey] = true
		if name != "" {
			a.findings = append(a.findings, Finding{
				Pos:      pos,
				Message:  fmt.Sprintf("possible nil dereference of %s — add a nil check before use", name),
				Severity: Warning,
			})
		} else {
			a.findings = append(a.findings, Finding{
				Pos:      pos,
				Message:  "possible nil dereference — add a nil check before use",
				Severity: Warning,
			})
		}
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

	// Check if this is a load from a refined global
	if unOp, ok := v.(*ssa.UnOp); ok && unOp.Op == token.MUL {
		if key, ok := resolveAddress(unOp.X); ok {
			if m, ok := a.addrState[block]; ok {
				if s, ok := m[key]; ok {
					return s
				}
			}
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

	// An UnOp is the SSA representation of a pointer dereference op
	// Example: a = *b
	if unOp, ok := variable.(*ssa.UnOp); ok && unOp.Op == token.MUL {
		if key, ok := resolveAddress(unOp.X); ok {
			if a.addrState[block] == nil {
				a.addrState[block] = make(map[addressKey]NilState)
			}
			a.addrState[block][key] = res
		}
	}
}

func (a *NilAnalyzer) transferCall(block *ssa.BasicBlock, call *ssa.Call) {
	callees := a.resolveCallees(call)
	if len(callees) == 0 {
		a.state[block][call] = MaybeNil
		return
	}

	res := NilBottom
	for _, callee := range callees {
		summary := a.lookupOrComputeSummary(callee)
		if len(summary.Returns) > 0 {
			res = res.Join(summary.Returns[0])
		}
	}

	// If no summary produced a return (e.g., all callees had empty returns),
	// fall back to MaybeNil instead of letting NilBottom leak as DefinitelyNil.
	if res == NilBottom {
		res = MaybeNil
	}

	a.state[block][call] = res
}

func (a *NilAnalyzer) resolveCallees(v *ssa.Call) []*ssa.Function {
	if a.resolver != nil {
		return a.resolver.Resolve(v)
	}
	if callee := v.Call.StaticCallee(); callee != nil {
		return []*ssa.Function{callee}
	}

	return nil
}

func (a *NilAnalyzer) lookupOrComputeSummary(fn *ssa.Function) NilFunctionSummary {
	if summary, ok := a.summaries[fn]; ok {
		return summary
	}

	// Skip functions outside the target packages — return conservative MaybeNil.
	if a.targetPkgs != nil && !a.targetPkgs[fn.Package()] {
		return NilFunctionSummary{Returns: []NilState{MaybeNil}}
	}

	if a.callDepth >= a.maxCallDepth {
		return NilFunctionSummary{Returns: []NilState{MaybeNil}}
	}

	// Cache a conservative sentinel before analyzing to break recursive calls.
	a.summaries[fn] = NilFunctionSummary{Returns: []NilState{MaybeNil}}

	childNilAnalyzer := NewNilAnalyzer(a.resolver, a.paramNilStates)
	childNilAnalyzer.callDepth = a.callDepth + 1
	childNilAnalyzer.summaries = a.summaries
	childNilAnalyzer.targetPkgs = a.targetPkgs

	_ = childNilAnalyzer.Analyze(fn)

	returns := childNilAnalyzer.computeReturnNilStates(fn)
	summary := NilFunctionSummary{Returns: returns}
	a.summaries[fn] = summary
	return summary
}

func (a *NilAnalyzer) computeReturnNilStates(fn *ssa.Function) []NilState {
	var returns []NilState
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			if returns == nil {
				returns = make([]NilState, len(ret.Results))
				for i := range returns {
					returns[i] = NilBottom
				}
			}
			for i, result := range ret.Results {
				returns[i] = returns[i].Join(a.lookupNilState(block, result))
			}
		}
	}
	// If a return position is still NilBottom, it means the analysis
	// didn't observe any return value for that position (e.g., external
	// function, unhandled instruction, unreachable block). Fall back to
	// MaybeNil (conservative) instead of letting NilBottom leak through
	// as DefinitelyNil downstream.
	for i, s := range returns {
		if s == NilBottom {
			returns[i] = MaybeNil
		}
	}
	return returns
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
	case *ssa.FieldAddr: // &v always produces a not nil.
		a.state[block][v] = DefinitelyNotNil
	case *ssa.IndexAddr: //	&v[t1] always produces a not nil.
		a.state[block][v] = DefinitelyNotNil
	case *ssa.Convert:
		a.state[block][v] = a.lookupNilState(block, v.X)
	case *ssa.Store:
		a.transferStoreOp(block, v)
	case *ssa.Call:
		a.transferCall(block, v)
	}
}

func (a *NilAnalyzer) transferStoreOp(block *ssa.BasicBlock, v *ssa.Store) {
	if key, ok := resolveAddress(v.Addr); ok {
		if a.addrState[block] == nil {
			a.addrState[block] = make(map[addressKey]NilState)
		}
		a.addrState[block][key] = a.lookupNilState(block, v.Val)
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

// IsNillableExported is an exported wrapper for isNillable, used in tests.
func IsNillableExported(v ssa.Value) bool {
	return isNillable(v)
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
