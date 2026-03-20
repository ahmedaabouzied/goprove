package analysis

import (
	"golang.org/x/tools/go/ssa"
)

type ParamNilStates struct {
	// paramStates maps each function to the nil state of its parameters.
	// Computed by analyzing all call sites in the program.
	paramsStates map[*ssa.Function][]NilState
}

type callSite struct {
	caller *ssa.Function
	args   []ssa.Value
}

func ComputeParamNilStates(prog *ssa.Program, pkgs []*ssa.Package) *ParamNilStates {
	p := &ParamNilStates{
		paramsStates: make(map[*ssa.Function][]NilState),
	}

	sites := p.collectCallSites(pkgs)

	for callee, callSites := range sites {
		// Skip exported functions — can't see all callers.
		if callee.Object() != nil && callee.Object().Exported() {
			continue
		}

		// Initialize param states to NilBottom (identity for Join).
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

func (p *ParamNilStates) collectCallSites(pkgs []*ssa.Package) map[*ssa.Function][]callSite {
	// 1. Iterate all packages
	// 2. For each package, iterate all Members to find *ssa.Function
	// 3. For each function, iterate all blocks and instructions
	// 4. When we see *ssa.Call — get the static callee via StaticCallee(), record the args
	// 5. When we see *ssa.Go — same thing, it has a .Call field with the same structure
	// 6. Skip calls where StaticCallee() returns nil (interface dispatch — we'll handle that later)

	sites := make(map[*ssa.Function][]callSite)

	for _, pkg := range pkgs {
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
								args:   call.Call.Args,
							})
						} else if goCall, ok := instr.(*ssa.Go); ok {
							callee := goCall.Call.StaticCallee()
							if callee == nil {
								continue
							}
							sites[callee] = append(sites[callee], callSite{
								caller: fn,
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
