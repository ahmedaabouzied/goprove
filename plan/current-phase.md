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
- [x] Implement abstract arithmetic: Add, Sub, Mul, Div
- [x] checkSpecial helper for Bottom/Top propagation in arithmetic
- [x] Comprehensive tests with 100% coverage

### 1.2 Basic Analyzer
- [x] Analyzer struct with per-block state: map[*BasicBlock]map[Value]Interval
- [x] Finding struct with Pos, Message, Severity (Safe/Warning/Bug)
- [x] Walk blocks in RPO, transfer instructions per block
- [x] transferBinOp: handle ADD, SUB, MUL, QUO, REM
- [x] transferPhi: Join all edges starting from Bottom
- [x] flagDivisionByZero: distinguish Bug ([0,0]) from Warning (contains zero)
- [x] lookupInterval: handle *ssa.Const, state map, default Top

### 1.3 Branch Refinement
- [x] refineFromPredecessor: check if predecessor ends with *ssa.If
- [x] refineFromCondition: dispatch to equality vs comparison
- [x] refineFromEquality: handle == and != with ExcludeZero
- [x] refineFromComparison: handle <, <=, >, >= using Meet with constraint intervals
- [x] Per-block state (initBlockState) to prevent sibling branch corruption
- [x] 100% test coverage (30+ test cases including synthetic BinOp)

## Current Task

### 1.4 Worklist Algorithm + Widening
- [ ] Implement worklist (iteration to fixed point for loops)
- [ ] Implement widening operator on Interval (to guarantee termination)
- [ ] Implement narrowing (optional, to improve precision after widening)
- [ ] Detect loop back-edges and apply widening there
- [ ] Test on programs with loops (for, range)

## Remaining Tasks

### 1.5 Integer Overflow Detection
- [ ] Detect when result interval exceeds type bounds (int8, int16, int32, int64)
- [ ] Track type information per SSA value
- [ ] Flag overflow as Warning or Bug

### 1.6 Handle Type Conversions
- [ ] Handle *ssa.Convert (e.g. int32 → int)
- [ ] Handle *ssa.UnOp (negation)
- [ ] Propagate intervals through conversions

### 1.7 CLI Integration + Output
- [ ] Wire analyzer into cmd/goprove/main.go
- [ ] Produce colored terminal output (green/orange/red)
- [ ] Test against suite of known-good and known-bad Go programs

## Definition of Done

Phase 1 is complete when:
1. `goprove ./...` detects division by zero with proof
2. Integer overflow is flagged
3. Loops are handled correctly (widening guarantees termination)
4. Branch conditions narrow intervals
5. Output clearly shows Bug / Warning / Safe
