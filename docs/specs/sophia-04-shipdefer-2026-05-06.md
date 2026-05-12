# Sophia Dictionary draft-04 — Ship-or-Defer Recommendation

**Date:** 2026-05-06
**Author:** Marshal (overnight unattended run, Appendix A Step A3)
**Lane:** RFC Editor (analysis only — Marshal does not author specs)
**Reference status snapshot:** `docs/specs/2026-04-27-sophia-status.md`
**RATIFIED:** Stevie 2026-05-11 — DEFER accepted. Re-open only on the flip conditions in §"What would flip this to ship draft-04" below.

---

## Recommendation: **DEFER draft-04** ✅ RATIFIED 2026-05-11

No new evidence has surfaced between 2026-04-27 (last status snapshot) and 2026-05-06 (today, T+9 days) that justifies cutting a Sophia draft-04. The 2026-04-27 status was unambiguous: "**SHIP — already shipped**" — meaning draft-03 is the current publishable artifact and no draft-04 work is queued.

---

## Why defer

1. **draft-03 is already on main** (`docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md`, 979 lines) with matching XML and txt renders.
2. **S74 RFC audit is closed** — CRITICAL+HIGH (`bc7645c5`), MEDIUM+LOW (`aae6caed`), academic tone (`c39ef437`). Nothing outstanding from that audit.
3. **Wire format is FROZEN at v0x01** — Sophia normatively references Foundation; Foundation is locked, so Sophia has no upstream movement to chase.
4. **Forge research thread (WAVE13/14) does not touch Sophia.** The 2026-04-27 status flagged "low probability" of forge surfacing Sophia wire-format implications. WAVE17 (K8s substrate, 2026-05-04→05) and the new forge consolidation work confirm the substrate moved forward without Sophia changes.
5. **Captain Track-call is still pending.** The 2026-04-27 status correctly identified IETF Independent Submission as a Track-B/C-dependent decision — and the Track-call has slipped further (S3 citation against Captain in NORTH-STAR plan). Cutting draft-04 ahead of the Track-call adds a publication artefact whose audience is undecided.
6. **ADR-052 drift policy**: spec mtime and `references/timeline.md` are in sync. Cutting a new draft-04 today would force a corresponding timeline update for no shipping reason.

---

## What would flip this to "ship draft-04"

Any of the below — none have happened:

- Foundation spec un-freezes or an erratum lands that Sophia references.
- WAVE13/14 forge work uncovers a Sophia wire-format gap (e.g. dictionary handshake, sub-dictionary types).
- IETF datatracker feedback on draft-03 demands changes (would justify -04 as a response document).
- Captain calls Track A or Track B and explicitly requests a public-launch IETF submission, in which case the more relevant question is "submit draft-03" — not "cut draft-04".

---

## What to do tonight

Nothing. **The Marshal will not edit any spec content.** This file is the recommendation only.

The 2026-04-27 status snapshot stands as the authoritative current view.

---

## Cross-references

- `docs/specs/2026-04-27-sophia-status.md` — last RFC-Editor / Architect status (still valid 9 days later)
- `docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md` — current shipped draft
- `docs/specs/2026-04-27-wotan-status.md` — sibling status (handled in step A4)
- ADR-052 — spec/timeline drift policy (≤ 7 days); Sophia compliant
- NORTH-STAR battle plan 2026-05-05 — "Sophia draft-04 ship-or-defer" line 196 of `references/battle-plan-NORTH-STAR-2026-05-05.md`

---

## Provenance

Read-only audit. No spec content was changed. No IETF submission attempted.
