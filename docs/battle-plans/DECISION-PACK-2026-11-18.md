<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# Decision pack, 2026-11-18: the sittings answered ahead

**Why.** Stevie, 2026-11-18: "compose list containing batches of prompts to address all these 'sittings' so they can be addressed unattended rather than with me present - i will answer any questions hindering progress now". Each row below is a question a plan asked of him in person, with his answer. An answer here stands in for the `[STEVIE]` step it names: the step runs without him, behind the same gates and rollback. Where this page and a plan disagree about who must be present, this page wins.

**Source of the questions.** The inventory of everything open, read 2026-11-18 (`~/tmp/pending-2026-11-18/COMBINED.md` on west: 270 rows; group 5 is the decisions, group 6 the things only he could do). Ids `M-n` are that file's.

**Standing limits that no answer here lifts.** Claude never pushes. Claude never sets `~/tmp/west-hold.conf` to false. Nothing about the OS release upgrade is done by Claude. Credentials never enter the repository or a log.

## Round 1: authority on west

| # | Question | Answer | Covers |
|---|---|---|---|
| 1 | May the live steps of ADR-147 plans 1 to 4 that wait for a "go" run on west without him: the old stack started for the befores, the deploy (plan 2 Step 338), the Postgres restart (383), Zhen's tables moved (plan 3 Step 265), the volumes moved to the NVMe (plan 4 Phase 5)? | **Yes, all of them.** Each behind its gates and its rollback; the first failed gate stops that block and is written to `~/tmp/next.md`. | M-161, M-163 (the go); blocks A, D to H, plan 4 sittings A to C |
| 2 | May Claude use the passwords already in `/etc/unheaded/compose.env` without printing them (plan 2 Steps 340, 341; plan 3 Step 266), and make the keys of plan 2 Step 317 on the host with `unheaded-keygen`? | **Yes to both.** Values are sourced inside the command; private halves are 0400 and never read back; only public halves are enrolled. | M-231, M-232 |
| 3 | Plan 4 Phase 6, the default-deny input filter on west: who must be near? | **Fully unattended**, and: "just ensure we don't loose /rc if it is on". So: the self-reverting rehearsal first; new connections tested from east and loopback; outbound HTTPS and the running Claude session's Remote Control link proved alive during the rehearsal, before the filter is kept. | M-33 (plan 4 Steps 187-214) |
| 4 | Plan 4 Step 296, the reboot that proves the layout survives a boot. | **Skip the reboot proof for now.** Plan 4 closes its other rows; the boot proof stays open until west is next restarted for any reason. | plan 4 Phase 11 |

**OS release (M-182, M-98), said the same hour:** "ubunutu is lowest priority because i need to hook up a monitor and keyboard to east to supervise upgrade. i could do west now but might as well do both in rapid succession". His hands, both hosts together, later. The two KEV kernel rows wait on it.

## Round 2

| # | Question | Answer | Covers |
|---|---|---|---|
| 5 | May Claude change east without him (series rule 18 said no)? | **Yes, everything but the OS upgrade.** East's firewall and sshd steps each go behind a self-reverting rehearsal; anything that needs the console waits. | M-53 (collector fix), plan 4 Phase 9, east rows of S-E1-LIVE and S-EVIDENCE-RUNS |
| 6 | Plan 2 Step 394, the orphans, per item. | **Delete all three:** volume `unheaded-well-data`; ClickHouse `system.query_log_0`, `system.error_log_0`, `system.query_metric_log_0`; volume `unheaded-dev_kanban-data`. | M-161 (394), plan 2 Steps 395-396 |
| 7 | The runbook manifest's seal and `runbooks/security/ioc-audit.yaml` (plan 3 D7). | **"Approve ioc-audit; you seal."** He approves `ioc-audit.yaml` by this answer (`approved_by` and the date are recorded from it); Claude makes a seal key on west and seals `runbooks/MANIFEST.sha256`. The seal then proves the files are unchanged since sealing, not that a person read each one: the other 39 draft runbooks stay `draft`. | M-164, M-228 (the seal; not the other approvals) |
| 8 | The Zhen switch's state after plan 3 Phase 9 (D8, expired with the power-down). | **On.** | M-165 |

## Round 3

| # | Question | Answer | Covers |
|---|---|---|---|
| 9 | Plan 4 Step 250 (WG6), which side wins per drifted file. | **Host wins for WireGuard and FRR.** Any other drifted file is brought to the repo's version when that is reversible, and listed for him when it is not. | M-170 (WG6) |
| 10 | Plan 4 Step 286 (WG7), `read_ahead_kb` on the spinning disk. | **Keep 256 only on a 20% gain,** else back to 8,192. The scheduler stays `mq-deadline`. | M-170 (WG7) |
| 11 | Ports 16666 and 20103 in `west.nft`'s LAN set. | 16666: drop ("likely 1"). 20103: **asked again in round 4**: the question called it Zhen's old UI port, which was wrong; it is `zhen-ui-front`'s port today (`pkg/ports` `ZhenWebUI`), loopback only until S-ZHEN-FRONT opens a LAN listener. | M-171 |
| 12 | Plan 5 Steps 49 and 87. | **Truncate; ClickHouse stays unless Postgres wins.** If Postgres fails the bar on any of the five tests, ClickHouse stays and the plan closes; if it passes all five, stop and ask. | M-172 |

## Round 4

| # | Question | Answer | Covers |
|---|---|---|---|
| 13 | Port 20103 (`zhen-ui-front`) in `west.nft`'s LAN set, asked again with the fact right. 16666 goes either way. | **Keep 20103 for the LAN.** 16666 is dropped from `LAN_TCP` and `LAN_FWD_PORTS`. | M-171 |
| 14 | Plan 1 Step 354 (SS4): do the four new latency and error SLOs alert. | **A: alert as tickets.** | M-166 |
| 15 | Plan 3 Phase 8B: the copy stops on a `zhen_actions.trace_id` that is not a uuid, or on a `zhen_*` table it does not know. | **Null it, record it, continue:** a non-uuid `trace_id` is stored NULL, with the row id and the original text written to the plan's progress file. An unknown `zhen_*` table still stops the copy: roll back to the old tables, restart Zhen, report. (The copy script stops on both today: the first half needs a tested option in `scripts/well/zhen-copy-rows.sh` before 8B.) | M-163 |
| 16 | Plans 1 and 2 "as built": 180 starts in 2 h; at most 4 over-cap pipeline mails a day; a 2 h shutdown mark; a 15 min ride-out; Sword, chaos-controller and Zhen's worker hold while a critical alert fires, scheduled runbook runs keep running; `FLOW_STATE` an LRU; offsite copies with plan 6; the ClickHouse `BACKUP` grant added only if the first backup step finds it missing. | **Confirm all.** Each number stays a config key. | M-162, M-168, M-169 |

## Round 5

| # | Question | Answer | Covers |
|---|---|---|---|
| 17 | soupsieve 2.10 (three advisories; the last red gate): bump to 4.0.1 in Zhen's venv? | **No bump:** "replace it, make something if need be... we shouldn't be using sketchy packages like this at all". soupsieve leaves the environment with whatever exists only to pull it in; a first-party use is rewritten on the standard library. | M-179 |
| 18 | S-ZHEN-FRONT Steps 115-119, the runtime lock: approve 44 names, stay on `start-zhen.sh`, or see the names. | **"Show me the 44 names first."** The list: `~/tmp/pending-2026-11-18/zhen-lock-44-names.md` on west. The front's Phases 10 to 12 stay parked until he has read it. | M-180 (open) |
| 19 | Accept the built ADRs 103, 106, 093 (DP-506 wording), 124, 125, 127, 135, 120? | **Accept all eight.** | M-199, M-200 |
| 20 | 76 merged local branches and about 53 finished worktrees: clean up? | **Yes, remove merged ones.** Only branches git confirms merged into develop; only clean worktrees; `~/tmp/wt/no-jenkins` and anything with uncommitted files is kept and listed. | M-213 |

## Round 6

| # | Question | Answer | Covers |
|---|---|---|---|
| 21 | ADR-150, choices 1 to 14 as the ADR recommends: (1) a person promotes the Well, the stateless part fails over by itself; (2) no third vote; (3) the second path through a switch (H2); (4) a default route through the other host, after stage 3; (5) FRR on west and BIRD on east as today; (6) Wotan a bus per host (A), with an ADR-035 amendment; (7) asynchronous streaming standby, the 24 h target kept; (8) the service address; (9) Docker with the same image on east; (10) east's issuer has its own key; (11) a second mail credential on east; (12) alone, east runs the survival set plus issuer, board and dashboard; (13) E1, the mailer-only watcher; (14) PostgreSQL 18 before the standby. | **Accept all 14.** | M-173; plan 6 becomes forgeable when the plans before it close |
| 22 | ADR-150 choice 15: does west's stack start at boot? | **"2 with plan to change later":** the stack stays down until a person starts it, and says so: a boot unit records the boot as its own event and the daily digest carries "west booted, stack down". Starting at boot is the intended later change. | M-173 (#15); ADR-147 finding "an unplanned boot is its own event" |
| 23 | ADR-151 L1 to L6 as recommended: drivers for ClickHouse and Postgres plus `none`; no third store yet; `/etc/unheaded/logstore.yaml`; Grafana keeps its ClickHouse panels first; its own plan after plan 5's verdict; the measurement writer becomes the Postgres driver. | **Accept all six.** No code now. | M-174 |
| 24 | ADR-148 (Yggdrasil as packages; the seed image). | **Still parked.** | M-175 |

## Round 7

| # | Question | Answer | Covers |
|---|---|---|---|
| 25 | S-STAGE-2C open call 10: how `UPC_RUNG56` goes on. | **On; the goldens compare the user-mode trace only** (the option that booted 12 of 12 SAME). Kernel-side trace changes stop being a regression signal. | M-185; unblocks rung 7 (M-86), the atomics fix and the fork slot scan (M-87); ADR-135 D21 |
| 26 | S-WATCH-TRUE Step 105, the growth row (0.026 core at 41.9 h against a bar of 0.05; 3.4 times its first hour against 1.5). | **Accept as bounded.** ADR-144 stays parked; the same binary goes to east. | M-187 |
| 27 | S-WATCH-TRUE Step 178, `scripts/bootstrap-llm-lab.sh` (pulls Loki, images on `:latest`). | **Delete the script.** | M-188 |
| 28 | The wiki as a fourth login front (sums 11 and 32 against the issuer's 10 and 25). | **Shrink the fronts to fit:** each front's budget reduced so the four stay under 10 and 25, the numbers stay config keys, and a test fails when a front is added uncounted. | M-189 |

## Round 8

| # | Question | Answer | Covers |
|---|---|---|---|
| 29 | soupsieve again: it is in Zhen's venv only through the ScanCode licence scanner (ScanCode → beautifulsoup4 → soupsieve), which Zhen never imports. Removing it means removing ScanCode and the 52 packages only it needs, 53 of 197. | **Remove ScanCode from Zhen's venv.** A later licence scan runs from its own throwaway environment. | M-179 |
| 30 | The 44 names of Zhen's runtime lock (`~/tmp/pending-2026-11-18/zhen-lock-44-names.md`): 18 torch CUDA support, 4 more torch's alone, 18 needed as written, 8 declared and never imported. | **Lock what runs today, all 44.** Nothing is downloaded; trimming is a later job. S-ZHEN-FRONT Steps 115-119 and Phases 10 to 12 are unblocked. | M-180, M-81 |
| 31 | The export notification (S-E1-LIVE Steps 20-21) and the two crypto constructions nothing calls (`pkg/secrets/encryption/age.go`, `crates/zhend/src/crypto/`). | **"I'll send it; delete the unused code."** Claude deletes the two, updates the audit and the notice text, and leaves the final email text in `~/tmp/next.md`; sending is his, from his own address. | M-181, M-238 (the send stays his) |
| 32 | The five judgment calls on the six `-01` drafts, chiefly ruling 37 (the replay check is per signature, not per packet). | **A written analysis first:** the RFC editor and security seats write one page on what per-signature replay checks leave open, what a per-packet field costs in the frozen format, and a recommendation on all five; he rules from that. | M-196 (open until he reads it), M-239 |

## Round 9

| # | Question | Answer | Covers |
|---|---|---|---|
| 33 | Root housekeeping on west listed as his: which may Claude do? | **All four groups.** (a) `docker builder prune` and delete `/etc/unheaded/rotation-20261112T063907Z`. (b) The stopped `jenkins` container, the `jenkins/jenkins:lts` image and `/var/lib/jenkins` (deleted without opening files). (c) The unread `GRAFANA_ADMIN_PASSWORD` line in `/etc/unheaded/monitoring.env`; `mg-clone-1110a` and `-1110b`, worktree `agent-aa2a6bfa1f6254235`, `thread-watch.sh`, `~/tmp/unheaded-login-temp-2026-11-11.txt`; `/var/backups/trace-collector-2026-11-15` once east runs the fixed collector. (d) Unused images, build cache and unused volumes outside the stack, after the deploy has built and tagged its rollback images; anything the stack or a rollback uses is kept. | M-241, M-242, M-243, M-250, M-253 |
| 34 | The kanban audit's 53 proposals (`~/tmp/kanban-truth/`), once the stack is up. | **Apply all 53,** the two flagged ones included, each through the board's API with a note citing the audit file. | M-226 |
| 35 | Timeline Age 4's "Customer onboarding flows" and "Billing/metering"; Age 5 empty. | **Reword to community terms:** "Adopter onboarding (first-run guide and installer)"; "Usage metering for operators (resource accounting, no billing)"; Age 5 "not yet defined". | M-225 |
| 36 | zhend's four subsystems that nothing spawns (QUIC server, Monad bridge listener, topology observer, strata snapshot collector). | **Move them out of the repo** to a sibling archive repo with their history; zhend keeps its single-node gRPC path; a parked ADR records how to bring one back. | M-224 |

## Round 10

| # | Question | Answer | Covers |
|---|---|---|---|
| 37 | The five calls on the `-01` drafts, with the analysis written (`~/tmp/pending-2026-11-18/drafts-01-five-calls-analysis.md`): (1) option-type row confirmed; (2) registry count confirmed with no number; (3) ruling 37: the 20 bytes untouched, per-signature replay protection accepted and documented, the comparison fixed (`<=` to `<`: as drafted a second packet on one signature is dropped as a replay), a maximum signature age added, a per-packet MAC TLV named as -02 work; (4) flags note confirmed; (5) seven stale section references in the foundation draft fixed, the flags offset among them (0x0B to 0x07). | **Accept all five.** Claude edits the pqc and foundation sources and regenerates the upload files; that the code still implements the -00 replay model (and is called only from tests) is recorded as an open finding. The upload stays his. | M-196; M-239 stays his |
| 38 | S-E1-LIVE host steps never applied: west's sshd to keys only, rotation of service keys, certificates and the two API keys, the same hardening on east. | **Yes, with a lockout guard:** before any sshd change a key login is proved on a second port with the new config, and the old config comes back by itself after 10 minutes unless a fresh key login succeeds; keys rotate one service at a time with health checks between. | M-66 to M-72 as far as no account or physical action of his is needed |
| 39 | Version bumps. | **Security bumps now, stores after the week:** CoreDNS 1.14.x, the end-of-life Alpine images, `anthropic-sdk-go` 1.84.0 and anything with a published fix ride on the deploy, each behind its gates. Postgres, ClickHouse, Vector and VictoriaMetrics wait for plan 5's close. `grpc` 1.85.0 waits (it adds two modules). | M-208, M-209 |
| 40 | IOC audit settings. | **Apply all three:** `spl` known on west as a host setting; operator networks narrowed to the LAN, the link to east and the overlay; the 21-day baseline as a recording rule. All stay config keys; the manifest is resealed after. | M-191 |
