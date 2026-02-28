# S67 RFC OVERLAP REMEDIATION BATTLE PLAN — 7 Phases, 68 Steps

**Date**: 2026-02-27
**Sprint**: S67 — IANA Flag Registration, IPR Search, Zero-Deployment Wire Format Window
**Prerequisite**: RFC overlap analysis complete (Barrister + RFC Editor + Scientist triple-lens, this session)
**Target**: Monad flags registered with IANA, RFC 8928 IPR cleared, all breaking wire format changes identified and committed before first external implementation
**Estimated Duration**: 6-10 hours across 2-3 sessions
**Agent Strategy**: Phases 1-2 sequential (intelligence). Phases 3-4 parallelizable. Phase 5 sequential (depends on 3+4). Phase 6 parallelizable with 5. Phase 7 sequential (final gate).
**Commit Cadence**: Every 4 steps (max(3, min(5, 68/20)) = 4)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed attempts. Log STUCK marker. Move forward.

---

## LEGEND

[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK step

---

## PHASE 0: INTELLIGENCE GATHERING (Steps 1-8)

**Goal**: Confirm current state of all Unheaded I-Ds, IANA touchpoints, and wire format frozen/unfrozen status.
**Prerequisite**: Access to unheaded repo and RFC reference docs.
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 1** [R] ~1m: Read current Monad wire format spec
  ```bash
  cat /sessions/nice-blissful-dirac/mnt/tmp/draft-bellis-unheaded-monad-extended-register-00.md | head -200
  ```

- [ ] **Step 2** [R] ~1m: Read protocol summary for current field offsets and flag definitions
  ```bash
  cat /sessions/nice-blissful-dirac/mnt/.skills/skills/unheaded-rfceditor/references/unheaded-protocol-summary.md
  ```

- [ ] **Step 3** [R] ~1m: Read IANA guide for existing registry definitions
  ```bash
  cat /sessions/nice-blissful-dirac/mnt/.skills/skills/unheaded-rfceditor/references/iana-guide.md
  ```

- [ ] **Step 4** [B] ~1m: Check git log for any recent wire format changes
  ```bash
  cd /sessions/nice-blissful-dirac/mnt/tmp/unheaded && git log --oneline -20
  ```

- [ ] **Step 5** [R] ~2m: Inventory all files that define or reference the Monad flags byte
  ```bash
  grep -rl "flags\|K0\|K1\|Kingdom Mode\|0x01.*flags" /sessions/nice-blissful-dirac/mnt/tmp/unheaded --include="*.go" --include="*.rs" --include="*.md" 2>/dev/null | head -30
  ```

- [ ] **Step 6** [R] ~2m: Inventory all files that reference trace_id or flow_label immutability
  ```bash
  grep -rl "trace_id\|flow_label\|NEVER modify\|immutable" /sessions/nice-blissful-dirac/mnt/tmp/unheaded --include="*.go" --include="*.rs" --include="*.md" 2>/dev/null | head -30
  ```

- [ ] **Step 7** [R] ~1m: Check if any IANA registration documents already exist
  ```bash
  find /sessions/nice-blissful-dirac/mnt/tmp/unheaded -name "*iana*" -o -name "*IANA*" -o -name "*registration*" 2>/dev/null | head -10
  ```

- [ ] **Step 8** [V] ~1m: **PHASE 0 EXIT GATE** — Intelligence gathered
  - Confirm: Monad flags byte layout known (C|Y|T|E|S|M|K1|K0)
  - Confirm: All three I-D statuses known (foundation -03, sophia -00, wotan -00)
  - Confirm: No prior IANA registration docs exist (expected: none)
  - If all confirmed → Phase 1
  - If any unknown → resolve before proceeding

---

## PHASE 1: IETF IPR DATABASE SEARCH — RFC 8928 (Steps 9-20)

**Goal**: Clear RFC 8928 (AP-ND) and RFC 9927 (EARO C-Flag) of patent encumbrances before any crypto-identity feature work.
**Prerequisite**: Phase 0 complete.
**Time**: 60 minutes
**Agent**: Coordinator (requires web search + legal analysis)

- [ ] **Step 9** [B] ~3m: Search IETF IPR database for RFC 8928 disclosures
  - Web search: `site:datatracker.ietf.org/ipr rfc8928`
  - Also search: `site:datatracker.ietf.org/ipr "address protection" "6LoWPAN"`

- [ ] **Step 10** [B] ~3m: Search IETF IPR database for RFC 9927 disclosures
  - Web search: `site:datatracker.ietf.org/ipr rfc9927`
  - Also search: `site:datatracker.ietf.org/ipr "EARO" "C-Flag"`

- [ ] **Step 11** [B] ~3m: Search for Crypto-ID method patent disclosures
  - Web search: `site:datatracker.ietf.org/ipr "Crypto-ID" OR "cryptographic identifier" OR "address ownership"`

- [ ] **Step 12** [V] ~2m: Document IPR search results
  - Record: Number of IPR disclosures found per RFC
  - Record: Patent numbers if any
  - Record: FRAND/royalty-free/reasonable terms if declared
  - If zero IPR found → CLEAR, proceed
  - If IPR found → Step 13

- [ ] **Step 13** [D] ~5m: If IPR disclosures found, analyze terms
  - Fetch each IPR disclosure page
  - Classify: royalty-free / FRAND / restrictive / blanket
  - Assess: does the patent cover our intended use (Crypto-ID-style auth for Monad)?
  - If royalty-free → CLEAR with note
  - If FRAND → FLAG for Barrister detailed review
  - If restrictive → BLOCK crypto-identity feature, document alternative path

- [ ] **Step 14** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add -A && git commit -m "[PLAN S67] Steps 9-14: IPR search complete for RFC 8928/9927"
  ```

- [ ] **Step 15** [B] ~3m: Search for HbH option type patent disclosures affecting Monad
  - Web search: `site:datatracker.ietf.org/ipr "hop-by-hop" OR "HbH" option`
  - Rationale: Monad uses HbH option type 0x3E — check if any patents cover this mechanism

- [ ] **Step 16** [B] ~3m: Search for eBPF/XDP patent landscape
  - Web search: `site:datatracker.ietf.org/ipr "eBPF" OR "XDP" OR "BPF"`
  - Also: `"extended Berkeley Packet Filter" patent site:patents.google.com` (landscape awareness)

- [ ] **Step 17** [W] ~5m: Write IPR clearance report
  ```
  File: references/IPR-CLEARANCE-RFC8928-RFC9927.md
  Contents:
  - Search date
  - Databases searched (IETF IPR, Google Patents)
  - RFC 8928 results
  - RFC 9927 results
  - Crypto-ID method results
  - HbH option results
  - eBPF/XDP landscape
  - CLEARANCE STATUS: [CLEAR / CONDITIONAL / BLOCKED]
  - Recommended next actions
  ```

- [ ] **Step 18** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/IPR-CLEARANCE-RFC8928-RFC9927.md && git commit -m "[PLAN S67] Steps 15-18: IPR clearance report written"
  ```

- [ ] **Step 19** [V] ~2m: Cross-reference IPR findings against Sophia pqc_algo field
  - Sophia stores `pqc_algo` and `fingerprint` — these are future crypto touchpoints
  - Verify: no patent claims on the ECDSA/Ed25519 patterns used in RFC 8928's Crypto-Type 0/1/2
  - Note: ECDSA and Ed25519 are believed patent-free (NIST curves, Bernstein curves)

- [ ] **Step 20** [V] ~1m: **PHASE 1 EXIT GATE** — IPR landscape clear
  - IPR-CLEARANCE-RFC8928-RFC9927.md exists and is committed
  - CLEARANCE STATUS documented
  - If CLEAR → proceed to Phase 2
  - If BLOCKED → STOP. Escalate to Barrister for detailed patent analysis before ANY crypto feature work

---

## PHASE 2: MONAD FLAGS AUDIT — THE C-FLAG LESSON (Steps 21-32)

**Goal**: Audit Monad flags byte for collision risk, extensibility gaps, and RFC 9927-type hazards. Produce a formal flags allocation table.
**Prerequisite**: Phase 0 complete (can run parallel with Phase 1 if agent available).
**Time**: 45 minutes
**Agent**: Coordinator [P with Phase 1]

- [ ] **Step 21** [R] ~2m: Map current flags byte allocation
  ```
  Current: C(7) | Y(6) | T(5) | E(4) | S(3) | M(2) | K1(1) | K0(0)
  Allocation: 8/8 bits = 100% — ZERO extension room
  ```

- [ ] **Step 22** [W] ~5m: Write flags collision analysis document
  ```
  File: references/MONAD-FLAGS-COLLISION-ANALYSIS.md
  Contents:
  - Current allocation table (bit position → name → meaning → spec reference)
  - RFC 9927 lessons learned (C-flag collision with P-Field at bits 2-3)
  - Risk assessment: what happens if a third party defines a flag at bit 2 (M)?
  - Extension options:
    A) Reserve bits by moving K1|K0 to a sub-field
    B) Add a second flags byte (extend header from 20→21 bytes + padding)
    C) Use IPv6 HbH sub-options for new flags
    D) Accept 8-bit limit and freeze the flags space
  - Recommendation with rationale
  ```

- [ ] **Step 23** [V] ~1m: Validate that K1|K0 (Kingdom Mode) MUST be zeroed at egress
  - Confirm Shield egress pipeline clears bits 0-1
  - This is critical: if external parties see K1|K0 as "available" they'll try to reuse them

- [ ] **Step 24** [R] ~3m: Analyze overlap between our flag semantics and RFC 8928 EARO flags
  ```
  RFC 8928 EARO flags: R|P|I|T (4 bits)
  Our Monad flags: C|Y|T|E|S|M|K1|K0 (8 bits)
  Overlap: T flag exists in BOTH — different semantics (ours=Traced, theirs=??)
  Document any semantic collision risk if our packets traverse 6LoWPAN infrastructure
  ```

- [ ] **Step 25** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/MONAD-FLAGS-COLLISION-ANALYSIS.md && git commit -m "[PLAN S67] Steps 21-25: Flags collision analysis complete"
  ```

- [ ] **Step 26** [W] ~5m: Draft formal Monad Flags Bitfield IANA registration request
  ```
  File: references/IANA-REGISTRATION-MONAD-FLAGS.md
  Format: Follow IANA guide Registry 3 template exactly
  Contents:
  - Registry Name: Monad Flags Bitfield
  - Type: Bitfield (8 bits)
  - Registration Procedure: Specification Required + Expert Review
  - Initial Values table (all 8 bits with name, meaning, reference)
  - Expert Review Guidance
  - Change Control: IETF
  - Reference: draft-bellis-unheaded-protocol-foundation-03
  ```

- [ ] **Step 27** [W] ~5m: Draft IANA registration for Kingdom Mode Registry
  ```
  File: references/IANA-REGISTRATION-KINGDOM-MODE.md
  Format: Follow IANA guide Registry 7 template
  Contents:
  - Registry Name: Kingdom Modes
  - Type: Integer (0-3, 2-bit field)
  - Registration Procedure: Standards Action
  - All 4 values: NORMAL(00), PRIORITY(01), EXPERIMENTAL(10), RESERVED(11)
  ```

- [ ] **Step 28** [W] ~5m: Draft IANA registration for HbH Option Type
  ```
  File: references/IANA-REGISTRATION-HBH-OPTION.md
  Format: Follow IANA guide Registry 1 template
  Contents:
  - Request: Option Type 0x3E in IPv6 Hop-by-Hop Option Types
  - act=00 (skip), chg=1 (may change), opt assignment
  - Reference: draft-bellis-unheaded-protocol-foundation-03
  ```

- [ ] **Step 29** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/IANA-REGISTRATION-*.md && git commit -m "[PLAN S67] Steps 26-29: Three IANA registration drafts written"
  ```

- [ ] **Step 30** [W] ~3m: Draft IANA registration for Flow Action Registry
  ```
  File: references/IANA-REGISTRATION-FLOW-ACTIONS.md
  Format: Follow IANA guide Registry 4 template
  Contents: All 19 standard actions (0x00-0x12) + experimental range (0xF0-0xFE)
  ```

- [ ] **Step 31** [W] ~3m: Draft IANA registration for Anamnesis Event Types
  ```
  File: references/IANA-REGISTRATION-ANAMNESIS-EVENTS.md
  Format: Follow IANA guide Registry 9 template
  Contents: All 9 event types (0x00-0x08) + experimental range
  ```

- [ ] **Step 32** [V] ~2m: **PHASE 2 EXIT GATE** — All IANA registration drafts complete
  - Verify 5 registration documents exist:
    1. IANA-REGISTRATION-MONAD-FLAGS.md
    2. IANA-REGISTRATION-KINGDOM-MODE.md
    3. IANA-REGISTRATION-HBH-OPTION.md
    4. IANA-REGISTRATION-FLOW-ACTIONS.md
    5. IANA-REGISTRATION-ANAMNESIS-EVENTS.md
  - Verify flags collision analysis exists
  - If all present → Phase 3
  - If missing → write missing docs before proceeding

---

## PHASE 3: ZERO-DEPLOYMENT WIRE FORMAT WINDOW — BREAKING CHANGES AUDIT (Steps 33-48)

**Goal**: Identify ALL wire format changes we want to make while deployment=0. This is the RFC 9927 precedent — break now or hold forever.
**Prerequisite**: Phase 2 complete (flags audit informs what might change).
**Time**: 90 minutes
**Agent**: Coordinator

- [ ] **Step 33** [W] ~10m: Create wire format change proposal document
  ```
  File: references/WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md

  ## Breaking Change Window — Deployment = 0

  Per RFC 9927 Section 4 precedent: "The updates introduced in this
  document are not backward compatible. However, given that there are no
  known implementations or deployments... this document does not require
  any transition plan."

  Our deployment is also zero. Every breaking change we defer will cost
  a transition plan later. Decide NOW.

  ### Candidate Breaking Changes

  [To be filled in steps 34-43]
  ```

- [ ] **Step 34** [R] ~5m: CANDIDATE 1 — Flags byte extension
  ```
  Current: 1-byte flags (8 bits, fully allocated)
  Proposal: Extend to 2-byte flags (16 bits, 8 reserved for future)
  Impact: Header grows from 20→22 bytes (padded to 24)
  Pro: Room to grow without RFC 9927-type collision
  Con: Header size increase, 8-octet alignment changes
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 35** [R] ~5m: CANDIDATE 2 — CRC-16 → CRC-32 upgrade
  ```
  Current: CRC-16/CCITT-FALSE (16-bit, covers bytes 0x00-0x09)
  Proposal: CRC-32/MPEG-2 (32-bit, covers full header minus checksum)
  Impact: Stronger integrity, header grows by 2 bytes
  Pro: Better error detection for 6-hop ring (Hamming distance improvement)
  Con: 2 extra bytes, computational overhead (minimal)
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 36** [R] ~5m: CANDIDATE 3 — trace_id expansion to 64-bit
  ```
  Current: trace_id is uint32 (4 bytes, max 4.3B unique values)
  Proposal: Expand to uint64 (8 bytes, practically infinite)
  Impact: Eliminates trace_id collision in high-traffic deployments
  Pro: Future-proof for distributed tracing at scale
  Con: Header grows by 4 bytes
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 37** [R] ~5m: CANDIDATE 4 — Reserved field repurpose
  ```
  Current: 2-byte reserved field at offset 0x08
  Proposal: Repurpose for specific use (e.g., sequence number, TTL, auth tag)
  Impact: Uses existing space, no header growth
  Pro: Zero cost, adds functionality
  Con: Reduces future flexibility
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 38** [R] ~5m: CANDIDATE 5 — Add optional Crypto-ID field (RFC 8928 inspired)
  ```
  Current: No cryptographic identity in Monad header
  Proposal: Optional 8-byte Crypto-ID field (present when S flag set)
  Impact: Variable-length header (20 bytes base, 28 bytes with Crypto-ID)
  Pro: Enables per-packet auth for multi-domain deployments
  Con: Variable-length complicates BPF parsing, optional fields add complexity
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 39** [R] ~5m: CANDIDATE 6 — Exponent encoding revision
  ```
  Current: MTU exponent uses base 576 (odd choice, legacy)
  Proposal: Revise to power-of-2 base (e.g., 2^exponent × 64)
  Impact: Cleaner math, better alignment with modern MTU sizes
  Pro: Simpler encoding/decoding, BPF-verifier-friendly bit shifts
  Con: Breaking change to any existing implementations (none exist)
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 40** [R] ~5m: CANDIDATE 7 — Version field reduction (8-bit → 4-bit)
  ```
  Current: version is uint8 (256 possible versions, overkill)
  Proposal: Reduce to 4-bit version + 4-bit header-length indicator
  Impact: Enables variable-length headers cleanly
  Pro: Follows IPv4/IPv6 pattern, enables extension headers
  Con: Max 15 versions (more than enough), requires format change
  Decision: [ACCEPT / REJECT / DEFER]
  ```

- [ ] **Step 41** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md && git commit -m "[PLAN S67] Steps 33-41: 7 breaking change candidates documented"
  ```

- [ ] **Step 42** [W] ~10m: Score each candidate on impact matrix
  ```
  Append to WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md:

  ## Decision Matrix

  | # | Candidate | Header Impact | BPF Impact | Value | Risk | Verdict |
  |---|-----------|--------------|------------|-------|------|---------|
  | 1 | 16-bit flags | +2 bytes | Moderate | HIGH | LOW | ? |
  | 2 | CRC-32 | +2 bytes | Low | MED | LOW | ? |
  | 3 | 64-bit trace_id | +4 bytes | Moderate | HIGH | LOW | ? |
  | 4 | Reserved repurpose | 0 bytes | None | MED | NONE | ? |
  | 5 | Optional Crypto-ID | +0/+8 var | HIGH | HIGH | MED | ? |
  | 6 | Exponent revision | 0 bytes | Low | LOW | NONE | ? |
  | 7 | 4-bit version | 0 bytes | Low | MED | LOW | ? |

  ## Muck's Verdicts (FILL IN)
  [Requires human decision on each candidate]
  ```

- [ ] **Step 43** [V] ~2m: Verify all 7 candidates are documented with pro/con/impact
  - Each candidate has: current state, proposal, impact, pro, con, decision placeholder
  - If any incomplete → fill before proceeding

- [ ] **Step 44** [W] ~5m: Write "what if we change nothing" analysis
  ```
  Append to WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md:

  ## Alternative: Freeze Wire Format As-Is

  If we ACCEPT no breaking changes:
  - Flags byte is permanently 8 bits, fully allocated, no room to grow
  - CRC-16 stays (adequate for now, may limit multi-domain trust)
  - trace_id stays 32-bit (4.3B ceiling)
  - No crypto identity (multi-domain auth must use external mechanisms)
  - Exponent encoding stays weird but functional

  Risk of freezing: moderate. The protocol works for single-domain.
  Risk of NOT freezing: breaking changes require transition plans later.

  RECOMMENDATION: Accept candidates [1, 4, 6, 7] minimum.
  These are low-risk, high-value, zero-header-growth changes.
  ```

- [ ] **Step 45** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md && git commit -m "[PLAN S67] Steps 42-45: Decision matrix and freeze analysis complete"
  ```

- [ ] **Step 46** [W] ~5m: If any candidates ACCEPTED, draft wire format v2 proposal
  ```
  File: references/MONAD-WIRE-FORMAT-V2-PROPOSAL.md
  Contents:
  - New field table with all accepted changes
  - New ASCII wire diagram
  - Migration notes (trivial: deployment=0, just update spec)
  - Updated CRC scope if header changed
  - BPF parsing impact assessment
  ```

- [ ] **Step 47** [W] ~5m: Update I-D revision tracker
  ```
  Append to draft tracking:
  - draft-bellis-unheaded-protocol-foundation → bump to -04 if wire format changes accepted
  - draft-bellis-unheaded-sophia-dictionary → -01 if service_id field changes
  - draft-bellis-unheaded-wotan-memory → -01 if address space layout changes
  ```

- [ ] **Step 48** [V] ~2m: **PHASE 3 EXIT GATE** — Breaking changes window documented
  - WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md exists with all 7 candidates
  - Decision matrix populated (verdicts may be "PENDING MUCK")
  - Freeze analysis complete
  - V2 proposal drafted (if any ACCEPTED)
  - If all present → Phase 4
  - If decisions needed → PAUSE for Muck input before Phase 5

---

## PHASE 4: IANA CONSIDERATIONS SECTION DRAFT (Steps 49-56)

**Goal**: Write the formal IANA Considerations section for inclusion in draft-bellis-unheaded-protocol-foundation-04.
**Prerequisite**: Phase 2 registration drafts complete. Can run parallel with Phase 3.
**Time**: 45 minutes
**Agent**: Agent [P with Phase 3]

- [ ] **Step 49** [R] ~2m: Re-read IANA guide template (Section 8)
  ```bash
  # Focus on the IANA Section Template
  grep -A 100 "## 8. IANA Section Template" /sessions/nice-blissful-dirac/mnt/.skills/skills/unheaded-rfceditor/references/iana-guide.md
  ```

- [ ] **Step 50** [W] ~10m: Write complete IANA Considerations section
  ```
  File: references/IANA-CONSIDERATIONS-SECTION-DRAFT.md

  Format: RFC-ready text following BCP 14 keywords
  Contents:
  1. Overview statement
  2. New Registry: Monad Flags Bitfield (8-bit, Spec Required + Expert Review)
  3. New Registry: Kingdom Modes (2-bit, Standards Action)
  4. New Registry: Unheaded Flow Actions (8-bit, Expert Review)
  5. New Registry: Anamnesis Event Types (8-bit, Expert Review)
  6. New Registry: Wotan Error Codes (16-bit, Expert Review)
  7. New Registry: Sophia Sub-Dictionary Types (8-bit, Expert Review)
  8. New Registry: Monad Protocol Versions (4-bit, Standards Action)
  9. Registration into existing: IPv6 HbH Option Types (value 0x3E)
  10. Designated Expert nominations
  11. TBA references list
  ```

- [ ] **Step 51** [V] ~3m: Validate IANA section against RFC 8126 requirements
  - Every registry has: name, type, purpose, registration procedure, initial values, namespace description, reserved ranges, change control, reference
  - Every registration has: value, name, description, reference
  - TBA placeholders are listed and described

- [ ] **Step 52** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/IANA-CONSIDERATIONS-SECTION-DRAFT.md && git commit -m "[PLAN S67] Steps 49-52: IANA Considerations section drafted"
  ```

- [ ] **Step 53** [W] ~5m: Write Designated Expert Guidelines document
  ```
  File: references/DESIGNATED-EXPERT-GUIDELINES.md
  Contents:
  - Expert selection criteria (deep Unheaded protocol knowledge)
  - Review timeline (2 weeks)
  - Technical criteria per registry
  - Rejection criteria
  - Escalation path (IESG)
  - Naming conventions
  ```

- [ ] **Step 54** [W] ~5m: Write Security Considerations addendum for IANA registrations
  ```
  File: references/SECURITY-CONSIDERATIONS-IANA.md
  Contents:
  - Flag bit squatting risk (RFC 9927 lesson)
  - Malicious action registration risk
  - Kingdom Mode privilege escalation via spoofed registration
  - Mitigation: Standards Action for critical registries
  ```

- [ ] **Step 55** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add references/DESIGNATED-EXPERT-GUIDELINES.md references/SECURITY-CONSIDERATIONS-IANA.md && git commit -m "[PLAN S67] Steps 53-55: Expert guidelines and security considerations"
  ```

- [ ] **Step 56** [V] ~2m: **PHASE 4 EXIT GATE** — IANA section ready for I-D inclusion
  - IANA-CONSIDERATIONS-SECTION-DRAFT.md exists (10 registries + 1 existing registration)
  - Designated Expert Guidelines exists
  - Security Considerations addendum exists
  - All committed
  - If all present → Phase 5
  - If missing → complete before proceeding

---

## PHASE 5: I-D INTEGRATION — FOUNDATION SPEC UPDATE (Steps 57-62)

**Goal**: Integrate IANA section and any accepted wire format changes into the foundation I-D.
**Prerequisite**: Phase 3 (breaking changes decided) AND Phase 4 (IANA section drafted).
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 57** [W] ~10m: Update draft-bellis-unheaded-protocol-foundation
  ```
  If wire format changes ACCEPTED:
  - Bump version to -04
  - Update wire diagram
  - Update field table
  - Update CRC scope
  - Add "Changes from -03" section

  Regardless:
  - Insert IANA Considerations section from Phase 4
  - Insert Security Considerations addendum
  - Update references (add RFC 8928, RFC 9927 as informative references)
  ```

- [ ] **Step 58** [V] ~3m: Validate I-D structure
  - Has: Abstract, Introduction, Wire Format, IANA Considerations, Security Considerations, References
  - BCP 14 keywords used correctly (MUST/SHOULD/MAY)
  - All TBA references consistent

- [ ] **Step 59** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add draft-bellis-unheaded-* && git commit -m "[PLAN S67] Steps 57-59: Foundation I-D updated with IANA section"
  ```

- [ ] **Step 60** [W] ~5m: Update Sophia and Wotan I-Ds if affected
  ```
  If any breaking changes affect Sophia service_id or Wotan address space:
  - Bump respective I-D versions
  - Add IANA Considerations sections (Sophia Sub-Dict Types, Wotan Error Codes)
  - Cross-reference foundation I-D
  ```

- [ ] **Step 61** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add -A && git commit -m "[PLAN S67] Steps 60-61: All I-Ds updated"
  ```

- [ ] **Step 62** [V] ~2m: **PHASE 5 EXIT GATE** — All I-Ds updated and consistent
  - Foundation I-D has IANA Considerations
  - Wire format matches between I-D and protocol summary
  - All three I-Ds reference each other consistently
  - If consistent → Phase 6
  - If inconsistent → fix discrepancies

---

## PHASE 6: DOCUMENTATION RIPPLE — LIBRARIAN SWEEP (Steps 63-67)

**Goal**: Ripple all changes across the document web (wiki, CLAUDE.md, protocol summary, skills references).
**Prerequisite**: Phase 5 complete. Can partially overlap with Phase 5.
**Time**: 30 minutes
**Agent**: Agent [P]

- [ ] **Step 63** [W] ~5m: Update protocol summary reference
  ```
  File: skills/unheaded-rfceditor/references/unheaded-protocol-summary.md
  Changes:
  - Update wire format if changed
  - Update flags table if extended
  - Update spec status table (new version numbers)
  - Add RFC 8928/9927 to references
  ```

- [ ] **Step 64** [W] ~5m: Update wiki pages
  ```
  Files to update:
  - wiki/Protocol-Foundation.md (wire format changes)
  - wiki/Wire-Format-Patterns.md (new patterns if any)
  - Add new page: wiki/IANA-Registrations.md (summary of all registries)
  ```

- [ ] **Step 65** [W] ~3m: Update CLAUDE.md spec status
  ```
  Update the timeline/status section to reflect:
  - IANA registration drafts created
  - IPR clearance status
  - Wire format version (if bumped)
  ```

- [ ] **Step 66** [C] ~1m: **COMMIT CHECKPOINT**
  ```
  git add -A && git commit -m "[PLAN S67] Steps 63-66: Documentation ripple complete"
  ```

- [ ] **Step 67** [V] ~3m: **PHASE 6 EXIT GATE** — Document web consistent
  - Protocol summary matches I-D
  - Wiki pages updated
  - CLAUDE.md reflects current state
  - No stale references to old wire format

---

## PHASE 7: FINAL VERIFICATION & HANDOFF (Step 68)

**Goal**: Verify all deliverables exist. Write handoff for next session.
**Prerequisite**: All prior phases complete.
**Time**: 10 minutes
**Agent**: Coordinator

- [ ] **Step 68** [V] ~5m: **SPRINT EXIT GATE — S67 COMPLETE**

  Verify ALL deliverables:
  ```
  [ ] references/IPR-CLEARANCE-RFC8928-RFC9927.md — IPR search results
  [ ] references/MONAD-FLAGS-COLLISION-ANALYSIS.md — Flags audit
  [ ] references/WIRE-FORMAT-BREAKING-CHANGES-WINDOW.md — Breaking changes with decisions
  [ ] references/MONAD-WIRE-FORMAT-V2-PROPOSAL.md — V2 proposal (if changes accepted)
  [ ] references/IANA-REGISTRATION-MONAD-FLAGS.md — Flags registry
  [ ] references/IANA-REGISTRATION-KINGDOM-MODE.md — Kingdom Mode registry
  [ ] references/IANA-REGISTRATION-HBH-OPTION.md — HbH option type
  [ ] references/IANA-REGISTRATION-FLOW-ACTIONS.md — Flow actions registry
  [ ] references/IANA-REGISTRATION-ANAMNESIS-EVENTS.md — Event types registry
  [ ] references/IANA-CONSIDERATIONS-SECTION-DRAFT.md — Full IANA section
  [ ] references/DESIGNATED-EXPERT-GUIDELINES.md — Expert review guidance
  [ ] references/SECURITY-CONSIDERATIONS-IANA.md — Security addendum
  [ ] Updated foundation I-D with IANA section
  [ ] Documentation ripple complete
  ```

  Write handoff:
  ```
  File: references/S67-HANDOFF.md
  Contents:
  - What was accomplished
  - IPR clearance status
  - Wire format decisions (accepted/rejected/pending)
  - IANA registration status (drafted, not yet submitted)
  - Blocked items (if any STUCK)
  - Next steps: actual IANA submission process, bare metal validation
  ```

  Final commit:
  ```
  git add -A && git commit -m "[PLAN S67] Sprint complete: IANA registrations drafted, IPR cleared, wire format window documented"
  ```

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: IETF IPR Database Unreachable
- **Symptom**: datatracker.ietf.org returns 5xx or timeout
- **Recovery**: Use Google cache: `cache:datatracker.ietf.org/ipr/search/?rfc=8928`
- **Fallback**: Search Google Patents directly for RFC 8928 author names
- **If still blocked**: Document as STUCK, note "IPR search incomplete — retry next session"

### E2: Wire Format Change Creates BPF Verifier Incompatibility
- **Symptom**: Proposed header change exceeds BPF map value size or breaks alignment
- **Recovery**: Check `sizeof(struct monad_header)` against BPF_MAP_VALUE_SIZE_MAX
- **Constraint**: BPF map values max 32KB — our header is nowhere near this
- **Real risk**: Alignment — BPF verifier requires 4-byte aligned access
- **Fix**: Ensure all multi-byte fields start on 4-byte boundaries

### E3: Existing IANA Registration Conflicts with Our Option Type
- **Symptom**: 0x3E already assigned in IPv6 HbH Option Types registry
- **Recovery**: Check https://www.iana.org/assignments/ipv6-parameters/
- **Fix**: Choose different option type value, update all references
- **Note**: As of this plan's writing, 0x3E appears unassigned. Verify before submission.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel? | Depends On | Est. Time |
|-------|-------|-----------|------------|-----------|
| 0: Intelligence | Coordinator | No | — | 30m |
| 1: IPR Search | Coordinator | Yes (with P2) | P0 | 60m |
| 2: Flags Audit | Agent | Yes (with P1) | P0 | 45m |
| 3: Breaking Changes | Coordinator | Yes (with P4) | P2 | 90m |
| 4: IANA Section | Agent | Yes (with P3) | P2 | 45m |
| 5: I-D Integration | Coordinator | No | P3 + P4 | 30m |
| 6: Doc Ripple | Agent | Partial (with P5) | P5 | 30m |
| 7: Final Gate | Coordinator | No | ALL | 10m |

**Critical Path**: P0 → P2 → P3 → P5 → P7 = 30 + 45 + 90 + 30 + 10 = **205 minutes (~3.5 hours)**
**With parallelization**: P1 runs with P2, P4 runs with P3, P6 overlaps P5 = **~5 hours total wall clock**

---

## APPENDIX C: QUICK REFERENCE

### Monad Flags Byte (Current)
```
Bit 7: C (checksum valid)
Bit 6: Y (sync/ack required)
Bit 5: T (traced — Anamnesis enabled)
Bit 4: E (encrypted)
Bit 3: S (signed)
Bit 2: M (modified)
Bit 1: K1 (Kingdom Mode bit 1)
Bit 0: K0 (Kingdom Mode bit 0)
```

### RFC 9927 Key Lesson
```
Problem: RFC 8928 defined C-flag at bit 3
         RFC 9685 defined P-Field at bits 2-3
         COLLISION at bit 3
Fix:     RFC 9927 relocated C-flag to bit 1
Cost:    Entire corrective RFC + backward incompatibility
Lesson:  REGISTER YOUR BITS WITH IANA EARLY
```

### IANA Registration URLs
```
IPv6 HbH Options: https://www.iana.org/assignments/ipv6-parameters/
IPR Database:     https://datatracker.ietf.org/ipr/
RFC 8126 Policies: https://www.rfc-editor.org/rfc/rfc8126
```

### I-D Current Versions
```
draft-bellis-unheaded-protocol-foundation  -03  SHIPPED
draft-bellis-unheaded-sophia-dictionary    -00  SHIPPED
draft-bellis-unheaded-wotan-memory         -00  SHIPPED
```

---

*S67 Battle Plan — Forged 2026-02-27*
*7 Phases. 68 Steps. Securing the Kingdom's wire format before the world touches it.*
*The C-Flag fell because nobody registered it. Monad will not fall the same way.*
