# Amber IP Audit Report

**Project:** Unheaded
**Date:** March 25, 2026
**Scope:** Pre-public-launch IP clearance per ADR-69420 addendum
**Auditor:** Claude Code Agent (Opus 4.6, 1M context)
**Supersedes:** March 19, 2026 audit (Haiku 4.5)

---

## Verdict: CLEAR TO SHIP

No Zelazny-copyrighted character names appear as exported Go identifiers, compiled binary names, public-facing documentation branding, or IETF Internet-Draft content. All Amber references are confined to internal lore documents and one fiction piece, both of which constitute transformative fair use commentary with attribution.

---

## Audit Methodology

Searched all `.go` and `.rs` source files, all Markdown documentation, all IETF submission artifacts (`.md`, `.txt`, `.xml`), README.md, QUICKSTART.md, and binary naming conventions for the following Zelazny-specific names:

**Character names:** Corwin, Dworkin, Oberon, Brand, Merlin, Fiona, Random, Julian, Gerard, Caine, Bleys, Flora, Deirdre, Llewella, Benedict, Eric

**Concepts/artifacts:** Logrus, Trumps, Ghostwheel, Tir-na Nog'th, Rebma

**Exclusions (generic mythology, not Zelazny):** Monad, Sophia, Wotan, Anamnesis, Yaldabaoth, Pleroma, Kenoma -- these derive from Gnostic/Norse traditions.

**Exclusions (generic English):** "Pattern" and "Shadow" are only flagged if used as branded product terms. "Random" is standard CS terminology.

---

## 1. Exported Go Identifiers

**Status: PASS**

Grep of all `.go` files for the 16 character names and 5 concept names returned zero hits for Zelazny-specific identifiers. The only name overlap is "Random", which appears exclusively as standard load-balancing terminology (`RandomLoadBalancer`, `AlgorithmRandom`, `RandomState`, `NewRandom`, `GenerateRandomBytes`, etc.) across `pkg/loadbalancer/`, `pkg/mesh/`, `pkg/scheduler/`, `pkg/dns/`, `pkg/secrets/`, and `services/gateway/proxy/`. These are generic computer science terms with no connection to the Amber character Prince Random.

No Zelazny names appear as struct names, function names, package names, interface names, or exported constants.

## 2. Rust Source (eBPF Programs)

**Status: PASS**

Grep of all `.rs` files returned zero hits for any Zelazny character or concept name.

## 3. Compiled Binary Names

**Status: PASS**

Reviewed `/docs/lore/binary-lore-names.md`. The 16 binaries and 11 armor services draw names from three pillars:

- **Medieval Armory:** visor, vambraces, herald, sigil, codex, shield, sword, cape, cloak, hauberk, cuirass, gauntlets, pauldrons, tassets, sabatons, pennant
- **Norse Mythology:** wotan, gungnir, bifrost, ragnarok, mjolnir (all public domain)
- **Gnostic Theology:** monad, sophia, pleroma, kenoma, yaldabaoth (all public domain, pre-Christian era)
- **Greek Mythology:** aegis (public domain)

Zero Zelazny character names. Functional names (dashboard-backend, trace-collector-go, etc.) serve as primary distribution identifiers; lore names are aliases.

## 4. Public-Facing Documentation

**Status: PASS**

- **README.md:** No mention of Zelazny, Amber, or any character name. Uses only generic terms (Monad, Sophia, Wotan, UPC).
- **QUICKSTART.md:** No mention of Zelazny, Amber, or any character name. Purely technical setup instructions.
- **docs/lore/binary-lore-names.md:** Header references "Norse/Chronicles of Amber mythology" as a naming pillar, but no Amber character names are actually used as binary names. The reference is descriptive context, not branding.

## 5. IETF Internet-Draft Specifications

**Status: PASS**

Searched all 6 current specifications in both `docs/protocol/` (latest drafts) and `ietf-submission/` (submission-ready artifacts):

| Specification | Latest Draft | Amber References |
|---|---|---|
| Protocol Foundation | draft-06 | None |
| Sophia Dictionary | draft-03 | None |
| Wotan Memory | draft-03 | None |
| MBC ISA | draft-00 | None |
| Shim | draft-00 | None |
| PQC Authentication | draft-00 | None |

**One finding in an older draft:** `docs/protocol/draft-bellis-unheaded-protocol-foundation-02.md` (superseded) contains a Zelazny acknowledgment in an Acknowledgments section at line 1372. This was removed by draft-03 and is not present in the current draft-06 or any IETF submission artifact. **No action required** -- older drafts are historical records, not submission candidates.

## 6. Fiction: the_first_packet.md

**Status: PASS (Fair Use)**

`/docs/protocol/the_first_packet.md` (479 lines) is a creative fiction piece that uses Amber cosmology as extended metaphor for protocol architecture. It references:

- **Corwin** -- as metaphor for walking the Pattern/protocol
- **Dworkin** -- as metaphor for discovering (not inventing) the Pattern
- **Merlin** -- as architectural archetype bridging Pattern and Logrus
- **Brand** -- as adversary archetype (traitor prince mapped to threat model)
- **Ghostwheel** -- as AI/autonomy metaphor mapped to Zhen
- **Logrus** -- as chaos counterpart mapped to Yaldabaoth
- **Trumps** -- mentioned in passing

**Fair use analysis (17 U.S.C. 107):**

1. **Purpose and character:** Transformative. The piece maps Amber concepts to IPv6 protocol architecture as technical allegory. It does not retell Zelazny's plots, reproduce dialogue, or recreate character arcs. The protagonist is "Muck" (original character), not any Amber character.
2. **Nature of the copyrighted work:** Literary fiction. The First Packet is technical commentary in a different medium and domain.
3. **Amount used:** Conceptual frameworks only (Pattern, Shadow, Walking). No substantial taking of plot, dialogue, or character development.
4. **Market effect:** None. Does not compete with Amber books, illustrated editions, or the Colbert TV adaptation. If anything, promotes interest in the source material.

**Existing attribution (end of document):**
> *With respect to Roger Zelazny (1937-1995), who understood that the one true reality casts infinite shadows, and that walking the Pattern is always worth the fire.*

**Recommendation:** Add a brief fair use notice below the existing attribution:

```
This document is transformative commentary on network protocol design;
it does not reproduce or retell Zelazny's narrative works. Chronicles of
Amber character names and concepts remain property of the Zelazny estate.
```

## 7. Internal Lore Documents (ADR-69420, Skill Updates, Archives)

**Status: PASS (Fair Use / Internal)**

Amber character-to-component mappings appear in:

- `/docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` -- Lore Pillar 2 mapping table (Corwin=Wotan, Brand=BlackMage, Merlin=UPC, etc.)
- `/skills/unheaded-lore/SKILL.md` -- Lore reference for AI agents
- `/skills/unheaded-kingdom-PROTOCOL-UPDATE.md` -- Protocol update reference
- `/skills/unheaded-architect-SKILL-UPDATE.md`, `/skills/unheaded-captain-SKILL-UPDATE.md` -- Agent skill files
- `/references/timeline.md`, `/references/TIMELINE_APPENDIX_FEB18.md` -- Historical session notes
- `/docs/archive/` -- Archived versions of the above

These are internal architectural documentation that use Amber as a conceptual framework. ADR-69420 already contains explicit IP awareness:
> *"The Zelazny estate is actively managed by Zeno Agency, with recent illustrated editions (2025) and a TV adaptation in development with Stephen Colbert. This is NOT abandoned IP."*

Key point: None of these mappings leak into exported code, binary names, or IETF specs. The lore layer is documentation-only.

---

## Summary

| Category | Status | Amber Names Found | Action Required |
|---|---|---|---|
| Go exported identifiers | PASS | 0 | None |
| Rust source (eBPF) | PASS | 0 | None |
| Compiled binary names | PASS | 0 | None |
| README / QUICKSTART | PASS | 0 | None |
| IETF specs (current) | PASS | 0 | None |
| IETF specs (old drafts) | PASS | 1 (removed in draft-03) | None |
| the_first_packet.md | PASS (Fair Use) | Multiple (commentary) | Add fair use notice (recommended) |
| Internal lore docs | PASS (Fair Use) | Multiple (mappings) | None (already has IP notice) |

---

## Recommended Fair Use Notice for the_first_packet.md

Append after the existing Zelazny attribution at the end of the document:

```markdown
*The Chronicles of Amber (1970-1991) and all characters, concepts, and
terminology therein are the property of the Zelazny estate, managed by
Zeno Agency. This document is transformative commentary that maps
literary concepts to network protocol architecture; it does not
reproduce or retell Zelazny's narrative works.*
```

---

## Conclusion

The Unheaded repository is clear for public release with respect to Zelazny/Amber IP. The project correctly isolates Amber references to internal lore documentation and one fiction piece (both fair use), while keeping all code, binaries, public docs, and IETF specs free of copyrighted names. The single recommended action is adding a brief fair use notice to `the_first_packet.md`, which already carries attribution.

---

**Audit completed:** March 25, 2026
**Auditor:** Claude Code Agent (Opus 4.6, 1M context)
**Method:** Systematic ripgrep of all .go, .rs, .md, .txt, .xml files in repository
