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

## Open questions for Stevie

1. **Alarm receiver for pages:** self-hosted ntfy, email, or something else?
2. **Dead-man heartbeat:** west and east watching each other, or an outside service?
3. **Recovery if SSH or the firewall locks you out:** is there console or out-of-band access to east?
4. **Exposure:** is anything on west or east reachable from the internet, not just the LAN? Several severities depend on the answer.
5. **Enforcing strictness:** should enforcing ever refuse to *start* Unheaded services on a host that is out of baseline, or only refuse installs? The draft only refuses installs, because availability is also a control.

## Consequences

- There is one number, and it can only go down by fixing things or by visibly accepting risk with an expiry.
- Enforcing mode exists only behind a red-team pass and a soak period. Until then every host runs in audit mode, which is ADR-043's alerts-only rule with a register attached.
- New work: the agent, its `.deb`, Prometheus rules, alert routing and a LICH-012 campaign scoped to the agent.

## Register

Generated by `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md`. Do not edit by hand. Triage lives in `compliance/findings/register.yaml`.

<!-- BEGIN GENERATED FINDINGS -->
As of 2026-09-30T17:45:28Z: **24 open** (4 high, 12 medium, 8 low), 0 accepted, 0 overdue; zero claimable: **no** (24 open, 3 never observed).

| Severity | Finding | Host | Class | Step | Lockout | Due | Observed | Plan |
|---|---|---|---|---|---|---|---|---|
| high | `FND-001` | west | app | 1 | none | 2026-10-30 | Alerts reach nobody. Alertmanager routes everything to a receiver named null, and the monitoring stack is not running on west. | pick a receiver (open question 1) and a dead-man heartbeat (question 2); run the monitoring stack; add a Watchdog rule **decision:** receiver and heartbeat, ADR-098 questions 1 and 2 |
| high | `host-probe:east:firewall-inbound-deny` | east | firewall | 6 | high | 2026-10-30 | east: INPUT policy IPv4 ACCEPT, IPv6 ACCEPT (required DROP, DROP) | package-owned nft table inet unheaded, ssh allow before policy drop, commit-confirm (auto-rollback after 120s unless confirmed); east first **decision:** out-of-band access to east (ADR-098 question 3) before this lands |
| high | `host-probe:west:firewall-inbound-deny` | west | firewall | 6 | high | 2026-10-30 | west: INPUT policy IPv4 ACCEPT, IPv6 ACCEPT (required DROP, DROP) | the east nft table, after east has run clean; allow list from the port registry (pkg/ports) |
| high | `gate-script:scripts/compliance/check-service-auth.sh` |  | app | 7 | none | 2026-10-30 | exit 1 | token distribution design first (clients, dashboard, kanban), then AUTH_ENABLED per service, one at a time **decision:** auth mode (API key vs JWT) and where the keys live |
| medium | `github-job:Static Analysis/Gates can actually fail` |  | ci | 1 | none | 2026-12-29 | https://github.com/unheaded/unheaded/actions/runs/36708280882/job/109863668068 | 17/17 gates bite locally at HEAD; resolves when develop is pushed |
| medium | `github-job:Static Analysis/Rust (clippy)` |  | ci | 1 | none | 2026-12-29 | https://github.com/unheaded/unheaded/actions/runs/36708280882/job/109863667965 | fixed locally (57be0ee8, forge build skips HIP kernels in clippy); resolves when develop is pushed |
| medium | `host-probe:west:db-backup-recent` | west | backup | 1 | none | 2026-12-29 | west: newest database backup 2026-09-22 (required within 7 days) | take a backup now (scripts/db-backup.sh), then a systemd timer running it daily with retention **decision:** schedule and retention |
| medium | `host-probe:east:auditd-running` | east | service | 3 | none | 2026-12-29 | east: auditd inactive | install auditd; disk_full_action/admin_space_left_action never halt; no -e 2; east before west |
| medium | `host-probe:west:auditd-running` | west | service | 3 | none | 2026-12-29 | west: auditd inactive | install auditd; disk_full_action/admin_space_left_action never halt; no -e 2; check /var free space first |
| medium | `host-sshd:east:kbdinteractiveauthentication=no` | east | sshd | 5 | high | 2026-12-29 | east: sshd kbdinteractiveauthentication yes (required =no) | KbdInteractiveAuthentication no, in the same change as PasswordAuthentication no |
| medium | `host-sshd:east:passwordauthentication=no` | east | sshd | 5 | high | 2026-12-29 | east: sshd passwordauthentication yes (required =no) | prove key login for govan from west in a second session, keep a session open, then disable; sshd -t before reload |
| medium | `host-sshd:east:permitrootlogin=no` | east | sshd | 5 | low | 2026-12-29 | east: sshd permitrootlogin without-password (required =no) | first prove nothing logs in as root (root authorized_keys, repo grep for root@east); then PermitRootLogin no |
| medium | `host-sshd:west:kbdinteractiveauthentication=no` | west | sshd | 5 | high | 2026-12-29 | west: sshd kbdinteractiveauthentication yes (required =no) | KbdInteractiveAuthentication no in the drop-in after proving key login; west is Stevie's desk machine, so console access exists |
| medium | `host-sshd:west:permitrootlogin=no` | west | sshd | 5 | low | 2026-12-29 | west: sshd permitrootlogin without-password (required =no) | first prove nothing logs in as root on west; then PermitRootLogin no |
| medium | `FND-002` |  | container | 7 | none | 2026-12-29 | Seven third-party containers run without the least-privilege baseline (listed in docs/security/compose-hardening-baseline.txt) | one service at a time - read_only, cap_drop ALL plus the minimum, no-new-privileges, dedicated non-root user |
| medium | `gate-script:scripts/compliance/check-edge-tls.sh` |  | app | 7 | none | 2026-12-29 | exit 1 | Not a flag flip. The dashboard and kanban routers carry no TLS and websecure has no entrypoint TLS, so the edge effectively serves plain HTTP only; a redirect alone would 404 both. Order: pick a certificate source, enable TLS on websecure (entrypoint http.tls), a file-provider tls.options.default minVersion VersionTLS13, move ping off the web entrypoint (the healthcheck probes :80/ping), then redirect web to websecure, then drop api.insecure **decision:** certificate source for the edge (ACME, internal CA, or self-signed for LAN) |
| low | `host-sysctl:east:fs.suid_dumpable=0` | east | sysctl | 2 | none | 2027-03-29 | east: fs.suid_dumpable = 2 (required =0) | fs.suid_dumpable=0 in /etc/sysctl.d/99-unheaded.conf, sysctl --system |
| low | `host-sysctl:east:net.ipv4.conf.all.accept_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv4.conf.all.accept_redirects = 1 (required =0) | one sysctl.d file with the other east redirect settings; east routes only the P2P link, so redirects are not needed |
| low | `host-sysctl:east:net.ipv4.conf.all.send_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv4.conf.all.send_redirects = 1 (required =0) | one sysctl.d file with the other east redirect settings |
| low | `host-sysctl:east:net.ipv6.conf.all.accept_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv6.conf.all.accept_redirects = 1 (required =0) | one sysctl.d file with the other east redirect settings |
| low | `host-sysctl:west:fs.suid_dumpable=0` | west | sysctl | 2 | none | 2027-03-29 | west: fs.suid_dumpable = 2 (required =0) | fs.suid_dumpable=0 in /etc/sysctl.d/99-unheaded.conf, sysctl --system |
| low | `host-sshd:east:clientaliveinterval>=1` | east | sshd | 4 | none | 2027-03-29 | east: sshd clientaliveinterval 0 (required >=1) | ClientAliveInterval 300, ClientAliveCountMax 3 in the same drop-in |
| low | `host-sshd:east:maxauthtries<=4` | east | sshd | 4 | low | 2027-03-29 | east: sshd maxauthtries 6 (required <=4) | MaxAuthTries 4 in the same drop-in; an agent offering many keys can hit the limit, so use IdentitiesOnly client side |
| low | `host-sshd:east:x11forwarding=no` | east | sshd | 4 | none | 2027-03-29 | east: sshd x11forwarding yes (required =no) | drop-in /etc/ssh/sshd_config.d/00-unheaded.conf, sshd -t, reload |

Never observed (NOT_ASSESSED, not a pass):

- `attestation:compliance/attestations/security-policy-review.yaml`
- `attestation:compliance/attestations/ir-plan-review.yaml`
- `attestation:compliance/attestations/access-review.yaml`

<!-- END GENERATED FINDINGS -->
