# Changelog

## 2026-03-30 — Stdlib Cache, Multi-Pred Refinement Fix, Re-analysis

### Stdlib nil analysis cache (`goprove cache stdlib`)
- New command pre-computes nil return summaries for all Go stdlib functions
- 360 packages, 20,589 function summaries, ~15 seconds to generate
- Cache saved as versioned JSON (`summaries-<goversion>-<goproveversion>.json`)
- `SummaryCache` extended with `GoproveVersion`, `Merge()`, `LoadAndValidateCache()`
- Normal `goprove ./...` loads stdlib cache automatically from `DefaultCachePath` as fallback
- Cache will be hosted on goprove.dev, regenerated per goprove/Go release

### Multi-predecessor refinement overwrite bug (soundness fix)
- `refineFromPredecessor` iterated all predecessors and each `refineFromCondition` call overwrote the state with a raw assignment
- At merge points with multiple predecessors, the last edge's refinement won — e.g., `SetAll()` pattern: defensive `if b != nil` caused return value to be classified DefinitelyNil (Bug) instead of MaybeNil
- Fix: skip refinement when `len(block.Preds) != 1`
- 5 new integration tests covering the fix and verifying single-pred refinement still works

### Re-analysis of 20 OSS modules
- Total findings: 1,948 → 946 (-51%)
- Key insight: the original triage was wrong. Most "caller_invariant" FPs (897 findings) are actually **true warnings** — functions accept pointer params with no nil guard, and a caller could pass nil
- Modules that increased (gin +23%, backoff +267%, go-cache 0→6) are not regressions — the multi-pred bug was accidentally suppressing legitimate warnings
- Estimated real FP rate: ~10% (array IndexAddr, Store/Load tracking, type switch narrowing)
- **Phase 2.5 definition of done (FP rate < 30%) is met**

### Remaining real FPs (~100 estimated)
- Array IndexAddr on value types (P2-B) — ~40-80
- Store/Load tracking misses — ~20-40
- Type switch narrowing — ~20-40

## 2026-03-26 — Seed Analysis: 20 OSS Modules

### Findings
- Ran GoProve v0.2.3 against 20 popular Go modules (fiber, testify, validator, echo, gin, gjson, cobra, zerolog, cron, decimal, chi, mux, viper, multierror, otp, mapstructure, uuid, backoff, logrus, go-cache)
- **1,948 total findings**: 1,925 false positives (98.8%), 23 true positives (1.2%)
- 97% of findings are nil_deref category — interval/overflow analysis is relatively clean

### True Positives (the good news)
- Echo: discarded `f.Stat()` error → nil `FileInfo` panic (4 findings)
- Zerolog: `reflect.TypeOf(nil).Kind()` panic, discarded `os.Stdin.Stat()` error, division by zero in `appendUnixNanoTimes`
- Gin: missing negative overflow check in `safeInt8`
- Decimal: `abs(math.MinInt32)` overflows
- Testify: `reflect.TypeOf(nil)` in assertion helpers

### Root Cause Analysis
- **46.6%**: `collectCallSites` only walks `pkg.Members` — misses methods and closures entirely
- **~31%**: `*ssa.Extract` not handled in `transferInstruction` — breaks all multi-return patterns
- **~8%**: `*ssa.TypeAssert` and `*ssa.Lookup` not handled
- **4.3%**: No stdlib return value modeling (external → MaybeNil)
- **4.2%**: Array IndexAddr flagged as nil deref (arrays are value types)

### Decision
- Created Phase 2.5 (False Positive Reduction) to fix these before proceeding to Phase 3
- P0 fixes alone (collectCallSites + Extract) should cut FP rate to ~20-30%

## 2026-03-20 — Nil Pointer Analysis: Full Phase 2

### Interprocedural + Global Tracking + Message Quality
- Interprocedural nil summaries: analyze callee return values via CHA call graph, cache summaries to avoid recomputation
- Sentinel caching in lookupOrComputeSummary to break recursive call chains (A calls B calls A)
- Global variable nil tracking: nil checks on globals propagate to subsequent reads of the same global
- Skip global address loads in checkInstruction (the address itself is always valid, only the value can be nil)
- initBlockState: predecessor state flows across blocks, fixing `if p == nil { return }; *p` pattern
- Method receivers assumed DefinitelyNotNil (intentional pragmatic unsoundness)
- nilValueName: human-readable names (parameters, globals, call results), empty for SSA registers
- Actionable messages: "possible nil dereference of config — add a nil check before use"
- Per-function deduplication: same variable reported once per function
- Soft error handling in loader: type errors warn instead of failing (real codebases have transitive soft errors)
- Nil package guard in Prover.Prove (packages with load errors produce nil SSA packages)
- Real-world validation: production Go codebase went from 32 warnings to 1

### v0.1.0 Release + go/analysis Integration
- Tagged and published v0.1.0 on GitHub
- GitHub Action (goprove-action) for CI integration with whole-program analysis
- go/analysis integration: pkg/analyzer/ with Analyzer, NilAnalyzer, IntervalAnalyzer
- goprove-lint binary (singlechecker) for `go vet -vettool` compatibility
- goprove-multi binary (multichecker) exposing analyzers separately
- analysistest-based tests with `// want` annotations
- Exit code 1 on findings for CI pipelines
- Summary line: "N bugs, M warnings"
- Cross-package finding deduplication by formatted output string
- 296 test functions, 99.5% statement coverage
- README: install modes, usage (CLI/go vet/golangci-lint/GitHub Action), comparison table, soundness docs
- Logo (gopherize.me) and CI/release/license badges
- CI dogfooding: goprove-action runs on goprove's own repo

### Address Model + Fix Plan Completion
- Unified address-based memory model: replaces separate global, field, and store patches with one mechanism
- addressKey identifies memory locations by (base, field, kind) — handles globals, fields, indices uniformly
- resolveAddress extracts addressKey from FieldAddr, Global, IndexAddr instructions
- Store tracking: *ssa.Store writes propagate nil state to target address
- addrState properly reset per Analyze call (was leaking across functions)
- Interval analyzer dedup: findings deduplicated by (position, message) — 500+ → 3 on go-redis
- unsafe.Pointer Convert: *ssa.Convert propagates source nil state
- Interface invoke check: s.Method() on nil interface now detected via IsInvoke()
- go-redis validation: 0 false positives on search_commands.go field-reload pattern, 65 total unique findings

### Whole-Program Parameter Analysis
- Fixed-point iteration: analyze all functions, compute param nil states from converged caller state, repeat until stable
- Block-level argument lookup: at each call site, use caller's converged block state (not just SSA value type)
- Cross-package tracking: analyze any function with call sites in scope, not just unexported
- convergedStates: NilAnalyzer preserves per-function state for param analysis lookup
- Removed exported function skip — if callers are visible, use their data
- Real-world results: production logger 32 → 0 warnings, attribution worker 352 → 47

### Intraprocedural Foundation

- Implemented NilState abstract domain with Join, Meet, Equals — 4-element lattice (NilBottom, DefinitelyNil, DefinitelyNotNil, MaybeNil)
- Built NilAnalyzer with per-block state tracking, same worklist pattern as interval analyzer
- Transfer functions: Alloc, MakeInterface, MakeSlice, MakeMap, MakeChan → DefinitelyNotNil; FieldAddr, IndexAddr → DefinitelyNotNil (post-dereference); Phi → Join of predecessor edges
- Branch refinement: `if p != nil` / `if p == nil` narrows state in true/false branches, handles nil on either side of comparison
- Check pass: flags *ssa.UnOp(MUL), *ssa.FieldAddr, *ssa.IndexAddr on DefinitelyNil (Bug) or MaybeNil (Warning)
- Slice IndexAddr: only flags DefinitelyNil to reduce noise — MaybeNil deferred to bounds checker (Phase 3)
- Wired NilAnalyzer into CLI alongside interval analyzer — combined findings sorted by severity
- Fixed FieldAddr double-reporting: FieldAddr/IndexAddr results tracked as DefinitelyNotNil so subsequent loads don't re-flag
- lookupNilState, isNillable, isSliceType helpers with full test coverage
- 100% test coverage on nil_analyzer.go — 60+ test cases including synthetic, table-driven, and real SSA tests
- Verified against testdata: DerefParam (Warning), DerefAfterCheck (safe), DerefNew (safe), DerefNilLiteral (Bug), FieldAccessOnNil (Bug)

## 2026-03-03 — Integer Overflow Detection (Task 1.5 + 1.6)

- Implemented `IntervalForType` in `bounds.go`: maps `types.BasicKind` to type bounds for int8, int16, int32
- Implemented `Contains` method on `Interval` for checking if one interval is fully contained in another
- Implemented `flagOverflow`: checks BinOp result intervals against type bounds post-convergence
- Implemented `checkConvertOp`: detects narrowing overflow in type conversions (e.g. int16→int8)
- Implemented `checkUnOp`: detects negation overflow (e.g. `-(-128)` = 128 overflows int8)
- Extracted `checkOverflow` shared helper for Bug/Warning/Safe classification with context-aware messages
- Fixed param initialization: entry block params now start at type bounds via `IntervalForType` instead of Top
- Three-way classification: Bug (result entirely outside bounds), Warning (partial overlap), Safe (contained)
- Distinct messages per check: "integer overflow", "integer overflow in conversion", "integer overflow in negation"
- int64, int, uint types deliberately untracked (internal representation is int64, so overflow is undetectable)
- 20 bounds tests, 47 Contains tests, 30 overflow tests, 16 param bounds tests, 32 conversion tests, 30 negation tests
- Total: 175+ new test cases, all passing

## 2026-03-02 — Worklist Algorithm + Widening

- Implemented worklist algorithm with change detection (copyBlockState, stateEqual using maps.Copy/maps.Equal)
- Implemented Widen operator on Interval: detects bound growth, jumps to MinInt64/MaxInt64
- Identified loop headers via RPO index comparison (back-edge detection)
- Applied widening at loop headers after joining all predecessor states
- Separated finding collection into a final check pass on converged state (no duplicates)
- Added maxIterations safety cap (1000) for pre-narrowing termination guarantee
- Fixed critical Phi bug: transferPhi now looks up edge values from predecessor blocks
- Fixed lookupInterval: unvisited blocks return Bottom (identity for Join) instead of Top
- 48 test cases total (30 base + 5 loop + 13 advanced loop patterns), 100% coverage
- Test patterns include: nested loops, while-style loops, break, step-by-2, guarded division, mod in loops, complex CFGs

## 2026-03-01 — Branch Refinement + Per-Block State

- Added comparison operators (<, <=, >, >=) to refineFromCondition using Meet
- Refactored refineFromCondition into refineFromEquality + refineFromComparison
- Discovered and fixed soundness bug: sibling branches corrupting shared state
- Changed state from flat map[Value]Interval to per-block map[*BasicBlock]map[Value]Interval
- Added initBlockState: copies predecessor state (Join for multiple predecessors)
- ExcludeZero tests for Join, Meet, ContainsZero propagation
- Achieved 100% test coverage across all files (30+ analyzer test cases)

## 2026-02-28 — Interval Domain + Basic Analyzer

- Implemented Interval domain: NewInterval, Top, Bottom, Join, Meet, Equals
- Implemented interval arithmetic: Add, Sub, Mul, Div with checkSpecial helper
- Added ExcludeZero flag to handle != 0 branch refinement for division safety
- Built Analyzer with RPO walk, transferBinOp, transferPhi, flagDivisionByZero
- lookupInterval handles *ssa.Const, state map, defaults to Top
- Basic branch refinement for == and != operators

## 2026-02-27 — CFG Walker + Package Merge

- Implemented non-recursive RPO walker using explicit stack DFS (NASA P10 Rule 1)
- Merged cfg and domain packages into single analysis package
- Tests for linear, diamond, loop, nested loop CFG shapes
- buildSSA test helper for constructing SSA from source strings

## 2026-02-26 — Project Inception

- Designed the overall architecture and phase roadmap through conversation
- Studied existing tools: Gobra (ETH Zurich), NilAway (Uber), coq-of-go (Formal Land)
- Decided on abstract interpretation approach (not deductive verification)
- Created CLAUDE.md and plan/ directory structure
- Ready to begin Phase 0
