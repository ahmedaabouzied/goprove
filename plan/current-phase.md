# Current Phase: Phase 1 — Integer Interval Analysis

**Status**: In progress
**Branch**: `main`
**Goal**: Track integer ranges through the program and flag division by zero + overflow.

## Completed Tasks

### 0.5 CFG Walker (from Phase 0)
- [x] Non-recursive RPO walker using explicit stack DFS (NASA P10 compliant)
- [x] Tests for linear, diamond, loop, nested loop CFG shapes
- [x] 100% coverage

### 1.1 Interval Domain
- [x] Define the Interval abstract domain (Lo, Hi, IsTop, IsBottom)
- [x] Implement lattice operations: Join (union), Meet (intersection), Equals
- [x] Implement ContainsZero
- [x] Implement ExcludeZero flag for != 0 refinement
- [x] Implement abstract arithmetic: Add, Sub, Mul, Div, Neg
- [x] Implement Contains (interval containment check)
- [x] checkSpecial helper for Bottom/Top propagation in arithmetic
- [x] Comprehensive tests with 100% coverage

### 1.2 Basic Analyzer
- [x] Analyzer struct with per-block state: map[*BasicBlock]map[Value]Interval
- [x] Finding struct with Pos, Message, Severity (Safe/Warning/Bug)
- [x] Walk blocks in RPO, transfer instructions per block
- [x] transferBinOp: handle ADD, SUB, MUL, QUO, REM
- [x] transferPhi: Join all edges starting from Bottom
- [x] transferUnOp: handle negation via Neg()
- [x] transferConvert: propagate source interval through type conversions
- [x] flagDivisionByZero: distinguish Bug ([0,0]) from Warning (contains zero)
- [x] lookupInterval: handle *ssa.Const, state map, default Top

### 1.3 Branch Refinement
- [x] refineFromPredecessor: check if predecessor ends with *ssa.If
- [x] refineFromCondition: dispatch to equality vs comparison
- [x] refineFromEquality: handle == and != with ExcludeZero
- [x] refineFromComparison: handle <, <=, >, >= using Meet with constraint intervals
- [x] Per-block state (initBlockState) to prevent sibling branch corruption
- [x] 100% test coverage (30+ test cases including synthetic BinOp)

### 1.4 Worklist Algorithm + Widening
- [x] Implement worklist with change detection + successor re-queuing
- [x] Implement copyBlockState and stateEqual helpers (using maps.Copy/maps.Equal)
- [x] Implement Widen operator on Interval (jump to MinInt64/MaxInt64 on bound growth)
- [x] Detect loop headers via RPO index (back-edge = pred has higher RPO index)
- [x] Apply widening at loop headers after joining predecessor states
- [x] Separate check pass: findings collected only on converged state (no duplicates)
- [x] Max iterations safety cap (1000)
- [x] Fixed lookupInterval: unvisited blocks return Bottom (not Top) for Phi correctness
- [x] Fixed transferPhi: look up edge values from predecessor blocks (not current block)
- [x] 48 test cases passing, 100% coverage
- [ ] Implement narrowing (optional, to improve precision after widening)

### 1.5 Integer Overflow Detection
- [x] IntervalForType: maps types.BasicKind to type bounds (int8, int16, int32)
- [x] Contains method on Interval for checking if result fits type bounds
- [x] flagOverflow: checks BinOp results against type bounds (Bug/Warning/Safe)
- [x] checkConvertOp: detects narrowing overflow in type conversions (int16→int8, etc.)
- [x] checkUnOp: detects negation overflow (-MinInt8 = 128 > MaxInt8)
- [x] checkOverflow: shared helper for overflow classification (proven/possible)
- [x] Param initialization uses IntervalForType (params start at type bounds, not Top)
- [x] Three-way classification: Bug (disjoint via Meet.IsBottom), Warning (partial overlap), Safe (Contains)
- [x] Distinct messages: "integer overflow", "integer overflow in conversion", "integer overflow in negation"
- [x] int64/int/uint types deliberately untracked (can't detect overflow with int64 internals)
- [x] 30 overflow tests, 32 conversion tests, 30 negation tests — all passing

### 1.6 Handle Type Conversions and UnOps
- [x] transferConvert: propagate source interval through *ssa.Convert
- [x] transferUnOp: handle *ssa.UnOp negation via Neg()
- [x] Overflow detection wired into checkInstruction for Convert and UnOp

### 1.7 CLI Integration + Output
- [x] Wire analyzer into cmd/goprove/main.go via provePackage → analyzePkg → analyzeFunction
- [x] Colored terminal output: red (Bug), yellow (Warning), no output for Safe
- [x] Findings sorted by severity (Bugs first), then by source position
- [x] Relative file paths in output
- [x] Test against testdata fixtures (divzero.go, overflow.go, branches.go, loops.go, simple.go)
- [x] 20+ CLI tests: printFinding, analyzeFunction, analyzePkg sort order, provePackage integration
- [ ] Summary line: N proven bugs, N warnings (nice-to-have)

## Phase 1 Status: COMPLETE

All definition-of-done criteria met:
1. ✅ `goprove ./...` detects division by zero with proof
2. ✅ Integer overflow is flagged (arithmetic, conversion, negation)
3. ✅ Loops are handled correctly (widening guarantees termination)
4. ✅ Branch conditions narrow intervals
5. ✅ Output clearly shows Bug / Warning with colored terminal output

Optional improvements (not blocking):
- [ ] Summary line at the end of output
- [ ] Narrowing pass to improve precision after widening
- [ ] Unsigned integer overflow tracking

---

## Phase 1.8: Call Graph Integration

**Status**: In progress
**Goal**: Build a reusable call resolution layer using CHA call graphs. This unblocks interface dispatch for interval analysis and provides the foundation for nil analysis (Phase 2).

### Why now (before Phase 2)

The current `transferCall` uses `StaticCallee()` only — interface method calls return nil and fall back to `Top()`. Nil analysis will need the same call resolution for tracking nil state across function boundaries. Building this as a shared abstraction now avoids duplicating the work in Phase 2.

### Current limitations being addressed

1. No interface dispatch — `StaticCallee()` returns nil for interface calls, losing all precision
2. `FunctionSummary` is interval-specific (`[]Interval`) — can't reuse for nil analysis
3. CHA call graph is built (`BuildCallGraph`) but never used by the analyzer
4. `lookupOrComputeSummary` uses indirect recursion (Analyze → transferCall → lookupOrComputeSummary → child.Analyze) — design debt per NASA P10 Rule 1

### Tasks

- [ ] 1.8.1 Define `CallResolver` interface — `Resolve(call *ssa.Call) []*ssa.Function`
- [ ] 1.8.2 Implement `CHAResolver` wrapping `*callgraph.Graph` — try `StaticCallee()` first, fall back to CHA edge lookup
- [ ] 1.8.3 Add `resolver CallResolver` field to `Analyzer` struct
- [ ] 1.8.4 Refactor `transferCall` to use the resolver instead of `StaticCallee()` directly
- [ ] 1.8.5 Handle multi-callee case (interface dispatch) — Join return summaries from all possible callees
- [ ] 1.8.6 Tests: static call resolution, interface call resolution, unknown call fallback to Top
- [ ] 1.8.7 Consider whether `FunctionSummary` needs to be generic or if domain-specific summaries with a shared resolver is better (decision needed)

### Design decisions to make

- **Generic `AbstractValue` interface vs domain-specific summaries**: A generic `AbstractValue` interface (`Join`, `IsTop`, `IsBottom`, `Equals`) would let `FunctionSummary` work for any domain. But it loses type safety and requires runtime assertions. Alternative: keep `IntervalSummary` and `NilSummary` separate, sharing only the `CallResolver`. Decide before implementing 1.8.7.

### Definition of done

1. Interface method calls resolve to concrete implementations via CHA
2. `transferCall` uses the resolver, not `StaticCallee()` directly
3. Multi-callee results are Joined soundly
4. Tests cover static, interface, and unknown call scenarios
5. `printCallgraph` is removed or replaced with something useful
