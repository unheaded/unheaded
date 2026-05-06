# Compliance Matrix Completeness Audit — 2026-05-06

**Date:** 2026-05-06
**Author:** Marshal post-shift extension (NORTH-STAR Appendix A Phase E completeness verification)
**Purpose:** Audit every E1-E15 matrix for omissions per Stevie's directive *"audit and verify we omit nothing"*. Identify abbreviations, deferred enumerations, and explicit omissions. Provide a remediation map for any audit-readiness gap.

**Method:** This document acts as the **index** for the entire `docs/compliance/control-matrix/` directory plus a **per-matrix completeness assessment**. Where a matrix abbreviates (e.g. "selected controls" or "headline subset"), the abbreviation is logged here with a recommendation for expansion.

**Marshal honesty:** the matrices written tonight are **gap inventories at primary control / parent-control depth**, not full SSPs. Every framework's full enumeration includes sub-controls / control enhancements / Points of Focus / paragraph-level requirements that this matrix family does NOT exhaustively walk. The omissions below are the work that converts the gap inventory into an SSP-grade document.

---

## Matrix Family Index

| File | Framework | Date | Status |
|------|-----------|------|--------|
| `00-completeness-audit-2026-05-06.md` | (this index) | 2026-05-06 | — |
| `gdpr-2026-05-06.md` | EU GDPR | 2026-05-06 | E1 ✓ |
| `soc2-2026-05-06.md` | SOC 2 (2017 TSC) | 2026-05-06 | E2 ✓ |
| `pci-dss-2026-05-06.md` | PCI DSS v4.0 | 2026-05-06 | E3 ✓ |
| `hipaa-2026-05-06.md` | HIPAA Security/Privacy/Breach | 2026-05-06 | E4 ✓ |
| `nist-800-53-2026-05-06.md` | NIST SP 800-53 Rev 5 | 2026-05-06 | E5 ✓ |
| `fedramp-moderate-2026-05-06.md` | FedRAMP Moderate | 2026-05-06 | E6 ✓ |
| `nist-800-171-2026-05-06.md` | NIST SP 800-171 r3 | 2026-05-06 | E7 ✓ |
| `nist-800-207-2026-05-06.md` | NIST SP 800-207 ZTA | 2026-05-06 | E8 ✓ |
| `nist-csf-2-2026-05-06.md` | NIST CSF 2.0 | 2026-05-06 | E9 ✓ |
| `nist-800-218-ssdf-2026-05-06.md` | NIST SP 800-218 SSDF v1.1 | 2026-05-06 | E10 ✓ |
| `iso-27001-27002-2026-05-06.md` | ISO/IEC 27001:2022 + 27002:2022 | 2026-05-06 | E11 ✓ |
| `cis-controls-v8-2026-05-06.md` | CIS Controls v8/v8.1 | 2026-05-06 | E12 ✓ |
| `cmmc-2-2026-05-06.md` | CMMC 2.0 Level 2 | 2026-05-06 | E13 ✓ |
| `itar-ear-2026-05-06.md` | ITAR / EAR Export Control | 2026-05-06 | E14 ✓ |
| `ccpa-cpra-2026-05-06.md` | CCPA / CPRA | 2026-05-06 | E15 ✓ |

---

## Per-Matrix Completeness Audit

### E1 — GDPR (`gdpr-2026-05-06.md`)

**Covered explicitly:**
- Chapter II Principles (Articles 5-11) — all 7 articles
- Chapter III Data Subject Rights (Articles 12-23) — all 12 articles
- Chapter IV Controller and Processor (Articles 24-43) — all 20 articles
- Chapter V International Transfers (Articles 44-50) — all 7 articles
- Chapter VIII selected (Articles 77, 82, 83) — 3 of 7

**Articles abbreviated or omitted:**

| Articles | Coverage | Reason / Recommendation |
|----------|----------|-------------------------|
| Articles 1-4 (Subject matter, scope, definitions) | OMITTED | Definitional; non-control. |
| Articles 51-59 (Independent supervisory authorities) | OMITTED | Regulator-facing structure; no controller obligation. |
| Articles 60-76 (Cooperation and consistency mechanism) | OMITTED | Regulator-internal; no controller obligation. |
| Articles 78-81 (Other remedies) | OMITTED | Litigation-oriented; document handler GAP. |
| Articles 84-91 (Special situations) | OMITTED | Member State-specific derogations; check per-deployment-jurisdiction. |
| Articles 92-99 (Delegated acts / final provisions) | OMITTED | Regulatory machinery. |
| Recitals 1-173 (interpretive) | OMITTED | Non-binding; useful for narrative but not control-mappable. |

**Recommendation for full audit-readiness:**
- Add Article 31 (Cooperation with supervisory authority) — **GAP** (not currently documented as controller obligation in E1).
- Add Article 58 (Powers of supervisory authority) — controller MUST cooperate when investigated; document cooperation procedure.
- Add Articles 84-91 jurisdiction-conditional analysis when first EU adopter materializes.

### E2 — SOC 2 (`soc2-2026-05-06.md`)

**Covered explicitly:**
- Common Criteria CC1-CC9 — all 9 categories at numbered-control level (CC1.1, CC1.2, ... CC9.2). 33 numbered controls.
- Trust Service Categories: Availability A1.1-A1.3, Processing Integrity PI1.1-PI1.5, Confidentiality C1.1-C1.2, Privacy P1.0-P8.1.

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **Points of Focus (POFs)** for each numbered control | OMITTED | The 2017 TSC + 2022 update lists 200+ POFs across all categories. Each POF is illustrative ("how to satisfy") and not separately auditable. **Recommendation:** for SOC 2 Type 1/2 readiness, MoatGhost should walk POFs per-numbered-control to confirm operating effectiveness. This expansion is ~50K of additional matrix. |
| **Privacy P1-P8 sub-controls** beyond what was named | PARTIAL | P3.2 → P8.1 covered as named numbered controls; underlying obligations (privacy policy, choice, access, etc.) partially detailed. **Recommendation:** if Privacy is in audit scope, expand inline. |
| 2022 Points of Focus update | OMITTED specifically | The 2022 amendment added POFs not in the 2017 baseline. **Recommendation:** verify against AICPA's current Master TSC reference at audit time. |

**Recommendation for full audit-readiness:**
- POF-level expansion for an actual SOC 2 audit (post-Type-1-decision).
- Add sub-categories C1.0 + C1.1 + C1.2 (some auditors enumerate as 3 controls; I collapsed to 2 — verify).

### E3 — PCI DSS v4.0 (`pci-dss-2026-05-06.md`)

**Covered explicitly:**
- All 12 Requirements at top level.
- Selected sub-requirements (1.1-1.5, 2.1-2.3, 4.1-4.2, 5.1-5.4, 6.1-6.5, 7.1-7.3, 8.1-8.6, 10.1-10.7, 11.1-11.6, 12.1-12.10).

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **Defined-approach sub-requirements (e.g. 1.2.1.a, 1.2.5.a, 8.3.6, etc.)** | PARTIALLY ENUMERATED | PCI 4.0 has ~250 numbered sub-requirements. I covered the 1-2-decimal-deep level (e.g. 1.2.5, 6.4) but did not exhaustively walk the 3-decimal (1.2.5.a, 6.4.1, etc.) level. **Recommendation:** for an actual PCI ROC, walk every dotted sub-requirement. |
| **Customized-approach** alternative test procedures (PCI 4.0 added) | OMITTED | PCI 4.0 introduced "customized approach" — entity defines control objectives + targeted risk analysis + custom validation. Not yet relevant at gap-inventory stage. |
| Requirement 3 sub-requirements | COLLAPSED to N/A | All 3.1-3.7 collapsed to "N/A-DESIGN" because Unheaded does not store CHD. **Verify this scope-out is documented in a Self-Assessment Questionnaire (SAQ-A or SAQ-D type) for any adopter who pulls Unheaded in.** |
| Requirement 9 (physical access) | COLLAPSED to ADOPTER-OWNS | Same — single line for the entire requirement. |
| **Targeted Risk Analysis** (TRA) requirements (PCI 4.0) | OMITTED | PCI 4.0 mandates TRAs for certain controls; not covered here. |

**Recommendation for full audit-readiness:**
- Sub-requirement enumeration to 3-decimal depth. ~5-10 hours of MoatGhost time.
- TRA template for the controls PCI 4.0 requires it.

### E4 — HIPAA (`hipaa-2026-05-06.md`)

**Covered explicitly:**
- §164.308 Administrative Safeguards — all listed implementation specs (Required + Addressable).
- §164.310 Physical Safeguards — all listed.
- §164.312 Technical Safeguards — all listed.
- §164.314 Organizational Requirements — all listed.
- §164.316 Policies and Documentation — all listed.
- Privacy Rule §164.502/504(e)/510-512/520/524/526/528/530 — selected.
- Breach Notification §164.402/404/406/408/410/412/414 — all listed.

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **Privacy Rule §164.500-§164.534** | SELECTED ONLY | I covered §164.502 + §164.504(e) + §164.510-512 + §164.520 + §164.524 + §164.526 + §164.528 + §164.530. Omitted: §164.500 (applicability), §164.501 (definitions), §164.503 (organizational requirements), §164.506 (uses & disclosures for treatment/payment/healthcare ops), §164.508 (uses & disclosures requiring authorization), §164.509 (uses & disclosures requiring opportunity to agree or object), §164.514 (de-identification), §164.522 (rights to request privacy protection / restrictions), §164.532 (transition provisions), §164.534 (compliance dates). **Recommendation:** for a full HIPAA Privacy Rule audit, expand inline. Most are ADOPTER-OWNS for our architectural posture. |
| **§164.514 De-identification methods** (Safe Harbor + Expert Determination) | OMITTED | Critical if Unheaded ever ingests PHI for analytics. Architecturally none today. |
| **HITECH Act Subtitle D + Omnibus Rule clarifications** | LIGHT TOUCH | Mentioned in header; not separately enumerated. |

**Recommendation for full audit-readiness:**
- Privacy Rule full enumeration if any adopter requires Privacy Rule conformance (vs Security Rule only).
- Add §164.105 (Organizational Requirements — affiliated covered entities + hybrid entities) if adopter is hybrid entity.

### E5 — NIST 800-53 Rev 5 (`nist-800-53-2026-05-06.md`)

**Covered explicitly:**
- 20 control families: AC, AT, AU, CA, CM, CP, IA, IR, MA, MP, PE, PL, PM, PS, PT, RA, SA, SC, SI, SR.
- Parent control IDs at FedRAMP Moderate baseline depth.

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **Control enhancements (e.g. AC-2(1), AC-2(2), ..., SI-4(11))** | DEFERRED to E6 | I deliberately collapsed enhancements at the parent-control level in E5 and listed FedRAMP Moderate baseline enhancements in E6. **Net coverage:** E5 + E6 together enumerate parent + Moderate-baseline-enhancements. **Recommendation:** for FedRAMP High baseline, additional enhancements would be needed; for Low, fewer apply. |
| **Each control's parameter values** | LIGHT TOUCH | NIST 800-53 leaves many values as Organization-Defined Parameters (ODPs). E5 + E6 list a few; full ODP enumeration is part of SSP authoring. |
| **Family-level `-1` "Policy and Procedures" controls** | LISTED | I covered AC-1, AT-1, AU-1, etc. but treated them as "PARTIAL — implicit in CLAUDE.md" — for an actual audit, a per-family policy doc must exist. |
| **Privacy controls (PT family)** | LIGHT TOUCH | I covered PT-1 through PT-8 at parent level. Full PT family in 800-53 has more sub-controls (PT-2(1), PT-2(2), etc.) at FedRAMP-aligned baselines. |
| **PE-1 through PE-23** | COLLAPSED | Single line "Entire family ADOPTER-OWNS." For an audit, individual rationale per PE control is needed. |

**Recommendation for full audit-readiness:**
- Per-control enhancement enumeration is in E6 for FedRAMP Moderate. E5 + E6 paired is the audit-ready unit.
- ODP enumeration with kingdom-default values for each.
- Family-level policy documents (AC-1 through SR-1) — 17 documents at minimum.

### E6 — FedRAMP Moderate (`fedramp-moderate-2026-05-06.md`)

**Covered explicitly:**
- FedRAMP-specific structural requirements (FIPS 199, ConMon, 3PAO, etc.).
- Family-by-family control enhancements at FedRAMP Moderate baseline.
- FedRAMP-specific parameter values where they diverge from NIST defaults.

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **All ~80 control enhancements at Moderate baseline** | NEAR-COMPLETE | Most enumerated; some PE/MA/MP family enhancements collapsed under "family-wide ADOPTER-OWNS." |
| **FedRAMP-specific parameter values for every parameterized control** | PARTIAL | Listed for AC-7, AC-11, AC-12, AU-2, AU-11, RA-5, SC-13. Not exhaustive — full parameter list is in FedRAMP's Rev 5 baseline doc. |
| **Customer Responsibility Matrix (CRM) view** | OMITTED | FedRAMP requires a CRM showing which controls are 100%-CSP / 100%-Customer / shared. Not authored tonight. **Recommendation:** authoring the CRM is part of SSP authoring. |

### E7 — NIST 800-171 r3 (`nist-800-171-2026-05-06.md`)

**Covered explicitly:**
- All 17 control families (03.01 through 03.17).
- All 110 numbered requirements (consolidated requirements per r3 mapping).

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **Withdrawn-in-r3 controls** | LISTED | I marked withdrawn requirements explicitly so r2-vs-r3 mapping is preserved. CMMC currently uses r2; need to verify CMMC's r3 update timing. |
| **Organization-Defined Parameters (ODPs)** | LIGHT TOUCH | r3 introduced more ODPs than r2; flagged a few PARAM-GAPs. Not exhaustive. |

### E8 — NIST 800-207 ZTA (`nist-800-207-2026-05-06.md`)

**Covered explicitly:**
- All 7 ZTA tenets.
- Logical architecture components (PE, PA, PEP, data sources).
- 5 ZTA deployment variations.
- Trust algorithm variations.
- Network requirements.
- ZTA threats (5.1 through 5.7).
- Migration maturity progression across 7 pillars.

**Complete to NIST SP 800-207 publication scope.**

### E9 — NIST CSF 2.0 (`nist-csf-2-2026-05-06.md`)

**Covered explicitly:**
- All 6 functions (GOVERN, IDENTIFY, PROTECT, DETECT, RESPOND, RECOVER).
- All categories (GV.OC/RM/RR/PO/OV/SC + ID.AM/RA/IM + PR.AA/AT/DS/PS/IR + DE.CM/AE + RS.MA/AN/CO/MI + RC.RP/CO).
- Subcategories enumerated.

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **All 106 subcategories** | NEAR-COMPLETE | Some sparse subcategories (e.g. PR.IR-02 etc.) listed at parent-category level only. **Recommendation:** for CSF 2.0 profile, walk each subcategory. |
| **Implementation Examples** that NIST publishes alongside CSF 2.0 | OMITTED | Examples are illustrative, non-binding. |
| **Organizational Profile + Target Profile** | OMITTED | CSF 2.0 expects each org to author Current Profile + Target Profile. Not authored tonight; matrix is the input. |

### E10 — NIST 800-218 SSDF v1.1 (`nist-800-218-ssdf-2026-05-06.md`)

**Covered explicitly:**
- All 4 practice groups (PO, PS, PW, RV).
- All practices (PO.1 — PO.5, PS.1 — PS.3, PW.1 — PW.9, RV.1 — RV.3).
- Tasks within practices.
- EO 14028 self-attestation specifics.

**Complete to SSDF v1.1 publication scope.**

### E11 — ISO 27001:2022 + 27002:2022 (`iso-27001-27002-2026-05-06.md`)

**Covered explicitly:**
- ISO 27001:2022 management system clauses 4-10.
- ISO 27002:2022 all 93 controls in 4 themes (A.5, A.6, A.7, A.8).

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **ISO 27002:2022 attribute tagging** (control type, info-security property, cybersecurity concept, operational capability, security domain) | OMITTED | 42 attributes per control × 93 controls. Tagging is for SoA construction. **Recommendation:** mandatory for ISO 27001 Stage 1 audit; ~10 hours of work. |
| **Statement of Applicability (SoA)** itself | OMITTED | The SoA is the formal artefact mapping each Annex A control to "applicable / not applicable + rationale." Not authored tonight. |
| **ISO 27017 cloud-services controls + ISO 27018 PII processor controls** | CROSS-REFERENCED ONLY | These are 27002 overlays for cloud / PII processors. Not separately enumerated. **Recommendation:** add as separate matrices if cloud-specific or PII-processor-specific scope arises. |
| **ISO 27701 PIMS extension (privacy)** | CROSS-REFERENCED ONLY | Same — PIMS extension for privacy. |

### E12 — CIS Controls v8 (`cis-controls-v8-2026-05-06.md`)

**Covered explicitly:**
- All 18 Controls.
- All ~153 Safeguards across IG1/IG2/IG3.

**Complete to CIS Controls v8/v8.1 publication scope.**

### E13 — CMMC 2.0 Level 2 (`cmmc-2-2026-05-06.md`)

**Delegated to E7 NIST 800-171** for control content. **Covered explicitly:**
- CMMC 2.0 level hierarchy (L1/L2/L3).
- Assessment process.
- POA&M restrictions and 5-weighted practices that cannot be POA&M'd.
- Asset categorization (CUI Asset, Security Protection Asset, Contractor Risk Managed Asset, Specialized Asset, Out-of-Scope Asset).

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **Per-practice POA&M weight** (1, 3, or 5) | LIGHT TOUCH | Listed only the 5-weighted (no-POA&M) practices critical for assessment. Full weighting is in DoD's CMMC scoring methodology doc. |
| **Level 1 (FAR 52.204-21) 17 practices** | OMITTED | The kingdom likely passes Level 1 already; not separately enumerated. **Recommendation:** add Level 1 sub-matrix if any FCI-only contract materializes. |
| **Level 3 (NIST 800-172) 24+ practices** | OMITTED | Level 3 is post-headcount; out of scope for tonight. |

### E14 — ITAR / EAR (`itar-ear-2026-05-06.md`)

**Covered explicitly:**
- USML 21 categories at title level.
- EAR Category 5 Part 2 information security controls.
- License Exception ENC sub-exceptions (a/b1/b2/b3/c).
- License Exception TSU sub-exceptions (d/e).
- Embargo + SDN screening.

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **ECCN classification per Unheaded component** | DEFERRED-TO-LEGAL | Marshal does not classify ECCNs unilaterally. Barrister + export-control counsel to perform formal classification (Form BIS-748P). |
| **Per-country license requirement matrix** (Country Chart) | OMITTED | Generated per-export per-ECCN; not appropriate at gap-inventory stage. |
| **Wassenaar Arrangement / EU Dual-Use Regulation** | OMITTED | International parallel regimes; document at adopter-by-adopter level. |

### E15 — CCPA / CPRA (`ccpa-cpra-2026-05-06.md`)

**Covered explicitly:**
- Consumer Rights (sections 100, 105, 106, 110, 115, 120, 121, 125, 135, 150, 185).
- Business Obligations (sections 100(b), 130, 135, 140(d), 140(o), 155, 150).
- Sensitive Personal Information categories.
- Service Provider obligations (140(j)) and Contractor distinction (140(ai)).
- CPPA regulations (effective 2023+).
- Multi-state crosswalk (informational).

**Abbreviated:**

| Element | Coverage | Reason / Recommendation |
|---------|----------|-------------------------|
| **CPPA proposed regulations on ADMT, Risk Assessments, Cybersecurity Audits** | LIGHT TOUCH | Listed; not exhaustive (the proposed reg text is ~100+ pages). **Recommendation:** monitor CPPA rulemaking; expand when finalized. |
| **Per-section private right of action / damages** | LIGHT TOUCH | Section 150 covered; sections 199.45-199.55 on enforcement actions, damages, intervention not separately enumerated. |
| **Other state privacy laws** (VCDPA, CPA, CTDPA, UCPA, TDPSA, etc.) | CROSS-REFERENCED ONLY | Crosswalk note included; per-state matrices would require ~10 additional documents. **Recommendation:** add per-state matrices when adopters in those states materialize. |

---

## Cross-Framework Headline Gaps (universal — closing one closes many)

These five gaps appear in **every** matrix and represent the single highest aggregate leverage:

| # | Gap | Closes |
|---|-----|--------|
| 1 | **Incident Response plan + runbook** | GDPR Art. 33 + HIPAA §164.308(a)(6) + §164.410 + PCI 12.10 + SOC 2 CC7.4 + NIST IR family + 800-171 §03.06 + ISO 27002 A.5.24-A.5.27 + CIS Control 17 + CMMC 03.06 + CCPA §1798.150 + CSF 2.0 RESPOND function |
| 2 | **Service-provider / processor / BA agreement template** (single doc with 3 jurisdiction addenda) | GDPR Art. 28 + HIPAA §164.308(b) + §164.314(a) + §164.504(e) + CCPA §1798.140(j) |
| 3 | **Public privacy notice + DSR contact (security.txt + VDP)** | GDPR Art. 12-15 + CCPA §1798.130 + NIST PT-5 + SSDF RV.1.3 + CIS 16.2 |
| 4 | **Storage / retention policy per data class** | GDPR Art. 5(1)(e) + PCI 10.5 + HIPAA §164.316(b)(2)(i) + NIST AU-11 + CCPA §1798.140(o) + ISO A.5.33 + CIS 8.10 |
| 5 | **Contingency plan + DR runbook + RTO/RPO** | HIPAA §164.308(a)(7) + SOC 2 A1.3 + NIST CP family + ISO A.5.29-A.5.30 + CIS Control 11 + CSF 2.0 RECOVER function |

**Single-document leverage is enormous.** A focused 4-6 week MoatGhost + Architect engagement closing these five would dramatically improve every framework's matrix simultaneously.

---

## What Stevie should NOT expect from this matrix family

1. **Audit-ready SSP.** This is a gap inventory; an SSP requires control narratives + parameter values + responsibility assignments + evidence catalog + 3PAO/auditor coordination.
2. **Legal opinion.** ITAR/EAR classification, GDPR processor-vs-controller determination, HIPAA BAA scope determination — all REQUIRES-LEGAL.
3. **Operating effectiveness evidence.** Type 1 = design; Type 2 = operating effectiveness. This matrix is design-only.
4. **3PAO / auditor scoping.** That requires 3PAO selection, scope discussion, and engagement letter.

## What Stevie CAN do with this matrix family

1. **Show adopters the kingdom's compliance readiness story** — which frameworks, which controls, which gaps.
2. **Prioritize MoatGhost + Architect work** by the universal gaps above.
3. **Self-attest to SSDF** (closest to ready of any framework) once the VDP gap closes.
4. **Negotiate adopter contracts** by knowing exactly what's mapped vs gap.
5. **Inform Captain Track-call** with realistic month-0 → month-N timelines per framework.
6. **Establish the kingdom's ConMon baseline** by knowing what's in scope to monitor.

## Provenance and integrity

All 15 matrices written by the Marshal during overnight unattended run, 2026-05-06. Read-only audit; no live system queried that wasn't already part of tonight's earlier phases (A through D). No PII, PHI, CHD, or CUI processed (architectural floor — Unheaded never sees these data classes).

**This audit document itself confirms: the matrix family is a foundational gap inventory at parent-control depth. The omissions catalogued above are the work that converts the gap inventory into framework-specific SSPs.**

Marshal acknowledges Stevie's directive: *"audit and verify we omit nothing."* The omissions are catalogued. Each is a deliberate scope choice with a recommendation for expansion. **Nothing was silently dropped.**

Marshal signs off Phase E completeness audit. The E1-E15 matrix family is complete to gap-inventory depth.
