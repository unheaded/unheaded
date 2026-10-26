# Next five sprints (2026-10-23)

Brainstormed with Stevie on 2026-10-23 (Captain/Warmonger), then forged into
full battle plans. Order: 1 → 2 → 3 → 4 → 5. Each plan has its own PREFLIGHT,
gates and [ESCALATE] steps; the decisions below are the ones that need Stevie.

| # | Sprint | Plan | Size | ADR | Status (audited 2026-10-23, [SPRINT-AUDIT](SPRINT-AUDIT-2026-10-23.md)) |
|---|---|---|---|---|---|
| 1 | Quantum Armor: hybrid ML-KEM everywhere, crypto inventory, signed evidence | [S-QUANTUM-ARMOR](S-QUANTUM-ARMOR-battle-plan.md) | 15 phases, 277 steps | ADR-105 | **Done** pending Stevie's decisions (E1-E5, ADR-105 acceptance, aws-lc-rs sign-off): all 15 phases executed, nothing STUCK; final regression green. Closing summary: [progress](progress/S-QUANTUM-ARMOR-progress.md#sprint-1-closing-summary). Merged to develop 2026-10-24 |
| 2 | Two Brains, One Gate: Champion live, local model + Claude, runbooks, training-data flywheel | [S-TWO-BRAINS](S-TWO-BRAINS-battle-plan.md) | 15 phases, 456 steps | ADR-106 (Proposed) | Phases 0-11, 13, 14 done; Phase 12 waits on data and Step 400; merged to develop 2026-10-23 (3dc9dfb7) |
| 3 | Real Steel: dead code swept, deploy made real, then real chaos | [S-REAL-STEEL](S-REAL-STEEL-battle-plan.md) | 17 phases, 404 steps | ADR-107 | Phases 0, 2-4, 7, 8, 10 done; 6 code half only; 1, 5, 9, 11-14 wait on E1/E2; merged to develop 2026-10-23 (896a6ed4) |
| 4 | Lich Awakens: red team via MCP (local model + Claude) | [S-LICH-AWAKENS](S-LICH-AWAKENS-battle-plan.md) | 12 phases, 268 steps | none new | **Done**: all 12 phases; merged to develop (21874778, pushed) |
| 5 | UPC: finish pending work, then compute on the wire (MBC cartridges) | [S-UPC-WIRE-COMPUTE](S-UPC-WIRE-COMPUTE-battle-plan.md) | 20 phases, 452 steps | ADR-108 | **Done** pending Stevie's decisions: Phases 0–5, 9–17 and 19 executed; Phases 6–8 and 18 DEFERRED (east as a node, Steps 177/435, never contacted); getpid flip held (pid numbering); ADR-108 Proposed (RFC Editor items 1–5 open); benchmark gates #1(a) and #2 fail on interpreter cost, scorer trained on synthetic flows only. Closing summary: [log](../../references/s5-upc-wire-log.md#sprint-5-closing-summary). Merged to develop 2026-10-24 |

Done before forging (2026-10-23, on develop): `.mcp.json` registers the Zhen
MCP server for Claude Code; the `local_llm` MCP tool; `zhen-claude -route
auto`, `-capture`, and `-local-only` / `local_only` in
`~/.config/unheaded/zhen-claude.conf`.

## Found while grounding

- Wotan pins `CurvePreferences` to X25519/P256, which turns off Go's hybrid
  `X25519MLKEM768`; the WAF and zhend use rustls' ring provider (no ML-KEM);
  Postgres 16 has one key-exchange group (hybrid needs 18); internal mTLS
  still accepts TLS 1.2; Gungnir seals expire within 31 days.
- Every Yaldabaoth injector, `pkg/deploy`'s stages and strategies, Sword's
  rollback and Canary's metrics are simulated.
- Foundation-06 registers `EVENT_CHAOS` 0x04; the code uses 0x05
  (`EVENT_ANOMALY` 0x08 vs 0x04); shield-ebpf reuses the CHAOS flag.
- The Champion's confirm flow returns the confirmation token to its caller,
  which in a model loop is the model.
- zhenai-forge still drops unparseable training lines silently and falls back
  to the CLI `answer_start` (the H6 bug class).

## Decisions taken 2026-10-26 (Stevie)

- Sprint 3 E1: apply every default in `docs/sweeps/REAL-STEEL-DECISIONS.md`; ADR-090 Accepted.
- Sprint 3 E2/E3: west live windows (Cuirass on real LXD, live deploy, netem + container-kill chaos, lab netns) and the east chaos agent + polkit rule, all granted.
- Sprint 3 E4: spec wins; the code moves to foundation-06's EVENT_CHAOS 0x04 / EVENT_ANOMALY 0x08; no draft edit.
- Sprint 2 Phase 12: closed as DEFERRED-on-data (0 SFT train examples, 300 needed; no synthetic or Claude padding). Trigger: >= 300 SFT train examples across >= 3 families -> run Phase 12.

## Decisions for Stevie, by sprint

1. Quantum Armor (sprint done; these are what is left, all prepared):
   - E1: live Well PostgreSQL 16 → 18 with `ssl_groups` (FND-015;
     `runbooks/data/well-pg18-upgrade.yaml`).
   - E2: sshd hybrid-only on west/east, lockout risk (UH-PQC-03;
     `docs/security/pq-host-escalations.md`).
   - E3: first live WireGuard PSK rotation and the weekly timer (FND-016).
   - E4: where the production evidence build key lives, who holds it, how
     `build.pubkey` is published (FND-018).
   - E5: Heimdall drift key per node, enrolment in Wotan, a Heimdall unit
     (FND-019).
   - Accept ADR-105 (Proposed).
   - Sign off the `aws-lc-rs` / `aws-lc-sys` admission (ADR-004).
   - Sign off the `anthropic-sdk-go` admission (ADR-004, ADR-104).
2. Two Brains: apply Well migrations 030-034 as one batch; first unattended
   overnight run and its cap; approve the 11 new runbooks; Anthropic support
   on Claude-authored text as training targets (flag off until a written yes);
   when to run the LoRA (takes the GPU for hours).
3. Real Steel: one delete/keep table for the ADR-090 sweep; a west live
   window (Cuirass LXD trust, images, units); east chaos agent and polkit
   scope; spec vs code for `EVENT_CHAOS`.
4. Lich Awakens: hard stops only (gate or cap missing, sandbox not confined,
   a safety invariant bypassed); CTF mode stays gated by ADR-062's rule.
5. UPC (sprint done; the combined list is in the closing summary of
   `references/s5-upc-wire-log.md`):
   - East as a node: Mímir's Law Phases 9/11/13 re-run (Step 177) and the
     DISTRIBUTED second node (Step 435).
   - getpid: which pid numbering wins (xv6 `p->pid` vs the BPF slot) before
     `GETPID_INJECT` can default on.
   - Accept ADR-108 once the RFC Editor answers items 1–5 (Sophia root key,
     Anamnesis sub-kind, Wotan topics, CSR range, KF10).
   - DOT4 or any other new MBC opcode (ISA spec gate, its own sprint).
   - Whether the interpreter-cost gate failures (#1(a), #2) are accepted as
     the cost of interpreted cartridges or need follow-on work.
   - Whether the scorer gets real labelled traffic before anyone relies on it.

## Parked

zhend Layer 0 (QUIC server, gossip), Sabatons IPMI/Redfish.
