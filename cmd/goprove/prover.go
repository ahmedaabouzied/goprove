package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/ahmedaabouzied/goprove/pkg/loader"
	"golang.org/x/tools/go/ssa"
)

// provePackage is the main entry point for our prover.
func provePackage(path string) error {
	_, pkgs, err := loader.Load(path)
	if err != nil {
		return err
	}
	if len(pkgs) < 1 {
		return fmt.Errorf("no packages found at %s", path)
	}

	for _, pkg := range pkgs {
		if err := printPkg(pkg); err != nil {
			return err
		}
	}
	return nil
}

func printPkg(pkg *ssa.Package) error {
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		if err := printFunction(fn); err != nil {
			return err
		}
	}
	return nil
}

func printFunction(fn *ssa.Function) error {
	// Signature
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprintf(w, "func %s:\n", fn.Name())

	for i, block := range fn.Blocks {
		// Predecessor/Successor indices
		predIdxs := make([]string, len(block.Preds))
		for j, p := range block.Preds {
			predIdxs[j] = fmt.Sprintf("%d", p.Index)
		}
		succIdxs := make([]string, len(block.Succs))
		for j, s := range block.Succs {
			succIdxs[j] = fmt.Sprintf("%d", s.Index)
		}

		comment := ""
		if i == 0 {
			comment = " (entry)"
		}
		fmt.Fprintf(w, "  Block %d:%s\n", block.Index, comment)

		fmt.Fprintf(w, "    Succs: [%s]  Preds: [%s]\n",
			strings.Join(succIdxs, ", "),
			strings.Join(predIdxs, ", "))

		for _, instr := range block.Instrs {
			instrType := fmt.Sprintf("%T", instr)
			// Strip the "*ssa." prefix
			instrType = strings.TrimPrefix(instrType, "*ssa.")

			// If the instruction produces a value, show the assignment
			if val, ok := instr.(ssa.Value); ok {
				fmt.Fprintf(w, "      %-30s (%s)\n",
					fmt.Sprintf("%s = %s", val.Name(), instr),
					instrType)
			} else {
				fmt.Fprintf(w, "      %-30s (%s)\n", instr, instrType)
			}
		}
	}
	return w.Flush()
}
