package analysis

import (
	"maps"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type ParamNilStates struct {
	// paramStates maps each function to the nil state of its parameters.
	// Computed by analyzing all call sites in the program.
	paramsStates map[*ssa.Function][]NilState
}

func (p *ParamNilStates) States() map[*ssa.Function][]NilState {
	return p.paramsStates
}

type callSite struct {
	caller *ssa.Function
	block  *ssa.BasicBlock
	args   []ssa.Value
}

func ComputeParamNilStatesAnalysis(
	pkgs []*ssa.Package,
	analyzer *NilAnalyzer,
) *ParamNilStates {
	p := &ParamNilStates{
		paramsStates: make(map[*ssa.Function][]NilState),
	}
	sites := p.collectCallSites(pkgs, analyzer.Graph())

	pkgsMap := make(map[*ssa.Package]bool)
	for _, pkg := range pkgs {
		if pkg != nil {
			pkgsMap[pkg] = true
		}
	}

	var allFunctions []*ssa.Function
	if graph := analyzer.Graph(); graph != nil {
		for fn, node := range graph.Nodes {
			if node == nil || node.Func == nil {
				continue
			}
			pkg := node.Func.Package()
			if pkg != nil && pkgsMap[pkg] {
				allFunctions = append(allFunctions, fn)
			}
		}
	} else {
		for _, pkg := range pkgs {
			if pkg == nil {
				continue
			}
			for _, member := range pkg.Members {
				if fn, ok := member.(*ssa.Function); ok {
					allFunctions = append(allFunctions, fn)
				}
			}
		}
	}
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}

		if initFn := pkg.Func("init"); initFn != nil {
			allFunctions = append(allFunctions, initFn)
		}
	}

	var prevGlobalStates map[addressKey]NilState
	maxIterations := 5
	for iter := 0; iter < maxIterations; iter++ {
		// Analyze all functions with current param states.
		for _, fn := range allFunctions {
			analyzer.Analyze(fn)
		}

		globalStates := make(map[addressKey]NilState)
		for _, fn := range allFunctions {
			for _, block := range fn.Blocks {
				for _, instr := range block.Instrs {
					store, ok := instr.(*ssa.Store)
					if !ok {
						continue
					}
					key, ok := resolveAddress(store.Addr)
					if !ok || key.kind != addrGlobal {
						continue
					}
					// Look up stored value's nil state from converged analysis
					state := classifyArg(store.Val)
					if cs := analyzer.convergedStates[fn]; cs != nil {
						if bs := cs[block]; bs != nil {
							if s, ok := bs[store.Val]; ok {
								state = s
							}
						}
					}
					if existing, ok := globalStates[key]; ok {
						globalStates[key] = existing.Join(state)
					} else {
						globalStates[key] = state
					}
				}
			}
		}

		analyzer.SetGlobalNilStates(globalStates)

		changed := !maps.Equal(prevGlobalStates, globalStates)
		prevGlobalStates = globalStates
		for callee, callSites := range sites {
			nParams := len(callee.Params)
			if nParams == 0 {
				continue
			}

			paramStates := make([]NilState, nParams)
			for i := range paramStates {
				paramStates[i] = NilBottom
			}

			for _, site := range callSites {
				// Look up argument nil state from caller's converged state.
				callerState := analyzer.convergedStates[site.caller]
				if callerState == nil {
					// DEFENSIVE: unreachable under current implementation.
					// allFunctions and collectCallSites both iterate the same pkgs.Members,
					// so every caller that produces a call site IS in allFunctions and
					// gets analyzed (populating convergedStates). The only way callerState
					// could be nil is if a caller has fn.Blocks == nil (external/assembly),
					// but collectCallSites skips such functions (no blocks → no inner loop).
					// This guard exists for safety against future changes (e.g., if
					// allFunctions and collectCallSites diverge in scope).
					// Falls back to context-free classification.
					for i := 0; i < nParams && i < len(site.args); i++ {
						paramStates[i] = paramStates[i].Join(classifyArg(site.args[i]))
					}
					continue
				}

				blockState := callerState[site.block]
				for i := 0; i < nParams && i < len(site.args); i++ {
					if blockState != nil {
						if s, ok := blockState[site.args[i]]; ok {
							paramStates[i] = paramStates[i].Join(s)
							continue
						}
					}
					// Fall back to context-free classification.
					paramStates[i] = paramStates[i].Join(classifyArg(site.args[i]))
				}
			}

			// Check if anything changed
			old := p.paramsStates[callee]
			if !nilStatesEqual(old, paramStates) {
				changed = true
			}
			p.paramsStates[callee] = paramStates
		}

		// Update analyzer's param states for next iteration.
		analyzer.paramNilStates = p

		if !changed {
			break
		}
	}

	return p
}

func nilStatesEqual(a, b []NilState) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ComputeParamNilStates(prog *ssa.Program, pkgs []*ssa.Package, graph *callgraph.Graph) *ParamNilStates {
	p := &ParamNilStates{
		paramsStates: make(map[*ssa.Function][]NilState),
	}

	sites := p.collectCallSites(pkgs, graph)

	for callee, callSites := range sites {
		nParams := len(callee.Params)
		if nParams == 0 {
			continue
		}
		paramStates := make([]NilState, nParams)
		for i := range paramStates {
			paramStates[i] = NilBottom
		}

		// Join argument nil states across all call sites.
		for _, site := range callSites {
			for i := 0; i < nParams && i < len(site.args); i++ {
				paramStates[i] = paramStates[i].Join(classifyArg(site.args[i]))
			}
		}

		p.paramsStates[callee] = paramStates
	}

	return p
}

func (p *ParamNilStates) collectCallSites(pkgs []*ssa.Package, graph *callgraph.Graph) map[*ssa.Function][]callSite {
	if graph == nil {
		return p.collectCallSitesByWalk(pkgs)
	}
	sites := make(map[*ssa.Function][]callSite)

	pkgsMap := map[*ssa.Package]interface{}{}
	for _, pkg := range pkgs {
		pkgsMap[pkg] = struct{}{}
	}

	for fn, node := range graph.Nodes {
		if node == nil || node.Func == nil {
			continue
		}
		pkg := node.Func.Package()
		if pkg == nil {
			continue
		}

		if _, exists := pkgsMap[pkg]; !exists {
			continue
		}

		for _, edge := range node.In {
			sites[fn] = append(sites[fn], callSite{edge.Caller.Func, edge.Site.Block(), edge.Site.Common().Args})
		}
	}

	return sites
}

func (p *ParamNilStates) collectCallSitesByWalk(pkgs []*ssa.Package) map[*ssa.Function][]callSite {
	// 1. Iterate all packages
	// 2. For each package, iterate all Members to find *ssa.Function
	// 3. For each function, iterate all blocks and instructions
	// 4. When we see *ssa.Call — get the static callee via StaticCallee(), record the args
	// 5. When we see *ssa.Go — same thing, it has a .Call field with the same structure
	// 6. Skip calls where StaticCallee() returns nil (interface dispatch — we'll handle that later)

	sites := make(map[*ssa.Function][]callSite)

	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}
		for _, member := range pkg.Members {
			if fn, ok := member.(*ssa.Function); ok {
				for _, block := range fn.Blocks {
					for _, instr := range block.Instrs {
						if call, ok := instr.(*ssa.Call); ok {
							callee := call.Call.StaticCallee()
							if callee == nil {
								continue // interface dispatch .. Skip for now (TODO)
							}
							sites[callee] = append(sites[callee], callSite{
								caller: fn,
								block:  block,
								args:   call.Call.Args,
							})
						} else if goCall, ok := instr.(*ssa.Go); ok {
							callee := goCall.Call.StaticCallee()
							if callee == nil {
								continue
							}
							sites[callee] = append(sites[callee], callSite{
								caller: fn,
								block:  block,
								args:   goCall.Call.Args,
							})
						}
					}
				}
			}
		}
	}
	return sites
}

func classifyArg(arg ssa.Value) NilState {
	switch v := arg.(type) {
	case *ssa.Const:
		if v.IsNil() {
			return DefinitelyNil
		}
		return DefinitelyNotNil
	case *ssa.Alloc:
		return DefinitelyNotNil // new(T) or &x
	case *ssa.MakeInterface:
		return DefinitelyNotNil
	case *ssa.MakeSlice:
		return DefinitelyNotNil
	case *ssa.MakeMap:
		return DefinitelyNotNil
	case *ssa.MakeChan:
		return DefinitelyNotNil
	case *ssa.FieldAddr:
		return DefinitelyNotNil
	case *ssa.IndexAddr:
		return DefinitelyNotNil
	default:
		if !isNillable(arg) {
			return DefinitelyNotNil // int, bool, string, etc.
		}
		return MaybeNil
	}
}
