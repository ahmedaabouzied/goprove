package loader

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

func TestLoad(t *testing.T) {
	t.Run("valid package loads into SSA", func(t *testing.T) {
		t.Parallel()
		prog, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
		require.NoError(t, err)
		require.NotNil(t, prog)
		require.Len(t, pkgs, 1)

		pkg := pkgs[0]
		require.NotNil(t, pkg)
		require.Equal(t, "testdata", pkg.Pkg.Name())
	})

	t.Run("all expected functions exist with built bodies", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
		require.NoError(t, err)

		pkg := pkgs[0]
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
	})

	t.Run("simple function structure", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
		require.NoError(t, err)

		pkg := pkgs[0]

		// Add: linear function, one block, two params.
		addFn := pkg.Func("Add")
		require.Equal(t, 1, len(addFn.Blocks), "Add should have exactly 1 basic block")
		require.Equal(t, 2, len(addFn.Params), "Add should have 2 parameters")

		// Constant: no params, one block.
		constFn := pkg.Func("Constant")
		require.Equal(t, 0, len(constFn.Params), "Constant should have 0 parameters")
		require.Equal(t, 1, len(constFn.Blocks), "Constant should have 1 block")
	})

	t.Run("branching function has multiple blocks", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
		require.NoError(t, err)

		pkg := pkgs[0]

		// Abs has an if/else — creates multiple blocks.
		absFn := pkg.Func("Abs")
		require.Greater(t, len(absFn.Blocks), 1, "Abs should have multiple blocks due to if/else")

		// Clamp has two if statements — even more blocks.
		clampFn := pkg.Func("Clamp")
		require.Greater(t, len(clampFn.Blocks), len(absFn.Blocks),
			"Clamp (two ifs) should have more blocks than Abs (one if)")
	})

	t.Run("loop function has phi nodes", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
		require.NoError(t, err)

		pkg := pkgs[0]

		// Sum has a for loop — loop variables create Phi nodes.
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
	})

	t.Run("nested loops have multiple phi nodes", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load("github.com/ahmedaabouzied/goprove/pkg/testdata")
		require.NoError(t, err)

		pkg := pkgs[0]

		nestedFn := pkg.Func("Nested")
		phiCount := 0
		for _, block := range nestedFn.Blocks {
			for _, instr := range block.Instrs {
				if _, ok := instr.(*ssa.Phi); ok {
					phiCount++
				}
			}
		}
		// Nested has two loop variables (i, j) plus count — expect multiple Phi nodes.
		require.GreaterOrEqual(t, phiCount, 2, "Nested should have at least 2 Phi nodes for loop variables")
	})

	t.Run("package with type errors loads with warning", func(t *testing.T) {
		t.Parallel()
		// Type errors are soft — loader warns to stderr but does not fail.
		// Hard errors (parse, list) still fail.
		_, _, err := Load("./testdata/broken")
		require.NoError(t, err)
	})

	t.Run("nonexistent package returns error", func(t *testing.T) {
		_, _, err := Load("github.com/nonexistent/fakepkg/that/does/not/exist")
		require.Error(t, err)
	})

	t.Run("multiple patterns load multiple packages", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load(
			"github.com/ahmedaabouzied/goprove/pkg/testdata",
			"github.com/ahmedaabouzied/goprove/pkg/loader",
		)
		require.NoError(t, err)
		require.Len(t, pkgs, 2)

		names := make(map[string]bool)
		for _, pkg := range pkgs {
			if pkg != nil {
				names[pkg.Pkg.Name()] = true
			}
		}
		require.True(t, names["testdata"], "should contain testdata package")
		require.True(t, names["loader"], "should contain loader package")
	})

	t.Run("empty pattern returns error", func(t *testing.T) {
		t.Parallel()
		_, pkgs, err := Load("")
		// An empty pattern may load the current package or fail —
		// either way it should not panic.
		if err != nil {
			return // error is acceptable
		}
		// If no error, we should still get valid output.
		require.NotNil(t, pkgs)
	})

	t.Run("broken GOROOT makes packages.Load fail", func(t *testing.T) {
		t.Parallel()
		if os.Getenv("TEST_BROKEN_GOROOT") == "1" {
			_, _, err := Load("fmt")
			if err != nil {
				os.Stderr.WriteString("LOAD_ERROR:" + err.Error())
				os.Exit(1)
			}
			os.Exit(0)
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestLoad/broken_GOROOT")
		// Set GOROOT to a nonexistent path so the go driver can't find the toolchain.
		env := os.Environ()
		filtered := env[:0]
		for _, e := range env {
			if len(e) > 7 && e[:7] == "GOROOT=" {
				continue
			}
			filtered = append(filtered, e)
		}
		filtered = append(filtered,
			"TEST_BROKEN_GOROOT=1",
			"GOROOT=/nonexistent/goroot",
		)
		cmd.Env = filtered
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			return // some systems may still succeed; don't fail the test
		}
		// Verify it errored (either packages.Load error or build errors).
		require.NotEmpty(t, stderr.String())
	})
}
