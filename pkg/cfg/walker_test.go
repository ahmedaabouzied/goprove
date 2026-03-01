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

func TestReversePostOrder(t *testing.T) {
	srcs := map[string]string{
		// Multiply represents a case for a function with one block.
		"multiply": `
		package example

		func multiply(a int, b int) int {
			return a * b	
		}
	`,

		// Devide represents a case for a function with branches
		"devide": `
			package example

			func devide(a int, b int) int {
				if b == 0 {
					panic("devide by 0")	
				}
				return a / b
			}
	`,
		// Sum slice represents a case for loops
		"SumSlice": `
		package example

		func SumSlice(s []int) int {
			total := 0
			for _, v := range s {
				total += v
			}
			return total
		}
		`,
		"Nested": `
		package example
			func Nested(n int) int {
				count := 0
				for i := 0; i < n; i++ {
					for j := 0; j < n; j++ {
						count++
					}
				}
				return count
			}
		`,
	}

	for fnName, src := range srcs {
		t.Run(fnName, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, src)

			for _, member := range ssaPkg.Members {
				fn, ok := member.(*ssa.Function)
				if !ok {
					continue
				}

				if fn.Name() == fnName {
					blocks, err := ReversePostOrder(fn)
					require.NoError(t, err)
					assertBlocks(t, fn.Blocks, blocks)
				}
			}
		})
	}
}

func assertBlocks(t *testing.T, fnBlocks []*ssa.BasicBlock, actual []*ssa.BasicBlock) {
	// All blocks present
	require.Equal(t, len(fnBlocks), len(actual))

	// Entry block is first
	require.Equal(t, fnBlocks[0], actual[0])

	// No duplicates
	seen := make(map[int]bool)
	for _, b := range actual {
		require.False(t, seen[b.Index], "duplicate block %d", b.Index)
		seen[b.Index] = true
	}

	// Predecessor ordering: every block appears after all its
	// non-back-edge predecessors (pred with a lower RPO position)
	pos := make(map[int]int) // block index → position in RPO
	for i, b := range actual {
		pos[b.Index] = i
	}
	for i, b := range actual {
		for _, pred := range b.Preds {
			// If pred comes after b in RPO, it's a back-edge — skip
			if pos[pred.Index] > i {
				continue
			}
			require.Less(t, pos[pred.Index], i,
				"block %d should come after predecessor %d", b.Index, pred.Index)
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
