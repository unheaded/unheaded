# S73 PUBLIC LAUNCH CLEANUP — Master Battle Plan

**Date**: 2026-03-18
**Sprint**: S73 — Public Launch Cleanup
**Target**: Repository ready for GitHub public visibility flip
**Total Phases**: 5
**Estimated Duration**: 12-18 hours across multiple agent sessions

## Intelligence Summary

| Category | Count | Source |
|----------|-------|--------|
| Production code issues | 74 | grep sweep of cmd/, internal/, pkg/, services/, ebpf/ |
| Documentation markers | 418 | grep sweep of docs/ |
| CRITICAL blockers | 9 | JWT auth, LXD clients, eBPF loader, scaffolds |
| HIGH priority | 10 | API stubs, PQC gaps, gRPC unimplemented |
| MEDIUM (WAF migration) | 39 | pkg/waf/ — intentional Rust rebuild markers |
| MEDIUM (other) | 10 | Misc prod code |
| LOW | 6 | Nice-to-have |

## Phase Dependency Graph

```
Phase 1 (Critical Blockers)
    │
    ▼
Phase 2 (High Priority)
    │
    ├──────────────────┐
    ▼                  ▼
Phase 3 (WAF)    Phase 4 (Docs)     ← PARALLEL
    │                  │
    └──────┬───────────┘
           ▼
     Phase 5 (Final Verification Gate)
           │
           ▼
     PUBLIC FLIP ✓
```

## Phase Manifest

| Phase | File | Steps | Agent | Parallel |
|-------|------|-------|-------|----------|
| 1 | [phase-1-critical-blockers.md](phase-1-critical-blockers.md) | 1-46 | Claude Code + Marshal | Sequential |
| 2 | [phase-2-high-priority.md](phase-2-high-priority.md) | 100-139 | Claude Code + Marshal | After Phase 1 |
| 3 | [phase-3-waf-cleanup.md](phase-3-waf-cleanup.md) | 200-251 | Claude Code | Parallel with Phase 4 |
| 4 | [phase-4-docs-cleanup.md](phase-4-docs-cleanup.md) | 300-331 | Claude Code | Parallel with Phase 3 |
| 5 | [phase-5-final-verification.md](phase-5-final-verification.md) | 400-418 | Claude Code + Marshal | After ALL phases |

## Execution Instructions

### For Claude Code agents:

1. Read the phase document assigned to you
2. Execute steps in order, following all [V] verification gates
3. Do NOT skip exit gates
4. Commit at every [C] checkpoint
5. If stuck for >3x estimated time on any step, activate Skip Protocol
6. Report STUCK steps in commit messages

### For Marshal:

1. Monitor agent progress via git log
2. Verify exit gates are genuinely passed (not rubber-stamped)
3. Block Phase 5 until Phases 1-4 exit gates are all GREEN
4. Block PUBLIC FLIP until Phase 5 exit gate is GREEN

### For Muck:

1. Review Phase 5 LAUNCH_READINESS.md report
2. Make go/no-go call on any MEDIUM findings
3. Flip the switch: GitHub Settings → Public

## Commit Message Format

```
[S73-P{phase}] Steps {X}-{Y}: {description}
```

## Legend

[B] = Bash command | [V] = Verification | [D] = Debug | [W] = Write file
[R] = Read file | [S] = Sudo | [P] = Parallelizable | [C] = Commit checkpoint

---

*S73 Master Battle Plan — Forged 2026-03-18*
*5 Phases. ~200 Steps. The Kingdom goes public.*
*Every scaffold removed. Every stub resolved. Every gate verified.*
