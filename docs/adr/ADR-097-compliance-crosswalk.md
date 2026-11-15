<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-097 — Compliance crosswalk: common controls, publisher denominators, evidence that can fail

**Status:** Accepted (Stevie, 2026-11-03, decision pack DP-603). Previously: Proposed
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
   and validates it (strict YAML, many-to-many). `pkg/compliance` was left
   as is at the time; the legacy engine and its 1:1 mappings
   (`pkg/compliance/{engine.go,audit,controls,report,standards}`) were
   deleted 2026-10-26 (S-REAL-STEEL E1 G10c), nothing ever called them.

2. **Denominators come from the publishers.** Every framework file holds
   the complete requirement list at a stated granularity, generated where a
   machine-readable release exists (`scripts/compliance/import-frameworks.py`,
   which records source URL, retrieval time and sha256):
   NIST SP 800-53 Rev 5.2.0 (1014), CSF 2.0 (106 subcategories), SP 800-171
   r3 (97), SP 800-218 SSDF (19 practices) from NIST OSCAL; FedRAMP Rev 5
   Low/Moderate/High (156/323/410) from FedRAMP's own baseline workbook
   (FedRAMP labels it legacy since 20x). SOC 2 (33 common criteria) and
   ISO/IEC 27001:2022 Annex A (93) are authored from their published
   numbering with titles omitted, both texts being copyright. CIS Ubuntu
   Linux 24.04 LTS Benchmark v1.0.0 (Level 2 Server 312, Level 1 Server 251,
   added 2026-09-30) comes from ComplianceAsCode's BSD-licensed transcription
   pinned to a commit, IDs and levels only (CIS text is CC BY-NC-SA 4.0);
   the hosts run Ubuntu 25.10 and CIS publishes LTS benchmarks only, so it is
   the nearest benchmark, not an exact one. A test pins every count.

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
     FAILING when any is FAIL, INCOMPLETE when any is stale or unassessed,
     DECLARED when a mapped control reads declared configuration (see the
     2026-11-06 amendment), UNMAPPED when no control maps to it;
   - EVIDENCED means "supported by passing evidence", never "satisfied": a
     mapping covers part of a requirement at most, and the page says so;
   - every figure is shown against the framework's full total; unmapped
     requirements are counted.

6. **Dashboard.** `/compliance` and `/api/v1/compliance/{summary,
   frameworks/{id}}` in dashboard-backend, read-only, catalog and evidence
   mounted read-only. The overlap matrix (controls x frameworks) is the
   "attest once" view.

### Amendment (2026-09-30): per-source mappings

An evidence source may carry its own mapping (`maps: {framework: [ids]}`
on the evidence entry). A requirement mapped that way follows that
source's evidence alone. Before this, a requirement was EVIDENCED only
if every whole control mapped to it passed, and that fit poorly with a
benchmark that has one recommendation per check. One failing service
(nginx on west) turned all 21 CIS 2.1.x services FAILING. CIS L1 Server
read 1 evidenced and 104 failing; the same evidence now reads 59
evidenced and 46 failing.

The rules:

- A control maps a given framework either at control level or per
  source, never both.
- Derived frameworks inherit per-source mappings, as they already
  inherit control-level ones.
- A source's status is judged against its control's freshness window.
- The CSV export lists each source-level contributor with its latest
  record.

The CIS mappings were converted by script. The script asserted that each
control's set of CIS IDs is identical before and after (12 of 12
controls).

### Amendment (2026-11-06): declared is not evidenced

Stevie, 2026-11-06, on FND-037. A gate script reads files, so a passing
control that rests on one says what is written, not what runs. Such a control
carries `declared:`, naming what it reads in place of the running system. A
pass then reads **DECLARED**, never PASS. A requirement with no failure and no
gap, where a contributor is DECLARED, reads **DECLARED**, never EVIDENCED, and
it is counted, so every denominator holds. Failing, stale and missing evidence
behave as before.

A test holds every gate-script control to this, so a new one cannot reach PASS
by reading a file; `UH-PQC-01` is the one exception, since the repository is
what it makes a claim about. Moving a control back to PASS takes evidence read
from what runs (FND-037's verify), not a change to this list.

The figures fell, as expected. Same evidence file (2026-11-07), evidenced
before and after, then declared: controls passing 19 to 13, with 6 declared;
800-53 13 to 9 (4); FedRAMP Low 5 to 4 (1), Moderate 9 to 8 (1), High 10 to 8
(2); 800-171 5 to 3 (2); SSDF 4 to 3 (1). SOC 2 (0), ISO 27001 (6), CSF (2) and
CIS L1 and L2 (136, 141) did not move.

### Amendment (2026-11-15): host-service evidence kind (FND-037)

FND-037's verify asks for evidence "read from what runs". The `host-service`
kind is that evidence for UH-AUTHN-01 (S-EVIDENCE-RUNS Phase 5).

- **The ref.** `<host>:<service>`. The service comes from a fixed table in the
  code (`crosswalk.HostServices`), so the catalog names a service, never a
  command, a container, a port or a path. A ref whose service is not in the
  table, is exempt, or does not run on that host is refused when the catalog
  loads.
- **Per service, the table gives:** how it runs on each host (a compose
  container name, or a systemd unit and its package), its port from
  `pkg/ports`, one protected route with its method, and its auth form:
  `AUTH_ENABLED=true` (pkg/auth), or a named token variable that must be set
  (the same table `check-service-auth.sh` keeps as `TOKEN_AUTH`). A test fails
  when the table and the gate's first-party set differ, so the two cannot
  drift. The OIDC issuer is exempt in both, for the gate's reason, and has no
  ref.
- **Pass condition.** Pass = the service is running, its auth setting is on,
  and a TLS request with no credentials to the protected route answers 401.
  Not running where the table says it runs = fail. Auth setting off or absent
  = fail. Any answer but 401 = fail ("answers without a login" for a 2xx).
  docker, systemd or ssh unreachable, or the probe not answering at all = no
  record, so the source is not assessed.
- **The route.** A GET where the service has a protected read. Two services
  have none: the Kanban board opens every read by design, so its probe is a
  DELETE of the task collection with no id (401 with auth on; a 400 from the
  handler with auth off, and nothing is deleted either way); logstore-writer
  has one route, a POST, and the probe sends an empty body (the token is
  checked before the body is read). Akira's only API route is an open read, so
  its probe asks for `/`, which the auth layer refuses before the mux.
- **Secrets.** A container's or a unit's environment holds passwords and
  tokens. The command that reads it prints only the auth keys: the
  `AUTH_ENABLED` entry as written, and for a token variable the word `set` or
  nothing, never its value. The collector then parses that output and keeps
  the same keys only, so a reader that printed more would still leak nothing
  into a record, a detail, an error or a log line. A test feeds it a full
  environment with canary values and looks for them everywhere.
- **TLS.** The probe verifies the service's certificate against the host's CA
  file (`/etc/unheaded/tls/ca.pem`; the path per host is in the table).
  Timeouts are 5 s and the body read is bounded.
- **Effect on figures.** A `host-service` record is a reading of what runs, so
  UH-AUTHN-01 can leave DECLARED. It keeps `declared:` until every host that
  runs a first-party service is read: west is read now; east is read after
  its redeploy (S-E1-LIVE Step 218). The figures move when `declared:` goes,
  which is the point of FND-037.

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
- ~~Whether `pkg/compliance`'s standards are retired or migrated.~~ Retired: deleted 2026-10-26 (E1 G10c); the crosswalk is the only control catalogue.
