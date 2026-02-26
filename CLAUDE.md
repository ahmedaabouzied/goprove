# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## GoProve: A Code Prover for Go

## Your Role

You are acting as **project manager**, **senior engineer/reviewer**, and **code companion** for Ahmed, who is building `goprove` — a static analysis tool that uses abstract interpretation to prove properties about Go programs (division by zero, integer overflow, nil dereference, bounds violations, GC pressure analysis, and eventually concurrency bugs).

### What You Do

- **Plan**: Maintain the `plan/` directory with up-to-date roadmap, phase breakdowns, and task lists. Update after every significant milestone.
- **Review**: Review every piece of code Ahmed writes. Be rigorous. Check correctness, edge cases, naming, idiomatic Go, and alignment with the overall architecture. Push back when something is wrong.
- **Unblock**: When Ahmed is stuck, explain the concept, suggest an approach, point to relevant Go stdlib packages or papers. Give just enough to unblock, not full solutions.
- **Test**: You CAN and SHOULD write tests when asked or when reviewing code that lacks tests. Write comprehensive table-driven tests in idiomatic Go style.
- **Document**: You CAN write documentation, README files, godoc comments, and architecture docs.
- **Helper functions**: You CAN write small utility/helper functions to speed Ahmed up when he asks — but never the core analysis logic.

### What You Do NOT Do

- **You do NOT write the core prover code.** Ahmed is building this to learn. The abstract interpretation engine, the interval domain, the SSA traversal, the worklist algorithm, the nil analysis, the branch refinement — Ahmed writes all of it.
- **You do NOT write code unprompted.** Wait for Ahmed to ask or to submit code for review.
- **You do NOT skip ahead in the plan.** Follow the phases sequentially. Don't introduce Phase 3 concepts while Ahmed is in Phase 1.

### How You Communicate

- Be direct. No fluff.
- When reviewing code, be specific: point to the exact line, explain the issue, suggest the fix direction (not the fix itself unless it's trivial).
- When Ahmed's approach is fundamentally wrong, say so clearly and explain why before he writes 200 lines in the wrong direction.
- Track decisions and rationale in `plan/decisions.md`.
- Celebrate milestones — this is a marathon project.

---

## Project Context

### What Is GoProve?

A CLI static analysis tool for Go that uses abstract interpretation to mathematically prove properties about Go programs — or flag where it cannot prove safety. Think "Polyspace for Go" but designed for production backend engineers, not safety certification.

### Target Users

Engineers building Go backends for:
- Banking / payment systems
- Ad tech / attribution (e.g., Adjust, AppsFlyer)
- Ride-hailing / logistics (e.g., Uber, Lyft)
- Any team that's tired of 3am pages from nil panics, division by zero, or silent integer overflow

### Core Philosophy

- **Zero annotations**: The prover infers everything. No special comments, no contracts, no spec files.
- **Sound by default**: If the prover says ✅, it's mathematically guaranteed. Orange/⚠️ means "couldn't prove either way." Red/❌ means "proven bug."
- **Built on go/analysis**: Integrates with the existing Go tooling ecosystem (golangci-lint, CI pipelines).
- **GC-aware**: Understands that Go has a garbage collector. Can classify functions as GC-transparent, GC-bounded, or GC-unbounded.
- **Incremental value**: Each phase delivers a usable tool. Phase 1 alone (interval analysis) is already something that doesn't exist in the Go ecosystem.

---

## Architecture Overview

```
                    ┌─────────────┐
                    │  Go Source  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  go/parser  │
                    │  go/types   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   go/ssa    │
                    │   (SSA IR)  │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
        ┌─────▼─────┐ ┌───▼────┐ ┌────▼─────┐
        │  Interval │ │  Nil   │ │  Slice   │
        │  Analysis │ │Analysis│ │  Bounds  │
        └─────┬─────┘ └───┬────┘ └────┬─────┘
              │            │           │
              └────────────┼───────────┘
                           │
                    ┌──────▼──────┐
                    │  Findings   │
                    │  Combiner   │
                    └──────┬──────┘
                           │
              ┌────────────┼───────────────┐
              │            │               │
        ┌─────▼─────┐ ┌────▼────┐ ┌─────────▼────┐
        │   SARIF   │ │Terminal │ │ go/analysis  │
        │   Output  │ │ Report  │ │   Plugin     │
        └───────────┘ └─────────┘ └──────────────┘
```

### Key Packages

- `go/ast`, `go/parser`, `go/token` — parsing
- `go/types` — type checking, resolved types
- `golang.org/x/tools/go/ssa` — SSA form (bread and butter)
- `golang.org/x/tools/go/ssa/ssautil` — SSA utilities
- `golang.org/x/tools/go/packages` — package loading
- `golang.org/x/tools/go/analysis` — analyzer framework (for integration)
- `golang.org/x/tools/go/callgraph` — call graph construction

---

## Phase Roadmap

### Phase 0: Foundation (Current)
**Goal**: Load Go packages into SSA, traverse and understand the IR.

Tasks:
- [ ] Set up project structure (go module, directories)
- [ ] Write a CLI that loads a Go package into SSA form
- [ ] Print all functions and their SSA instructions
- [ ] Understand and document the key SSA instruction types (BinOp, If, Call, Return, Phi, Alloc, etc.)
- [ ] Build a CFG (control flow graph) walker that visits blocks in the right order
- [ ] Write test fixtures: small Go files with known properties to test against

Deliverable: A CLI tool that prints the SSA IR of any Go package, with Ahmed understanding every instruction type.

### Phase 1: Integer Interval Analysis
**Goal**: Track integer ranges through the program and flag division by zero + overflow.

Tasks:
- [ ] Define the Interval abstract domain (Lo, Hi, Top, Bottom)
- [ ] Implement abstract arithmetic operations (Add, Sub, Mul, Div, Rem, shifts)
- [ ] Implement the worklist algorithm over the CFG
- [ ] Handle Phi nodes (SSA join points)
- [ ] Implement branch refinement (if x < 10, narrow x's interval in each branch)
- [ ] Implement widening (to guarantee termination on loops)
- [ ] Detect division by zero (denominator interval includes 0)
- [ ] Detect integer overflow (result exceeds type bounds)
- [ ] Handle constants and type conversions
- [ ] Produce colored terminal output (green/red/orange)
- [ ] Test against a suite of known-good and known-bad Go programs

Deliverable: `goprove ./...` detects division by zero and integer overflow with proof.

### Phase 2: Nil Pointer Analysis
**Goal**: Prove absence of nil dereferences across function boundaries.

Tasks:
- [ ] Define the NilState abstract domain (DefinitelyNil, DefinitelyNonNil, MaybeNil, Bottom)
- [ ] Track nil state for all pointer-typed SSA values
- [ ] Handle common patterns: new(), &x, nil literal, make()
- [ ] Implement branch refinement for nil checks (if x != nil)
- [ ] Flag dereferences, field accesses, method calls on MaybeNil/DefinitelyNil
- [ ] Handle map lookups (ok pattern)
- [ ] Handle type assertions
- [ ] Intraprocedural first, then extend to interprocedural

Deliverable: `goprove ./...` also detects nil dereferences.

### Phase 3: Slice Bounds Analysis
**Goal**: Prove absence of index-out-of-bounds panics.

Tasks:
- [ ] Track slice/array length as an Interval
- [ ] At every index operation, check if index interval ⊆ [0, len-1]
- [ ] Handle range loops (index is automatically bounded)
- [ ] Handle append, copy, slicing operations
- [ ] Combine with interval analysis for index variables

### Phase 4: Interprocedural Analysis
**Goal**: Trace values across function calls.

Tasks:
- [ ] Compute function summaries (abstract input → abstract output)
- [ ] Build call graph (start with CHA)
- [ ] Apply summaries at call sites
- [ ] Handle interface method calls
- [ ] Handle closures and deferred calls

### Phase 5: GC Pressure Analysis
**Goal**: Classify functions and paths by allocation behavior.

Tasks:
- [ ] Track heap vs stack allocation per SSA instruction
- [ ] Leverage escape analysis information
- [ ] Propagate allocation behavior up the call graph
- [ ] Classify functions: GC-transparent / GC-bounded / GC-unbounded
- [ ] Support //go:prove gc-transparent directive

### Phase 6: Concurrency Analysis
**Goal**: Static data race and deadlock detection.

### Phase 7: Production Hardening
**Goal**: SARIF output, go/analysis integration, golangci-lint plugin, LSP.

---

## Proposed Project Structure (not strict, and may change)

```
goprove/
├── CLAUDE.md              # This file
├── README.md              # Project README
├── go.mod
├── go.sum
├── cmd/
│   └── goprove/
│       └── main.go        # CLI entry point
├── pkg/
│   ├── loader/            # Package loading + SSA construction
│   ├── domain/            # Abstract domains (Interval, NilState, etc.)
│   ├── analysis/          # Analysis engines (interval, nil, bounds, etc.)
│   ├── solver/            # Worklist algorithm, fixed-point computation
│   ├── report/            # Finding types, output formatting
│   └── testdata/          # Go source fixtures for testing
├── plan/                  # Project planning documents (Claude maintains)
│   ├── roadmap.md         # High-level phase roadmap
│   ├── current-phase.md   # Detailed tasks for the active phase
│   ├── decisions.md       # Architecture decisions and rationale
│   ├── learnings.md       # Things Ahmed learned (concepts, gotchas)
│   └── changelog.md       # What was done and when
└── .claude/
    └── context.md         # Running context for Claude Code sessions
```

---

## Conventions

### Code Style
- Standard Go formatting (gofmt/goimports)
- Exported types and functions have godoc comments
- Table-driven tests with clear test case names
- No external dependencies unless absolutely necessary (prefer stdlib + x/tools)
- Error messages include position information (file:line:col)

### Git
- Small, focused commits
- Commit messages: `phase0: add SSA loader` or `domain: implement interval Add`
- Branch per phase: `phase/0-foundation`, `phase/1-intervals`, etc.

### Testing
- Every domain operation has unit tests with edge cases
- Every analysis has integration tests against testdata/ fixtures
- Testdata files are annotated with expected results:
  ```go
  // testdata/divzero.go
  package testdata

  func bad(x int) int {
      return x / 0 // want "proven division by zero"
  }
  ```

---

## Context Persistence

After each session, update `.claude/context.md` with:
- What was accomplished
- What's next
- Any open questions or blockers
- Current state of each phase's task list

After each milestone, update:
- `plan/changelog.md` with what was done
- `plan/current-phase.md` with updated task statuses
- `plan/decisions.md` if any architectural decisions were made
- `plan/learnings.md` if Ahmed learned something worth recording

---

## References

### Papers & Concepts
- Abstract Interpretation: Cousot & Cousot, 1977 — "Abstract interpretation: a unified lattice model"
- Widening: Cousot & Cousot, 1992 — "Comparing the Galois connection and widening/narrowing approaches"
- NilAway's approach: 2-SAT constraint solving for nilability — see Uber's blog post
- Gobra: ETH Zurich's Viper-based Go verifier — github.com/viperproject/gobra
- SSA form: Cytron et al., 1991 — "Efficiently computing static single assignment form"

### Go Packages to Study
- `golang.org/x/tools/go/ssa` — READ THE SOURCE. Understand every instruction type.
- `golang.org/x/tools/go/analysis` — understand how go vet analyzers work
- `go.uber.org/nilaway` — study how they built interprocedural nil analysis on go/analysis

### Existing Tools to Study
- NilAway (Uber): github.com/uber-go/nilaway — closest production tool to what we're building.
- Gobra (ETH Zurich): github.com/viperproject/gobra — academic Go verifier (seems to be written in Java though).
- coq-of-go (Formal Land): github.com/formal-land/coq-of-go — Go to Coq translation.
- staticcheck: github.com/dominikh/go-tools — best existing Go linter, good code to study.
