# GoProve

A static analysis tool for Go that uses abstract interpretation to mathematically prove properties about Go programs — or flag where it cannot prove safety.

## What It Does

GoProve analyzes Go source code at compile time and classifies each potential error site with a verdict:

- **Proven safe** — mathematically guaranteed to be safe at runtime.
- **Proven bug** — the code will definitely fail.
- **Unknown** — the tool couldn't prove it either way. Worth a human look.

## Target Properties

- Division by zero
- Integer overflow
- Nil pointer dereference
- Slice out-of-bounds access
- GC pressure classification
- Concurrency bugs (data races, deadlocks)

## Design Principles

- **Zero annotations.** No special comments, contracts, or spec files. The prover infers everything from your code.
- **Sound by default.** If it says safe, it's safe. No false negatives.
- **Built on `go/ssa`.** Works with Go's existing tooling ecosystem.

## Roadmap

| Phase | Focus | Status |
|-------|-------|--------|
| 0 | Foundation — SSA loading, CFG traversal | Not started |
| 1 | Integer interval analysis (div-by-zero, overflow) | — |
| 2 | Nil pointer analysis | — |
| 3 | Slice bounds analysis | — |
| 4 | Interprocedural analysis | — |
| 5 | GC pressure analysis | — |
| 6 | Concurrency analysis | — |
| 7 | Production hardening (SARIF, golangci-lint plugin) | — |

## License

[MIT](LICENSE)
