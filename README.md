# GoProve

A static analysis tool for Go that uses abstract interpretation to detect bugs and prove safety properties in Go programs.

## What It Does

GoProve analyzes Go source code at compile time and classifies each potential error site:

- **Bug** — proven to fail at runtime. The analysis guarantees this code will crash.
- **Warning** — could not prove safe or unsafe. Worth a human look.
- **Safe** (no output) — the analysis proved this operation is safe for the patterns it tracks.

```
$ goprove ./...
 Error: pkg/billing/charge.go:47 nil dereference of nil pointer — value is always nil
 Error: pkg/billing/charge.go:52 division by zero
 Warning: pkg/handler/click.go:89 possible nil dereference of config — add a nil check before use
```

## Currently Detects

### Nil Pointer Dereference
- **Proven bug**: dereferencing a value that is always nil (`var p *int; *p`)
- **Warning**: dereferencing a value that may be nil (unchecked parameter, conditional assignment)
- **Safe**: nil checks are tracked through branches (`if p != nil { *p }`)
- Tracks: `new()`, `&x`, `make()`, `MakeInterface` as always non-nil
- Interprocedural return tracking: analyzes callee return values to determine nil state
- Whole-program parameter tracking: fixed-point iteration analyzes what callers actually pass — if all callers pass non-nil after nil guards, the parameter is proven non-nil
- Cross-package analysis: parameter tracking works across package boundaries within the analyzed scope
- Global variables: nil checks on globals propagate across subsequent reads
- Method receivers: assumed non-nil (calling a method on nil is the caller's responsibility)
- Deduplication: same variable reported once per function, not per dereference site

### Division by Zero
- **Proven bug**: divisor is always zero
- **Warning**: divisor range includes zero but isn't guaranteed
- **Safe**: divisor range provably excludes zero (e.g., after `if y != 0` guard)

### Integer Overflow
- **Arithmetic overflow**: result of `+`, `-`, `*`, `/` exceeds type bounds (int8, int16, int32)
- **Conversion overflow**: narrowing cast loses data (e.g., int16 → int8 when value > 127)
- **Negation overflow**: `-x` overflows when x is the minimum value (e.g., `-(-128)` for int8)
- Branch guards are respected: `if x < 100 { int8(x) }` is proven safe

## Soundness

GoProve aims for **soundness** — when it says Bug, it's a real bug. When it says Safe, it means safe *for the patterns the tool tracks*. However, there are important caveats:

**What "proven" means:**
- Bug findings are sound: if GoProve says "nil dereference — value is always nil", abstract interpretation has proven this across all execution paths.
- Interval findings are sound: widening guarantees convergence, and the final check pass operates on converged (stable) abstract state.

**What the tool does NOT cover (known incompleteness):**
- Store/Load tracking: nil state is not tracked through pointer stores (`*p = x`) and loads (`y = *p`). Variables that are address-taken may produce false warnings.
- Interface method invocations: `s.Method()` on a possibly-nil interface is not flagged (only pointer dereferences, field access, and index operations are checked).
- Map `ok` pattern: `v, ok := m[key]` — the `ok` guard is not tracked.
- Type assertion `ok` pattern: `v, ok := x.(T)` — same limitation.
- Concurrency: no tracking of values across goroutines.

**Intentional pragmatic unsoundness:**
- Method receivers are assumed non-nil. This eliminates noise on real code but means GoProve won't catch `(*T)(nil).Method()`.
- Slice `IndexAddr` only flags proven nil (Bug), not possible nil (Warning). Nil slice indexing is deferred to the bounds checker.

In short: **Bug = guaranteed real. Warning = worth checking. No finding = safe for what we track, but we don't track everything.**

## How It Works

1. Builds SSA (Static Single Assignment) form via `golang.org/x/tools/go/ssa`
2. Runs abstract interpretation with two domains:
   - **Interval domain** `[lo, hi]` per integer variable — detects division by zero and overflow
   - **Nil state domain** `{Bottom, DefinitelyNil, DefinitelyNotNil, MaybeNil}` per pointer variable — detects nil dereferences
3. Worklist algorithm iterates blocks to a fixed point (widening for intervals, finite lattice for nil)
4. Branch refinement narrows state through `if` conditions (`if p != nil`, `if y != 0`, comparisons)
5. Interprocedural analysis: CHA call graph resolution, function summaries for return values
6. Whole-program parameter analysis: fixed-point iteration computes what callers actually pass to each parameter, using converged caller state at call sites
7. Post-convergence check pass classifies findings as Bug/Warning/Safe

## Design Principles

- **Zero annotations.** No special comments, contracts, or spec files. The tool infers everything from your code.
- **Honest about limits.** The three-color model (Bug/Warning/Safe) is transparent about what the analysis can and cannot prove.
- **Built on `go/ssa`.** Works with Go's existing tooling ecosystem.
- **Incremental value.** Each phase delivers a usable tool. Phase 1 alone detects real division-by-zero bugs.

## Usage

```bash
go install github.com/ahmedaabouzied/goprove/cmd/goprove@latest
goprove <package>
```

## Roadmap

| Phase | Focus | Status |
|-------|-------|--------|
| 0 | Foundation — SSA loading, CFG traversal | Done |
| 1 | Integer interval analysis (div-by-zero, overflow) | Done |
| 1.8 | Call graph integration (CHA) | Done |
| 2 | Nil pointer analysis (intraprocedural + interprocedural returns) | Done |
| 3 | Slice bounds analysis | Not started |
| 4 | Full interprocedural analysis (parameter tracking) | Not started |
| 5 | GC pressure analysis | Not started |
| 6 | Concurrency analysis | Not started |
| 7 | Production hardening (SARIF, golangci-lint plugin) | Not started |

## License

[MIT](LICENSE)
