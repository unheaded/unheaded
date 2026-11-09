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
- **The generator** is `go run ./cmd/compliance-evidence findings`. It joins catalog, evidence and register and prints the table; `-out var/compliance/findings.md` also writes it on the host. The table is collected data and is never committed (amendment 2026-11-09).

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
- `.gitleaksignore` and `docs/policy/gitleaks-history-baseline.txt` hold known findings, not cleared ones. The repository is public, so every value in history counts as burned, and each still in use is rotated at S-E1-LIVE (`docs/security/PRE-PUBLIC-BLOCKERS.md`).

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
4. West and east are directly connected over the P2P link on the same switch. **Later on 2026-09-30, Stevie:** the ISP's edge firewall drops unsolicited inbound traffic, so FND-004 was closed. That rested on his statement; it has not been tested from outside, and the entry's verify asked for that test. **Reopened 2026-11-06** at Stevie's word, until the test exists. Severities keep assuming LAN-only exposure.
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

**Amendment 2026-11-09 (Stevie: "should only be schema, collected data should live on host").** The generated table no longer lives here. The repository holds the catalog, the register's hand-written triage (`compliance/findings/register.yaml`) and the generator; the table, its counts and every observed value are built on the host: `go run ./cmd/compliance-evidence findings -out var/compliance/findings.md` (`var/` is gitignored). The tool refuses an output path inside the tree outside `var/`, and `cmd/compliance-evidence`'s `TestNoCollectedDataCommitted` fails if generated output reappears under `docs/`, `compliance/` or `wiki/`.

## Decisions 2026-11-03 (decision pack, round 2)

From `docs/battle-plans/DECISION-PACK-2026-11-03.md` (DP ids in parentheses).

- Q1 (Q5 of the enforcing draft) enforcing refuses installs only; drift pages a finding; starts are never refused (DP-10). Q2 default-deny rolls out east first in an attended window (with east's sshd on 7375), west after E1 is live (DP-22). Q3 LAN scope decided 2026-11-03. Q4 Ubuntu's `postgresql-client-16` now, PGDG at the 26.04 upgrade (DP-24). Q5 every internal link on TLS at E1 (DP-23). Q6 journal the 2026-10-01 fixes via `unheaded-remediate -record-existing`. Q7 blacklist `usb-storage` on both hosts. Q8 test `nodev,nosuid` on `/var` on east, then fstab on both. Q9 GOTMPDIR/TMPDIR under `/var/tmp`, then `noexec` on west's `/tmp`. Q10 purge X and `multi-user.target` on west. Q11 keep ISP IPv6 with the firewall as the compensating control. Q12 torch/ROCm upgrade and training smoke scheduled after E1. Q13 `UMASK 027` after a bind-mount check. Q14 restrict `su` with `pam_wheel`. Q15 (D11) the host-a/host-b stacks are retired. Q16 a command-scoped root-on-west key on east; the weekly PSK timer is enabled. Q17 accept the unconfined userns profiles with reason. Q18 accept east's missing separate mounts with reason. Q19 accept wireless on west; wired when a cable is there (DP-25). Journal caps: west 4G/500M, east 1G/200M (DP-09). east sshd: 7375, password allowed, fail2ban; `lxd-installer` removed (DP-11, DP-12). west sshd: `PermitRootLogin no` now; key-only once a laptop key exists (DP-15).

## Decisions 2026-11-04 (decision pack, round 3)

- DP-702 edge certificates come from orion's Let's Encrypt renew, copied down, under `lab.worldsoldestlobster.com` (back burner). DP-703 and DP-704: left as they are.
