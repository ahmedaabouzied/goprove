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

### 2.14 Whole-Program Parameter Analysis
- [x] collectCallSites: walk all packages, collect call sites from *ssa.Call and *ssa.Go
- [x] classifyArg: context-free nil classification of SSA values
- [x] ComputeParamNilStatesAnalysis: fixed-point iteration using converged caller state
- [x] Block-level argument lookup: at each call site, look up argument nil state from caller's converged block state
- [x] Sentinel summary caching to break recursive call chains
- [x] Cross-package parameter tracking: analyze all functions with call sites in scope, not just unexported
- [x] convergedStates: NilAnalyzer saves per-function state after Analyze for param analysis lookup
- [x] SetParamNilStates: post-construction param state update for iterative refinement

## Phase 2 Status: COMPLETE

All definition-of-done criteria met:
1. ✅ `goprove ./...` detects nil dereferences (proven and possible)
2. ✅ `if p != nil { *p }` is proven safe (branch refinement)
3. ✅ `if p == nil { return }; *p` is proven safe (state propagation)
4. ✅ `new(T)`, `&x`, `make(...)` are proven non-nil
5. ✅ `var p *int; *p` is a proven Bug
6. ✅ No double-reporting on FieldAddr/IndexAddr chains
7. ✅ Interprocedural: callee return nil states tracked via function summaries
8. ✅ Whole-program: parameter nil states computed from converged caller state via fixed-point iteration
9. ✅ Cross-package: parameter tracking works across package boundaries within analyzed scope
10. ✅ Global variables: nil checks propagate across subsequent reads
11. ✅ Method receivers: assumed non-nil (intentional pragmatic unsoundness)
12. ✅ Real-world validation: 32 → 0 warnings on production logger package, 352 → 47 on attribution worker
13. ✅ Tested on net/http (0 false bugs), core/kafka, core/platform (0 warnings)
14. ✅ 60+ integration tests, 100% unit test coverage on nil_analyzer.go

## Known Limitations (to fix later)

### Store/Load tracking (address-taken locals)
When a local variable has its address taken (`&x`), SSA uses Store/UnOp(MUL) pairs instead of direct value flow. The nil analyzer does not track nil state through these pairs. This means:
```go
var p *int
p = new(int)   // SSA: Store to p's alloc
_ = *p         // SSA: UnOp MUL on p's alloc — state is MaybeNil, not DefinitelyNotNil
```
For most Go code, variables are not address-taken and SSA uses direct values + Phi nodes, so this rarely causes false positives. Fix requires tracking Store destinations and propagating nil state from stored values.

### Resolved (previously listed as limitations)
- [x] Map lookup ok pattern (`v, ok := m[key]`) — works correctly via SSA branch refinement
- [x] Type assertion ok pattern (`v, ok := x.(T)`) — works correctly via SSA branch refinement
- [x] Stdlib return values (`time.NewTimer`, `bytes.NewBuffer`) — resolved by interprocedural analysis into stdlib

---

## Fix Plan: Noise Reduction and Edge Cases

Priority order based on real-world impact (tested on go-redis v9.7.3 and production codebases).

### Fix 1: Interval Analyzer Finding Deduplication (HIGH — blocks usability)
**Problem**: The interval analyzer reports the same finding once per CHA callee. A single `int(x)` conversion produces 100+ duplicate "possible integer overflow" warnings when the function is callable via many interface paths.
**Impact**: Makes the tool unusable on any codebase with interface dispatch. go-redis has 500+ duplicate findings from 3 unique overflow sites.
**Fix**: Add deduplication to the interval analyzer's check pass, same pattern as nil analyzer — deduplicate by `(file, line, message)` before appending to findings.
**Effort**: Small — add `reported` map to `Analyzer.Analyze`, filter in `checkInstruction`.

### Fix 2: unsafe.Pointer Conversion False Positive (MEDIUM)
**Problem**: `*(*string)(unsafe.Pointer(&b))` produces a nil warning. `unsafe.Pointer(&b)` is always non-nil (address-of local), but SSA represents the conversion as a value that could be nil.
**Impact**: 2 false positives in go-redis `internal/util/unsafe.go`. Any package using unsafe pointer casts will hit this.
**Fix**: In `checkInstruction`, when checking `*ssa.UnOp{MUL}`, if `v.X` was produced by `*ssa.Convert` (unsafe pointer cast), skip the check. Alternatively, handle `*ssa.Convert` in `transferInstruction` to propagate the source value's nil state.
**Effort**: Small — add `case *ssa.Convert` to `transferInstruction`.

### Fix 3: Interface Invoke Nil Check (MEDIUM — correctness gap)
**Problem**: `s.Method()` on a nil interface is not flagged. The nil analyzer only checks `*ssa.UnOp{MUL}`, `*ssa.FieldAddr`, and `*ssa.IndexAddr`. Interface method invocations use `*ssa.Call` with `IsInvoke() == true`.
**Impact**: Missing real nil dereference warnings on interface values. `MethodCallOnParam(s fmt.Stringer)` produces no warning.
**Fix**: In `checkInstruction`, add a case for `*ssa.Call` where `v.Call.IsInvoke()` — check the receiver's nil state.
**Effort**: Small — add invoke check to `checkInstruction`.

### Fix 4: Field Access After Nil Check on Reloaded Value (LOW — rare in practice)
**Problem**: In SSA, `if x.Field != nil { x.Field.Use() }` produces two separate loads from `x.Field`. The nil check refines the first load but the second load is a new SSA value. This is the same pattern as the global variable issue, but for struct fields.
**Impact**: Seen in go-redis `search_commands.go` with `options.WithCursorOptions`. Uncommon pattern.
**Fix**: Extend the global refinement approach — when refining a value that is a load from a FieldAddr, also store the refinement keyed by the FieldAddr base+index. Then in `lookupNilState`, check if the value is a load from a refined field.
**Effort**: Medium — requires tracking FieldAddr sources similar to Global sources.

### Fix 5: Tests for Param Analysis (HIGH — correctness confidence)
**Problem**: `ComputeParamNilStatesAnalysis`, `collectCallSites`, and `classifyArg` have zero test coverage. These are the newest and most complex pieces.
**Impact**: Risk of regressions in the core whole-program analysis.
**Fix**: Write integration tests: single caller non-nil, single caller nil, multiple callers mixed, exported function no callers, recursive calls, goroutine calls.
**Effort**: Medium — need to build multi-function SSA packages in tests.

---

## Previous Phases

### Phase 1: Integer Interval Analysis — COMPLETE
### Phase 1.8: Call Graph Integration — COMPLETE
