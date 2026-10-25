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
As of 2026-10-25T06:47:49Z: **155 open** (9 untriaged, 5 high, 12 medium, 129 low), 62 accepted, 0 overdue; zero claimable: **no** (155 open, 3 never observed).

| Severity | Finding | Host | Class | Step | Lockout | Due | Observed | Plan |
|---|---|---|---|---|---|---|---|---|
| UNTRIAGED | `git-signatures:develop` |  |  |  |  |  | 30 of 50 most recent commits not signed with a good signature | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `github-job:GPL Boundary Check/GPL/AGPL Boundary Verification` |  |  |  |  |  | https://github.com/unheaded/unheaded/actions/runs/40011818705/job/120613847420 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `github-job:Security Scan (Daily + PR)/Go Security Scan (gosec)` |  |  |  |  |  | https://github.com/unheaded/unheaded/actions/runs/40011818743/job/120613847177 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `github-job:Security Scan (Daily + PR)/Go Vulnerability Check` |  |  |  |  |  | https://github.com/unheaded/unheaded/actions/runs/40011818743/job/120613847650 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `github-job:Static Analysis/Gates can actually fail` |  |  |  |  |  | https://github.com/unheaded/unheaded/actions/runs/40011818719/job/120613847705 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `github-job:Static Analysis/Python (ruff + bandit)` |  |  |  |  |  | https://github.com/unheaded/unheaded/actions/runs/40011818719/job/120613847217 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `github-job:Static Analysis/Shell (shellcheck)` |  |  |  |  |  | https://github.com/unheaded/unheaded/actions/runs/40011818719/job/120613847615 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `host-agent:east:baseline-report` | east |  |  |  |  | agent ran baseline c9f21f8df380, the repo's is 12c6a543c995 | add an entry to compliance/findings/register.yaml |
| UNTRIAGED | `host-agent:west:baseline-report` | west |  |  |  |  | agent ran baseline c9f21f8df380, the repo's is 12c6a543c995 | add an entry to compliance/findings/register.yaml |
| high | `FND-001` | west | app | 1 | none | 2026-10-30 | Alerts reach nobody. Alertmanager routes everything to a receiver named null, and the monitoring stack is not running on west. | email receiver (Stevie 2026-09-30; PagerDuty or similar later); run the monitoring stack; Watchdog heartbeat cross-watched by west and east. Email receiver BUILT 2026-10-19: scripts/monitoring/render-alertmanager.sh renders it from /etc/unheaded/alertmanager.env (relay, from, to), password from a file outside the repo; critical repeats hourly and silences its own warning; tested end to end against an SMTP sink (CI). 2026-10-20: Prometheus had no alerting block, so no alert ever reached Alertmanager; fixed (d5e39b31) and the same test now proves a firing rule arrives. The monitoring stack can now start on west (its subnet overlapped kind/netbox) and needs GRAFANA_ADMIN_PASSWORD in monitoring/.env. 2026-10-20 at Stevie's word: monitoring/.env written, the stack runs on west (5/5 healthy), every deployed host target up, alerts reach Alertmanager (only true ones firing). What is left is the SMTP relay choice and its env and password files (the receiver is still null). 2026-10-20 (Stevie: "deviation in slo and sla should be the only alerts - no noise", "I might read 5"): ADR-100 built. Only SLO burn-rate alerts notify (observability/alerting/policy.yaml, 7 SLOs, urgency page/ticket/none); alert-mailer is the only sender, capped at 6 a day, tickets in one 09:00 digest, running on west in dry-run. Left: a week of dry-run outbox review, then the Gmail app password (wiki/Alerting.md) and mode send **decision:** SMTP relay (Stevie leaning to the bellis.tech Google Workspace account) and when to leave dry-run |
| high | `FND-011` |  | package | 3 | none | 2026-11-18 | Two Linux kernel vulnerabilities on CISA's KEV list (exploited in the wild) are unpatched on west and east, and Ubuntu has no fixed kernel yet: CVE-2026-53266 (ebtables SNAT out-of-bounds write, KEV 2026-09-18) and CVE-2026-53362 (privilege escalation via the IPv6 stack, KEV 2026-08-27). Both hosts run 6.17.0-41-generic | Install the fixed kernel as soon as Ubuntu publishes it (unattended-upgrades or apt), then reboot; re-run the verify commands. Until then, for CVE-2026-53266: nothing here uses ebtables, so /etc/modprobe.d/unheaded-cis.conf can gain `install ebtables /bin/false`, `install ebtable_nat /bin/false`, `install ebt_snat /bin/false` (no module to unload today). CVE-2026-53362 has no published mitigation short of the fixed kernel **decision:** the modprobe stopgap on both hosts (host change); whether west keeps unprivileged user namespaces open for LXD (turning the restriction back on may break LXD's unprivileged containers: test first) |
| high | `host-probe:east:firewall-inbound-deny` | east | firewall | 6 | high | 2026-10-30 | east: INPUT policy IPv4 ACCEPT, IPv6 ACCEPT; nft inet unheaded input absent (required DROP, DROP or nft policy drop) | deploy/nftables/east.nft (nft -c checked on east 2026-09-30): default-deny input; ssh from anything on the P2P link, everything from west. Apply with scripts/host/nft-apply-confirm.sh apply deploy/nftables/east.nft (rollback armed before apply, 120s), confirm, then enable deploy/systemd/unheaded-nft.service for boot. Physical access to east exists (Stevie 2026-09-30) |
| high | `host-probe:west:firewall-inbound-deny` | west | firewall | 6 | high | 2026-10-30 | west: INPUT policy IPv4 ACCEPT, IPv6 ACCEPT; nft inet unheaded input absent (required DROP, DROP or nft policy drop) | deploy/nftables/west.nft (nft -c checked on west 2026-09-30), after east has run clean: everything from east (P2P, wg0); LXD DHCP/DNS; Docker bridges to 9100/9110/9323 and 16666-26666; from the home LAN only ssh 7375 and haproxy 80/443; from the ISP IPv6 prefix nothing but ssh over link-local. Same commit-confirm procedure **decision:** anything else the LAN must reach on west (lxd 8443 remote, dashboards 20000/20001, the apt repo on 18888) |
| high | `gate-script:scripts/compliance/check-service-auth.sh` |  | app | 7 | none | 2026-10-30 | exit 1 | token distribution design first (clients, dashboard, kanban), then AUTH_ENABLED per service, one at a time **decision:** mode settled - Stevie 2026-09-30 wants several (SSO/OIDC, JWT, API key), selectable per deployment; where keys and the IdP live is proposed in ADR-103 (2026-10-23, 4 open questions for Stevie) |
| medium | `FND-014` | west | app | 1 | none | 2027-01-21 | The Well's role passwords were written in plaintext to the Postgres server log (log_statement = 'ddl' logs ALTER ROLE ... PASSWORD) and kept in pg_stat_statements, by db/init.sh and scripts/well/set-role-passwords.sh. Fixed for future password sets (586212e5); copies set since 2026-10-15 probably remain on the live Well. | With the Well up, at Stevie's yes: SELECT pg_stat_statements_reset(); clear or rotate the old Postgres logs that carry ALTER ROLE ... PASSWORD lines; rotate the role passwords with scripts/well/set-role-passwords.sh (fixed: it now sets them with logging off). The suites test-backup-restore and test-config check the fix. |
| medium | `host-probe:east:aide-installed` | east | package | 3 | none | 2026-12-30 | east: aide not installed | apt install aide aide-common; aideinit (minutes of disk I/O; run off-hours); exclude /var/lib/docker, LXD storage and build trees (target/, ~/go) from the database or every build alerts |
| medium | `host-probe:west:aide-installed` | west | package | 3 | none | 2026-12-30 | west: aide not installed | apt install aide aide-common; aideinit (minutes of disk I/O; run off-hours); exclude /var/lib/docker, LXD storage and build trees (target/, ~/go) from the database or every build alerts |
| medium | `host-sshd:east:kbdinteractiveauthentication=no` | east | sshd | 5 | high | 2026-12-29 | east: sshd kbdinteractiveauthentication yes (required =no) | accepted until 2026-12-30; KbdInteractiveAuthentication no, in the same change as PasswordAuthentication no |
| medium | `host-sshd:east:passwordauthentication=no` | east | sshd | 5 | high | 2026-12-29 | east: sshd passwordauthentication yes (required =no) | accepted until 2026-12-30; prove key login for govan from west in a second session, keep a session open, then disable; sshd -t before reload |
| medium | `host-sshd:east:permitrootlogin=no` | east | sshd | 5 | low | 2026-12-29 | east: sshd permitrootlogin without-password (required =no) | accepted until 2026-12-30; root has no authorized keys on east (checked 2026-09-30), so nothing logs in as root by key; PermitRootLogin no |
| medium | `host-sshd:west:kbdinteractiveauthentication=no` | west | sshd | 5 | high | 2026-12-29 | west: sshd kbdinteractiveauthentication yes (required =no) | accepted until 2026-12-30; West has no authorized keys for govan, so keyboard-interactive (PAM password) is the only remote login today. Add Stevie's public key to ~govan/.ssh/authorized_keys, prove a key login, then KbdInteractiveAuthentication no. Stevie has physical access (2026-09-30) |
| medium | `host-sshd:west:permitrootlogin=no` | west | sshd | 5 | low | 2026-12-29 | west: sshd permitrootlogin without-password (required =no) | accepted until 2026-12-30; root has no authorized keys on west (checked 2026-09-30); scripts/bare-metal/validate-cross-host.sh ssh's as root and cannot work today anyway; PermitRootLogin no |
| medium | `FND-010` |  | service | 7 | none | 2027-01-16 | Units that still start as root: the three Doom units (BPF/XDP/netns, binaries under /home/govan) and the Well's backup/drill/offsite jobs (docker exec into the Well, i.e. the Docker socket) | Well jobs: PostgreSQL 16 client+server binaries on the host (PGDG), backup over TLS as well_backup from its own account with a systemd credential, drill as an unprivileged throwaway cluster in a private dir; waits for Stevie's Ubuntu 26.04 upgrade (he does OS-level changes). Baseline: DONE, installed on east then west 2026-10-22 at Stevie's yes: runs as unheaded-baseline with CAP_DAC_READ_SEARCH + CAP_NET_ADMIN, root-only facts from one ExecStartPre=+ line; its 219 verdicts match the central pull on both hosts. Doom: unheaded-doom with CAP_BPF CAP_PERFMON CAP_NET_ADMIN CAP_SYS_ADMIN, artifacts installed out of /home **decision:** PGDG apt repo + postgresql-16 packages on west, after the OS upgrade |
| medium | `FND-015` | west | app | 7 | none | 2027-01-21 | The Well's TLS key exchange is classical P-256: PostgreSQL 16's ssl_ecdh_curve takes one curve, so no client can negotiate a hybrid ML-KEM group with it (proved in throwaway containers; PG 18 with ssl_groups negotiates X25519MLKEM768 with every client we run) | E1: runbooks/data/well-pg18-upgrade.yaml (dump/restore into a new volume with restore-drill.sh --into-volume, PG 16 volume kept as the rollback, db/postgresql.pg18.conf.d/10-pq-groups.conf with ssl_groups=X25519MLKEM768:X25519:prime256v1). Clients need no change (libpq 16/18 and lib/pq offer the hybrid) **decision:** E1, when to take the Well to PostgreSQL 18 (downtime, data-bearing major upgrade) |
| medium | `FND-017` |  | app | 7 | none | 2027-01-21 | Wotan and the internal service links run without TLS in the compose stack (no -enable-tls), so their traffic is plaintext, post-quantum or not | The ADR-034 / ADR-103 sprint (certificates, distribution, service auth). The code is ready: Wotan gRPC, wotan-client, pkg/transport/mtls and pkg/mesh are hybrid-only TLS 1.3 the day TLS is on (S-QUANTUM-ARMOR Phase 5) **decision:** when to turn TLS on for internal links (accepted as plaintext 2026-09-30 with the edge finding) |
| medium | `gate-script:scripts/compliance/check-edge-tls.sh` |  | app | 7 | none | 2026-12-29 | exit 1 | accepted until 2026-12-29; Decided (Stevie 2026-09-30): nothing until an edge goes internet-facing, then Let's Encrypt. Not a flag flip. The dashboard and kanban routers carry no TLS and websecure has no entrypoint TLS, so the edge effectively serves plain HTTP only; a redirect alone would 404 both. Order: pick a certificate source, enable TLS on websecure (entrypoint http.tls), a file-provider tls.options.default minVersion VersionTLS13, move ping off the web entrypoint (the healthcheck probes :80/ping), then redirect web to websecure, then drop api.insecure |
| low | `host-journal:east:apply-journal` | east | app | 2 | none | 2027-03-30 | no journal: no fix has been applied by unheaded-remediate (sysctl-cis, apport-off, kmod-cis, cron-perms, banner-cis, apparmor-utils, auditd, sudo-cis, sshd-safe, auditd-rules, journald-caps, time-p2p wanted) | As west: the lab fixes were applied by remediate.sh without a journal. plan each fix with unheaded-remediate, then apply through it so each is journaled. COMPLIANCE-WELL steps 81 and 93 **decision:** running unheaded-remediate on the host (plan, then apply of fixes already in place) |
| low | `host-journal:west:apply-journal` | west | app | 2 | none | 2027-03-30 | no journal: no fix has been applied by unheaded-remediate (sysctl-cis, apport-off, kmod-cis, cron-perms, banner-cis, apparmor-utils, auditd, sudo-cis, journald-caps, time-p2p wanted) | The eight lab fixes were applied 2026-10-01 by remediate.sh, which kept backups but no journal. Run `sudo unheaded-remediate plan <fix>` for each (read-only; expect "already as wanted"), then apply each through unheaded-remediate so the journal records it (no file changes when already applied). COMPLIANCE-WELL steps 81 and 93 **decision:** running unheaded-remediate on the host (plan, then apply of fixes already in place) |
| low | `host-probe:east:kmod-usb-storage-disabled` | east | kmod | 2 | none | 2027-03-29 | east: usb-storage can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/usb/storage/usb-storage.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install usb-storage /bin/false` and `blacklist usb-storage` (one file for all seven modules); takes effect for new loads at once, nothing to unload today **decision:** does anyone plug USB storage into east (Stevie has physical access); if so, accept on that host |
| low | `host-probe:east:mount-dev-shm-noexec` | east | mount | 2 | none | 2027-03-29 | east: /dev/shm (on /dev/shm) mounted without noexec: rw,nosuid,nodev,inode64 | add noexec to /dev/shm (fstab `tmpfs /dev/shm tmpfs defaults,nodev,nosuid,noexec 0 0`); `mount -o remount /dev/shm` |
| low | `host-probe:east:mount-home-nodev` | east | mount | 2 | none | 2027-03-29 | east: /home (on /) mounted without nodev: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:east:mount-home-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /home (on /) mounted without nosuid: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:east:mount-tmp-noexec` | east | mount | 2 | none | 2027-03-29 | east: /tmp (on /tmp) mounted without noexec: rw,nosuid,nodev,nr_inodes=1048576,inode64 | add noexec to the /tmp tmpfs mount (systemd tmp.mount drop-in Options=...,noexec); remount |
| low | `host-probe:east:mount-var-log-audit-nodev` | east | mount | 2 | none | 2027-03-29 | east: /var/log/audit (on /var) mounted without nodev: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:east:mount-var-log-audit-noexec` | east | mount | 2 | none | 2027-03-29 | east: /var/log/audit (on /var) mounted without noexec: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:east:mount-var-log-audit-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /var/log/audit (on /var) mounted without nosuid: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:east:mount-var-log-nodev` | east | mount | 2 | none | 2027-03-29 | east: /var/log (on /var) mounted without nodev: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:east:mount-var-log-noexec` | east | mount | 2 | none | 2027-03-29 | east: /var/log (on /var) mounted without noexec: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:east:mount-var-log-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /var/log (on /var) mounted without nosuid: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:east:mount-var-nodev` | east | mount | 2 | low | 2027-03-29 | east: /var (on /var) mounted without nodev: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:east:mount-var-nosuid` | east | mount | 2 | low | 2027-03-29 | east: /var (on /var) mounted without nosuid: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:east:mount-var-tmp-nodev` | east | mount | 2 | none | 2027-03-29 | east: /var/tmp (on /var) mounted without nodev: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:east:mount-var-tmp-noexec` | east | mount | 2 | none | 2027-03-29 | east: /var/tmp (on /var) mounted without noexec: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:east:mount-var-tmp-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /var/tmp (on /var) mounted without nosuid: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:east:pkg-ftp-client-absent` | east | package | 2 | none | 2027-03-29 | east: installed: ftp tnftp | apt purge ftp tnftp (check nothing scripted uses them: grep the repo and crontabs first) |
| low | `host-probe:east:pkg-telnet-client-absent` | east | package | 2 | none | 2027-03-29 | east: installed: inetutils-telnet telnet | apt purge inetutils-telnet telnet (nc or openssl s_client cover port checks) |
| low | `host-probe:east:var-log-perms` | east | fileperm | 2 | none | 2027-04-11 | east: 63 files looser than allowed: /var/log/alternatives.log 0644 (max 0640), /var/log/alternatives.log.1 0644 (max 0640), /var/log/alternatives.log.2.gz 0644 (max 0640), /var/log/alternatives.log.3.gz 0644 (max 0640), /var/log/alternatives.log.4.gz 0644 (max 0640), and 58 more | 48 files at Ubuntu's 0644 defaults (dpkg, apt, alternatives, installer, unattended-upgrades, landscape, sysstat): chmod o-r, and fix the create mode in the rotation that recreates each |
| low | `host-probe:west:kmod-usb-storage-disabled` | west | kmod | 2 | none | 2027-03-29 | west: usb-storage can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/usb/storage/usb-storage.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install usb-storage /bin/false` and `blacklist usb-storage` (one file for all seven modules); takes effect for new loads at once, nothing to unload today **decision:** does anyone plug USB storage into west (Stevie has physical access); if so, accept on that host |
| low | `host-probe:west:mount-dev-shm-noexec` | west | mount | 2 | none | 2027-03-29 | west: /dev/shm (on /dev/shm) mounted without noexec: rw,nosuid,nodev,inode64 | add noexec to /dev/shm (fstab `tmpfs /dev/shm tmpfs defaults,nodev,nosuid,noexec 0 0`); `mount -o remount /dev/shm` |
| low | `host-probe:west:mount-home-nodev` | west | mount | 2 | none | 2027-03-29 | west: /home (on /) mounted without nodev: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:west:mount-home-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /home (on /) mounted without nosuid: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:west:mount-tmp-noexec` | west | mount | 2 | low | 2027-03-29 | west: /tmp (on /tmp) mounted without noexec: rw,nosuid,nodev,nr_inodes=1048576,inode64 | Go builds and tests execute from /tmp on west (the dev box); set GOTMPDIR (and TMPDIR for cargo/pip builds) to a directory under /var/tmp or ~, then add noexec to the /tmp tmpfs **decision:** is west's /tmp used to run build output (Go, cargo, test binaries) outside what GOTMPDIR covers |
| low | `host-probe:west:mount-var-log-audit-nodev` | west | mount | 2 | none | 2027-03-29 | west: /var/log/audit (on /var) mounted without nodev: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:west:mount-var-log-audit-noexec` | west | mount | 2 | none | 2027-03-29 | west: /var/log/audit (on /var) mounted without noexec: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:west:mount-var-log-audit-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /var/log/audit (on /var) mounted without nosuid: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:west:mount-var-log-nodev` | west | mount | 2 | none | 2027-03-29 | west: /var/log (on /var) mounted without nodev: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:west:mount-var-log-noexec` | west | mount | 2 | none | 2027-03-29 | west: /var/log (on /var) mounted without noexec: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:west:mount-var-log-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /var/log (on /var) mounted without nosuid: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:west:mount-var-nodev` | west | mount | 2 | low | 2027-03-29 | west: /var (on /var) mounted without nodev: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:west:mount-var-nosuid` | west | mount | 2 | low | 2027-03-29 | west: /var (on /var) mounted without nosuid: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:west:mount-var-tmp-nodev` | west | mount | 2 | none | 2027-03-29 | west: /var/tmp (on /var) mounted without nodev: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:west:mount-var-tmp-noexec` | west | mount | 2 | none | 2027-03-29 | west: /var/tmp (on /var) mounted without noexec: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:west:mount-var-tmp-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /var/tmp (on /var) mounted without nosuid: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:west:pkg-ftp-client-absent` | west | package | 2 | none | 2027-03-29 | west: installed: ftp tnftp | apt purge ftp tnftp (check nothing scripted uses them: grep the repo and crontabs first) |
| low | `host-probe:west:pkg-telnet-client-absent` | west | package | 2 | none | 2027-03-29 | west: installed: inetutils-telnet telnet | apt purge inetutils-telnet telnet (nc or openssl s_client cover port checks) |
| low | `host-probe:west:svc-x-server-not-in-use` | west | package | 2 | none | 2027-03-29 | west: installed: xserver-common | CIS Level 2. xserver-common is installed and the default target is graphical, but no display manager runs; if west needs no local desktop, set multi-user.target and purge the X server packages **decision:** does west need its local graphical desktop |
| low | `host-probe:west:var-log-perms` | west | fileperm | 2 | none | 2027-04-11 | west: 220 files looser than allowed: /var/log/alternatives.log 0644 (max 0640), /var/log/alternatives.log.1 0644 (max 0640), /var/log/alternatives.log.2.gz 0644 (max 0640), /var/log/alternatives.log.3.gz 0644 (max 0640), /var/log/alternatives.log.4.gz 0644 (max 0640), and 215 more | Done 2026-10-13 (Stevie: yes): the two world-writable SMI logs (0666, in 0777 directories without the sticky bit, made by the ROCm installer, empty since February) are now 0640 in 0755 directories; undo in /var/lib/unheaded/remediate/20261013T133915Z-smi-log-perms. A ROCm reinstall may recreate them 0777/0666. The other ~208 are Ubuntu's 0644 defaults (dpkg, apt, alternatives, installer, sysstat): chmod o-r, and logrotate's create mode or the package's own rotation, else they come back on the next rotation |
| low | `host-sysctl:west:net.ipv6.conf.all.accept_ra=0` | west | sysctl | 2 | low | 2027-03-29 | west: net.ipv6.conf.all.accept_ra = 1 (required =0) | find out what does SLAAC on west before touching this; accept_ra=0 on the wrong layer drops west's IPv6. If IPv6 from the ISP is wanted, this becomes an acceptance with the firewall as the compensating control **decision:** does west need ISP IPv6 at all (see FND-004) |
| low | `FND-013` | west | package | 3 | none | 2027-04-17 | PyTorch carries two published advisories: the pinned torch==2.10.0 (raft/requirements.txt, sbom/python-requirements.txt) and, worse, the torch actually installed on west, 2.6.0 (+rocm6.1 system-wide, +cpu in ~/tmp/gemma4-venv): CVE-2025-3000 (memory corruption in torch.jit.script, fixed in 2.13.0) and CVE-2026-4538 (deserialization in the pt2 loader, no fixed release) | Move the training environment and the pins to a current torch (2.14.1 on PyPI 2026-10-19) with the matching ROCm wheel, run a short forge/RAFT training smoke on the GPU, then regenerate sbom/python-requirements.txt from that environment rather than by hand. Until PYSEC-2026-139 has a fix, load only .pt2 files built here **decision:** when to take the GPU stack to a current torch (a ROCm upgrade and a training run, not a pin edit) |
| low | `host-probe:east:aide-timer` | east | package | 3 | none | 2027-03-30 | east: no AIDE timer | systemctl enable --now dailyaidecheck.timer (shipped by aide-common) once the database exists; its report needs a destination (FND-001 email) |
| low | `host-probe:east:sudo-logfile` | east | sudo | 3 | none | 2027-03-29 | east: no Defaults logfile= in sudoers | accepted until 2026-12-30; /etc/sudoers.d/00-unheaded (0440): Defaults logfile="/var/log/sudo.log"; visudo -cf before installing |
| low | `host-probe:west:aide-timer` | west | package | 3 | none | 2027-03-30 | west: no AIDE timer | systemctl enable --now dailyaidecheck.timer (shipped by aide-common) once the database exists; its report needs a destination (FND-001 email) |
| low | `host-probe:west:sudo-logfile` | west | sudo | 3 | none | 2027-03-29 | west: no Defaults logfile= in sudoers | accepted until 2026-12-30; /etc/sudoers.d/00-unheaded (0440): Defaults logfile="/var/log/sudo.log"; visudo -cf before installing |
| low | `host-probe:east:default-umask` | east | shell | 4 | none | 2027-04-11 | east: no default umask set (the system default is 022) | same as west; east has fewer shared-file workflows **decision:** as west |
| low | `host-probe:east:shell-tmout` | east | shell | 4 | none | 2027-03-29 | east: TMOUT -1 (want 1-900) | accepted until 2026-12-30; /etc/profile.d/00-unheaded-tmout.sh: readonly TMOUT=900; export TMOUT. Long interactive jobs belong in tmux or systemd-run, which TMOUT does not end |
| low | `host-probe:east:sshd-banner` | east | sshd | 4 | none | 2027-03-29 | east: no Banner | accepted until 2026-12-30; Banner /etc/issue.net with an authorised-use notice and no OS or version details (CIS 1.6.x covers the file's content) |
| low | `host-probe:west:default-umask` | west | shell | 4 | none | 2027-04-11 | west: no default umask set (the system default is 022) | UMASK 027 in /etc/login.defs (pam_umask reads it). Every new file a user creates is then not world-readable; check the dev workflows that share files between govan and the service users (docker bind mounts, ~/tmp/unheaded read by services) before applying **decision:** umask 027 for interactive users on a dev host |
| low | `host-probe:west:dot-files` | west | fileperm | 4 | none | 2027-04-11 | west: /home/govan/.gitconfig 0664 (group or other writable), /home/govan/.wget-hsts 0664 (group or other writable), /home/govan/.zshrc 0664 (group or other writable) | chmod g-w /home/govan/.gitconfig /home/govan/.wget-hsts /home/govan/.zshrc (0664 from an earlier umask 002) |
| low | `host-probe:west:perm-sshd-config` | west | fileperm | 4 | none | 2027-03-29 | west: /etc/ssh/sshd_config mode 644 (max 600); /etc/ssh/sshd_config.d/99-unheaded.conf mode 644 (max 600) | accepted until 2026-12-30; chmod 600 /etc/ssh/sshd_config /etc/ssh/sshd_config.d/99-unheaded.conf (and any drop-in created later, including 00-unheaded.conf) |
| low | `host-probe:west:root-path` | west | fileperm | 4 | none | 2027-04-11 | west: root PATH: /root/.vector/bin owned by UNKNOWN | /root/.vector/bin is owned by uid 1001, which has no account: the Vector tarball was extracted as root with its archive owner kept. Empty, and /root is 0700, so no one else can reach it. chown -R root:root /root/.vector, or remove it and /root/.vector/bin from root's PATH (Vector runs in compose) |
| low | `host-probe:west:shell-tmout` | west | shell | 4 | none | 2027-03-29 | west: TMOUT -1 (want 1-900) | accepted until 2026-12-30; /etc/profile.d/00-unheaded-tmout.sh: readonly TMOUT=900; export TMOUT. Long interactive jobs belong in tmux or systemd-run, which TMOUT does not end |
| low | `host-probe:west:sshd-banner` | west | sshd | 4 | none | 2027-03-29 | west: no Banner | accepted until 2026-12-30; Banner /etc/issue.net with an authorised-use notice and no OS or version details (CIS 1.6.x covers the file's content) |
| low | `host-probe:west:sshd-macs` | west | sshd | 4 | low | 2027-03-29 | west: MACs not on the strong list: umac-64-etm@openssh.com, umac-128-etm@openssh.com, hmac-sha1-etm@openssh.com, umac-64@openssh.com, umac-128@openssh.com, hmac-sha1 | accepted until 2026-12-30; MACs hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com,hmac-sha2-512,hmac-sha2-256 in 00-unheaded.conf; every current OpenSSH client supports these; test a second session before closing the first |
| low | `host-probe:west:sshd-maxstartups` | west | sshd | 4 | none | 2027-03-29 | west: MaxStartups 10:30:100 (want 10:30:60 or stricter) | accepted until 2026-12-30; MaxStartups 10:30:60 in 00-unheaded.conf |
| low | `host-sshd:east:disableforwarding=yes` | east | sshd | 4 | low | 2027-03-29 | east: sshd disableforwarding no (required =yes) | accepted until 2026-12-29; first find out whether anything uses ssh -L/-R/-A to east; if so, accept (Level 1 does not require it) |
| low | `host-sshd:west:disableforwarding=yes` | west | sshd | 4 | low | 2027-03-29 | west: sshd disableforwarding no (required =yes) | accepted until 2026-12-29; first find out whether Stevie or any script uses ssh -L/-R/-A to west; if so, accept (Level 1 does not require it) |
| low | `host-sshd:west:logingracetime<=60` | west | sshd | 4 | none | 2027-03-29 | west: sshd logingracetime 120 (required <=60) | accepted until 2026-12-30; LoginGraceTime 60 in the 00-unheaded.conf drop-in (CIS 5.1.13) |
| low | `host-probe:east:grub-password` | east | boot | 5 | high | 2027-03-29 | east: grub superusers set: false, password_pbkdf2: false | accepted until 2026-12-30; grub-mkpasswd-pbkdf2, a superuser in /etc/grub.d/40_custom, --unrestricted on the default menu entries so unattended boots still work, update-grub; the password goes in the operator's password manager. Without --unrestricted every boot waits for the password |
| low | `host-probe:east:pam-faillock-deny` | east | account | 5 | high | 2027-03-30 | east: pam_faillock.so is not enabled, so deny has no effect | accepted until 2026-12-30; deny = 4 (or fewer) in /etc/security/faillock.conf once pam_faillock is enabled |
| low | `host-probe:east:pam-faillock-enabled` | east | account | 5 | high | 2027-03-30 | east: pam_faillock.so missing from: auth, account | accepted until 2026-12-30; pam-auth-update profile for pam_faillock (preauth/authfail in common-auth, account line); test sudo in a second root shell first |
| low | `host-probe:east:pam-faillock-unlock` | east | account | 5 | high | 2027-03-30 | east: pam_faillock.so is not enabled, so unlock_time has no effect | accepted until 2026-12-30; unlock_time = 900 in /etc/security/faillock.conf (with pam_faillock enabled) |
| low | `host-probe:east:pam-pwhistory-authtok` | east | account | 5 | low | 2027-03-30 | east: pam_pwhistory without use_authtok (or not enabled) | accepted until 2026-12-30; use_authtok on the pam_pwhistory line |
| low | `host-probe:east:pam-pwhistory-enabled` | east | account | 5 | low | 2027-03-30 | east: pam_pwhistory.so missing from: password | accepted until 2026-12-30; pam_pwhistory line in common-password (remember=24 enforce_for_root use_authtok), via a pam-auth-update profile |
| low | `host-probe:east:pam-pwhistory-enforce-root` | east | account | 5 | low | 2027-03-30 | east: pam_pwhistory.so is not enabled, so enforce_for_root has no effect | accepted until 2026-12-30; enforce_for_root on the pam_pwhistory line |
| low | `host-probe:east:pam-pwhistory-remember` | east | account | 5 | low | 2027-03-30 | east: pam_pwhistory.so is not enabled, so remember has no effect | accepted until 2026-12-30; remember=24 on the pam_pwhistory line or in /etc/security/pwhistory.conf |
| low | `host-probe:east:pam-pwq-dictcheck` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so dictcheck has no effect | accepted until 2026-12-30; dictcheck defaults to 1; passes once pam_pwquality is enabled |
| low | `host-probe:east:pam-pwq-difok` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so difok has no effect | accepted until 2026-12-30; difok = 2 in /etc/security/pwquality.conf |
| low | `host-probe:east:pam-pwq-enforce-root` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so enforce_for_root has no effect | accepted until 2026-12-30; enforce_for_root in /etc/security/pwquality.conf |
| low | `host-probe:east:pam-pwq-enforcing` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so enforcing has no effect | accepted until 2026-12-30; enforcing defaults to 1; passes once pam_pwquality is enabled |
| low | `host-probe:east:pam-pwq-maxrepeat` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so maxrepeat has no effect | accepted until 2026-12-30; maxrepeat = 3 in /etc/security/pwquality.conf |
| low | `host-probe:east:pam-pwq-maxsequence` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so maxsequence has no effect | accepted until 2026-12-30; maxsequence = 3 in /etc/security/pwquality.conf |
| low | `host-probe:east:pam-pwq-minlen` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so is not enabled, so minlen has no effect | accepted until 2026-12-30; minlen = 14 in /etc/security/pwquality.conf |
| low | `host-probe:east:pam-pwquality-enabled` | east | account | 5 | low | 2027-03-30 | east: pam_pwquality.so missing from: password | accepted until 2026-12-30; comes with libpam-pwquality via pam-auth-update |
| low | `host-probe:east:pam-unix-authtok` | east | account | 5 | high | 2027-03-30 | east: password pam_unix: no use_authtok | accepted until 2026-12-30; use_authtok on the password pam_unix line (set automatically once pam_pwquality precedes it) |
| low | `host-probe:east:pam-unix-no-nullok` | east | account | 5 | high | 2027-03-30 | east: auth pam_unix: nullok (empty passwords accepted) | accepted until 2026-12-30; remove nullok from pam_unix through a local pam-auth-update profile (hand edits to common-auth are overwritten by pam-auth-update); no account has an empty password today (7.2.2 passes) |
| low | `host-probe:east:pkg-pam-pwquality` | east | package | 5 | low | 2027-03-30 | east: not installed: libpam-pwquality | accepted until 2026-12-30; apt install libpam-pwquality (pam-auth-update adds pam_pwquality to common-password) |
| low | `host-probe:east:pw-inactive` | east | account | 5 | none | 2027-03-29 | east: "-1" (want INACTIVE 0-45) | accepted until 2026-12-30; useradd -D -f 45 and chage --inactive 45 on accounts with a password |
| low | `host-probe:east:pw-max-days` | east | account | 5 | low | 2027-03-29 | east: "99999" (want PASS_MAX_DAYS 1-365) | accepted until 2026-12-30; PASS_MAX_DAYS 365 in /etc/login.defs and chage -M 365 on accounts with a password; with key-only ssh the password is used for sudo, so expiry means a yearly password change, not a lockout |
| low | `host-probe:east:sshd-access` | east | sshd | 5 | high | 2027-03-29 | east: none of AllowUsers, AllowGroups, DenyUsers, DenyGroups is set | accepted until 2026-12-30; AllowGroups sshusers in 00-unheaded.conf after creating the group and adding govan; prove a second login before reloading. An empty or wrong group is a lockout |
| low | `host-probe:east:su-restricted` | east | account | 5 | high | 2027-04-11 | east: su is not restricted (no auth required pam_wheel.so in /etc/pam.d/su) | same as west **decision:** as west |
| low | `host-probe:west:grub-password` | west | boot | 5 | high | 2027-03-29 | west: grub superusers set: false, password_pbkdf2: false | accepted until 2026-12-30; grub-mkpasswd-pbkdf2, a superuser in /etc/grub.d/40_custom, --unrestricted on the default menu entries so unattended boots still work, update-grub; the password goes in the operator's password manager. Without --unrestricted every boot waits for the password |
| low | `host-probe:west:pam-faillock-deny` | west | account | 5 | high | 2027-03-30 | west: pam_faillock.so is not enabled, so deny has no effect | accepted until 2026-12-30; deny = 4 (or fewer) in /etc/security/faillock.conf once pam_faillock is enabled |
| low | `host-probe:west:pam-faillock-enabled` | west | account | 5 | high | 2027-03-30 | west: pam_faillock.so missing from: auth, account | accepted until 2026-12-30; pam-auth-update profile for pam_faillock (preauth/authfail in common-auth, account line); test sudo in a second root shell first |
| low | `host-probe:west:pam-faillock-unlock` | west | account | 5 | high | 2027-03-30 | west: pam_faillock.so is not enabled, so unlock_time has no effect | accepted until 2026-12-30; unlock_time = 900 in /etc/security/faillock.conf (with pam_faillock enabled) |
| low | `host-probe:west:pam-pwhistory-authtok` | west | account | 5 | low | 2027-03-30 | west: pam_pwhistory without use_authtok (or not enabled) | accepted until 2026-12-30; use_authtok on the pam_pwhistory line |
| low | `host-probe:west:pam-pwhistory-enabled` | west | account | 5 | low | 2027-03-30 | west: pam_pwhistory.so missing from: password | accepted until 2026-12-30; pam_pwhistory line in common-password (remember=24 enforce_for_root use_authtok), via a pam-auth-update profile |
| low | `host-probe:west:pam-pwhistory-enforce-root` | west | account | 5 | low | 2027-03-30 | west: pam_pwhistory.so is not enabled, so enforce_for_root has no effect | accepted until 2026-12-30; enforce_for_root on the pam_pwhistory line |
| low | `host-probe:west:pam-pwhistory-remember` | west | account | 5 | low | 2027-03-30 | west: pam_pwhistory.so is not enabled, so remember has no effect | accepted until 2026-12-30; remember=24 on the pam_pwhistory line or in /etc/security/pwhistory.conf |
| low | `host-probe:west:pam-pwq-dictcheck` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so dictcheck has no effect | accepted until 2026-12-30; dictcheck defaults to 1; passes once pam_pwquality is enabled |
| low | `host-probe:west:pam-pwq-difok` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so difok has no effect | accepted until 2026-12-30; difok = 2 in /etc/security/pwquality.conf |
| low | `host-probe:west:pam-pwq-enforce-root` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so enforce_for_root has no effect | accepted until 2026-12-30; enforce_for_root in /etc/security/pwquality.conf |
| low | `host-probe:west:pam-pwq-enforcing` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so enforcing has no effect | accepted until 2026-12-30; enforcing defaults to 1; passes once pam_pwquality is enabled |
| low | `host-probe:west:pam-pwq-maxrepeat` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so maxrepeat has no effect | accepted until 2026-12-30; maxrepeat = 3 in /etc/security/pwquality.conf |
| low | `host-probe:west:pam-pwq-maxsequence` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so maxsequence has no effect | accepted until 2026-12-30; maxsequence = 3 in /etc/security/pwquality.conf |
| low | `host-probe:west:pam-pwq-minlen` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so is not enabled, so minlen has no effect | accepted until 2026-12-30; minlen = 14 in /etc/security/pwquality.conf |
| low | `host-probe:west:pam-pwquality-enabled` | west | account | 5 | low | 2027-03-30 | west: pam_pwquality.so missing from: password | accepted until 2026-12-30; comes with libpam-pwquality via pam-auth-update |
| low | `host-probe:west:pam-unix-authtok` | west | account | 5 | high | 2027-03-30 | west: password pam_unix: no use_authtok | accepted until 2026-12-30; use_authtok on the password pam_unix line (set automatically once pam_pwquality precedes it) |
| low | `host-probe:west:pam-unix-no-nullok` | west | account | 5 | high | 2027-03-30 | west: auth pam_unix: nullok (empty passwords accepted) | accepted until 2026-12-30; remove nullok from pam_unix through a local pam-auth-update profile (hand edits to common-auth are overwritten by pam-auth-update); no account has an empty password today (7.2.2 passes) |
| low | `host-probe:west:pkg-pam-pwquality` | west | package | 5 | low | 2027-03-30 | west: not installed: libpam-pwquality | accepted until 2026-12-30; apt install libpam-pwquality (pam-auth-update adds pam_pwquality to common-password) |
| low | `host-probe:west:pw-inactive` | west | account | 5 | none | 2027-03-29 | west: "-1" (want INACTIVE 0-45) | accepted until 2026-12-30; useradd -D -f 45 and chage --inactive 45 on accounts with a password |
| low | `host-probe:west:pw-max-days` | west | account | 5 | low | 2027-03-29 | west: "99999" (want PASS_MAX_DAYS 1-365) | accepted until 2026-12-30; PASS_MAX_DAYS 365 in /etc/login.defs and chage -M 365 on accounts with a password; with key-only ssh the password is used for sudo, so expiry means a yearly password change, not a lockout |
| low | `host-probe:west:sshd-access` | west | sshd | 5 | high | 2027-03-29 | west: none of AllowUsers, AllowGroups, DenyUsers, DenyGroups is set | accepted until 2026-12-30; AllowGroups sshusers in 00-unheaded.conf after creating the group and adding govan; prove a second login before reloading. An empty or wrong group is a lockout |
| low | `host-probe:west:su-restricted` | west | account | 5 | high | 2027-04-11 | west: su is not restricted (no auth required pam_wheel.so in /etc/pam.d/su) | groupadd sugroup (no members); in /etc/pam.d/su: auth required pam_wheel.so use_uid group=sugroup. Root via sudo is unaffected; su to another user then needs root. Keep a root shell open, test su from a second session **decision:** whether su is used at all on the lab hosts (Stevie 2026-10-01 kept SSH and password settings as they are; su was not part of that) |
| low | `host-probe:east:firewall-single-utility` | east | firewall | 6 | high | 2027-04-11 | east: no firewall utility in use | ufw.service is enabled and active on east but `ufw status` is inactive, so no firewall utility has rules live. Closes with the firewall decision (UH-FW-01, ADR-098 step 6): load deploy/nftables/east.nft through scripts/host/nft-apply-confirm.sh and enable unheaded-nft.service, then disable ufw.service so one utility owns it **decision:** the firewall rollout (same as host-iptables:east default deny) |
| low | `host-probe:west:svc-web-server-not-in-use` | west | service | 6 | none | 2027-03-29 | west: installed: nginx; in use: nginx.service enabled/active | nginx serves the local apt repository (/var/lib/apt-repo on 18888) that east installs from. Needed: accept once the firewall (step 6) limits 18888 to east, with that as the compensating control |
| low | `host-sysctl:west:net.ipv4.ip_forward=0` | west | sysctl | 6 | high | 2027-03-29 | west: net.ipv4.ip_forward = 1 (required =0) | never flip it (it would cut every container and the overlay); once the firewall has a default-deny FORWARD policy (step 6), accept it with that as the compensating control |
| low | `host-sysctl:west:net.ipv6.conf.all.forwarding=0` | west | sysctl | 6 | high | 2027-03-29 | west: net.ipv6.conf.all.forwarding = 1 (required =0) | as net.ipv4.ip_forward on west; accept with a default-deny FORWARD policy for IPv6 as the compensating control |
| low | `FND-009` |  | container | 7 | none | 2027-04-16 | 22 services in the compose files other than docker-compose.yml are not least-privilege (was 75 on 2026-10-18): the firewall VMs (opnsense, frr, ipfire, bird: privileged or root), shield and unheaded-daemon in host-a (the D11 host-network decision), WireGuard twice (root by necessity, bounded), and 13 host-a services whose images exist nowhere in the repo | Done 2026-10-20 for 53 services (monitoring, telemetry, sophia-eye, loadbalancers, vllm, host-a/host-b known services): own pkg/uids number, read_only + measured tmpfs, cap_drop ALL, no-new-privileges, volume-init chown; promtail off the Docker socket everywhere (journal/var/log via the adm group); each stack that can run was run on west. Left: WireGuard needs root (EPERM otherwise, measured), so it stays listed with cap_drop ALL + NET_ADMIN and the four s6 init caps, read-only; the firewall VMs need a decision on whether they stay containers at all; host-a's shield/unheaded-daemon wait on D11; the 13 imageless host-a services get uids only if D11 keeps the stack **decision:** whether the firewall VMs stay containers, and D11 (host-a network mode; keep or retire the per-host stacks) |
| low | `FND-016` |  | app | 7 | low | 2027-04-21 | The WireGuard pre-shared key, the post-quantum part of the west-east overlay, has not changed since setup (2026-03-25): one static PSK, never rotated | E3: first live rotation with scripts/wireguard/rotate-psk.sh --apply (dry run by default; waits 200 s for a handshake and rolls back), then enable the weekly timer from deploy/systemd/ (docs/security/pq-host-escalations.md) **decision:** E3, the first live rotation and enabling the weekly timer |
| low | `FND-018` |  | app | 7 | none | 2027-04-21 | No production evidence build key exists, so no Yggdrasil evidence pack can be signed for real; the sign/verify path works with throwaway keys only | E4: an ML-DSA-65 seed held under ADR-043 #4 custody (HSM/TPM/sealed, or strict ACL with an audit log), separate from the Heimdall and drift keys; publish build.pubkey; a quarterly rotation ceremony **decision:** E4, where the build key lives, who holds it, how build.pubkey is published |
| low | `FND-019` |  | app | 7 | none | 2027-04-21 | Heimdall's signed drift alerts reach no one: no node has a drift key, Wotan's drift. prefix trusts no key (fails closed), and no unit runs Heimdall with --wotan-grpc | E5 (plan Step 246; docs/battle-plans/progress/S-QUANTUM-ARMOR-drift-progress.md): enrol the keys, add heimdall to topics.auto_approve, ship and enable a Heimdall unit **decision:** E5, key custody per node and running a root-reading daemon on the hosts |
| low | `FND-020` |  | app | 7 | none | 2027-04-21 | Wotan cluster replication drops message signatures: replication.proto carries no signature, key or signed_at, so a signed config. or drift. event replayed from a peer node reaches subscribers unsigned | additive replication.proto fields (signature, public_key, algorithm, signed_at_unix_ms) copied onto the ring-buffer message, as TopicEvent field 12 was |
| low | `host-probe:east:apparmor-profiles-enforced` | east | mac | 7 | none | 2027-03-29 | east: 187 profiles loaded, not enforce/complain: 74 unconfined | The unconfined profiles are Ubuntu's flags=(unconfined) profiles that grant user namespaces to specific programs (browsers, container tools); moving them to enforce or complain changes those programs' behaviour. Review the list (aa-status), then accept with that reason or move individual profiles to complain **decision:** accept Ubuntu's unconfined userns profiles against CIS 1.3.1.3, or move them to complain one by one |
| low | `host-probe:east:mount-home-separate` | east | mount | 7 | low | 2027-03-29 | east: /home is on /, not its own mount | Level 2: its own filesystem. east has no free disk space or LVM: accept, or shrink /var **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:mount-var-log-audit-separate` | east | mount | 7 | low | 2027-03-29 | east: /var/log/audit is on /var, not its own mount | Level 2: its own filesystem. east has no free disk space or LVM: accept, or shrink /var. /var/log/audit exists only once auditd is installed (step 3) **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:mount-var-log-separate` | east | mount | 7 | low | 2027-03-29 | east: /var/log is on /var, not its own mount | Level 2: its own filesystem. east has no free disk space or LVM: accept, or shrink /var **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:mount-var-tmp-separate` | east | mount | 7 | none | 2027-03-29 | east: /var/tmp is on /var, not its own mount | Level 2: a real filesystem for /var/tmp. east has no free disk space or LVM: accept, or shrink /var **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:sshd-kex-pq` | east | sshd | 7 | high | 2027-04-21 | east: classical KEX allowed: curve25519-sha256, curve25519-sha256@libssh.org, ecdh-sha2-nistp256, ecdh-sha2-nistp384, ecdh-sha2-nistp521 | E2, as west (east first) **decision:** E2 |
| low | `host-probe:west:apparmor-profiles-enforced` | west | mac | 7 | none | 2027-03-29 | west: 331 profiles loaded, not enforce/complain: 165 unconfined | The unconfined profiles are Ubuntu's flags=(unconfined) profiles that grant user namespaces to specific programs (browsers, container tools); moving them to enforce or complain changes those programs' behaviour. Review the list (aa-status), then accept with that reason or move individual profiles to complain **decision:** accept Ubuntu's unconfined userns profiles against CIS 1.3.1.3, or move them to complain one by one |
| low | `host-probe:west:mount-home-separate` | west | mount | 7 | low | 2027-03-29 | west: /home is on /, not its own mount | Level 2: its own filesystem. west: an LV from ubuntu-vg (588G free), copy data with services stopped, fstab, mount |
| low | `host-probe:west:mount-var-log-audit-separate` | west | mount | 7 | low | 2027-03-29 | west: /var/log/audit is on /var, not its own mount | Level 2: its own filesystem. west: an LV from ubuntu-vg (588G free), copy data with services stopped, fstab, mount. /var/log/audit exists only once auditd is installed (step 3) |
| low | `host-probe:west:mount-var-log-separate` | west | mount | 7 | low | 2027-03-29 | west: /var/log is on /var, not its own mount | Level 2: its own filesystem. west: an LV from ubuntu-vg (588G free), copy data with services stopped, fstab, mount |
| low | `host-probe:west:mount-var-tmp-separate` | west | mount | 7 | none | 2027-03-29 | west: /var/tmp is on /var, not its own mount | Level 2: a real filesystem for /var/tmp. west: an LV from ubuntu-vg (588G free); copy contents, fstab, mount |
| low | `host-probe:west:radio-wireless-down` | west | account | 7 | high | 2027-03-29 | west: offenders: wlp17s0 up | needed while west has no wired LAN: accept with that reason, or move west to a wired link and then disable the radio **decision:** is a wired LAN link for west possible, or accept wireless on west |
| low | `host-probe:west:sshd-kex-pq` | west | sshd | 7 | high | 2027-04-21 | west: classical KEX allowed: curve25519-sha256, curve25519-sha256@libssh.org, ecdh-sha2-nistp256, ecdh-sha2-nistp384, ecdh-sha2-nistp521 | E2 (docs/security/pq-host-escalations.md): KexAlgorithms mlkem768x25519-sha256,sntrup761x25519-sha512, sntrup761x25519-sha512@openssh.com in /etc/ssh/sshd_config.d, east first, second session kept open, sshd -t before reload, rollback from the .bak **decision:** E2, whether to make sshd hybrid-only (lockout risk for any client without ML-KEM or sntrup) |

Never observed (NOT_ASSESSED, not a pass):

- `attestation:compliance/attestations/security-policy-review.yaml`
- `attestation:compliance/attestations/ir-plan-review.yaml`
- `attestation:compliance/attestations/access-review.yaml`

<!-- END GENERATED FINDINGS -->
