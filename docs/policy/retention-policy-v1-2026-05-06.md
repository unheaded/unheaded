# Unheaded Kingdom — Data Retention Policy v1

**Version:** 1.0 (DRAFT — UNCOMMITTED)
**Date:** 2026-05-06
**Status:** Initial draft, not yet ratified. Pre-public-launch.
**Author:** Marshal (Phase F1 follow-on, post-scrutiny)
**Audience:** Stevie + future contributors + adopters operating an Unheaded deployment.
**Applies to:** All data classes produced, processed, stored, or replicated by the
Unheaded Kingdom (`github.com/unheaded/unheaded`) and any first-party adopter
deployment downstream of it.
**Supersedes:** Nothing. This is the kingdom's first written retention policy.

---

## 0. Why this exists

This policy closes a **cross-framework universal gap** surfaced in
`docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` (the Marshal-family
scrutiny doc). At least seven frameworks demand a written retention rule:

| Framework | Control | Demand |
|---|---|---|
| **GDPR** | Art. 5(1)(e) "storage limitation" | Personal data kept no longer than necessary |
| **PCI DSS 4.0** | 10.5 / 9.4.1.1 | Audit logs ≥ 1 year, ≥ 3 months immediately accessible |
| **HIPAA** | §164.316(b)(2)(i) | Documentation ≥ 6 years from creation or last effect |
| **NIST SP 800-53** | AU-11 | Audit-record retention defined and enforced |
| **CCPA / CPRA** | §1798.140(o) + §1798.105 | Disclosed retention windows + right-to-delete |
| **ISO/IEC 27001** | A.5.33 | Records protected from loss/destruction/falsification per the records-management policy |
| **CIS Controls v8** | 8.10 | Retain audit logs across enterprise; minimum 90 days local |
| **FedRAMP Moderate** | AU-11 (parameterized) | **90 days online + 1 year archive** for audit logs |

A single policy document satisfying the union of these criteria closes ~7
universal-gap rows simultaneously across the matrix family. The policy must
enumerate per-data-class retention, name the framework that drives each floor,
state the legal basis on which an adopter may delete sooner (right-to-erasure),
and identify the procedural enforcement mechanism (cron job, manual process,
code-level limit, or IMMUTABLE-BY-DESIGN).

---

## 1. Doctrine and scope

### 1.1 The Kingdom does not see customer PII (architectural floor — overstated, see §1.3)

By architectural design, Unheaded does not perform first-class processing of
end-user PII / PHI / cardholder data / CUI. The "head" (the adopter's
application) processes that; the "unheaded" infrastructure (this kingdom)
processes packet metadata, infrastructure telemetry, source code, ADRs, and
operator-side audit/operational data. The Zero User Data Access principle is
documented in `CLAUDE.md` §"Security First, Always".

### 1.2 What this floor means for retention

For data classes covered by §1.1, the retention drivers are **operational +
audit-compliance**, not **subject-rights** (GDPR Art. 17 / CCPA §1798.105). The
adopter's data-protection officer remains responsible for retention of any data
their head application processes — that is **out of scope** for this policy.

### 1.3 Where the floor leaks (overstated-claim correction)

Per the scrutiny doc §S3, the "no PII" claim is overstated. PII can leak into
kingdom-owned data classes via three vectors:

1. **Packet metadata via eBPF** — IP addresses are personal data under GDPR
   Art. 4(1) in EU contexts; HTTP query strings can carry email/identifiers.
2. **Operator-pasted content in service logs** — an operator pastes a customer
   ticket into a debug log; aggregation captures it.
3. **Operator-pasted content in Zhen AI conversations** (`zhen_conversations`
   table) — operator asks Zhen about a customer, pastes the customer's email.

Therefore the retention windows below assume the **PII Containment** controls
(UH-PII-1..4 per Session 10 notes; `pii_mode: eu|strict` in
DataIsolationConfig) MAY be in effect for an adopter operating in an EU/strict
posture. When `pii_mode: strict` is set, the shorter of (this policy's window,
the strict-mode window) applies. Strict-mode windows are noted per class.

### 1.4 Permanent does not mean unmanaged

"Permanent" in this policy means "no scheduled deletion". It does NOT mean
"no review". Permanent classes (source code, ADRs, SBOMs, IR records, deployed
sealed casks) are subject to **annual review** to confirm they are still
relevant, accessible, and integrity-checked. The review cadence is captured
in §6.

### 1.5 Defer-to-IR-plan items

Several classes (backup retention, RTO/RPO-driven windows) are deliberately
deferred to the **IR plan v1** (task #52 / F5) and the **DR runbook**. Where
deferred, this document records the deferral and links forward.

---

## 2. Per-data-class retention table — summary view

Numbered to match the order in the original task brief. **Today-state** is
listed where it materially differs from policy. **STATUS** column uses
the formal taxonomy from F9 (the status-enum doc): `MEETS-FLOOR` / `BELOW-FLOOR`
/ `IMMUTABLE` / `UNVERIFIED` / `DEFERRED`.

| # | Data Class | Today (effective) | Policy v1 floor | Driver | Deletion basis | Enforcement | Status |
|---|---|---|---|---|---|---|---|
| 1 | Source code + git history | Permanent (git) | Permanent | Operational + audit | Cannot delete (immutable record of build) | git itself | IMMUTABLE |
| 2 | ADRs | Permanent (git) | Permanent | ISO A.5.33 + audit | Cannot delete (decision record) | git itself | IMMUTABLE |
| 3 | CI artefacts (default) | 30 days (`gpl-boundary.yml`) | **30 days default** (per-workflow exceptions §3.3) | Operational | Auto-expire (GHA) | GHA `retention-days:` | MEETS-FLOOR |
| 4 | Audit logs (`pkg/auth.AuditLogger`) | File-write-only, no retention enforcement | **90 days online + 1 year archive** | FedRAMP AU-11 floor | None — audit-class is no-delete in window | Cron + archive job (TODO) | BELOW-FLOOR |
| 5 | Wotan ring buffer (logagg) | Seconds-to-minutes (10K entries, in-memory) | Ring buffer is **NOT audit storage**; persist to PG (§3.5) | Operational only | Ring eviction (automatic) | `pkg/logagg/ringbuffer.go` (code-level) | BELOW-FLOOR (as audit) / MEETS-DESIGN (as ring) |
| 6 | eBPF traces (Anamnesis Lite) | Same ring-buffer pattern | Persist marked traces to PG; default 90 days online | NIST AU-11 / CIS 8.10 | Operational triage | Trace-collector → PG sink (TODO) | BELOW-FLOOR |
| 7 | Champion gate audit + snapshots | `zhen_actions` PG (no DELETE), snapshots PG (append-only) — no scheduled archive | **90 days online + 1 year archive** for actions; **permanent** for snapshots (before-state is sacred) | NIST AU-11 / FedRAMP | Append-only by design | DB grants exclude DELETE (`009_zhen_actions.sql`) | UNVERIFIED (archive job missing) |
| 8 | The Well (PostgreSQL × 3) | See §3.8 per-DB | Per-database (§3.8) | Mixed | Per-database | DB-level per §3.8 | Mixed |
| 9 | Zhen conversations (`zhen_conversations`) | No retention enforcement | **90 days default**; operator-controlled longer via `--persist` | GDPR Art. 5(1)(e) (operator PII risk) | Operator deletion + scheduled purge | Cron purge (TODO) | BELOW-FLOOR |
| 10 | Zhen memories (`zhen_memories`) | No retention | **Indefinite by intent**; mandatory **monthly review** | Operational | Operator-driven (memory poisoning threat) | Manual review checklist | MEETS-DESIGN (review missing) |
| 11 | SBOM artefacts | Permanent in git (`docs/sbom/`, `sbom/`, `LICENSES/`) | Permanent | NIST SSDF / EO 14028 | Cannot delete (supply-chain record) | git itself | IMMUTABLE |
| 12 | Vuln scan results | 30 days online (security.yml, sbom-audit.yml mix) | **12 months online + 3 years archive** | FedRAMP RA-5 + trend analysis | Auto-expire after archive | GHA + S3 archive (TODO) | BELOW-FLOOR |
| 13 | Incident records | None (no IR plan yet) | Permanent (when IR plan ships) | HIPAA §164.316 + ISO A.5.33 | Cannot delete (regulator + post-mortem record) | IR plan v1 (task #52) | DEFERRED |
| 14 | Backup data (Wotan WAL, PG backups) | UNVERIFIED | Defer to IR plan + DR runbook | RTO/RPO-driven | Per IR plan | TBD | DEFERRED |
| 15 | Sealed Cask archives (`deploy/cask-*.tar.gz`) | Built on demand; no archive policy | **Last 5 versions online; permanent in immutable storage** (S3 object-lock or equivalent) | NIST SSDF + reproducibility | Cannot delete archive (rollback record) | Adopter-chosen object-lock | BELOW-FLOOR |

---

## 3. Per-data-class detail

### 3.1 Source code + git history (PERMANENT)

**Today.** Two repos:
- `github.com/unheaded/unheaded` — primary monorepo. Currently **private**.
  Contains the kingdom (~415K production LOC, 753K with tests).
- `github.com/unheaded/wotan` — message-bus repo. Already **public**
  (referenced from CLAUDE.md as separate). Older / smaller.

**Public-vs-private split.** Pre-launch (today, 2026-05-06): the monorepo is
private. Post-launch plan per S35 strategic decisions: monorepo goes public
under GPL-3.0; protocol specs (`docs/protocol/`, `proto/`, IETF submissions)
dual-licensed GPL-3.0 / Apache-2.0. Doom remains a **private fork** of
id-Software/DOOM (with sound) per the S35 decision; it is moved out of
the public monorepo before public-launch.

**Retention.** **Permanent.** Git history is the canonical record of who
changed what, when, and why. There is no scheduled deletion mechanism, and
the kingdom does not history-rewrite (no force-pushes to `main`; rewriting
history is a documented anti-pattern in CLAUDE.md commit guidelines).

**Driver.** Operational (build reproducibility) + ISO A.5.33 (records
management) + NIST SSDF (provenance).

**Legal basis for deletion.**
- Right-to-erasure does **not** apply: source code is the kingdom's own work
  product; it does not contain end-user PII by policy (§1.1).
- Where a contributor's name/email appears in commit metadata, that is
  contributor identification under the CLA / CONTRIBUTOR-GUIDE.md, not
  end-user PII. Removal would require history rewrite + force-push, which
  is an explicit CLAUDE.md prohibition. Contributor right-to-erasure is
  **declined** by this policy; contributors are notified at CLA signing that
  their attribution is permanent.
- Court-ordered redaction is the only contemplated trigger for history
  rewrite; that is an IR-plan event, not a routine retention event.

**Enforcement.** None needed — git itself is the enforcement.

**Strict-mode override.** None; source code is not personal data.

---

### 3.2 ADRs (PERMANENT)

**Today.** `docs/adr/` — 27 ADRs resolved as of S78 (April 2026 sweep).
Markdown files in git.

**Retention.** **Permanent.** ADRs are decision records; superseded ADRs
are marked `Status: Superseded by ADR-NNN` but **not deleted**. The
superseded record is part of the audit trail.

**Driver.** ISO A.5.33 + audit. Documentation for HIPAA §164.316(b)(2)(i)
6-year minimum is a floor — kingdom exceeds it by going permanent.

**Legal basis for deletion.** None contemplated.

**Enforcement.** git itself; PR template requires that ADR changes go
through review.

---

### 3.3 CI artefacts (30-day default with per-workflow overrides)

**Today** (audit of `.github/workflows/` 2026-05-06):

| Workflow | Artefact | Today | Policy v1 |
|---|---|---|---|
| `ci.yml` | `coverage-out` | 7 days | 7 days (operational only) |
| `ci-protocol.yml` | (two artefacts) | 7 days, 14 days | 14 days |
| `gpl-boundary.yml` | `gpl-boundary-report` | 30 days | 30 days |
| `sbom-audit.yml` | `sbom-artifacts` | 30 days | **365 days** (raised — SBOM evidence) |
| `sbom-audit.yml` | `sbom-audit-report` | 90 days | **365 days** (raised — audit evidence) |
| `security.yml` | `cargo-audit-reports` | 30 days | **365 days** (raised — vuln evidence, see §3.12) |
| `sbom.yml` | `sbom` | (no `retention-days:` set → GHA default 90) | **365 days** (raised — SBOM evidence) |

**Default for new CI workflows:** **30 days**. Workflows producing
audit-class artefacts (SBOM, vuln scans, license compliance, security
gate reports) must override to **365 days online**, with the underlying
SARIF/SPDX/JSON also pushed to a permanent archive (git `docs/sbom/` for
SBOMs, S3 + object-lock for vuln scans).

**Driver.** Operational (debug recent builds) + supply-chain audit
(NIST SSDF) for SBOM/vuln subsets.

**Legal basis for deletion.** GHA auto-expiry; the policy here is to
**not** lower retention below these floors, only raise when needed.

**Enforcement.** `retention-days:` field on every `actions/upload-artifact@v4`
step. To be added/raised in the workflows listed above. Pre-commit hook
(future) should reject `actions/upload-artifact@v4` without explicit
`retention-days:` to make the choice deliberate.

**Strict-mode override.** None — CI artefacts do not contain end-user PII.

---

### 3.4 Audit logs (`pkg/auth.AuditLogger`) — **BELOW FLOOR TODAY**

**Today.** `pkg/auth/audit.go` writes JSON-Lines to an `io.Writer` (file or
stdout). `NewFileAuditLogger(path, serviceName)` opens the file
`O_APPEND|O_CREATE|O_WRONLY` mode `0600`. **There is no log rotation, no
retention enforcement, and no archive integration in the package itself.**
Effective retention is "until disk fills" or "until container restart wipes
the volume."

**Policy v1 floor.** **90 days online + 1 year archive** (FedRAMP Moderate
AU-11 floor; matches CIS 8.10 90-day local minimum; satisfies NIST AU-11
requiring a defined enforced window; satisfies PCI 10.5/10.7 1-year-with-
3-months-immediate; falls short of HIPAA 6-year by intent — see strict mode).

**Driver.** **FedRAMP Moderate AU-11**. This is the universal floor across
the matrix family; selecting it ensures all other audit-class controls are
satisfied simultaneously.

**Legal basis for deletion.**
- Within the window: **no deletion**. Audit-class records are no-delete by
  policy.
- After 90 days online: archived to encrypted-at-rest immutable storage
  (S3 + object-lock with 1-year retention period, or filesystem equivalent
  using append-only attributes).
- After 1 year archive: deletable by automated job UNLESS subject to legal
  hold (see §4.2).

**Enforcement.** Three pieces, all **TO BE BUILT**:
1. **Log rotation** — systemd `logrotate` or app-level rotation by date.
   Rotated files written to `/var/log/unheaded/audit/<service>-YYYY-MM-DD.jsonl`.
2. **Archive job** — daily cron pushes files older than 90 days to S3
   (or adopter-chosen immutable store) with 1-year object-lock.
3. **Purge job** — weekly cron deletes archive objects past 1-year object-lock,
   subject to legal-hold list.

**Strict-mode override.** HIPAA tenants set archive duration to **6 years
+ 90 days online** (effectively making the second-tier permanent for a
HIPAA-covered adopter). Add `audit_retention: hipaa` to DataIsolationConfig.

**Today-state vs floor: SIGNIFICANTLY UNDER FLOOR.** No rotation, no
archive, no purge. **Highest-priority gap in this policy.**

---

### 3.5 Wotan ring buffer (`pkg/logagg/`) — DESIGN-CORRECT, BELOW AUDIT FLOOR

**Today.** `pkg/logagg/ringbuffer.go`: 10,000-entry ring buffer (default), in
memory, per-process. Sentinel scrutiny §SEN3 estimates the ring at ~1000
events/sec wraps in **~10 seconds**. The ring buffer is **not audit storage**
— it is a short-term aggregation surface for the dashboard live-tail and
operational triage.

**Policy v1.** The ring buffer's design retention is **seconds-to-minutes,
by intent**. It is not an audit-class store and **must not be cited** as
evidence of audit-control coverage in any matrix or compliance artefact.

For audit-class retention of log lines, the kingdom must:
1. Persist log lines from Wotan topic `logs.<service>.<level>` to
   `unheaded_app.audit_events` (or a sibling table) — current schema in
   `db/migrations/004_ops_schema.sql` already defines `audit_events` with
   tamper-evident chaining (`previous_hash`, `hash` columns).
2. Apply §3.4 retention (90 days online + 1 year archive) to that PG-backed
   audit store.
3. External SIEM (Elastic / Splunk / Loki) is acceptable as the durable sink
   in lieu of PG; adopter chooses.

**Driver.** NIST AU-11 + CIS 8.10 (audit class) — but only when the ring
buffer is cited as audit coverage. As a live-tail UI, no driver applies.

**Legal basis for deletion.** Ring eviction is automatic and cannot be paused;
this is a code-level limit (`RingBuffer.Push` overwrites `head`). For the
durable PG sink, see §3.4.

**Enforcement.** Code-level (ring eviction). The PG sink is **TO BE BUILT** —
the hook from `zerolog → Wotan topic → PG audit_events table` exists in
parts (`pkg/logagg/publisher.go`) but the topic-subscriber → PG-writer
pipe is not yet wired.

**Strict-mode override.** N/A (no PII can be retained in a 10-second window
in a way that creates a retention obligation).

**Today-state: BELOW FLOOR for any audit citation; MEETS DESIGN INTENT
as a live-tail buffer.** The matrix family must stop citing the ring buffer
as AU-2/AU-11/PCI 10.x coverage (see scrutiny §SEN3 finding).

---

### 3.6 eBPF traces (Anamnesis Lite) — SAME RING-BUFFER PROBLEM

**Today.** Anamnesis Lite uses kernel ring buffers (BPF perf/ringbuf maps)
to surface eBPF events to userspace. The trace-collector (Rust, with
Go bridge) reads these and forwards to Wotan. The kernel-side ring buffers
are even shorter than Wotan's: tens of milliseconds typically.

**Policy v1.** Same pattern as §3.5: **kernel + userspace ring buffers are
NOT audit storage**. Marked traces (e.g. SOC-relevant flow events, security
alerts) must be persisted to PG `audit_events` or to an external SIEM with
**90 days online + 1 year archive** retention.

Unmarked / routine traces may be discarded at ring eviction with no
retention obligation; they are operational-instrumentation only.

**Driver.** NIST AU-11 + CIS 8.10 + PCI 10.x for marked traces only.

**Legal basis for deletion.** Ring eviction (kernel + userspace). For the
marked-trace persistent sink: same as §3.4.

**Enforcement.** Mark-and-persist policy: trace-collector emits to PG sink
when `mark=security|audit|alert`; otherwise discard at eviction. **TO BE
BUILT** — currently `cmd/trace-collector` writes to Wotan but does not
durably persist marked subsets.

**Strict-mode override.** EU-mode adopter must filter packet metadata for
PII (IP-address pseudonymization, query-string scrubbing) before persistence.
This is the UH-PII-1..4 control set referenced in Session 10 notes.

**Today-state: BELOW FLOOR for marked-trace audit citation.**

---

### 3.7 Champion gate audit + snapshots (`pkg/champion/`)

**Today.**
- **`pkg/champion/pgstore/schema.sql`** + **`db/migrations/009_zhen_actions.sql`**:
  `zhen_actions` table in `unheaded_app`. Grants `SELECT, INSERT, UPDATE`
  to `app_zhen` — **no DELETE** (intentional — actions are permanent record).
- `zhen_action_snapshots` table: append-only. Grants `SELECT, INSERT` only
  — **no UPDATE, no DELETE** (before-state is sacred).
- **No archive job**, no purge job, no rotation. Tables grow unbounded.

**Policy v1.**
- **`zhen_actions`:** **90 days online + 1 year archive** (FedRAMP AU-11
  audit-class). After archive, deletable subject to legal hold.
- **`zhen_action_snapshots`:** **PERMANENT.** Before-state archive is
  irreplaceable forensic record. Cannot be deleted.

**Driver.** NIST AU-11 (actions); ISO A.5.33 + IR / forensic preservation
(snapshots).

**Legal basis for deletion.**
- `zhen_actions`: post-archive, automated purge, subject to legal hold.
- `zhen_action_snapshots`: none. Database role grants **deny DELETE** at the
  PG level — code-level enforcement, not relying on policy compliance.
- GDPR right-to-erasure: snapshots may contain operator-pasted customer
  content. Strict-mode override applies (§1.3).

**Enforcement.** PG GRANT statements (already deny DELETE on snapshots and
deny DELETE on `zhen_actions`). Archive + purge jobs **TO BE BUILT**.

**UNVERIFIED:** the live `pkg/champion/pgstore/schema.sql` and the
`db/migrations/009_zhen_actions.sql` define overlapping but **not identical**
schemas (the migration has 27 columns, pgstore.go has 13). Reconciliation is a
separate task; for retention purposes, the `zhen_actions.id` + `planned_at`
+ `status` + `parameters` columns are sufficient regardless of which schema
is canonical.

**Strict-mode override.** EU-mode: snapshot `content_before` / `content_after`
columns must be PII-scrubbed before write OR the snapshot subsystem must be
disabled (champion gate operates without snapshots — degraded forensic mode
but compliant).

**Today-state: ACTIONS BELOW FLOOR (no archive); SNAPSHOTS MEETS-DESIGN
(immutable by grant).**

---

### 3.8 The Well (PostgreSQL × 3) — per-database

The Well is three databases on one PG cluster (`db/migrations/002_create_databases.sql`):

#### 3.8.1 `unheaded_app`

Contains `zhen_memories`, `zhen_conversations`, `zhen_actions`, `zhen_action_snapshots`,
plus app-side service tables (timeguru timeline cache, kanban tasks, etc.).

| Table | Retention | See section |
|---|---|---|
| `zhen_memories` | Indefinite + monthly review | §3.10 |
| `zhen_conversations` | 90 days default, `--persist` longer | §3.9 |
| `zhen_actions` | 90 days online + 1 year archive | §3.7 |
| `zhen_action_snapshots` | Permanent | §3.7 |
| (timeguru / kanban tables) | Operational; mirror to git references; effectively permanent via git mirror | This row |

**Driver:** mixed (per-table). **Legal basis for deletion:** per-table.
**Enforcement:** per-table; cron jobs (TODO) per table.

#### 3.8.2 `unheaded_ops`

Contains `service_health_current`, `service_health_transitions`,
`service_health_hourly`, `audit_events` (per `004_ops_schema.sql`).

| Table | Retention | Driver | Enforcement |
|---|---|---|---|
| `service_health_current` | Live (UPSERT) — no historical retention | Operational | None — overwritten in place |
| `service_health_transitions` | **90 days online + 1 year archive** | NIST AU-11 | Cron purge (TODO) |
| `service_health_hourly` | **13 months online** (trend analysis YoY) + 3 years archive | Operational + capacity planning | Cron purge (TODO) |
| `audit_events` | **90 days online + 1 year archive** | FedRAMP AU-11 | Cron archive + purge (TODO); tamper-evident chain via `previous_hash` |

**Today-state: BELOW FLOOR — no scheduled retention enforcement on any
of these tables.**

#### 3.8.3 `unheaded_config`

Contains kingdom configuration (declarative state). Tables defined in
`005_config_schema.sql`. Configuration is itself an audit record (who set
what when), and the canonical source is git (`configs/`, `services.yaml`,
NixOS modules, Helm values). The PG copy is operational cache; overwrite
on convergence is acceptable.

| Aspect | Retention | Driver | Enforcement |
|---|---|---|---|
| Current config rows | Live (overwritten on convergence) | Operational | None |
| Config-change audit log | Permanent (in git) | NIST CM-3 / ISO A.5.33 | git itself |

**Strict-mode override.** None — no end-user PII in config.

---

### 3.9 Zhen AI conversation history (`zhen_conversations`)

**Today.** `db/migrations/010_zhen_conversations.sql`: `zhen_conversations`
in `unheaded_app`. Columns: `session_id`, `role`, `content`, `sources`,
`model`, `tokens_input`, `tokens_output`, `elapsed_ms`, `created_at`,
generated `search_vector`. Grants `SELECT, INSERT` to `app_zhen` only —
no UPDATE, no DELETE today.

**Threat:** operators paste customer data into Zhen prompts (e.g.
"summarize this customer ticket"). Conversation rows therefore may contain
customer PII despite the kingdom's no-PII architectural floor (§1.3).

**Policy v1.**
- **Default retention:** **90 days** rolling window. Rows older than 90 days
  are purged by a daily cron job.
- **Operator-controlled longer retention:** an operator may invoke
  `zhen --persist <session_id> --retention=<duration>` (CLI to be built)
  to extend retention up to **1 year** for a specific session, e.g. for
  ongoing investigations or capability training data. Persisted sessions
  are flagged in a side-table `zhen_conversations_holds(session_id, expires_at, reason)`.
- **Right-to-erasure:** GDPR/CCPA-aligned operator can issue
  `zhen --forget <session_id>` to delete all rows for a session before the
  90-day window expires. Requires PG grant change to add DELETE for `app_zhen`
  on `zhen_conversations` (intentional — see enforcement).

**Driver.** GDPR Art. 5(1)(e) + CCPA §1798.105 + the matrix-scrutiny finding
that operator-pasted PII is the leak vector.

**Legal basis for deletion.**
- Routine: 90-day cron purge.
- On-demand: operator-issued `zhen --forget` per right-to-erasure.
- Persisted session expiry: hold-record-driven; cron purges expired holds.

**Enforcement.** Three pieces, **TO BE BUILT**:
1. Migration to add `DELETE` grant on `zhen_conversations` to `app_zhen`
   (currently denied by `010_zhen_conversations.sql` line 53).
2. Cron job (daily): `DELETE FROM zhen_conversations WHERE created_at <
   NOW() - INTERVAL '90 days' AND session_id NOT IN (SELECT session_id
   FROM zhen_conversations_holds WHERE expires_at > NOW());`
3. CLI: `zhen --persist`, `zhen --forget`.

**Strict-mode override.** EU-mode: default **30 days** (not 90), with the
same `--persist` extension cap at 90 days. Stricter because operator-PII
leak is the dominant risk.

**Today-state: BELOW FLOOR — no purge, no forget, no persist mechanism;
table grows unbounded with potential PII content.**

---

### 3.10 Zhen AI memory store (`zhen_memories`)

**Today.** `db/migrations/008_zhen_memories.sql`: `zhen_memories` in
`unheaded_app`. Columns: `id`, `question`, `answer`, `embedding`, `source`,
`model`, `created_at`. Grants `SELECT, INSERT, UPDATE, DELETE` to `app_zhen`
— full CRUD intended (operator can `memory.forget` per `zhen_actions`
action_type list).

**Threat:** memory poisoning — adversary plants a malicious memory that
biases future Zhen answers. Per `010_zhen_conversations.sql` comments:
"the test_memory_poison fixture seeds and cleans up zhen_memories rows
tagged source='poison'." This is a known attack surface; see also Phase 3
H3 testing pattern.

**Policy v1.**
- **Retention:** **indefinite by intent** — memories accumulate operator
  knowledge; pruning loses signal.
- **Mandatory monthly review:** the operator (Stevie or designate) reviews
  all rows added in the past month for poisoning indicators. Rows tagged
  `source IN ('poison', 'untrusted', 'public-untrusted')` are flagged.
  Rows the reviewer cannot vouch for are deleted.
- **Quarterly full audit:** a full sweep of all `zhen_memories` rows by
  embedding-similarity clustering, identifying near-duplicates and outliers.

**Driver.** Operational (capability) + AI-supply-chain integrity (no specific
framework yet, but maps to NIST AI RMF GOVERN-1.4 and ISO/IEC 42001 5.4
for adopters under those regimes).

**Legal basis for deletion.**
- Operator review (any row at any time).
- Right-to-erasure: if a memory contains operator-pasted customer data, the
  operator deletes the row directly. There is no automatic erasure because
  the schema does not link memories back to data subjects.

**Enforcement.**
1. Monthly review: calendar event + checklist (`docs/runbooks/zhen-memory-review.md`,
   **TO BE WRITTEN**).
2. Quarterly audit: scripted (`scripts/zhen-memory-cluster-audit.py`,
   **TO BE WRITTEN**).
3. Code-level: the champion gate logs `memory.remember` and `memory.forget`
   actions to `zhen_actions`, so review activity is itself audit-trailed.

**Strict-mode override.** EU-mode: monthly review becomes **weekly review**.
Reviewer must produce a written attestation per cycle.

**Today-state: MEETS DESIGN INTENT (indefinite + grant CRUD) but review
cadence is unenforced — no calendar events, no checklist, no audit log of
reviews having happened.**

---

### 3.11 SBOM artefacts (PERMANENT)

**Today.**
- `docs/sbom/` — generated SBOM markdown reports (in git).
- `sbom/` — root-level SBOM directory (in git).
- `LICENSES/` — per-dependency license texts (in git).
- CI artefacts from `sbom.yml` and `sbom-audit.yml` — currently 30/90 days
  in GHA artefact storage; raised to 365 days per §3.3.

**Retention.** **Permanent in git.** Every release tag carries a frozen
SBOM. Historical SBOMs are recoverable from git history via tag/commit.

**Driver.** NIST SSDF (PS.3.2 — provide a software bill of materials) +
EO 14028 + NIS2 + CIS 18 (supply chain).

**Legal basis for deletion.** None contemplated. SBOM is part of the
software-supply-chain audit record, not personal data.

**Enforcement.** git itself for the in-repo SBOMs; release-tagging discipline
ensures every release has a frozen SBOM committed.

**Strict-mode override.** None.

---

### 3.12 Vulnerability scan results

**Today.**
- `security.yml` cargo-audit reports: 30 days GHA retention.
- `sbom-audit.yml` Grype results + audit report: 30/90 days GHA retention.
- SARIF uploads to GitHub Security tab: GitHub-managed, default retention
  (effectively permanent while public, but not a primary store).

**Policy v1.**
- **12 months online** for trend analysis (raised from 30/90 to 365 days
  per §3.3).
- **3 years archive** in immutable storage (S3 + object-lock or equivalent),
  satisfying FedRAMP RA-5 evidence retention.
- For each scan, retain: SBOM input, tool version, rule set version, raw
  findings, triage decisions (accept/mitigate/defer), remediation evidence.

**Driver.** FedRAMP RA-5 (vulnerability scanning + retention of evidence)
+ NIST 800-171 03.11.02 + PCI 6.3.1 + CIS 7.x.

**Legal basis for deletion.** Auto-purge after 3-year archive completes,
subject to legal hold.

**Enforcement.**
1. CI: raise `retention-days:` on relevant workflows to 365 (per §3.3).
2. Archive job (TO BE BUILT): nightly mirror of vuln-scan artefacts to
   S3 + object-lock with 3-year retention.
3. Triage tracking: link to the F6 vulnerability-management runbook
   (task #53) for the close-out / POA&M side.

**Strict-mode override.** None — vuln data is not end-user PII.

**Today-state: BELOW FLOOR.** Scrutiny §SEN6 already flagged this:
"consuming a feed is not vulnerability management". Retention is one of
several gaps; the F6 runbook covers the rest.

---

### 3.13 Incident records (DEFERRED to IR plan v1)

**Today.** None. The kingdom has no IR program (per scrutiny §SEN2),
therefore no incident records exist.

**Policy v1.** When the IR plan v1 ships (task #52 / F5):
- **Permanent retention** of all incident records (incident report,
  forensic artefacts, post-mortem, remediation evidence, regulator
  notifications).
- Storage: git (`docs/incidents/<date>-<short-name>/`) for the
  human-readable record; S3 + object-lock for forensic-binary artefacts
  (memory dumps, packet captures, disk images).

**Driver.** HIPAA §164.316(b)(2)(i) + ISO A.5.33 + GDPR Art. 33 evidence
+ PCI 12.10.6 + NIST IR-4 / IR-5 / IR-6 + SOC 2 CC7.4 + many more —
incident records are perhaps the most universally retention-mandated class.

**Legal basis for deletion.** None contemplated. Even after the standard
6-year HIPAA / ISO floor, kingdom retains permanently as institutional
memory.

**Enforcement.** When IR plan ships.

**Today-state: DEFERRED.** Cross-reference: `docs/policy/ir-plan-v1-2026-05-06.md`
(F5 / task #52) — to be written.

---

### 3.14 Backup data (DEFERRED to IR plan + DR runbook)

**Today.**
- Wotan WAL replication: implementation status UNVERIFIED. Wotan has
  ring-buffer + topic publishing but durable replication to a sibling
  Wotan instance for catastrophic recovery is not documented.
- PostgreSQL backups: implementation status UNVERIFIED. The Well runs as
  a PG cluster (per CLAUDE.md) but pg_dump cadence, WAL archiving, off-site
  copies are not documented in this repo.

**Policy v1.** **DEFERRED to IR plan v1 + DR runbook.** Retention windows
for backups are RTO/RPO-driven and cannot be set independently of the
recovery objectives.

Placeholder targets (subject to IR plan):
- PG full backup: nightly, retain 30 days online + 1 year archive.
- PG WAL: 7 days online + 90 days archive.
- Wotan replication: live (not a backup retention question — durability
  question).

**Driver.** Per IR plan; floor will be the union of FedRAMP CP-9, NIST
CP-9, ISO A.8.13.

**Legal basis for deletion.** Per IR plan.

**Enforcement.** Per IR plan + DR runbook.

**Strict-mode override.** Backups containing strict-mode-flagged tables
inherit those tables' PII-handling rules (encrypted-at-rest mandatory,
geographic restriction enforced).

**Today-state: DEFERRED.**

---

### 3.15 Sealed Cask archives (`deploy/cask-*.tar.gz`)

**Today.** Sealed Casks are deterministic, cryptographically-bound image
archives produced by `scripts/build-sealed-cask.sh` and verified by
`scripts/verify-binding-rune.sh`. They are not currently committed to
`deploy/` in this repo (no `*.tar.gz` files present); they are produced
on demand and consumed by the deployment pipeline. Per CLAUDE.md S35,
the production cask flow is: build deterministically → sign → distribute.

**Policy v1.**
- **Last 5 versions online** in the deployer's working directory
  (`deploy/cask-<version>.tar.gz`). Older versions evicted by retention
  cron.
- **Permanent in immutable storage.** Every cask ever built is archived
  to immutable storage with object-lock or equivalent (S3 with object-lock,
  Azure Immutable Blob, or filesystem with append-only attribute on a
  dedicated host). The cask + its `binding-rune.json` (the SHA256 +
  signature manifest) form the rollback / supply-chain evidence and
  are irreplaceable.

The "5 versions online" is operational: a deployer needs the current
version, immediate rollback (-1), and a few older for forensic comparison.
The permanent archive is what proves "this binary was deployed on
this date" for any future audit, breach investigation, or supply-chain
incident.

**Driver.** NIST SSDF PS.1.1 (protect software from unauthorized changes) +
NIST SSDF PS.3 (provenance) + EO 14028 + the kingdom's own reproducibility
doctrine.

**Legal basis for deletion.** **None for archive.** The 5-versions-online
local copies are evicted by cron; the immutable archive is never deleted
within this policy.

**Enforcement.**
1. Local: cron job evicts oldest cask when a 6th is created
   (`scripts/cask-rotate-local.sh`, **TO BE BUILT**).
2. Archive: `scripts/cask-archive-push.sh` — runs at cask creation, pushes
   to S3 + object-lock. **TO BE BUILT.** Adopter chooses the immutable
   store; the script is a thin wrapper.
3. Verification: monthly integrity check — sample N casks from the archive,
   re-verify binding-rune. **TO BE BUILT.**

**Strict-mode override.** None — casks are infrastructure binaries, not
personal data.

**Today-state: BELOW FLOOR — no online rotation, no archive push, no
integrity check. Manual / ad-hoc.**

---

## 4. Cross-cutting concerns

### 4.1 Right-to-erasure (GDPR Art. 17 / CCPA §1798.105)

The kingdom's architectural floor (§1.1) means the bulk of these data
classes are **not subject to subject-level erasure** because they don't
contain end-user PII. Three classes are exceptions where operator-pasted
content can land:

| Class | Erasure mechanism | Latency |
|---|---|---|
| Audit logs (§3.4) | Best-effort: a regulator-ordered redaction triggers a manual review of the audit-log archive for any rows referencing the data subject; matched rows are tombstoned (replaced with a redaction marker, leaving the hash chain intact). | Up to 30 days from request |
| Zhen conversations (§3.9) | `zhen --forget <session_id>` direct deletion | Same-day |
| Champion snapshots (§3.7) | **Cannot be deleted** by policy. Strict-mode adopter must run with snapshots disabled. EU-mode adopter must enable PII scrubbing before snapshot write. | N/A (refuse + advise) |

The kingdom publishes a public erasure-request endpoint (TBD; part of the
public-launch checklist) where adopters or end-users-via-adopters can
file an erasure request that's triaged within 30 days.

### 4.2 Legal hold

When the kingdom or an adopter receives notice of pending litigation,
regulatory investigation, or breach-related preservation order, **all
retention purges for affected classes are suspended** until the hold is
lifted.

Mechanism: a `legal-holds.yaml` file in a private operations repo (or PG
table for adopter deployments) lists active holds with scope (which data
classes / which subject identifiers / time windows). Cron purge jobs read
this file and skip matching rows.

This file is itself permanent (audit record of holds applied).

### 4.3 Encryption at rest

All retention applies equally to encrypted-at-rest copies. Encryption is
not a substitute for retention enforcement: an encrypted-at-rest archive
past its retention window must still be deleted, and the deletion must
include the key (key destruction = cryptographic erasure under NIST 800-88
guidelines).

Per-class encryption-at-rest status is OUT OF SCOPE for this policy and
documented in the encryption-at-rest control matrix (TBD).

### 4.4 Tamper-evidence

Audit-class records (§3.4 audit logs, §3.7 champion actions, §3.8.2
ops audit_events) maintain hash-chain tamper-evidence (`previous_hash`,
`hash` columns where present). Retention purges must NOT rewrite the
chain — they delete tail rows. Archive moves the row out of the live
chain; the chain head pointer advances. A re-import from archive must
re-verify chain integrity.

---

## 5. Procedural enforcement summary

This table consolidates §3 enforcement into a single owner/cadence view.

| Mechanism | Class(es) | Cadence | Owner | Today |
|---|---|---|---|---|
| `git` (no scheduled deletion) | 3.1, 3.2, 3.11, 3.13 (post-IR) | N/A — immutable | git itself | LIVE |
| GHA `retention-days:` | 3.3, 3.12 | Per upload | CI maintainer | LIVE for some, raise needed |
| Code-level ring eviction | 3.5, 3.6 | Continuous | Compiler/runtime | LIVE |
| PG GRANT denial of DELETE | 3.7 (snapshots), 3.7 (actions) | Continuous | DBA migration | LIVE |
| Log-rotation (logrotate / app) | 3.4 | Daily | systemd / cron | **TO BUILD** |
| Archive-push cron (S3 + object-lock) | 3.4, 3.7 (actions), 3.8.2, 3.12, 3.15 | Daily | Cron | **TO BUILD** |
| Purge cron (PG DELETE) | 3.8.2 (transitions, hourly), 3.9, 3.7 (actions, post-archive) | Daily | Cron | **TO BUILD** |
| Operator CLI (`zhen --forget`, `zhen --persist`) | 3.9 | On-demand | Operator | **TO BUILD** |
| Manual review (memory) | 3.10 | Monthly + Quarterly | Stevie / designate | **TO BUILD** (calendar + checklist) |
| Cask local rotation | 3.15 | On cask creation | Build script | **TO BUILD** |
| Cask archive push | 3.15 | On cask creation | Build script | **TO BUILD** |
| Cask integrity check | 3.15 | Monthly | Cron | **TO BUILD** |
| Legal-hold gate | 4.2 | Pre-purge check | All purge jobs | **TO BUILD** |

**TO BUILD count: 10 mechanisms.** This is the work backlog implied by
this policy.

---

## 6. Review cadence

This policy is reviewed:

- **Monthly:** review of zhen_memories per §3.10.
- **Quarterly:** policy itself reviewed for accuracy of "today-state" status
  rows; data classes added or removed as the kingdom evolves.
- **Annually:** full review of permanent-retention classes (§3.1, 3.2, 3.7
  snapshots, 3.11, 3.13) confirming integrity, accessibility, and continued
  relevance.
- **Event-triggered:**
  - New regulated adopter onboarding (HIPAA-covered, PCI-merchant, EU-strict)
    → strict-mode overrides confirmed.
  - New data class introduced (e.g. a future Wotan WAL store) → policy
    amended.
  - Framework certification cycle (FedRAMP, ISO, SOC 2) → policy mapped
    to certification-specific evidence requirements.

Review owner: Stevie (today, single operator). Post-launch:
data-protection officer or designate.

---

## 7. Compliance crosswalk (which framework cells this closes)

| Framework | Control | This policy closes? | Notes |
|---|---|---|---|
| GDPR | Art. 5(1)(e) storage limitation | **YES** | Per-class windows, strict-mode |
| GDPR | Art. 17 right-to-erasure | **PARTIAL** | §4.1 — mechanism exists for 2 of 3 leak classes |
| PCI DSS 4.0 | 10.5 / 10.7 audit log retention | **YES** | §3.4 (90 online + 1 year archive ≥ 3 months immediate + 1 year total) |
| HIPAA | §164.316(b)(2)(i) doc retention 6yr | **PARTIAL** | strict-mode override §3.4 — adopter must opt in |
| NIST SP 800-53 | AU-11 audit-record retention | **YES** | §3.4 + §3.5 + §3.6 + §3.7 + §3.8.2 |
| NIST SP 800-53 | SI-12 information management | **YES** | This document |
| NIST SP 800-171 | 03.03.08 audit-log review and retention | **YES** | §3.4 + §6 |
| CCPA / CPRA | §1798.105 right-to-delete | **PARTIAL** | §4.1 + §3.9 |
| CCPA / CPRA | §1798.140(o) disclosed retention | **YES** | This document discloses |
| ISO/IEC 27001 | A.5.33 records protection | **YES** | All classes |
| ISO/IEC 27001 | A.8.10 information deletion | **YES** | §4 + §5 |
| CIS Controls v8 | 8.10 audit-log retention | **YES** | §3.4 (≥ 90 day floor met) |
| FedRAMP Moderate | AU-11 (Mod parameter: 90d/1yr) | **YES** | §3.4 explicitly meets the floor |
| FedRAMP Moderate | RA-5 vuln scan retention | **YES** | §3.12 |
| NIST CSF 2.0 | PR.DS-1 / PR.DS-2 | **YES** | Coverage |
| SOC 2 | CC7.2 / A1.2 retention | **YES** | All classes |
| NIS2 | Art. 21 supply-chain records | **YES** | §3.11 + §3.15 |
| EO 14028 | SBOM retention | **YES** | §3.11 |

**Net effect:** ~14 framework controls move from GAP to MEETS-FLOOR (or
PARTIAL with documented gap-to-MEETS work) once this policy is committed
**and** the 10 TO-BUILD mechanisms in §5 are implemented.

The policy alone (without mechanisms) closes the **documentation** gap
across all 7 universal-gap frameworks. The mechanisms close the
**operational** gap.

---

## 8. Today-state honest summary

Of the 15 enumerated classes:

- **5 classes are IMMUTABLE / MEETS-FLOOR / MEETS-DESIGN today:**
  3.1 source code, 3.2 ADRs, 3.3 CI artefacts (default), 3.5 ring buffer
  (as design), 3.11 SBOM, 3.7 snapshots (as design).
- **6 classes are SIGNIFICANTLY BELOW FLOOR today:**
  - **3.4 Audit logs** — no rotation/archive/purge. **Highest priority.**
  - **3.5/3.6 Ring buffers cited as audit** — the citation must stop;
    PG sink not built.
  - **3.7 Champion actions** — no archive job.
  - **3.8.2 Ops audit_events / transitions** — no archive, no purge.
  - **3.9 Zhen conversations** — no purge, no forget, no persist; PII risk.
  - **3.12 Vuln scans** — 30/90 days vs 12-months-online floor.
  - **3.15 Sealed Casks** — no rotation, no archive, no integrity check.
- **2 classes are DEFERRED:**
  3.13 incident records (awaits IR plan v1), 3.14 backups (awaits IR plan + DR
  runbook).
- **2 classes are MEETS-DESIGN with unenforced review cadence:**
  3.10 zhen memories (monthly review unscheduled).

**Single highest-priority gap:** §3.4 — the auth audit logger has no
rotation, no archive, and no enforced retention. This is the universal-gap
control across 7 frameworks; closing it operationally (not just on paper)
is the highest-leverage action.

---

## 9. Open questions / UNVERIFIED items

1. **§3.7** — `pkg/champion/pgstore/schema.sql` vs `db/migrations/009_zhen_actions.sql`
   schema reconciliation. Which is canonical at runtime?
2. **§3.14** — Does Wotan have durable WAL replication? (Status: UNVERIFIED.)
3. **§3.14** — PG backup cadence and off-site copy: documented anywhere?
4. **§3.4** — Where do `pkg/auth.AuditLogger` files actually land in
   deployed services today? (`/var/log/unheaded/`? Service-local? Stdout
   in Docker?)
5. **§3.6** — Anamnesis Lite "marked-trace" semantics: is there already a
   `mark` field in the trace event schema, or is this policy proposing one?
6. **§3.8.2** — Has the `audit_events` tamper-evident hash chain ever been
   verified end-to-end on real data? (Implementation present in schema;
   verifier UNVERIFIED.)
7. **§3.15** — Adopter S3 / object-lock recommendation: which provider
   defaults does the kingdom suggest, and is the kingdom prepared to ship
   a non-S3 reference (MinIO with object-lock, IPFS-with-pinning)?

These open questions are individually triaged in subsequent policy
revisions and threat-model docs; they do not block this policy v1's
adoption.

---

## 10. Acceptance & ratification

This is **draft v1**. Ratification requires:

1. Stevie's review and explicit "ratify retention-policy-v1".
2. Cross-link from each compliance-control-matrix that cites a
   universal-gap retention control (handled by F12, task #59).
3. Creation of the 10 TO-BUILD mechanisms in §5 as kanban tasks (out of
   scope of this draft).
4. Initial run of the §3.4 audit-log archive job within 30 days of
   ratification (the highest-priority action).

**Until ratified, this policy is non-binding.** It documents intent, not
established practice. Adopters reading this draft should not represent
its windows as currently-enforced; the today-state column in §2 is the
ground truth.

---

## Appendix A: Files referenced

- `pkg/auth/audit.go` — current audit logger implementation
- `pkg/logagg/ringbuffer.go` — Wotan ring buffer (10K default, in-memory)
- `pkg/champion/pgstore/schema.sql` — champion zhen_actions schema (alt)
- `db/migrations/002_create_databases.sql` — The Well 3-DB topology
- `db/migrations/004_ops_schema.sql` — `unheaded_ops` audit_events + health
- `db/migrations/008_zhen_memories.sql` — Zhen memory store
- `db/migrations/009_zhen_actions.sql` — Champion action log + snapshots
- `db/migrations/010_zhen_conversations.sql` — Zhen conversation history
- `.github/workflows/{ci,ci-protocol,gpl-boundary,sbom,sbom-audit,security}.yml`
  — CI artefact retention current settings
- `scripts/build-sealed-cask.sh` — Sealed Cask producer
- `scripts/verify-binding-rune.sh` — Sealed Cask integrity verifier
- `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` — scrutiny doc
  driving this policy
- `CLAUDE.md` — project doctrine, architecture, ports, The Well topology

## Appendix B: Acronyms

| Acronym | Meaning |
|---|---|
| AU | Audit and Accountability (NIST 800-53 family) |
| CCPA | California Consumer Privacy Act |
| CHD | Cardholder Data |
| CPRA | California Privacy Rights Act |
| CUI | Controlled Unclassified Information |
| DPA | Data Processing Agreement |
| DR | Disaster Recovery |
| EO | Executive Order |
| FedRAMP | Federal Risk and Authorization Management Program |
| FTS | Full-Text Search |
| GDPR | General Data Protection Regulation |
| GHA | GitHub Actions |
| HIPAA | Health Insurance Portability and Accountability Act |
| IR | Incident Response |
| ISO | International Organization for Standardization |
| KEV | Known Exploited Vulnerabilities (CISA) |
| NIS2 | Network and Information Systems Directive 2 (EU) |
| NIST | National Institute of Standards and Technology |
| NVD | National Vulnerability Database |
| PCI DSS | Payment Card Industry Data Security Standard |
| PHI | Protected Health Information |
| PII | Personally Identifiable Information |
| POA&M | Plan of Action and Milestones |
| RPO | Recovery Point Objective |
| RTO | Recovery Time Objective |
| SAD | Sensitive Authentication Data |
| SARIF | Static Analysis Results Interchange Format |
| SBOM | Software Bill of Materials |
| SOC 2 | System and Organization Controls 2 |
| SPDX | Software Package Data Exchange |
| SSDF | Secure Software Development Framework (NIST 800-218) |

---

**END retention-policy-v1-2026-05-06.md**
