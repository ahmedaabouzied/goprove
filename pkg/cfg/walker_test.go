package cfg

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
)

func TestReversePostOrder_sum(t *testing.T) {
	src := `
		package example

		func sum(a int, b int) int {
			return a + b	
		}
	`

	ssaPkg := buildSSA(t, src)

	for _, member := range ssaPkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}
		if fn.Name() == "sum" {
			require.Equal(t, 1, len(fn.Blocks)) // Our function has just one block. No loop, no branches.
			blocks, err := ReversePostOrder(fn)
			require.NoError(t, err)
			require.Equal(t, blocks[0], fn.Blocks[0]) // The entry block is the first and only block.
		}
	}
}

func TestReversePostOrder_multiply(t *testing.T) {
	src := `
		package example

		func multiply(a int, b int) int {
			return a * b	
		}
	`

	ssaPkg := buildSSA(t, src)

	for _, member := range ssaPkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}

		if fn.Name() == "multiply" {
			require.Equal(t, 1, len(fn.Blocks)) // Our function has just one block. No loop, no branches.
			blocks, err := ReversePostOrder(fn)
			require.NoError(t, err)
			require.Equal(t, blocks[0], fn.Blocks[0]) // The entry block is the first and only block.
		}
	}
}

func TestReversePostOrder_devide(t *testing.T) {
	src := `
		package example

		func devide(a int, b int) int {
			if b == 0 {
				panic("devide by 0")	
			}
			return a / b
		}
	`

	ssaPkg := buildSSA(t, src)

	for _, member := range ssaPkg.Members {
		fn, ok := member.(*ssa.Function)
		if !ok {
			continue
		}

		if fn.Name() == "devide" {
			require.Equal(t, 3, len(fn.Blocks)) // Our function has just one block. No loop, no branches.
			blocks, err := ReversePostOrder(fn)
			require.NoError(t, err)
			require.Equal(t, blocks[0], fn.Blocks[0]) // The entry block is the first and only block.
		}
	}
}

func buildSSA(t *testing.T, src string) *ssa.Package {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "anything.go", src, 0)
	require.NoError(t, err)

	conf := types.Config{Importer: nil}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := conf.Check("example", fset, []*ast.File{file}, info)
	require.NoError(t, err)

	prog := ssa.NewProgram(fset, ssa.SanityCheckFunctions)

	ssaPkg := prog.CreatePackage(pkg, []*ast.File{file}, info, false)
	prog.Build()

	return ssaPkg
}
