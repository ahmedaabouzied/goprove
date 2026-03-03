package analysis_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/ahmedaabouzied/goprove/pkg/analysis"
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
					blocks, err := analysis.ReversePostOrder(fn)
					require.NoError(t, err)
					assertBlocks(t, fn.Blocks, blocks)
				}
			}
		})
	}
}

func TestReversePostOrderUnreachableBlocks(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		src    string
		fnName string
	}{
		"dead code after return": {
			// The block containing y := 2; return y is unreachable.
			src: `
				package example

				func deadAfterReturn(x int) int {
					return x
					y := 2
					return y
				}
			`,
			fnName: "deadAfterReturn",
		},
		"dead else with constant condition": {
			// The compiler may create unreachable blocks for constant conditions.
			src: `
				package example

				func deadElse() int {
					if true {
						return 1
					}
					return 2
				}
			`,
			fnName: "deadElse",
		},
		"dead code after panic": {
			// Code after panic is unreachable.
			src: `
				package example

				func deadAfterPanic(x int) int {
					if x < 0 {
						panic("negative")
					}
					return x
				}
			`,
			fnName: "deadAfterPanic",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)

			for _, member := range ssaPkg.Members {
				fn, ok := member.(*ssa.Function)
				if !ok {
					continue
				}
				if fn.Name() == tt.fnName {
					blocks, err := analysis.ReversePostOrder(fn)
					require.NoError(t, err)

					// All returned blocks must be non-nil
					for i, b := range blocks {
						require.NotNil(t, b, "block at index %d is nil", i)
					}

					// Returned count <= total blocks (some may be unreachable)
					require.LessOrEqual(t, len(blocks), len(fn.Blocks),
						"RPO should not return more blocks than exist")

					// No duplicates
					seen := make(map[int]bool)
					for _, b := range blocks {
						require.False(t, seen[b.Index], "duplicate block %d", b.Index)
						seen[b.Index] = true
					}

					// Entry block must be first
					require.Equal(t, fn.Blocks[0], blocks[0], "entry block must be first")
				}
			}
		})
	}
}

func TestReversePostOrderNoNilBlocks(t *testing.T) {
	t.Parallel()

	// Specifically tests that unreachable blocks don't produce nil entries.
	// This was the bug that crashed goprove on google/uuid.
	src := `
		package example

		func switchWithDefault(x int) int {
			switch {
			case x > 0:
				return 1
			case x < 0:
				return -1
			default:
				return 0
			}
		}
	`
	ssaPkg := buildSSA(t, src)
	fn := findFunctionInPkg(ssaPkg, "switchWithDefault")
	require.NotNil(t, fn)

	blocks, err := analysis.ReversePostOrder(fn)
	require.NoError(t, err)

	for i, b := range blocks {
		require.NotNilf(t, b, "block at RPO index %d is nil (total blocks: %d, reachable: %d)",
			i, len(fn.Blocks), len(blocks))
	}
}

func TestReversePostOrderAnalyzeWithUnreachable(t *testing.T) {
	t.Parallel()

	// End-to-end: ensure the analyzer doesn't crash on functions with unreachable blocks.
	tests := map[string]struct {
		src    string
		fnName string
	}{
		"dead code after return": {
			src: `
				package example

				func f(x int) int {
					return x
					y := 2
					return y
				}
			`,
			fnName: "f",
		},
		"switch all return": {
			src: `
				package example

				func g(x int) int {
					switch {
					case x > 0:
						return 1
					case x < 0:
						return -1
					default:
						return 0
					}
				}
			`,
			fnName: "g",
		},
		"early panic": {
			src: `
				package example

				func h(x int) int {
					if x == 0 {
						panic("zero")
					}
					return 10 / x
				}
			`,
			fnName: "h",
		},
		"multiple returns": {
			src: `
				package example

				func m(x int8) int8 {
					if x > 100 {
						return x
					}
					if x < -100 {
						return -x
					}
					return x + 1
				}
			`,
			fnName: "m",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ssaPkg := buildSSA(t, tt.src)
			fn := findFunctionInPkg(ssaPkg, tt.fnName)
			require.NotNil(t, fn, "function %s not found", tt.fnName)

			// Must not panic
			analyzer := &analysis.Analyzer{}
			_ = analyzer.Analyze(fn)
		})
	}
}

func findFunctionInPkg(pkg *ssa.Package, name string) *ssa.Function {
	for _, member := range pkg.Members {
		fn, ok := member.(*ssa.Function)
		if ok && fn.Name() == name {
			return fn
		}
	}
	return nil
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

	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := conf.Check("example", fset, []*ast.File{file}, info)
	require.NoError(t, err)

	prog := ssa.NewProgram(fset, ssa.SanityCheckFunctions)

	// Recursively create SSA packages for all imports so prog.Build()
	// can resolve them. Without this, importing "fmt" or "math" panics.
	var createDeps func(p *types.Package)
	createDeps = func(p *types.Package) {
		if prog.Package(p) != nil {
			return
		}
		prog.CreatePackage(p, nil, nil, true)
		for _, imp := range p.Imports() {
			createDeps(imp)
		}
	}
	for _, imp := range pkg.Imports() {
		createDeps(imp)
	}

	ssaPkg := prog.CreatePackage(pkg, []*ast.File{file}, info, false)
	prog.Build()

	return ssaPkg
}
