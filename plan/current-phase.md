# Current Phase: Phase 2 — Nil Pointer Analysis

**Status**: Complete (intraprocedural + interprocedural returns)
**Branch**: `main`
**Goal**: Detect nil dereferences — proven bugs and possible warnings.

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

### 2.9 State Propagation
- [x] initBlockState: copy/join predecessor states so refinements flow across blocks
- [x] `if p == nil { return }; *p` pattern now correctly proves p non-nil after the guard

### 2.10 Method Receiver Skip
- [x] Method receivers (fn.Signature.Recv) initialized as DefinitelyNotNil
- [x] Eliminates false positives on all method body dereferences

### 2.11 Interprocedural Nil Summaries
- [x] NilFunctionSummary with return nil states
- [x] transferCall: resolve callees via CHA, compute/cache summaries, Join return states
- [x] lookupOrComputeSummary with call depth cap (3) and sentinel caching to break recursion
- [x] computeReturnNilStates: walk Return instructions, Join nil states across all paths
- [x] resolveCallees: CHA resolver with StaticCallee fallback

### 2.12 Global Variable Nil Tracking
- [x] refineFromCondition stores refinement keyed by global address
- [x] lookupNilState checks global address state for loads from refined globals
- [x] Skip global address loads in checkInstruction (address itself is always valid)

### 2.13 Message Quality + Deduplication
- [x] nilValueName: human-readable names for parameters, globals, nil constants, call results
- [x] Empty string for SSA register names (t0, t1) — omitted from messages
- [x] Actionable fix tips in messages ("add a nil check before use")
- [x] Per-function deduplication: same variable reported once, not per dereference site
- [x] Safe Phi edge traversal (no recursion into Phi edges to avoid cycles)

## Phase 2 Status: COMPLETE

All definition-of-done criteria met:
1. ✅ `goprove ./...` detects nil dereferences (proven and possible)
2. ✅ `if p != nil { *p }` is proven safe (branch refinement)
3. ✅ `if p == nil { return }; *p` is proven safe (state propagation)
4. ✅ `new(T)`, `&x`, `make(...)` are proven non-nil
5. ✅ `var p *int; *p` is a proven Bug
6. ✅ No double-reporting on FieldAddr/IndexAddr chains
7. ✅ Interprocedural: callee return nil states tracked via function summaries
8. ✅ Global variables: nil checks propagate across subsequent reads
9. ✅ Method receivers: assumed non-nil (intentional pragmatic unsoundness)
10. ✅ Real-world validation: 32 → 1 warning on a production Go codebase
11. ✅ 40+ integration tests, 100% unit test coverage on nil_analyzer.go

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
