# Current Phase: Phase 2 — Nil Pointer Analysis

**Status**: Core complete, intraprocedural
**Branch**: `main`
**Goal**: Prove absence of nil dereferences within function boundaries.

## Completed Tasks

### 2.1 NilState Abstract Domain
- [x] Define NilState: NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil
- [x] Implement Join (lattice union) with lookup table
- [x] Implement Meet (lattice intersection) with lookup table
- [x] Implement Equals
- [x] Comprehensive tests with 100% coverage

### 2.2 NilAnalyzer Foundation
- [x] NilAnalyzer struct with per-block state: map[*BasicBlock]map[Value]NilState
- [x] lookupNilState: handle *ssa.Const (IsNil), non-nillable types, state map, default MaybeNil
- [x] isNillable: classify types via Underlying() — Pointer, Interface, Slice, Map, Chan, Signature
- [x] Comprehensive tests including named types, real SSA params, precedence rules

### 2.3 Transfer Functions
- [x] transferInstruction switch dispatching to type-specific handlers
- [x] "Always non-nil" producers: Alloc, MakeInterface, MakeSlice, MakeMap, MakeChan
- [x] FieldAddr and IndexAddr results: DefinitelyNotNil (if execution continues past them)
- [x] transferPhi: Join nil states from all predecessor edges (starting from NilBottom)
- [x] Table-driven tests for all producers, Phi join table, synthetic + real SSA tests
- [x] 100% coverage on transferInstruction and transferPhi

### 2.4 Branch Refinement
- [x] refineFromPredecessor: check if predecessor ends with *ssa.If
- [x] refineFromCondition: identify nil constant on either side (X or Y)
- [x] Handle == nil and != nil with correct state for true/false branches
- [x] Handle nil on left side (nil == p) and right side (p == nil)
- [x] Early return for non-nil comparisons and unsupported operators
- [x] 100% coverage — 8 individual tests + 8-entry table-driven + 5 no-op tests + 5 real SSA tests + 4 edge case tests

### 2.5 Worklist Algorithm
- [x] Analyze method: RPO block ordering, worklist with change detection
- [x] Block state initialization on first visit
- [x] copyBlockState and stateEqual helpers (using maps.Copy/maps.Equal)
- [x] Max iterations safety cap (1000)
- [x] No widening needed — NilState lattice has finite height (4 elements)

### 2.6 Check Pass + Finding Emission
- [x] checkInstruction: flag dereferences after worklist convergence
- [x] Handle *ssa.UnOp (token.MUL — pointer deref *p)
- [x] Handle *ssa.FieldAddr (struct field access p.Field)
- [x] Handle *ssa.IndexAddr (array/slice index p[i])
- [x] flagNilDeref: DefinitelyNil → Bug, MaybeNil → Warning
- [x] Slice IndexAddr: only flag DefinitelyNil (Bug), skip MaybeNil (deferred to bounds checker)
- [x] isSliceType helper for slice-specific handling

### 2.7 CLI Integration
- [x] NilAnalyzer wired into Prover alongside interval analyzer
- [x] Renamed analyzer field to intervalAnalyzer for clarity
- [x] Combined findings from both analyzers, sorted by severity then position
- [x] Verified against testdata: correct Bug/Warning/Safe classification

### 2.8 Precision Fixes
- [x] FieldAddr/IndexAddr results tracked as DefinitelyNotNil — eliminates double-reporting
- [x] Slice IndexAddr warnings suppressed (MaybeNil too noisy, deferred to Phase 3)

## Phase 2 Intraprocedural Status: COMPLETE

All definition-of-done criteria met for intraprocedural analysis:
1. ✅ `goprove ./...` detects nil dereferences (proven and possible)
2. ✅ `if p != nil { *p }` is proven safe (branch refinement)
3. ✅ `new(T)`, `&x`, `make(...)` are proven non-nil
4. ✅ `var p *int; *p` is a proven Bug
5. ✅ No double-reporting on FieldAddr/IndexAddr chains
6. ✅ 100% test coverage on nil_analyzer.go

## Known Limitations (to fix later)

### Store/Load tracking (address-taken locals)
When a local variable has its address taken (`&x`), SSA uses Store/UnOp(MUL) pairs instead of direct value flow. The nil analyzer does not track nil state through these pairs. This means:
```go
var p *int
p = new(int)   // SSA: Store to p's alloc
_ = *p         // SSA: UnOp MUL on p's alloc — state is MaybeNil, not DefinitelyNotNil
```
For most Go code, variables are not address-taken and SSA uses direct values + Phi nodes, so this rarely causes false positives. Fix requires tracking Store destinations and propagating nil state from stored values.

### No interprocedural nil analysis
Function parameters default to MaybeNil. If a caller always passes non-nil, the callee still warns. Phase 4 (interprocedural analysis) will add function summaries for nil state, similar to interval summaries.

### No map lookup tracking
`v, ok := m[key]` — the `ok` pattern is not tracked. `v` defaults to MaybeNil even when guarded by `ok`. Requires tracking Extract instructions from Tuple results.

### No type assertion tracking
`v, ok := x.(T)` — similar to map lookup, the ok pattern is not tracked.

---

## Previous Phases

### Phase 1: Integer Interval Analysis — COMPLETE
### Phase 1.8: Call Graph Integration — COMPLETE
