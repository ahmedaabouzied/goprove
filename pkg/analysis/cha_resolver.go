package analysis

import (
	"bufio"
	"os"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type CHAResolver struct {
	prog  *ssa.Program
	graph *callgraph.Graph
}

func NewCHAResolver(prog *ssa.Program) *CHAResolver {
	graph := BuildCallGraph(prog)
	return &CHAResolver{
		prog,
		graph,
	}
}

func (r *CHAResolver) Resolve(call ssa.CallInstruction) []*ssa.Function {
	fn := call.Common().StaticCallee()
	if fn != nil {
		return []*ssa.Function{fn}
	}

	node := r.graph.Nodes[call.Parent()]
	if node == nil {
		return nil // Caller not in the graph
	}
	res := []*ssa.Function{}
	for _, edge := range node.Out {
		if edge.Site == call {
			res = append(res, edge.Callee.Func)
		}
	}
	return res
}

func (r *CHAResolver) String() {
	bufio.NewWriter(os.Stdout)
	for _, pkg := range prog.AllPackages() {
		for name, member := range pkg.Members {
			fn, ok := member.(*ssa.Function)
			if !ok {
				continue
			}
			node, ok := cg.Nodes[fn]
			if !ok {
				continue
			}
			if node == nil {
				fmt.Printf("%s not in callgrpah\n", name)
				continue
			}

			fmt.Printf("Function %s\n", name)
			for _, param := range fn.Params {
				fmt.Printf(" Param %s %s\n", param.Name(), param.Type().String())
			}
			fmt.Printf("   Callers: \n")
			for _, edge := range node.In {
				fmt.Printf("    <- %s\n", edge.Caller.Func.RelString(pkg.Pkg))
			}
			fmt.Printf("  Callees: \n")

			for _, edge := range node.Out {
				fmt.Printf("    -> %s  [site: %s]\n",
					edge.Callee.Func.RelString(pkg.Pkg),
					edge.Site, // the call instruction (ssa.CallInstruction)
				)
			}
		}
	}
}
