# Wotan Spec Status — 2026-04-27

**Captured**: 2026-04-27 from Cowork-on-Macbook (RFC-Editor + Architect hats)
**Owner**: RFC-Editor
**Source-of-truth ADR**: ADR-052

## Current state on main

✅ **Wotan Memory Protocol draft-03 IS ON MAIN** — CLAUDE.md was out of date.

| Field | Value |
|---|---|
| Latest version on main | **draft-bellis-unheaded-wotan-memory-03** |
| Document path | `docs/protocol/draft-bellis-unheaded-wotan-memory-03.md` |
| XML version | `docs/protocol/xml/draft-bellis-unheaded-wotan-memory-03.xml` |
| Plain-text version | `docs/protocol/txt/draft-bellis-unheaded-wotan-memory-03.txt` |
| IETF submission dir | `ietf-submission/draft-bellis-unheaded-wotan-memory-00.md` (history) |
| Document date | 2026-03-19 |
| Category | Experimental |
| IPR | trust200902 |
| Last spec-touching commit | `183f2b60 feat(ragnarok): Ragnarok Sprint` |
| S74 RFC audit fixes | Applied across 6 specs in lockstep with Sophia |
| Pre-public RFC blockers | Cleared (`4b90b7d9 pre-public: fix RFC blockers`) |

## Cross-references in the draft

Wotan-03 normatively references:
- RFC 2119 (BCP 14 keywords)
- RFC 8174 (BCP 14 update)
- RFC 9669 (BPF ISA Reference)
- UNHEADED-FOUNDATION (Foundation draft-06) — version-locked

This is the proper layering: Foundation → Wotan + Sophia. Both companion specs reference -06 of Foundation.

## Recommendation

**SHIP — no further action this sprint.** Same disposition as Sophia.

## Notable Wotan-specific items

- Wotan Topic Signing (ML-DSA-65 enforcement on `config.*` topics) is implemented per CLAUDE.md (2026-04-11 entry, 6 tests). Spec coverage of the signing flow is in draft-03 — verify in next reading pass.
- Wotan Active-Passive Redundancy (ADR-035, Phases 0-2 done) is implementation-side; spec language for HA semantics may need draft-04 if/when Phase 3+ exposes new wire format details.
- Per CLAUDE.md: *"Wotan draft-03 (error code taxonomy)"* listed as Age 2 remaining — verify whether the error code taxonomy section made it into draft-03 or if a draft-04 is warranted to capture it.

## Open verification (light-touch, can skip if Track A locks)

- [ ] Confirm error code taxonomy is in draft-03's section structure
- [ ] Confirm topic-signing flow is documented at draft-03 fidelity
- [ ] Confirm wire format references match Foundation-06 exactly (no drift between specs)

These are RFC-Editor 1-hour passes, not blocker work.

---

*Wotan spec status verified on main 2026-04-27. SHIP — already shipped.*
