package analysis

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

func NewCHAResolver(prog *ssa.Program) *CHAResolver {
	graph := BuildCallGraph(prog)
	return &CHAResolver{
		prog,
		graph,
	}
}

type CHAResolver struct {
	prog  *ssa.Program
	graph *callgraph.Graph
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
	w := bufio.NewWriter(os.Stdout)
	for _, pkg := range r.prog.AllPackages() {
		for name, member := range pkg.Members {
			fn, ok := member.(*ssa.Function)
			if !ok {
				continue
			}
			node, ok := r.graph.Nodes[fn]
			if !ok {
				continue
			}
			if node == nil {
				fmt.Fprintf(w, "%s not in callgrpah\n", name)
				continue
			}

			fmt.Fprintf(w, "Function %s\n", name)
			for _, param := range fn.Params {
				fmt.Fprintf(w, " Param %s %s\n", param.Name(), param.Type().String())
			}
			fmt.Fprintf(w, "   Callers: \n")
			for _, edge := range node.In {
				fmt.Printf("    <- %s\n", edge.Caller.Func.RelString(pkg.Pkg))
			}
			fmt.Fprintf(w, "  Callees: \n")

			for _, edge := range node.Out {
				fmt.Fprintf(w, "    -> %s  [site: %s]\n",
					edge.Callee.Func.RelString(pkg.Pkg),
					edge.Site, // the call instruction (ssa.CallInstruction)
				)
			}
		}
	}
}
