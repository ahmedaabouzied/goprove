package loader

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

func TestLoad(t *testing.T) {
	prog, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
	require.NoError(t, err)
	require.NotNil(t, prog)
	require.Len(t, pkgs, 1)

	pkg := pkgs[0]
	require.NotNil(t, pkg)
	require.Equal(t, "testdata", pkg.Pkg.Name())

	// Functions from each testdata file should exist and have built bodies.
	// A built function has blocks; blocks have instructions.
	expectedFuncs := []string{
		// simple.go
		"Add", "Multiply", "Constant", "LocalVar",
		// branches.go
		"Abs", "Clamp", "Max", "Sign",
		// loops.go
		"Sum", "Countdown", "SumSlice", "Nested",
		// divzero.go
		"DivByZeroLiteral", "DivByParam", "DivSafe", "DivByConstant",
		"ModByZero", "DivAfterComputation", "DivInLoop",
		// overflow.go
		"OverflowAdd", "OverflowMul", "SafeAdd", "UnderflowSub",
		"ShiftOverflow", "SafeSmallArithmetic",
		// nilderef.go
		"DerefParam", "DerefAfterCheck", "DerefNew", "DerefNilLiteral",
		"FieldAccessOnNil", "MethodCallOnParam", "MapLookupOk",
		// slices.go
		"IndexDirect", "IndexAfterCheck", "IndexConstant", "RangeLoop",
		"IndexOutOfBounds", "SliceOp", "AppendAndIndex",
	}

	for _, name := range expectedFuncs {
		fn := pkg.Func(name)
		require.NotNilf(t, fn, "function %s not found in SSA package", name)
		require.NotEmptyf(t, fn.Blocks, "function %s has no basic blocks (Build not called?)", name)
		require.NotEmptyf(t, fn.Blocks[0].Instrs, "function %s block 0 has no instructions", name)
	}

	// Spot-check structural properties of specific functions.

	// Add is simple: one block, a BinOp, a Return.
	addFn := pkg.Func("Add")
	require.Equal(t, 1, len(addFn.Blocks), "Add should have exactly 1 basic block")
	require.Equal(t, 2, len(addFn.Params), "Add should have 2 parameters")

	// Abs has a branch: should have more than 1 block (if/else creates multiple blocks).
	absFn := pkg.Func("Abs")
	require.Greater(t, len(absFn.Blocks), 1, "Abs should have multiple blocks due to if/else")

	// Sum has a loop: should have multiple blocks and at least one Phi node
	// (the loop variable merges values from the init and the back-edge).
	sumFn := pkg.Func("Sum")
	require.Greater(t, len(sumFn.Blocks), 1, "Sum should have multiple blocks due to loop")
	foundPhi := false
	for _, block := range sumFn.Blocks {
		for _, instr := range block.Instrs {
			if _, ok := instr.(*ssa.Phi); ok {
				foundPhi = true
				break
			}
		}
	}
	require.True(t, foundPhi, "Sum should have at least one Phi node for loop variables")
}
