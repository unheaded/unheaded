<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-EVIDENCE-RUNS BATTLE PLAN: 12 Phases, 236 Steps

**Date**: 2026-11-12
**Sprint**: S-EVIDENCE-RUNS. Compliance reads what runs on the hosts. The findings register and the crypto inventory match the evidence. Enforcing mode gets a measured path to its flip. Backlog sweep bundle 4: rows D-20, D-21, D-22 and D-34, settling E-01 to E-07 on the way (`docs/sweeps/BACKLOG-SWEEP-2026-11-12.md`).
**Prerequisite**: develop at `59a8fbd2` or later. Stevie answers DE1-DE7 at Phase 0. Phases 1-7 are code and register work on west. Phase 9 (east window) needs S-E1-LIVE Evening 10B done: Step 218, its exit gate, which is sweep row B-11. Phase 10 (west window) needs Phase 9.
**Target**:
- Every E-01 to E-07 row is either closed against evidence that can be rerun, or carries a dated line saying why it stays open.
- `compliance-evidence findings` shows 0 untriaged rows and an empty "remove from the register" list.
- UH-AUTHN-01 reads the running services on west, and on east once B-11 is done. When both hosts are read, FND-037 closes.
- The CBOM inventories AEADs, KDFs, hashes and the hand-written SHA-256.
- AIDE and auditd rules are journaled on both hosts.
- Both hosts run the 7-day soak on a sealed snapshot, with a readiness report naming what still blocks the enforcing flip. The flip itself is not in this plan.
**Estimated duration**: 14-18 h over 4-5 sessions plus the 7-day soak. Phases 1-7 take about 9 h unattended. Phases 8-10 are attended windows of about 1.5 h each. Phase 11 runs on days 1-7 of the soak.
**Agent strategy**: Coordinator for Phases 0, 1, 8, 9, 10 and 11. Phases 2, 3, 4, 5, 6 and 7 are code, independent of each other, and can run as `[P]` agents in their own worktrees, merged one at a time into `sprint/evidence-runs`. Each of those phases touches `compliance/findings/register.yaml` only in its final record step, so register edits never run in parallel.
**Commit cadence**: every 5 steps, at every exit gate and before every skip. Commits are signed. If gpg-agent has timed out and Stevie is away, use `--no-gpg-sign` and log it for `scripts/resign-unpushed.py`. Stage named files only, never `git add -A`. Stevie pushes.
**Stuck protocol**: skip after 3× the step estimate or two failed debug attempts. Mark the step `[STUCK]` and its dependents `[BLOCKED]`, commit, and move to the next independent phase.

## Grounding (checked 2026-11-12 ~23:10Z against develop `59a8fbd2`, read-only)

Sources read:
- `compliance/findings/register.yaml`: FND-009, -025, -033, -037, -043, and every E row.
- The live findings report: `go run ./cmd/compliance-evidence findings`, from evidence collected 22:23Z.
- ADR-097, ADR-098 (§2 and §3), ADR-099 (progress through 2026-11-03) and ADR-127.
- The DP-09, DP-10 and DP-25 answers in `DECISION-PACK-2026-11-03.md`.
- S-HOST-TIDY and its progress file, and S-E1-LIVE Steps 206-219 and 289-306.
- `docs/compliance/enforcing-soak.md` and `tomb/lich/LICH-017-baseline-agent/`.
- `docs/legal/EXPORT-CRYPTO-AUDIT-2026-11-12.md`, `compliance/crypto/cbom.cdx.json` and `cmd/crypto-inventory/`.
- On west, read-only: `systemctl`, `/var/lib/unheaded/baseline/`, and VictoriaMetrics SLI history.
- Public check-run annotations for the 2026-11-11 CI run.

East was not contacted.

| Row | Sweep says | Found at `59a8fbd2` | Plan does |
|---|---|---|---|
| D-20 | Add the host-side evidence kind; east must be redeployed first | Holds. `check-service-auth.sh` **fails today** (exit 1, "overdue"): `logstore-writer` (S-E1-LIVE Step 277) has no `AUTH_ENABLED`, but it checks its own bearer token (`LOGSTORE_INGEST_TOKEN`), so the gate reads a closed service as open. Eight catalog controls carry `declared:`; FND-037 names UH-AUTHN-01. | Phase 2 teaches the gate token-auth services. Phase 5 builds `host-service` for UH-AUTHN-01: west now, east after B-11. The other seven declared controls are listed in Appendix C, not planned. |
| D-21 | AIDE on both hosts, auditd rules on west, east's lab fixes journaled | East's journal is **13 of 15**: only `aide-install` and `apparmor-enforce` are not applied (`host-journal:east` detail). West has `auditd` installed (dpkg `ii`); only the rules are missing (1 rule line). The register plan for AIDE says to exclude `/var/lib/docker`, LXD storage and build trees, but **the `aide-install` fix has no exclude action**. **AIDE has no third-party approval**: DP-09's batch named it, but the answer line names only the journald caps, the ROCm chmod and the `remediate.sh` retirement. Moving west to `lab-hardened` also turns on `sshd-safe` (risk high, B-08's evening) and `apparmor-enforce`. | DE1 (AIDE), DE4 (west profile), DE7 (east AppArmor). Phase 7 adds the excludes. Phases 9 and 10 are the host windows. |
| D-22 | Build enforcing: typed actions, breaker, preinst gate, soak, Lich campaign | **The code is built** (`836a52ba`, S-WELL-SPRING Steps 276-279): enforce path, flapping breaker, preinst gate, soak clock, soak doc, LICH-017 harness (green today, 0.6 s). What is missing: no host has a sealed snapshot (`/etc/unheaded/compliance/` is absent on west), so the soak clock never starts (LICH-017 case T6). **Nobody owns the seal key**: `compliance-evidence snapshot -key SEED` names no custody, and the TTL is 7 d by default, 31 d at most. ADR-098 condition 3 (zero open findings in enforceable classes) is blocked by the firewall rows (B-06, B-07), the sudo rows (A-43) and the sshd rows (B-08). West's baseline unit has no `--textfile-dir`, so the soak metrics are not exported (S-WATCH-TRUE Phase 7 adds it). | DE3 (seal custody). Phase 6 (readiness report), Phase 8 (key and snapshots), the live LICH-017 drill in Phase 9, the soak in Phase 11. The flip stays Stevie's: condition 4, `compliance/hosts/<host>.yaml`. |
| D-34 | CBOM lacks ChaCha20-Poly1305, HKDF, AES-GCM, x25519-dalek, BLAKE3 | The gap is a **whole class**: `cmd/crypto-inventory` knows only signature, KEM, key-agreement, TLS, SSH and GODEBUG kinds. It also misses SHA-3 (`age.go`, `envelope.go`). Rust crypto crates are seen only through rustls/pqcrypto lock entries. | Phase 4 adds the `aead`, `kdf`, `hash` and `own-primitive` kinds for Go and Rust, plus a SHA-256 round-constant tripwire. The notice text is `[BLOCKED]` on A-25. |
| E-01 | FND-043 fixed, no `resolved` block | Holds. `24025056` and `90e45458` exist. Tests `TestPermissions_GRPCStreamAndPublish`, `TestShippedAllowlistCoversDeployedSubscribers` and `TestEveryWotanClientIdentityIsAllowedAndApproved` exist. `configs/wotan.yaml` has `mode: enforce`, `unlisted: deny`. | Phase 1: run the tests, read the live refusal counter, add the `resolved` block. |
| E-02 | FND-025 closes on a root run at HEAD | The proof run is `60c4bb23` (2026-11-09 23:31Z). Since then nothing under `ebpf/`, `crates/` or `tests/hopchecks` has changed. | Phase 1: an attended root rerun at HEAD, then `resolved`. |
| E-03 | FND-009 open by design or awaiting acceptance | Holds: the alternate baseline holds WireGuard alone. Its verify ("baseline-alt empty") can never pass while WireGuard needs root. | DE6 |
| E-04 | alert-route, db-backup-recent should clear | **Both pass** (22:23Z): the findings report lists both under "remove from the register". | Phase 1, under DE2 |
| E-05 | Six SLO rows: re-read after the TLS rollout | The planned stop (10-28 to 11-10) does **not** explain the spent budgets. `avg_over_time` skips absent samples, and the daily SLI since 11-10 reads 0.98-1.0, yet `slo:error_budget_remaining` reads -2126 % to -4345 %. That needs reading, not waiting. `ci-develop` reads 0 because `unheaded_ci_run_checked_timestamp_seconds` has no producer: the repo has `unheaded-ci-status.{service,timer}`, but west runs no such timer (sweep E-10, M-087). | Phase 1 reads each SLI's bad samples by time and writes the cause on each row. A broken SLI is fixed as code; a real outage gets a postmortem. |
| E-06 | CI rows close on the next green run | `git-signatures:develop`, `ruff + bandit` and `sbom` **pass** (remove). `gosec` and `Gates can actually fail` are **red again** on the 2026-11-11 push run. **Two new untriaged rows** from that run: `Go Vulnerability Check` (net/http stdlib call sites) and `Secret Scan (gitleaks)`. `check-service-auth.sh` fails on logstore-writer (D-20 above). | Phase 3 reproduces each in a clean clone, fixes it on develop and records it. The green run waits on Stevie's push. |
| E-07 | 26 host rows, no plan step | DP-25 rows: 25.3 is `/var` nodev+nosuid, and 25.4 is west `/tmp` noexec; both are in B-18 (S-E1-LIVE 292-295). 25.11 accepted east's separate mounts (15 rows, until 2027-02-02). Nothing ruled on: `/dev/shm` noexec (both hosts), east `/tmp` noexec, west's `/home` options and the `/var` children's noexec and separate mounts. Once B-18 sets nodev+nosuid on `/var`, six west rows (`var-log*`, `var-tmp` nodev+nosuid) clear, because the probe reads the containing mount. **New:** `host-probe:east:dot-files` fails and is **untriaged** (east `.gitconfig` 0660). | DE5a, b, c. Phases 9 and 10 apply the fixes. Phase 1 writes the acceptances Stevie gives. |

**MoatGhost (compliance).**
- No severity is moved by this plan, and the ratchet stays rule-ID exclusion.
- A register row closes only on evidence that can be rerun (FND-039): a `resolved` block names a check, and a source row is removed only when its source passes.
- The new `host-service` kind reads container and unit environments. Those hold the passwords, so the collector parses only `AUTH_ENABLED` in-process. No other value enters a record, a log line or an error, and Phase 5 has a test for exactly that.
- AIDE is a new distro package on both hosts: DE1.
- Lab-only gaps (separate filesystems, noexec on `/var` children) go to Stevie as acceptances with a "revisit before real data or internet exposure" trigger (feedback 2026-10-15), not as remediation work.
- Adding host-service sources moves UH-AUTHN-01's framework figures. That is the point of FND-037, and its plan carries Stevie's 2026-11-06 word.

**Marshal (scope).** In scope: D-20, D-21, D-22 (up to the soak and readiness, not the flip), D-34 (inventory, not the notice), E-01 to E-07, plus the three untriaged rows (they are E-06's and E-07's kin).

Parked, with their owners:
- The other seven DECLARED controls → Appendix C.
- The B-18 remounts and fstab → S-E1-LIVE 289-306.
- The firewall and sudo rows that block condition 3 → B-06, B-07, A-43.
- The ci-status timer → E-10, recorded on the `slo:ci-develop` row.
- Deleting the two caller-less encryption constructions → A-25.
- Expiring acceptances → A-50.

No new Well schema: DE4's recommendation is a profile, not per-host `off` lists, which would need a migration.

**Busboy (handoffs).**
- **S-WATCH-TRUE** adds FND-044 to the register and `--textfile-dir` to the baseline unit (its Phase 7). This plan reads the soak through `soak.json` and the journal when the metrics are absent, and takes no FND id. Its register edits are rows and blocks, never a new FND, so the two plans cannot collide on numbering.
- **S-RESIDUE** edits `test_ioc_audit.py`, ADR-INDEX and the register's D-26 rows. This plan's register edits are E-rows only. Rebase on develop before each register commit.
- **S-ZHEN-FRONT** gives dashboard-backend a Well role in `docker-compose.yml`. This plan does not edit compose. Phase 2 edits only `scripts/compliance/check-service-auth.sh` and its test.
- **S-E1-LIVE** owns east's redeploy (206-219) and the B-18 mounts (289-306). Phase 9 waits on Step 218, and Phase 10's `/dev/shm` step runs before or after Evening 17, never during it.

**Micromanager (QA).**
- Every code phase is TDD: a red test, then green with `-race`.
- Every host change has a plan or dry run read first, a `[V]` and a rollback.
- Register edits are validated by `go test ./pkg/compliance/crosswalk/`, and each closes against a rerun of the findings report.
- Definition of done: the findings report's untriaged and remove lists are empty, and every E row has an outcome line in the progress file.

---

## Decisions for Stevie (Phase 0 asks these in rounds of four)

| # | Question | Recommendation | Without an answer |
|---|---|---|---|
| DE1 | Install AIDE (Ubuntu `aide`, no recommends) on both hosts: weekly check, excludes for Docker, LXD pools and build trees. It is a new distro package, and DP-09's answer did not name it. | **Yes.** It is SI-7 file integrity and is already in the `lab-hardened` model. The excludes keep the weekly report readable. | Steps on `aide-install` are `[BLOCKED]`; the four AIDE rows stay open |
| DE2 | Remove every register row the findings report lists as passing (`remove from the register`), as the register header prescribes. Today that is 7 rows; more will come during the plan. Sweep row A-51. | **Yes, all of them.** Each removal names its source and the observation time in the commit body. | Rows stay, with a dated "passes since" note |
| DE3 | Who seals the compliance snapshots a host's root acts on? | **A dedicated ML-DSA-65 key on west** under a nologin account `unheaded-compliance-seal` (DP-06 daemon-key rule), with the public half in the repo, a 31-day TTL and a monthly reseal at your hands. The alternative is the B-10 build key at a ceremony each month. | Phase 8 and the soak are `[BLOCKED]` |
| DE4 | West's profile. `lab-hardened` also turns on `sshd-safe` (risk high) and `apparmor-enforce`. | **New profile `lab-audited`** = `lab` + `auditd-rules` + `aide-install` + `rocm-log-perms`. West moves to `lab-hardened` at B-08 (sshd) and after B-18 (the X server purge). | `auditd-rules` on west is `[BLOCKED]` |
| DE5a | Fix in the host windows: `dot-files` (both hosts; chmod g-w on govan's own files), west `root-path` (remove the empty `/root/.vector`), purge the ftp and telnet clients (both), trial `/dev/shm` noexec (both) and east `/tmp` noexec. | **Yes to all.** Each has a dry run and a rollback. | Those rows stay open |
| DE5b | Accept 90 days, "revisit before real data or internet exposure": west `home-{nodev,nosuid,separate}`, `var-log-{noexec,separate}`, `var-log-audit-{noexec,separate}`, `var-tmp-{noexec,separate}` (9 rows). `/home` lives on `/`, and `/var` holds Docker, so noexec there breaks it. This is the west twin of DP-25.11. | **Accept.** | They stay open and overdue |
| DE5c | `var-log-perms` (west 235 files, east 66, Ubuntu's 0644 defaults): a `fileperm` fix in the model (enforceable, so it counts for condition 3; rotation recreates the modes and enforcing re-fixes them), or accept. | **Fix** (`var-log-modes`) | Stays open; condition 3 stays blocked by it |
| DE6 | FND-009, WireGuard root by necessity: accept for 90 days (cap_drop ALL + NET_ADMIN, read-only, deployed on neither host), or rewrite its verify to "WireGuard is the only entry". | **Accept**, reason as written | Row stays open with no next action |
| DE7 | East: apply `apparmor-enforce` (the Xorg profile from complain to enforce; no Xorg runs on east) through the executor, so east's journal reads 15 of 15. | **Yes** | `host-journal:east` stays failing |

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; it must pass before you go on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or create a file (Write/Edit tool, never a heredoc for shell-bearing text)
- `[R]`: read or inspect
- `[S]`: needs sudo
- `[P]`: can run in parallel with other `[P]` phases
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first (host window, decision, push)
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. `R=/home/govan/tmp/unheaded`, `PROG=$R/docs/battle-plans/progress/S-EVIDENCE-RUNS-progress.md`, `SCR=/tmp/claude-1000/s-evidence-runs` (create it; a scratch area that is not tracked). Collected host data (findings reports, evidence trials) goes under `$R/var/compliance/` (gitignored), never into a tracked file. The progress file holds counts and verdicts only.

---

## PHASE 0: DECISIONS AND PRECONDITIONS (Steps 1-16)

**Goal**: DE1-DE7 answered and recorded; branch, tools and a green baseline.
**Prerequisite**: none.
**Time**: 30 min plus Stevie.
**Agent**: Coordinator.

- [ ] **Step 1** [R] ~10m: Read the Grounding sources above. As a minimum: ADR-098 §2, `docs/compliance/enforcing-soak.md`, FND-037's entry, and this plan's Decisions table.
- [ ] **Step 2** [STEVIE] ~5m: First AskUserQuestion round: DE1, DE2, DE3, DE4.
- [ ] **Step 3** [STEVIE] ~5m: Second round: DE5a, DE5b, DE5c, DE6. Third round: DE7.
- [ ] **Step 4** [W] ~5m: Create `$PROG` with the sections Decisions (answers verbatim, dated), Step log, E-row outcomes (one line each for E-01 to E-07, empty), and Stuck.
- [ ] **Step 5** [V] ~1m: Every DE has an answer, or is "not yet" with the steps it blocks marked `[BLOCKED]` in `$PROG`.
- [ ] **Step 6** [B] ~1m: Create the branch and the scratch area.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git switch -c sprint/evidence-runs && mkdir -p /tmp/claude-1000/s-evidence-runs
  ```
- [ ] **Step 7** [B] ~1m: Check the tools. Don't run anything that isn't installed.
  ```bash
  for t in go jq curl ~/go/bin/govulncheck ~/go/bin/gosec ~/.local/bin/gitleaks; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done
  go version; grep -E '^(go|toolchain) ' go.mod
  ```
- [ ] **Step 8** [V] ~1m: All ok. If a tool is missing, its phase is `[BLOCKED]`; ask before installing anything new.
- [ ] **Step 9** [B] ~6m: Baseline the packages this plan touches.
  ```bash
  cd /home/govan/tmp/unheaded && go test -race -count=1 ./pkg/compliance/... ./cmd/compliance-evidence/... ./cmd/crypto-inventory/... ./cmd/unheaded-baseline/... ./cmd/unheaded-remediate/... ./tomb/lich/LICH-017-baseline-agent/... 2>&1 | tail -15
  ./scripts/check-crypto-inventory.sh 2>&1 | tail -3
  ```
- [ ] **Step 10** [V] ~1m: All `ok`, and the inventory check passes. If anything is red, record it in `$PROG` and fix it before going on: this sprint must not inherit red.
- [ ] **Step 11** [B] ~2m: Today's findings report, kept on the host.
  ```bash
  go run ./cmd/compliance-evidence findings -out var/compliance/findings-s-evidence-runs-before.md | head -1
  grep -cE '^\| UNTRIAGED' var/compliance/findings-s-evidence-runs-before.md; grep -c 'remove from the register' var/compliance/findings-s-evidence-runs-before.md
  ```
- [ ] **Step 12** [V] ~1m: Expect about 167 open, 3 untriaged (govulncheck, gitleaks, `east:dot-files`) and 7 "remove" lines. Write the three counts into `$PROG`. A different number is not a failure; note what changed.
- [ ] **Step 13** [B] ~1m: State of the sibling plans this one waits on.
  ```bash
  grep -nE 'Step 218|EVENING 10B EXIT' docs/battle-plans/progress/S-E1-LIVE-progress.md | tail -3
  grep -nE 'Phase 7|textfile' docs/battle-plans/progress/S-WATCH-TRUE-progress.md 2>/dev/null | tail -3 || echo "S-WATCH-TRUE not started"
  systemctl cat unheaded-baseline.service | grep -c 'textfile-dir'
  ```
- [ ] **Step 14** [V] ~1m: Record it in `$PROG`. If Step 218 has not passed, Phase 9 and Steps 145-149 are `[BLOCKED]`. If `textfile-dir` is 0, Phase 11 reads the soak from `soak.json` and the journal.
- [ ] **Step 15** [C] ~1m: Commit `$PROG`: `[PLAN S-EVIDENCE-RUNS] Steps 1-15: decisions + baseline`.
- [ ] **Step 16** [V] ~1m: **PHASE 0 EXIT GATE**: decisions are recorded, the baseline is green, and the blocked phases are marked.

---

## PHASE 1: THE REGISTER READS THE EVIDENCE (E-01 to E-07) (Steps 17-50)

**Goal**: Every E row is closed on rerunnable evidence or carries a dated reason; `east:dot-files` is triaged.
**Prerequisite**: Phase 0.
**Time**: 2.5 h (plus 15 min attended for Step 24).
**Agent**: Coordinator (register edits are serial).

### E-01: FND-043

- [ ] **Step 17** [B] ~3m: The refusal tests.
  ```bash
  go test -race -count=1 ./services/wotan/... -run 'TestPermissions|TestShippedAllowlistCoversDeployedSubscribers|TestEveryWotanClientIdentityIsAllowedAndApproved' 2>&1 | tail -5
  ```
- [ ] **Step 18** [V] ~1m: All `ok`. Fail → stop: FND-043 is not closed. Record it in `$PROG`, leave the row open, and go to Step 22.
- [ ] **Step 19** [B] ~2m: The live state: config mode and refusals over the last day.
  ```bash
  grep -nE '^\s+(mode|unlisted):' configs/wotan.yaml
  curl -s 'http://127.0.0.1:8428/api/v1/query?query=sum(increase(unheaded_wotan_topic_permission_refusals_total[24h]))' | jq -r '.data.result[0].value[1] // "absent"'
  ```
- [ ] **Step 20** [V] ~1m: `mode: enforce`, `unlisted: deny`, and refusals `0`. Above 0 → Step 21. `absent` → note it and go on: the counter only appears after the first refusal, and the tests carry the proof.
- [ ] **Step 21** [D] ~5m: Read Wotan's log for the refused identity and topic: `sudo -n docker logs --since 24h unheaded-wotan 2>&1 | grep -i 'permission' | tail -5`. A deployed service being refused is a live gap, the same class as Sophia's on 2026-11-12. Record it and leave FND-043 open. The alert for this is S-WATCH-TRUE Phase 5.
- [ ] **Step 22** [W] ~5m: If 18 and 20 passed, add to FND-043:
  ```yaml
      resolved:
        on: <today>
        evidence: >-
          24025056 enforces topics.permissions (unlisted: deny, mode: enforce); 90e45458 closed the first list's gaps
          (sophia knowledge.shared, gateway and upc-cartridgectl approved). Rerunnable: go test ./services/wotan/...
          -run 'TestPermissions|TestShippedAllowlistCoversDeployedSubscribers|TestEveryWotanClientIdentityIsAllowedAndApproved';
          live refusals over 24 h read <n> on <date>
  ```
- [ ] **Step 23** [B] ~1m: The register still loads.
  ```bash
  go test -count=1 ./pkg/compliance/crosswalk/ 2>&1 | tail -2
  ```

### E-02: FND-025

- [ ] **Step 24** [STEVIE][S] ~10m: Run tests/hopchecks as root at HEAD. Its verify asks for that. Nothing it covers has changed since `60c4bb23`; check that first, so the rerun proves exactly that tree.
  ```bash
  git log --oneline 60c4bb23..HEAD -- ebpf/ crates/ tests/hopchecks | wc -l
  ```
  For the rerun itself, read the command from FND-033's plan text and `tests/hopchecks/README*`, then run that as root. Don't improvise flags.
- [ ] **Step 25** [V] ~1m: Every case passes on both objects (46 on 2026-11-09). Fail → FND-025 stays open; record the failing case in `$PROG`.
- [ ] **Step 26** [W] ~3m: If it passed, add `resolved` to FND-025, with the evidence: date, HEAD hash, kernel, pass count, the command, and "what stays is FND-031's".
- [ ] **Step 27** [C] ~1m: Commit `compliance/findings/register.yaml $PROG`: `[PLAN S-EVIDENCE-RUNS] Steps 17-27: FND-043 and FND-025 closed on rerunnable evidence`.

### E-04 and E-06 (passing half): removals under DE2

- [ ] **Step 28** [B] ~2m: The current "remove" list, with each source's newest verdict and when it was observed.
  ```bash
  go run ./cmd/compliance-evidence findings | grep 'remove from the register' | sed 's/.*`\(.*\)`.*/\1/' > $SCR/remove.txt; cat $SCR/remove.txt
  python3 -I -c 'import json,sys; r=json.load(open(sys.argv[1]))["records"]; w=set(open(sys.argv[2]).read().split("\n"))-{""}; best={}
  for x in r:
      k=x["source"]["kind"]+":"+x["source"]["ref"]
      if k in w and (k not in best or x["observed_at"]>best[k]["observed_at"]): best[k]=x
  [print(k, best[k]["verdict"], best[k]["observed_at"]) for k in sorted(best)]' var/compliance/evidence.json $SCR/remove.txt
  ```
- [ ] **Step 29** [V] ~1m: Every source row's **newest** record reads `pass`. Use the newest by `observed_at`, never the last in file order: that was S-HOST-TIDY Step 121's trap. FND ids with a `resolved` block (FND-032, FND-038, and FND-043/FND-025 from above) are listed by design.
- [ ] **Step 30** [W] ~10m: DE2 = yes → delete exactly the entries in `$SCR/remove.txt`, one by one, by name. DE2 = no → under each, add a plan line "<date>: passes since <observed_at>; kept at Stevie's word (DE2)".
- [ ] **Step 31** [B] ~1m: Rerun the report.
  ```bash
  go test -count=1 ./pkg/compliance/crosswalk/ 2>&1 | tail -1; go run ./cmd/compliance-evidence findings | grep -c 'remove from the register'
  ```
- [ ] **Step 32** [V] ~1m: `0` (DE2 yes) or the same count (DE2 no). Write the E-04 outcome line in `$PROG`.
- [ ] **Step 33** [C] ~1m: Commit `compliance/findings/register.yaml $PROG`. The body lists each removed source with its observation time.

### E-03: FND-009

- [ ] **Step 34** [B] ~2m: Check it is still WireGuard alone.
  ```bash
  cat docs/security/compose-hardening-baseline-alt.txt; ./scripts/check-compose-hardening.sh 2>&1 | tail -2
  ```
- [ ] **Step 35** [V] ~1m: One entry (WireGuard) and the gate passes. Anything else → a new finding; record it, don't fold it into FND-009.
- [ ] **Step 36** [W] ~3m: Following DE6, either add `accepted: {reason: "WireGuard needs root (EPERM otherwise, measured 2026-10-20); deployed on neither host", compensating: "cap_drop ALL + NET_ADMIN and the four s6 init caps, read-only, the gate keeps any other service from joining", accepted_on: <today>, until: <today+90d>}`, or rewrite `verify` as Stevie worded it. Then rerun Step 23's test.

### E-05: the six SLO rows

- [ ] **Step 37** [R] ~5m: `monitoring/prometheus/rules/slo.yml`: each `slo:good` expression and its `error_budget_remaining` divisor. Note that every `slo:good` has `or on() vector(0)`: a missing target counts as 0 while the rule evaluator runs, but nothing counts while it is down.
- [ ] **Step 38** [B] ~5m: Where the bad samples are. Use 6-hour buckets over the 28-day window, per SLO.
  ```bash
  for s in dashboard wotan well well-backups log-shipping ci-develop; do echo "== $s"
    curl -s "http://127.0.0.1:8428/api/v1/query_range?query=avg_over_time(slo:good%7Bslo=%22$s%22%7D%5B6h%5D)&start=$(date -u -d '28 days ago' +%s)&end=$(date -u +%s)&step=6h" \
    | jq -r '.data.result[0].values[]? | select((.[1]|tonumber) < 0.99) | "\(.[0]|todate) \(.[1])"' | head -20
  done > $SCR/slo-bad.txt; wc -l $SCR/slo-bad.txt
  ```
- [ ] **Step 39** [B] ~3m: The budget arithmetic for one SLO, checked by hand against Step 38. Compute 28-day average good, then `1 - (1 - avg) / (1 - objective)`.
  ```bash
  curl -s 'http://127.0.0.1:8428/api/v1/query?query=avg_over_time(slo:good%7Bslo=%22dashboard%22%7D%5B28d%5D)' | jq -r '.data.result[0].value[1]'
  curl -s 'http://127.0.0.1:8428/api/v1/query?query=slo:error_budget_remaining%7Bslo=%22dashboard%22%7D' | jq -r '.data.result[0].value[1]'
  ```
- [ ] **Step 40** [V] ~5m: Classify each SLO, one line in `$PROG`:
  - (a) Bad samples fall only inside known work: the 2026-10-26 game days, S24 restarts, the 11-10 bring-up, the 11-12 06:40Z password pass. The row stays open with "window clears <date of last bad sample + 28 d>; re-read then".
  - (b) Bad samples outside any known work → Step 41.
  - (c) The arithmetic in Step 39 disagrees with Step 38 → the SLI or budget rule is wrong → Step 42.
  - `ci-develop` is (c) by construction: no producer of `unheaded_ci_run_*` runs on west (`systemctl list-timers unheaded-ci-status.timer`).
- [ ] **Step 41** [D] ~20m: For each (b): a postmortem from `docs/incidents/TEMPLATE.md` at `docs/incidents/<date>-slo-<name>.md`. It gives the time window, what the target did (journal and container restarts in that window), the cause, and the fix or a dated "no fix: <why>". The row's plan text links it.
- [ ] **Step 42** [D] ~15m: For each (c), excluding ci-develop: a test-first fix in `monitoring/prometheus/rules/slo.yml` with a promtool case in `monitoring/prometheus/rules/tests/`. Run `./scripts/check-prometheus-rules.sh`. For `ci-develop`: write on the row "no producer on west: `unheaded-ci-status.timer` not installed (sweep E-10, M-087); the SLI reads 0 by construction until it is". Installing the timer is not in this plan.
- [ ] **Step 43** [W] ~10m: Write the Step 40 outcome into each of the six rows' `plan` as a dated line. Never edit the earlier text.
- [ ] **Step 44** [C] ~1m: Commit the register, any postmortems, any rules and tests, and `$PROG`: `[PLAN S-EVIDENCE-RUNS] Steps 34-44: FND-009 per DE6; SLO rows read`.

### E-07 (paper half) and the untriaged host row

- [ ] **Step 45** [W] ~5m: Add a source entry for the untriaged row:
  ```yaml
    - source: "host-probe:east:dot-files"
      severity: low
      class: fileperm
      opened: <today>
      lockout: none
      step: 4
      plan: "chmod g-w /home/govan/.gitconfig on east (0660 today); S-EVIDENCE-RUNS Phase 9 (DE5a)"
  ```
- [ ] **Step 46** [W] ~15m: Following DE5b, add an `accepted:` block to each accepted row, using the exact list Stevie gave: reason "lab host: /home on /, /var holds Docker and LXD; separate filesystems need a repartition", compensating "LAN and P2P only; revisit before real data or internet exposure", `until` today+90d. Rows in DE5a and DE5c get a plan line naming the Phase 9 or 10 step. The six `var-log*`/`var-tmp` nodev+nosuid rows get "clears with B-18 (S-E1-LIVE 292-295)".
- [ ] **Step 47** [B] ~1m: Validate and rerun.
  ```bash
  go test -count=1 ./pkg/compliance/crosswalk/ 2>&1 | tail -1; go run ./cmd/compliance-evidence findings | grep -cE '^\| UNTRIAGED'
  ```
- [ ] **Step 48** [V] ~1m: The test is `ok`, and UNTRIAGED is `2` (the two CI rows, which are Phase 3's).
- [ ] **Step 49** [C] ~1m: Commit `compliance/findings/register.yaml $PROG`: `[PLAN S-EVIDENCE-RUNS] Steps 45-49: east dot-files triaged; E-07 acceptances per DE5b`.
- [ ] **Step 50** [V] ~2m: **PHASE 1 EXIT GATE**: the E-01 to E-05 and E-07 outcome lines are filled in `$PROG`, and E-06 is marked "Phase 3". The register test passes.

---

## PHASE 2: THE SERVICE-AUTH GATE READS TOKEN-AUTH SERVICES (D-20 part, E-06 part) (Steps 51-62) [P]

**Goal**: `check-service-auth.sh` passes on the tree because logstore-writer is understood, not exempted blindly; a service with neither form of auth still fails.
**Prerequisite**: Phase 0.
**Time**: 45 min.
**Agent**: Agent [P] (its own worktree).

- [ ] **Step 51** [R] ~5m: `cmd/logstore-writer/main.go` from `handler()` on. Every route other than `/health`, `/ready` and `/metrics` must check `LOGSTORE_INGEST_TOKEN` before it reads the body.
- [ ] **Step 52** [V] ~1m: True → Step 53. A write route without the check → **this is a real open service**: stop. Record a finding in `$PROG` (Stevie names the id; S-WATCH-TRUE owns FND-044, so ask) and fix the writer instead.
- [ ] **Step 53** [W] ~10m: First, a test in `pkg/compliance/crosswalk/serviceauth_script_test.go`, after `TestServiceAuthCheck`'s fixture style. It covers four cases:
  - (a) A service named in the gate's token-auth table, with its token variable set in compose and no `AUTH_ENABLED`, passes.
  - (b) The same service with the token variable missing fails.
  - (c) An unnamed service with a token-looking variable fails.
  - (d) A service with neither form fails.
- [ ] **Step 54** [B] ~1m: Red.
  ```bash
  go test -count=1 ./pkg/compliance/crosswalk/ -run TestServiceAuth 2>&1 | tail -5
  ```
- [ ] **Step 55** [W] ~10m: `scripts/compliance/check-service-auth.sh`. Add a `TOKEN_AUTH` table beside `EXEMPT`:
  `{"logstore-writer": ("LOGSTORE_INGEST_TOKEN", "bearer token checked in cmd/logstore-writer/main.go handler before any write")}`.
  A listed service passes only when that variable is present in its compose environment. Print `[TOKEN] name: reason`. The header comment says a new entry needs a reason a reviewer can check in the code.
- [ ] **Step 56** [B] ~1m: Green, and the gate passes on the tree.
  ```bash
  go test -race -count=1 ./pkg/compliance/crosswalk/ 2>&1 | tail -1; ./scripts/compliance/check-service-auth.sh 2>&1 | tail -3; echo rc=$?
  ```
- [ ] **Step 57** [V] ~1m: `ok`, `[TOKEN] logstore-writer: ...`, rc=0.
- [ ] **Step 58** [B] ~3m: The gate can still fail. Read the meta-gate's row for this gate (`grep -n 'service-auth' scripts/check-gates-can-fail.sh`) and run that provocation as the script documents (`./scripts/check-gates-can-fail.sh -h`).
- [ ] **Step 59** [V] ~1m: The provocation is caught (the gate goes red on the provoked tree).
- [ ] **Step 60** [W] ~3m: Add a dated plan line to the `gate-script:scripts/compliance/check-service-auth.sh` register row: "<date>: failed since 2026-11-10 on logstore-writer read as open; the gate now knows token-auth services; still DECLARED until Phase 5 reads both hosts (FND-037)".
- [ ] **Step 61** [C] ~1m: Commit the script, its test, the register and `$PROG`: `[PLAN S-EVIDENCE-RUNS] Steps 51-61: service-auth gate knows token-auth services`.
- [ ] **Step 62** [V] ~1m: **PHASE 2 EXIT GATE**: the gate passes on the tree and fails when provoked.

---

## PHASE 3: CI REDS FROM THE 2026-11-11 PUSH (E-06 part) (Steps 63-90) [P]

**Goal**: The four red jobs (govulncheck, gitleaks, gosec, the meta-gate) are reproduced in a clean clone and green on develop; their register rows say why and when they close.
**Prerequisite**: Phase 0.
**Time**: 2-3 h.
**Agent**: Agent [P].

- [ ] **Step 63** [B] ~3m: Make a clean clone. `/var/tmp` can be hidden from a sandboxed shell, so use the worktree area.
  ```bash
  rm -rf /home/govan/tmp/wt/ci-evidence && git clone -q --no-hardlinks /home/govan/tmp/unheaded /home/govan/tmp/wt/ci-evidence && cd /home/govan/tmp/wt/ci-evidence && git switch -q sprint/evidence-runs && git log --oneline -1
  ```
- [ ] **Step 64** [B] ~5m: govulncheck, at the version CI pins (`.github/workflows/security.yml`: v1.9.0).
  ```bash
  cd /home/govan/tmp/wt/ci-evidence && ~/go/bin/govulncheck -version 2>&1 | head -2; ~/go/bin/govulncheck ./... 2>&1 | grep -E '^Vulnerability #|Fixed in|Found in' | head -30
  ```
- [ ] **Step 65** [V] ~2m: Write each GO-ID with its "Fixed in" into `$PROG`. Expect stdlib (`net/http` call sites) fixed in a later go1.27.x. A local version different from v1.9.0 → install the pinned one into `$SCR` with `GOBIN` before trusting a clean result.
- [ ] **Step 66** [B] ~3m: Every pin of the Go version, which must move together (feedback 2026-10: build pins stay in sync).
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE '1\.27\.[0-9]+' --include=go.mod --include='Dockerfile*' --include='*.yml' --include='*.yaml' --include='*.nix' --include='*.sh' . | grep -v -e '/vendor/' -e 'node_modules' | head -40
  ```
- [ ] **Step 67** [W] ~10m: Bump the `toolchain` line in `go.mod` and `tools/gosec/go.mod`, plus every pin from Step 66, to the newest go1.27.x that holds every Step 65 fix. This is Go itself, already approved, at its latest patch.
- [ ] **Step 68** [B] ~10m: Build, test and recheck.
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && go test -race -count=1 ./pkg/... 2>&1 | grep -v '^ok' | head -10; ~/go/bin/govulncheck ./... 2>&1 | tail -3
  ```
- [ ] **Step 69** [V] ~1m: The build is clean, there is no FAIL, and govulncheck reports "No vulnerabilities found".
- [ ] **Step 70** [C] ~1m: Commit the named go.mod files and pins: `[PLAN S-EVIDENCE-RUNS] Steps 63-70: go1.27.<n> for GO-<ids>`.
- [ ] **Step 71** [B] ~3m: gitleaks, the way CI runs it.
  ```bash
  cd /home/govan/tmp/wt/ci-evidence && git pull -q /home/govan/tmp/unheaded sprint/evidence-runs && ./scripts/check-gitleaks.sh 2>&1 | tail -15
  ```
- [ ] **Step 72** [V] ~1m: Passes → the CI failure came from the push's history range. Read `security.yml`'s gitleaks steps for the range it scans, rerun that range with the same command, then Step 73. Fails → Step 73.
- [ ] **Step 73** [D] ~15m: For each finding, read the file and line **without printing the matched value** (use gitleaks' `--redact`).
  - A real credential → stop, tell Stevie, and rotate on his word (B-03's class). It is never printed or committed.
  - A false positive in new code → change the code or fixture so the pattern no longer matches. The baseline (`.gitleaksignore`) is shrink-only, so nothing is added to it.
- [ ] **Step 74** [C] ~1m: Commit the fix (named files): `[PLAN S-EVIDENCE-RUNS] Steps 71-74: gitleaks green`.
- [ ] **Step 75** [B] ~5m: gosec, the way `security.yml` runs it. Read the job's step and run the same command, with the pinned build in `tools/gosec`.
- [ ] **Step 76** [V] ~1m: `Issues : 0`.
- [ ] **Step 77** [D] ~20m: For each issue, either fix it, or annotate it with the **leading** `#nosec G<id> -- <reason>` (gosec ignores `//nolint:gosec // #nosec`).
- [ ] **Step 78** [C] ~1m: Commit (named files).
- [ ] **Step 79** [B] ~25m: Run the meta-gate in the clean clone with the CI venv **last** on PATH (standing rule). Read every row.
  ```bash
  cd /home/govan/tmp/wt/ci-evidence && git pull -q /home/govan/tmp/unheaded sprint/evidence-runs && PATH="$PATH:/var/tmp/unheaded-sprint/civenv/bin" ./scripts/check-gates-can-fail.sh 2>&1 | tee $SCR/meta-gate.txt | tail -20
  ```
- [ ] **Step 80** [V] ~2m: Every gate bites, none is INCONCLUSIVE, and the wall time is under the job's 25 minutes.
- [ ] **Step 81** [D] ~20m: A failing row → fix its cause. A time overrun → the cause from the "Gates can actually fail" row's history: reduce the slow provocation, or raise `timeout-minutes` with the measured time.
- [ ] **Step 82** [C] ~1m: Commit (named files).
- [ ] **Step 83** [W] ~10m: Register:
  - Add source entries for `github-job:Security Scan (Daily + PR)/Go Vulnerability Check` and `.../Secret Scan (gitleaks)`, each with severity, class `ci`, the cause found, the fix commit, and "closes on the next push's run".
  - Add a dated line to the `gosec` and `Gates can actually fail` rows.
- [ ] **Step 84** [B] ~1m: The register test, then the untriaged count.
  ```bash
  go test -count=1 ./pkg/compliance/crosswalk/ 2>&1 | tail -1; go run ./cmd/compliance-evidence findings | grep -cE '^\| UNTRIAGED'
  ```
- [ ] **Step 85** [V] ~1m: `ok` and `0`.
- [ ] **Step 86** [C] ~1m: Commit `compliance/findings/register.yaml $PROG`.
- [ ] **Step 87** [STEVIE] ~?: Push develop after the merge in Step 233. CI's main workflow stays off until B-02 (FND-027). The security and static-analysis workflows ran on 2026-11-11, so they run on push.
- [ ] **Step 88** [S] ~5m: After that run, collect.
  ```bash
  sudo systemctl start unheaded-compliance-evidence.service && go run ./cmd/compliance-evidence findings | grep -E 'github-job' | cut -c1-160
  ```
- [ ] **Step 89** [V] ~1m: The four jobs pass, so they fall under DE2 removal at Step 230. Still red → read the run's annotations (`curl -s https://api.github.com/repos/unheaded/unheaded/check-runs/<id>/annotations`) and repeat that job's steps.
- [ ] **Step 90** [V] ~1m: **PHASE 3 EXIT GATE**: all four are green in the clean clone (code half). The live half is marked "after push" in `$PROG`.

---

## PHASE 4: THE CBOM COUNTS SYMMETRIC CRYPTO, KDFS AND HASHES (D-34) (Steps 91-118) [P]

**Goal**: `compliance/crypto/cbom.cdx.json` lists every AEAD, KDF and hash construction input the export audit found, plus the one hand-written primitive. A tripwire catches the next hand-written SHA-256.
**Prerequisite**: Phase 0.
**Time**: 2 h.
**Agent**: Agent [P].

- [ ] **Step 91** [R] ~10m: In `cmd/crypto-inventory/`, read the `scan_go.go` import table (lines ~150-175), `scan_rust.go` (lock and dep kinds), `cbom.go` (primitive and assetType mapping), and `testdata/`. Read the export audit's classification table, rows 1-41.
- [ ] **Step 92** [R] ~3m: Confirm today's uses.
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE '"(golang.org/x/crypto/(chacha20poly1305|hkdf|sha3)|crypto/(hkdf|sha3|aes))"' --include=*.go . | grep -v _test | grep -v '^./cmd/crypto-inventory'
  grep -nE '^(x25519-dalek|aes-gcm|hkdf|sha2|blake3|chacha20poly1305)\b' $(git ls-files '*Cargo.toml')
  grep -rln '0x428a2f98' --include=*.rs --include=*.go --include=*.c . | grep -v testdata
  ```
- [ ] **Step 93** [V] ~1m: There are hits in `pkg/secrets/encryption/{age,envelope}.go` and `pkg/crypto/pqc/kem_tunnel.go`, plus `crates/zhend/Cargo.toml` (x25519-dalek, aes-gcm, hkdf, sha2, blake3). The only round-constant file is `crates/upc-api/src/surface_hash.rs`. Anything more is added to the test set.
- [ ] **Step 94** [W] ~20m: Tests first:
  - Go testdata imports `chacha20poly1305` (kind `aead-import`), `x/crypto/hkdf` and `crypto/hkdf` (`kdf-import`), `crypto/aes` used with `cipher.NewGCM` (`aead-import`, AES-GCM), and `crypto/sha3` and `x/crypto/sha3` (`hash-import`).
  - Rust testdata: a Cargo.toml naming those crates gives `rust-crypto-dep` rows, with a primitive per crate.
  - Own primitive: a testdata file with the SHA-256 round constants outside the allow table fails `lint`, and the allowed path gives an `own-primitive` row.
  - `crypto/sha256` alone gives **no** row. It is a digest everywhere; record that choice in the scanner's comment and in a test name.
- [ ] **Step 95** [B] ~1m: Red.
  ```bash
  go test -count=1 ./cmd/crypto-inventory/ 2>&1 | tail -8
  ```
- [ ] **Step 96** [W] ~20m: Implement the Go kinds in the import table, with primitive classes `ae` (authenticated encryption), `kdf` and `hash`, matching CycloneDX 1.6 `cryptoProperties.algorithmProperties.primitive`.
- [ ] **Step 97** [W] ~15m: Implement the Rust `rust-crypto-dep` from Cargo.toml dependency lines, for a fixed crate table only.
- [ ] **Step 98** [W] ~10m: Implement `own-primitive`. There is a fixed table `{path: "crates/upc-api/src/surface_hash.rs", name: "SHA-256 (hand-written, FIPS 180-4)"}`, and a `lint` rule that fails on `0x428a2f98` in any tracked source not in the table.
- [ ] **Step 99** [B] ~3m: Green with race.
  ```bash
  go test -race -count=1 ./cmd/crypto-inventory/ 2>&1 | tail -2
  ```
- [ ] **Step 100** [C] ~1m: Commit the scanner and tests: `[PLAN S-EVIDENCE-RUNS] Steps 91-100: CBOM kinds for AEAD, KDF, hash, own primitive`.
- [ ] **Step 101** [B] ~2m: Regenerate the CBOM and count the rows by kind.
  ```bash
  go run ./cmd/crypto-inventory scan --out compliance/crypto/cbom.cdx.json && jq -r '.components[]."bom-ref" | split(":") | .[-1]' compliance/crypto/cbom.cdx.json | sort | uniq -c
  ```
- [ ] **Step 102** [V] ~2m: There are new rows for ChaCha20-Poly1305, HKDF (Go and Rust), AES-GCM, SHA-3, x25519-dalek, BLAKE3 and the own SHA-256, and every old row is still present (compare `bom-ref`s with `git show HEAD:compliance/crypto/cbom.cdx.json`).
- [ ] **Step 103** [B] ~2m: The inventory gate, plus the meta-gate row that provokes it.
  ```bash
  ./scripts/check-crypto-inventory.sh 2>&1 | tail -3; grep -n 'check-crypto-inventory' scripts/check-gates-can-fail.sh | head -2
  ```
- [ ] **Step 104** [V] ~1m: The gate passes, and the provocation (Step 79's run, or `-h` for its single-row form) still bites.
- [ ] **Step 105** [C] ~1m: Commit `compliance/crypto/cbom.cdx.json`.
- [ ] **Step 106** [W] ~10m: Docs: `grep -rln 'cbom\|crypto-inventory' wiki docs/compliance` finds where the kind list lives. Add the four kinds and the sha256 exclusion with its reason.
- [ ] **Step 107** [B] ~3m: Public facts the export audit left undetermined. Read-only, no login. Write the results in `$PROG`, not into the dated audit.
  ```bash
  curl -s https://api.github.com/repos/unheaded/unheaded | jq -r '.created_at, .visibility'
  curl -s 'https://api.github.com/orgs/unheaded/repos?per_page=100' | jq -r '.[] | "\(.name) \(.visibility) \(.created_at)"'
  ```
- [ ] **Step 108** [V] ~1m: Both answered. Private GHCR images need a login: say so in `$PROG` as "Stevie checks in the GitHub UI".
- [ ] **Step 109** [C] ~1m: Commit the docs and `$PROG`.
- [ ] **Step 110** [BLOCKED: A-25] ~15m: The drafted export notice gains the zhend hybrid with AES-GCM, the KEM tunnel and the hand-written SHA-256. It waits on Stevie's notification decision.
- [ ] **Step 111** [BLOCKED: A-25] ~5m: Barrister read of Step 110's text.
- [ ] **Step 112** [BLOCKED: A-25] ~1m: Commit.
- [ ] **Step 113** [R] ~5m: The audit's "constructions without callers" (age-style, the zhend hybrid). Deleting them is A-25's call; record that in `$PROG`.
- [ ] **Step 114** [B] ~2m: The CBOM and the audit agree. Every construction in the audit's "Constructions defined in this repository" has at least one CBOM row at its path.
  ```bash
  for p in pkg/secrets/encryption/age.go pkg/secrets/encryption/envelope.go crates/zhend/Cargo.toml pkg/crypto/pqc/kem_tunnel.go crates/upc-api/src/surface_hash.rs pkg/protocol/integrity/hmac.go; do printf '%s ' $p; jq --arg p "$p" '[.components[] | select(."bom-ref" | startswith($p))] | length' compliance/crypto/cbom.cdx.json; done
  ```
- [ ] **Step 115** [V] ~1m: Every count is ≥ 1, except `hmac.go` if HMAC-SHA-256 is classed as a digest use. Write the reason in `$PROG` either way.
- [ ] **Step 116** [D] ~10m: A zero → add that import to the table (test first) and rerun Steps 99-104.
- [ ] **Step 117** [C] ~1m: Commit if anything changed.
- [ ] **Step 118** [V] ~1m: **PHASE 4 EXIT GATE**: the inventory half is done and gated. Steps 110-112 are `[BLOCKED: A-25]`.

---

## PHASE 5: HOST-SIDE EVIDENCE, `host-service` (FND-037, D-20) (Steps 119-150) [P]

**Goal**: UH-AUTHN-01 reads each running first-party service: what runs, its auth setting, and an unauthenticated probe answering 401. West now, east after B-11; FND-037 closes when both are read.
**Prerequisite**: Phase 2 merged (so the token table exists to reuse).
**Time**: 3.5 h.
**Agent**: Agent [P] for code; Coordinator for the live read.

- [ ] **Step 119** [R] ~15m: Read the following:
  - ADR-097's evidence kinds section.
  - `pkg/compliance/crosswalk/catalog.go` kind validation (lines ~300-325).
  - `collect/collect.go`: the host-probe ssh path (~line 972) and host-sshd (~569).
  - `pkg/ports/ports.go`.
  - `scripts/compliance/check-service-auth.sh`, for its first-party list and how it finds them.
- [ ] **Step 120** [B] ~3m: What runs on west now (names only).
  ```bash
  sudo -n docker ps --format '{{.Names}}' | sort; systemctl list-units 'unheaded-*' --type=service --state=running --no-legend --plain | awk '{print $1}'
  ```
- [ ] **Step 121** [W] ~20m: Add a dated section to ADR-097, "host-service evidence kind (FND-037)". It covers:
  - **The ref.** `<host>:<service>`; the service comes from a fixed table in `collect`, so the catalog names a service, never a command.
  - **Per service, the table gives:** how it runs per host (compose container name or unit), its port from `pkg/ports`, one protected route, and its auth form (`AUTH_ENABLED=true`, or the token table).
  - **Pass condition.** Pass = running, auth set, and an unauthenticated TLS GET to the protected route answers 401. Not running where the table says it runs = fail. ssh or docker unreachable = not assessed.
  - **Secrets.** The environment holds secrets, so the collector parses it in-process and keeps only the auth key. No other value enters a record, a log or an error.
  - **Effect on figures.** This moves UH-AUTHN-01's figures. `declared:` stays until every host that runs a service is read.
- [ ] **Step 122** [C] ~1m: Commit the ADR section.
- [ ] **Step 123** [W] ~10m: Catalog tests first. `host-service` with `west:dashboard-backend` loads. An unknown service, a ref with a space, `;` or `/`, or a missing host are each rejected with `ErrInvalidCatalog`.
- [ ] **Step 124** [B] ~1m: Red, then implement `KindHostService` with its ref regex and table membership.
- [ ] **Step 125** [B] ~1m: Green.
  ```bash
  go test -race -count=1 ./pkg/compliance/crosswalk/ 2>&1 | tail -1
  ```
- [ ] **Step 126** [W] ~30m: Collector tests first. Use a fake runner for `docker inspect`, `systemctl show` and `dpkg-query` output, and an `httptest.NewTLSServer`.
  - (a) Running, `AUTH_ENABLED=true` and a 401 → pass; the detail names the image ID or package version.
  - (b) The server answers 200 → fail, "answers without a login".
  - (c) No auth variable → fail.
  - (d) The token table service with its variable → pass.
  - (e) Not running → fail.
  - (f) The runner returns an error → not assessed.
  - (g) **Secrets:** the fake env holds `WELL_PASSWORD=s3cr3t-canary` and `LOGSTORE_INGEST_TOKEN=tok-canary`. Neither canary appears in the record, the detail, any returned error or captured log output.
- [ ] **Step 127** [B] ~1m: Red.
- [ ] **Step 128** [W] ~40m: Implement `collect/service.go`:
  - Env parsing keeps only the auth keys.
  - TLS verifies against `/etc/unheaded/tls/ca.pem` (the path per host comes from the table). Timeouts are 5 s, and `io.LimitReader` bounds the body.
  - The remote host goes over the existing `ssh -o BatchMode=yes` path, with a constant remote script and no interpolated input.
- [ ] **Step 129** [B] ~3m: Green with race, then vet and gosec on the package.
  ```bash
  go test -race -count=1 ./pkg/compliance/crosswalk/... 2>&1 | tail -2; go vet ./pkg/compliance/crosswalk/...
  ```
- [ ] **Step 130** [C] ~1m: Commit `[PLAN S-EVIDENCE-RUNS] Steps 123-130: host-service evidence kind`.
- [ ] **Step 131** [W] ~20m: The table. List every first-party HTTP service the compose gate counts (14 today), with the hosts it runs on. Add a test that fails when `check-service-auth.sh`'s first-party set and the table differ, so the two cannot drift.
- [ ] **Step 132** [B] ~1m: Green.
- [ ] **Step 133** [W] ~10m: Catalog: UH-AUTHN-01 gains a `host-service` source for each west entry. The gate-script source stays, and so does `declared:`, which now reads "east not yet read (B-11)". Update the statement's last sentence to match.
- [ ] **Step 134** [B] ~3m: Catalog and status tests, then the baseline regeneration check. `baseline.yaml` must not change: `host-service` is central-only, not a baseline probe.
  ```bash
  go test -count=1 ./pkg/compliance/... ./cmd/compliance-evidence/... 2>&1 | tail -3; git status --short compliance/baseline/
  ```
- [ ] **Step 135** [C] ~1m: Commit the catalog, the table and the tests.
- [ ] **Step 136** [B] ~10m: The live trial on west, into a file that is not tracked.
  ```bash
  cp var/compliance/evidence.json $SCR/evidence-trial.json && go run ./cmd/compliance-evidence -out $SCR/evidence-trial.json -skip-gates 2>&1 | tail -3
  jq -r '.records[] | select(.source.kind=="host-service") | "\(.verdict) \(.source.ref) \(.detail)"' $SCR/evidence-trial.json | sort -u
  ```
- [ ] **Step 137** [V] ~3m: Every west entry passes.
- [ ] **Step 138** [D] ~15m: A fail. If the service really answers without a login, it is **a finding**: stop, record it, and tell Stevie; it is not a collector bug. If it is a collector misread (wrong route or port), fix the table test first.
- [ ] **Step 139** [V] ~2m: Secret sweep of the trial. It reads only detail strings and prints names, never values.
  ```bash
  jq -r '.records[] | select(.source.kind=="host-service") | .detail' $SCR/evidence-trial.json | grep -ciE 'password|secret|token=|key=' || true
  ```
  Expect `0`. Not 0 → stop; Step 128's filter is wrong. Shred the trial file (`shred -u $SCR/evidence-trial.json`) before fixing.
- [ ] **Step 140** [B] ~2m: The evidence timer picks it up on its next run. Check that its unit runs as an account that can read docker and ssh east: `systemctl cat unheaded-compliance-evidence.service | grep -E 'User|Group|SupplementaryGroups'`.
- [ ] **Step 141** [V] ~1m: It runs as govan or has the `docker` group. Otherwise record it in `$PROG`: the unit needs that group, and that is a host change for Phase 10.
- [ ] **Step 142** [W] ~3m: Add a dated line to FND-037's plan: "<date>: host-service reads west's services (all pass); east waits on S-E1-LIVE Step 218".
- [ ] **Step 143** [C] ~1m: Commit the register and `$PROG`.
- [ ] **Step 144** [V] ~1m: **West half of the exit gate**: the table, kind and tests are green; the live trial passes; the secret sweep reads 0.
- [ ] **Step 145** [BLOCKED: S-E1-LIVE Step 218] ~10m: Add east entries to the table and catalog, for what Step 211 enabled on east.
- [ ] **Step 146** [BLOCKED: S-E1-LIVE Step 218] ~10m: Run Step 136's trial against east.
- [ ] **Step 147** [BLOCKED: S-E1-LIVE Step 218] ~1m: Every east entry passes.
- [ ] **Step 148** [BLOCKED: S-E1-LIVE Step 218] ~5m: Remove `declared:` from UH-AUTHN-01. Give FND-037 a `resolved` block naming the test command and the record times on both hosts. `check-service-auth.sh` then falls to DE2 removal when it passes.
- [ ] **Step 149** [BLOCKED: S-E1-LIVE Step 218] ~1m: Commit.
- [ ] **Step 150** [V] ~1m: **PHASE 5 EXIT GATE**: the west half passes (Step 144). Steps 145-149 run in Phase 9's window or after it.

---

## PHASE 6: ENFORCING READINESS, MEASURED (ADR-098 condition 3) (Steps 151-160) [P]

**Goal**: One command answers "could this host be flipped?" and names every open finding in an enforceable class.
**Prerequisite**: Phase 0.
**Time**: 1 h.
**Agent**: Agent [P].

- [ ] **Step 151** [W] ~15m: Tests first in `cmd/compliance-evidence/findings_test.go`. `findings -enforce-ready <host>` lists that host's open entries whose class is in the ADR-098 enforcing table with an action:
  - The included classes: sysctl, service, sshd, firewall, kmod, fileperm, sudo and shell.
  - Accepted rows count, because the register header says they are "still counted".
  - The command prints `ready: no (N)` or `ready: yes` and exits 0.
  - An unknown host is a usage error.
- [ ] **Step 152** [B] ~1m: Red.
- [ ] **Step 153** [W] ~20m: Implement it, reusing the findings report's open-entry computation (no second copy of it).
- [ ] **Step 154** [B] ~2m: Green, then run it for both hosts. The output is host data, so it goes to `var/`.
  ```bash
  go test -race -count=1 ./cmd/compliance-evidence/ 2>&1 | tail -1
  for h in west east; do go run ./cmd/compliance-evidence findings -enforce-ready $h > var/compliance/enforce-ready-$h.txt; tail -1 var/compliance/enforce-ready-$h.txt; done
  ```
- [ ] **Step 155** [V] ~1m: Both read `ready: no (N)`. Write N per host and the class counts in `$PROG`.
- [ ] **Step 156** [W] ~5m: `docs/compliance/enforcing-soak.md`, under "Before the flip": add "`compliance-evidence findings -enforce-ready <host>` reads `ready: yes` (ADR-098 condition 3)".
- [ ] **Step 157** [C] ~1m: Commit.
- [ ] **Step 158** [W] ~3m: Add to `$PROG` "For Stevie" the rows that block each host by class, with the plan or sweep row that owns each (firewall: B-06/B-07; sudo: A-43; sshd: B-08; and this plan's fileperm rows).
- [ ] **Step 159** [C] ~1m: Commit `$PROG`.
- [ ] **Step 160** [V] ~1m: **PHASE 6 EXIT GATE**: the command is tested, both hosts are measured, and the owners are named.

---

## PHASE 7: THE MODEL BEFORE THE HOST WINDOWS (Steps 161-180) [P]

**Goal**: The fixes the windows will apply exist in the model, tested: AIDE with excludes (DE1), the `lab-audited` profile (DE4) and `var-log-modes` (DE5c).
**Prerequisite**: DE1, DE4, DE5c answered; any "no" marks its steps `[BLOCKED]`.
**Time**: 2 h.
**Agent**: Agent [P].

- [ ] **Step 161** [R] ~10m: In `compliance/remediation/fixes.yaml`, read `aide-install`, `rocm-log-perms` and the `mode` action. In `pkg/compliance/executor/`, read the run allowlist and the `validate` action. Read the Ansible renderer drift test in `pkg/iac` (what it says to run when the model changes).
- [ ] **Step 162** [W] ~15m: Test first (`pkg/compliance/remediation/model_test.go` and the executor tests). `aide-install` writes `/etc/aide/aide.conf.d/70_unheaded_excludes` (mode 0644) **before** `aideinit`, then validates with `aide --config-check`; `aide --config-check` is on the run allowlist. Exclude lines (`!/path`):
  - Both hosts: `/var/lib/docker`, `/var/lib/containerd`, `/var/snap/lxd/common/lxd/storage-pools`, `/var/lib/unheaded/baseline`, `/var/lib/unheaded/remediate`, `/tmp`, `/var/tmp`, `/var/log/journal`.
  - West part only (`hosts: [west]`): `/home/govan/tmp`, `/home/govan/go`, `/home/govan/.cache`, `/home/govan/.venv`, `/var/lib/govan-cold`, `/var/zhen`.
- [ ] **Step 163** [B] ~1m: Red.
  ```bash
  go test -count=1 ./pkg/compliance/... ./cmd/unheaded-remediate/... 2>&1 | grep -v '^ok' | head
  ```
- [ ] **Step 164** [W] ~15m: The model and the allowlist. The fix note says why each exclude is there, and that undo removes the file.
- [ ] **Step 165** [B] ~5m: Green, including `TestExecutorMatchesRetiredScript` (aide-install was never in the retired script, so its parity data must not change) and the renderer drift test. Regenerate the rendered Ansible the way the failing test's message says.
- [ ] **Step 166** [C] ~1m: Commit the model, executor, rendered Ansible and tests: `[PLAN S-EVIDENCE-RUNS] Steps 161-166: aide-install excludes`.
- [ ] **Step 167** [W] ~10m: Following DE4, add to `compliance/profiles/profiles.yaml` a `lab-audited` profile: description "west until B-08 and B-18 (Stevie <date>, DE4)", fixes = `lab` + `auditd-rules`, `aide-install`, `rocm-log-perms`. The profile loader test gains the name.
- [ ] **Step 168** [B] ~2m: Green. `compliance-evidence profile -h` says how a profile reaches the Well; record that for Phase 10 (the runbook `runbooks/security/compliance-profile-switch.yaml` is the switch).
- [ ] **Step 169** [W] ~20m: Following DE5c, test first, then add the `var-log-modes` fix: class fileperm, risk none, `closes: ["host-probe:*:var-log-perms"]`. It uses a `mode` action on `/var/log` files looser than 0640 (o-r), with the same exceptions the probe allows; read the probe at `cisprobes.go` first so the fix and the check agree exactly. The note says that rotation recreates some files at 0644, and enforcing would re-fix them.
- [ ] **Step 170** [B] ~3m: Green. Add `var-log-modes` to `lab-audited` and `lab-hardened`; the profile tests pass.
- [ ] **Step 171** [C] ~1m: Commit.
- [ ] **Step 172** [B] ~3m: `baseline.yaml` and the snapshot schema are untouched by profile and fix additions. A test failing here means `compliance-evidence baseline` must be rerun; do it and commit the regenerated file.
  ```bash
  go test -count=1 ./cmd/compliance-evidence/ -run Baseline 2>&1 | tail -2
  ```
- [ ] **Step 173** [B] ~5m: A dry run of every changed fix against a scratch root.
  ```bash
  for f in aide-install var-log-modes; do go run ./cmd/unheaded-remediate --root $SCR/root-$f --model compliance/remediation/fixes.yaml --host west plan $f 2>&1 | tail -8; done
  ```
- [ ] **Step 174** [V] ~2m: The plans read as intended: excludes before aideinit, modes only on files looser than 0640.
- [ ] **Step 175** [W] ~5m: Update `compliance/hosts/west.yaml` to `profile: lab-audited` **only in Phase 10** after the runbook. Nothing changes here; write this note in `$PROG` so no one moves it early.
- [ ] **Step 176** [B] ~10m: Full package tests with race.
  ```bash
  go test -race -count=1 ./pkg/compliance/... ./cmd/unheaded-remediate/... ./cmd/unheaded-baseline/... ./cmd/compliance-evidence/... ./pkg/iac/... 2>&1 | grep -v '^ok' | head
  ```
- [ ] **Step 177** [V] ~1m: No FAIL.
- [ ] **Step 178** [C] ~1m: Commit any regenerated files and `$PROG`.
- [ ] **Step 179** [B] ~2m: The merge order of the `[P]` phases into `sprint/evidence-runs` is 2, 3, 4, 5, 6, 7. After each merge, run Step 176's tests on the sprint branch.
- [ ] **Step 180** [V] ~1m: **PHASE 7 EXIT GATE**: every code phase is merged on the sprint branch and green.

---

## PHASE 8: SEAL KEY AND EAST'S SNAPSHOT (DE3) (Steps 181-194)

**Goal**: A custodied key seals east's snapshot; west's is sealed in Phase 10 after its profile changes.
**Prerequisite**: DE3 answered; Phase 7.
**Time**: 45 min, attended for the key.
**Agent**: Coordinator.

- [ ] **Step 181** [R] ~3m: Read `go run ./cmd/unheaded-keygen -h` (the ML-DSA-65 seed form and owner flag), and `go run ./cmd/compliance-evidence snapshot -h`.
- [ ] **Step 182** [W] ~10m: DE3 = dedicated key:
  - Give `unheaded-compliance-seal` a uid in `pkg/uids`; the uniqueness test covers it.
  - Add a line in `deploy/sysusers.d/unheaded.conf`: nologin, home `/var/lib/unheaded-compliance-seal`.
  - Test first: `go test ./pkg/uids/...`.
  - DE3 = build key → skip to Step 186 and follow B-10's custody instead.
- [ ] **Step 183** [C] ~1m: Commit.
- [ ] **Step 184** [STEVIE][S] ~10m: Create the account and the key, unseen.
  ```bash
  sudo systemd-sysusers /home/govan/tmp/unheaded/deploy/sysusers.d/unheaded.conf && sudo install -d -o unheaded-compliance-seal -g unheaded-compliance-seal -m 0700 /var/lib/unheaded-compliance-seal
  ```
  Then run the seed-file command from Step 181 as that account, writing into that directory. The seed never leaves it.
- [ ] **Step 185** [V] ~2m: The seed is 0400 and owned by the account. Copy the public half to `configs/keys/compliance-seal.pub` (public, DP-06).
  ```bash
  sudo stat -c '%a %U %n' /var/lib/unheaded-compliance-seal/*
  ```
- [ ] **Step 186** [C] ~1m: Commit `configs/keys/compliance-seal.pub`.
- [ ] **Step 187** [S] ~5m: Seal east's snapshot with a 31-day TTL. Build the binary once and run it as root, so it reads the seed in place. `-from-yaml` is used when the Well is not reachable from root's environment, and the snapshot records it.
  ```bash
  cd /home/govan/tmp/unheaded && go build -o $SCR/compliance-evidence ./cmd/compliance-evidence
  sudo $SCR/compliance-evidence snapshot -repo-dir . -host east -key /var/lib/unheaded-compliance-seal/<seed> -ttl 744h -model-commit $(git rev-parse HEAD) -from-yaml -out $SCR/east.snapshot.json
  sudo chown govan: $SCR/east.snapshot.json
  ```
- [ ] **Step 188** [V] ~2m: It verifies with the public key and lists east's profile fixes.
  ```bash
  go run ./cmd/unheaded-remediate --snapshot $SCR/east.snapshot.json --pubkey configs/keys/compliance-seal.pub --host east list 2>&1 | head -20
  ```
- [ ] **Step 189** [D] ~5m: Refused → read the reason (another host or key, expired, downgrade) and reseal. Don't edit the file.
- [ ] **Step 190** [W] ~10m: Record the ops facts in the wiki (`wiki/Compliance-Paperwork.md`, "Snapshot seal"): where the key lives, who reseals, the 31-day TTL, the reseal command, the host paths (`/etc/unheaded/compliance/snapshot.json` and `snapshot.pub`), and what an expired snapshot does (the soak clock restarts; LICH-017 T6).
- [ ] **Step 191** [W] ~2m: Add a "reseal by <date+31d>" line to `$PROG` and to `~/tmp/next.md`'s recurring list.
- [ ] **Step 192** [C] ~1m: Commit the wiki and `$PROG`.
- [ ] **Step 193** [B] ~1m: `shred -u` nothing: the snapshot is public data. The seed never left its directory; check with `ls $SCR`.
- [ ] **Step 194** [V] ~1m: **PHASE 8 EXIT GATE**: the key is custodied, the public half is published, and east's snapshot verifies.

---

## PHASE 9: EAST HOST WINDOW (ATTENDED; after S-E1-LIVE Step 218) (Steps 195-216)

**Goal**: East's journal is 15 of 15. AIDE runs. The DE5a rows are fixed. The live LICH-017 drill passes. East's soak clock is running. FND-037's east half is read.
**Prerequisite**: S-E1-LIVE Step 218 passed; Phase 8; DE1, DE5a, DE7.
**Time**: 1.5 h, off-hours for `aideinit`.
**Agent**: Coordinator, Stevie present.

- [ ] **Step 195** [V] ~2m: Step 218 has passed (`$PROG` from Step 14, rechecked), and east runs the current `unheaded-baseline` and `unheaded-remediate`.
  ```bash
  ssh east 'dpkg-query -W -f "\${Package} \${Version}\n" "unheaded-*" | sort; unheaded-remediate -V'
  ```
- [ ] **Step 196** [STEVIE][S] ~2m: Silence first (`maintenance.sh 120m`), and keep a second terminal on east.
- [ ] **Step 197** [S] ~3m: Install the snapshot and public key on east.
  ```bash
  scp $SCR/east.snapshot.json /home/govan/tmp/unheaded/configs/keys/compliance-seal.pub east:/tmp/
  ssh east 'sudo install -d -o root -g root -m 0755 /etc/unheaded/compliance && sudo install -o root -g root -m 0644 /tmp/east.snapshot.json /etc/unheaded/compliance/snapshot.json && sudo install -o root -g root -m 0644 /tmp/compliance-seal.pub /etc/unheaded/compliance/snapshot.pub && rm /tmp/east.snapshot.json /tmp/compliance-seal.pub'
  ```
- [ ] **Step 198** [V] ~1m: The snapshot loads on east.
  ```bash
  ssh east 'sudo unheaded-remediate --snapshot /etc/unheaded/compliance/snapshot.json list | head'
  ```
- [ ] **Step 199** [S] ~3m: Plan `aide-install` (DE1).
  ```bash
  ssh east 'sudo unheaded-remediate --snapshot /etc/unheaded/compliance/snapshot.json plan aide-install'
  ```
- [ ] **Step 200** [STEVIE][S] ~20m: Apply. `aideinit` hashes the filesystem minus the excludes, and takes minutes.
  ```bash
  ssh east 'sudo unheaded-remediate --snapshot /etc/unheaded/compliance/snapshot.json apply aide-install 2>&1 | tail -5; sudo aide --config-check && echo CONFIG-OK; systemctl is-enabled dailyaidecheck.timer'
  ```
- [ ] **Step 201** [V] ~1m: `CONFIG-OK` and `enabled`, and `check aide-install` exits 0. Fail → Step 202.
- [ ] **Step 202** [D] ~10m: The executor restored the files. Read the journal tail and the backup directory it printed. `undo` it if anything is half-applied. Leave the row open and record the cause.
- [ ] **Step 203** [STEVIE][S] ~5m: `apparmor-enforce` (DE7), plan then apply. Then `ssh east 'sudo aa-status --json | jq -r ".profiles.Xorg"'`.
- [ ] **Step 204** [V] ~1m: `enforce`, and `check apparmor-enforce` exits 0.
- [ ] **Step 205** [C] ~1m: Commit `$PROG`: `[PLAN S-EVIDENCE-RUNS] Steps 195-205: east aide-install + apparmor-enforce journaled`.
- [ ] **Step 206** [B] ~2m: DE5a on east:
  - `ssh east 'chmod g-w /home/govan/.gitconfig && stat -c %a /home/govan/.gitconfig'`.
  - Dry-run the purge: `ssh east 'sudo apt-get -s purge ftp tnftp inetutils-telnet telnet | grep -E "^(Purg|Remv)"'`.
- [ ] **Step 207** [STEVIE][S] ~3m: If the dry run lists only those four packages, purge them; never `autoremove`. Any other package in the list → stop and ask.
- [ ] **Step 208** [S] ~10m: The east `/tmp` and `/dev/shm` noexec trial. Remount live, run east's timers that write there, then read the result.
  ```bash
  ssh east 'sudo mount -o remount,noexec /tmp && sudo mount -o remount,noexec /dev/shm && sudo systemctl start unheaded-baseline.service && systemctl is-failed unheaded-baseline.service; findmnt -no OPTIONS /tmp /dev/shm'
  ```
- [ ] **Step 209** [V] ~2m: The baseline run is `active` or `inactive`, not `failed`, and both mounts show `noexec`. A failure → roll back at once (`remount,exec` on both) and record it.
- [ ] **Step 210** [STEVIE][W] ~5m: Make it permanent. For `/tmp` (east's `tmp.mount` unit), add a drop-in `/etc/systemd/system/tmp.mount.d/60-noexec.conf` with `[Mount]` and `Options=mode=1777,strict,nosuid,nodev,noexec`, reading the unit's current `Options=` first and keeping it. For `/dev/shm`, add an fstab line `tmpfs /dev/shm tmpfs defaults,nosuid,nodev,noexec 0 0`, then `sudo findmnt --verify`. Reboot proof is S-E1-LIVE Step 301.
- [ ] **Step 211** [S] ~5m: The live LICH-017 drill on east (cases T2 and T5). The snapshot is installed but the soak is under 7 days.
  ```bash
  ssh east 'echo mode=enforcing | sudo tee /etc/unheaded/baseline.conf >/dev/null; sleep 20; sudo journalctl -u unheaded-baseline --since -2min --no-pager | grep -E "CRITICAL|SUMMARY" | tail -3; sudo jq -r .mode_effective /var/lib/unheaded/baseline/report.json; sudo rm /etc/unheaded/baseline.conf'
  ```
- [ ] **Step 212** [V] ~1m: There is a `CRITICAL enforcing requested ... not in effect` line, `mode_effective` reads `audit`, and the mode file is gone afterwards. Enforcing in effect → **P1**: stop. Every host stays audit (enforcing-soak.md "Undo"), and Stevie is told.
- [ ] **Step 213** [V] ~2m: The soak clock started: `ssh east 'sudo jq . /var/lib/unheaded/baseline/soak.json'` shows `clean_since` today. Write it in `$PROG`. Note that Step 211's swap restarted it once; that is expected.
- [ ] **Step 214** [S] ~5m: Collect, then read east's rows.
  ```bash
  sudo systemctl start unheaded-compliance-evidence.service; go run ./cmd/compliance-evidence findings | grep -E 'east:(aide|dot-files|pkg-(ftp|telnet)|mount-(tmp|dev-shm)-noexec|apply-journal)' | cut -c1-140
  ```
- [ ] **Step 215** [V] ~1m: None of those is listed (all pass). `host-journal:east:apply-journal` passes. Now run Steps 145-149 (FND-037's east half).
- [ ] **Step 216** [C][V] ~2m: **PHASE 9 EXIT GATE**: commit `$PROG` and any Phase 5 east work. East's journal is complete, the drill passed and the soak is running.

---

## PHASE 10: WEST HOST WINDOW (ATTENDED) (Steps 217-226)

**Goal**: West moves to `lab-audited`. AIDE and auditd rules are journaled. The DE5a rows are fixed. West's soak clock is running.
**Prerequisite**: Phase 9 exit (east first, ADR-098 §4); DE4; not during S-E1-LIVE Evening 17.
**Time**: 1.5 h, off-hours for `aideinit`.
**Agent**: Coordinator, Stevie present.

- [ ] **Step 217** [R] ~5m: Read `runbooks/security/compliance-profile-switch.yaml` all the way through. Run its plan steps for west to `lab-audited`, as it says.
- [ ] **Step 218** [STEVIE][W] ~5m: Switch per the runbook: `compliance/hosts/west.yaml` `profile: lab-audited`, and the Well row through the runbook's command. Commit the YAML.
- [ ] **Step 219** [S] ~5m: Seal west's snapshot (Step 187 with `-host west`, then Step 188), and install it as in Step 197 but locally (`sudo install ...`).
- [ ] **Step 220** [STEVIE][S] ~10m: `auditd-rules`, plan then apply. Then build something and measure the noise.
  ```bash
  sudo unheaded-remediate --snapshot /etc/unheaded/compliance/snapshot.json apply auditd-rules 2>&1 | tail -3; sudo auditctl -l | wc -l
  ( cd /home/govan/tmp/unheaded && go build ./cmd/compliance-evidence ) ; sudo ausearch -ts recent -k access 2>/dev/null | grep -c '^type=SYSCALL'
  ```
  The rule count should be at least 40 (east has 42). The access-key count after one build is the noise figure; write it in `$PROG`. More than 1,000 → Appendix A2.
- [ ] **Step 221** [STEVIE][S] ~25m: Apply `aide-install` (excludes, then `aideinit`), then `aide --config-check`. Then `rocm-log-perms` (`check`, else `-record-existing` with `-applied-by "installer modes (2026-10-27)"`, 28 characters, under the 30 cap) and `var-log-modes` (DE5c).
- [ ] **Step 222** [B] ~3m: DE5a on west:
  - `chmod g-w /home/govan/.gitconfig /home/govan/.wget-hsts /home/govan/.zshrc`.
  - Root's PATH line: `sudo grep -n vector /root/.profile /root/.bashrc`, then remove that line and `sudo rm -rf /root/.vector` (empty; Vector runs in compose).
  - Dry-run the ftp/telnet purge as in Step 206.
- [ ] **Step 223** [STEVIE][S] ~15m: The purge, if the dry run is clean. Then the `/dev/shm` noexec trial with the stack running. Check `docker ps` health, a Zhen and llama start if they are running, `go test ./pkg/uids/...` and one Python multiprocessing call (`python3 -I -c 'import multiprocessing as m; print(m.Pool(2).map(abs,[-1,-2]))'`). Roll back on any failure; the fstab line is as in Step 210.
- [ ] **Step 224** [S] ~5m: Collect and read west's rows (Step 214 with `west:`), then `findings -enforce-ready west`.
- [ ] **Step 225** [V] ~2m: These pass: aide x2, `auditd-rules-loaded`, `dot-files`, `root-path`, `pkg-ftp`/`pkg-telnet`, `mount-dev-shm-noexec`, `var-log-perms`, and `host-journal:west`. West's soak clock started (`soak.json`).
- [ ] **Step 226** [C][V] ~2m: **PHASE 10 EXIT GATE**: commit `$PROG` and `compliance/hosts/west.yaml`.

---

## PHASE 11: SOAK, READINESS, RECORDS AND CLOSE (Steps 227-236)

**Goal**: Seven clean days on both hosts, read. The register matches the evidence. Docs and the handoff are written, and everything is merged.
**Prerequisite**: Phases 9 and 10.
**Time**: 10 min a day for 7 days, then 1 h.
**Agent**: Coordinator.

- [ ] **Step 227** [B] ~5m: Daily, for 7 days: read both soak clocks and the would-enforce lines.
  ```bash
  for h in west east; do c="sudo jq -r .clean_since /var/lib/unheaded/baseline/soak.json; sudo journalctl -u unheaded-baseline --since -1d --no-pager | grep -c would_enforce="; [ $h = west ] && bash -c "$c" || ssh east "$c"; done
  ```
  With S-WATCH-TRUE's textfile-dir in place, use the PromQL in `enforcing-soak.md` instead.
- [ ] **Step 228** [V] ~2m: `clean_since` has not moved, and there are 0 `would_enforce=` lines per day. If the clock restarted, follow Appendix A5, and the 7 days restart. Write a dated line in `$PROG` each day.
- [ ] **Step 229** [W] ~10m: Day 7. Run `findings -enforce-ready` for both hosts, and put the blockers into `$PROG` "For Stevie":
  - The soak passed on both hosts.
  - The flip needs condition 3: the listed rows and their owners.
  - It also needs condition 4: his signature in `compliance/hosts/<host>.yaml`.
  - The flip recipe stays `enforcing-soak.md` "The flip", east first.
- [ ] **Step 230** [W] ~10m: Final register pass:
  - Rerun the findings report into `var/compliance/findings-s-evidence-runs-after.md`.
  - Under DE2, remove every source now in the "remove" list.
  - Fill the E-row outcome lines in `$PROG`.
  - Add a dated section to `docs/compliance/enforcing-soak.md`: the soak result, the live drill, and seal custody.
- [ ] **Step 231** [B] ~30m: Full checks on the sprint branch, with the CI venv last on PATH as in Step 79.
  ```bash
  go test -race -count=1 ./pkg/compliance/... ./cmd/compliance-evidence/... ./cmd/crypto-inventory/... ./cmd/unheaded-baseline/... ./cmd/unheaded-remediate/... ./tomb/lich/LICH-017-baseline-agent/... 2>&1 | grep -v '^ok' | head
  ./scripts/check-gates-can-fail.sh 2>&1 | tail -5
  ```
- [ ] **Step 232** [V] ~2m: Check:
  - No FAIL, and every gate bites.
  - `findings` shows 0 untriaged and an empty remove list (or only DE2-held rows).
  - Every E row has an outcome line.
- [ ] **Step 233** [C] ~6m: Guarded merge:
  ```bash
  git switch sprint/evidence-runs && git merge --no-ff develop && go build ./... && git switch develop && git merge --no-ff sprint/evidence-runs -m "[PLAN S-EVIDENCE-RUNS] merge"
  ```
  This merges develop into the sprint branch, builds and gates, then merges the sprint into develop. The main checkout is never left half-merged.
- [ ] **Step 234** [W] ~10m: Librarian ripple:
  - `wiki/Compliance-Paperwork.md` (host-service, readiness, AIDE schedule, seal).
  - The ADR-097 section linked from the wiki ADR index.
  - `docs/sweeps/` is not edited (dated): add the plan's outcome to `~/tmp/next.md` instead.
- [ ] **Step 235** [W] ~10m: Write the `~/tmp/next.md` handoff: E-row outcomes, what waits on Stevie (the flip, A-25, the push of Step 87), the reseal date, and the soak result.
- [ ] **Step 236** [V] ~2m: **FINAL GATE**: Steps 231-232 are green on develop after the merge, `$PROG` has every outcome, and next.md is written.

---

## Appendix A: Emergency procedures

**A1. `aideinit` runs long or fills the disk.**
1. Check with `df -h /var/lib/aide /` and `pgrep -a aide`.
2. If `/` passes 85 %, stop it (`sudo kill <pid>`, by pid, never `pkill -f`).
3. `undo` the fix through the backup directory the apply printed.
4. Find the large tree with `sudo du -xh --max-depth=2 / | sort -h | tail`, add it to the excludes (Phase 7, test first) and reseal.

**A2. auditd floods after `auditd-rules`.**
1. `sudo ausearch -ts recent --raw | awk '{print $NF}' | sort | uniq -c | sort -n | tail` names the key.
2. If it is `access` from builds, `undo` the apply now (the rules are not immutable: no `-e 2`).
3. Add an `-F dir!=` exclusion for the build trees, test first, reseal and reapply. Never set `disk_full_action=halt`.

**A3. A remount breaks a service.**
1. `sudo mount -o remount,exec /tmp /dev/shm` at once.
2. Find the unit with `journalctl -p err --since -10min`.
3. The row stays open with the unit named, and the trial is noted as failed in `$PROG`.

**A4. An apply fails midway.** The executor restores before it returns. Check `sudo unheaded-remediate check <fix>` and read the journal tail. Run `undo <dir>` only if `check` shows partial state. Never hand-edit a file the fix owns.

**A5. The soak clock restarts.**
1. Read the restarting check: `journalctl -u unheaded-baseline | grep would_enforce= | tail -3`.
2. Drift a fix would close → re-apply that fix through the executor (attended); the clock starts again.
3. A missing or expired snapshot → reseal (Phase 8).
4. State tamper (`event=state-tamper`) with no one having touched the files → treat it as an incident: keep the files, tell Stevie, and stop the soak.

**A6. The mode file is left on `mode=enforcing`.** `sudo rm /etc/unheaded/baseline.conf` (missing = audit), then run the unit once and confirm `mode_effective` reads `audit`.

**A7. A secret appears in an evidence record or a log.**
1. Stop the collector timer.
2. `shred -u` the affected trial file.
3. Rewrite the evidence file without the record (`jq` filter to a new file, then `mv`).
4. Fix the parser test-first (Phase 5 Step 126 case g).
5. Tell Stevie which value class leaked. Rotate it if it left the host.

**A8. A snapshot is refused on the host.** The refusal names its reason: another host or key, expired, edited or a downgrade. Reseal from the repo, never edit. A downgrade refusal means the host already holds a newer model: reseal from the newer commit.

## Appendix B: Agent assignment and critical path

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Decisions, baseline | Coordinator | no | none | 0.5 h |
| 1 Register reads evidence | Coordinator | no | 0 | 2.5 h |
| 2 Service-auth gate | Agent | [P] | 0 | 0.75 h |
| 3 CI reds | Agent | [P] | 0 | 2-3 h |
| 4 CBOM | Agent | [P] | 0 | 2 h |
| 5 host-service | Agent, then Coordinator | [P] | 2 | 3.5 h (east half after B-11) |
| 6 Readiness | Agent | [P] | 0 | 1 h |
| 7 Model | Agent | [P] | DE1, DE4, DE5c | 2 h |
| 8 Seal key, east snapshot | Coordinator + Stevie | no | 7, DE3 | 0.75 h |
| 9 East window | Coordinator + Stevie | no | 8, S-E1-LIVE 218 | 1.5 h |
| 10 West window | Coordinator + Stevie | no | 9 | 1.5 h |
| 11 Soak and close | Coordinator | no | 10 | 7 d + 1 h |

The critical path is 0 → 7 → 8 → 9 → 10 → 11, gated outside this plan by S-E1-LIVE Step 218. Phases 1-6 fill the wait.

## Appendix C: Quick reference

- **Findings:** `go run ./cmd/compliance-evidence findings [-out var/compliance/x.md]`. The report keeps the newest record per source by `observed_at`. Source rows close by **removal** when their source passes; FND ids close by `resolved: {on, evidence}`. An `accepted` block is at most 90 days and is still counted.
- **Evidence kinds:** `github-job`, `gate-script`, `git-signatures`, `host-sysctl`, `attestation`, `host-sshd`, `host-probe`, `host-journal`, `host-agent`, `slo`, and, new in Phase 5, `host-service`.
- **Enforceable classes (ADR-098 §2):** sysctl, service, sshd, firewall, kmod, fileperm, sudo, shell. Alert only: package, boot, mac, account, mount.
- **Executor:**
  - `unheaded-remediate [--snapshot S] [--host H] list|check|plan|apply <fix>|undo <dir>`.
  - `-record-existing <fix> -applied-by "<≤30 chars>"`.
  - State lives in `/var/lib/unheaded/remediate/journal.jsonl`.
- **Baseline agent:**
  - Mode file `/etc/unheaded/baseline.conf` (missing = audit).
  - State in `/var/lib/unheaded/baseline/{report.json,soak.json}`.
  - Snapshot at `/etc/unheaded/compliance/{snapshot.json,snapshot.pub}`.
- **Snapshot:** `compliance-evidence snapshot -host H -key SEED -out F [-ttl 744h] [-from-yaml] [-model-commit C]`. The TTL is at most 31 days.
- **Profiles:** `none`, `lab` (east was here before 10-12), `lab-hardened` (east), `lab-audited` (west, new), `cis-l1-server`, `all`.
- **The other DECLARED controls** (not this plan; FND-037 names UH-AUTHN-01): UH-CFG-01, UH-CFG-02, UH-CTR-01, UH-EVID-01, UH-LIC-01, UH-PQC-02, UH-TLS-01. Each needs its own reading of what runs; one row each for the next sweep.
- **Untouchable here:** the export notice (A-25), the flip (condition 4 is Stevie's), the B-18 mounts (S-E1-LIVE 289-306), the firewall and sudo rows.

---

*S-EVIDENCE-RUNS Battle Plan, forged 2026-11-12.*
*12 phases, 236 steps. The evidence stops reading the paper and starts reading the hosts.*
