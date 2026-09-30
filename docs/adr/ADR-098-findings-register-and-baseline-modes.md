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

### Answers (Stevie, 2026-09-30)

1. **Email** now, with PagerDuty or similar later. Still open: the SMTP relay. Credentials live in a file outside the repo.
2. Not answered. The draft default is west and east watching each other, with no third party.
3. **Physical access** to both hosts exists. That is the recovery path for the sshd and firewall steps.
4. West and east are directly connected over the P2P link on the same switch. West's public IPv6 address comes from the ISP router, so FND-004 stays open until the router's inbound IPv6 policy is checked.
5. Not answered yet; installs-only remains the draft.
6. **Edge TLS is deferred.** There is no internet-facing edge in develop mode; the plan is Let's Encrypt once one exists. The finding is accepted until 2026-12-29.
7. **Backups:** GitHub is enough for now, so the database backup finding is accepted. Unpushed commits on west are to be mirrored to east every 30 minutes (FND-005, low priority).
8. **Service auth:** several modes (SSO/OIDC, JWT, API key), selectable per deployment.
9. **SSH forwarding** may be used in develop, so CIS DisableForwarding (Level 2) is accepted on both hosts.

## Consequences

- There is one number, and it can only go down by fixing things or by visibly accepting risk with an expiry.
- Enforcing mode exists only behind a red-team pass and a soak period. Until then every host runs in audit mode, which is ADR-043's alerts-only rule with a register attached.
- New work: the agent, its `.deb`, Prometheus rules, alert routing and a LICH-012 campaign scoped to the agent.

## Register

Generated by `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md`. Do not edit by hand. Triage lives in `compliance/findings/register.yaml`.

<!-- BEGIN GENERATED FINDINGS -->
As of 2026-09-30T20:43:10Z: **93 open** (5 high, 12 medium, 76 low), 4 accepted, 0 overdue; zero claimable: **no** (93 open, 3 never observed).

| Severity | Finding | Host | Class | Step | Lockout | Due | Observed | Plan |
|---|---|---|---|---|---|---|---|---|
| high | `FND-001` | west | app | 1 | none | 2026-10-30 | Alerts reach nobody. Alertmanager routes everything to a receiver named null, and the monitoring stack is not running on west. | email receiver (Stevie 2026-09-30; PagerDuty or similar later); run the monitoring stack; Watchdog heartbeat cross-watched by west and east **decision:** SMTP relay for the email receiver (credentials from a file outside the repo) |
| high | `FND-004` | west | firewall | 1 | none | 2026-10-30 | West has a public, ISP-assigned IPv6 address (SLAAC) and ip6tables INPUT policy ACCEPT, with sshd, BGP and other services listening on [::]. Whether it can be reached from the internet depends on the home router dropping unsolicited inbound IPv6. Nobody has checked. | The west-east P2P link is not the question: west's public IPv6 address comes from the ISP router's advertisements on the home LAN. Check the router's IPv6 inbound firewall, or from a phone on cellular (not wifi) try west's global address on port 7375; until then treat west's firewall, sshd and service-auth findings as internet-facing **decision:** ADR-098 question 4 (exposure) |
| high | `host-probe:east:firewall-inbound-deny` | east | firewall | 6 | high | 2026-10-30 | east: INPUT policy IPv4 ACCEPT, IPv6 ACCEPT; nft inet unheaded input absent (required DROP, DROP or nft policy drop) | deploy/nftables/east.nft (nft -c checked on east 2026-09-30): default-deny input; ssh from anything on the P2P link, everything from west. Apply with scripts/host/nft-apply-confirm.sh apply deploy/nftables/east.nft (rollback armed before apply, 120s), confirm, then enable deploy/systemd/unheaded-nft.service for boot. Physical access to east exists (Stevie 2026-09-30) |
| high | `host-probe:west:firewall-inbound-deny` | west | firewall | 6 | high | 2026-10-30 | west: INPUT policy IPv4 ACCEPT, IPv6 ACCEPT; nft inet unheaded input absent (required DROP, DROP or nft policy drop) | deploy/nftables/west.nft (nft -c checked on west 2026-09-30), after east has run clean: everything from east (P2P, wg0); LXD DHCP/DNS; Docker bridges to 9100/9110/9323 and 16666-26666; from the home LAN only ssh 7375 and haproxy 80/443; from the ISP IPv6 prefix nothing but ssh over link-local. Same commit-confirm procedure **decision:** anything else the LAN must reach on west (lxd 8443 remote, dashboards 20000/20001, the apt repo on 18888) |
| high | `gate-script:scripts/compliance/check-service-auth.sh` |  | app | 7 | none | 2026-10-30 | exit 1 | token distribution design first (clients, dashboard, kanban), then AUTH_ENABLED per service, one at a time **decision:** mode settled - Stevie 2026-09-30 wants several (SSO/OIDC, JWT, API key), selectable per deployment; open is where keys and the IdP live |
| medium | `github-job:Static Analysis/Gates can actually fail` |  | ci | 1 | none | 2026-12-29 | https://github.com/unheaded/unheaded/actions/runs/36708280882/job/109863668068 | 17/17 gates bite locally at HEAD; resolves when develop is pushed |
| medium | `github-job:Static Analysis/Rust (clippy)` |  | ci | 1 | none | 2026-12-29 | https://github.com/unheaded/unheaded/actions/runs/36708280882/job/109863667965 | fixed locally (57be0ee8, forge build skips HIP kernels in clippy); resolves when develop is pushed |
| medium | `host-probe:west:db-backup-recent` | west | backup | 1 | none | 2026-12-29 | west: newest database backup 2026-09-22 (required within 7 days) | accepted until 2026-12-29; re-decide at expiry; a scheduled db-backup.sh with retention when space allows |
| medium | `host-probe:east:auditd-running` | east | service | 3 | none | 2026-12-29 | east: auditd inactive | install auditd; disk_full_action/admin_space_left_action never halt; no -e 2; east before west |
| medium | `host-probe:west:auditd-running` | west | service | 3 | none | 2026-12-29 | west: auditd inactive | install auditd; disk_full_action/admin_space_left_action never halt; no -e 2; check /var free space first |
| medium | `host-sshd:east:kbdinteractiveauthentication=no` | east | sshd | 5 | high | 2026-12-29 | east: sshd kbdinteractiveauthentication yes (required =no) | KbdInteractiveAuthentication no, in the same change as PasswordAuthentication no |
| medium | `host-sshd:east:passwordauthentication=no` | east | sshd | 5 | high | 2026-12-29 | east: sshd passwordauthentication yes (required =no) | prove key login for govan from west in a second session, keep a session open, then disable; sshd -t before reload |
| medium | `host-sshd:east:permitrootlogin=no` | east | sshd | 5 | low | 2026-12-29 | east: sshd permitrootlogin without-password (required =no) | root has no authorized keys on east (checked 2026-09-30), so nothing logs in as root by key; PermitRootLogin no |
| medium | `host-sshd:west:kbdinteractiveauthentication=no` | west | sshd | 5 | high | 2026-12-29 | west: sshd kbdinteractiveauthentication yes (required =no) | West has no authorized keys for govan, so keyboard-interactive (PAM password) is the only remote login today. Add Stevie's public key to ~govan/.ssh/authorized_keys, prove a key login, then KbdInteractiveAuthentication no. Stevie has physical access (2026-09-30) |
| medium | `host-sshd:west:permitrootlogin=no` | west | sshd | 5 | low | 2026-12-29 | west: sshd permitrootlogin without-password (required =no) | root has no authorized keys on west (checked 2026-09-30); scripts/bare-metal/validate-cross-host.sh ssh's as root and cannot work today anyway; PermitRootLogin no |
| medium | `FND-002` |  | container | 7 | none | 2026-12-29 | Seven third-party containers run without the least-privilege baseline (listed in docs/security/compose-hardening-baseline.txt) | one service at a time - read_only, cap_drop ALL plus the minimum, no-new-privileges, dedicated non-root user |
| medium | `gate-script:scripts/compliance/check-edge-tls.sh` |  | app | 7 | none | 2026-12-29 | exit 1 | accepted until 2026-12-29; Decided (Stevie 2026-09-30): nothing until an edge goes internet-facing, then Let's Encrypt. Not a flag flip. The dashboard and kanban routers carry no TLS and websecure has no entrypoint TLS, so the edge effectively serves plain HTTP only; a redirect alone would 404 both. Order: pick a certificate source, enable TLS on websecure (entrypoint http.tls), a file-provider tls.options.default minVersion VersionTLS13, move ping off the web entrypoint (the healthcheck probes :80/ping), then redirect web to websecure, then drop api.insecure |
| low | `FND-005` | west | backup | 1 | none | 2027-03-29 | Unpushed commits on west exist in one place only; a disk failure on west loses everything not yet pushed to GitHub | Stevie 2026-09-30 (low priority): every 30 minutes. `git push --mirror` to a bare repo on east over the P2P link rather than rsync of the tree: it copies commits atomically (a copy of .git taken mid-commit can be inconsistent) and skips build output. Working-tree changes that are not committed are not covered; an rsync of the tree without target/ could be added if wanted. |
| low | `host-probe:east:kmod-cramfs-disabled` | east | kmod | 2 | none | 2027-03-29 | east: cramfs can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/mtd/mtd.ko.zst  insmod /lib/modules/6.17.0-41-generic/kernel/fs/cramfs/cramfs.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install cramfs /bin/false` and `blacklist cramfs` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:east:kmod-freevxfs-disabled` | east | kmod | 2 | none | 2027-03-29 | east: freevxfs can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/fs/freevxfs/freevxfs.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install freevxfs /bin/false` and `blacklist freevxfs` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:east:kmod-hfs-disabled` | east | kmod | 2 | none | 2027-03-29 | east: hfs can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/fs/hfs/hfs.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install hfs /bin/false` and `blacklist hfs` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:east:kmod-hfsplus-disabled` | east | kmod | 2 | none | 2027-03-29 | east: hfsplus can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/fs/hfsplus/hfsplus.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install hfsplus /bin/false` and `blacklist hfsplus` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:east:kmod-jffs2-disabled` | east | kmod | 2 | none | 2027-03-29 | east: jffs2 can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/mtd/mtd.ko.zst  insmod /lib/modules/6.17.0-41-generic/kernel/fs/jffs2/jffs2.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install jffs2 /bin/false` and `blacklist jffs2` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:east:kmod-udf-disabled` | east | kmod | 2 | none | 2027-03-29 | east: udf can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/lib/crc/crc-itu-t.ko.zst  insmod /lib/modules/6.17.0-41-generic/kernel/fs/udf/udf.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install udf /bin/false` and `blacklist udf` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:east:kmod-usb-storage-disabled` | east | kmod | 2 | none | 2027-03-29 | east: usb-storage can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/usb/storage/usb-storage.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install usb-storage /bin/false` and `blacklist usb-storage` (one file for all seven modules); takes effect for new loads at once, nothing to unload today **decision:** does anyone plug USB storage into east (Stevie has physical access); if so, accept on that host |
| low | `host-probe:east:mount-dev-shm-noexec` | east | mount | 2 | none | 2027-03-29 | east: /dev/shm (on /dev/shm) mounted without noexec: rw,nosuid,nodev,inode64 | add noexec to /dev/shm (fstab `tmpfs /dev/shm tmpfs defaults,nodev,nosuid,noexec 0 0`); `mount -o remount /dev/shm` |
| low | `host-probe:east:mount-home-nodev` | east | mount | 2 | none | 2027-03-29 | east: /home (on /) mounted without nodev: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:east:mount-home-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /home (on /) mounted without nosuid: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:east:mount-tmp-noexec` | east | mount | 2 | none | 2027-03-29 | east: /tmp (on /tmp) mounted without noexec: rw,nosuid,nodev,nr_inodes=1048576,inode64 | add noexec to the /tmp tmpfs mount (systemd tmp.mount drop-in Options=...,noexec); remount |
| low | `host-probe:east:mount-var-log-audit-nodev` | east | mount | 2 | none | 2027-03-29 | east: /var/log/audit does not exist | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:east:mount-var-log-audit-noexec` | east | mount | 2 | none | 2027-03-29 | east: /var/log/audit does not exist | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:east:mount-var-log-audit-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /var/log/audit does not exist | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:east:mount-var-log-nodev` | east | mount | 2 | none | 2027-03-29 | east: /var/log (on /var) mounted without nodev: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:east:mount-var-log-noexec` | east | mount | 2 | none | 2027-03-29 | east: /var/log (on /var) mounted without noexec: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:east:mount-var-log-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /var/log (on /var) mounted without nosuid: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:east:mount-var-nodev` | east | mount | 2 | low | 2027-03-29 | east: /var (on /var) mounted without nodev: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:east:mount-var-nosuid` | east | mount | 2 | low | 2027-03-29 | east: /var (on /var) mounted without nosuid: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:east:mount-var-tmp-nodev` | east | mount | 2 | none | 2027-03-29 | east: /var/tmp (on /var) mounted without nodev: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:east:mount-var-tmp-noexec` | east | mount | 2 | none | 2027-03-29 | east: /var/tmp (on /var) mounted without noexec: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:east:mount-var-tmp-nosuid` | east | mount | 2 | none | 2027-03-29 | east: /var/tmp (on /var) mounted without nosuid: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:west:kmod-cramfs-disabled` | west | kmod | 2 | none | 2027-03-29 | west: cramfs can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/mtd/mtd.ko.zst  insmod /lib/modules/6.17.0-41-generic/kernel/fs/cramfs/cramfs.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install cramfs /bin/false` and `blacklist cramfs` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:west:kmod-freevxfs-disabled` | west | kmod | 2 | none | 2027-03-29 | west: freevxfs can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/fs/freevxfs/freevxfs.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install freevxfs /bin/false` and `blacklist freevxfs` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:west:kmod-hfs-disabled` | west | kmod | 2 | none | 2027-03-29 | west: hfs can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/fs/hfs/hfs.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install hfs /bin/false` and `blacklist hfs` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:west:kmod-hfsplus-disabled` | west | kmod | 2 | none | 2027-03-29 | west: hfsplus can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/fs/hfsplus/hfsplus.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install hfsplus /bin/false` and `blacklist hfsplus` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:west:kmod-jffs2-disabled` | west | kmod | 2 | none | 2027-03-29 | west: jffs2 can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/mtd/mtd.ko.zst  insmod /lib/modules/6.17.0-41-generic/kernel/fs/jffs2/jffs2.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install jffs2 /bin/false` and `blacklist jffs2` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:west:kmod-udf-disabled` | west | kmod | 2 | none | 2027-03-29 | west: udf can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/lib/crc/crc-itu-t.ko.zst  insmod /lib/modules/6.17.0-41-generic/kernel/fs/udf/udf.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install udf /bin/false` and `blacklist udf` (one file for all seven modules); takes effect for new loads at once, nothing to unload today |
| low | `host-probe:west:kmod-usb-storage-disabled` | west | kmod | 2 | none | 2027-03-29 | west: usb-storage can be loaded (modprobe: insmod /lib/modules/6.17.0-41-generic/kernel/drivers/usb/storage/usb-storage.ko.zst) | /etc/modprobe.d/unheaded-cis.conf with `install usb-storage /bin/false` and `blacklist usb-storage` (one file for all seven modules); takes effect for new loads at once, nothing to unload today **decision:** does anyone plug USB storage into west (Stevie has physical access); if so, accept on that host |
| low | `host-probe:west:mount-dev-shm-noexec` | west | mount | 2 | none | 2027-03-29 | west: /dev/shm (on /dev/shm) mounted without noexec: rw,nosuid,nodev,inode64 | add noexec to /dev/shm (fstab `tmpfs /dev/shm tmpfs defaults,nodev,nosuid,noexec 0 0`); `mount -o remount /dev/shm` |
| low | `host-probe:west:mount-home-nodev` | west | mount | 2 | none | 2027-03-29 | west: /home (on /) mounted without nodev: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:west:mount-home-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /home (on /) mounted without nosuid: rw,relatime | /home is on /, which cannot take nosuid (sudo) or nodev safely; resolves with the separate /home filesystem, mounted nodev,nosuid |
| low | `host-probe:west:mount-tmp-noexec` | west | mount | 2 | low | 2027-03-29 | west: /tmp (on /tmp) mounted without noexec: rw,nosuid,nodev,nr_inodes=1048576,inode64 | Go builds and tests execute from /tmp on west (the dev box); set GOTMPDIR (and TMPDIR for cargo/pip builds) to a directory under /var/tmp or ~, then add noexec to the /tmp tmpfs **decision:** is west's /tmp used to run build output (Go, cargo, test binaries) outside what GOTMPDIR covers |
| low | `host-probe:west:mount-var-log-audit-nodev` | west | mount | 2 | none | 2027-03-29 | west: /var/log/audit does not exist | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:west:mount-var-log-audit-noexec` | west | mount | 2 | none | 2027-03-29 | west: /var/log/audit does not exist | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:west:mount-var-log-audit-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /var/log/audit does not exist | resolves with the separate filesystem, mounted nodev,nosuid,noexec (after auditd, step 3) |
| low | `host-probe:west:mount-var-log-nodev` | west | mount | 2 | none | 2027-03-29 | west: /var/log (on /var) mounted without nodev: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:west:mount-var-log-noexec` | west | mount | 2 | none | 2027-03-29 | west: /var/log (on /var) mounted without noexec: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:west:mount-var-log-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /var/log (on /var) mounted without nosuid: rw,relatime | resolves with the separate filesystem, mounted nodev,nosuid,noexec |
| low | `host-probe:west:mount-var-nodev` | west | mount | 2 | low | 2027-03-29 | west: /var (on /var) mounted without nodev: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:west:mount-var-nosuid` | west | mount | 2 | low | 2027-03-29 | west: /var (on /var) mounted without nosuid: rw,relatime | /var holds Docker and LXD storage; nodev/nosuid on it may break containers that need device nodes or setuid binaries. Test on east first (remount -o nodev,nosuid live, run the stack), then fstab **decision:** test nodev/nosuid on /var with the container stack before adopting |
| low | `host-probe:west:mount-var-tmp-nodev` | west | mount | 2 | none | 2027-03-29 | west: /var/tmp (on /var) mounted without nodev: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:west:mount-var-tmp-noexec` | west | mount | 2 | none | 2027-03-29 | west: /var/tmp (on /var) mounted without noexec: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-probe:west:mount-var-tmp-nosuid` | west | mount | 2 | none | 2027-03-29 | west: /var/tmp (on /var) mounted without nosuid: rw,relatime | options via the /var/tmp filesystem once separate; until then /var's options govern (see /var entries) |
| low | `host-sysctl:east:fs.suid_dumpable=0` | east | sysctl | 2 | none | 2027-03-29 | east: fs.suid_dumpable = 2 (required =0) | apport is active and sets fs.suid_dumpable=2 at start, so a sysctl.d value alone is overwritten; disable apport (enabled=0 in /etc/default/apport, systemctl disable --now apport), then fs.suid_dumpable=0 in /etc/sysctl.d/99-unheaded.conf |
| low | `host-sysctl:east:net.ipv4.conf.all.accept_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv4.conf.all.accept_redirects = 1 (required =0) | one sysctl.d file with the other east redirect settings; east routes only the P2P link, so redirects are not needed |
| low | `host-sysctl:east:net.ipv4.conf.all.log_martians=1` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv4.conf.all.log_martians = 0 (required =1) | all and default log_martians=1 (CIS 3.3.9) |
| low | `host-sysctl:east:net.ipv4.conf.all.secure_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv4.conf.all.secure_redirects = 1 (required =0) | net.ipv4.conf.all.secure_redirects=0 and default.secure_redirects=0 in /etc/sysctl.d (CIS 3.3.6) |
| low | `host-sysctl:east:net.ipv4.conf.all.send_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv4.conf.all.send_redirects = 1 (required =0) | one sysctl.d file with the other east redirect settings |
| low | `host-sysctl:east:net.ipv6.conf.all.accept_ra=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv6.conf.all.accept_ra = 1 (required =0) | all and default accept_ra=0 (CIS 3.3.11) |
| low | `host-sysctl:east:net.ipv6.conf.all.accept_redirects=0` | east | sysctl | 2 | none | 2027-03-29 | east: net.ipv6.conf.all.accept_redirects = 1 (required =0) | one sysctl.d file with the other east redirect settings |
| low | `host-sysctl:west:fs.suid_dumpable=0` | west | sysctl | 2 | none | 2027-03-29 | west: fs.suid_dumpable = 2 (required =0) | apport is active and sets fs.suid_dumpable=2 at start, so a sysctl.d value alone is overwritten; disable apport (enabled=0 in /etc/default/apport, systemctl disable --now apport), then fs.suid_dumpable=0 in /etc/sysctl.d/99-unheaded.conf |
| low | `host-sysctl:west:net.ipv4.conf.all.log_martians=1` | west | sysctl | 2 | none | 2027-03-29 | west: net.ipv4.conf.all.log_martians = 0 (required =1) | all and default log_martians=1 (CIS 3.3.9); watch kernel log volume on west, which routes Docker and BGP traffic |
| low | `host-sysctl:west:net.ipv4.conf.all.secure_redirects=0` | west | sysctl | 2 | none | 2027-03-29 | west: net.ipv4.conf.all.secure_redirects = 1 (required =0) | net.ipv4.conf.all.secure_redirects=0 and default.secure_redirects=0 in /etc/sysctl.d (CIS 3.3.6) |
| low | `host-sysctl:west:net.ipv6.conf.all.accept_ra=0` | west | sysctl | 2 | low | 2027-03-29 | west: net.ipv6.conf.all.accept_ra = 1 (required =0) | find out what does SLAAC on west before touching this; accept_ra=0 on the wrong layer drops west's IPv6. If IPv6 from the ISP is wanted, this becomes an acceptance with the firewall as the compensating control **decision:** does west need ISP IPv6 at all (see FND-004) |
| low | `host-probe:east:perm-sshd-config` | east | fileperm | 4 | none | 2027-03-29 | east: /etc/ssh/sshd_config mode 644 (max 600) | chmod 600 /etc/ssh/sshd_config; the 00-unheaded.conf drop-in (step 4/5) is installed 0600 |
| low | `host-probe:west:perm-sshd-config` | west | fileperm | 4 | none | 2027-03-29 | west: /etc/ssh/sshd_config mode 644 (max 600); /etc/ssh/sshd_config.d/99-unheaded.conf mode 644 (max 600) | chmod 600 /etc/ssh/sshd_config /etc/ssh/sshd_config.d/99-unheaded.conf (and any drop-in created later, including 00-unheaded.conf) |
| low | `host-sshd:east:clientaliveinterval>=1` | east | sshd | 4 | none | 2027-03-29 | east: sshd clientaliveinterval 0 (required >=1) | ClientAliveInterval 300, ClientAliveCountMax 3 in the same drop-in |
| low | `host-sshd:east:disableforwarding=yes` | east | sshd | 4 | low | 2027-03-29 | east: sshd disableforwarding no (required =yes) | accepted until 2026-12-29; first find out whether anything uses ssh -L/-R/-A to east; if so, accept (Level 1 does not require it) |
| low | `host-sshd:east:logingracetime<=60` | east | sshd | 4 | none | 2027-03-29 | east: sshd logingracetime 120 (required <=60) | LoginGraceTime 60 in the 00-unheaded.conf drop-in (CIS 5.1.13) |
| low | `host-sshd:east:maxauthtries<=4` | east | sshd | 4 | low | 2027-03-29 | east: sshd maxauthtries 6 (required <=4) | MaxAuthTries 4 in the same drop-in; an agent offering many keys can hit the limit, so use IdentitiesOnly client side |
| low | `host-sshd:east:x11forwarding=no` | east | sshd | 4 | none | 2027-03-29 | east: sshd x11forwarding yes (required =no) | drop-in /etc/ssh/sshd_config.d/00-unheaded.conf, sshd -t, reload |
| low | `host-sshd:west:disableforwarding=yes` | west | sshd | 4 | low | 2027-03-29 | west: sshd disableforwarding no (required =yes) | accepted until 2026-12-29; first find out whether Stevie or any script uses ssh -L/-R/-A to west; if so, accept (Level 1 does not require it) |
| low | `host-sshd:west:logingracetime<=60` | west | sshd | 4 | none | 2027-03-29 | west: sshd logingracetime 120 (required <=60) | LoginGraceTime 60 in the 00-unheaded.conf drop-in (CIS 5.1.13) |
| low | `host-sysctl:west:net.ipv4.ip_forward=0` | west | sysctl | 6 | high | 2027-03-29 | west: net.ipv4.ip_forward = 1 (required =0) | never flip it (it would cut every container and the overlay); once the firewall has a default-deny FORWARD policy (step 6), accept it with that as the compensating control |
| low | `host-sysctl:west:net.ipv6.conf.all.forwarding=0` | west | sysctl | 6 | high | 2027-03-29 | west: net.ipv6.conf.all.forwarding = 1 (required =0) | as net.ipv4.ip_forward on west; accept with a default-deny FORWARD policy for IPv6 as the compensating control |
| low | `FND-003` | west | service | 7 | none | 2027-03-29 | 13 unit files in deploy/systemd deny syscalls that systemd silently ignores. `SystemCallFilter=~@privileged ~@resources ~@reboot ~@module ~@swap` accepts `~` only as the first character of the list, so the later groups are unknown names (systemd-analyze verify prints "System call ~@resources is not known, ignoring"). Only @privileged is denied. @reboot, @module and @swap are already outside the @system-service allow-list, so the real loss is @resources. huginn.service is installed and enabled on west with this line. | Repo side fixed 2026-09-30: all 13 units now read `SystemCallFilter=~@privileged @resources @reboot @module @swap`, after every binary (akira, architect, captain, dashboard-backend, huginn, kanban-app, micromanager, monad, sophia, timeguru, unheaded-daemon, wotan, zhen-agentd) ran identically under the old and corrected filter in transient units on west (timeguru also served /health under both). scripts/check-systemd-units.sh (meta-gate registered) keeps it fixed. Remaining: the installed /etc/systemd/system/huginn.service on west still has the old line; replace it with the repo copy and restart huginn, with Stevie's yes. |
| low | `host-probe:east:mount-home-separate` | east | mount | 7 | low | 2027-03-29 | east: /home is on /, not its own mount | Level 2: its own filesystem. east has no free disk space or LVM: accept, or shrink /var **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:mount-var-log-audit-separate` | east | mount | 7 | low | 2027-03-29 | east: /var/log/audit does not exist | Level 2: its own filesystem. east has no free disk space or LVM: accept, or shrink /var. /var/log/audit exists only once auditd is installed (step 3) **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:mount-var-log-separate` | east | mount | 7 | low | 2027-03-29 | east: /var/log is on /var, not its own mount | Level 2: its own filesystem. east has no free disk space or LVM: accept, or shrink /var **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:east:mount-var-tmp-separate` | east | mount | 7 | none | 2027-03-29 | east: /var/tmp is on /var, not its own mount | Level 2: a real filesystem for /var/tmp. east has no free disk space or LVM: accept, or shrink /var **decision:** accept Level 2 filesystem separations on east (no free space, no LVM) |
| low | `host-probe:west:mount-home-separate` | west | mount | 7 | low | 2027-03-29 | west: /home is on /, not its own mount | Level 2: its own filesystem. west: an LV from ubuntu-vg (588G free), copy data with services stopped, fstab, mount |
| low | `host-probe:west:mount-var-log-audit-separate` | west | mount | 7 | low | 2027-03-29 | west: /var/log/audit does not exist | Level 2: its own filesystem. west: an LV from ubuntu-vg (588G free), copy data with services stopped, fstab, mount. /var/log/audit exists only once auditd is installed (step 3) |
| low | `host-probe:west:mount-var-log-separate` | west | mount | 7 | low | 2027-03-29 | west: /var/log is on /var, not its own mount | Level 2: its own filesystem. west: an LV from ubuntu-vg (588G free), copy data with services stopped, fstab, mount |
| low | `host-probe:west:mount-var-tmp-separate` | west | mount | 7 | none | 2027-03-29 | west: /var/tmp is on /var, not its own mount | Level 2: a real filesystem for /var/tmp. west: an LV from ubuntu-vg (588G free); copy contents, fstab, mount |

Never observed (NOT_ASSESSED, not a pass):

- `attestation:compliance/attestations/security-policy-review.yaml`
- `attestation:compliance/attestations/ir-plan-review.yaml`
- `attestation:compliance/attestations/access-review.yaml`

<!-- END GENERATED FINDINGS -->
