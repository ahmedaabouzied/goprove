# Architecture Decisions

## ADR-001: Use SSA as the primary IR (not AST)

**Date**: 2026-02-26
**Status**: Accepted

**Context**: Go provides both AST (`go/ast`) and SSA (`golang.org/x/tools/go/ssa`) representations. We need to choose which to analyze.

**Decision**: Use SSA form as the primary intermediate representation.

**Rationale**:
- SSA assigns each variable exactly once -> reaching definitions are implicit -> dataflow analysis is dramatically simpler
- The CFG is explicit in SSA (basic blocks with edges)
- Phi nodes make join points explicit — essential for interval merging
- Constants are propagated, dead code is eliminated
- Only ~30 instruction types to handle (vs hundreds of AST node types)
- NilAway and other serious Go analyzers use SSA

**Trade-off**: SSA is lower-level than AST, so error messages need extra work to map back to source positions. But every SSA instruction carries a `token.Pos` for this.

---

## ADR-002: Build on go/analysis framework

**Date**: 2026-02-26
**Status**: Accepted for later phases (Phase 7), standalone CLI first

**Context**: The `go/analysis` framework provides a plugin architecture for analyzers that integrates with `go vet` and `golangci-lint`.

**Decision**: Build as a standalone CLI first, port to `go/analysis` in Phase 7.

**Rationale**:
- `go/analysis` imposes constraints (modular analysis, fact exports) that add complexity early on
- A standalone CLI is easier to develop and debug
- We can always wrap the analysis in an `analysis.Analyzer` later
- NilAway started standalone and then integrated with `go/analysis`

---

## ADR-003: Zero external dependencies for core analysis

**Date**: 2026-02-26
**Status**: Accepted

**Context**: We could use external libraries for abstract domains, constraint solving, etc.

**Decision**: No external dependencies for core analysis. Only `golang.org/x/tools` beyond stdlib.

**Rationale**:
- We're building this to learn — implementing the domains ourselves is the point
- Fewer dependencies = easier to understand, audit, and maintain
- Go's stdlib + x/tools provides everything we need for SSA loading

---

## ADR-004: Interval domain as first abstract domain

**Date**: 2026-02-26
**Status**: Accepted

**Context**: Multiple abstract domains exist (intervals, octagons, polyhedra, symbolic). We need to pick a starting point.

**Decision**: Start with intervals `[lo, hi]` per variable.

**Rationale**:
- Simplest useful domain — can prove div-by-zero and overflow
- Well-understood theory, straightforward implementation
- Fast (O(1) per operation)
- Can be extended to relational domains (octagons, polyhedra) later
- The limitation (can't express relationships between variables like `i < len(s)`) is acceptable for Phase 1

---

## ADR-005: Three-color output model

**Date**: 2026-02-26
**Status**: Accepted

**Decision**: Findings are classified as:
- **GREEN** (Safe) — proven safe for all inputs
- **RED** (Bug) — proven bug exists
- **ORANGE** (Warning) — could not prove safe or unsafe

**Rationale**: Matches Polyspace's model, which is well-understood in the static analysis world. The orange category is honest about the limits of the analysis — better than either false confidence or excessive noise.

---

## ADR-006: ExcludeZero flag instead of interval unions

**Date**: 2026-03-01
**Status**: Accepted

**Context**: `if y != 0 { x / y }` — after the != 0 check, y's interval is still Top (all integers). We can't represent "all except 0" with a single [lo, hi] interval. Options: (1) ExcludeZero flag, (2) interval unions, (3) exclude list.

**Decision**: Add an `excludeZero bool` field to the Interval struct.

**Rationale**:
- Division by zero is the only case where excluding a single value matters
- Interval unions add significant complexity (every operation must handle sets of intervals)
- An exclude list is overengineering — no other excluded values are needed
- The flag is checked in ContainsZero, propagated through Join (both must exclude) and Meet (either excludes), not propagated through arithmetic
- Simple, targeted, correct

---

## ADR-007: Per-block state map

**Date**: 2026-03-01
**Status**: Accepted

**Context**: With a single flat `map[Value]Interval`, sibling branches corrupt each other. Block 1 (true branch) writes its refinement, then Block 2 (false branch) reads the corrupted state.

**Decision**: Change state to `map[*BasicBlock]map[Value]Interval`. Each block has its own state. `initBlockState` copies/joins predecessor states before refinement.

**Rationale**:
- This is the textbook approach for dataflow analysis
- Required anyway for the worklist algorithm with widening (Phase 1.4)
- Each block starts with the Join of its predecessors' exit states
- Refinement writes only to the current block's state
- Eliminates the sibling branch corruption bug

---

## ADR-008: Non-recursive algorithms (NASA P10 Rule 1)

**Date**: 2026-02-27
**Status**: Accepted

**Context**: RPO walker could use recursive DFS or explicit stack.

**Decision**: Use explicit stack-based DFS for all graph algorithms.

**Rationale**:
- NASA P10 Rule 1 prohibits recursion (call graph must be acyclic)
- Explicit stack avoids stack overflow on deep CFGs
- Same time complexity, slightly more code but fully controllable

---

## ADR-009: Only track signed int8/int16/int32 for overflow

**Date**: 2026-03-03
**Status**: Accepted

**Context**: The interval domain uses int64 internally. We need to decide which types to check for overflow.

**Decision**: Only check overflow for int8, int16, and int32. Return `false` for int64, int, and all unsigned types.

**Rationale**:
- int64 overflow is undetectable when our internal representation is int64 (the computation itself would overflow)
- int is platform-dependent (32 or 64 bit) and typically 64-bit — same problem
- Unsigned integers have different overflow semantics (wrapping is defined behavior in Go) and require separate treatment
- int8/int16/int32 are the types where silent overflow is most dangerous and most detectable

---

## ADR-010: Separate overflow messages per instruction kind

**Date**: 2026-03-03
**Status**: Accepted

**Context**: Overflow can happen in BinOp (arithmetic), Convert (narrowing), and UnOp (negation). Should they share a message?

**Decision**: Use distinct messages with a context suffix: "integer overflow", "integer overflow in conversion", "integer overflow in negation".

**Rationale**:
- Users need to know *what kind* of overflow to fix it
- Arithmetic overflow (fix: widen the type or add bounds checks) differs from conversion overflow (fix: check before casting) differs from negation overflow (fix: guard against MinInt)
- Shared `checkOverflow` helper takes a context string, keeping code DRY while messages are specific
