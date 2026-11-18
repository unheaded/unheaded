<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# ADR-147 series: the live run order after the evidence week

**Written** 2026-11-17 (about 04:00Z), before the week ends at 18:34Z, from a read of the four plans' remaining phases against `integrate/adr147` `18fae670`. Nothing in it has been run. It adds no scope: every step it names is a step of plan 1 to 5. Where it and a plan disagree about order or about a step listed under "Amendments", this page wins; everything else is the plan's.

**Why it exists.** Each plan was written to merge its own branch and deploy alone. The four were merged into one branch instead (`docs/battle-plans/progress/ADR147-INTEGRATION.md`), so one tree now carries all four: any image built from it holds every plan's code, and any migration batch sees every plan's files. Read alone, the steps listed under "Amendments" are stale. Run in the order below with those amendments, they are not.

## The plans and what is left

| # | Plan | Done | Left | Mode |
|---|---|---|---|---|
| 1 | S-SEE-AND-SURVIVE | Steps 1-246 (code) | Phases 12-19, Steps 247-387 | sudo throughout; Stevie for the gate, SS1-SS4, the inbox check |
| 2 | S-WASTE-OUT | Steps 1-304 (code) | Phases 17-23, Steps 305-414 | Stevie at 305, 306, 317, 338, 340, 341, 383, 394 |
| 3 | S-ZHEN-SWITCH | Steps 1-237 (code) | Phases 8-10, Steps 238-330 | 8A sudo; 8B (the Well) with Stevie |
| 4 | S-WEST-GROUND | Steps 1, 12-13, 42-101, 104-119 | Phases 0, 1, 4-12 | four attended sittings, A to D |
| 5 | S-LOG-WEEK-2 | forged 2026-11-17 | all 96 steps | Phase 1 unattended; the rest after plans 2 and 4 |

## Before anything runs: Stevie, about 30 minutes

| # | What | Why it gates |
|---|---|---|
| 1 | Unlock the GPG key (any signed commit), then `scripts/resign-unpushed.py`, then push develop | Every commit since 2026-11-16 20:45Z is unsigned, on develop and on `integrate/adr147`. The merge of block C should be signed. |
| 2 | S-E1-LIVE Step 284: read the week against the bar (`docs/battle-plans/progress/S-E1-LIVE-progress.md`, "Step 284, prepared for Stevie") and say the week is closed | Three plans' gates open only on that record or on your word: plan 1 Step 247, plan 2 Step 305, plan 3 Step 238. A date alone opens none of them. |
| 3 | The rulings, in one round (table below) | Plan 3 forbids a merge to develop on an unanswered D1 to D6. |
| 4 | Say go for block B (the gates) and block C (the merge) | The merge changes files that running containers read at their next restart. |

**2026-11-18, about 00:30Z.** Row 2 is done: Stevie closed the week, ClickHouse stays, and he asked for a parked ADR on making the log store a setting (ADR-151). Row 3 is done but for plan 3's D7. Row 4 is given: blocks A, then B, then C, without asking again; block C only if every gate of B is green. Plan 2 Step 306 (the five entries in west's compose override): yes. Row 1 is still open, so every commit and the merge are unsigned until the key is unlocked.

**The rulings.** "Lane took" is what the code already does on the plan's recommendation; a different answer means rework before the live phase.

| Plan | Id | Question, short | Recommended | Lane took | Needed by |
|---|---|---|---|---|---|
| 1 | SS1 | Which alerts page | A, the urgency table | **Ruled 2026-11-17: one digest mail a day and nothing else** ("it's a lab, my email doesn't need the noise"; tune later). The table's urgencies stay as built; `mailer.delivery: digest` keeps every page for the 09:00 digest, where it leads. An intrusion or an indicator of compromise from an IDS or IPS, once one exists, mails at once: `mailer.immediate`, empty today | Phase 15 |
| 1 | SS2 | Mail cap on run days | A, 24 a day / 6 an hour | A, as built. It has no effect while `delivery: digest` and `immediate: []` | Phase 15 |
| 1 | SS3 | Start limits that trip | A (now 180 starts in 2 h, REV-3) | A | Phase 14 |
| 1 | SS4 | New SLOs alert as tickets | A | not asked | Phase 17 |
| 1 | | Is zhen-agentd the exception to the start limit | | **Ruled 2026-11-17: no exception.** The unit in the tree already trips (90 starts in 30 min, the tree's rule for `RestartSec` 10 s; this row first said plan 3 kept "never give up", which the merged tree does not). Step 308's drop-in goes on it like the rest | Phase 14 |
| 2 | D1-D7 | Publisher, subscriber, discovery reader, Prometheus keep-list, Akira's cert check, `FLOW_EVENTS`, `FLOW_STATE` | A, B, A, A, A, A, A | as recommended | Phase 19 |
| 2 | D8 | Postgres's budget | A: 2 GiB, 1.0 CPU, `shared_buffers` 512MB | **Ruled 2026-11-17: A.** Steps 384-393 are open | Phase 22 |
| 2 | Step 317 | Where the two public keys live (A, B or C); you make both ML-DSA-65 keys | B | **Ruled 2026-11-17: B**, a host-local `/etc/unheaded/wotan/wotan.yaml`. Stevie makes both keys | live publishing on `alerts.critical`; the deploy itself does not wait |
| 3 | D1-D5 | Switch is a systemd target; own database; fix the sweep; vor outside the switch; allowlist for private addresses | as listed | as recommended. **Ruled 2026-11-17: yes to all five** | block C |
| 3 | D6 | `app_zhen` connection limit; off-site copy | 12; local only | **Ruled 2026-11-17: 20; local only; the limit must be adjustable.** Done: `APP_ZHEN_CONNECTION_LIMIT` and `scripts/well/set-role-limits.sh` (any role), 20 stays the default | Phase 8B |
| 3 | D7, D8 | Approve `ioc-audit.yaml` and seal; Zhen on or off at the end | yours; on | **D8 ruled 2026-11-17: on, "till next power down".** D7 not answered: the audit timer stays installed and disabled | Phase 9 |
| 4 | WG1-WG5 | Move ClickHouse before the batch fix; backup sets to `sdb`; the two LAN paths; Alertmanager and Prometheus on loopback; west's filter before east's | A each | **Ruled 2026-11-17: A each, and the reboot (WG8) at the end of sitting D, "but document and ensure this isn't hard coded, and is configurable"** | sittings A and B |
| 5 | LW1-LW5 | Bar; indexes; start empty; timer; plan 8's baseline | A, C, A, A, A | **Ruled 2026-11-17: as recommended** (A, C, A, A, A) | its Phases 3 to 7 |

## 2026-11-18: the order after the power cycle, and the answers given ahead

Written 2026-11-18, about 16:30Z. Sources: `docs/battle-plans/DECISION-PACK-2026-11-18.md` (Stevie's answers), `docs/battle-plans/progress/ADR147-INTEGRATION.md` ("2026-11-18"), block B's two run directories on west, `~/tmp/next.md`. Where this section and the tables below it disagree about order or about who must be present, this section wins. Where this section and the decision pack disagree, the pack wins.

### What happened

| When (UTC, 2026-11-18) | What | Record |
|---|---|---|
| 00:39 to 00:50 | Block A, first readings, on the old stack | on west, `~/.local/state/unheaded/adr147-blockA/` |
| 00:47 | **Block C done**, before block B, on Stevie's word: merge `eacbd344` | develop |
| 00:50 to 00:53 | West stopped for a shutdown; powered on about 06:04 | stack down as left, 0 of 35 containers running |
| 10:56 to 12:07 | Block B, first run, on develop: 25 of 32 green, 7 red | `~/tmp/adr147-table/runs/after-week-20261118T105634Z/` |
| 13:59 to 14:35 | **Block B, second run, on `67af4654`: 32 of 32 green** | `~/tmp/adr147-table/runs/after-week-20261118T135930Z/SUMMARY.txt` |
| after 14:35 | The meta-gate, run alone: 103 gates bite, 6 inconclusive (already failing on that head) | the same directory |
| 16:05 to 16:08 | **Block A restarted** on the OLD stack | `~/tmp/next.md` |

The seven failures of the first run, fixed forward on develop (the merge was already in):

| Gate | What was wrong | Fixed by |
|---|---|---|
| `well-batch-apply` | `batch-apply.sh` refused a batch that creates a database: 051 makes `unheaded_zhen`, so the pre-batch set can never hold its dump; `batch-rollback.sh` refused the same directory | `6b1a588d` |
| `ci-local` | A data race in kanban-app: `openWell`'s retry read two package variables after `openWell` had returned (about 1 run in 15 under `-race`) | `8679ef2c` |
| `store-backups` | The test "damaged" a set by writing 0x00 over a first byte that was already 0x00 | `07fc96d8` |
| `prometheus-rules` | Three rule tests that could not pass (one sample an hour against a 5 min lookback; an expected `job` label the rule drops; a bystander series named like the renamed caller). No rule changed | `8a3210eb` |
| `well-zhen-database` | A type error in `zhen-schema-fingerprint.sql`. Behind it: 053's `ALTER DATABASE ... RESET` is refused for `well_app_owner`, which would have stopped the live 8B after the copy; two function bodies and three grants differed | `18f07c21` |
| `alert-path` | Prometheus v3.15.1 refuses a wildcard anywhere but the last path component: the module globs made `prometheus.yml` unloadable, so Prometheus would not have started at the deploy. Also the test's fixture alert had become a page | `5d7b23dc` |
| `ebpf-loads` | The runner's fault, not the tree's: `sudo -n` lost cargo, and the script takes root itself | the runner (`~/tmp/adr147-table/runs/after-week.sh`); no commit |

`git log --oneline 43ba3310..HEAD` (the first run's head to `4a4f312a`), oldest first:

| Commit | What |
|---|---|
| `6b1a588d` | fix(well): a migration batch may create a database (`well-batch-apply`) |
| `8679ef2c` | fix(kanban-app): `openWell` reads its two variables before the goroutine starts (`ci-local`) |
| `07fc96d8` | test(backup): damage a set by flipping its first byte (`store-backups`) |
| `8a3210eb` | test(prometheus): three rule tests corrected, no rule changed (`prometheus-rules`) |
| `18f07c21` | fix(well): the Zhen database rehearsal runs to its end; the copy's record for 053 is a table comment (`well-zhen-database`) |
| `5d7b23dc` | fix(monitoring): `prometheus.yml` loads again; a module's Prometheus files are copied to flat directories (`alert-path`) |
| `3bea949b` | docs(adr): ADR-093, 103, 106, 120, 124, 125, 127, 135 accepted (pack row 19); no gate |
| `67af4654` | docs(plans): decision pack, rounds 1 to 7; the head of block B's green run |
| `3bb69f4c` | chore(sbom): ScanCode leaves Zhen's venv; `check-python-audit` green (pack rows 17, 29) |
| `267ab4c2` | refactor(crypto): the two uncalled constructions deleted, CBOM regenerated; `check-crypto-inventory` green (pack row 31) |
| `4a4f312a` | docs(plans): decision pack, rounds 8 to 10 |

The meta-gate's six inconclusive gates:

| Gate | State |
|---|---|
| `check-python-audit` | fixed since (`3bb69f4c`) |
| `check-crypto-inventory` | fixed since (`267ab4c2`) |
| `check-ruff` | open |
| `check-tool-freshness` | open |
| `gen-man` | open |
| `well/test-pq-groups` | open |

Block A as it runs now:

| Item | State |
|---|---|
| The stack | The OLD containers, started 16:05Z to 16:08Z with `sudo docker start <name>` (no compose): the stores, then Wotan, Vector and the writer, then the applications and Alertmanager. All healthy |
| The mailer | `unheaded-alert-mailer` left stopped: it is the old build, and the committed policy stays `mode: dry-run` until plan 1 Step 319 |
| What the old images read | The merged tree's config files: `akira.yaml`, `wotan.yaml`, `prometheus.yml`, `scrape.yml` and the Corefile changed since `7be626d4`; `vector.yaml` and Postgres's did not |
| Hourly readings | Plan 2 Step 313 and Step 311's `cgio` pair: after a quiet hour (it ends 17:10Z), and again an hour later |
| Series readings | Those of 00:39Z stand; W-20's exact before stands (0.73 upserts/s, 0.98 WAL syncs/s). Every cgroup and statistics reading of 00:39Z is void after the power cycle |
| Rule | During the quiet hour and the hour between the readings: no container tests, no builds |

### The order now

| # | Block | State |
|---|---|---|
| 1 | C, the merge | done 00:47Z, `eacbd344` |
| 2 | B, the gates | done, 32 of 32 green on `67af4654` |
| 3 | A, the befores | running since 16:05Z |
| 4 to 10 | D, E, F, G, H, I, J | as in "The order" below, with this section's amendments. A failure is fixed forward on develop |

B ran before A because the gates need only dockerd, and the befores need the old stack up and an hour of quiet.

### The Step 348 trap (C-6)

| | |
|---|---|
| Fact | West's `docker-compose.override.yml` (gitignored, on the host) still names architect, captain, micromanager, monad and sophia. `docker-compose.yml` on develop no longer has them |
| Consequence | A compose command in the main project fails ("service X has neither an image nor a build context", plan 2 Appendix A procedure 1). The first one is plan 1 Step 276 in block D, `docker compose up -d --no-deps clickhouse`. The deletions were in plan 2 Step 348, block E |
| **Amendment** | The five deletions of plan 2 Step 348 are done at the START of block D, before plan 1 Step 276. Stevie's yes to Step 306 is on record (2026-11-18, "Before anything runs"). Then plan 2 Step 350's check: `sudo docker compose config --services` gives no error and none of the five |
| Stays in block E | The Wotan mount half of Step 348 (`/etc/unheaded/wotan/wotan.yaml`, Step 317 = B) |
| Side effect | The role containers still running count as orphans of the project until plan 2 Step 349 removes them. No step of the five plans passes `--remove-orphans` |

### Steps that no longer wait for Stevie

"Pack row" is the row of `DECISION-PACK-2026-11-18.md`. Each step still runs behind its own gates and rollback; the first failed gate stops its block and is written to `~/tmp/next.md` (row 1).

| Plan | Step | What the step asked | Pack row | What the executor does instead of asking |
|---|---|---|---|---|
| 1 | 273 | A missing ClickHouse `BACKUP` grant goes under "For Stevie"; Steps 276-292 blocked | 16 | Adds the grant only if Step 272 finds it missing, records the grant line in `$PROG`, goes on |
| 1 | 293 | SS5: the offsite copy to east now, or later | 16 | SS5 = A: "offsite with plan 6" in SSV-21/22's status |
| 1 | 324 | His inbox: did the mail arrive | 1 (block D), with the SS1 ruling of 2026-11-17 | Nothing is asked. The next 09:00 digest (America/Chicago) proves the relay: SSV-03's after is filled when the outbox shows it `sent`. A line under "For Stevie" asks him to look |
| 1 | 354 | SS4: do the four new latency and error SLOs alert | 14 | SS4 = A, as tickets. "Away: B" does not apply |
| 2 | 305 | The E-GATE: is the week closed | 1 (block A) | Reads the record: he closed it 2026-11-18 about 00:30Z (S-E1-LIVE progress, "Step 284: Stevie's verdict") |
| 2 | 306 | May the five override entries be deleted | 1 | His yes is on record (2026-11-18). The deletions move to the start of block D (the trap above) |
| 2 | 317 | Where the two public keys live; he makes both ML-DSA-65 keys | 2 | B stands. Claude makes both keys on west with `unheaded-keygen -algo mldsa65`: private halves 0400, never read back, never printed; only the public halves go into `/etc/unheaded/wotan/wotan.yaml` |
| 2 | 338 | "Deploy now?" | 1 | Deploys |
| 2 | 340 | He puts `APP_TIMEGURU_PASSWORD` in `.env` and runs the one role's password command | 2 | The key is already in `/etc/unheaded/compose.env` (its name was checked, nothing else). Claude runs the step's command; the value is sourced inside it and never printed |
| 2 | 341 | Migration 050, with him and the migration password | 2 | Claude runs the dry run, then the apply. The password is sourced from `/etc/unheaded/compose.env` inside the command, never printed |
| 2 | 383 | "Restart Postgres now with the D8 budget?" | 1 | Restarts it; not in the same hour as plan 3 Phase 9 |
| 2 | 394 | The orphans, yes or no per item | 6 | All three deleted: volume `unheaded-well-data`; ClickHouse `system.query_log_0`, `system.error_log_0`, `system.query_metric_log_0`; volume `unheaded-dev_kanban-data` |
| 3 | 265 | "Move Zhen's tables into their own database now?" | 1 | Moves them; never in the same sitting as plan 4's move |
| 3 | 266 | He types the migration password without echo | 2 | Sourced from `/etc/unheaded/compose.env` inside the command, as plan 2 Step 341 does; never typed, echoed or printed |
| 3 | 276 (8B) | The copy stops on a `zhen_actions.trace_id` that is not a uuid, or on a `zhen_*` table it does not know; what then was his | 15 | A non-uuid `trace_id` is stored NULL, the row id and the original text go to the plan's progress file, the copy goes on. An unknown `zhen_*` table still stops it: roll back to the old tables, restart Zhen, report. **Not runnable yet:** `scripts/well/zhen-copy-rows.sh` refuses both today; the first half needs a tested option in that script before 8B |
| 3 | 308 | D7: approve `runbooks/security/ioc-audit.yaml` and seal the manifest | 7 | `ioc-audit.yaml` is approved by the answer (`approved_by` and the date recorded from it). Claude makes a seal key on west and seals `runbooks/MANIFEST.sha256`, then the step's install and timer. The other 39 draft runbooks stay `draft` |
| 3 | 310 | D8: the switch's state at the end (the answer of 2026-11-17 expired with the power-down) | 8 | On |
| 4 | 123 | "At the desk with the console?" before swap, huge pages and the move | 1 | Phases 4 and 5 run unattended |
| 4 | 133 | "Delete `/var/swapfile2` and `/var/swapfile3`?" | 1 (sitting A) | Deletes them |
| 4 | 156 | "Stop the stack now for the move?" | 1 | Stops it |
| 4 | 187 | "Laptop and phone on the LAN, console in reach?" before the input filter | 3 | Fully unattended. The self-reverting rehearsal (Step 194) first. Inside it, beside Step 195: outbound HTTPS works and the running Claude session's Remote Control link is alive. One of them dead: no apply, `[STUCK]`. Only then Step 199 and the confirm of Step 204 |
| 4 | 189, 200 | The before and the after from his laptop and his phone | 3 | New connections are tested from east and from loopback. The laptop's LAN list and the phone on cellular have no stand-in (see "Not settled") |
| 4 | 192 | Not a question: ports 16666 and 20103 in `west.nft`'s LAN set | 13 | Before the install: `LAN_TCP` and `LAN_FWD_PORTS` in the `define` block of `deploy/nftables/west.nft` lose 16666 and keep 20103; Step 98's check; a commit |
| 4 | 225 | He adds `AUTH_ANONYMOUS_FROM` to `/etc/unheaded/compose.env`, then says yes to the recreate | 1 and 2 (see "Not settled") | Claude appends the step's line to `/etc/unheaded/compose.env` without printing the file, runs the step's `grep -c` check (→ 2), recreates |
| 4 | 250 | WG6: which side wins per drifted file | 9 | The host wins for WireGuard and FRR. Any other drifted file is brought to the repository's version when that is reversible, and listed for him when it is not |
| 4 | 264, 267, 271-274 (Phase 9) | Every step that changes east was his | 5 | East may be changed: `iperf3` from the distribution, the four runs. A `sudo` password prompt on east, or anything that needs its console, waits for him |
| 4 | 286 | WG7: keep `read_ahead_kb` 256 on `sdb`, or revert | 10 | Keep 256 only on a 20% gain (Step 285's rule), else back to 8,192. The scheduler stays `mq-deadline` |
| 4 | 292, 295-297, Phase 11 | WG8 and the reboot that proves the layout survives a boot | 4 | **Skipped for now.** No reboot. Steps 293 to 305 wait until west is next restarted for any reason. Plan 4 closes its other rows; the boot proof stays open |
| 5 | 49 | "Delete the Postgres copy of week 1 (`TRUNCATE`)?" | 12 | Truncates |
| 5 | 87 | He says which store stays | 12 | Postgres fails the bar on any of the five tests: ClickHouse stays and the plan closes (Steps 88 to 96). Postgres passes all five: stop and ask |

East, for every plan: series rule 18 ("east is attended") is lifted by pack row 5 for everything but the OS upgrade. East's firewall and sshd steps each go behind a self-reverting rehearsal.

### Not settled by the pack (listed, not guessed)

| Item | The plan says | The pack says | Left as |
|---|---|---|---|
| Plan 2 Steps 340, 341; plan 3 Step 266: reading the secrets file | The commands source `./.env` as the invoking user | Row 2: values are sourced inside the command | `./.env` is a symlink to `/etc/unheaded/compose.env`, root 0600 since 2026-11-12: as written the commands cannot read it without root. They are not changed here |
| Plan 4 Step 225: editing the secrets file | "An agent does not read or edit that file" | No row names the edit; rows 1 and 2 are the nearest | This section has Claude append the one line (a list of networks, no secret). His confirmation is wanted |
| Plan 4 Steps 189, 200, 210, 239: the LAN's view and the cellular check | Laptop on the LAN, phone on cellular; Step 210 resolves `host-probe:west:firewall-inbound-deny` only if the cellular test passed | Row 3: east and loopback | W-05's LAN list, the cellular check, Step 210's `resolved` entry and Step 239's LAN proof of the two source-gated paths stay open for him |
| Plan 4 Step 280: the 13 h freeze for the read-ahead measurement | His yes | Row 1 names sittings A to C; row 10 presupposes Phase 10 | Not marked as answered |
| Plan 4 Step 304: delete `/var/swapfile4` | Asked after the boot | Row 4: no reboot for now | Waits with the boot proof; W-03 stays open |
| Plan 5 Step 40, and block J "after the reboot of Phase 11" | Plan 4's new layout "with its reboot past", else plan 5 is `[BLOCKED]` | Row 4: no reboot for now | Not settled: as written, plan 5's Phase 2 stops on that line |
| Plan 1 Step 371 against plan 2 Step 408: the rollback tags | Plan 1 removes `:pre-ssv` at its close; plan 2 keeps `:pre-s-waste-out` for 3 days of green | Row 33 (d): what a rollback uses is kept | Plan 1's loop removes nothing (no `:pre-ssv` tag exists); the tags go by plan 2 Step 408 |
| Plan 4 Step 295: the silence's length | `maintenance.sh 30m` | Nothing (this page's amendment table says 60m) | `--planned` added, 30m kept |
| Plan 3 Step 272: an open `zhen-claude` or `zhen-cli` session | "Stevie closes it" | No row | 8B stops there until the session is gone |

### What still needs him

| What | Why it is his |
|---|---|
| The GPG re-sign and the push | `scripts/resign-unpushed.py --only-needed --merges`, then push. Develop is ahead of origin with unsigned commits; Claude never pushes |
| The OS release upgrade, west and east | His hands, both hosts together, a monitor and keyboard on east; lowest priority. The two KEV kernel rows wait on it |
| Hardware for east | Plan 6: its own uplink, the second path through a switch (ADR-150) |
| The upload of the six `-01` drafts | His account (pack row 37) |
| The export notification email | He sends it from his own address (pack row 31). The text: `~/tmp/pending-2026-11-18/export-email-final.txt` on west |
| The rows of "Not settled" above | His word, or his laptop and phone |

## The order

Blocks run in this order. A block's steps are the plan's own, with the amendments of the next section.

| Block | What | Steps | Who | Time | Must come after |
|---|---|---|---|---|---|
| A | **The befores, on a quiet west.** No container is built or started here. | The closing log store reading (`scripts/logstore-measure.sh`); plan 2 Phase 17 (Steps 305-319); plan 3 Step 15; plan 1 Step 263 (the smoke check on develop as it is) | Claude with sudo; Stevie at 305, 306, 317 | 2.5 h, two 1 h gaps inside | 18:34Z and row 2 above |
| B | **The gates that start a container**, on a clean copy of the integration branch | `~/tmp/adr147-table/runs/after-week.sh` (32 gates; add `--meta` for the slow provocations). It covers plan 1 Steps 255, 257, 260; plan 2 Steps 321, 323 and the eBPF load gate of Steps 273-277; plan 3's throwaway-Well tests; plan 4 Steps 75-76 | Claude, unattended | 1.5 to 2 h | A (the gates load the host; the befores want it quiet) |
| C | **One merge**, `integrate/adr147` into `develop`, in place of each plan's own | See "The merge" | Claude on Stevie's go | 15 min | every gate of B green, or its failure fixed on the integration branch and that gate rerun |
| D | Plan 1 live, part 1: backups and a drill; huginn, host state, start limits; rules loaded, mail on, four drills | Phases 13, 14, 15 (Steps 271-334) | Claude with sudo; Stevie's inbox at Step 324 | 4.25 h | C |
| E | Plan 2: tag the old, build, deploy. Plan 1's bucket rollout rides on it. | Plan 2 Steps 327-358; plan 1 Steps 342-348 an hour later | Stevie at 338, 340, 341 | 3 h, then a 1 h wait | D (a container stopped by mistake now mails) |
| F | Plan 2: the 1 h afters; Vector's batch; the Well sitting. Plan 3: the switch installed. | Plan 2 Phase 20 (first reading), Phases 21 and 22; plan 3 Phase 8A (Steps 238, 242-264) | Stevie at 383, 394 | 4 h | E plus 1 h |
| G | Plan 3: Zhen's database; the switch proved. Plan 2: the 24 h afters. Plan 1: SLOs from what was measured; vor and nginx. | Plan 3 Phase 8B and Phase 9; plan 2 Steps 379-380, 392-393 and the rest of Phase 20; plan 1 Phases 17 and 18 | Stevie for 8B and Steps 308, 310 | 5 h | F plus 24 h; 8B never in the same sitting as plan 4's move |
| H | The three closes | Plan 1 Phase 19; plan 2 Phase 23; plan 3 Phase 10 | Claude; Stevie pushes | 4 h | G |
| I | Plan 4, sittings A to D | Phases 0, 1, then 4-5; 6-7; 8-9; 10-12 | Stevie present | 2.5 h, 3 h, 2 h, 2 h over 14 h | D (its Step 7 needs plan 1's backups and swap alerts live); plan 2's 24 h afters taken, or its befores taken again on the new disk |
| J | Plan 5, the second week | Phases 2 to 7 | Stevie at the start and the verdict | 24 h baseline, 7 days | I closed (the reboot of its Phase 11 past) |

**Beside the blocks, unattended, any time after C:** plan 5 Phase 1 (the instrument: 5 h of repository work in its own worktree, tests against stubs, nothing live). And before sitting B of plan 4: merge develop into `sprint/s-west-ground-compose` and rerun `scripts/check-compose-pids.sh` (the lane still edits the five removed services; a conflict is likely; about 40 minutes).

**Why this order and not the series order plan by plan.**

| Constraint | Consequence |
|---|---|
| Plan 2's befores (Step 311 on, cgroup and store readings) must be read before anything is rebuilt from the merged tree. | Block A comes first, before the gates and before plan 1's rollout. |
| Plan 1 Phase 16 rebuilds wotan, dashboard-backend, kanban-app, logstore-writer, auth, timeguru. From the merged tree timeguru needs migration 050 and its role's password (plan 2 Steps 340-341), and the other five are in plan 2's build list anyway. | Phase 16's build and recreate (Steps 335-341) are done by plan 2 Steps 327-355. Its readings (Steps 342-348) run an hour after plan 2 Step 355. |
| Plan 1 Step 325's drill stops one of the role services; plan 2 Step 349 removes them. | Plan 1 Phase 15 before plan 2 Phase 19. |
| Migrations 050 (plan 2) and 051 to 053 (plan 3) are in one manifest; 053 refuses until the rows are copied. | Plan 2 Step 341 applies with `--through 050` (added 2026-11-17, `18fae670`); plan 3 Step 267 then finds exactly its three. |
| Plan 4 Step 7 proves plan 1's store backups and swap alerts are live. | Plan 4 after block D. |
| Plan 2 retakes a before if plan 4 moves the volumes before its after. | Plan 4's move (Phase 5) after plan 2's 24 h afters. |
| Plan 3 Phase 9 turns Zhen off and on; plan 2 Phase 22 restarts Postgres. | Not in the same hour. |

## The merge (block C)

It replaces plan 1 Steps 249-253 and 267, plan 2 Steps 320 and 325-326, plan 3 Steps 239-241, plan 4 Steps 102 and 120 (Merge A).

```bash
cd /home/govan/tmp/unheaded && git switch develop && test -z "$(git status --short --untracked-files=no)" && git merge --no-ff -S integrate/adr147 -m "Merge integrate/adr147: ADR-147 plans 1 to 4, the code (S-SEE-AND-SURVIVE, S-WASTE-OUT, S-ZHEN-SWITCH, S-WEST-GROUND)" && git log --oneline -1
for w in S-SEE-AND-SURVIVE S-WASTE-OUT S-ZHEN-SWITCH S-WEST-GROUND; do git -C /home/govan/tmp/wt/$w merge --ff-only develop | tail -1; done
```

- develop is already inside the integration branch, so the merge cannot conflict. A gpg timeout: the same with `--no-gpg-sign`, said in each plan's progress file.
- The four lane branches are inside the integration branch, so each `--ff-only` moves a lane worktree to develop. Later steps that say "in `WT`" then work on the merged tree.
- From this moment west's bind-mounted files are the new ones (rules, alert policy, `configs/wotan.yaml`, scrape files, the compose files). Nothing reads them until a HUP or a recreate; restart policy is `no`, so nothing recreates by itself. `docker-compose.yml` no longer names the five role services: their containers keep running as orphans until plan 2 Step 349.
- Then `scripts/ci-local.sh` on develop once (plan 1 Step 372's command), and each plan's progress file gets the merge commit.
- Not merged here: `sprint/s-west-ground-compose` (plan 4 Step 223).

## Amendments, by plan

A step not listed runs as written.

### Plan 1, S-SEE-AND-SURVIVE

| Step | As written | Instead |
|---|---|---|
| 247 | Opens on a line in S-E1-LIVE's progress file | No such line yet: it opens on Stevie's answer at Step 284, written there as a new dated entry |
| 249-253, 267 | Merge seven lanes, then the sprint branch | Block C. Six of the seven lanes never existed |
| 254-262, 264 | Gates on the sprint tree | Block B ran them on the merged tree; read `SUMMARY.txt` of that run |
| 263 | After the merge steps | In block A, before the merge |
| 276 | `docker compose up -d --no-deps clickhouse` as block D's first compose call | Added 2026-11-18: first plan 2 Step 348's five override deletions, or the call fails ("The Step 348 trap") |
| 308 | Expects `15min` and `10` | `2h` and `180` (`deploy/systemd/dropins/10-start-limit.conf`, REV-3) |
| 321, 324, 326, 329, 333 | A page and the pipeline mail show as `sent`; Stevie's inbox is asked at Step 324 | Amended in the plan (2026-11-17, one digest a day): each shows as `digest` in the outbox and no mail leaves. The relay is proved by the next 09:00 digest (America/Chicago), which carries the drills; SSV-03's after is filled then |
| 335-341 | Tag, build, recreate seven services and three host units | Done by plan 2 Steps 327-355. Take the planned stop of Step 335 before plan 2 Step 339 and end it (Step 341) after plan 2 Step 356. `unheaded-wiki-server` is rebuilt by plan 3 Step 256 |
| 346, 371 | `:pre-ssv` tags | The rollback tags are plan 2's `:pre-s-waste-out` |
| 350, 352 | `pq` on `unheaded_http_*` | Prometheus drops those series after plan 2's keep-list (Step 355); read VictoriaMetrics (`q`) |
| 355-358 | Four SLOs into policy and rules | Also `python3 scripts/gen-prom-keep-list.py` and the SLO source rows in `compliance/catalog/controls.yaml`, in the same commit |
| 166, Emergency 9 | "30d" | The code keeps a mark at most 2 h past its end; read `scripts/monitoring/planned-stop.sh` |

### Plan 2, S-WASTE-OUT

| Step | As written | Instead |
|---|---|---|
| 320, 325-326 | Merge `sprint/s-waste-out` | Block C |
| 321, 323-324 | Container gates and ci-local on the sprint tree | Block B. The sprint branch alone fails `check-image-tags` and the image digest test; the fixes are integration commits |
| 273-277 (never run) | eBPF load gate and instruction ratchet | Block B (`ebpf-loads`, `ebpf-insns`). Step 343 installs nothing that gate has not passed |
| before 339 | No mark | `sudo scripts/monitoring/maintenance.sh --planned 60m "S-WASTE-OUT deploy"`; end it after Step 356 (plan 1's rules are live by then) |
| 340 | `env -u` of eight variables | Amended in the plan: every `*_PASSWORD` but `APP_TIMEGURU_PASSWORD` is unset in a subshell (the script also knows `POSTGRES`, `WELL_BACKUP`, `RETENTION_RUNNER` and `LOG_WRITER`) |
| 341 | `--from 050`, expects `pending (1)` | Amended in the plan: `--from 050 --through 050`; expects `pending (1) through 050` and three files left for a later batch |
| 348 | The five override deletions and the Wotan mount, in block E | Added 2026-11-18: the five deletions at the START of block D, before plan 1 Step 276; the Wotan mount half stays here ("The Step 348 trap") |
| 371 | Add job `victoria`; rule in `rules/host.yml` | The job exists (plan 1); the rule goes in `rules/watchers.yml` beside `VictoriaMetricsDown` |
| 372 | "Step 198's promtool" in a lane worktree | `scripts/check-prometheus-rules.sh` in `WT`; no lane worktree was made |
| 359 (W-25) | Bar "tmpfs: none" | timeguru keeps its `/data` tmpfs on purpose (progress file, Phase 16); record the row as it is |
| 367, 375, 384, 387 | `git merge --ff-only` between `WT` and develop | Work after block C; a "not possible to fast-forward" is handled as Step 375 says |

### Plan 3, S-ZHEN-SWITCH

| Step | As written | Instead |
|---|---|---|
| 239-241 | Merge the sprint branch | Block C |
| 247 | `HEAD^1` is the unit before the merge | Compare against `7be626d4` (develop before block C) |
| 252, 301 | Expects `30s auto-reload-config` | The tree passes `--config.auto-reload`; read what the command prints and record it |
| 257 | `curl http://127.0.0.1:20002/auth/nav.js` | The wiki unit serves TLS: `https` with the CA file the unit names |
| 266 | `052_app_drop_zhen.sql` | Amended in the plan: `053_app_drop_zhen.sql` |
| 267 | Stops unless only its three are pending | True once plan 2 Step 341 has applied 050 |
| 273-275, 278, 281, Appendix A procedure 4 | Files named 050, 051, 052 | They are 051 (database and role), 052 (schema), 053 (drop). Procedure 4's ledger delete names `051_zhen_database.sql` |
| 274, 287, 322 | Connection limit `12` | Amended in the plan at Step 274: `20` (migration 051, REV-5). The same at 287 and 322 |
| 283 | `/etc/unheaded/zhen-agentd.env` | The unit in the tree reads `/etc/zhen-agentd.env`; Step 247's diff shows which the host has. Edit the one the installed unit names |
| after 283 | Nothing | Two things the progress file promises "at 8B": `scripts/write-zhen-app-env.sh` (REV-2), and `WELL_ZHEN_DSN` in the overnight user unit's env file (REV-1) |
| 319, Appendix A procedure 7 | Baseline `1393e5ee` | `7be626d4` |
| 322 | | Also the budget table in `docs/architecture/THE_WELL_OPERATIONS.md`: say that `app_zhen`'s role limit is 20 and its pools are 2 x 5 |
| 323 | A wiki link check script | `tests/scripts/test_doc_links.py` under the CI venv |

### Plan 4, S-WEST-GROUND

| Step | As written | Instead |
|---|---|---|
| 102, 120 | Merge A and the Phase 3 merge | Block C |
| 7 | Plan 1's backup and swap-alert rows read done | True only after block D; until then Phases 4 and 5 are `[BLOCKED]`, as the step says |
| 147, 171 | Checks the Well's backup timer | Also `unheaded-clickhouse-backup`, `unheaded-victoria-backup` and `unheaded-store-drill` timers (plan 1): none due inside the window |
| 157, 226, 295 | `maintenance.sh 60m` | `maintenance.sh --planned 60m "<reason>"`: a silence alone lets `ContainerStopped` and the SLO budgets count the stop |
| 215, 217 | Merge develop into the compose lane | Expect a conflict on the five removed services; 18 services remain; the limits then sum to 12,800 (11.1% of the thread limit). Since lane commit `6b50634b` the limits, the slice and the gate's thresholds are variables with these values as defaults; two comments in the lane still say 15,360 and 13.3% |
| 225 | Stevie says yes to the recreate | Amended in the plan (2026-11-17): first he adds `AUTH_ANONYMOUS_FROM` with west's two networks to `/etc/unheaded/compose.env`; the compose default no longer carries them (lane commit `6b50634b`) |
| 218-219 | A grep for slice names | It also matches huginn's test fixtures; `cmd/huginn/cgroups.go` walks any slice to depth 4, so a hit in a fixture is not a defect |
| 165 | Installs Docker's drop-in from `deploy/systemd/docker.service.d/` | Amended in the plan (2026-11-17): the file is west's own, `deploy/host-settings/west-docker-data.conf`, installed under the same host name |
| 246 | Expects `rc=1` | The gate exits 4 on drift (REV-2) |
| 256 | The drift unit names `west` and the checkout | Amended in the plan (2026-11-17): the unit takes both from `/etc/unheaded/host-settings.env`, which the step installs first. Step 245's direct call with `--host west --repo ...` is unchanged |
| 189-200 | The filter's values are read in the rules | They are a `define` block at the top of `deploy/nftables/west.nft` since 2026-11-17; the loaded ruleset is identical. `wiki/Host-Settings.md` lists them |
| 296 | Reboot | `~/tmp/west-hold.conf` holds a block lock while Claude or tmux runs: `sudo reboot` is refused. Lifting it is Stevie's (`hold=false`, or `sudo systemctl reboot -i`); Claude never does either |

## What could go wrong on the day, and what is already in place

| Risk | In place |
|---|---|
| A gate of block B fails on the merged tree. Several were written or changed in the week and have never met a container: the hadolint and trivy baselines (edited by hand), the Zhen test scripts moved to `unheaded_zhen`, the new cases of `test-batch-apply.sh` | Nothing is merged until it is fixed on the integration branch and rerun. Each gate has its own log under `~/tmp/adr147-table/runs/after-week-<time>/` |
| The Postgres copy of the logs reaches the writer's 4 GiB ceiling about 2026-11-22 20Z | It then answers 503 and Vector drops at that sink only; the record in ClickHouse is not touched. Plan 5 empties the table at its Step 52. To stop it sooner: plan 5 Steps 47-48 (a mark, then `compose stop logstore-writer`), no Vector restart |
| After plan 2 Step 349 a role container reads as stopped | They are removed in the same step; the mark of the amended Step 339 covers the minutes between |
| A sitting ends between blocks | Every block ends at a plan's exit gate. D, E and F each leave west in a state the next morning's alerts describe truthfully |

*Not executed. Step numbers were read from the plans at `integrate/adr147` `18fae670`; four read-only agents checked the remaining phases against the tree, and the findings cited as amendments to Steps 308, 340, 341, 266 and 274 were verified by hand.*
