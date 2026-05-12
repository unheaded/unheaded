# Wotan Memory Protocol draft-04 — Ship-or-Defer Recommendation

**Date:** 2026-05-06
**Author:** Marshal (overnight unattended run, Appendix A Step A4)
**Lane:** RFC Editor (analysis only — Marshal does not author specs)
**Reference status snapshot:** `docs/specs/2026-04-27-wotan-status.md`
**RATIFIED:** Stevie 2026-05-11 — DEFER accepted. Re-open only on the flip conditions in §"What would flip this to ship draft-04" below.

---

## Recommendation: **DEFER draft-04** ✅ RATIFIED 2026-05-11

Same disposition as Sophia (see `docs/specs/sophia-04-shipdefer-2026-05-06.md`). The 2026-04-27 status snapshot recommended SHIP for draft-03 — already shipped — and identified three light-touch verification items that are 1-hour RFC-Editor passes, not blockers. Nothing in the 9 days since has changed that picture.

---

## Why defer

1. **draft-03 is already on main** (`docs/protocol/draft-bellis-unheaded-wotan-memory-03.md`) with XML and txt renders.
2. **Pre-public RFC blockers cleared** (`4b90b7d9 pre-public: fix RFC blockers`).
3. **S74 RFC audit closed** (lockstep with Sophia; same commits).
4. **Foundation-06 is the upstream lock** — Wotan-03 normatively references Foundation-06; Foundation is frozen, so Wotan has no upstream movement to chase.
5. **Topic-signing implementation has shipped** (ML-DSA-65 on `config.*` topics, 6 tests, per CLAUDE.md 2026-04-11 entry). Spec coverage exists in draft-03 — daytime RFC-Editor verification recommended but not draft-blocking.
6. **Wotan Active/Passive HA** (ADR-035) Phases 0-2 are implementation-side; ADR-064 active/active 3-node spec is **deferred per Stevie**. No spec movement until ADR-064 implementation lands, which is post-Track-call.

---

## What would flip this to "ship draft-04"

- ADR-064 active/active implementation begins and exposes wire-format gaps for HA semantics → spec language needs to catch up.
- Foundation un-freezes (low probability).
- Error-code taxonomy verification finds the section is missing or insufficient at draft-03 (would warrant a -04 specifically for that section).
- IETF datatracker feedback on draft-03.
- Captain Track-call lands on Track B/C with explicit "submit Wotan to IETF" action.

---

## Light-touch open verifications (carry-forward — not for tonight)

The 2026-04-27 status flagged three items the Marshal also will NOT close tonight (RFC-Editor authoring lane, not Marshal lane):

- [ ] Confirm error-code taxonomy section is present and complete in draft-03.
- [ ] Confirm topic-signing flow is documented at draft-03 fidelity.
- [ ] Confirm wire-format references match Foundation-06 exactly (cross-spec drift check).

These are recommended for the next RFC-Editor session (≈ 1 hour total). Not draft-blocking.

---

## What to do tonight

Nothing. **The Marshal will not edit any spec content.** This file is the recommendation only.

---

## Cross-references

- `docs/specs/2026-04-27-wotan-status.md` — last RFC-Editor / Architect status (still valid 9 days later)
- `docs/protocol/draft-bellis-unheaded-wotan-memory-03.md` — current shipped draft
- `docs/specs/sophia-04-shipdefer-2026-05-06.md` — sibling recommendation (handled in step A3)
- ADR-035 — Wotan Active-Passive Redundancy (Phases 0-2 implementation done)
- ADR-064 — Wotan active/active 3-node (deferred per Stevie)
- ADR-052 — spec/timeline drift policy; Wotan compliant
- NORTH-STAR battle plan 2026-05-05 — "Wotan draft-04 ship-or-defer" line 197

---

## Provenance

Read-only audit. No spec content was changed. No IETF submission attempted.
