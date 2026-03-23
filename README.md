<p align="center">
  <img src="logo.png" alt="GoProve" width="200">
</p>

<h1 align="center">GoProve</h1>

<p align="center">
  A static analysis tool for Go that uses abstract interpretation to <strong>mathematically prove</strong> properties about your code — or tell you exactly where it can't.
</p>

<p align="center">
  <a href="https://github.com/ahmedaabouzied/goprove/actions/workflows/go.yml"><img src="https://github.com/ahmedaabouzied/goprove/actions/workflows/go.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/ahmedaabouzied/goprove/actions/workflows/go.yml"><img src="https://img.shields.io/badge/GoProve-proven-brightgreen?logo=go" alt="GoProve"></a>
  <a href="https://github.com/ahmedaabouzied/goprove/releases"><img src="https://img.shields.io/github/v/release/ahmedaabouzied/goprove" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

When GoProve says a bug exists, it's guaranteed. When it says your code is safe, it's proven. When it's unsure, it tells you honestly.

---

## Table of Contents

- [Why GoProve?](#why-goprove)
- [Install](#install)
- [Usage](#usage)
- [What It Detects](#what-it-detects)
- [How It Works](#how-it-works)
- [Three-Color Model](#three-color-model)
- [Comparison with Other Tools](#comparison-with-other-tools)
- [Soundness Details](#soundness-details)
- [Real-World Results](#real-world-results)
- [CI Integration](#ci-integration)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Credits](#credits)
- [License](#license)

---

```
$ goprove ./...
 Error: billing/charge.go:47 nil dereference of nil pointer — value is always nil
 Warning: handler/click.go:89 possible nil dereference of config — add a nil check before use

 Summary: 1 bugs, 1 warnings.
```

## Why GoProve?

Go's type system prevents many classes of bugs at compile time, but three categories consistently escape into production: **nil pointer dereferences**, **division by zero**, and **integer overflow**. These are runtime panics — they depend on values, not types, and no amount of type safety prevents them.

Existing Go tools approach this through pattern matching (`go vet`, `staticcheck`) or constraint inference (`NilAway`). These are useful but limited — they can miss real bugs and flag correct code, with no formal guarantee either way.

GoProve takes a different approach. It uses **abstract interpretation** (Cousot & Cousot, 1977) — a mathematically rigorous framework for computing sound approximations of program behavior. The same theoretical foundation is used in tools like Polyspace (automotive/aerospace) and Astrée (Airbus flight controllers). GoProve applies this to production Go code.

### Why proving matters outside aerospace

Formal verification has traditionally been reserved for safety-critical systems — flight controllers, medical devices, nuclear reactors. The assumption is that ordinary software doesn't need mathematical proof.

But consider what "ordinary" Go backends actually do: process payments, calculate attribution, route vehicles, serve millions of requests. A nil panic in these systems doesn't kill anyone — but it loses money, corrupts data, breaks SLAs, and pages engineers at 3am. The cost of these bugs is real and measurable.

The barrier to proving wasn't that backend code didn't need it — it was that proving tools required annotations, specifications, or PhDs to operate. Abstract interpretation removes that barrier. It works on unmodified source code, runs in seconds, and gives you the same mathematical guarantee that Airbus gets for flight software: **if the tool says it's safe, it's safe.**

The question isn't "is my code safety-critical enough for proving?" It's "why wouldn't I want a mathematical guarantee that my nil checks actually work?"

### Who is this for?

Engineers building Go backends for:
- **Banking / payment systems** — where a nil panic in a transaction handler means lost money
- **Ad tech / attribution** — where a silent integer overflow in billing means wrong charges
- **Ride-hailing / logistics** — where a division by zero in routing means a 3am page
- Any team tired of runtime panics that their linter didn't catch

### Core philosophy

- **Zero annotations.** No special comments, contracts, or spec files. The tool infers everything from your code.
- **Sound for bugs.** When GoProve says Bug, abstract interpretation has proven it across all execution paths. No false negatives for proven bugs.
- **Honest about limits.** The three-color model (Bug/Warning/Safe) is transparent about what the analysis can and cannot prove. Completeness is deliberately sacrificed — the analysis may warn about code that's actually safe, in exchange for never missing a proven bug.
- **Incremental value.** Each analysis domain delivers a usable tool on its own. You don't need the full suite to get value.

### What GoProve is not

- Not a replacement for testing — it proves properties about all inputs, but only for the patterns it tracks.
- Not a certification tool — it's designed for production engineers, not safety certification (IEC 62304, DO-178C).
- Not an annotation-based verifier — that's [Gobra's](https://github.com/viperproject/gobra) territory.
- Not a theorem prover — that's [coq-of-go's](https://github.com/formal-land/coq-of-go) territory.
- Not a runtime analyzer — that's the race detector's territory.

## Install

```bash
go install github.com/ahmedaabouzied/goprove/cmd/goprove@latest
```

## Usage

```bash
# Analyze a single package
goprove ./pkg/server

# Analyze all packages (recommended)
goprove ./...

# Use in CI — exits with code 1 if any findings
goprove ./... || exit 1
```

## What It Detects

### Nil Pointer Dereference

GoProve tracks nil state through your entire program:

```go
func process(config *Config) {
    if config == nil {
        return
    }
    helper(config) // GoProve knows config is non-nil here
}

func helper(config *Config) {
    config.Validate() // No warning — all callers pass non-nil
}
```

- Proven nil derefs (`var p *int; *p`) are flagged as **Bug**
- Unchecked params are flagged as **Warning** — unless all callers pass non-nil
- Nil checks (`if p != nil`), early returns (`if p == nil { return }`), `new()`, `make()`, `&x` are all tracked
- Field reloads after nil checks (`if o.Field != nil { o.Field.Use() }`) are proven safe
- Interface method calls (`s.Method()`) on nil interfaces are detected
- Global variable nil checks propagate across subsequent reads

### Division by Zero

```go
func safe(x, y int) int {
    if y != 0 {
        return x / y // GoProve: safe
    }
    return 0
}

func unsafe(x int) int {
    zero := 0
    return x / zero // GoProve: Bug — division by zero
}
```

### Integer Overflow

```go
func narrow(x int16) int8 {
    if x < 100 && x > -100 {
        return int8(x) // GoProve: safe — x fits in int8
    }
    return 0
}
```

- Arithmetic overflow on int8, int16, int32
- Narrowing conversion overflow (int16 → int8)
- Negation overflow (`-math.MinInt8`)

## How It Works

GoProve builds on Go's SSA (Static Single Assignment) intermediate representation and runs abstract interpretation with two domains:

1. **Interval domain** `[lo, hi]` per integer variable — proves division and overflow safety
2. **Nil state domain** `{DefinitelyNil, DefinitelyNotNil, MaybeNil}` per pointer — proves nil safety

Key techniques:
- **Address-based memory model** — tracks nil state per memory address, not per SSA register. Two loads from the same field share nil state.
- **Whole-program parameter analysis** — fixed-point iteration computes what callers actually pass to each parameter. If all callers pass non-nil after nil guards, the parameter is proven non-nil.
- **Interprocedural return summaries** — callee return nil/interval state is tracked via CHA call graph resolution.
- **Branch refinement** — `if p != nil`, `if y != 0`, `<`, `<=`, `>`, `>=` narrow abstract state per branch.
- **Worklist with widening** — guarantees termination on loops while maintaining soundness.

## Three-Color Model

GoProve uses an honest three-color classification:

| Color | Meaning | Guarantee |
|-------|---------|-----------|
| **Bug** (red) | Proven to crash at runtime | Mathematical guarantee — no false negatives |
| **Warning** (yellow) | Could not prove safe or unsafe | Worth investigating — may be real or may be a limitation |
| **Safe** (no output) | Proven safe for tracked patterns | Mathematical guarantee — for the patterns we track |

## Comparison with Other Tools

| | **GoProve** | **NilAway (Uber)** | **staticcheck** | **go vet** |
|---|---|---|---|---|
| Technique | Abstract interpretation | Constraint-based 2-SAT | Pattern matching | Pattern matching |
| Nil detection | Lattice-based with proof | Constraint inference | Limited patterns | Very limited |
| Division by zero | Yes (interval domain) | No | No | No |
| Integer overflow | Yes (interval domain) | No | No | No |
| Soundness | **Sound for bugs** | Neither sound nor complete | Not sound | Not sound |
| Interprocedural | Return summaries + whole-program params | go/analysis Facts | No | No |
| Parameter tracking | **Whole-program dataflow** | Constraint propagation | No | No |
| Address model | **Memory-address based** | Assertion trees | No | No |
| golangci-lint | Not yet | Yes | Yes | Yes |
| Scale tested | ~50k LOC | 90M LOC (Uber) | Production-grade | Production-grade |

**GoProve's advantage**: when it says Bug, it's proven. When it says Safe, it's mathematically guaranteed. No other open-source Go tool offers this.

**NilAway's advantage**: faster, scales to massive codebases, integrates with golangci-lint. But it can produce both false positives and false negatives.

## Soundness Details

**What "proven" means:**
- Bug findings are sound: abstract interpretation has proven the bug exists across all execution paths.
- Safe findings are sound for tracked patterns: if GoProve produces no finding, the operation is safe for the patterns it analyzes.

**What the tool does NOT cover:**
- Concurrency: no tracking of values across goroutines or channels.
- Reflection: `reflect.Call` arguments are invisible to static analysis.
- Some `unsafe.Pointer` cast chains.
- CGo: C function calls are outside Go's type system.

**Intentional pragmatic choices:**
- Method receivers are assumed non-nil (eliminates noise, misses `(*T)(nil).Method()`).
- Slice indexing only flags proven nil slices, not possible nil (deferred to future bounds checker).

**Patterns correctly handled:**
- `v, ok := m[key]; if ok && v != nil { *v }` — map ok pattern
- `v, ok := x.(T); if ok { v.Do() }` — type assertion ok pattern
- `time.NewTimer()`, `bytes.NewBuffer()` — stdlib return values (via interprocedural analysis)
- `if x.F != nil { x.F.Use() }` — field reload after nil check (via address model)

## Real-World Results

| Codebase | Findings |
|----------|----------|
| Production logger package | 0 warnings (was 32 before whole-program parameter tracking) |
| go-redis v9.7.3 | 62 nil + 3 overflow warnings, all legitimate. 0 false positives on guarded patterns. |
| net/http (stdlib) | 0 proven bugs. Warnings only on exported function parameters. |
| Production platform package | 0 warnings |
| gboost (ML library) | 0 bugs, 16 warnings |

## CI Integration

GoProve exits with code 1 when findings are detected:

```yaml
# GitHub Actions
- name: Run GoProve
  run: |
    go install github.com/ahmedaabouzied/goprove/cmd/goprove@latest
    goprove ./...
```

## Roadmap

| Phase | Focus | Status |
|-------|-------|--------|
| 1 | Integer interval analysis (div-by-zero, overflow) | Done |
| 2 | Nil pointer analysis (address model, interprocedural, whole-program params) | Done |
| 3 | Slice bounds analysis | Planned |
| 4 | Whole-program integer range tracking | Planned |
| 5 | GC pressure analysis | Planned |
| 6 | Concurrency analysis | Planned |
| 7 | SARIF output, golangci-lint plugin, GitHub Action | Planned |

## Contributing

Contributions welcome. The codebase has 296 test functions with 99.5% statement coverage.

```bash
git clone https://github.com/ahmedaabouzied/goprove
cd goprove
go test ./...
```

## Credits

Gopher mascot created with [gopherize.me](https://gopherize.me) by [Mat Ryer](https://github.com/matryer) and [Ashley McNamara](https://github.com/ashleymcnamara), based on original Go gopher by [Renee French](https://reneefrench.blogspot.com/).

## License

[MIT](LICENSE)
