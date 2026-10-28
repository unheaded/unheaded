<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-098: The rolling findings register, and host baselines in audit or enforcing mode

**Status:** Proposed (rolling: the register section is regenerated, the decision is not)
**Date:** 2026-09-30
**Related:** ADR-097 (compliance crosswalk: the evidence this counts),
ADR-043 (Mímir's Law: drift is alerts-only until LICH-012 clears),
ADR-093 (a gate is not a gate until it has failed), ADR-062 (LICH).

## Context

Stevie asked for three things:

1. **A rolling ADR that holds every open finding.** The work continues until the count reaches 0.
2. **No deviation after 0.** Once a host meets the baseline, any host running Unheaded packages must not be able to drift from it without alerts or alarms.
3. **Two modes, like SELinux on RHEL and Rocky.** An audit (detection) mode and an enforcing mode.

It must be done carefully, not rushed.

What exists today:

- ADR-097 collects evidence that can fail: CI jobs, gate scripts, signed commits, host sysctl, `sshd -T` and host probes over ssh on west and east, and signed attestations. That evidence is written to `var/compliance/evidence.json`.
- The dashboard's `/compliance` page lists the failing checks. Nothing assigns them a severity, an owner or a due date, and nothing stops a finding from quietly disappearing.
- `monitoring/alertmanager/alertmanager.yml` routes every alert to a receiver named `null`, and the monitoring stack is not running on west. **Today an alarm reaches nobody.**
- ADR-043 made drift handling alerts-only (`pkg/enkrateia`). Auto-restore waits on LICH-012, semantic diffs and signed `config.*` topics. LICH-012 is still OPEN.

Consulted:

- **MoatGhost:**
  - A register that cannot go stale or be gamed.
  - Severity without inflation.
  - What an auditor needs: detection time, routing and an exceptions register.
- **Sentinel:**
  - Detection design and silence detection.
  - Alerting for a single operator.
  - The ways enforcement can lock the operator out of west or east.

## Decision

### 1. The register: the count is generated, the triage is written

- **A finding** is any evidence source in the catalog whose latest record is `fail`. A stale fail stays a fail (ADR-097). Findings the tools cannot see yet are listed by hand in the same file.
- **Stable IDs.** A machine finding is identified by its source key, e.g. `host-sshd:east:passwordauthentication=no`. A hand-listed finding gets `FND-NNN` and must say how it will be verified.
- **Triage is hand-written** in `compliance/findings/register.yaml`: severity, class, host, the date it opened, lockout risk, remediation step, plan, and any decision pending. Entries are signed commits, so every change is change-management evidence.
- **The generator** is `go run ./cmd/compliance-evidence findings`. It joins catalog, evidence and register and prints the table. With `-adr` it rewrites the generated section at the end of this file, between the markers.

**Rules that stop the number being gamed:**

- **A finding closes when its evidence passes, never when its entry is edited.** A register entry whose source now passes is reported as resolved, remove me. It is not counted as open.
- **Untriaged findings count.** A failing source with no register entry still counts, as severity `untriaged`.
- **Deleting a check is visible.** A register entry must name a source the catalog still declares, or loading fails. Removing a check therefore removes its entry in the same signed commit, and the diff shows both.
- **Accepted risk is still open.** `accepted` needs a reason, a compensating control and an expiry no more than 90 days out. Accepted findings are counted separately but are not subtracted. An expired acceptance is flagged.
- **Zero is claimable only when all three hold:**
  - 0 findings open.
  - 0 sources never observed. A missing attestation is `NOT_ASSESSED`, which is not a pass.
  - 0 stale sources.

  The report prints `zero claimable: yes/no` and the reason.
- **Due dates come from severity**, counted from `opened`:

  | Severity | Due within |
  |---|---|
  | critical | 7 days |
  | high | 30 days |
  | medium | 90 days |
  | low | 180 days |

  Overdue findings are flagged. These are targets for a project with one operator. They are not contractual SLAs.

**Severity** follows exposure × impact, using the MoatGhost rubric: no inflation, one blocker is a blocker. The register records the exposure it assumed. For example, if east is reachable only from the LAN, password SSH is medium. The severity rises if that assumption turns out to be wrong.

**Shrink-only baselines are accepted deviations, not passes.**

- A non-empty `docs/security/compose-hardening-baseline.txt` is a finding.
- `gitleaks-baseline-fingerprints.txt` holds reviewed false positives. It is listed as an exception, not a finding, with the review recorded.

### 2. Two modes, per host, on the SELinux model

| SELinux | Unheaded baseline | What happens on a deviation |
|---|---|---|
| `permissive` | **audit** (the default, and the only mode allowed today) | Detect, log, alert. Record what enforcing *would* have done. Change nothing. |
| `enforcing` | **enforcing** | Detect, log, alert, and apply the class action below |
| `disabled` | none: there is no off switch | A host with Unheaded packages but no agent reporting is itself a critical alarm |
| `semanage permissive -a <domain>` | per-control exemption | Allowed only while the register holds an unexpired acceptance for that control on that host |
| `setenforce 0` | local downgrade to audit | Always possible for root (break glass). Always a critical alarm. The host is a finding until the repo inventory agrees. |

Where the mode is recorded:

- The host reads its mode from a root-owned `/etc/unheaded/baseline.conf`.
- The intended mode lives in the repo, in `compliance/hosts/<host>.yaml`, as a signed commit.
- If the two disagree, that is a finding.

This is the honest version of "cannot deviate": **root on a host can always defeat an agent on that host.** The guarantee is that the host cannot deviate *undetected*. It rests on two independent paths:

1. The host agent pushes its reports.
2. The central collector pulls independently over ssh (ADR-097, as today).

If they disagree, that is a tamper alarm. If the agent goes silent, that is also an alarm.

**What enforcing does, per finding class.** Each class has a fixed, typed, idempotent action and a stated lockout risk:

| Class | Enforcing action | Lockout risk and safeguard |
|---|---|---|
| sysctl | Re-apply the baseline value from the package's `sysctl.d` file | none |
| service (auditd, time sync, AppArmor, unattended-upgrades) | Unmask and start the unit | none. Never set auditd `disk_full_action=halt`, never lock auditd rules with `-e 2`, so the host cannot halt. |
| sshd | Restore the package-owned drop-in `sshd_config.d/00-unheaded.conf` (first value wins in sshd). Then run `sshd -t`, then `reload`. | **high.** Never touches `authorized_keys`. A reload keeps existing sessions. If `sshd -t` fails, the reload is skipped and it alarms. The key-login precheck is described below. |
| firewall | Atomically reload the package-owned `nft` table `inet unheaded`. The ssh allow rule comes before the policy drop. | **high.** Only a ruleset the operator already confirmed can be restored. Operator changes use commit-confirm: roll back after 120s unless confirmed. |
| kmod | Write `/etc/modprobe.d/unheaded-cis.conf` (`install <m> /bin/false`, `blacklist <m>`); unload only if loaded and unused | none |
| fileperm | chown to root (and the CIS group) and chmod to the CIS mode | none |
| package | None: alert only. Removing a package can take dependents with it, so the operator runs `apt purge`. Units of an unneeded service are disabled and masked (class service) | none |
| sudo | Write `/etc/sudoers.d/00-unheaded` (0440): use_pty, logfile, timestamp_timeout. `visudo -cf` must pass first, otherwise nothing is installed | high: a broken sudoers file removes sudo, so it is validated before install and never edited in place |
| boot | None: alert only. A boot loader password is the operator's to set and keep | n/a |
| mac | None: alert only. Moving an AppArmor profile to enforce can break the confined program | n/a |
| account | None: alert only. Password ageing, shells and group membership change how people and services log in; the operator applies them | n/a |
| shell | Write `/etc/profile.d/00-unheaded-tmout.sh` (readonly TMOUT=900; export TMOUT) | none: it only affects new login shells |
| mount | None: alert only. Mount options and partitions change through fstab and a remount or reboot, which the operator does | n/a |
| package gate | Installing or upgrading an Unheaded `.deb` on a host outside its baseline is refused by `preinst`. In audit mode it only warns. | none. Running services are never stopped. |
| app / CI / backup / process | Not a host action. Enforcement is by CI gates and alarms. | none |

**Flapping** (the agent reverts, someone changes it back, the agent reverts again): after 3 reverts of the same check in 1 hour the agent stops reverting that check, marks it `contested` and alarms. It never fights a human in a loop.

**This does not amend ADR-043.** Enkrateia stays alerts-only for general file drift. Enforcing here is narrower:

- It uses a closed list of the typed actions above, and never an arbitrary file restore.
- Its baseline comes only from the signed package installed on the host.
- It never acts on a bus message.

Even so, it is the same class of risk that BlackMage gated in ADR-043. So:

**No host may be switched to enforcing until all four of these hold:**

1. LICH-012 has been run against this agent: forged baselines, time-of-check/time-of-use (TOCTOU) races on restore, flapping, and silencing the agent.
2. The host has run in audit mode for 7 days with zero would-have-enforced events.
3. The host has zero open findings in the enforceable classes. A baseline the host does not already meet cannot be enforced; SELinux likewise needs a relabel before enforcing.
4. Stevie has signed off on that host in `compliance/hosts/<host>.yaml`.

### 3. Detection and alarms

- **The agent** is `unheaded-baseline`. A systemd timer runs it every 5 minutes, and path units re-run it straight away when `sshd_config.d`, `sysctl.d` or the nft ruleset changes.
- **Agent output:**
  - Structured journald events.
  - Prometheus metrics through node-exporter's textfile collector, so no new port or listener is added:
    - `unheaded_baseline_check{control,check}`
    - `unheaded_baseline_mode`
    - `unheaded_baseline_last_run_timestamp_seconds`
  - A Wotan `compliance.deviation.<host>` event for the live dashboard. Wotan is not the alarm path; the alarm path stays simple and independent.
- **Time to detect** (targets, to be measured before they are claimed):

  | What | Target |
  |---|---|
  | A change to a watched file | under 1 minute |
  | Any other configuration drift | 5 minutes or less |
  | An agent gone silent | 15 minutes or less |
  | Independent central confirmation | 1 hour or less |
- **Silence detection:**
  - `time() - unheaded_baseline_last_run_timestamp_seconds > 900` is critical.
  - node-exporter being down is critical.
  - An always-firing Watchdog alert goes to a heartbeat outside the monitoring stack. If the whole stack dies, the missing heartbeat is noticed. West and east watch each other, with no third party (open question 2).
- **Routing for one operator:**
  - **Critical** pages at any hour. That covers:
    - a mode downgrade;
    - a silent agent;
    - an agent and central collector that disagree;
    - a weakened sshd auth setting;
    - a firewall policy of ACCEPT on an enforcing host;
    - a contested check;
    - an expired acceptance.
  - **Warning** goes into a daily digest.
  - **Info** shows only on the dashboard.

  Paging only on critical keeps alarm fatigue down.
- **Alertmanager's `null` receiver is finding FND-001.** Enforcement without a working alarm path would be theatre.

### 4. Remediation order: safe first, lockout risk last

Every host change is proposed with its exact commands and applied only after Stevie approves it. East goes first, because it is the sacrificial host.

1. **No host risk.** Push the local CI fixes (red CI). Set up alert routing (FND-001). Run a backup and put it on a timer. Write the three attestations.
2. **sysctl:** `fs.suid_dumpable`, ICMP redirects. Reversible, no lockout.
3. **auditd,** with a safe configuration. Check disk space first.
4. **Low-risk sshd keys:** `X11Forwarding`, `MaxAuthTries`, `ClientAlive*`.
5. **sshd authentication:**
   - Before turning off passwords and keyboard-interactive, prove key login works for Stevie's user.
   - Before setting `PermitRootLogin no`, prove no automation logs in as root.
   - Keep an existing session open while the change is made.
6. **Firewall default-deny.** East first, with commit-confirm, then west.
7. **Application level:** `AUTH_ENABLED` on the 11 services (clients need tokens, so this needs a distribution design first), edge TLS and Traefik `api.insecure`, then the seven third-party containers one at a time.

**Applying a fix.** Low-risk fixes (steps 2 to 4) are named fixes in `scripts/host/remediate.sh`:
`sysctl-cis`, `apport-off`, `kmod-cis`, `cron-perms`, `shell-tmout`, `sshd-safe`.

- **Plan is the default** and changes nothing. `apply` needs root and backs up every file it touches. It also writes an `undo.sh` that replays those changes newest first.
- **Content is host-aware.** East gets `accept_ra=0`; west does not, because it takes its ISP IPv6 address from router advertisements.
- **West is never loosened.** Its existing `99-unheaded.conf` already sets stricter values than CIS asks for. `sshd-safe` does not repeat them, because sshd uses the first value it reads, so repeating them would override the stricter ones.
- **`sshd-safe` runs `sshd -t` before reloading.** If the check fails, it restores every file and does not reload.
- **Anything awaiting a decision stays out.** For example, usb-storage is left loadable.
- **Tests:** `scripts/host/remediate_test.go` runs every fix against a fake root with fake binaries.

sshd authentication and the firewall stay outside the script; the firewall goes through `nft-apply-confirm.sh`.

### Phases

| Phase | Scope | Host changes |
|---|---|---|
| 0 | This ADR, `register.yaml`, the `findings` generator, a register view on the dashboard | none |
| 1 | Alert routing and the Watchdog heartbeat (needs answers to questions 1 and 2). Evidence metrics and Prometheus rules. | monitoring stack only |
| 2 | `unheaded-baseline` agent, audit mode only, packaged as a `.deb`, installed on east, then west | install an agent that only reads |
| 3 | Burn-down, following the section 4 order | yes, each approved |
| 4 | Enforcing mode, per host, behind the four conditions in section 2 | yes, each approved |

Progress:

- **Phase 0 is done.** It covers the register, `compliance-evidence findings`, `/api/v1/compliance/findings` and the dashboard view.
- **Phase 1 has started.** dashboard-backend exports `unheaded_compliance_*` metrics, and `monitoring/prometheus/rules/compliance.yml` holds 9 rules, unit-tested with promtool in `rules/tests/`.
- **Phase 1 still needs:**
  - a real receiver and the heartbeat (questions 1 and 2);
  - the monitoring stack running on west;
  - a timer for `compliance-evidence`.

### Phase 2 specification: `unheaded-baseline`, audit only

- **One definition of the baseline.** `compliance/baseline/baseline.yaml` is generated from the catalog by `compliance-evidence baseline`. A test fails if it is out of date. It lists every host-scoped source: `host-sysctl`, `host-sshd`, and `host-probe` for probes marked as baseline probes. Operational probes are excluded; for example, backup recency depends on the operator's `$HOME`.
- **Where it runs.** The package installs the baseline at `/usr/share/unheaded/baseline/baseline.yaml`. The agent refuses a baseline file that is not root-owned, or that group or others can write, in the same way sshd's `StrictModes` works.
- **What it checks.** The agent evaluates only the sources for its own hostname. It uses the same collectors as the central pull, so the two paths cannot drift in what they check. It never uses ssh: any other host is an error.
- **Mode.** The agent reads `/etc/unheaded/baseline.conf` (`mode=audit|enforcing`); a missing file means audit. **This build has no enforcing code path.** If the file asks for enforcing, the agent reports `mode_config=enforcing mode_effective=audit` as a critical event. The config therefore cannot switch enforcement on by accident.
- **Output.**
  - `/var/lib/unheaded/baseline/report.json`, written atomically. It holds host, modes, time, the baseline sha256, and every check with its verdict and detail.
  - One journald line per deviation, carrying `would_enforce=<action>`. This is the audit-mode record of what enforcing would have done, like SELinux logging `permissive=1` denials.
  - With `--textfile-dir`, a node-exporter textfile.
- **Exit codes.** A deviation still exits 0, because it is data, not a failure. Failing to evaluate exits 1, so `systemctl --failed` shows a broken agent.
- **Checking the agent against the central pull (phase 2c).** The central collector reads each host's `report.json` over ssh and compares verdicts. A disagreement is a tamper alarm.
  *Built 2026-10-13:* evidence kind `host-agent` (`<host>:baseline-report`, control UH-BASE-01, `collect/agent.go`). It also fails on a missing report, one older than three hours, a baseline whose sha256 differs from the repo's, or a mode config other than audit. Not installed on either host yet, so both read "no agent report"; run into a scratch directory on west, the agent agreed with the central pull on all 219 checks both evaluated.

## Open questions for Stevie

1. **Alarm receiver for pages:** self-hosted ntfy, email, or something else?
2. **Dead-man heartbeat:** west and east watching each other, or an outside service?
3. **Recovery if SSH or the firewall locks you out:** is there console or out-of-band access to east?
4. **Exposure:** is anything on west or east reachable from the internet, not just the LAN? Several severities depend on the answer.
5. **Enforcing strictness:** should enforcing ever refuse to *start* Unheaded services on a host that is out of baseline, or only refuse installs? The draft only refuses installs, because availability is also a control.

### Answers (Stevie, 2026-09-30)

1. **Email** now, with PagerDuty or similar later. Still open: the SMTP relay. Credentials live in a file outside the repo.
2. Not answered. The draft default is west and east watching each other, with no third party.
3. **Physical access** to both hosts exists. That is the recovery path for the sshd and firewall steps.
4. West and east are directly connected over the P2P link on the same switch. **Later on 2026-09-30, Stevie:** the ISP's edge firewall drops unsolicited inbound traffic, so FND-004 is closed. That rests on his statement; it has not been tested from outside. Severities keep assuming LAN-only exposure.
5. Not answered yet; installs-only remains the draft.
6. **Edge TLS is deferred.** There is no internet-facing edge in develop mode; the plan is Let's Encrypt once one exists. The finding is accepted until 2026-12-29.
7. **Backups:** GitHub is enough for now, so the database backup finding is accepted. Unpushed commits on west are to be mirrored to east every 30 minutes (FND-005, low priority).
8. **Service auth:** several modes (SSO/OIDC, JWT, API key), selectable per deployment.
9. **SSH forwarding** may be used in develop, so CIS DisableForwarding (Level 2) is accepted on both hosts.
10. **2026-10-01: SSH and passwords stay as they are; this is a lab.** Every sshd, PAM, password-ageing, boot-loader-password and idle-shell finding is accepted until 2026-12-30. They stay counted and are reported, not changed.

    Low-hanging fixes are approved, on the condition that nothing in the environment breaks. Reporting misconfigurations is fine.

## Consequences

- There is one number, and it can only go down by fixing things or by visibly accepting risk with an expiry.
- Enforcing mode exists only behind a red-team pass and a soak period. Until then every host runs in audit mode, which is ADR-043's alerts-only rule with a register attached.
- New work: the agent, its `.deb`, Prometheus rules, alert routing and a LICH-012 campaign scoped to the agent.

## Register

Generated by `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md`. Do not edit by hand. Triage lives in `compliance/findings/register.yaml`.

<!-- BEGIN GENERATED FINDINGS -->
As of 2026-10-28T11:37:35Z: **13 open** (2 high, 6 medium, 5 low), 0 accepted, 0 overdue; zero claimable: **no** (13 open, 479 never observed).

| Severity | Finding | Host | Class | Step | Lockout | Due | Observed | Plan |
|---|---|---|---|---|---|---|---|---|
| high | `FND-001` | west | app | 1 | none | 2026-10-30 | Alerts reach nobody. Alertmanager routes everything to a receiver named null, and the monitoring stack is not running on west. | email receiver (Stevie 2026-09-30; PagerDuty or similar later); run the monitoring stack; Watchdog heartbeat cross-watched by west and east. Email receiver BUILT 2026-10-19: scripts/monitoring/render-alertmanager.sh renders it from /etc/unheaded/alertmanager.env (relay, from, to), password from a file outside the repo; critical repeats hourly and silences its own warning; tested end to end against an SMTP sink (CI). 2026-10-20: Prometheus had no alerting block, so no alert ever reached Alertmanager; fixed (d5e39b31) and the same test now proves a firing rule arrives. The monitoring stack can now start on west (its subnet overlapped kind/netbox) and needs GRAFANA_ADMIN_PASSWORD in monitoring/.env. 2026-10-20 at Stevie's word: monitoring/.env written, the stack runs on west (5/5 healthy), every deployed host target up, alerts reach Alertmanager (only true ones firing). What is left is the SMTP relay choice and its env and password files (the receiver is still null). 2026-10-20 (Stevie: "deviation in slo and sla should be the only alerts - no noise", "I might read 5"): ADR-100 built. Only SLO burn-rate alerts notify (observability/alerting/policy.yaml, 7 SLOs, urgency page/ticket/none); alert-mailer is the only sender, capped at 6 a day, tickets in one 09:00 digest, running on west in dry-run. Left: a week of dry-run outbox review, then the Gmail app password (wiki/Alerting.md) and mode send. 2026-10-27 (S24, Stevie: send from personal Gmail stevenrbellis@gmail.com to stevie@bellis.tech, short-term): app password at /etc/unheaded/alert-mailer/smtp-password (0400, uid 16710), relay keys in the gitignored monitoring/.env, one synthetic page sent through smtp.gmail.com:587 (outbox outcome sent, HTTP 204), policy.yaml mailer.mode=send, monitoring stack up on west. Left: the first real SLO page, or the alert-route host probe, closes this entry **decision:** SMTP relay: personal Gmail (Stevie 2026-10-27, short-term; Workspace account later). Left dry-run 2026-10-27 |
| high | `FND-011` |  | package | 3 | none | 2026-11-18 | Two Linux kernel vulnerabilities on CISA's KEV list (exploited in the wild) are unpatched on west and east, and Ubuntu has no fixed kernel yet: CVE-2026-53266 (ebtables SNAT out-of-bounds write, KEV 2026-09-18) and CVE-2026-53362 (privilege escalation via the IPv6 stack, KEV 2026-08-27). Both hosts run 6.17.0-41-generic | Install the fixed kernel as soon as Ubuntu publishes it (unattended-upgrades or apt), then reboot; re-run the verify commands. Until then, for CVE-2026-53266: nothing here uses ebtables, so /etc/modprobe.d/unheaded-cis.conf can gain `install ebtables /bin/false`, `install ebtable_nat /bin/false`, `install ebt_snat /bin/false` (no module to unload today). CVE-2026-53362 has no published mitigation short of the fixed kernel. 2026-10-27 (S24, S-CLEAN-SLATE E4, Stevie's yes): the stopgap is in remediate.sh kmod-cis and fixes.yaml (9d3c6cf1, 12adcb92) and applied on east (12:42:11Z) then west (12:42:12Z); on both, modprobe -n -v ebtables shows /bin/false, a real modprobe fails, no ebt module is loaded; undo dirs under /var/lib/unheaded/remediate/. LXD (nftables driver) and Docker unaffected. This entry stays open until the fixed kernel is installed **decision:** stopgap applied 2026-10-27; west keeps unprivileged user namespaces open for LXD until an LXD test says closing them is safe (Stevie, decision 6) |
| medium | `FND-027` |  | ci | 1 | none | 2027-01-26 | The main CI workflow (.github/workflows/ci.yml: Go build and test, golangci-lint, Rust check, SPDX, licence compliance and the CI Gate) is disabled in GitHub (state disabled_manually); its last run was 2026-02-20 | Prerequisites done on develop 2026-10-28: golangci-lint v2.15.0 at 0 issues and pinned in ci.yml (ba25221f), go-licenses pinned and passing (9cd6cc05). What remains is Stevie's: re-enable the workflow (Actions, CI, Enable workflow) and check one green run. Enabling is a GitHub setting, so not done unattended. |
| medium | `FND-010` |  | service | 7 | none | 2027-01-16 | Units that still start as root: the three Doom units (BPF/XDP/netns, binaries under /home/govan) and the Well's backup/drill/offsite jobs (docker exec into the Well, i.e. the Docker socket) | Well jobs: PostgreSQL 16 client+server binaries on the host (PGDG), backup over TLS as well_backup from its own account with a systemd credential, drill as an unprivileged throwaway cluster in a private dir; waits for Stevie's Ubuntu 26.04 upgrade (he does OS-level changes). Baseline: DONE, installed on east then west 2026-10-22 at Stevie's yes: runs as unheaded-baseline with CAP_DAC_READ_SEARCH + CAP_NET_ADMIN, root-only facts from one ExecStartPre=+ line; its 219 verdicts match the central pull on both hosts. Doom: unheaded-doom with CAP_BPF CAP_PERFMON CAP_NET_ADMIN CAP_SYS_ADMIN, artifacts installed out of /home **decision:** PGDG apt repo + postgresql-16 packages on west, after the OS upgrade |
| medium | `FND-015` | west | app | 7 | none | 2027-01-21 | The Well's TLS key exchange is classical P-256: PostgreSQL 16's ssl_ecdh_curve takes one curve, so no client can negotiate a hybrid ML-KEM group with it (proved in throwaway containers; PG 18 with ssl_groups negotiates X25519MLKEM768 with every client we run) | E1: runbooks/data/well-pg18-upgrade.yaml (dump/restore into a new volume with restore-drill.sh --into-volume, PG 16 volume kept as the rollback, db/postgresql.pg18.conf.d/10-pq-groups.conf with ssl_groups=X25519MLKEM768:X25519:prime256v1). Clients need no change (libpq 16/18 and lib/pq offer the hybrid) **decision:** E1, when to take the Well to PostgreSQL 18 (downtime, data-bearing major upgrade) |
| medium | `FND-017` |  | app | 7 | none | 2027-01-21 | Wotan and the internal service links run without TLS in the compose stack (no -enable-tls), so their traffic is plaintext, post-quantum or not | The ADR-034 / ADR-103 sprint (certificates, distribution, service auth). The code is ready: Wotan gRPC, wotan-client and pkg/transport/mtls are hybrid-only TLS 1.3 the day TLS is on (S-QUANTUM-ARMOR Phase 5) Progress 2026-10-27 S-IRON-GATE Phase 8 code merged (Strict TLS 1.3 hybrid on every first-party listener and link, per-service dev certs, compose and units on TLS, no plaintext fallback); closes when Steps 264-265 pass in compose and on hosts after E3 **decision:** when to turn TLS on for internal links (accepted as plaintext 2026-09-30 with the edge finding) |
| medium | `FND-025` |  | app | 7 | none | 2027-01-26 | Shield's HMAC-key, retry-token, hop-validator, GOAWAY and authority checks are stubs (lookups whose result is discarded, or no lookup at all) while a handoff doc said WIRED | S-TRUE-WIRE Phase 5 (Steps 169-230, on sprint/true-wire): pkg/shieldmaps and cmd/shield write the maps from a host keys file; the checks run in one bpf_loop in observe mode, enforce behind SHIELD_CONFIG; GOAWAY and authority follow the Phase 4 ruling (enforce or delete); docs corrected first, then again when true. |
| medium | `FND-026` | west | service | 7 | none | 2027-01-25 | West serves an old unsigned apt repo on 0.0.0.0:18888 with autoindex; its conf/ and db/ are readable on the LAN. | Retire it in the S-IRON-GATE E3 window: runbooks/deploy/apt-repo-publish.yaml removes the old site and installs deploy/nginx/unheaded-apt.conf (127.0.0.1 and 192.168.13.2 only; only dists/, pool/ and the key served, everything else 403) with the signed reprepro repo (scripts/apt-repo.sh). Host change, so not done unattended. |
| low | `FND-013` | west | package | 3 | none | 2027-04-17 | PyTorch carries two published advisories: the pinned torch==2.10.0 (raft/requirements.txt, sbom/python-pip-freeze.txt) and, worse, the torch actually installed on west, 2.6.0 (+rocm6.1 system-wide, +cpu in ~/tmp/gemma4-venv): CVE-2025-3000 (memory corruption in torch.jit.script, fixed in 2.13.0) and CVE-2026-4538 (deserialization in the pt2 loader, no fixed release) | Move the training environment and the pins to a current torch (2.14.1 on PyPI 2026-10-19) with the matching ROCm wheel, run a short forge/RAFT training smoke on the GPU, then regenerate sbom/python-pip-freeze.txt from that environment rather than by hand. Until PYSEC-2026-139 has a fix, load only .pt2 files built here **decision:** when to take the GPU stack to a current torch (a ROCm upgrade and a training run, not a pin edit) |
| low | `FND-009` |  | container | 7 | none | 2027-04-16 | 22 services in the compose files other than docker-compose.yml are not least-privilege (was 75 on 2026-10-18): the firewall VMs (opnsense, frr, ipfire, bird: privileged or root), shield and unheaded-daemon in host-a (the D11 host-network decision), WireGuard twice (root by necessity, bounded), and 13 host-a services whose images exist nowhere in the repo | Done 2026-10-20 for 53 services (monitoring, telemetry, sophia-eye, loadbalancers, vllm, host-a/host-b known services): own pkg/uids number, read_only + measured tmpfs, cap_drop ALL, no-new-privileges, volume-init chown; promtail off the Docker socket everywhere (journal/var/log via the adm group); each stack that can run was run on west. Left: WireGuard needs root (EPERM otherwise, measured), so it stays listed with cap_drop ALL + NET_ADMIN and the four s6 init caps, read-only; the firewall VMs need a decision on whether they stay containers at all; host-a's shield/unheaded-daemon wait on D11; the 13 imageless host-a services get uids only if D11 keeps the stack **decision:** whether the firewall VMs stay containers, and D11 (host-a network mode; keep or retire the per-host stacks) |
| low | `FND-016` |  | app | 7 | low | 2027-04-21 | The WireGuard pre-shared key, the post-quantum part of the west-east overlay, has not changed since setup (2026-03-25): one static PSK, never rotated | E3: first live rotation with scripts/wireguard/rotate-psk.sh --apply (dry run by default; waits 200 s for a handshake and rolls back), then enable the weekly timer from deploy/systemd/ (docs/security/pq-host-escalations.md) 2026-10-27 (S24, E3 granted by Stevie): first live rotation done at 12:23:16Z (handshake within seconds, overlay ping 3/3, verify-wireguard 8/8 PASS); wg0.conf backed up to /root on both hosts first; east's setup-era /etc/wireguard/preshared.key shredded. Script and units installed on west, timer NOT enabled: the unit runs as root and root on west has no SSH key for east, so enabling it needs a new root key authorized on govan@east (which has passwordless sudo, i.e. root on east). Closes when that trust decision is made and the timer is enabled. **decision:** E3 rotation done 2026-10-27; open: root-on-west SSH key to east for the weekly timer (trust expansion) |
| low | `FND-018` |  | app | 7 | none | 2027-04-21 | No production evidence build key exists, so no Yggdrasil evidence pack can be signed for real; the sign/verify path works with throwaway keys only | E4: an ML-DSA-65 seed held under ADR-043 #4 custody (HSM/TPM/sealed, or strict ACL with an audit log), separate from the Heimdall and drift keys; publish build.pubkey; a quarterly rotation ceremony Progress 2026-10-27 S-IRON-GATE Phase 13 code merged (ML-DSA-65 signed Binding Rune, cmd/cask-verify, hardened tar extraction, build-key ceremony runbook, uid 16737); custody is Iron Gate E1 **decision:** E4, where the build key lives, who holds it, how build.pubkey is published |
| low | `FND-019` |  | app | 7 | none | 2027-04-21 | Heimdall's signed drift alerts reach no one: no node has a drift key, Wotan's drift. prefix trusts no key (fails closed), and no unit runs Heimdall with --wotan-grpc | E5 (plan Step 246; docs/battle-plans/progress/S-QUANTUM-ARMOR-drift-progress.md): enrol the keys, add heimdall to topics.auto_approve, ship and enable a Heimdall unit Progress 2026-10-27 S-IRON-GATE Phase 9 code merged (unheaded-heimdall.service uid 16736, unheaded-keygen -algo mldsa65, enrol runbook, heimdall on auto_approve via verified identity); keys and running it are Iron Gate E2 **decision:** E5, key custody per node and running a root-reading daemon on the hosts |

- remove from the register: `FND-020` (its evidence passes, or it was closed with evidence)
- remove from the register: `FND-029` (its evidence passes, or it was closed with evidence)
- remove from the register: `FND-028` (its evidence passes, or it was closed with evidence)

Never observed (NOT_ASSESSED, not a pass):

- `github-job:Security Scan (Daily + PR)/Go Vulnerability Check`
- `github-job:Security Scan (Daily + PR)/Rust Cargo Audit`
- `github-job:SBOM + Vulnerability Scan/sbom`
- `github-job:Security Scan (Daily + PR)/Go Security Scan (gosec)`
- `github-job:Static Analysis/Rust (clippy)`
- `github-job:Static Analysis/Python (ruff + bandit)`
- `github-job:Static Analysis/Shell (shellcheck)`
- `github-job:Static Analysis/JavaScript (oxlint)`
- `github-job:Security Scan (Daily + PR)/Secret Scan (gitleaks)`
- `github-job:Security Scan (Daily + PR)/Trivy Filesystem Scan`
- `github-job:Security Scan (Daily + PR)/Lich Fuzz Harnesses`
- `github-job:Static Analysis/Gates can actually fail`
- `git-signatures:develop`
- `github-job:GPL Boundary Check/GPL/AGPL Boundary Verification`
- `gate-script:scripts/check-compose-log-caps.sh`
- `gate-script:scripts/check-compose-bind-nesting.sh`
- `host-journal:west:apply-journal`
- `host-journal:east:apply-journal`
- `gate-script:scripts/check-systemd-units.sh`
- `host-sysctl:west:kernel.unprivileged_bpf_disabled>=1`
- `host-sysctl:west:kernel.kptr_restrict>=1`
- `host-sysctl:west:kernel.dmesg_restrict=1`
- `host-sysctl:west:kernel.yama.ptrace_scope>=1`
- `host-sysctl:west:kernel.perf_event_paranoid>=2`
- `host-sysctl:west:kernel.randomize_va_space=2`
- `host-sysctl:west:fs.protected_symlinks=1`
- `host-sysctl:west:fs.protected_hardlinks=1`
- `host-sysctl:west:fs.suid_dumpable=0`
- `host-sysctl:east:kernel.unprivileged_bpf_disabled>=1`
- `host-sysctl:east:kernel.kptr_restrict>=1`
- `host-sysctl:east:kernel.dmesg_restrict=1`
- `host-sysctl:east:kernel.yama.ptrace_scope>=1`
- `host-sysctl:east:kernel.perf_event_paranoid>=2`
- `host-sysctl:east:kernel.randomize_va_space=2`
- `host-sysctl:east:fs.protected_symlinks=1`
- `host-sysctl:east:fs.protected_hardlinks=1`
- `host-sysctl:east:fs.suid_dumpable=0`
- `host-probe:west:apparmor-enabled`
- `host-probe:east:apparmor-enabled`
- `host-probe:west:pkg-prelink-absent`
- `host-probe:west:svc-apport-not-in-use`
- `host-probe:east:pkg-prelink-absent`
- `host-probe:east:svc-apport-not-in-use`
- `host-probe:west:kmod-cramfs-disabled`
- `host-probe:west:kmod-freevxfs-disabled`
- `host-probe:west:kmod-hfs-disabled`
- `host-probe:west:kmod-hfsplus-disabled`
- `host-probe:west:kmod-jffs2-disabled`
- `host-probe:west:kmod-usb-storage-disabled`
- `host-probe:west:kmod-udf-disabled`
- `host-probe:east:kmod-cramfs-disabled`
- `host-probe:east:kmod-freevxfs-disabled`
- `host-probe:east:kmod-hfs-disabled`
- `host-probe:east:kmod-hfsplus-disabled`
- `host-probe:east:kmod-jffs2-disabled`
- `host-probe:east:kmod-usb-storage-disabled`
- `host-probe:east:kmod-udf-disabled`
- `host-probe:west:perm-etc-passwd`
- `host-probe:west:perm-etc-passwd-backup`
- `host-probe:west:perm-etc-group`
- `host-probe:west:perm-etc-group-backup`
- `host-probe:west:perm-etc-shadow`
- `host-probe:west:perm-etc-shadow-backup`
- `host-probe:west:perm-etc-gshadow`
- `host-probe:west:perm-etc-gshadow-backup`
- `host-probe:west:perm-etc-shells`
- `host-probe:west:perm-etc-security-opasswd`
- `host-probe:west:perm-sshd-config`
- `host-probe:west:perm-ssh-host-private-keys`
- `host-probe:west:perm-ssh-host-public-keys`
- `host-probe:east:perm-etc-passwd`
- `host-probe:east:perm-etc-passwd-backup`
- `host-probe:east:perm-etc-group`
- `host-probe:east:perm-etc-group-backup`
- `host-probe:east:perm-etc-shadow`
- `host-probe:east:perm-etc-shadow-backup`
- `host-probe:east:perm-etc-gshadow`
- `host-probe:east:perm-etc-gshadow-backup`
- `host-probe:east:perm-etc-shells`
- `host-probe:east:perm-etc-security-opasswd`
- `host-probe:east:perm-sshd-config`
- `host-probe:east:perm-ssh-host-private-keys`
- `host-probe:east:perm-ssh-host-public-keys`
- `host-probe:west:mount-tmp-separate`
- `host-probe:west:mount-tmp-nodev`
- `host-probe:west:mount-tmp-nosuid`
- `host-probe:west:mount-tmp-noexec`
- `host-probe:west:mount-dev-shm-separate`
- `host-probe:west:mount-dev-shm-nodev`
- `host-probe:west:mount-dev-shm-nosuid`
- `host-probe:west:mount-dev-shm-noexec`
- `host-probe:west:mount-home-separate`
- `host-probe:west:mount-home-nodev`
- `host-probe:west:mount-home-nosuid`
- `host-probe:west:mount-var-separate`
- `host-probe:west:mount-var-nodev`
- `host-probe:west:mount-var-nosuid`
- `host-probe:west:mount-var-tmp-separate`
- `host-probe:west:mount-var-tmp-nodev`
- `host-probe:west:mount-var-tmp-nosuid`
- `host-probe:west:mount-var-tmp-noexec`
- `host-probe:west:mount-var-log-separate`
- `host-probe:west:mount-var-log-nodev`
- `host-probe:west:mount-var-log-nosuid`
- `host-probe:west:mount-var-log-noexec`
- `host-probe:west:mount-var-log-audit-separate`
- `host-probe:west:mount-var-log-audit-nodev`
- `host-probe:west:mount-var-log-audit-nosuid`
- `host-probe:west:mount-var-log-audit-noexec`
- `host-probe:east:mount-tmp-separate`
- `host-probe:east:mount-tmp-nodev`
- `host-probe:east:mount-tmp-nosuid`
- `host-probe:east:mount-tmp-noexec`
- `host-probe:east:mount-dev-shm-separate`
- `host-probe:east:mount-dev-shm-nodev`
- `host-probe:east:mount-dev-shm-nosuid`
- `host-probe:east:mount-dev-shm-noexec`
- `host-probe:east:mount-home-separate`
- `host-probe:east:mount-home-nodev`
- `host-probe:east:mount-home-nosuid`
- `host-probe:east:mount-var-separate`
- `host-probe:east:mount-var-nodev`
- `host-probe:east:mount-var-nosuid`
- `host-probe:east:mount-var-tmp-separate`
- `host-probe:east:mount-var-tmp-nodev`
- `host-probe:east:mount-var-tmp-nosuid`
- `host-probe:east:mount-var-tmp-noexec`
- `host-probe:east:mount-var-log-separate`
- `host-probe:east:mount-var-log-nodev`
- `host-probe:east:mount-var-log-nosuid`
- `host-probe:east:mount-var-log-noexec`
- `host-probe:east:mount-var-log-audit-separate`
- `host-probe:east:mount-var-log-audit-nodev`
- `host-probe:east:mount-var-log-audit-nosuid`
- `host-probe:east:mount-var-log-audit-noexec`
- `host-probe:west:svc-autofs-not-in-use`
- `host-probe:west:svc-avahi-not-in-use`
- `host-probe:west:svc-dhcp-server-not-in-use`
- `host-probe:west:svc-dns-server-not-in-use`
- `host-probe:west:svc-dnsmasq-not-in-use`
- `host-probe:west:svc-ftp-server-not-in-use`
- `host-probe:west:svc-ldap-server-not-in-use`
- `host-probe:west:svc-mail-access-not-in-use`
- `host-probe:west:svc-nfs-server-not-in-use`
- `host-probe:west:svc-nis-server-not-in-use`
- `host-probe:west:svc-print-server-not-in-use`
- `host-probe:west:svc-rpcbind-not-in-use`
- `host-probe:west:svc-rsync-not-in-use`
- `host-probe:west:svc-samba-not-in-use`
- `host-probe:west:svc-snmp-not-in-use`
- `host-probe:west:svc-tftp-server-not-in-use`
- `host-probe:west:svc-web-proxy-not-in-use`
- `host-probe:west:svc-web-server-not-in-use`
- `host-probe:west:svc-xinetd-not-in-use`
- `host-probe:west:svc-x-server-not-in-use`
- `host-probe:west:mta-local-only`
- `host-probe:east:svc-autofs-not-in-use`
- `host-probe:east:svc-avahi-not-in-use`
- `host-probe:east:svc-dhcp-server-not-in-use`
- `host-probe:east:svc-dns-server-not-in-use`
- `host-probe:east:svc-dnsmasq-not-in-use`
- `host-probe:east:svc-ftp-server-not-in-use`
- `host-probe:east:svc-ldap-server-not-in-use`
- `host-probe:east:svc-mail-access-not-in-use`
- `host-probe:east:svc-nfs-server-not-in-use`
- `host-probe:east:svc-nis-server-not-in-use`
- `host-probe:east:svc-print-server-not-in-use`
- `host-probe:east:svc-rpcbind-not-in-use`
- `host-probe:east:svc-rsync-not-in-use`
- `host-probe:east:svc-samba-not-in-use`
- `host-probe:east:svc-snmp-not-in-use`
- `host-probe:east:svc-tftp-server-not-in-use`
- `host-probe:east:svc-web-proxy-not-in-use`
- `host-probe:east:svc-web-server-not-in-use`
- `host-probe:east:svc-xinetd-not-in-use`
- `host-probe:east:svc-x-server-not-in-use`
- `host-probe:east:mta-local-only`
- `host-probe:west:svc-bluetooth-not-in-use`
- `host-probe:east:svc-bluetooth-not-in-use`
- `host-probe:west:pkg-gdm-absent`
- `host-probe:east:pkg-gdm-absent`
- `host-probe:west:pkg-nis-client-absent`
- `host-probe:west:pkg-rsh-client-absent`
- `host-probe:west:pkg-talk-client-absent`
- `host-probe:west:pkg-telnet-client-absent`
- `host-probe:west:pkg-ldap-client-absent`
- `host-probe:west:pkg-ftp-client-absent`
- `host-probe:east:pkg-nis-client-absent`
- `host-probe:east:pkg-rsh-client-absent`
- `host-probe:east:pkg-talk-client-absent`
- `host-probe:east:pkg-telnet-client-absent`
- `host-probe:east:pkg-ldap-client-absent`
- `host-probe:east:pkg-ftp-client-absent`
- `host-probe:west:cron-active`
- `host-probe:west:perm-etc-crontab`
- `host-probe:west:perm-etc-cron-hourly`
- `host-probe:west:perm-etc-cron-daily`
- `host-probe:west:perm-etc-cron-weekly`
- `host-probe:west:perm-etc-cron-monthly`
- `host-probe:west:perm-etc-cron-d`
- `host-probe:west:cron-allow-restricted`
- `host-probe:west:at-restricted`
- `host-probe:east:cron-active`
- `host-probe:east:perm-etc-crontab`
- `host-probe:east:perm-etc-cron-hourly`
- `host-probe:east:perm-etc-cron-daily`
- `host-probe:east:perm-etc-cron-weekly`
- `host-probe:east:perm-etc-cron-monthly`
- `host-probe:east:perm-etc-cron-d`
- `host-probe:east:cron-allow-restricted`
- `host-probe:east:at-restricted`
- `host-probe:west:acct-passwd-shadowed`
- `host-probe:west:acct-shadow-no-empty`
- `host-probe:west:acct-groups-exist`
- `host-probe:west:acct-shadow-group-empty`
- `host-probe:west:acct-unique-uid`
- `host-probe:west:acct-unique-gid`
- `host-probe:west:acct-unique-user-name`
- `host-probe:west:acct-unique-group-name`
- `host-probe:east:acct-passwd-shadowed`
- `host-probe:east:acct-shadow-no-empty`
- `host-probe:east:acct-groups-exist`
- `host-probe:east:acct-shadow-group-empty`
- `host-probe:east:acct-unique-uid`
- `host-probe:east:acct-unique-gid`
- `host-probe:east:acct-unique-user-name`
- `host-probe:east:acct-unique-group-name`
- `host-probe:west:acct-root-only-uid0`
- `host-probe:west:acct-root-only-gid0`
- `host-probe:west:group-root-only-gid0`
- `host-probe:west:acct-system-no-shell`
- `host-probe:west:acct-noshell-locked`
- `host-probe:east:acct-root-only-uid0`
- `host-probe:east:acct-root-only-gid0`
- `host-probe:east:group-root-only-gid0`
- `host-probe:east:acct-system-no-shell`
- `host-probe:east:acct-noshell-locked`
- `host-probe:west:su-restricted`
- `host-probe:west:root-access-controlled`
- `host-probe:west:root-path`
- `host-probe:west:root-umask`
- `host-probe:west:default-umask`
- `host-probe:west:home-dirs`
- `host-probe:west:dot-files`
- `host-probe:east:su-restricted`
- `host-probe:east:root-access-controlled`
- `host-probe:east:root-path`
- `host-probe:east:root-umask`
- `host-probe:east:default-umask`
- `host-probe:east:home-dirs`
- `host-probe:east:dot-files`
- `host-agent:west:baseline-report`
- `host-agent:east:baseline-report`
- `host-probe:west:apparmor-installed`
- `host-probe:west:apparmor-profiles-enforced`
- `host-probe:east:apparmor-installed`
- `host-probe:east:apparmor-profiles-enforced`
- `host-probe:west:grub-password`
- `host-probe:west:perm-boot-grub-cfg`
- `host-probe:east:grub-password`
- `host-probe:east:perm-boot-grub-cfg`
- `host-probe:west:sudo-installed`
- `host-probe:west:sudo-use-pty`
- `host-probe:west:sudo-logfile`
- `host-probe:west:sudo-no-noauth`
- `host-probe:west:sudo-timeout`
- `host-probe:east:sudo-installed`
- `host-probe:east:sudo-use-pty`
- `host-probe:east:sudo-logfile`
- `host-probe:east:sudo-no-noauth`
- `host-probe:east:sudo-timeout`
- `host-probe:west:pw-max-days`
- `host-probe:west:pw-warn-age`
- `host-probe:west:pw-hash-algo`
- `host-probe:west:pw-inactive`
- `host-probe:west:pw-change-past`
- `host-probe:east:pw-max-days`
- `host-probe:east:pw-warn-age`
- `host-probe:east:pw-hash-algo`
- `host-probe:east:pw-inactive`
- `host-probe:east:pw-change-past`
- `host-probe:west:shell-tmout`
- `host-probe:east:shell-tmout`
- `host-probe:west:banner-motd`
- `host-probe:west:banner-issue`
- `host-probe:west:banner-issue-net`
- `host-probe:west:perm-etc-motd`
- `host-probe:west:perm-etc-issue`
- `host-probe:west:perm-etc-issue-net`
- `host-probe:east:banner-motd`
- `host-probe:east:banner-issue`
- `host-probe:east:banner-issue-net`
- `host-probe:east:perm-etc-motd`
- `host-probe:east:perm-etc-issue`
- `host-probe:east:perm-etc-issue-net`
- `host-probe:west:radio-wireless-down`
- `host-probe:east:radio-wireless-down`
- `host-probe:west:aide-installed`
- `host-probe:west:aide-timer`
- `host-probe:east:aide-installed`
- `host-probe:east:aide-timer`
- `host-probe:west:logging-single-system`
- `host-probe:west:rsyslog-active`
- `host-probe:west:journald-forward-syslog`
- `host-probe:west:rsyslog-filecreatemode`
- `host-probe:west:rsyslog-no-listen`
- `host-probe:east:logging-single-system`
- `host-probe:east:rsyslog-active`
- `host-probe:east:journald-forward-syslog`
- `host-probe:east:rsyslog-filecreatemode`
- `host-probe:east:rsyslog-no-listen`
- `host-probe:west:journald-file-access`
- `host-probe:west:journald-rotation`
- `host-probe:west:logrotate-active`
- `host-probe:west:var-log-perms`
- `host-probe:east:journald-file-access`
- `host-probe:east:journald-rotation`
- `host-probe:east:logrotate-active`
- `host-probe:east:var-log-perms`
- `host-probe:west:pkg-pam-modules`
- `host-probe:west:pkg-pam-pwquality`
- `host-probe:west:pam-runtime-version`
- `host-probe:west:pam-unix-enabled`
- `host-probe:west:pam-faillock-enabled`
- `host-probe:west:pam-pwquality-enabled`
- `host-probe:west:pam-pwhistory-enabled`
- `host-probe:west:pam-faillock-deny`
- `host-probe:west:pam-faillock-unlock`
- `host-probe:west:pam-pwq-difok`
- `host-probe:west:pam-pwq-minlen`
- `host-probe:west:pam-pwq-maxrepeat`
- `host-probe:west:pam-pwq-maxsequence`
- `host-probe:west:pam-pwq-dictcheck`
- `host-probe:west:pam-pwq-enforcing`
- `host-probe:west:pam-pwq-enforce-root`
- `host-probe:west:pam-pwhistory-remember`
- `host-probe:west:pam-pwhistory-enforce-root`
- `host-probe:west:pam-pwhistory-authtok`
- `host-probe:west:pam-unix-no-nullok`
- `host-probe:west:pam-unix-no-remember`
- `host-probe:west:pam-unix-strong-hash`
- `host-probe:west:pam-unix-authtok`
- `host-probe:east:pkg-pam-modules`
- `host-probe:east:pkg-pam-pwquality`
- `host-probe:east:pam-runtime-version`
- `host-probe:east:pam-unix-enabled`
- `host-probe:east:pam-faillock-enabled`
- `host-probe:east:pam-pwquality-enabled`
- `host-probe:east:pam-pwhistory-enabled`
- `host-probe:east:pam-faillock-deny`
- `host-probe:east:pam-faillock-unlock`
- `host-probe:east:pam-pwq-difok`
- `host-probe:east:pam-pwq-minlen`
- `host-probe:east:pam-pwq-maxrepeat`
- `host-probe:east:pam-pwq-maxsequence`
- `host-probe:east:pam-pwq-dictcheck`
- `host-probe:east:pam-pwq-enforcing`
- `host-probe:east:pam-pwq-enforce-root`
- `host-probe:east:pam-pwhistory-remember`
- `host-probe:east:pam-pwhistory-enforce-root`
- `host-probe:east:pam-pwhistory-authtok`
- `host-probe:east:pam-unix-no-nullok`
- `host-probe:east:pam-unix-no-remember`
- `host-probe:east:pam-unix-strong-hash`
- `host-probe:east:pam-unix-authtok`
- `host-sysctl:west:net.ipv4.conf.all.rp_filter>=1`
- `host-sysctl:west:net.ipv4.conf.all.accept_redirects=0`
- `host-sysctl:west:net.ipv6.conf.all.accept_redirects=0`
- `host-sysctl:west:net.ipv4.conf.all.send_redirects=0`
- `host-sysctl:west:net.ipv4.tcp_syncookies=1`
- `host-sysctl:east:net.ipv4.conf.all.rp_filter>=1`
- `host-sysctl:east:net.ipv4.conf.all.accept_redirects=0`
- `host-sysctl:east:net.ipv6.conf.all.accept_redirects=0`
- `host-sysctl:east:net.ipv4.conf.all.send_redirects=0`
- `host-sysctl:east:net.ipv4.tcp_syncookies=1`
- `host-sysctl:west:net.ipv4.ip_forward=0`
- `host-sysctl:west:net.ipv6.conf.all.forwarding=0`
- `host-sysctl:west:net.ipv4.icmp_ignore_bogus_error_responses=1`
- `host-sysctl:west:net.ipv4.icmp_echo_ignore_broadcasts=1`
- `host-sysctl:west:net.ipv4.conf.all.secure_redirects=0`
- `host-sysctl:west:net.ipv4.conf.all.accept_source_route=0`
- `host-sysctl:west:net.ipv6.conf.all.accept_source_route=0`
- `host-sysctl:west:net.ipv4.conf.all.log_martians=1`
- `host-sysctl:west:net.ipv6.conf.all.accept_ra=0`
- `host-sysctl:east:net.ipv4.ip_forward=0`
- `host-sysctl:east:net.ipv6.conf.all.forwarding=0`
- `host-sysctl:east:net.ipv4.icmp_ignore_bogus_error_responses=1`
- `host-sysctl:east:net.ipv4.icmp_echo_ignore_broadcasts=1`
- `host-sysctl:east:net.ipv4.conf.all.secure_redirects=0`
- `host-sysctl:east:net.ipv4.conf.all.accept_source_route=0`
- `host-sysctl:east:net.ipv6.conf.all.accept_source_route=0`
- `host-sysctl:east:net.ipv4.conf.all.log_martians=1`
- `host-sysctl:east:net.ipv6.conf.all.accept_ra=0`
- `host-sshd:west:passwordauthentication=no`
- `host-sshd:west:kbdinteractiveauthentication=no`
- `host-sshd:west:permitemptypasswords=no`
- `host-sshd:west:permitrootlogin=no`
- `host-sshd:west:x11forwarding=no`
- `host-sshd:west:maxauthtries<=4`
- `host-sshd:west:clientaliveinterval>=1`
- `host-sshd:west:clientaliveinterval<=300`
- `host-sshd:east:passwordauthentication=no`
- `host-sshd:east:kbdinteractiveauthentication=no`
- `host-sshd:east:permitemptypasswords=no`
- `host-sshd:east:permitrootlogin=no`
- `host-sshd:east:x11forwarding=no`
- `host-sshd:east:maxauthtries<=4`
- `host-sshd:east:clientaliveinterval>=1`
- `host-sshd:east:clientaliveinterval<=300`
- `host-sshd:west:hostbasedauthentication=no`
- `host-sshd:west:ignorerhosts=yes`
- `host-sshd:west:logingracetime<=60`
- `host-sshd:west:loglevel=INFO`
- `host-sshd:west:maxsessions<=10`
- `host-sshd:west:permituserenvironment=no`
- `host-sshd:west:usepam=yes`
- `host-sshd:west:disableforwarding=yes`
- `host-sshd:west:gssapiauthentication=no`
- `host-sshd:east:hostbasedauthentication=no`
- `host-sshd:east:ignorerhosts=yes`
- `host-sshd:east:logingracetime<=60`
- `host-sshd:east:loglevel=INFO`
- `host-sshd:east:maxsessions<=10`
- `host-sshd:east:permituserenvironment=no`
- `host-sshd:east:usepam=yes`
- `host-sshd:east:disableforwarding=yes`
- `host-sshd:east:gssapiauthentication=no`
- `host-probe:west:sshd-access`
- `host-probe:west:sshd-banner`
- `host-probe:west:sshd-ciphers`
- `host-probe:west:sshd-kex`
- `host-probe:west:sshd-macs`
- `host-probe:west:sshd-maxstartups`
- `host-probe:east:sshd-access`
- `host-probe:east:sshd-banner`
- `host-probe:east:sshd-ciphers`
- `host-probe:east:sshd-kex`
- `host-probe:east:sshd-macs`
- `host-probe:east:sshd-maxstartups`
- `gate-script:scripts/check-compose-hardening.sh`
- `host-probe:west:firewall-inbound-deny`
- `host-probe:west:firewall-single-utility`
- `host-probe:east:firewall-inbound-deny`
- `host-probe:east:firewall-single-utility`
- `host-probe:west:auditd-running`
- `host-probe:east:auditd-running`
- `host-probe:west:journald-active`
- `host-probe:east:journald-active`
- `host-probe:west:unattended-upgrades-enabled`
- `host-probe:east:unattended-upgrades-enabled`
- `host-probe:west:ntp-synchronized`
- `host-probe:east:ntp-synchronized`
- `host-probe:west:time-single-daemon`
- `host-probe:west:chrony-sources`
- `host-probe:west:chrony-user`
- `host-probe:west:chrony-active`
- `host-probe:east:time-single-daemon`
- `host-probe:east:chrony-sources`
- `host-probe:east:chrony-user`
- `host-probe:east:chrony-active`
- `gate-script:scripts/compliance/check-edge-tls.sh`
- `gate-script:scripts/compliance/check-service-auth.sh`
- `host-probe:west:db-backup-recent`
- `gate-script:scripts/check-crypto-inventory.sh`
- `gate-script:scripts/check-pq-kex.sh`
- `host-probe:west:sshd-kex-pq`
- `host-probe:east:sshd-kex-pq`
- `gate-script:scripts/check-evidence-pack.sh`
- `attestation:compliance/attestations/security-policy-review.yaml`
- `attestation:compliance/attestations/ir-plan-review.yaml`
- `attestation:compliance/attestations/access-review.yaml`
- `slo:wotan`
- `slo:dashboard`
- `slo:well`
- `slo:well-backups`
- `slo:overlay`
- `slo:log-shipping`
- `slo:kev-feed`
- `slo:ci-develop`

<!-- END GENERATED FINDINGS -->
