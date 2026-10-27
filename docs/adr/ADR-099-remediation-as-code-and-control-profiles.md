<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-099: Remediation as code, with control profiles that switch fixes on and off

**Status:** Proposed
**Date:** 2026-10-01
**Amended 2026-10-01 (same day):** state lives in **The Well**, YAML is the seed and fallback. Stevie: *"They should exist in the well"*; *"yaml is ok as fallback but db config is ideal long term"*. Profiles, host bindings, the apply journal, findings triage and evidence are Well tables; fix definitions, the catalog and frameworks stay repo-authored (they run as root or are pinned by tests) and are mirrored into The Well by sha256; hosts act only on a Gungnir-sealed snapshot, never a live row; The Well stays on loopback and the collector pulls host journals over SSH. The decision table and the execution order are in `docs/battle-plans/COMPLIANCE-WELL-BATTLE-PLAN.md`. Where this ADR says "`compliance/profiles/<name>.yaml`" or "`compliance/hosts/<host>.yaml`", read: the Well table, seeded from and falling back to that file.
**Related:** ADR-100 (SRE alongside compliance), ADR-097 (crosswalk: controls and evidence), ADR-098 (findings register; audit and enforcing modes), `pkg/iac` (one desired-state model, interchangeable IaC renderers).

## Context

Stevie, 2026-10-01:

- **Fixes should be repeatable and enforceable.** Note how each fix is implemented, along the way.
- **Fixes should be usable as compliance as code,** for example Ansible playbooks and runbooks.
- **Controls should be easy to switch:** all on, none, or anything in between. Long term, switching from the dashboard needs logins and RBAC.

What exists today:

- `scripts/host/remediate.sh` holds the first ten fixes. Each is a bash function with plan, backed-up apply and undo, and they are tested against a fake root. They were applied on both hosts on 2026-10-01.
- The fixes are code, though, not data. No other tool can read them:
  - the enforcing agent (ADR-098);
  - Ansible, or the other `pkg/iac` backends;
  - the dashboard.

  Nothing records which fixes a host is meant to have, or which it actually has.

## Decision

### 1. One fix model, as data

`compliance/remediation/fixes.yaml` describes every fix once.

Each fix carries:

- **Identity and scope:** `id`, `title`, the controls it serves, and the evidence sources it closes (so the register can say which fix closes which finding).
- **Risk and class:** its `risk` and its ADR-098 `class`.
- **Hosts:** which hosts it applies to, with per-host values. For example, `accept_ra=0` applies on east only.
- **Actions:** an ordered list of actions drawn from a **closed set of types**:

| Action | Does |
|---|---|
| `file` | Writes a whole file: path, mode, owner, group, content (optionally per host). Drop-in files only; never edits a line inside a file someone else owns. |
| `mode` | Sets mode, owner and group on an existing path. |
| `service` | Enable, disable, start, stop or mask a unit. |
| `package` | Install a package. Removal is never automatic (ADR-098 class `package`). |
| `run` | One command from a fixed allowlist, such as `sysctl --system` or `systemctl reload ssh`. |
| `validate` | A command that must succeed after the change, or the whole fix is undone. Examples: `visudo -c`, `sshd -t`. |
| `setting` | Sets one `key=value` in a file owned by a package (e.g. `/etc/default/apport` `enabled=0`); the only in-place edit allowed, one key, backed up. |
| `absent` | Removes a file (backed up first), e.g. `/etc/cron.deny`. |

A `file` action's content may instead come from a named generator from a closed set (first: `root-and-group-members`, which builds `cron.allow` from root plus a group's members at apply time).

There are no free-form shell steps. That keeps a fix reviewable, renderable to other tools, and safe to run unattended once enforcing is allowed.

### 2. Profiles switch controls

`compliance/profiles/<name>.yaml` lists the fixes a profile turns on. `all: true` turns on every fix.

| Profile | Contents |
|---|---|
| `none` | Nothing. Detection and reporting only. |
| `lab` | What west and east run today. |
| `cis-l1-server` | Every fix that serves a CIS Level 1 Server recommendation. |
| `all` | Everything. |

`compliance/hosts/<host>.yaml` binds each host to one profile and an ADR-098 mode (audit or enforcing). It may also turn single fixes on or off for that host.

Switching all controls on or off is a one-line change, and it is a signed commit. That commit is change-management evidence (CM-3).

**Profiles switch remediation, never detection.** Every check still runs and every failure is still reported, as Stevie put it: reporting misconfigurations is fine. A finding whose fix is switched off in the host's profile stays open and counted, and the register shows the reason: off in profile `lab`. Nothing is subtracted (ADR-098 honesty rules).

### 3. Renderers, the `pkg/iac` way

The same model renders to:

- **The native executor,** which takes over from `remediate.sh`. It keeps plan by default, backed-up apply, newest-first undo, and validate-or-restore. The ADR-098 agent uses the same executor when a host is enforcing, and applies only the fixes in that host's profile.
- **Ansible:** one role per fix, a playbook per profile, and an inventory from `compliance/hosts`. The rendered files live in the repo, and a test fails when they drift from the model, as `baseline.yaml` does today.
- **Runbooks:** one `runbook/v1` per profile, listing plan, apply and verify steps.
- **Later,** the other `pkg/iac` backends.

### 4. A journal of what was applied

Every apply appends a record to `/var/lib/unheaded/remediate/journal.jsonl`: host, fix, model commit, time, backup directory and operator. A new evidence kind reads it back, so the record of "fix X applied to host Y" is itself evidence. That covers both "noted along the way" and an auditor's "show me when this was changed". The host files in `compliance/hosts` record what is applied, and the journal shows when.

### 5. Admin changes from the dashboard come later

Until the dashboard has logins (FND-001 and the service-auth finding), it shows profiles and host bindings **read-only**. Switching controls from the dashboard needs:

- authentication;
- an RBAC role allowed to change profiles;
- an audit record per change;
- and writing the change back as a signed commit or a signed record. It is never a silent edit to a host.

Until then, switching is a commit.

## Phases

| Phase | Scope | Host changes |
|---|---|---|
| 1 | This ADR. The model (`fixes.yaml`) with the ten existing fixes, validated by a loader. Profiles `none`, `lab`, `cis-l1-server` and `all`. Host bindings recording what is applied today. A dashboard read-only view of profiles. | none |
| 2 | Native executor with parity to `remediate.sh`: its tests are ported and must pass, then `remediate.sh` is retired. Journal plus evidence kind. | none (same fixes) |
| 3 | Ansible renderer in `pkg/iac`, rendered output in the repo, drift test, and runbooks per profile | none |
| 4 | Enforcing mode uses the executor with the host's profile, behind ADR-098's four conditions | per host, with Stevie's yes |
| 5 | Dashboard admin: logins, RBAC, signed change records | after auth exists |

## Consequences

- A fix is written once. The executor, the agent, Ansible and the dashboard all read the same description, so they cannot drift apart.
- "All controls on" and "none" are one-line, reviewable changes that leave evidence.
- Free-form shell is gone from remediation. Any fix that cannot be expressed in the action set needs a new, reviewed action type.

## Applied so far (2026-10-01)

On east, then west, with Stevie's approval ("remediate all low hanging fruit; nothing should break"):

- `apport-off`
- `sysctl-cis`
- `kmod-cis`
- `cron-perms`
- `banner-cis`
- `apparmor-utils`
- `auditd`
- `sudo-cis`

Each one's backup and `undo.sh` is under `/var/lib/unheaded/remediate/` on each host.

Not applied:

- `sshd-safe`: SSH stays as it is.
- `shell-tmout`: it would end idle shells.

These become the `lab` profile.

## As built (2026-10-01)

Plan and step-by-step status: `docs/battle-plans/COMPLIANCE-WELL-BATTLE-PLAN.md`.

| Piece | Where | Proven by |
|---|---|---|
| Fix model: the closed action set, allowlisted commands, checked against the catalog | `pkg/compliance/remediation`, `compliance/remediation/fixes.yaml` | `TestModelMatchesRemediateScript`: the model renders what `remediate.sh` writes, 40 subtests |
| Profiles and host bindings in The Well, with history; YAML as the labelled read-only fallback | `db/migrations/013`, `pkg/compliance/well` | `scripts/well/test-compliance-schema.sh [--go]`: constraints and integration tests against a throwaway Postgres |
| Apply journal: hash-chained on each host, ingested into The Well, which refuses a rewritten or truncated journal | `remediation/journal.go`, `db/migrations/014`, `well.OpsStore.IngestJournal` | Chain and ingest tests. Without the hash-for-hash check, a rewritten journal got through |
| Native executor and CLI | `pkg/compliance/executor`, `cmd/unheaded-remediate` | Apply then undo restores the root for every fix. `TestExecutorMatchesRemediateScript` shows identical files and commands to the script |
| Switching | `compliance-evidence profile set` (shows on/off, `-yes`, who and why) and `profile export` | CLI and round-trip tests |
| Renderers | `pkg/iac` `RenderRemediationAnsible`, `deploy/ansible/compliance/`. Runbook: `runbooks/security/compliance-profile-switch.yaml` | Structure and drift tests. **Not yet syntax-checked with `ansible-playbook`** |
| Evidence that fixes are applied and current | Kind `host-journal`, control UH-REMED-01 | `JudgeJournal` cases. Both hosts fail today with "no journal": the lab fixes were applied by `remediate.sh` |
| Hosts act only on sealed snapshots | `pkg/compliance/snapshot`, `compliance-evidence snapshot`, `unheaded-remediate -snapshot` | Refused on: another host or key, an edited, expired or downgraded seal, or a validly sealed command not on the allowlist |
| Dashboard | `/api/v1/compliance/profiles` (read-only); every response labelled with its source; fallback banner; `ComplianceWellFallback` alert with a runbook | API tests, jstest `compliance.cjs`, promtool |

Open, waiting on decisions:

- Starting The Well, and applying 013 and 014 to it.
- `unheaded-remediate plan` on the hosts, then re-applying the lab fixes through the executor so they are journaled. After that, `remediate.sh` is retired.
- Which key seals real snapshots, and where it lives (ADR-043 #4).
- Installing Ansible for the syntax check.
- Dashboard admin (switching from the UI) waits for logins and RBAC (§5).

## Progress (2026-10-27): deploy history and the chaos journal in The Well

§4's "journal of what was applied" now also covers deploys and faults (S-WELL-SPRING Phase 2, M-072).
Migration `039_ops_deploy_chaos.sql` (unheaded_ops, append-only by trigger, ops_writer INSERT/SELECT,
ops_reader SELECT) holds `deploy_history` + `deploy_history_transitions` (Sword: the request, then every change
with the whole record) and `chaos_journal` + `chaos_journal_transitions` (Yaldabaoth and `chaos-controller
packet-chaos`: each fault's revert recipe before it lands, then a `reverted` row; never an edit). Views
`deploy_history_current` and `chaos_journal_open`. Stores: `pkg/database` `DeployStore`, `ChaosStore`.

- The Well is used when `WELL_DB` is set; set and unreachable (or 039 missing) is a refusal to start, never a
  fall back, so one history is never split across two stores. Without `WELL_DB`, `pkg/statefile` files are the
  documented fallback (`SWORD_DATA_DIR/sword.json`, `CHAOS_STATE_DIR/journal.json`, `packet-chaos.json`).
- Rows are per host (`KINGDOM_NODE`, else the hostname): a Sword rolls back through its own host's Cuirass, a
  controller reverts only its own faults, and west and east share one Well.
- Switching a host to the Well moves its file state in once: Sword imports `sword.json` into an empty Well; the
  controller moves open journal entries in, then empties the file.
- `pkg/statefile` remains for architect, micromanager and `unheaded-daemon`'s desired store (follow-on, not
  this sprint).
- Proof: `scripts/well/test-deploy-journal.sh` (throwaway server: grants, triggers, CHECKs, and the Go stores,
  Sword, Yaldabaoth and `revert-all` against it as ops_writer). Live: 039 is applied in the E-A window; until
  then a rebuilt Sword (compose sets `WELL_DB`) refuses to start.

## Progress (2026-10-27): phase 2 closed, phase 4's code path (S-WELL-SPRING Phase 13)

- **Phase 2 done: `remediate.sh` retired on parity.** Both parity tests passed for all 13 fixes (104 subtests);
  what the script wrote per fix, host and sudo flavour is kept in `cmd/unheaded-remediate/testdata/parity` and
  `TestExecutorMatchesRetiredScript` holds the executor to it. `scripts/host/remediate.sh` is a shim that names the
  `unheaded-remediate` command and exits 2.
- **Executor:** `check <fix>` (exit 3 when not in place) and `-record-existing <fix> -applied-by "..."`, which journals
  a backfilled apply only when check finds the fix in place, and changes nothing: how the 2026-10-01 lab fixes get a
  journal (UH-REMED-01) in the E-D window. Fixed: undo replayed a fix's reload before restoring its files; reloads are
  now replayed last.
- **Model:** 16 fixes. New: `rocm-log-perms`, `aide-install`, `apparmor-enforce` (in `lab-hardened`). Action set:
  `apparmor` (enforce; refuses a profile the operator disabled or forced to complain), `package.no_recommends`, mode on
  `/var/log` files, `service` enable with start, and `systemctl daemon-reload`, `aideinit -y -f` on the run allowlist.
  `closes` is the deviation-to-fix map enforcing uses (the plan's `checks:`). Old fix hashes are unchanged.
- **Phase 3:** the renderer covers the new actions; `ansible-playbook --syntax-check` runs in static-analysis
  (ansible-core 2.21.5, hash-pinned) on every rendered playbook and on the hand-written `deploy/ansible/kingdom/`
  roles (kingdom-base, huginn, victoria).
- **Phase 4, code only:** `unheaded-baseline` applies the fix that closes a deviation through the executor, only after
  a 7-day soak it measures itself, behind a flapping breaker (no third apply of one fix inside 24 h) and the packages'
  `preinst` gate. Every mode file stays `audit`; the flip, its root drop-in and Q5 are Stevie's (E-E).
  `docs/compliance/enforcing-soak.md`; red team LICH-017.
