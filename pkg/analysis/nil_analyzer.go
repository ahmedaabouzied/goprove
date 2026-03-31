package analysis

import (
	"fmt"
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type NilAnalyzer struct {
	state           map[*ssa.BasicBlock]map[ssa.Value]NilState
	summaries       map[*ssa.Function]NilFunctionSummary
	summaryCache    *SummaryCache
	callDepth       int
	maxCallDepth    int
	resolver        *CHAResolver
	paramNilStates  *ParamNilStates
	targetPkgs      map[*ssa.Package]bool
	findings        []Finding
	convergedStates map[*ssa.Function]map[*ssa.BasicBlock]map[ssa.Value]NilState

	// func level. Gets reset at each call of Analyze().
	addrState map[*ssa.BasicBlock]map[addressKey]NilState

	// package level. Tracks all addresses across the package.
	convergedAddrState  map[*ssa.Function]map[*ssa.BasicBlock]map[addressKey]NilState
	globalAddressStates map[addressKey]NilState
	err                 error
}

type NilFunctionSummary struct {
	Returns []NilState
}

func NewNilAnalyzer(resolver *CHAResolver, paramStates *ParamNilStates, cache *SummaryCache) *NilAnalyzer {
	return &NilAnalyzer{
		resolver:       resolver,
		paramNilStates: paramStates,
		summaries:      make(map[*ssa.Function]NilFunctionSummary),
		summaryCache:   cache,
		maxCallDepth:   10,
	}
}

func (a *NilAnalyzer) Graph() *callgraph.Graph {
	if a.resolver == nil {
		return nil
	}
	return a.resolver.graph
}

func (a *NilAnalyzer) SetParamNilStates(states *ParamNilStates) {
	a.paramNilStates = states
}

func (a *NilAnalyzer) SetGlobalNilStates(states map[addressKey]NilState) {
	a.globalAddressStates = states
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

	if a.globalAddressStates != nil {
		if a.addrState[blocks[0]] == nil {
			a.addrState[blocks[0]] = make(map[addressKey]NilState)
		}
		for k, v := range a.globalAddressStates {
			a.addrState[blocks[0]][k] = v
		}
	}

	// Initialize convergedAddrState only on the root Analyze() call where it has not been initialized before.
	if a.convergedAddrState == nil {
		a.convergedAddrState = make(map[*ssa.Function]map[*ssa.BasicBlock]map[addressKey]NilState)
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
	a.convergedAddrState[fn] = a.addrState

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
			// Example: x.String()
			// SSA: t0 = Invoke x.String()
			a.flagNilDeref(block, v.Call.Value, v.Pos(), reported)
			break
		} else if v.Call.StaticCallee() == nil {
			// Func value call: fn(), where fn is a variable holding a function.
			// StaticCallee is nil because the callee isn't known at compile time.
			// v.Call.Value is the func-typed value being called — check its nil state.
			if _, isBuiltin := v.Call.Value.(*ssa.Builtin); !isBuiltin {
				// Builtins are always non nil
				a.flagNilDeref(block, v.Call.Value, v.Pos(), reported)
			}
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
	if _, ok := v.(*ssa.Function); ok {
		// functions are always non-nil
		return DefinitelyNotNil
	}

	if _, ok := v.(*ssa.Global); ok {
		// Globals are always non-nil.
		// They're named variables holding addresses of
		// package level variables.
		// Example:
		// Global array:
		// var vchars = [256]byte{'"':2, '{': 3}
		// Global value:
		// var defaultTimeout = 30 * time.Second
		// Global struct:
		// var mu sync.Mutex
		// Global pointer:
		// var DefaultOptions *Options
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
	if len(block.Preds) != 1 {
		// The block has multiple predecessors. The initBlockState call captured the correct state already.
		return
	}
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
	if a.summaryCache != nil {
		if summary, ok := a.summaryCache.Get(fn.RelString(nil)); ok {
			a.summaries[fn] = summary
			return summary
		}
	}

	if a.callDepth >= a.maxCallDepth {
		return NilFunctionSummary{Returns: []NilState{MaybeNil}}
	}

	// Cache a conservative sentinel before analyzing to break recursive calls.
	a.summaries[fn] = NilFunctionSummary{Returns: []NilState{MaybeNil}}

	childNilAnalyzer := NewNilAnalyzer(a.resolver, a.paramNilStates, a.summaryCache)
	childNilAnalyzer.callDepth = a.callDepth + 1
	childNilAnalyzer.summaries = a.summaries
	childNilAnalyzer.convergedAddrState = a.convergedAddrState
	childNilAnalyzer.targetPkgs = a.targetPkgs

	_ = childNilAnalyzer.Analyze(fn)

	returns := childNilAnalyzer.computeReturnNilStates(fn)
	summary := NilFunctionSummary{Returns: returns}
	a.summaries[fn] = summary
	return summary
}

// SummarizeFunction analyzes fn and returns its nil
// return states. Used by cache generation to extract
// summaries for individual functions.
func (a *NilAnalyzer) SummarizeFunction(fn *ssa.Function) []NilState {
	a.Analyze(fn)
	return a.computeReturnNilStates(fn)
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
	case *ssa.MakeClosure:
		a.transferMakeClosure(block, v)
	case *ssa.Store:
		a.transferStoreOp(block, v)
	case *ssa.UnOp:
		a.transferUnOpInstr(block, v)
	case *ssa.Call:
		a.transferCall(block, v)
	case *ssa.Extract:
		a.transferExtractInstr(block, v)
	case *ssa.TypeAssert:
		a.transferTypeAssertInstr(block, v)
	case *ssa.Lookup:
		a.transferMapLookup(block, v)
	}
}

func (a *NilAnalyzer) transferUnOpInstr(block *ssa.BasicBlock, v *ssa.UnOp) {
	/*
		Handling this Go code example where init() sets a var (pointer to something) and then this var is passed to a function
		var db *int new(int)
		init(){
				db = 1 // Whatever .. Just an example
		}

		func query(p *int) int {
			return *p
		}

		func handler() int {
			return query(db) // passes global var as an argument
		}

		handle() in SSA is represented as:
		entry:
			t0 = *db:*int <- UnOp MUL loading the value to the global db
			t1 = query(t0)
			return t1
	*/
	if v.Op == token.MUL {
		if key, ok := resolveAddress(v.X); ok {
			if m, ok := a.addrState[block]; ok {
				if s, ok := m[key]; ok {
					a.state[block][v] = s
				}
			}
		}
	}
}

func (a *NilAnalyzer) transferMakeClosure(block *ssa.BasicBlock, v *ssa.MakeClosure) {

	/*
		A MakeClosure appears when the anonymous function captures variables from its enclosing scope (free variables):

		  func makeAdder(x int) func(int) int {
		      return func(y int) int { return x + y }
		      //                              ^ captures x
		  }

		  SSA for makeAdder:
		  Block 0:
		    t0 = MakeClosure makeAdder$1 [x]    → type: func(int) int
		    Return t0

		  The [x] is the captured free variable. Because there's a capture, SSA must create a closure at runtime → MakeClosure.

		  Compare with your makeHandler which has no captures:

		  func makeHandler() func() int {
		      return func() int { return 42 }
		      //                         ^ no captures
		  }


		  SSA:
		  Block 0:
		    Return makeHandler$1

		  No captures → no MakeClosure → returns the *ssa.Function
		  directly.
	*/
	a.state[block][v] = DefinitelyNotNil
}

func (a *NilAnalyzer) transferMapLookup(block *ssa.BasicBlock, v *ssa.Lookup) {
	/*
		Example 1 (Not CommaOk):
		v := m[key]
		SSA Represtantion:
		t0 = Lookup m key  -> type: V

		Example 2 (CommaOk):
		v, ok := m[key]
		t0 = Lookup m key, ok  -> type (V, bool)
		t1 = Extract t0 #0     -> type: V
		t2 = Extract t0 #1     -> type: bool
	*/
	if !v.CommaOk {
		if isNillable(v) {
			a.state[block][v] = MaybeNil
			return
		}
		a.state[block][v] = DefinitelyNotNil
		return
	}
	a.state[block][v] = MaybeNil
}

func (a *NilAnalyzer) transferTypeAssertInstr(block *ssa.BasicBlock, v *ssa.TypeAssert) {
	/*
		Example 1 (Not CommaOk):
		v := x.(T)
		SSA Representation is:
		t0 = TypeAssert x.(*T)
		If the assertion fails this panics.

		Example 2 (CommaOk):
		v, ok := x.(T)
		t0 = TypeAssert x.(*T) ,ok → type: (*T, bool)
		t1 = Extract t0 #0     → type: *T
		t2 = Extract t0 #1     → type: bool
	*/
	if !v.CommaOk {
		// In both cases of v being nillable or not,
		// because this op panics on failure,
		// if we're here, the conversion succeeded
		a.state[block][v] = DefinitelyNotNil
	} else {
		// CommaOk. This is an tuple instruction now
		a.state[block][v] = MaybeNil
	}
}

func (a *NilAnalyzer) transferExtractInstr(block *ssa.BasicBlock, v *ssa.Extract) {
	if call, isCall := v.Tuple.(*ssa.Call); isCall {
		/*
			Example: f, err := os.Open(path)
			t0 = Call os.Open(path)        → type: (*os.File, error)  [this is a *ssa.Call]
			t1 = Extract t0 #0             → type: *os.File            [this is *ssa.Extract, Index=0]
			t2 = Extract t0 #1             → type: error               [this is *ssa.Extract, Index=1]
		*/
		if callee := call.Call.StaticCallee(); callee != nil {
			summary := a.lookupOrComputeSummary(callee)
			if v.Index < len(summary.Returns) {
				a.state[block][v] = summary.Returns[v.Index]
				return
			}
			// It's not expected to have summary of the callee
			// having less returns than our value index.
			// We default to MaybeNil here if this happens.
			a.state[block][v] = MaybeNil
			return
		}
		// Callee is nil. Strange.
		// We fall back to MaybeNil
		a.state[block][v] = MaybeNil
		return
	}
	// Can be an *ssa.TypeAssert
	if ta, isTA := v.Tuple.(*ssa.TypeAssert); isTA && ta.CommaOk {
		/*
			An example is:
				type tHelper interface {
			      Helper()
			  }

			  if h, ok := t.(tHelper); ok {
			      h.Helper()  // ← Warning: h might be nil
			  }

			  This compiles to the same SSA pattern:

			  t0 = TypeAssert t <tHelper> ,ok
			  t1 = Extract t0 #1          (ok bool)
			  if t1 goto true else false
			  true:
			    t2 = Extract t0 #0        (tHelper — DefinitelyNotNil here)
			    invoke t2.Helper()         ← flagged as MaybeNil
		*/
		if v.Index == 1 {
			// ok bool not nillable
			a.state[block][v] = DefinitelyNotNil
			return
		}
		// Index 0 - The asserted value
		if len(block.Preds) == 1 {
			pred := block.Preds[0]
			if ifInstr, ok := pred.Instrs[len(pred.Instrs)-1].(*ssa.If); ok {
				if okExtract, ok := ifInstr.Cond.(*ssa.Extract); ok {
					if okExtract.Tuple == ta && okExtract.Index == 1 && pred.Succs[0] == block {
						if isNillable(v) {
							a.state[block][v] = DefinitelyNotNil
							return
						}
					}
				}
			}
		}
	}
	// Not an *ssa.Call. or *ssa.TypeAssert. Fallback to MaybeNil.
	a.state[block][v] = MaybeNil
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
