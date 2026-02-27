package cfg

import (
	"fmt"

	"golang.org/x/tools/go/ssa"
)

func ReversePostOrder(fn *ssa.Function) ([]*ssa.BasicBlock, error) {
	if len(fn.Blocks) < 1 {
		return nil, fmt.Errorf("fn blocks are empty")
	}

	visited := make(map[*ssa.BasicBlock]bool, len(fn.Blocks))

	postOrder := make([]*ssa.BasicBlock, len(fn.Blocks))
	writeIdx := len(fn.Blocks) - 1

	stack := []*ssa.BasicBlock{fn.Blocks[0]}

	visited[fn.Blocks[0]] = true

	expanded := make(map[*ssa.BasicBlock]bool, len(fn.Blocks))

	// Main loop
	for len(stack) > 0 {
		block := stack[len(stack)-1] // Pop one element from the stack
		stack = stack[:len(stack)-1]

		// Check if expanded before. If expanded, record it in the postOrder list.
		if expanded[block] { // Missing keys return false. Safe to not consider the "ok" value of map lookup.
			postOrder[writeIdx] = block
			writeIdx--
			continue
		}

		// Mark it as expanded
		expanded[block] = true

		// Push block back into the stack
		stack = append(stack, block)

		// Push all he block successors into the stack
		for _, succ := range block.Succs {
			if !visited[succ] {
				visited[succ] = true
				stack = append(stack, succ)
			}
		}
	}

	return postOrder, nil
}
