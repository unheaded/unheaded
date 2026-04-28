# Remote-Handoff Index — Sprint 2026-04-27

**Captured**: 2026-04-27 from Cowork-on-Macbook (Phase 8 sprint exit)
**HEAD at sprint exit**: see git log; sprint commits prefixed `[PLAN SPRINT-04-27 LOCAL]`
**Audience**: future Stevie at WEST/EAST/Linux dev box

This sprint shipped paper. The Linux box ships steel. Three work packets queue up for the next time Stevie is at a box with full toolchain + GPU + hardware. Each packet is self-contained — pick whichever the next box can run.

---

## Packet 1 — WAVE13 Phase 2 Quality Run (HIGHEST UNBLOCK VALUE)

**Path**: `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md`
**Result skeleton**: `crates/zhenai-forge/notes/wave13-phase2-quality.md`
**ADR to finalize**: `docs/adr/ADR-051-wave13-generate-path.md`

| Field | Value |
|---|---|
| Prerequisites | GPU dev box, gemma-4 GGUF (~9.3GB), Kingdom LoRA, eval JSONL, gemma4-venv |
| Estimated wall-clock | 2–4 hours |
| Output | win-rate number + verdict (SHIP / RETRAIN / RANK-UP / DATA-FIX) |
| Unblocks | Captain Track A/B/C decision, ADR-051 acceptance, WAVE14 scope clarification |
| Commit message template | `[PLAN SPRINT-04-27 REMOTE] CHUNK B: WAVE13 Phase 2 quality verdict + ADR-051 finalized` |

**Why this first**: Captain's recommendation in the Track call options is *conditional* on this verdict. Running it unblocks 3 downstream items.

---

## Packet 2 — Branch Hygiene Execution (LOWEST RISK)

**Path**: `docs/branch-audits/2026-04-27-summary.md` (single-pass Linux execution checklist)
**Per-branch detail**: `docs/branch-audits/2026-04-27-{claude-migrate-packages-V2Ctr,s73-public-launch,public-release-cleanup,docs-legal-planning}.md`

| Field | Value |
|---|---|
| Prerequisites | Linux dev box, push credentials |
| Estimated wall-clock | ~5 minutes |
| Output | 4 archive tags pushed, 4 stale branches deleted, content-grep flags any items missing on main |
| Unblocks | Public-launch readiness (pre-public scrub gating item flagged in branch audit) |
| Commit message template | `chore(branches): archive + delete 4 stale branches per 2026-04-27 audit (preserves history via archived/*-2026-04-27 tags)` |

**Why low risk**: archive tags push BEFORE branches delete; nothing destructive without safety net.

---

## Packet 3 — Compliance Remote Refresh (MEDIUM EFFORT)

**Path**: `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md` (8 sections A–H)
**Skeleton to fill**: `docs/security/compliance-snapshot-2026-04-27.md` (2 PENDING markers)

| Field | Value |
|---|---|
| Prerequisites | Linux dev box, syft installable, cargo-deny, go-licenses, Rust toolchain |
| Estimated wall-clock | 30–60 minutes |
| Output | SBOM regen (delta vs CLAUDE.md baseline 553), cargo-deny advisories+licenses report, go-licenses CSV, 6 Go SPDX backfills |
| Unblocks | Public-launch compliance posture, future drift on SPDX coverage in Go |
| Commit message template | `[PLAN SPRINT-04-27 REMOTE] Lane E: SBOM + license scan + threat register + 6 Go SPDX backfill` |

**Note**: KEV pull was already completed in this sprint via Stevie's manual upload assist. Re-running KEV from a non-proxied box is informational only (the snapshot at `docs/security/feeds/cisa-kev-2026-04-27.json` is canonical for this sprint).

---

## Optional Packet 4 — CLAUDE.md "Age 2 remaining" cleanup

**Why it's here**: Phase 7 spec sweep discovered CLAUDE.md still lists Sophia draft-03 + Wotan draft-03 as "Age 2 remaining" — but they're on main. Also Foundation-06 is treated inconsistently. Quick cleanup pass.

| Field | Value |
|---|---|
| Prerequisites | None (any box; pure doc edit) |
| Estimated wall-clock | ~10 minutes |
| Output | CLAUDE.md "Age 2 remaining" section accurately reflects main state |
| Cross-reference | `docs/specs/2026-04-27-sophia-status.md`, `docs/specs/2026-04-27-wotan-status.md` |
| Commit message template | `docs(claude): correct Age 2 remaining list — Sophia/Wotan draft-03 already shipped` |

This can run from anywhere including this Cowork session if Stevie wants — flagged here because it relates to spec sweep findings.

---

## Sequencing recommendation

If Stevie has 30 minutes: **Packet 2** (branches) — quickest win, unblocks public scrub story.
If Stevie has 1 hour: **Packet 2 + Packet 4** — branches + CLAUDE.md cleanup.
If Stevie has half a day: **Packets 1 + 2 + 4** — verdict + branches + cleanup, leaves Packet 3 for next session.
If Stevie has a full day: **All packets** — full follow-through.

---

## Cross-references

- Sprint mini-Round-Table block in `battle-plan.md` (sprint-exit summary)
- ADR-052 — drift-guard CI will pass on each remote commit since timeline.md was refreshed in this sprint
- ADR-053 — Hybrid Claude+Zhenai routing; remote work routes via Claude (HEAVY classifier), not local Zhenai

---

*Remote-handoff index forged 2026-04-27 sprint-exit from Cowork-on-Macbook. Three packets queued; soldiers fight when the box is ready.*
