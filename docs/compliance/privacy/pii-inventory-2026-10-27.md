<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Personal Data Inventory 2026-10-27

Where this deployment (west, east, The Well, the log store) holds personal data. Columns and fields only; no values are copied here. Built from the migrations (`db/migrations/*.sql`), the Vector pipeline (`config/vector/vector.yaml`, `config/clickhouse/schema/001-logs.sql`) and the services' own files.

**Whose data:** the operator's (Stevie Bellis), plus whatever a network peer or log line carries about a remote party (a login name, a source address). There are no adopter, customer or end-user records: adopters run their own deployments. Packet payloads are never stored; eBPF programs read headers and flow metadata (NIST 800-207 Zero Trust Architecture: [privacy notice](privacy-notice.md)).

**Classes:**

- **identifier**: a name, account, key owner or address that points at a person.
- **content**: free text a person wrote, which may contain anything.
- **network**: an address or login name from traffic or logs.

**Retention** cites retention policy v1 (`docs/policy/retention-policy-v1-2026-05-06.md`, row numbers in its section 2; unratified, S-WELL-SPRING E-C) and what enforces it today.

## The Well, `unheaded_app`

| Table.column | Class | What | Retention (policy v1 row) | Enforced today |
|---|---|---|---|---|
| `kanban_tasks.title`, `.description` | content | card text the operator or an agent wrote | as source records (row 8) | none |
| `kanban_tasks.owner` | identifier | who owns a card (operator or an agent name) | row 8 | none |
| `zhen_conversations.content` | content | operator prompts and model answers | 90 days (row 9) | none: no purge job, and no app role has DELETE |
| `zhen_memories.question`, `.answer` | content | facts the operator asked Zhen to keep | indefinite, monthly review (row 10) | none |
| `zhen_actions.content_before`, `.content_after` | content | file contents before and after a Champion edit | 90 days + 1 year archive (row 7) | none (no DELETE grant; archive job missing) |
| `zhen_actions.triggered_by` | identifier | user, zhen, schedule or hook | row 7 | as above |
| `zhen_pending_confirmations.emitted_by`, `.decided_by`, `.summary` | identifier, content | who asked for and who approved a write | row 7 | none |
| `zhen_captures.prompt`, `.response` | content | prompts and answers captured for training (redacted T0/T1 at capture) | 180 days unless exported | `purge_after` set by trigger; `zhen_captures_purge()` |
| `zhen_captures.author_kind` | identifier | local, claude or human | as above | as above |
| `api_keys.owner` | identifier | the account an API key belongs to | while the key exists | revocation keeps the row |

## The Well, `unheaded_ops`

| Table.column | Class | What | Retention (policy v1 row) | Enforced today |
|---|---|---|---|---|
| `audit_events.actor_id`, `.actor_type`, `.actor_name` | identifier | who did an audited action | 90 days online + 1 year archive (row 4) | append-only; partition drop planned, no job yet |
| `audit_events.source_ip` | network | client address of an audited request | row 4 | as above |
| `service_health_transitions.reason` | content | machine text; no person named by design | row 8 | append-only |
| `compliance_findings.updated_by`, `.change_reason`; `compliance_findings_history.changed_by`, `.change_reason` | identifier, content | who triaged a finding and why | permanent (audit record) | append-only history |
| `llm_requests` | none | counts, cost, timing, model, route; never prompt, response or tool arguments (migration 030 header) | row 4 | append-only |

## The Well, `unheaded_config`

| Table.column | Class | What | Retention | Enforced today |
|---|---|---|---|---|
| `kingdom_config.updated_by`; `kingdom_config_history.changed_by`, `.change_reason` | identifier, content | who changed a setting and why | permanent (change record) | append-only history |
| `compliance_profiles.updated_by`; `compliance_host_bindings.updated_by`, `.change_reason`; `compliance_host_bindings_history.changed_by`, `.change_reason` | identifier, content | who moved a host between profiles | permanent | append-only history |
| `well_schema_migrations.applied_by` (all databases) | identifier | database role that applied a migration | permanent | ledger |

## Logs and metrics

| Store.field | Class | What | Retention | Enforced today |
|---|---|---|---|---|
| systemd journal (west, east) | network, content | every unit's output, including sshd and sudo lines that carry login names and client addresses | bounded by size (`journald-caps`: SystemMaxUse) | applied on hosts per E-D |
| ClickHouse `unheaded_logs.logs.message`, `.fields` | network, content | journal lines shipped by Vector, as above | 90 days; 455 days where `retention = 'audit'` (auth, authpriv, kernel audit) | ClickHouse TTL |
| Prometheus `unheaded_auth_failures_total` labels `host`, `unit`, `kind` | none | counts only; no login name or address | Prometheus retention | Prometheus |
| alert-mailer outbox | identifier | the operator's mail address as recipient | in memory | restart |

## Files outside the database

| File | Class | What | Retention |
|---|---|---|---|
| `/etc/unheaded/auth/users.json` | identifier | login names, argon2id hashes, roles for the built-in issuer (ADR-103) | while the account exists |
| `/etc/unheaded/alertmanager.env`, `/etc/unheaded/alert-mailer/` | identifier | mail relay, sender and recipient addresses | while configured |
| `~/.ssh/authorized_keys` (both hosts) | identifier | public keys with their comment fields | while access is granted (access review, UH-AC-01) |
| git history | identifier | author and committer name and mail on every commit | permanent (policy v1 row 1) |
| Well backup sets (west, east) | all of the above | nightly dumps of every database | 14 sets |

## Not held

Packet payloads; adopters' application data; payment data; health data; government identifiers; data about children. A new table or log field that adds personal data updates this file in the same change.
