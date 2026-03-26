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
- **Widening**: Forces fixed-point convergence at loop headers by jumping to extremes when bounds grow. `[1,1].Widen([1,2]) = [1, MaxInt64]`. Applied after joining all predecessors, before transfer.
- **Loop header detection**: A block is a loop header if any predecessor has a higher RPO index (back-edge). Computed via `rpoIndex` map.
- **Unvisited predecessors return Bottom**: When a Phi node looks up an edge from an unvisited block, returning Top poisons everything. Bottom is correct — it's "no info yet" and is the identity for Join. This was the key insight for making Phi nodes work across back-edges.
- **Phi edges come from predecessor blocks**: `transferPhi` must look up `v.Edges[i]` from `block.Preds[i]`'s state, not the current block's state. Otherwise back-edge values are lost.
- **Separate check pass**: Findings must be collected after the worklist converges, not during iteration. Otherwise re-processed blocks produce duplicates, and intermediate (unsound) states generate stale findings.

## Go Tooling Internals

- `types.Config{Importer: nil}` doesn't support importing packages. Tests that need `import "unsafe"` or other packages will fail with "Config.Importer not installed".
- `types.Info` must have initialized maps (`Types`, `Defs`, `Uses`) and be passed to both `conf.Check()` and `CreatePackage`.
- `prog.Build()` must be called after `CreatePackage`, not before.
- Go's type checker rejects `x / 0` (literal zero divisor) at compile time. The SSA is never built for it. Runtime division by zero requires an intermediate variable: `zero := 0; x / zero`.
- Go's built-in `min`/`max` functions (added in Go 1.21) work on int64 — no need for custom helpers.

## Nil Analysis Concepts

- **NilState lattice**: 4-element flat lattice. NilBottom (unreachable) < DefinitelyNil / DefinitelyNotNil < MaybeNil (unknown/Top). No widening needed — finite height guarantees convergence.
- **isNillable types**: Pointer, Interface, Slice, Map, Chan, Signature. Non-nillable types (int, bool, struct, array) always return DefinitelyNotNil — no need to track them.
- **`ssa.Const.IsNil()`**: Returns true for nil constants of nillable types. Zero-value consts of non-nillable types (e.g., `struct{}{}`) have `Value == nil` but `IsNil() == false`. Always use `IsNil()`, never check `Value == nil` directly.
- **MakeInterface of nil pointer**: `interface((*T)(nil))` produces a non-nil interface. The interface value is non-nil even though the underlying pointer is nil. This is a classic Go gotcha — the nil analyzer correctly marks MakeInterface as DefinitelyNotNil.
- **FieldAddr/IndexAddr post-dereference**: If `p.Field` or `p[i]` didn't panic, the resulting pointer is non-nil. Recording these as DefinitelyNotNil prevents double-reporting (the base dereference is already flagged).
- **SSA optimizes away `make([]T, constant)`**: When slice length is a constant, SSA may inline the allocation and not produce an `*ssa.MakeSlice` instruction. Use parameter-based lengths in test fixtures to force MakeSlice emission.
- **Synthetic `ssa.Parameter{}` has no type**: Cannot use bare `&ssa.Parameter{}` in tests that call `isNillable` — it panics on `v.Type().Underlying()`. Use `ssa.NewConst(...)` with proper types for synthetic tests.

## Seed Analysis Learnings

- **`pkg.Members` only contains top-level named functions.** Methods, closures, and init functions are NOT in `pkg.Members`. To find methods, iterate `pkg.Members` for `*ssa.Type` and get their method sets. For closures, recurse into `fn.AnonFuncs`. This was the #1 source of false positives — `collectCallSites` and `allFunctions` both missed half the program.
- **`*ssa.Extract` is how Go multi-return works in SSA.** `f, err := os.Open(path)` becomes `Call` → `Extract #0` (f) → `Extract #1` (err). If `transferInstruction` doesn't handle Extract, the extracted values have no state and default to MaybeNil. This is the #2 source of false positives.
- **Go's ok-pattern depends on Extract + branch refinement working together.** `v, ok := m[key]; if ok { v.Use() }` requires: (1) Lookup produces tuple, (2) Extract #0 gets value, Extract #1 gets bool, (3) branch refinement on the bool narrows the value's state. Missing any step breaks the chain.
- **Fixed-size arrays are value types in Go.** `[256]byte` cannot be nil. Indexing one is not a pointer dereference. `isSliceType` distinguishes slices but not arrays from pointers-to-arrays.
- **FP rate is the adoption gate.** 23 real bugs in 20 popular libraries is genuinely useful. But at 98.8% FP rate, no one will use the tool. Signal-to-noise ratio matters more than raw detection capability.

## Defensive Coding

- Value receivers on methods that modify struct state silently discard changes. Always use pointer receivers on Analyzer methods.
- Mixed value/pointer receivers on the same type is a Go code smell.
- The `go/types` package uses `isComparison(op)` helper functions to categorize token.Token operators — this is idiomatic Go for operator dispatch.
