package cfg

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/tools/go/ssa"
)

func ReversePostOrder(fn *ssa.Function) ([]*ssa.BasicBlock, error) {
	blocks := make([]*ssa.BasicBlock, len(fn.Blocks))
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprintln(w, fn.Name())
	for i, block := range fn.Blocks {
		blocks[i] = block
		for _, instr := range block.Instrs {
			fmt.Fprintln(w, instr)
		}
	}
	if err := w.Flush(); err != nil {
		return nil, err
	}
	return blocks, nil
}
