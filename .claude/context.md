# Claude Code Session Context

## Current State
- **Active Phase**: Phase 0 — Foundation
- **Active Task**: 0.1 — Project Setup
- **Branch**: (not yet created)
- **Last Session**: 2026-02-26 (project inception)

## What Was Done Last
- Full project architecture designed in conversation with Ahmed
- CLAUDE.md created with complete role definition and phase roadmap
- Plan directory initialized with roadmap, decisions, learnings, changelog
- Studied existing Go provers: NilAway, Gobra, coq-of-go

## What's Next
- Ahmed initializes the Go module and directory structure
- Begin Task 0.2: Package Loader
- Start exploring `golang.org/x/tools/go/ssa` package

## Open Questions
- None yet

## Ahmed's Background (Relevant to Project)
- Software engineer (ad tech / mobile attribution)
- Strong Go experience, works with Aerospike, Prometheus, infrastructure monitoring
- Enjoys building from scratch to understand deeply
- Has ML/fraud detection experience (relevant: understanding false positives/negatives)
- Prefers terminal-based workflows
- This project is both a learning exercise and a real tool he wants to use

## Key Technical Decisions Made
- SSA over AST (ADR-001)
- Standalone CLI first, go/analysis integration later (ADR-002)
- Zero external deps for core (ADR-003)
- Interval domain first (ADR-004)
- Three-color output: green/red/orange (ADR-005)
