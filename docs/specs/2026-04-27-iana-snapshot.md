# IANA Considerations Snapshot — 2026-04-27

**Captured**: 2026-04-27 from Cowork-on-Macbook
**Owner**: RFC-Editor
**Source-of-truth ADR**: ADR-052
**Foundation reference**: draft-bellis-unheaded-protocol-foundation-06 (ON MAIN)

## Monad wire format freeze status

✅ **Monad v0x01 wire format remains FROZEN.** Per CLAUDE.md S67 and verified via git log on `docs/protocol/draft-bellis-unheaded-protocol-foundation-*` and `pkg/protocol/monad/`:

Recent commits since the 2026-02-23 freeze announcement are all DOC-LEVEL (RFC audit fixes, IETF submission prep, academic tone polish, README work). No wire-format-byte-changing commits detected.

```
b7d4b83b docs(protocol): advance Foundation draft-06, Sophia draft-03, Wotan draft-03
4b90b7d9 pre-public: fix RFC blockers, rewrite README, forge S73 battle plan
bc7645c5 [S74-P1] Steps 1-10: Fix all CRITICAL and HIGH findings from RFC audit
aae6caed [S74-P2] Steps 13-21: Fix MEDIUM/LOW findings and consistency
c39ef437 [S74-P7] Steps 72-82: Academic tone enforcement across all 6 specs
61d567be feat(ietf): prepare all 6 Internet-Drafts for IETF datatracker submission
183f2b60 feat(ragnarok): Ragnarok Sprint —
825111c0 chore: battle plan template v2, opcode count fix (48), vendor genericization
058d3c14 diag(doom): CALLR instruction IS correct in ROM — executor never reaches it
0f26f0f6 feat(upc): Level 6 — bFLT binary loader
9c9b2256 feat(upc): Level 6 — atomic ops (CLI/STI/XCHG/CAS) + vfork
```

The two `feat(upc): Level 6` commits touch UPC instruction set (CLI/STI/XCHG/CAS atomics, bFLT loader) — UPC ISA is companion-spec territory (MBC-ISA-00), not Monad wire format. Freeze intact.

## 12 IANA registries from Foundation-06

Per CLAUDE.md S67, Foundation draft-06 enumerates 12 IANA registries to be created upon spec acceptance. Inventory snapshot:

| # | Registry name | Status | Notes |
|---:|---|---|---|
| 1 | Monad Protocol Version Numbers | Defined in -06 | Frozen at 0x01 (the only currently-allocated version) |
| 2 | Monad Flags Bitfield (C\|Y\|T\|E\|S\|M\|CUST\|R) | Defined in -06 | 8 flag positions documented |
| 3 | Monad Flow Actions | Defined in -06 | 13 entries enumerated |
| 4 | Kingdom Mode Values | Defined in -06 | Cross-references KINGDOM_MODE.md |
| 5 | Sophia Dictionary IDs | Defined in Sophia-03 | Sub-dictionary type registry |
| 6 | Sophia Sub-Dictionary Types | Defined in Sophia-03 | QPACK-influenced |
| 7 | Wotan Memory Region Types | Defined in Wotan-03 | LOCAL + DISTRIBUTED region semantics |
| 8 | Wotan Topic Naming Authority | Defined in Wotan-03 | `system.*`, `config.*`, `threats.*`, etc. |
| 9 | MBC ISA Opcodes | Defined in MBC-ISA-00 | 48 opcodes per `825111c0 opcode count fix (48)` |
| 10 | Shim Pipeline Stage IDs | Defined in Shim-00 | Framebuffer/scanline/compute |
| 11 | PQC Authentication Suite IDs | Defined in PQC-00 | ML-DSA-65 + SLH-DSA per FIPS 205 |
| 12 | Anamnesis Event Types | **NOT YET DEFINED** | Spec gap per `docs/specs/2026-04-27-anamnesis-status.md` — will be deferred until Anamnesis draft-00 ships |

**Net status**: 11 of 12 registries defined; Anamnesis registry blocked by missing Anamnesis spec.

## IPR clearance

Per CLAUDE.md S67: *"IPR clearance: RFC 8928/9927 CLEAR."* No outstanding IPR concerns at the wire format level. Sophia/Wotan/MBC/Shim/PQC inherit the same trust200902 IPR posture (verifiable in each draft's front matter).

## Backward compatibility audit (since freeze)

Spot check on the 11 spec-touching commits since 2026-02-23 — none are flagged as wire-format-breaking:

| Commit | Touches | Wire format breaking? | Marshal verdict |
|---|---|:---:|---|
| b7d4b83b | Foundation/Sophia/Wotan draft advances | No (doc) | OK |
| 4b90b7d9 | RFC blockers, README | No (doc) | OK |
| bc7645c5 | RFC audit CRITICAL+HIGH | No (doc fixes) | OK |
| aae6caed | RFC audit MEDIUM+LOW | No (doc fixes) | OK |
| c39ef437 | Academic tone | No (style) | OK |
| 61d567be | IETF datatracker prep | No (xml2rfc) | OK |
| 183f2b60 | Ragnarok Sprint | TBD — verify | Inspect on next pass |
| 825111c0 | MBC opcode count fix (48) | Maybe (MBC ISA) | OK if Foundation/Monad untouched |
| 058d3c14 | DOOM diag (CALLR) | No (UPC runtime) | OK |
| 0f26f0f6 | UPC Level 6 bFLT loader | No (UPC, not Monad) | OK |
| 9c9b2256 | UPC atomics + vfork | No (UPC, not Monad) | OK |

**Recommendation**: Light-touch RFC-Editor pass on `183f2b60 feat(ragnarok)` (label suggests sweeping changes — confirm no wire format regressions). 30-min review.

## Cross-references

- ADR-052 — drift policy applies; this snapshot must be re-run if any wire-format-touching commit lands
- `docs/specs/2026-04-27-sophia-status.md` — Sophia ships
- `docs/specs/2026-04-27-wotan-status.md` — Wotan ships
- `docs/specs/2026-04-27-anamnesis-status.md` — Anamnesis spec gap (registry #12 blocked)
- `docs/protocol/PROTOCOL_FOUNDATION.md` — narrative companion to Foundation-06
- CLAUDE.md S67 — original 12-registry enumeration

## Action items

- [ ] RFC-Editor 30-min review of `183f2b60` (Ragnarok Sprint) for wire-format implications
- [ ] Anamnesis spec stub Phase 1 (per `docs/specs/2026-04-27-anamnesis-status.md`) — unblocks registry #12
- [ ] When IETF datatracker submission gate opens (Track B/C), submit all 6 ready I-Ds + Anamnesis-00 stub

---

*IANA snapshot captured 2026-04-27. 11/12 registries defined. Monad freeze intact since 2026-02-23.*
