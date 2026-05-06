# Compliance Matrix Status Enum — Formal Taxonomy

**Date:** 2026-05-06
**Author:** Marshal (post-shift extension, F9 of S1 remediation)
**Authority:** Resolves scrutiny finding **S1** in `01-scrutiny-2026-05-06.md` ("the status enum is logically corrupt").
**Scope:** Governs all status assignments in the 15 framework matrices in `docs/compliance/control-matrix/` and any future matrix added to that family.
**Status:** Draft v1. Pending MoatGhost inter-rater pilot (§3) before promotion to LOCKED.

---

## 0. Why this document exists

The matrix family authored 2026-05-06 grew its label vocabulary by accretion. Scientist's S1 critique correctly identified that **the boundaries between MAPPED / PARTIAL / GAP / N/A / N/A-DESIGN / ADOPTER-OWNS / DEFERRED / UNVERIFIED / PARAM-GAP / MAPPED-CONTRIBUTES / PARTIAL-CONTRIBUTES / REQUIRES-LEGAL** were never declared, and inter-rater reliability was never measured. Cross-matrix comparison breaks where one author wrote `PARTIAL` and another wrote `PARTIAL-CONTRIBUTES` for substantively-equivalent kingdom postures.

This document declares the taxonomy, draws decision criteria sharply, gives each label a real example from the existing matrix family (with file path), specifies the inter-rater reliability test, defines the audit trail for status changes, and estimates migration effort.

The kingdom commits: **after this taxonomy is locked, no new label may be introduced without an ADR amendment.** The accretion ends here.

---

## 1. Formal Label Set

There are **twelve** terminal labels, organized into five families. STUCK / CAPTURED / INTENTIONAL / ENG-DECLARED / CONFIRMED-CLEAN — labels enumerated in S1 as "appearing across the matrix family" — were investigated and found to **not appear inside matrix rows**. STUCK is a Skip-Protocol convention belonging to battle plans (e.g. `docs/internal/battle-plans/S-WEST-BOOTSTRAP-battle-plan.md` line 25). The matrix family inherits no obligation to support those terms; this taxonomy excludes them.

### Family A — Coverage claims (kingdom owns the control)

#### A1. `MAPPED`
- **Definition:** Kingdom owns this control and satisfies it directly with shipped, operating, evidenced kingdom artefacts. Every clause of the control text is addressed.
- **Decision criteria (ALL must hold):**
  1. Control applies to Unheaded as a system, not solely to an adopter.
  2. Kingdom has shipped code, configuration, runbook, or doctrine that addresses every sub-condition of the control text.
  3. Evidence is reproducible — a reviewer can run a command, follow a citation, or inspect a file path that demonstrates coverage.
  4. The evidence is operating, not merely scheduled. (Sentinel SEN3, SEN5, SEN7 corrections applied.)
  5. No required sub-condition is qualified with "Gap:" in the matrix cell.
- **Example:** `nist-800-53-2026-05-06.md` line 52 — `AC-14 Permitted Actions Without Identification | MAPPED | /health, /ready, /metrics are unauth by design (per CLAUDE.md). Documented exceptions.` Every AC-14 clause is met by a stable kingdom posture with citation.
- **Counter-example (do NOT use MAPPED):** `nist-800-53-2026-05-06.md` line 74 — AU-2 currently shows MAPPED but Scientist's S2 critique correctly identifies four sub-conditions ((a)-(d)) not all addressed. Under this taxonomy AU-2 must move to PARTIAL.

#### A2. `MAPPED-CONTRIBUTES`
- **Definition:** Kingdom provides infrastructure that *supports* an adopter's compliance for this control, but the control itself is **fully discharged at the adopter level**. Kingdom is not the duty-holder.
- **Decision criteria (ALL must hold):**
  1. Duty-holder for this control is the adopter (Covered Entity, controller, processor, business, etc.) per the framework's text.
  2. Kingdom infrastructure — when correctly configured by the adopter — would let them satisfy the control.
  3. Adopter could fail this control even with kingdom infrastructure present (e.g., by misconfiguring it).
  4. There is no kingdom-side gap left open (in which case use `PARTIAL-CONTRIBUTES`).
- **Example:** `hipaa-2026-05-06.md` line 22 — `308(a)(1)(ii)(D) | Information system activity review | R | MAPPED-CONTRIBUTES | pkg/auth.AuditLogger; eBPF traceability; Wotan log aggregation; Sentinel daily detection loop.` HIPAA requires the **CE** to perform the review; kingdom *enables* it.
- **Distinction from MAPPED:** MAPPED says "kingdom satisfies the control." MAPPED-CONTRIBUTES says "kingdom hands the adopter the bricks; adopter builds the wall."

#### A3. `PARTIAL`
- **Definition:** Kingdom has *some* coverage of the control but a specific gap is named in the cell. Coverage is not yet complete.
- **Decision criteria (ALL must hold):**
  1. Same scope test as MAPPED — control applies to kingdom.
  2. At least one sub-condition is addressed by kingdom artefacts.
  3. At least one sub-condition is **explicitly named as a gap** in the cell, after the word "Gap:" or equivalent.
  4. The gap is concrete enough to be turned into a backlog item (not aspirational hand-waving).
- **Example:** `nist-800-53-2026-05-06.md` line 47 — `AC-7 Unsuccessful Logon Attempts | PARTIAL | pkg/auth handles auth failures; rate limiter at edge (HAProxy). Gap: no documented lockout policy.` Some coverage; specific gap named.
- **PARTIAL vs MAPPED boundary:** if any sub-condition is materially un-addressed and that under-coverage is documented in the cell, use PARTIAL. If the cell is entirely positive coverage prose with no "Gap:" qualifier, MAPPED is the candidate — but pause and run it through the falsification test in §1.A1 criterion 4 before committing.

#### A4. `PARTIAL-CONTRIBUTES`
- **Definition:** Kingdom *contributes* infrastructure toward an adopter-discharged control, AND has a kingdom-side gap that diminishes the contribution.
- **Decision criteria (ALL must hold):**
  1. Same duty-holder test as MAPPED-CONTRIBUTES.
  2. Some kingdom infrastructure exists.
  3. A kingdom-side gap is named (not merely an adopter-side gap — that's MAPPED-CONTRIBUTES with adopter caveat).
- **Example:** `hipaa-2026-05-06.md` line 31 — `308(a)(4)(ii)(C) | Access establishment & modification | A | PARTIAL-CONTRIBUTES | pkg/auth supports modification; tonight's RBAC review F1 — unheaded-armory over-grants block confident least-privilege claim.` Adopter is duty-holder; kingdom contributes; kingdom has a known RBAC over-grant.

### Family B — Open work (kingdom should fix)

#### B1. `GAP`
- **Definition:** Control applies to the kingdom AND kingdom has not addressed it. The kingdom is the duty-holder for closure.
- **Decision criteria (ALL must hold):**
  1. Control applies to Unheaded.
  2. No kingdom artefact materially addresses any sub-condition.
  3. Kingdom *should* close this gap — it is a legitimate kingdom obligation, not adopter delegation.
  4. There is no plan-of-record yet (else `DEFERRED`).
- **Example:** `nist-800-53-2026-05-06.md` line 48 — `AC-8 System Use Notification | GAP | No login banner.` Kingdom is on the hook; nothing exists; no plan yet.
- **GAP vs PARTIAL:** PARTIAL has *some* coverage with named gap. GAP has effectively zero coverage.

#### B2. `DEFERRED`
- **Definition:** Kingdom plans to address; the plan exists in writing (battle plan, ADR, runbook, or roadmap entry); ship date is named or constrained.
- **Decision criteria (ALL must hold):**
  1. Control applies to Unheaded.
  2. Kingdom acknowledges the obligation.
  3. **Specific cite to a plan-of-record** in the cell — battle plan filename, ADR number, or runbook path. Not "we should do this."
  4. The plan names a closure target (sprint, phase, age, or framework parameter).
- **Example:** `nist-800-53-2026-05-06.md` line 268 (PT-6) and `gdpr-2026-05-06.md` line 24 use this concept partially; the cleanest existing form is the cross-reference in `hipaa-2026-05-06.md` line 40 — backup plan citing ADR-035 + ADR-064 with explicit "deferred per Stevie." Future DEFERRED entries MUST follow this pattern: status + plan cite + reason.
- **DEFERRED vs GAP:** GAP has no plan; DEFERRED has a written plan. If the cell says "should do X" with no cite, it's GAP, not DEFERRED.

### Family C — Out of scope (kingdom is not the duty-holder)

#### C1. `ADOPTER-OWNS`
- **Definition:** Adopter is the duty-holder; kingdom is not in the loop. **Kingdom provides nothing here**, by design or by scope.
- **Decision criteria (ALL must hold):**
  1. Framework text places duty on the adopter (controller / CE / business / customer).
  2. Kingdom architecturally cannot satisfy the control (not its layer of the stack).
  3. Kingdom infrastructure neither helps nor hinders.
- **Example:** `gdpr-2026-05-06.md` line 16 — `5(1)(a) | Lawfulness, fairness, transparency | ADOPTER-OWNS | Unheaded never sees personal data by design (zero-customer-data-access posture per CLAUDE.md). Adopter chooses lawful basis.`
- **ADOPTER-OWNS vs MAPPED-CONTRIBUTES:** if kingdom *infrastructure helps the adopter*, use MAPPED-CONTRIBUTES. If kingdom is genuinely silent on this control, ADOPTER-OWNS.
- **ADOPTER-OWNS vs N/A-DESIGN:** ADOPTER-OWNS means *somebody* must satisfy it — just not the kingdom. N/A-DESIGN means nobody must satisfy it in this deployment because of architectural choices.

#### C2. `N/A`
- **Definition:** Control is not applicable to a software platform of this class **in any deployment**. The control text targets a different system class (paper-record systems, physical facilities with staff, hardware-only products, etc.).
- **Decision criteria (ALL must hold):**
  1. Control text is structurally incompatible with software-as-a-platform.
  2. Inapplicability is independent of the kingdom's specific architectural choices — *any* platform of this kind would be N/A.
  3. No adopter deployment can change the answer.
- **Example:** `nist-800-53-2026-05-06.md` line 54 — `AC-18 | Wireless Access | N/A | n/a.` (Unheaded ships software; wireless-network policy targets physical-layer ops, not the substrate.)
- **N/A vs N/A-DESIGN:** N/A says "this control was never going to apply." N/A-DESIGN says "this control could apply, but kingdom architectural choices make it not apply."

#### C3. `N/A-DESIGN`
- **Definition:** Control would apply in principle, but **kingdom architectural choices make it inapplicable** (canonical case: "no PII processed" makes most privacy controls inapplicable).
- **Decision criteria (ALL must hold):**
  1. Control text *could* apply to a software platform.
  2. A specific kingdom architectural choice (cited in the cell or in CLAUDE.md) places the kingdom outside the control's scope.
  3. The architectural claim is **falsifiable** — you can test whether the kingdom's design holds.
  4. If the design changes, the status changes.
- **Example:** `nist-800-53-2026-05-06.md` line 264 — `PT-2 | Authority to Process PII | N/A-DESIGN | Architecturally none — Unheaded does not process PII.`
- **Caveat (S3 in scrutiny):** the "no PII" architectural floor is asserted, not yet measured. Every N/A-DESIGN entry citing the no-PII floor is conditional on F-PII (the falsifiability test in `01-scrutiny-2026-05-06.md` §S3). Until the PII telemetry scan completes and the architectural floor is demonstrated empirically, N/A-DESIGN entries based on the no-PII floor carry an implicit asterisk.

### Family D — Honest uncertainty

#### D1. `UNVERIFIED`
- **Definition:** Author was not certain at write-time and **did not verify**. The control may be MAPPED, PARTIAL, or GAP — review needs to determine which.
- **Decision criteria (ALL must hold):**
  1. The matrix author admits uncertainty in the cell.
  2. The cell names what verification is needed (a file to check, a CI run to inspect, a person to ask).
  3. UNVERIFIED is **time-bounded** — it must be re-statused within the next MoatGhost daytime check, no later than 14 days from creation.
- **Example:** `gdpr-2026-05-06.md` line 65 — `37 | DPO appointment | UNVERIFIED | Whether Unheaded itself meets the DPO threshold (Art. 37(1)) is unverified — single-developer kingdom likely below threshold; recheck if/when staff scale.`
- **UNVERIFIED vs GAP:** UNVERIFIED says "I don't know." GAP says "I know — there's nothing." UNVERIFIED is honest agnosticism; GAP is honest acknowledgement of absence.
- **UNVERIFIED vs PARTIAL:** PARTIAL is "I know — there's some, with named gap." UNVERIFIED is "I haven't checked."
- **Audit-context note:** in an audit setting, UNVERIFIED is treated as GAP unless cleared. Authors should expedite UNVERIFIED → terminal-status conversion when an audit window opens.

### Family E — Special handling

#### E1. `PARAM-GAP`
- **Definition:** Kingdom owns the control AND substantively addresses the control's intent, but a specific framework-defined parameter value is not met.
- **Decision criteria (ALL must hold):**
  1. The control would be MAPPED on intent.
  2. The framework specifies a parameter value (audit log retention = N days, password complexity = M characters, FIPS module validation status, etc.).
  3. The kingdom's parameter value differs from the framework's required value.
  4. The cell names the parameter and the divergence.
- **Example:** `nist-800-53-2026-05-06.md` line 452 — `SC-13 | crypto algorithms | FIPS 140-3 module-validated | PARAM-GAP — cloudflare/circl FIPS-track but not 140-3 module-validated.` Crypto exists; the FIPS-validation parameter is unmet.
- **PARAM-GAP vs PARTIAL:** PARTIAL is "missing sub-condition." PARAM-GAP is "all sub-conditions present; numeric/parametric value off."

#### E2. `REQUIRES-LEGAL`
- **Definition:** Decision needs Barrister or external counsel. **Not a Marshal/MoatGhost lane.** Authors must not unilaterally promote REQUIRES-LEGAL → another status.
- **Decision criteria (ALL must hold):**
  1. The control's resolution depends on a legal determination (export classification, processor-vs-controller assignment, BAA scope, statutory derogation, etc.).
  2. The kingdom's competence does not include making that determination.
  3. A legal-review action is named or recommended.
- **Example:** `itar-ear-2026-05-06.md` line 23 — `5D002 — encryption software | REQUIRES-LEGAL | TLS 1.3 + ML-DSA-65 + SLH-DSA implementations are subject to 5D002. May qualify for License Exception ENC, License Exception TSU (publicly-available source), or no-license-required (NLR) depending on adopter. Need BIS notification under §740.13(e) for publicly-available source.`
- **REQUIRES-LEGAL vs UNVERIFIED:** UNVERIFIED is technical uncertainty resolvable by kingdom skills. REQUIRES-LEGAL is jurisdictional uncertainty resolvable only by counsel.

### Forbidden / retired labels

The following labels appeared in S1's enumeration but are NOT part of this taxonomy. New entries MUST NOT use them:
- `CONTRIBUTES` (without MAPPED- or PARTIAL- prefix) — too ambiguous; pick the prefixed form.
- `CONFIRMED-CLEAN`, `INTENTIONAL`, `STUCK`, `CAPTURED`, `ENG-DECLARED` — these belong to other artefact families (battle plans, sprint reports, CI gates), not the compliance matrices. If a matrix author wants to convey "we tested and found no issue," use `MAPPED` with a citation to the test.

---

## 2. Decision Tree

A reviewer encountering a control in a framework reference and assigning a label must walk this tree top-to-bottom. The first terminal hit is the assigned label.

```
START → Q1
                                                                              
Q1: Is the control's resolution a legal determination
    (export classification, processor/controller, BAA scope,
    statutory derogation, sanctioned-party screening)?
  ├─ YES → REQUIRES-LEGAL                                               [E2]
  └─ NO  → Q2

Q2: Is the control structurally inapplicable to a software platform
    of this class, regardless of architecture (e.g. wireless access
    rules, paper records, physical guards)?
  ├─ YES → N/A                                                          [C2]
  └─ NO  → Q3

Q3: Does a specific kingdom architectural choice place this control
    out of scope (e.g. "no PII processed" → privacy controls;
    "no CHD stored" → PCI Req 3 sub-controls)?
  ├─ YES → N/A-DESIGN                                                   [C3]
  └─ NO  → Q4

Q4: Per the framework text, is the duty-holder the adopter
    (controller / CE / business / contractor), not the kingdom?
  ├─ YES → Q4a
  └─ NO  → Q5

  Q4a: Does the kingdom provide infrastructure that materially
       helps the adopter satisfy the control?
    ├─ YES → Q4b
    └─ NO  → ADOPTER-OWNS                                               [C1]

  Q4b: Is there a kingdom-side gap that diminishes the contribution
       (named in the cell)?
    ├─ YES → PARTIAL-CONTRIBUTES                                        [A4]
    └─ NO  → MAPPED-CONTRIBUTES                                         [A2]

Q5: Has the author actually verified what kingdom has shipped
    against this control (file paths, CI logs, runbook cites,
    operating evidence)?
  ├─ NO  → UNVERIFIED                                                   [D1]
  └─ YES → Q6

Q6: Is there a written plan of record (battle plan, ADR, runbook
    file, or roadmap entry) for closure, with a specific cite?
  ├─ YES (and nothing shipped yet) → DEFERRED                           [B2]
  └─ NO / OR (something shipped) → Q7

Q7: Has the kingdom shipped any artefact that addresses any
    sub-condition of the control?
  ├─ NO  → GAP                                                          [B1]
  └─ YES → Q8

Q8: Are ALL sub-conditions of the control addressed by shipped,
    operating, evidenced kingdom artefacts (no "Gap:" qualifier
    needed in the cell)?
  ├─ NO  → PARTIAL                                                      [A3]
  └─ YES → Q9

Q9: Does the framework specify a numeric / parametric value
    (retention days, FIPS validation level, password length, etc.)
    that the kingdom's value does NOT match?
  ├─ YES → PARAM-GAP                                                    [E1]
  └─ NO  → MAPPED                                                       [A1]
```

**Tie-breakers and edge cases:**
- If Q4 and Q3 are both YES (control applies to adopter only AND is also out-of-scope by design), prefer **N/A-DESIGN** — it carries more semantic weight than ADOPTER-OWNS for adopters reading the matrix.
- If the kingdom has shipped *and* there's a written plan for further work, label by what is shipped today (Q7 → Q8 path), not the plan. DEFERRED is reserved for cases where nothing has shipped.
- If ALL sub-conditions are addressed but one is *barely* addressed (e.g., a stub implementation), the conservative move is PARTIAL with the gap named. Reserve MAPPED for genuine completion.

---

## 3. Inter-Rater Reliability Test (Cohen's κ)

S1 demands a falsifiable test of the taxonomy's reproducibility. This section specifies that test.

### Procedure

1. **Sample frame.** Take the union of all status-bearing rows across the 15 framework matrices (≈1,645 rows total per the file-by-file count below). Stratify by framework so the sample reflects the family's distribution.
2. **Sample size.** N = 50 controls. Stratification: ≈3-4 controls per matrix, weighted by row count (more from `nist-800-53` and `fedramp-moderate` which together contribute ≈30% of rows).
3. **Reviewer pool.** Two independent raters:
   - **Rater A:** Marshal (or matrix-original-author proxy).
   - **Rater B:** MoatGhost daytime check (independent — must not be shown the existing status before re-rating).
4. **Procedure per control:**
   - Each rater receives: the control identifier (e.g. `AC-2`), the framework's official control text, and the kingdom's evidence inventory (CLAUDE.md excerpts, ADRs, code paths, runbooks). The raters do **NOT** receive the existing matrix cell's status or rationale prose.
   - Each rater walks the §2 decision tree independently and records a label from the §1 set.
5. **Compute Cohen's κ** across the 50-control sample using the standard formula:
   ```
   κ = (Po - Pe) / (1 - Pe)
   where:
     Po = observed agreement = (#agreements) / 50
     Pe = expected agreement by chance, computed from each rater's marginal label distribution
   ```
6. **Pass threshold:** **κ ≥ 0.6** (substantial agreement per Landis-Koch). κ < 0.6 means the taxonomy is not yet reproducible and the labels need sharper criteria; iterate §1 and re-test.
7. **Disagreement adjudication:** disagreements drive taxonomy refinement. For each disagreement, both raters write a one-line note on which decision-tree node they branched at and why. The notes feed §1 sharpening for the next iteration.

### When to run the κ test

- **Initial run:** within 14 days of this taxonomy's draft date (i.e. by 2026-05-20).
- **Recurring run:** every six months, or any time the label set changes (which would require an ADR, per §0).
- **Pre-audit run:** before any audit engagement uses these matrices as evidence.

### Reporting

The κ result is recorded in a new file `docs/compliance/control-matrix/03-kappa-YYYY-MM-DD.md` with: rater identities, sample list (control IDs only), each rater's label vector, the agreement matrix, computed κ, and a disagreement-notes section. If κ ≥ 0.6, the taxonomy is promoted from Draft to LOCKED. If κ < 0.6, this taxonomy v1 is revised and a v2 is drafted.

---

## 4. Audit Trail for Status Changes

Once a row is in a matrix file, its status may change because: a gap closes, evidence is found, an architectural choice changes, or a κ-test refinement re-classifies it. Every change must be auditable.

### Mechanism — append-only status-history footer per matrix

Each matrix file gains a `## Status Change History` section at the bottom. Entries are append-only. Format:

```markdown
## Status Change History

| Date       | Control ID  | Old Status        | New Status     | Reason                                                                                  | Cite                                                                |
|------------|-------------|-------------------|----------------|-----------------------------------------------------------------------------------------|---------------------------------------------------------------------|
| 2026-05-20 | AU-2        | MAPPED            | PARTIAL        | S2 falsification — control text (a)-(d) not all addressed; "frequency sufficient" undocumented | scrutiny S2; F10 falsifications run                                |
| 2026-05-22 | SC-13       | MAPPED            | PARAM-GAP      | Re-rated under formal taxonomy; circl FIPS-track but not 140-3 module-validated         | this taxonomy §1.E1; nist-800-53-2026-05-06.md line 452           |
| 2026-06-01 | AC-7        | PARTIAL           | MAPPED         | Lockout policy doc shipped at runbooks/security/auth-lockout.md                         | commit hash; runbook file                                            |
```

### Required fields

- **Date:** YYYY-MM-DD of the change.
- **Control ID:** the framework's identifier (e.g. `AC-2`, `5(1)(c)`, `8.5.1`, `308(a)(1)(ii)(D)`).
- **Old Status:** the prior label (must be a valid §1 label or empty for newly-added rows).
- **New Status:** the new label (must be a valid §1 label).
- **Reason:** one-line, prose, identifying the trigger. Should reference a falsification, a κ-test outcome, a closure event, or a scrutiny-finding remediation.
- **Cite:** one or more references — commit hash, runbook path, ADR number, scrutiny-finding ID, or a `docs/` filename. Bare prose without cite is not acceptable.

### Process

1. The author opening a status change drafts the change in the matrix row AND adds the history line in the same commit.
2. CI gate (`.github/workflows/matrix-status-check.yml` — to be authored as F12-adjacent work) verifies: (a) every matrix file has a `## Status Change History` section if it has any non-initial commits; (b) every cell change in matrix tables has a corresponding history-row in the same PR; (c) all history-row cite fields are non-empty.
3. The 14-day UNVERIFIED conversion rule (§1.D1) is enforced as a CI warning that surfaces UNVERIFIED rows older than 14 days.

### Why footer rather than separate change log

A single per-matrix audit footer keeps the change log co-located with the data. Separate change-log files invite drift (the matrix changes, the log doesn't). The footer is the source of truth and is parseable by tooling.

---

## 5. Migration Plan

### Counts (current as of 2026-05-06)

By scanning the 15 active framework matrices (excluding `00-completeness-audit` and `01-scrutiny`):

| Label              | Current count | Status under formal taxonomy                                  |
|--------------------|---------------|---------------------------------------------------------------|
| `MAPPED`           | 378           | Most need re-rating per §1.A1 criterion 4 (operating, not scheduled)  |
| `PARTIAL`          | 479           | Most stable; re-rate only those with implicit "Gap:" missing  |
| `GAP`              | 377           | Stable; verify no DEFERRED candidates lurk                    |
| `N/A`              | 117           | Re-rate distinguishing N/A vs N/A-DESIGN                      |
| `N/A-DESIGN`       | 25            | Stable in form; conditional on F-PII (S3 floor proof)         |
| `ADOPTER-OWNS`     | 179           | Re-rate splitting ADOPTER-OWNS vs MAPPED-CONTRIBUTES          |
| `DEFERRED`         | 2             | Stable; expand by promoting GAPs with extant plans            |
| `UNVERIFIED`       | 86            | Convert within 14 days per §1.D1 (large backlog)              |
| `PARAM-GAP`        | 3             | Stable; expand by re-rating SC-13-style cases                 |
| `MAPPED-CONTRIBUTES` | 79          | Stable                                                        |
| `PARTIAL-CONTRIBUTES` | 18         | Stable                                                        |
| `REQUIRES-LEGAL`   | 4             | Stable                                                        |
| **TOTAL**          | **≈1,747**    |                                                               |

(Total exceeds 1,645 because some matrices use compound forms — e.g., `MAPPED-CONTRIBUTES` rows are also caught by `MAPPED`.)

### Re-status workload estimate

| Migration class                                      | Estimated rows | Effort                                                               |
|------------------------------------------------------|---------------:|----------------------------------------------------------------------|
| MAPPED → PARTIAL (S2 over-claim correction)          | ~150           | 12-15 hours (one-line falsifiability test per row, per Scientist S2) |
| MAPPED → MAPPED (confirmed)                          | ~228           | 6-8 hours (review, no change)                                        |
| MAPPED → PARAM-GAP (numeric value mismatch found)    | ~10            | 1-2 hours                                                            |
| ADOPTER-OWNS → MAPPED-CONTRIBUTES (kingdom helps)    | ~60            | 4-6 hours                                                            |
| ADOPTER-OWNS → ADOPTER-OWNS (confirmed)              | ~119           | 2-3 hours                                                            |
| N/A → N/A-DESIGN (architectural reasoning explicit)  | ~50            | 3-4 hours                                                            |
| N/A → N/A (confirmed structural)                     | ~67            | 2 hours                                                              |
| GAP → DEFERRED (extant plan found)                   | ~25            | 3-4 hours                                                            |
| GAP → GAP (confirmed)                                | ~352           | 4-6 hours (review, no change)                                        |
| UNVERIFIED → terminal status (per §1.D1 14-day rule) | 86             | 8-10 hours (real verification work)                                  |
| PARTIAL → PARTIAL-CONTRIBUTES (clarify duty-holder)  | ~30            | 2-3 hours                                                            |
| PARTIAL → PARTIAL (confirmed)                        | ~449           | 4-6 hours (review, no change)                                        |
| Add Status Change History footer to 15 matrices      | 15 files       | 1-2 hours                                                            |
| **TOTAL ESTIMATED**                                  | **~1,641 rows reviewed; ~411 actually re-statused** | **~50-65 hours** |

This is **MoatGhost daytime work**. It is not Marshal lane (no execution-time pressure justifies overnight unattended re-rating). The output is the v1.1 matrix family with formal taxonomy applied.

### Migration execution order

1. **Pass A — taxonomy-mechanical changes** (~15 hours): N/A → N/A-DESIGN; ADOPTER-OWNS → MAPPED-CONTRIBUTES; GAP → DEFERRED. Pure re-classification by reading the cell.
2. **Pass B — UNVERIFIED expiry** (~10 hours): walk each UNVERIFIED row, do the verification, write the terminal status with cite.
3. **Pass C — MAPPED falsification** (~15 hours): apply Scientist S2 falsifiability test to each MAPPED row. Demote to PARTIAL where the test fails.
4. **Pass D — PARTIAL refinement** (~10 hours): identify PARTIAL → PARTIAL-CONTRIBUTES candidates by re-checking duty-holder.
5. **Pass E — κ test** (~5 hours): execute §3 procedure, document κ, iterate taxonomy if needed.
6. **Pass F — audit-trail backfill** (~5 hours): add Status Change History footer to all 15 matrices, populate with the Pass A-D changes as initial entries.

---

## 6. Style Guide for Writing New Matrix Entries

### Row format

Every status row in every matrix MUST conform to:

```
| <Control ID> | <Control Title or short text> | <STATUS> | <Evidence prose with file/line cites and explicit "Gap:" if PARTIAL> |
```

For frameworks with extra columns (HIPAA Required/Addressable, FedRAMP baseline, etc.), the STATUS column remains in the same position relative to evidence prose.

### Mandatory fields

1. **Control ID.** The framework's identifier verbatim. No paraphrasing.
2. **Status.** Exactly one of the §1 labels. Capitalized exactly as written. No invented labels.
3. **Evidence prose.** At minimum:
   - For `MAPPED` / `MAPPED-CONTRIBUTES`: name the kingdom artefact(s) with file path or identifier (e.g. `pkg/auth.AuditLogger`, `services/wotan/internal/signing/`, `ADR-043`, `runbooks/security/foo.md`). Multiple cites separated by semicolons. **No infrastructure-name dropping without specifying which control intent each cite addresses** (Scientist S2).
   - For `PARTIAL` / `PARTIAL-CONTRIBUTES`: name the coverage AND name the gap explicitly with the `Gap:` prefix or equivalent.
   - For `GAP`: state what's missing and (optionally) what would close it.
   - For `DEFERRED`: cite the plan-of-record by filename / ADR number and note the closure target.
   - For `N/A` / `N/A-DESIGN`: state the reason; for N/A-DESIGN, name the architectural choice and (where possible) cite CLAUDE.md or an ADR.
   - For `ADOPTER-OWNS`: state which adopter role holds the duty (controller, CE, business, contractor).
   - For `UNVERIFIED`: name what verification is needed and who should do it.
   - For `PARAM-GAP`: name the framework's parameter, the kingdom's actual value, and the divergence.
   - For `REQUIRES-LEGAL`: state which kind of legal determination is needed (export classification, BAA scope, etc.) and recommend Barrister or external counsel as appropriate.

### Citation requirements

- **File citations** use the form `path/to/file.go` (no leading slash). For specific lines: `path/to/file.go:123`.
- **Code package citations** use the form `pkg/<name>` or `services/<name>/internal/<sub>`.
- **ADR citations** use `ADR-NNN` with optional title (`ADR-043 Mímir's Law`).
- **Runbook citations** use the relative path `runbooks/<category>/<name>.md`.
- **CI workflow citations** use `.github/workflows/<name>.yml`.
- **Sentinel / test citations** use the test name or TestXxx Go test identifier.
- **Operating-evidence citations** (when claiming MAPPED on a detective control per Sentinel SEN3): include a recency marker — "last fired YYYY-MM-DD" or "30-day success rate N%".

### Forbidden practices (matrix author's no-fly list)

1. **Infrastructure-name dropping** without mapping each name to a sub-condition the control demands (Scientist S2). MAPPED rows must demonstrate intent-coverage, not just feature-listing.
2. **Scheduled-as-operating** (Sentinel SEN3, SEN5). "Sentinel runs daily" without a last-fired timestamp does not establish operating status; downgrade to PARTIAL until a recency cite is added.
3. **Internal red-team as third-party assessment** (Sentinel SEN7). BlackMage daily red-team is internal validation. Do not cite it for CA-8 / 11.3 / CIS 18 third-party requirements without the qualifier "Internal validation; NOT a substitute for independent third-party assessment required by [framework]."
4. **Inventing labels.** If you reach for a label not in §1, stop. Either the existing taxonomy fits (revisit §2 decision tree) or you've found a real gap requiring an ADR amendment to extend the taxonomy. Do not silently introduce a new label.
5. **Asserting architectural floors without proof.** N/A-DESIGN entries citing the no-PII floor must reference the F-PII falsifiability test (per scrutiny §S3) until that test runs.
6. **Headline-gap promotion without rubric** (Scientist S5). Headline gaps in matrix headers must be selected against a transparent severity / leverage / closability rubric, not author intuition.

### Worked example (MAPPED row)

```
| AC-3 | Access Enforcement | MAPPED | pkg/auth.RBACAuthorizer middleware on every service (ALL 10 services per CLAUDE.md S36 Four Pillars); Champion 3-rule gate on mutations (pkg/champion/, ADR-019); 64 auth tests pass (.github/workflows/test.yml last green commit YYYY-MM-DD). All AC-3 sub-conditions: (i) authorization decisions before access — RBACAuthorizer pre-handler; (ii) enforcement at access enforcement points — pkg/auth.Middleware on every service router; (iii) prevention of unauthorized access — Champion gate denies disallowed mutations. |
```

### Worked example (PARTIAL row)

```
| AU-2 | Event Logging | PARTIAL | (a) event determination: pkg/auth.AuditLogger logs auth events; eBPF Anamnesis Lite captures network events; structured zerolog covers app events. (b) coordination: Wotan log aggregation (pkg/logagg/) is the centralization point. (c) frequency: not formally specified — emission is event-driven, no review-frequency SOP. (d) review: Sentinel daily detection loop; Stevie reviews when convenient. **Gap:** sub-conditions (c) frequency-sufficient-for-investigation and (d) review-cadence-formal-SOP are not documented. Retention also limited to ring-buffer ~10s window per Sentinel SEN3 — see also AU-11 GAP. |
```

### Worked example (DEFERRED row, going forward)

```
| CP-2 | Contingency Plan | DEFERRED | Plan of record: ADR-064 active/active speced (deferred per Stevie 2026-04-XX). Closure target: Age 3 RAFT-fine-tune bare-metal phase. Backup-runbook will be authored at runbooks/recovery/cp-2-contingency.md. Until then, recovery is best-effort by single operator (cross-ref Sentinel SEN2, BlackMage BM4). |
```

---

## 7. Conformance gates

A matrix file conforms to this taxonomy when:

1. Every status cell uses exactly one §1 label (no compound or invented forms).
2. Every PARTIAL / PARTIAL-CONTRIBUTES cell names a specific gap.
3. Every DEFERRED cell cites a plan-of-record.
4. Every N/A-DESIGN cell names the architectural choice that scopes-out the control.
5. Every UNVERIFIED cell is < 14 days old or has been re-statused.
6. Every MAPPED cell on a detective control includes a recency cite.
7. The file has a `## Status Change History` section if any post-initial-commit changes have occurred.
8. Headline gaps in the matrix header are tagged against a transparent rubric (S5 remediation; rubric to be defined in F-rubric work).

The CI workflow `matrix-status-check.yml` (to be authored alongside this taxonomy's promotion to LOCKED) enforces (1)-(7) mechanically. (8) is reviewed manually at quarterly cadence.

---

## 8. Provenance & relationship to other documents

- This taxonomy resolves S1 of `01-scrutiny-2026-05-06.md` (Scientist's status-enum critique).
- This taxonomy is one of twelve scrutiny-remediation deliverables (F1-F12); see CLAUDE-MEMORY task list.
- The κ test in §3 is one of the Scientist's six recommended falsifications (specifically item 1 in that list).
- The audit trail in §4 supports any future operating-effectiveness audit (SOC 2 Type 2, FedRAMP ConMon, ISO 27001 surveillance) that demands historical status change records.
- This taxonomy does NOT resolve S2 (per-MAPPED-claim falsifiability), S3 (PII floor proof), S4 (cross-framework arithmetic), S5 (headline-gap rubric), S6 (verification-section functional-verification), S7 (CI cadence audit), or S8 (meta-omission risk). Those are tracked as separate F-items in the remediation list.

The matrix family is **honest at gap level and aspirational at coverage level** (per Sentinel's verdict). This taxonomy moves a meaningful slice of that aspiration toward auditability — but only the migration plan in §5 actually executes the move. The taxonomy is the precondition; the work is still ahead.

---

**END — `02-status-taxonomy-2026-05-06.md` v1 Draft.** Pending κ-test pilot in §3 before LOCKED promotion. No status changes elsewhere in the matrix family until κ ≥ 0.6 is demonstrated.
