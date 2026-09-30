<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-097 — Compliance crosswalk: common controls, publisher denominators, evidence that can fail

**Status:** Proposed
**Date:** 2026-09-30
**Related:** ADR-093 (a gate is not a gate until it has failed), ADR-062
(LICH), ADR-083 (terminology: user-facing "NIST 800-207 Zero Trust"),
`docs/compliance/control-matrix/` (the May 2026 gap matrices and their
scrutiny, which this does not replace).

## Context

Stevie: the whole platform should be "incredibly easy to audit" for SOC 2,
NIST 800-53, ISO 27001, FedRAMP and the rest, with overlapping controls
between attestations: one piece of engineering evidence supporting many
frameworks' requirements at once.

What existed:

- `pkg/compliance` (11K lines): five standards, ~250 authored controls,
  `Control.Mappings map[string]string` (one requirement per framework, so
  no many-to-many), no checker registered anywhere (every control
  `NotAssessed`), and nothing outside the package imports it.
- `docs/compliance/control-matrix/`: 16 framework gap matrices. Their own
  scrutiny doc (01) found the "MAPPED" claims unfalsifiable and the status
  vocabulary inconsistent.
- Real, machine-checkable evidence: CI gates proven to fail
  (`scripts/check-gates-can-fail.sh`), govulncheck, gosec, gitleaks,
  cargo-audit, grype on the SBOM, fuzzing, signed commits.

Consulted: Inquisitor (crosswalk shape, honesty rules), Architect
(placement, pipeline), Developer (types, validation, tests).

## Decision

1. **Common controls are data in the repo.** `compliance/catalog/controls.yaml`
   holds each Unheaded control: statement, evidence sources, freshness
   window, mappings (a list of requirement IDs per framework), and a
   rationale for the mappings. Changes are reviewed, signed commits, which
   is itself change-management evidence. `pkg/compliance/crosswalk` loads
   and validates it (strict YAML, many-to-many). `pkg/compliance` is left
   as is; retiring its 1:1 mappings is a later decision.

2. **Denominators come from the publishers.** Every framework file holds
   the complete requirement list at a stated granularity, generated where a
   machine-readable release exists (`scripts/compliance/import-frameworks.py`,
   which records source URL, retrieval time and sha256):
   NIST SP 800-53 Rev 5.2.0 (1014), CSF 2.0 (106 subcategories), SP 800-171
   r3 (97), SP 800-218 SSDF (19 practices) from NIST OSCAL; FedRAMP Rev 5
   Low/Moderate/High (156/323/410) from FedRAMP's own baseline workbook
   (FedRAMP labels it legacy since 20x). SOC 2 (33 common criteria) and
   ISO/IEC 27001:2022 Annex A (93) are authored from their published
   numbering with titles omitted, both texts being copyright. A test pins
   every count.

3. **Derived frameworks.** A framework may be `derived_from` another
   (FedRAMP baselines from 800-53). Controls map to the base only; the
   loader projects each mapping onto the derived framework's requirements.
   Mapping directly to a derived framework is rejected: one mapping, no
   drift.

4. **Evidence must be able to fail.** A source is a GitHub Actions job
   (conclusion on the branch's latest completed run), a gate script
   (`scripts/*.sh` exit status), or commit signatures. A job whose steps
   are `continue-on-error` is not evidence. A skipped, cancelled or
   timed-out observation produces no record: it says nothing about the
   control. `cmd/compliance-evidence` collects into
   `var/compliance/evidence.json` (git-ignored, bounded history).

5. **Honesty rules, enforced in code, not in the page:**
   - a control is PASS only when every source's latest record passes and is
     within the freshness window; a source with no record makes it
     NOT_ASSESSED; an old pass makes it STALE; any failure makes it FAIL,
     and an old failure stays FAIL (it is not evidence of a fix);
   - a requirement is EVIDENCED only when every mapped control is PASS,
     FAILING when any is FAIL, INCOMPLETE otherwise, UNMAPPED when no
     control maps to it;
   - EVIDENCED means "supported by passing evidence", never "satisfied": a
     mapping covers part of a requirement at most, and the page says so;
   - every figure is shown against the framework's full total; unmapped
     requirements are counted.

6. **Dashboard.** `/compliance` and `/api/v1/compliance/{summary,
   frameworks/{id}}` in dashboard-backend, read-only, catalog and evidence
   mounted read-only. The overlap matrix (controls x frameworks) is the
   "attest once" view.

## Consequences

- The first real run shows how little is evidenced: 800-53 12 of 1014,
  FedRAMP Moderate 10 of 323, SOC 2 Security 3 of 33. That is the honest
  starting point. Coverage grows one evidenced control at a time; the
  number never moves because a mapping was added without evidence.
- Much of every framework is policy, people and process (training, HR,
  physical security, contracts). Machine evidence will not cover it. Those
  controls need an evidence kind for signed attestations with an expiry,
  still to be designed; until then they are UNMAPPED, not claimed.
- Mappings are Unheaded's judgement. Each carries a rationale so a reviewer
  can contest it, and they should be checked against published
  crosswalks (NIST OLIR, the AICPA TSC mappings, CSA CCM) before any of
  this is shown to an assessor.

## Open

- Host and runtime evidence (sysctl posture via the host agent, TLS
  configuration, container hardening) as new source kinds.
- Scheduling: `cmd/compliance-evidence` is run by hand; a timer or a CI
  job that publishes evidence is a later step.
- Attestation evidence for non-technical controls (above).
- Whether `pkg/compliance`'s standards are retired or migrated.
