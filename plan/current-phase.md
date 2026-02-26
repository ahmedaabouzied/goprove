# Current Phase: Phase 0 — Foundation

**Status**: Not started
**Branch**: `phase/0-foundation`
**Goal**: Load Go packages into SSA form, traverse the IR, understand every instruction type.

## Why This Phase Matters

Everything else builds on SSA. If you don't deeply understand how Go's SSA represents programs — how Phi nodes work, how the CFG connects blocks, how branch conditions map to successors — the interval analysis in Phase 1 will be a nightmare. Take your time here.

## Tasks

### 0.1 Project Setup
- [ ] Initialize go module: `go mod init github.com/ahmedakef/goprove`
- [ ] Create directory structure (cmd/, pkg/, testdata/, plan/)
- [ ] Add CLAUDE.md to root
- [ ] First commit

### 0.2 Package Loader
- [ ] Write `pkg/loader/loader.go` that takes a package path and returns SSA program
- [ ] Use `golang.org/x/tools/go/packages` with `LoadAllSyntax` mode
- [ ] Use `golang.org/x/tools/go/ssa/ssautil.AllPackages()` to build SSA
- [ ] Handle errors gracefully (package not found, syntax errors, etc.)
- [ ] Write tests for loader (load a known package, verify functions exist)

### 0.3 SSA Explorer CLI
- [ ] Write `cmd/goprove/main.go` as entry point
- [ ] Accept a package path as argument
- [ ] Print every function in the package
- [ ] For each function, print every basic block
- [ ] For each block, print every instruction with its type
- [ ] Print the CFG structure (block → successor blocks)

### 0.4 Understand SSA Instruction Types
Study and document (in learnings.md) how these SSA instructions work:
- [ ] `*ssa.BinOp` — binary operations (add, sub, mul, div, comparisons)
- [ ] `*ssa.UnOp` — unary operations (negation, not, dereference, arrow)
- [ ] `*ssa.Phi` — phi nodes (join points where values from different paths merge)
- [ ] `*ssa.If` — conditional branch (has exactly 2 successor blocks)
- [ ] `*ssa.Jump` — unconditional jump (has exactly 1 successor block)
- [ ] `*ssa.Return` — function return
- [ ] `*ssa.Call` — function/method call
- [ ] `*ssa.Alloc` — memory allocation (stack or heap)
- [ ] `*ssa.FieldAddr` / `*ssa.Field` — struct field access
- [ ] `*ssa.IndexAddr` / `*ssa.Index` — array/slice indexing
- [ ] `*ssa.Slice` — slice operation
- [ ] `*ssa.MakeSlice` / `*ssa.MakeMap` / `*ssa.MakeChan`
- [ ] `*ssa.Store` / `*ssa.Load` — memory operations (in SSA these may appear as UnOp)
- [ ] `*ssa.Convert` — type conversion
- [ ] `*ssa.ChangeType` / `*ssa.ChangeInterface`
- [ ] `*ssa.Extract` — extract element from tuple (used for multi-return functions)
- [ ] `*ssa.Const` — constant values
- [ ] `*ssa.Parameter` — function parameters

### 0.5 CFG Walker
- [ ] Write a function that visits all blocks in a function in **reverse post-order**
  (this is the order you need for forward dataflow analysis)
- [ ] Verify it handles loops correctly (a block can be its own predecessor)
- [ ] Write tests: create test Go functions with known CFG shapes (linear, if/else, loop, nested)

### 0.6 Test Fixtures
Create `pkg/testdata/` with small Go files that will be used throughout the project:
- [ ] `simple.go` — basic arithmetic, no control flow
- [ ] `branches.go` — if/else with various conditions
- [ ] `loops.go` — for loops, range loops
- [ ] `divzero.go` — functions with possible and impossible division by zero
- [ ] `overflow.go` — functions with possible integer overflow
- [ ] `nilderef.go` — functions with possible nil dereference (for later phases)
- [ ] `slices.go` — functions with slice indexing (for later phases)

## Definition of Done

Phase 0 is complete when:
1. `goprove ./path/to/package` prints clean SSA output for any valid Go package
2. Ahmed can look at any SSA instruction and explain what it does
3. The CFG walker correctly traverses any Go function's basic blocks
4. Test fixtures exist and the loader can process all of them
5. `plan/learnings.md` documents key SSA concepts with examples

## Estimated Time: 1-2 weeks
