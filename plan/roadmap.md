# GoProve Roadmap

## Vision
A CLI tool that proves Go code is safe — or tells you exactly where it isn't.

```
$ goprove ./...
pkg/billing/charge.go:47  ❌ division by zero — divisor can be 0
pkg/handler/click.go:89   ✅ bounds proven safe — i ∈ [0, len-1]
pkg/model/user.go:23      ⚠️ possible nil dereference — user from DB
Summary: 1 proven bug, 1 warning, 342 proven safe
```

## Phases

| Phase | Name | What It Proves | Status |
|-------|------|----------------|--------|
| 0 | Foundation | Nothing yet — understand SSA | ✅ Complete |
| 1 | Integer Intervals | Division by zero, integer overflow | ✅ Complete |
| 2 | Nil Analysis | Nil pointer dereference | 🔲 Not started |
| 3 | Slice Bounds | Index out of bounds | 🔲 Not started |
| 4 | Interprocedural | Cross-function bugs (all above) | 🔲 Not started |
| 5 | GC Pressure | Allocation behavior, GC-transparency | 🔲 Not started |
| 6 | Concurrency | Data races, deadlocks | 🔲 Not started |
| 7 | Production | SARIF, golangci-lint, LSP | 🔲 Not started |

## Key Milestones

- **M1**: First SSA dump of a real package (Phase 0) ✅
- **M2**: First proven division-by-zero bug found (Phase 1) ✅
- **M2.5**: First proven integer overflow bug found (Phase 1) ✅
- **M3**: First proven nil dereference found (Phase 2)
- **M4**: First cross-function bug found (Phase 4)
- **M5**: Open source release with CI integration (Phase 7)

## Non-Goals (For Now)
- Certification against IEC 62304 or any safety standard
- Annotation-based specification (that's Gobra's territory)
- Full formal verification / theorem proving (that's coq-of-go's territory)
- Runtime analysis (that's the race detector's territory)
