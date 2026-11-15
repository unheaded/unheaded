<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-EVIDENCE-RUNS progress

Plan: `docs/battle-plans/S-EVIDENCE-RUNS-battle-plan.md` (12 phases, 262 steps). Branch `sprint/evidence-runs`, worktree `~/tmp/wt/S-EVIDENCE-RUNS`, cut from develop `a36a8190`. This file holds counts and verdicts only; collected host data stays under `~/tmp/unheaded/var/compliance/` (untracked).

## Decisions

All nine answered by Stevie on 2026-11-13 (copied verbatim from the plan's table). No step is blocked on a decision.

| # | Question | Recommendation | Answer |
|---|---|---|---|
| DE1 | Install AIDE (Ubuntu `aide`, no recommends) on both hosts. It is a new distro package, and DP-09's answer did not name it. | **Yes.** It is SI-7 file integrity and is already in the `lab-hardened` model. | **ANSWERED 2026-11-13: yes, both hosts**, in Stevie's words "set up carefully. Ensure no spam or useless disk io is being used". The design that follows is in "AIDE, quiet by design" below: a tight include list, an idle-scheduled weekly timer, no package cron, no mail, one notification per new diff, init and updates only after an attended review, and a measured first-run gate. |
| DE2 | Remove every register row the findings report lists as passing (`remove from the register`), as the register header prescribes. Today that is 7 rows; more will come during the plan. Sweep row A-51. | **Yes, all of them.** Each removal names its source and the observation time in the commit body. | **ANSWERED 2026-11-13: yes, remove passing rows.** |
| DE3 | Who seals the compliance snapshots a host's root acts on? | **A dedicated ML-DSA-65 key on west** under a nologin account `unheaded-compliance-seal` (DP-06 daemon-key rule), with the public half in the repo, a 31-day TTL and a monthly reseal at your hands. The alternative is the B-10 build key at a ceremony each month. | **ANSWERED 2026-11-13: the dedicated ML-DSA-65 key on west, as recommended.** |
| DE4 | West's profile. `lab-hardened` also turns on `sshd-safe` (risk high) and `apparmor-enforce`. | **New profile `lab-audited`** = `lab` + `auditd-rules` + `aide-install` + `rocm-log-perms`. West moves to `lab-hardened` at B-08 (sshd) and after B-18 (the X server purge). | **ANSWERED 2026-11-13: `lab-audited`, as recommended.** |
| DE5a | Fix in the host windows: `dot-files` (both hosts; chmod g-w on govan's own files), west `root-path` (remove the empty `/root/.vector`), purge the ftp and telnet clients (both), trial `/dev/shm` noexec (both) and east `/tmp` noexec. | **Yes to all.** Each has a dry run and a rollback. | **ANSWERED 2026-11-13: yes, fix them** (round 7). |
| DE5b | Accept 90 days, "revisit before real data or internet exposure": west `home-{nodev,nosuid,separate}`, `var-log-{noexec,separate}`, `var-log-audit-{noexec,separate}`, `var-tmp-{noexec,separate}` (9 rows). `/home` lives on `/`, and `/var` holds Docker, so noexec there breaks it. This is the west twin of DP-25.11. | **Accept.** | **ANSWERED 2026-11-13: accept the 9 west rows for 90 days** (round 7). |
| DE5c | `var-log-perms` (west 235 files, east 66, Ubuntu's 0644 defaults): a `fileperm` fix in the model (enforceable, so it counts for condition 3; rotation recreates the modes and enforcing re-fixes them), or accept. | **Fix** (`var-log-modes`) | **ANSWERED 2026-11-13: fix, `var-log-modes`** (round 7). |
| DE6 | FND-009, WireGuard root by necessity: accept for 90 days (cap_drop ALL + NET_ADMIN, read-only, deployed on neither host), or rewrite its verify to "WireGuard is the only entry". | **Accept**, reason as written | **ANSWERED 2026-11-13: accept for 90 days, reason as written** (round 7). |
| DE7 | East: apply `apparmor-enforce` (the Xorg profile from complain to enforce; no Xorg runs on east) through the executor, so east's journal reads 15 of 15. | **Yes** | **ANSWERED 2026-11-13: yes, through the executor, attended** (round 7). |

The AIDE design that DE1's answer asks for is the plan's section "AIDE, quiet by design".

## Baseline (Phase 0, 2026-11-15)

- Tools: go 1.27.3 (go.mod `go 1.26.0`, `toolchain go1.27.3`), jq, curl, govulncheck, gosec, gitleaks: all present.
- Go baseline green under `-race`: `pkg/compliance/...` (6 packages), `cmd/compliance-evidence`, `cmd/crypto-inventory`, `cmd/unheaded-baseline`, `cmd/unheaded-remediate`, `tomb/lich/LICH-017-baseline-agent`. `check-crypto-inventory.sh` PASS.
- Findings report before (evidence collected 2026-11-15 01:29Z, report 02:03Z): **168 open** (3 untriaged, 11 high, 26 medium, 128 low), 77 accepted, 5 overdue, **7 "remove" lines**. The plan expected about 167 open: FND-044 (S-WATCH-TRUE) is the one more.
  - Untriaged: `github-job:Security Scan (Daily + PR)/Go Vulnerability Check`, `github-job:Security Scan (Daily + PR)/Secret Scan (gitleaks)`, `host-probe:east:dot-files`.
  - Remove: `FND-032`, `FND-038`, `git-signatures:develop`, `github-job:Static Analysis/Python (ruff + bandit)`, `github-job:SBOM + Vulnerability Scan/sbom`, `host-probe:west:alert-route`, `host-probe:west:db-backup-recent`.
- Sibling plans:
  - S-E1-LIVE Step 218 (Evening 10B exit) has **not** passed: Phase 9 (east window) and Steps 145-149 are `[BLOCKED]` on it.
  - S-WATCH-TRUE Phase 7 is done and live on west since 2026-11-15T01:39Z: `unheaded-baseline.service` runs with `-textfile-dir` (`systemctl cat` shows it), so Phase 11 can read the soak from the metrics.
  - FND-044 is in the register (S-WATCH-TRUE); this plan takes no FND id.

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-16 | Phase 0. Decisions copied; baseline green; findings report before written to the host (`var/compliance/findings-s-evidence-runs-before.md`). The worktree has no `var/`, so the report reads the main checkout's `var/compliance/evidence.json` through `-evidence`. |

## E-row outcomes

- E-01:
- E-02:
- E-03:
- E-04:
- E-05:
- E-06:
- E-07:

## Blocked

- Phase 9 (east host window) and Steps 145-149: S-E1-LIVE Step 218.
- Phase 10 (west host window): attended, after Phase 9.
- Steps 110-112 (the notice text): A-25.

## For Stevie

Nothing yet.

## Stuck

Nothing.
