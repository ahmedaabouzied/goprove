# Architecture Decisions

## ADR-001: Use SSA as the primary IR (not AST)

**Date**: 2026-02-26
**Status**: Accepted

**Context**: Go provides both AST (`go/ast`) and SSA (`golang.org/x/tools/go/ssa`) representations. We need to choose which to analyze.

**Decision**: Use SSA form as the primary intermediate representation.

**Rationale**:
- SSA assigns each variable exactly once → reaching definitions are implicit → dataflow analysis is dramatically simpler
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
- ✅ **GREEN** — proven safe for all inputs
- ❌ **RED** — proven bug exists
- ⚠️ **ORANGE** — could not prove safe or unsafe

**Rationale**: Matches Polyspace's model, which is well-understood in the static analysis world. The orange category is honest about the limits of the analysis — better than either false confidence or excessive noise.
