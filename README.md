# GoProve

A static analysis tool for Go that uses abstract interpretation to mathematically prove properties about Go programs — or flag where it cannot prove safety.

## What It Does

GoProve analyzes Go source code at compile time and classifies each potential error site with a verdict:

- **Bug** — proven to fail at runtime. Guaranteed.
- **Warning** — could not prove safe or unsafe. Worth a human look.
- **Safe** — mathematically guaranteed to be safe for all inputs.

```
$ goprove ./...
pkg/billing/charge.go:47  BUG   division by zero — divisor is always 0
pkg/billing/charge.go:52  WARN  possible integer overflow in conversion
pkg/handler/click.go:89   WARN  possible division by zero
Summary: 1 proven bug, 2 warnings
```

## Currently Detects

### Division by Zero
- **Proven bug**: divisor is always zero
- **Warning**: divisor range includes zero but isn't guaranteed
- **Safe**: divisor range provably excludes zero (e.g., after `if y != 0` guard)

### Integer Overflow
- **Arithmetic overflow**: result of `+`, `-`, `*`, `/` exceeds type bounds (int8, int16, int32)
- **Conversion overflow**: narrowing cast loses data (e.g., int16 → int8 when value > 127)
- **Negation overflow**: `-x` overflows when x is the minimum value (e.g., `-(-128)` for int8)
- Branch guards are respected: `if x < 100 { int8(x) }` is proven safe

### How It Works
- Builds SSA (Static Single Assignment) form via `golang.org/x/tools/go/ssa`
- Runs abstract interpretation with an interval domain `[lo, hi]` per variable
- Worklist algorithm with widening guarantees termination on loops
- Branch refinement narrows intervals through `if` conditions
- Post-convergence check pass classifies findings as Bug/Warning/Safe

## Design Principles

- **Zero annotations.** No special comments, contracts, or spec files. The prover infers everything from your code.
- **Sound by default.** If it says safe, it's safe. No false negatives.
- **Built on `go/ssa`.** Works with Go's existing tooling ecosystem.

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
| 2 | Nil pointer analysis | Not started |
| 3 | Slice bounds analysis | Not started |
| 4 | Interprocedural analysis | Not started |
| 5 | GC pressure analysis | Not started |
| 6 | Concurrency analysis | Not started |
| 7 | Production hardening (SARIF, golangci-lint plugin) | Not started |

## License

[MIT](LICENSE)
