# Battle Plans

Index of the execution plans in `docs/battle-plans/`, newest first, with where each stands. Plans are written by the Warmonger and run under the Marshal; each one is a numbered checklist with verification gates. Roadmap: [[Timeline]]. Session history: [[Sessions|Session-Index]].

## Current (2026-10-16)

| Plan | What | Status |
|------|------|--------|
| [WELL-ERDA-AUDIT-2026-10-13](../docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md) | The Well: 25 audit findings → ledger, roles, TLS, backups, offsite, lint ([[The-Well]], [[Well-Backups]]) | Done except time-gated items (5.3 re-run health after 2026-11-15, 5.4 partition at 1M rows / 1 GB, 5.7 types) |
| [COMPLIANCE-WELL-BATTLE-PLAN](../docs/battle-plans/COMPLIANCE-WELL-BATTLE-PLAN.md) | Compliance state into The Well (findings, evidence, journal), ADR-097–099 | 48 steps done, 43 open (2026-10-01) |
| [STAGING-PROMOTION-LOG](../docs/battle-plans/STAGING-PROMOTION-LOG.md) | `develop` → `staging` promotion batches and the definition of done for `main` | Last promotion 2026-09-24 |
| [STAGING-LADDER-2026-08-03](../docs/battle-plans/STAGING-LADDER-2026-08-03.md) + [PROGRESS](../docs/battle-plans/STAGING-LADDER-PROGRESS.md) / [DECISIONS](../docs/battle-plans/STAGING-LADDER-DECISIONS.md) | Lint and security ratchets to an empty exclude list | Ladder complete (phases 0–15) |
| [UPC-LINUX-MASTER-BATTLE-PLAN](../docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md) | Linux on the UPC: harness, loader unification, owned kernel ([[Linux-on-UPC]]) | Epics 1.1, 1.2.1, 1.2.3 done; 1.2.2 and 1.2.4 partial |
| [EVOLUTION-1-TRAP-FLIP](../docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md) | Waking the owned kernel's syscall path (hybrid syscalls, getpid probe) | Stage 1: S1.0/S1.1 green; S1.2 blocked on an MBC codegen bug (`csrw sscratch` in `prepare_return`), flip deferred |

## Completed UPC phases

[PHASE17-GATE-D-FS-READER](../docs/battle-plans/PHASE17-GATE-D-FS-READER.md) (FS reader, `ls`/`cat`/argv) · [PHASE22-SH-REPLACE](../docs/battle-plans/PHASE22-SH-REPLACE.md) (our own shell) · [PHASE23-KERNEL-EDGES](../docs/battle-plans/PHASE23-KERNEL-EDGES.md) · [PHASE24-KERNEL-CORE](../docs/battle-plans/PHASE24-KERNEL-CORE.md) (kernel 100% Unheaded-authored, 2026-07-07).

## Earlier waves (2026-04 → 05)

WAVE13–17 (Zhen inference, retrain, rewire, overnight model vetting, K8s substrate), the 2026-04-27 sprints, and the 2026-04-30 Round Tables (practical tooling; the community-first doctrine amendment). See the directory.

## The S33 Round Table plan (2026-03, historical)

[battle-plan.md](../docs/battle-plans/battle-plan.md): the unified plan from all Kingdom skills at S33. Wotan's triple role (ring buffer + event bus + protocol RAM), DNS-hierarchy cluster redundancy, hardening sprint priorities, post-Doom workstreams, origin-story integration.

---

> **Source:** [docs/battle-plans/](../docs/battle-plans/) · `~/tmp/next.md`
