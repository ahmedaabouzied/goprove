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

---

## ADR-011: CallResolver interface for call graph integration

**Date**: 2026-03-08
**Status**: Proposed

**Context**: The analyzer uses `StaticCallee()` to resolve calls, which returns nil for interface method calls. We have a CHA call graph (`BuildCallGraph`) but it's unused. Phase 2 (nil analysis) will also need call resolution. We need a shared abstraction.

**Decision**: Define a `CallResolver` interface with a single method `Resolve(*ssa.Call) []*ssa.Function`. Implement `CHAResolver` wrapping the CHA graph. The analyzer receives a resolver at construction time.

**Rationale**:
- Decouples call resolution strategy from the analysis engine
- CHA today, RTA/VTA later — swap the resolver, analysis code doesn't change
- Both interval and nil analysis share the same resolver (no duplication)
- Interface calls resolved to multiple callees → Join all return summaries (sound overapproximation)

**Trade-off**: Adds one level of indirection. Worth it for reusability and testability (can inject a mock resolver in tests).

---

## ADR-012: Domain-specific summaries over generic AbstractValue

**Date**: 2026-03-08
**Status**: Proposed

**Context**: `FunctionSummary` currently uses `[]Interval`. For nil analysis we'll need `[]NilState`. Options: (1) generic `AbstractValue` interface, (2) separate summary types per domain.

**Decision**: Keep domain-specific summaries (`IntervalSummary`, `NilSummary`). Share only the `CallResolver`.

**Rationale**:
- Type safety — no runtime type assertions needed
- Simpler — each domain knows exactly what it's working with
- The shared part (call resolution) is the `CallResolver`, not the summary
- Can always generalize later if real duplication emerges
- YAGNI — don't abstract until the pattern repeats

---

## ADR-013: No widening for nil analysis

**Date**: 2026-03-20
**Status**: Accepted

**Context**: The interval analyzer uses widening to guarantee convergence on loops. Does the nil analyzer need widening too?

**Decision**: No widening for nil analysis. The NilState lattice has finite height (4 elements: NilBottom < DefinitelyNil/DefinitelyNotNil < MaybeNil), so the worklist is guaranteed to converge without widening.

**Rationale**:
- Every value can only change at most 3 times before reaching the top (MaybeNil)
- With N values and B blocks, worst case is O(3 × N × B) state changes — always terminates
- The maxIterations cap (1000) is a safety net, not a convergence mechanism

---

## ADR-014: Slice IndexAddr — Bug only, no MaybeNil warning

**Date**: 2026-03-20
**Status**: Accepted

**Context**: Every slice parameter triggers "possible nil dereference" on IndexAddr because slice params default to MaybeNil. This is noisy — indexing a nil slice panics with "index out of range", which is the bounds checker's domain.

**Decision**: For slice-typed IndexAddr bases, only flag DefinitelyNil (Bug). Skip MaybeNil (Warning). Non-slice IndexAddr (pointer-to-array) keeps both Bug and Warning.

**Rationale**:
- Nil slice indexing is a bounds violation, not a nil pointer dereference
- Phase 3 (slice bounds analysis) will track slice length and flag nil slice access properly
- Reduces false positives significantly on real Go code (slice params are common)
- DefinitelyNil slice indexing is still flagged — this is always a bug

---

## ADR-015: FieldAddr/IndexAddr results are DefinitelyNotNil

**Date**: 2026-03-20
**Status**: Accepted

**Context**: `s.X` in SSA becomes `FieldAddr s .X` followed by `UnOp MUL` (load). The FieldAddr correctly flags nil deref on `s`, but the UnOp sees the FieldAddr result as MaybeNil and emits a spurious second warning.

**Decision**: Transfer functions for FieldAddr and IndexAddr record their result as DefinitelyNotNil in the state map.

**Rationale**:
- If FieldAddr/IndexAddr didn't panic, execution continues — the resulting pointer is valid (non-nil)
- This models "post-dereference" knowledge: the program only reaches the next instruction if the base was non-nil
- Eliminates double-reporting on `s.X` patterns (Bug on FieldAddr + spurious Warning on load)
- Sound: the result pointer points to a sub-element of a live object
