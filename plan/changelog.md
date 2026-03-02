# Changelog

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
