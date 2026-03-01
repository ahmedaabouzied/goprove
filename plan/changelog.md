# Changelog

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
