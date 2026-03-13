package analysis

import "golang.org/x/tools/go/ssa"

type CallResolver interface {
	Resolve(call *ssa.CallInstruction) []*ssa.Function
}
