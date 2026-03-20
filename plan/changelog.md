# Changelog

## 2026-03-20 — Nil Pointer Analysis: Full Phase 2

### Interprocedural + Global Tracking + Message Quality
- Interprocedural nil summaries: analyze callee return values via CHA call graph, cache summaries to avoid recomputation
- Sentinel caching in lookupOrComputeSummary to break recursive call chains (A calls B calls A)
- Global variable nil tracking: nil checks on globals propagate to subsequent reads of the same global
- Skip global address loads in checkInstruction (the address itself is always valid, only the value can be nil)
- initBlockState: predecessor state flows across blocks, fixing `if p == nil { return }; *p` pattern
- Method receivers assumed DefinitelyNotNil (intentional pragmatic unsoundness)
- nilValueName: human-readable names (parameters, globals, call results), empty for SSA registers
- Actionable messages: "possible nil dereference of config — add a nil check before use"
- Per-function deduplication: same variable reported once per function
- Soft error handling in loader: type errors warn instead of failing (real codebases have transitive soft errors)
- Nil package guard in Prover.Prove (packages with load errors produce nil SSA packages)
- Real-world validation: production Go codebase went from 32 warnings to 1

### Intraprocedural Foundation

- Implemented NilState abstract domain with Join, Meet, Equals — 4-element lattice (NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil)
- Built NilAnalyzer with per-block state tracking, same worklist pattern as interval analyzer
- Transfer functions: Alloc, MakeInterface, MakeSlice, MakeMap, MakeChan → DefinitelyNotNil; FieldAddr, IndexAddr → DefinitelyNotNil (post-dereference); Phi → Join of predecessor edges
- Branch refinement: `if p != nil` / `if p == nil` narrows state in true/false branches, handles nil on either side of comparison
- Check pass: flags *ssa.UnOp(MUL), *ssa.FieldAddr, *ssa.IndexAddr on DefinitelyNil (Bug) or MaybeNil (Warning)
- Slice IndexAddr: only flags DefinitelyNil to reduce noise — MaybeNil deferred to bounds checker (Phase 3)
- Wired NilAnalyzer into CLI alongside interval analyzer — combined findings sorted by severity
- Fixed FieldAddr double-reporting: FieldAddr/IndexAddr results tracked as DefinitelyNotNil so subsequent loads don't re-flag
- lookupNilState, isNillable, isSliceType helpers with full test coverage
- 100% test coverage on nil_analyzer.go — 60+ test cases including synthetic, table-driven, and real SSA tests
- Verified against testdata: DerefParam (Warning), DerefAfterCheck (safe), DerefNew (safe), DerefNilLiteral (Bug), FieldAccessOnNil (Bug)

## 2026-03-03 — Integer Overflow Detection (Task 1.5 + 1.6)

- Implemented `IntervalForType` in `bounds.go`: maps `types.BasicKind` to type bounds for int8, int16, int32
- Implemented `Contains` method on `Interval` for checking if one interval is fully contained in another
- Implemented `flagOverflow`: checks BinOp result intervals against type bounds post-convergence
- Implemented `checkConvertOp`: detects narrowing overflow in type conversions (e.g. int16→int8)
- Implemented `checkUnOp`: detects negation overflow (e.g. `-(-128)` = 128 overflows int8)
- Extracted `checkOverflow` shared helper for Bug/Warning/Safe classification with context-aware messages
- Fixed param initialization: entry block params now start at type bounds via `IntervalForType` instead of Top
- Three-way classification: Bug (result entirely outside bounds), Warning (partial overlap), Safe (contained)
- Distinct messages per check: "integer overflow", "integer overflow in conversion", "integer overflow in negation"
- int64, int, uint types deliberately untracked (internal representation is int64, so overflow is undetectable)
- 20 bounds tests, 47 Contains tests, 30 overflow tests, 16 param bounds tests, 32 conversion tests, 30 negation tests
- Total: 175+ new test cases, all passing

## 2026-03-02 — Worklist Algorithm + Widening

- Implemented worklist algorithm with change detection (copyBlockState, stateEqual using maps.Copy/maps.Equal)
- Implemented Widen operator on Interval: detects bound growth, jumps to MinInt64/MaxInt64
- Identified loop headers via RPO index comparison (back-edge detection)
- Applied widening at loop headers after joining all predecessor states
- Separated finding collection into a final check pass on converged state (no duplicates)
- Added maxIterations safety cap (1000) for pre-narrowing termination guarantee
- Fixed critical Phi bug: transferPhi now looks up edge values from predecessor blocks
- Fixed lookupInterval: unvisited blocks return Bottom (identity for Join) instead of Top
- 48 test cases total (30 base + 5 loop + 13 advanced loop patterns), 100% coverage
- Test patterns include: nested loops, while-style loops, break, step-by-2, guarded division, mod in loops, complex CFGs

## 2026-03-01 — Branch Refinement + Per-Block State

- Added comparison operators (<, <=, >, >=) to refineFromCondition using Meet
- Refactored refineFromCondition into refineFromEquality + refineFromComparison
- Discovered and fixed soundness bug: sibling branches corrupting shared state
- Changed state from flat map[Value]Interval to per-block map[*BasicBlock]map[Value]Interval
- Added initBlockState: copies predecessor state (Join for multiple predecessors)
- ExcludeZero tests for Join, Meet, ContainsZero propagation
- Achieved 100% test coverage across all files (30+ analyzer test cases)

## 2026-02-28 — Interval Domain + Basic Analyzer

- Implemented Interval domain: NewInterval, Top, Bottom, Join, Meet, Equals
- Implemented interval arithmetic: Add, Sub, Mul, Div with checkSpecial helper
- Added ExcludeZero flag to handle != 0 branch refinement for division safety
- Built Analyzer with RPO walk, transferBinOp, transferPhi, flagDivisionByZero
- lookupInterval handles *ssa.Const, state map, defaults to Top
- Basic branch refinement for == and != operators

## 2026-02-27 — CFG Walker + Package Merge

- Implemented non-recursive RPO walker using explicit stack DFS (NASA P10 Rule 1)
- Merged cfg and domain packages into single analysis package
- Tests for linear, diamond, loop, nested loop CFG shapes
- buildSSA test helper for constructing SSA from source strings

## 2026-02-26 — Project Inception

- Designed the overall architecture and phase roadmap through conversation
- Studied existing tools: Gobra (ETH Zurich), NilAway (Uber), coq-of-go (Formal Land)
- Decided on abstract interpretation approach (not deductive verification)
- Created CLAUDE.md and plan/ directory structure
- Ready to begin Phase 0
