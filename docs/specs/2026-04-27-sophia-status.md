# Sophia Spec Status — 2026-04-27

**Captured**: 2026-04-27 from Cowork-on-Macbook (RFC-Editor + Architect hats)
**Owner**: RFC-Editor
**Source-of-truth ADR**: ADR-052 (drift policy applies to specs as much as timeline.md)

## Current state on main

✅ **Sophia Dictionary draft-03 IS ON MAIN** — CLAUDE.md was out of date listing this as "Age 2 remaining."

| Field | Value |
|---|---|
| Latest version on main | **draft-bellis-unheaded-sophia-dictionary-03** |
| Document path | `docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md` |
| XML version | `docs/protocol/xml/draft-bellis-unheaded-sophia-dictionary-03.xml` |
| Plain-text version | `docs/protocol/txt/draft-bellis-unheaded-sophia-dictionary-03.txt` |
| IETF submission dir | `ietf-submission/draft-bellis-unheaded-sophia-dictionary-00.md` (older, kept for IETF datatracker history) |
| Document date | 2026-03-19 |
| Category | Experimental |
| IPR | trust200902 |
| Submission type | Independent |
| Last spec-touching commit | `183f2b60 feat(ragnarok): Ragnarok Sprint` |
| S74 RFC audit fixes | Applied (CRITICAL + HIGH per `bc7645c5`, MEDIUM/LOW per `aae6caed`, academic tone per `c39ef437`) |
| IETF datatracker prep | Done (`61d567be feat(ietf): prepare all 6 Internet-Drafts`) |

## Recommendation

**SHIP — no further action this sprint.** draft-03 is publishable in its current form.

The actual outstanding question is *should we submit draft-03 to the IETF Independent Submission stream?* That's a Captain Track decision (Track B/C makes IETF visibility valuable for public-launch optics; Track A makes it a defer-able polish item).

## Cross-references

- ADR-052 — drift policy; if Sophia spec content changes, the spec file mtime + corresponding section of `references/timeline.md` must stay in sync
- Foundation-06 — Sophia normatively references Foundation; both are version-locked at this snapshot
- Wotan-03 — Sophia + Wotan + Foundation move together; see `docs/specs/2026-04-27-wotan-status.md`

## What we do NOT need to do

- ~~Author Sophia draft-03~~ — already on main
- ~~Audit Sophia draft-02 → 03 deltas~~ — done in S74 sweep
- ~~Convert to RFC xml2rfc~~ — XML version present (`docs/protocol/xml/`)

## What we MIGHT do (Track-call dependent)

- Submit to IETF datatracker if Track B or C unlocks public visibility focus
- Begin drafting draft-04 if WAVE13 Phase 2 forge work surfaces wire-format implications (low probability — forge runs over Wotan, not Sophia)

---

*Sophia spec status verified on main 2026-04-27. SHIP — already shipped.*
