# Next five sprints (2026-10-23)

Brainstormed with Stevie on 2026-10-23 (Captain/Warmonger), then forged into
full battle plans. Order: 1 → 2 → 3 → 4 → 5. Each plan has its own PREFLIGHT,
gates and [ESCALATE] steps; the decisions below are the ones that need Stevie.

| # | Sprint | Plan | Size | ADR | Status (2026-10-23) |
|---|---|---|---|---|---|
| 1 | Quantum Armor: hybrid ML-KEM everywhere, crypto inventory, signed evidence | [S-QUANTUM-ARMOR](S-QUANTUM-ARMOR-battle-plan.md) | 15 phases, 277 steps | ADR-105 | **Done** pending Stevie's decisions (E1-E5, ADR-105 acceptance, aws-lc-rs sign-off): all 15 phases executed, nothing STUCK; final regression green. Closing summary: [progress](progress/S-QUANTUM-ARMOR-progress.md#sprint-1-closing-summary). Merged to develop 2026-10-24 |
| 2 | Two Brains, One Gate: Champion live, local model + Claude, runbooks, training-data flywheel | [S-TWO-BRAINS](S-TWO-BRAINS-battle-plan.md) | 15 phases, 456 steps | ADR-106 | In progress on branch `sprint/two-brains` (not merged) |
| 3 | Real Steel: dead code swept, deploy made real, then real chaos | [S-REAL-STEEL](S-REAL-STEEL-battle-plan.md) | 17 phases, 404 steps | next free | In progress on branch `sprint/real-steel` (not merged) |
| 4 | Lich Awakens: red team via MCP (local model + Claude) | [S-LICH-AWAKENS](S-LICH-AWAKENS-battle-plan.md) | 12 phases, 268 steps | none new | Not started (no branch in the 2026-10-23 handoff) |
| 5 | UPC: finish pending work, then compute on the wire (MBC cartridges) | [S-UPC-WIRE-COMPUTE](S-UPC-WIRE-COMPUTE-battle-plan.md) | 20 phases, 452 steps | ADR-108 | **Done** pending Stevie's decisions: Phases 0–5, 9–17 and 19 executed; Phases 6–8 and 18 DEFERRED (east as a node, Steps 177/435, never contacted); getpid flip held (pid numbering); ADR-108 Proposed (RFC Editor items 1–5 open); benchmark gates #1(a) and #2 fail on interpreter cost, scorer trained on synthetic flows only. Closing summary: [log](../../references/s5-upc-wire-log.md#sprint-5-closing-summary). On branch `sprint/upc-wire-compute` (not merged) |

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

## Decisions for Stevie, by sprint

1. Quantum Armor: accept ADR-105; Postgres 16 → 18 with `ssl_groups`; sshd
   hybrid-only on west/east (lockout risk); first live WireGuard PSK
   rotation; where the evidence-signing key lives; Heimdall per-node keys.
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
