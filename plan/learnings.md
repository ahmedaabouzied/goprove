# Learnings

Things learned while building GoProve. Updated as we go.

---

## SSA Concepts

- `block.String()` only returns the block index. To see instructions, iterate `block.Instrs`.
- SSA adds init blocks to packages, so `len(fn.Blocks)` may be larger than expected.
- `*ssa.Const` values have a `Value` field of type `go/constant.Value`. Use `c.Value.Kind()` to check the type before calling `c.Int64()`.
- External functions (declared without body, e.g. assembly-backed) have `fn.Blocks == nil`.
- In SSA, `-x` is a `*ssa.UnOp`, not a `*ssa.BinOp`. `x & 1 == 1` becomes two BinOps — the If condition is always the comparison, not the bitwise op.
- `*ssa.Parameter` values are the function's input parameters. They're the starting point for interval analysis (initialized as Top).
- `*ssa.Phi` nodes appear at join points (merge of two branches). Each has `Edges` corresponding to the block's `Preds`.

## Abstract Interpretation Concepts

- **Reverse Post-Order (RPO)**: The iteration order for forward dataflow analysis. Guarantees that (for acyclic CFGs) every predecessor is processed before its successors. For loops, back-edges break this guarantee — hence the need for a worklist with iteration.
- **Top**: Unknown — the value could be anything. This is the starting state for parameters.
- **Bottom**: Unreachable — no value flows here. Used as the identity for Join.
- **Join (union)**: Merges states from different paths. Used at Phi nodes and block merge points. `Bottom.Join(x) = x`, `Top.Join(x) = Top`.
- **Meet (intersection)**: Narrows a state based on a constraint. Used for branch refinement. `Top.Meet(x) = x`, `Bottom.Meet(x) = Bottom`.
- **Transfer function**: Computes the output abstract state from the input abstract state for a single instruction. E.g., `[1,5].Add([2,3]) = [3,8]`.
- **Branch refinement**: When processing a successor of an `if` block, narrow the variable's interval based on the condition. E.g., `if x > 0` true branch: meet with `[1, MaxInt64]`.
- **ExcludeZero**: A targeted extension to handle `!= 0` checks. Can't be represented as a single interval (Top minus {0} is not an interval). The flag is checked in `ContainsZero`.
- **Per-block state**: Each block must have its own state map. A single global state causes sibling branches to corrupt each other. This is the standard approach in dataflow analysis frameworks.
- **Non-relational limitation**: Interval analysis can't prove `x - x = 0` because `Top.Sub(Top) = Top`. Relational domains (octagons, polyhedra) could, but are not on the current roadmap.

## Go Tooling Internals

- `types.Config{Importer: nil}` doesn't support importing packages. Tests that need `import "unsafe"` or other packages will fail with "Config.Importer not installed".
- `types.Info` must have initialized maps (`Types`, `Defs`, `Uses`) and be passed to both `conf.Check()` and `CreatePackage`.
- `prog.Build()` must be called after `CreatePackage`, not before.
- Go's type checker rejects `x / 0` (literal zero divisor) at compile time. The SSA is never built for it. Runtime division by zero requires an intermediate variable: `zero := 0; x / zero`.
- Go's built-in `min`/`max` functions (added in Go 1.21) work on int64 — no need for custom helpers.

## Defensive Coding

- Value receivers on methods that modify struct state silently discard changes. Always use pointer receivers on Analyzer methods.
- Mixed value/pointer receivers on the same type is a Go code smell.
- The `go/types` package uses `isComparison(op)` helper functions to categorize token.Token operators — this is idiomatic Go for operator dispatch.
