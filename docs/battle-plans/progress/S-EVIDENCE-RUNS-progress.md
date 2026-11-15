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
| 2026-11-15 | 193-194 | Phase 8, the repo half (`dd2e0e17`). `unheaded-compliance-seal` is uid 16765 in `pkg/uids` (next free after zhen-app's 16764; free in the registry, sysusers and both hosts' passwd), with a sysusers line: nologin, home `/var/lib/unheaded-compliance-seal`. Uid test first, red then green; `systemd-sysusers --dry-run` parses the file. Nothing is created on a host: Step 195 on is Stevie's. |
| 2026-11-15 | 142, 174 (register) | `093748d4`. FND-037's dated line (west read, east waits). Four new rows: `host-probe:{west,east}:aide-quiet` and `aide-unreviewed`, low, class package, due in the host windows. The four older AIDE rows carried a plan that DE1 replaced (aide-common, `aideinit`, the package's daily timer); each has a dated superseding line. Findings report on the host's evidence, `var/compliance/findings-s-evidence-runs-after.md`: **168 open** (1 untriaged, 11 high, 25 medium, 131 low), **87 accepted**, 5 overdue, 19 never observed, **1 "remove" line** (FND-038, kept: see For Stevie). The untriaged one is `gate-script:scripts/check-crypto-inventory.sh`, exit 1 in the evidence of 03:31Z: the inventory was stale until `b57230ef`; it clears at the next collection from a tree that holds that commit. No row passes, so DE2 removes nothing. |
| 2026-11-15 | 187-191 | **Phase 7 exit gate met on the sprint branch** (`690b4659`: Phases 0-1, lanes A, B, D, C, develop merged in). Race tests of Step 187's packages green. The whole Go suite, `go vet ./...` and the wider checks ran on the merged tree. **What the merge broke, fixed here:** (1) `check-crypto-inventory.sh` stale: lane B regenerated the CBOM before lane C's host-service collector (a TLS client config) and S-ZHEN-FRONT's edit to `cmd/zhen-ui-front` landed; regenerated, 116 components (`b57230ef`). (2) `cmd/dashboard-backend/internal/server` pins 18 fixes and 5 profiles; Phase 7 added one of each and its test list did not include that package (`2a44088d`). **Green as merged:** runbook manifest fresh (93, unsealed), `check-runbooks.sh` (0 new), `check-prometheus-rules.sh` (13 rule files, 13 test files), `build-binaries.sh --check-units`, `check-systemd-units.sh` (55 units), `check-unit-ceilings.sh`, `check-log-sources.sh`, ruff, `tests/scripts` and `tests/runbooks` (549 passed, 4 skipped), `gen-man.sh` (57), `check-nosec-ratchet.sh`, `alert-policy --check`, `test-deb-maintscripts.sh`, `check-version-flag.sh` (62). **Red before this sprint and still red:** `check-python-audit` (soupsieve, For Stevie). Not run here: the meta-gate (lane D ran it in full on its own head: 97 gates bite). |
| 2026-11-15 | 174-186 | Lane C, second shift (`0672c82d`). Probes `aide-unreviewed` (central: a diff nobody reviewed, or a last check over 8 days old; one row per host whatever the run count) and `aide-quiet` (baseline: package timer masked or absent, cron script diverted, our unit at `Nice=19` and idle I/O), both on UH-FIM-01 for both hosts; `aide-install` closes `aide-quiet` too; `baseline.yaml` regenerated (two lines). Alerts `AIDEChangeDetected` and `AIDECheckStale` in `host.yml`, ticket urgency by policy, a draft runbook each (`aide-change-review`, `aide-check-stale`). **`AIDEChangeDetected` is bounded to the day after the run that first saw the diff**: the textfile keeps `new_diff 1` until the next weekly run, and the plan's bare `== 1` would be a digest line every day for a week. Profile `lab-audited` (DE4): lab plus `auditd-rules`, `aide-install`, `rocm-log-perms`, `var-log-modes`. Fix `var-log-modes` (DE5c): a new `ceiling` form of the mode action; the ceiling is `crosswalk.VarLogMaxMode`, moved out of the probe so check and fix are one function; the Ansible rendering agrees with it on 18 names. Dry-run plans of `aide-install` and `var-log-modes` read as intended, no `aideinit`. Step 179: a profile reaches the Well with `compliance-evidence profile set -host <host> -profile <name> -reason "..."` (subcommands `set`, `export`); the switch is `runbooks/security/compliance-profile-switch.yaml`. |
| 2026-11-15 | 169-173 | Lane C, second shift (`5eed0983`). The first shift stalled after Step 168 with `cmd/unheaded-aide` written and uncommitted; all of it was kept. `check` maps aide's exit code, writes one line and six metrics, keeps `state.json`; a diff is new once; it never passes `--init` or `--update`. **The unit ran a binary nothing built** (`build-binaries.sh --check-units` red since `70a102c0`): `unheaded-aide` is now in the build map and the deb table with its timer, holds no service key; `debian/control` regenerated; man page; a log source entry, excluded until installed. One reviewed `#nosec` (G204 on the exec of the aide binary) added to the baseline by hand. Proved with the package's own AIDE 0.19.1 binary run from a scratch directory, nothing installed: `--config-check` passes on the model's config; init, promote, clean, a new diff `new=1`, the same diff `new=0`, a wrong hash refused, accept, clean. AIDE 0.19 refuses a negative line that does not start with `!/`. |
| 2026-11-15 | 161-168 | Lane C, first shift (`70a102c0`). Step 162-163: `aide` ships only the binary; `aide-common` ships `dailyaidecheck.timer`, `dailyaidecheck.service`, `dailyaidecheck-buildcache.service`, the cron script under `/etc/cron.daily/`, `/etc/default/aide` and `aide.conf.d`. `aide-install` installs `aide` without recommends, writes `/etc/aide/aide-unheaded.conf` (include list, no `@@include`, no atime) and our service and timer, masks the package timer, diverts both cron script names, validates with `--config-check`, enables our timer. Typed `divert` and masked-service actions with undo. The fix never builds a database. |
| 2026-11-15 | 136-144, 150 | Lane C, second shift. Live trial on west into scratch: **12 `host-service` entries, all pass** (akira, architect, captain, cuirass, dashboard-backend, kanban-app, logstore-writer, micromanager, monad, sophia, timeguru, wotan: running, auth on, 401 without credentials). Secret sweep 0. Step 140: **no `unheaded-compliance-evidence` unit exists in the repo or on west**; the collector runs by hand as govan, who is in the `docker` group, so Step 141 holds for hand runs. **West half of Phase 5's exit gate met.** Steps 145-149 `[BLOCKED]` on S-E1-LIVE Step 218. |
| 2026-11-15 | 119-135 | Lane C, first shift (`05ea746e`, `f4fc0518`, `3fbb94c8`). ADR-097 amendment; the `host-service` evidence kind (central only, not a baseline probe); UH-AUTHN-01 gains a `host-service` source per west service, `declared:` reads "east not yet read (B-11)". |
| 2026-11-15 | 151-160 | Lane A (`4cd36fcd`). `compliance-evidence findings -enforce-ready <host>`: build red, then green under race; gosec clean. **West `ready: no (25)`, east `ready: no (15)`**; readings in `var/compliance/enforce-ready-{west,east}.txt` on the host. It counts two kinds of row the plan did not name: an untriaged row on the host, and a row in an enforceable class that names no host (FND-010, counted for both). Owners table below. |
| 2026-11-15 | 91-118 | Lane B (`68811829`, `5fead657`, `f4216ca5`). CBOM kinds for AEAD, KDF, hash and own primitive: nine tests red, then green under race. 97 → 115 components, every old row unchanged: `aead-import` 2, `kdf-import` 4, `hash-import` 2, `rust-crypto-dep` 8, `own-primitive` 2 (the second is `services/sophia/hkdf_derive.go`). A fifth kind, `cipher-import`, covers `crypto/aes` with no GCM (no file uses it today). Every audited path has a row except `pkg/protocol/integrity/hmac.go` (HMAC is read as a digest use; there is no `mac` kind). `own-primitive-unlisted` rides the `check-pq-kex.sh` lint and cannot be baselined. `unheaded/unheaded` is public, created 2026-01-26; eight public org repos. Steps 110-112 `[BLOCKED]` on A-25. No register change. |
| 2026-11-15 | 63-90 | Lane D (`1c90b9ed`, `ec11410b`, `4991fab4`). Each red job of the 2026-11-11 push reproduced in a clean clone at `1b8b46be` and at the lane head. **govulncheck:** GO-2026-6859 (`net/http`, go1.27.3); every live Go pin to 1.27.4 (30 files; `check-ci-verdicts` caught four Jenkinsfiles and `zhen-agentd.Dockerfile`); now no vulnerabilities. **gitleaks:** the history step only; `2613b0c2` deleted a README and its ignore line; one history-baseline row (a 14-character all-caps placeholder, not a credential); `.gitleaksignore` untouched. **gosec:** G104 in `cmd/wiki-server/nav.go` and the nosec ratchet; green since `1dcae498`. **Meta-gate, full:** 97 gates bite; `monitoring/test-alert-path` was failing since `eafa3806` (the test's policy rewrite broke the hourly-cap rule and the mailer exited), fixed; `check-python-audit` stays red. Register rows applied by the coordinator (`c053317e`). Steps 87-89 wait on Stevie's push. |
| 2026-11-15 | 51-62 | Lane A (`b52b1bf3`). `check-service-auth.sh` knows token-auth services: red on the pass case, then green under race. On the tree: 14 first-party services, 12 by `AUTH_ENABLED`, 1 by token (`logstore-writer`, checked in `ingest.go` `ServeHTTP`), 1 exempt, rc=0; three provocations on copies of the compose file all exit 1. An empty token value fails, and so does a bare `- NAME` entry. The meta-gate has no row for this gate (it does not cover `scripts/compliance/`). Register line applied by the coordinator (`c053317e`). |
| 2026-11-15 | 45-50 | East `dot-files` triaged; 9 acceptances (DE5b); 17 dated plan lines (DE5a, DE5c, B-18). Register test `ok`. **Phase 1 exit gate met.** Findings after Phase 1 (evidence of 01:29Z, report 02:14Z): **167 open** (2 untriaged, 11 high, 25 medium, 129 low), 87 accepted, 5 overdue, 1 "remove" line (`FND-038`, see Step log 28-33). Before: 168 open, 3 untriaged, 77 accepted, 7 remove lines. Report on the host: `var/compliance/findings-s-evidence-runs-phase1.md`. |
| 2026-11-15 | 34-44 | FND-009 accepted per DE6 (register test `ok`; accepted count 77 → 78). E-05 read. **Steps 38-39 as written read VictoriaMetrics, which holds none of 2026-10-22 14:00 to 2026-10-25 20:00** (9,598 samples of `slo:good` in 28 days against Prometheus's 14,671), so its 28-day average (dashboard 0.9963) disagrees with the budget (-3,405 %). The budget rule runs in Prometheus on Prometheus's samples, and there the arithmetic checks (0.8248 → -34.05). The readings were redone against Prometheus (`127.0.0.1:9090`). One postmortem instead of five: the five class (b) rows share one window and one cause. |
| 2026-11-15 | 28-33 | DE2 removals. Seven entries deleted by name, each on its newest record: `git-signatures:develop` (pass, 2026-11-15T01:29:05Z), `github-job:Static Analysis/Python (ruff + bandit)` (pass, 2026-11-11T22:55:53Z), `github-job:SBOM + Vulnerability Scan/sbom` (pass, 2026-11-11T22:57:14Z), `host-probe:west:alert-route` and `host-probe:west:db-backup-recent` (pass, 2026-11-15T01:29:09Z), and the resolved `FND-032` and `FND-043`. **Step 32 reads 1, not 0:** `FND-038` stays, because `TestRegister_FND038CanBeMetAndKeepsAttestation` fails without it (the test keeps the attestation half of the finding on record). The plan's grounding did not see that test. Register test `ok`, `check-ci-verdicts.sh` PASS. Open count 168 → 167. |
| 2026-11-15 | 17-27 | E-01: FND-043 `resolved` added (register test `ok`). E-02: Step 24 is attended, not run; Steps 25-26 blocked on it. |
| 2026-11-15 | 1-16 | Phase 0. Decisions copied; baseline green; findings report before written to the host (`var/compliance/findings-s-evidence-runs-before.md`). The worktree has no `var/`, so the report reads the main checkout's `var/compliance/evidence.json` through `-evidence`. |

## E-row outcomes

- E-01: **closed.** FND-043 has a `resolved` block dated 2026-11-15. Nine refusal tests pass under `-race`; `configs/wotan.yaml` reads `unlisted: deny`, `mode: enforce`; live refusals over 24 h read 0 on west. Wotan was recreated at 02:00:35Z today (S-WATCH-TRUE Phase 5), so the refusal series start at zero for every listed caller.
- E-02: **open, waits on Stevie** (Step 24, attended root rerun of `tests/hopchecks` at HEAD). The pre-check differs from the plan's grounding: one commit since `60c4bb23` touches `ebpf/` (`33fb3754`, S-WATCH-TRUE, `latency-probe` only; nothing under `ebpf/hop-ebpf`, `crates/` or `tests/hopchecks`). The rerun at HEAD covers it. Steps 25-26 are `[BLOCKED]` on Step 24.
- E-03: **accepted** per DE6. FND-009 carries `accepted` (2026-11-13 until 2027-02-11). The alternate baseline still holds WireGuard alone and `check-compose-hardening.sh` passes (41 of 42 services hardened, 1 baselined).
- E-04: **removed.** `host-probe:west:alert-route` and `host-probe:west:db-backup-recent` both pass (newest record 2026-11-15T01:29:09Z) and are out of the register (DE2).
- E-05: **read; all six rows stay open with a dated cause.**
  - `dashboard`, `wotan`, `well`, `log-shipping`: class (b). The bad samples are 2026-10-22 14:00 to 2026-10-24 03:30 UTC (Wotan, The Well) and to 2026-10-25 20:00 (west's dashboard-backend, Vector): west's kingdom stack was down while the monitoring stack ran. One postmortem for the shared window, `docs/incidents/2026-10-22-slo-kingdom-down-monitoring-up.md`, linked from each row. Re-read on 2026-11-25.
  - `well-backups`: class (b), same postmortem, plus a missed nightly backup on 2026-10-21 (newest backup 37.9 h old at 12:00, limit 26 h) and absent timestamps after the 2026-10-27 reboot.
  - `ci-develop`: class (c) by construction. `unheaded-ci-status.timer` is not installed on west (0 timers listed), 7,682 samples all 0. Row says so; installing the timer is E-10, not this plan.
  - No SLI or budget rule was wrong, so Step 42 changed no rule. Budgets on 2026-11-15 (Prometheus): dashboard -3,405 %, wotan -1,655 %, well -3,078 %, well-backups -1,894 %, log-shipping -3,048 %, ci-develop -9,900 %.
- E-06: **code half done; closes on Stevie's push.** Passing rows removed under DE2 (`git-signatures:develop`, `ruff + bandit`, `sbom`). The two CI rows are triaged with their cause and fix (`Go Vulnerability Check`: GO-2026-6859, `1c90b9ed`; `Secret Scan (gitleaks)`: `ec11410b`); `gosec` and `Gates can actually fail` carry dated lines (`1dcae498`, `4991fab4`); `check-service-auth.sh` no longer false-fails on logstore-writer and stays DECLARED until both hosts are read (FND-037). Each `github-job` row closes on the next push's run (Steps 87-89). `check-python-audit` is still red (soupsieve).
- E-07: **paper half done; the fixes wait on the host windows.**
  - `host-probe:east:dot-files` triaged (low, fileperm, Step 228).
  - DE5b: 9 west rows accepted 2026-11-13 until 2027-02-11 (`mount-home-{nodev,nosuid,separate}`, `mount-var-log-{noexec,separate}`, `mount-var-log-audit-{noexec,separate}`, `mount-var-tmp-{noexec,separate}`).
  - DE5a: dated plan lines name the step on `dot-files` and `root-path` (west, Step 248), the ftp and telnet rows (east 228-229, west 248-249), `/dev/shm` noexec (east 230-232, west 249), east `/tmp` noexec (230-232).
  - DE5c: `var-log-perms` on both hosts points at `var-log-modes` (built: `0672c82d`, in `lab-audited` and `lab-hardened`; west Step 247). **The plan has no numbered step that applies `var-log-modes` on east**; the row says "the Phase 9 window".
  - Six west rows (`mount-var-log*`, `mount-var-tmp` nodev and nosuid) say "clears with B-18 (S-E1-LIVE 292-295)".

## Enforcing readiness: who owns what (Step 158)

West `ready: no (25)`, east `ready: no (15)` on 2026-11-15 (`findings -enforce-ready`). Rows by class, west / east: sshd 8 / 5, fileperm 4 / 2, service 4 / 3, sysctl 3 / 0, firewall 2 / 2, sudo 2 / 2, shell 2 / 1.

| Class | Owner (where it gets fixed) |
|---|---|
| firewall | B-06 (east), B-07 (west, with FND-004 and the two forwarding sysctls) |
| sudo | A-43 |
| sshd | B-08 |
| fileperm, shell | this plan's DE5a and DE5c rows (Phases 9 and 10); B-18 for umask |
| service | D-21 (auditd rules), B-10 (FND-026), B-11 (FND-036, east baseline-report), A-49 (FND-010) |
| sysctl `accept_ra` | A-53 |

## Notes for the host windows

- **Step 186: `compliance/hosts/west.yaml` moves to `profile: lab-audited` only in Phase 10**, after `runbooks/security/compliance-profile-switch.yaml` (`compliance-evidence profile set -host west -profile lab-audited -reason "..."`). The profile exists in the repo now; nothing binds west to it. Do not move it early.
- **Before `aide-install` on a host:** the `unheaded-aide` package (the binary and the `unheaded-aide` account); the fix writes the unit, not the binary or the account.
- **`sudo setfacl -m u:unheaded-aide:rwx /var/lib/unheaded/textfile`**, as `unheaded-baseline` has: without it every weekly check exits 1 on the metrics write.
- **The database is built attended:** `sudo unheaded-aide init`, read the summary (entries, bytes an entry, largest directories), then `sudo unheaded-aide promote`. Measure the first run against rule 6's bar (wall ≤ 45 min, read bytes ≤ 1.25 × the include set, database ≤ 256 MiB and ≤ 1 KiB a file); a second clean run must notify nothing.
- **`var-log-modes` on east has no numbered step** (Step 247 is west's); Phase 9 needs the same line.
- **The evidence collector has no unit or timer** on west; it runs by hand as govan. `ComplianceEvidenceSilent` fires six hours after the last hand run.
- After the windows, each host's `unit/unheaded-aide-check` log source entry becomes a `west/` or `east/` entry (one line a week, `max_silence` 8d).

## Blocked

- Phase 9 (east host window) and Steps 145-149 (east's `host-service` entries, FND-037's `resolved` block): S-E1-LIVE Step 218.
- Phase 10 (west host window): attended, after Phase 9.
- Phase 11's soak: needs a sealed snapshot, which needs Phase 8's key (Step 195 on, Stevie's) and east's snapshot.
- Steps 25-26 (FND-025's `resolved` block): Step 24.
- Steps 87-89 (the pushed CI runs): Stevie's push.
- Steps 110-112 (the notice text): A-25.

## For Stevie

- **Step 24 (E-02, FND-025):** run `tests/hopchecks` as root at HEAD (command from FND-033's plan text and `tests/hopchecks/README*`). 46 cases passed on 2026-11-09. A pass lets Step 26 add FND-025's `resolved` block.

- **A stop on purpose spends error budget** whenever the monitoring stack is left up (it is, by design, since `7f6b1101`). Mark a planned stop so the budget rule skips it, or keep counting it? Until that is decided the five SLO rows will read spent after every long stop.
- **VictoriaMetrics lacks SLO history** for 2026-10-22 14:00 to 2026-10-25 20:00; Prometheus has it. Any reading of a 28-day window goes to Prometheus until about 2026-11-25.
- The postmortem `docs/incidents/2026-10-22-slo-kingdom-down-monitoring-up.md` is a draft for your review. Why the stack was down that week is not recoverable: west's journal starts 2026-10-26.
- **East has no numbered step for `var-log-modes`** (DE5c). Step 247 applies it on west; Phase 9 needs the same line for east.
- **FND-038 and the "remove" list (Step 32):** the report will keep listing `FND-038` as "remove from the register" while `pkg/compliance/crosswalk/register_text_test.go` requires the entry. Either the entry and that test go together, or the plan's target ("an empty remove list") reads "empty except FND-038". Left as it is.

- **soupsieve, the last red gate (`check-python-audit`):** `sbom/python-pip-freeze.txt` is a freeze of `~/.venv/zhen`. `~/.venv/zhen/bin/pip install soupsieve==4.0.1`, then the freeze again; 4.0.1 fixes all three CVEs. A major bump in the live Zhen venv, so it waits for your yes.
- **Steps 87-89 wait on your push:** the `Go Vulnerability Check`, `Secret Scan (gitleaks)`, `gosec` and `Gates can actually fail` rows close on the pushed runs. The Prometheus alert rules job ran the same alert-path script and was red on the 11-11 push too.
- **A-25 (Steps 110-112):** the export notice text; with it, whether the constructions nothing calls are deleted.
- **Private GHCR images:** the API gives a repository's creation date, not when it went public; whether any container image is private is yours to check in the GitHub UI.
- **Phase 8 from Step 195:** create `unheaded-compliance-seal` (uid 16765) and its ML-DSA-65 seed on west, unseen; the public half goes to `configs/keys/compliance-seal.pub`; then the monthly reseal.
- **The host windows (Phases 9, 10):** see "Notes for the host windows" above.
- **Go services on the hosts still run a 1.27.3 build** (GO-2026-6859, an HTTP/1 server crash in `net/http`): the pins are 1.27.4 in the repo; the running binaries change only when rebuilt and redeployed.
- **The runbook manifest is unsealed** again (93 runbooks: two AIDE runbooks, and the host-bootstrap runbook names the Go version).
- **`AIDEChangeDetected` is bounded to 24 h** after the run that first saw the diff (the plan wrote a bare `== 1`). Say so if you want it to stay up until the review instead; the finding `aide-unreviewed` already does that.
- **`check-crypto-inventory.sh` is red on develop** until this branch merges: S-ZHEN-FRONT moved a TLS config line in `cmd/zhen-ui-front`. `b57230ef` regenerates the inventory.

## Stuck

Nothing.
