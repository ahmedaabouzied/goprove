package analysis

import (
	"fmt"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/ssa"
)

func BuildCallGraph(prog *ssa.Program) *callgraph.Graph {
	return cha.CallGraph(prog)
}

func printCallgraph(cg *callgraph.Graph, prog *ssa.Program) {
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
