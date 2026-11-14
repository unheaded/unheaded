<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WATCH-TRUE BATTLE PLAN: 12 Phases, 226 Steps

**Date**: 2026-11-12
**Sprint**: S-WATCH-TRUE. Every scrape job, alert and panel matches something that runs. The trace collector stops burning a core. Backlog sweep bundle 3, rows D-13 to D-18 (`docs/sweeps/BACKLOG-SWEEP-2026-11-12.md`).
**Prerequisite**: Stevie's answers of 2026-11-12, all in hand: A-22 fix the trace collector (freeze lifted for ADR-144 item 1 only) and record it in the findings register; A-18 add the Wotan refusal alert with its runbook; A-17 option (A), Vector counters by kind. W1-W4 answered 2026-11-12 (table below; W4 overrides the default for `lxd/telemetry`). W5's default stands unless Stevie tunes it at Step 3.
**Target**: On west, the trace collector holds under 0.05 of a core and its CPU no longer grows with uptime. It makes under 2,000 `bpf()` calls a second and publishes latency events again. VictoriaMetrics has no down target that nothing is meant to run. `WotanTopicPermissionRefused` and the new ioc-audit readings are live and tested. Every row D-13 to D-18 is closed or has a named reason.
**Estimated duration**: 9-12 h over 2-3 sessions. Phases 1-3, 5-9 are code (no stack needed). Phase 4 and the live halves of 5-9 need west's stack and sudo. East (Phase 4B) is attended.
**Agent strategy**: Coordinator for Phases 0, 4, 4B and 10. Phases 1-3 are one chain (trace collector). Phases 5, 6, 7, 8 and 9 are independent of it and of each other, and can run as `[P]` agents in separate worktrees. They merge one at a time.
**Commit cadence**: every 5 steps, at every exit gate, before every skip. Signed (`gpgsign = true` in `~/.gitconfig`). If gpg-agent has timed out and Stevie is away, use `--no-gpg-sign` and log it for `scripts/resign-unpushed.py`. Named files only, never `git add -A`. Stevie pushes.
**Stuck protocol**: skip after 3x the step estimate or two failed debug attempts. Mark the step `[STUCK]` and its dependents `[BLOCKED]`, commit, and go on to the next independent phase.

## Grounding (checked 2026-11-12 ~22:50Z against develop `a31be33b`, read-only)

Every row was checked against the tree and the live west. What changed from the sweep's wording:

| Row | Sweep said | Found | Plan does |
|---|---|---|---|
| D-17 | "Two readers walk LATENCY_MAP every 200 ms" (ADR-144) | **One reader.** `cmd/trace-collector-go/main.go:828-871` runs the direct cilium/ebpf sweep **or** the `LatencyReader` fallback, never both. The two start-up lines ADR-144 saw are both from the direct branch (lines 834 and 869). `latency_reader_polls_total` is 0 on west. The direct sweep is `Iterate()` every 200 ms, two `bpf()` calls per entry, no delete, and it has no metrics of its own. | Fix the one sweep. Record the correction in ADR-144 (dated section). |
| D-17 | "latency panels go stale some hours after boot" | **West publishes no latency events at all.** The installed object `/usr/local/lib/unheaded/ebpf/latency-probe` (2026-10-18) has a **40-byte** LATENCY_MAP key, the old 5-tuple `LatencyKey`. The binary `/usr/local/bin/trace-collector-go` (2026-11-10) decodes the **24-byte** `LatencyTraceKey` of S-WIRE-HONEST (`262e8786`). Every entry fails `DecodeLatencyTraceKey` and is skipped by a bare `continue`. Nothing counts it. Checked by reading both objects' map specs with cilium/ebpf: installed `Hash key 40`, tree `Hash key 24`. The S-WIRE-HONEST ship rule ("probe and collector ship together") was broken at deploy. | Ship them together. Refuse to load a mismatched object (fail loud). Count decode errors. |
| D-17 | east 1.56 cores | East runs a 2026-10-22 binary with a 2026-10-20 object, both pre-S-WIRE-HONEST, up since 2026-10-22. It is at **1.93 cores now** (VictoriaMetrics `rate(process_cpu_seconds_total{job="trace-collector"}[30m])`). Kernel 6.17.0-41 on both hosts. | Attended east deploy (Phase 4B). |
| D-17 | (not in the sweep) | The probe writes every sample to the `LATENCY_EVENTS` ring buffer, and **no userspace code reads it** (`grep LATENCY_EVENTS cmd/` finds nothing). So it is full and every reserve fails (`STAT_EVENTS_DROPPED`). `LATENCY_STATS` is not exported either. | Export `LATENCY_STATS`. Reading the ring (ADR-144's "better shape") stays parked: see Marshal. |
| D-13 | "`sword` is still a down target" | VictoriaMetrics (`config/victoria/scrape.yml`, which scrapes since `99e18bbb`) has three down targets: `sword` (`host.docker.internal:19011`, compose profile `deploy`) and `heimdall` on west and east. No unit runs heimdall-daemon on either host; `deploy/systemd/unheaded-heimdall.service` exists but is not installed. Prometheus has no `sword` job; it still has `heimdall`. Its only other down target is heimdall. The scrape.yml comment calling `syslog-relay` and `zhen-agentd` "down" is stale: both are up since 2026-11-12. | W1 and W2 decide sword and heimdall. Fix the stale comment. |
| D-13 | `ZhenSchedulerStalled` cannot tell absent from stalled | True of the alert: one rule ORs `absent_over_time` with the staleness test (`monitoring/prometheus/rules/zhen.yml:34-46`). The runbook already tells the cases apart by hand. | Two rules under the same name, label `reason: absent | stale`. |
| D-14 | Two empty Wotan idempotency panels | True. `pkg/wotan-client/idempotency.go` registers the metrics only when a cache is built, and no production binary builds one. Panels: `config/grafana/provisioning/dashboards/kingdom/00_kingdom_services.json:662,666`. | Remove the two panels (they return with a binary that builds a cache). |
| D-14 | `09_compliance` 0 of 8 until `cmd/unheaded-baseline` emits | The timer runs on west (`unheaded-baseline.timer`, every ~5 min), but the unit runs without `-textfile-dir` (`deploy/systemd/unheaded-baseline.service:57`). No `unheaded_baseline_*` series exists in VictoriaMetrics. The textfile directory huginn reads is `/var/lib/unheaded/textfile` (root 0755). `unheaded-ci-status` writes there through a per-user ACL, the house pattern. | `-textfile-dir` plus an ACL. |
| D-15 | Gates miss `haproxy:2.9.15-alpine` x2 and `unheaded-sword` | `docker/docker-compose.loadbalancers.yml` has haproxy x2 **and `nginx:1.25.5-alpine` x6**, none with a digest. `check-image-digests.sh` reads only `docker-compose.yml` and `monitoring/docker-compose.yml`, on purpose (ADR-102, Decision 4 "stands everywhere else"). ADR-102's 2026-11-07 update records Stevie's 2026-11-06 answer, "pin the digest the registry serves". `unheaded-sword` is built here, so it is out of the digest rule by definition: that half of the row is dropped. | Pin the eight lines and widen the gate (ADR-102 dated section). |
| D-15 | Untracked `docker-compose.override.yml` | Gitignored (`.gitignore:273`), dated 2026-09-29, 19 services. **Compose merges it automatically into every `docker compose` run on west.** No secret-like key names in it. | Record what it overrides (key names only). The file is Stevie's (W3, record only). |
| D-15 | `bootstrap-llm-lab.sh` pulls `:latest`; `lxd/telemetry` downloads archives | True: `scripts/bootstrap-llm-lab.sh:979-981` (prometheus, grafana, **loki**: Loki left with ADR-117). `lxd/telemetry/grafana-lxd.yaml:55` fetches Grafana 10.2.2 with no checksum; the directory predates ADR-117. `lxd/Makefile` has a `launch-telemetry` target and help lines that run it. | W4 (answered): pin and drop Loki in the script; delete `lxd/telemetry/` and every live reference. |
| D-16 | Alert on refusals; needs a runbook | Counter `unheaded_wotan_topic_permission_refusals_total{action,outcome}` (`services/wotan/internal/api/topic_permissions.go:185`). West reads `subscribe denied = 2`, flat for 16 h (the Sophia refusals before `90e45458`'s SIGHUP). No label says **who** was refused. | Alert plus runbook, and a bounded `caller` label. |
| D-18 | Vector counters by kind | `config/vector/vector.yaml` counts `unheaded_auth_failures_total{kind=ssh|sudo|pam}` and `unheaded_ssh_accepted_total`. Nothing counts account changes, sudo commands, crontab edits or kernel lines. Journal shapes checked on west: `sudo` lines are `_UID=0 _TRANSPORT=syslog`, "`<user> : ... COMMAND=`"; kernel lines are `_TRANSPORT=kernel` with no `_UID`; crontab lines exist; no `useradd` line in the retained journal. | New trusted-field transform, one metric, ioc-audit reads it. |

**Marshal (scope).** IN: D-13 to D-18 as answered, plus the three things found while grounding that are the same defects: the mismatched object, the decode-error counter, the `LATENCY_STATS` export. PARKED, with a home: reading `LATENCY_EVENTS` instead of sweeping (ADR-144 item 1, third option; the next trace-collector change); a per-unit CPU budget alert (ADR-144 item 6, still frozen); installing heimdall-daemon (W2 alternative; a deploy, not monitoring truth); east log shipping (B-22, S-LOG-HARVEST); `INFLIGHT` and `SOCK_TRACE`, which also drop inserts silently when full (noted in ADR-144's dated section, not fixed: not answered). Ruling: no new third-party code. Every change is first-party Go, Rust, VRL, PromQL or YAML on what is already in the tree.

**Micromanager (QA / DoD).** Code is red then green under `go test -race`. Each eBPF change passes `check-ebpf-loads.sh` (the verifier, root) and `check-ebpf-insns.sh`. Rules pass `check-prometheus-rules.sh` (promtool tests plus runbook links). Vector passes `test-vector-config.sh`. Runbooks pass their pytest files and `runbook-manifest.sh --write`. Metadata only: no new label carries log text, and every new label has a fixed or config-bounded alphabet (the Sacred Principle applied to logs). Done means the live gate on west (Phase 4 exit) is met with numbers in the progress file, not that the tests pass.

**Busboy (handoffs).** S-RESIDUE (written in parallel) owns doc residue D-28; the latency-probe "per-flow" comments are D-17 and stay here. Touching Vector restarts it during the **log store week (S-E1-LIVE Step 284, ends ~2026-11-17)**. Vector resumes from `vector-data`, so no row is lost, but note the restart time in `docs/battle-plans/progress/S-E1-LIVE-progress.md` the way the Well restart of 06:40Z was noted. `ioc-audit` stays `status: draft` until Stevie approves it (A-1). This plan changes its bytes, so the manifest is rewritten (`--write` only; the seal is Stevie's). FND-044 goes in the register here; S-EVIDENCE-RUNS later settles E-01 (FND-043's missing `resolved` block), not this plan.

## Decisions (W1-W4 answered by Stevie 2026-11-12; W5 is a default)

| # | Question | Ruling | Status |
|---|---|---|---|
| W1 | `sword` is scraped every 10 s and is down except during a deploy. No rule, SLO or panel reads `sword_*`. | **Remove the job** from `config/victoria/scrape.yml`. Sword's code and runbooks stay. | Answered 2026-11-12 |
| W2 | heimdall: two alert rules and two scrape jobs for a daemon no host runs | **Remove** the `heimdall` rule group, its tests and both scrape jobs (Prometheus and VictoriaMetrics). Keep heimdall's code and runbooks. A rule returns with its producer and a test (decision 25). | Answered 2026-11-12 |
| W3 | `docker-compose.override.yml` on west (gitignored, merged into every compose run) | **Record only**: list what it overrides, key names only, in the progress file and "For Stevie". Nothing is changed. | Answered 2026-11-12 |
| W4 | `bootstrap-llm-lab.sh` `:latest` x3 and `lxd/telemetry` (Grafana 10.2.2 tarball, no checksum, pre-ADR-117) | **Script**: pin the two the stack still uses to compose's exact tags; drop the Loki pull. **`lxd/telemetry/`: delete outright** (git history keeps it), with `lxd/Makefile`'s `launch-telemetry` target, its `.PHONY` entry and help lines, and every live reference in docs and wiki. Dated records are not edited. | Answered 2026-11-12 (replaces the pin-and-sha256 default) |
| W5 | ioc-audit judgment for the new kinds | **Indicators** (the run fails): an account change, a crontab edit, an unsigned or out-of-tree module load. **Notes** (the run passes): sudo command counts per host and user, kernel OOM kills and segfaults. Tunable by parameter, like the existing limits. | Default applies |

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; must pass before going on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or create a file
- `[R]`: read or inspect a file
- `[S]`: needs sudo
- `[P]`: can run in parallel with other `[P]` phases
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. `WT=/home/govan/tmp/wt/S-WATCH-TRUE`, `REPO=/home/govan/tmp/unheaded`, `PROG=$REPO/docs/battle-plans/progress/S-WATCH-TRUE-progress.md` (in the worktree: `$WT/docs/battle-plans/progress/S-WATCH-TRUE-progress.md`).

---

## PHASE 0: PRECONDITIONS AND THE "BEFORE" READING (Steps 1-16)

**Goal**: Branch ready, defaults confirmed, and the trace collector's cost measured before anything changes.
**Prerequisite**: west up with the stack (VictoriaMetrics on `127.0.0.1:8428`).
**Time**: 30 min.
**Agent**: Coordinator.

- [ ] **Step 1** [R] ~10m: Read ADR-144 (all of it), `docs/battle-plans/progress/S-WIRE-HONEST-lat-progress.md`, this plan's Grounding table, `monitoring/prometheus/rules/{collector,zhen}.yml`, `config/vector/vector.yaml` lines 38-224, `runbooks/security/ioc-audit.yaml`.
- [ ] **Step 2** [B] ~1m: Worktree off develop.
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git worktree add /home/govan/tmp/wt/S-WATCH-TRUE -b sprint/s-watch-true develop && git -C /home/govan/tmp/wt/S-WATCH-TRUE log --oneline -1
  ```
- [ ] **Step 3** [W] ~3m: Copy the rulings for W1-W4 (answered 2026-11-12, table above) verbatim under "Decisions" in `$PROG`. If Stevie is present, ask whether he wants to tune W5 (one AskUserQuestion, default first, marked Recommended). If he is away, write "W5 default, Stevie away".
- [ ] **Step 4** [W] ~3m: Create `$PROG`: header, Decisions, a Log section, and a "For Stevie" section.
- [ ] **Step 5** [V] ~1m: `test -s /home/govan/tmp/wt/S-WATCH-TRUE/docs/battle-plans/progress/S-WATCH-TRUE-progress.md && grep -c '^## ' $_` prints at least 3.
- [ ] **Step 6** [B] ~1m: Tools present (never run a missing one; `lxc` installs a snap).
  ```bash
  for t in go cargo docker strace bpftool python3 curl; do command -v $t >/dev/null && echo "$t ok" || echo "$t MISSING"; done; test -x /var/tmp/unheaded-sprint/civenv/bin/pytest && echo "pytest ok"
  ```
- [ ] **Step 7** [V] ~1m: All ok. `strace` missing → the bpf() reading uses `sudo perf stat -e 'syscalls:sys_enter_bpf' -p PID -- sleep 10` instead. Note which one in `$PROG`.
- [ ] **Step 8** [B] ~2m: CPU before, from VictoriaMetrics (no root).
  ```bash
  q(){ curl -s -m5 -G http://127.0.0.1:8428/api/v1/query --data-urlencode "query=$1" | python3 -c 'import json,sys; [print(r["metric"].get("host"), r["value"][1]) for r in json.load(sys.stdin)["data"]["result"]]'; }
  q 'rate(process_cpu_seconds_total{job="trace-collector"}[30m])'
  q 'avg_over_time(rate(process_cpu_seconds_total{job="trace-collector"}[30m])[7d:1h])'
  systemctl show -p ActiveEnterTimestamp unheaded-trace-collector
  ```
- [ ] **Step 9** [W] ~1m: Log both hosts' numbers and the uptime in `$PROG` under "Before". Reference reading 2026-11-12 22:50Z: west 0.81 core at 19 h up (7-day mean 0.74), east 1.93 cores (7-day mean 1.91).
- [ ] **Step 10** [S][B] ~2m: bpf() call rate before (10 s; strace slows the collector while it runs).
  ```bash
  PID=$(systemctl show -p MainPID --value unheaded-trace-collector); sudo timeout 10 strace -c -f -e trace=bpf -p "$PID" 2>&1 | tail -4
  ```
- [ ] **Step 11** [V] ~1m: The `bpf` row has a call count. Divide it by 10 and log calls per second in `$PROG` (ADR-144 measured ~71,000/s).
  - If strace prints "Operation not permitted" → Step 12.
- [ ] **Step 12** [D] ~3m: `sudo sysctl kernel.yama.ptrace_scope`. A value of 3 forbids attaching even as root: use the `perf stat` line from Step 7, and log which tool was used.
- [ ] **Step 13** [S][B] ~2m: Map fill before (root reads the map once; about 10 s at full size).
  ```bash
  sudo bpftool map show name LATENCY_MAP; sudo bpftool map dump name LATENCY_MAP 2>/dev/null | grep -c '"key"'
  ```
- [ ] **Step 14** [B] ~1m: Latency events published before, from the logagg/trace counters the collector already serves.
  ```bash
  curl -s 127.0.0.1:16671/metrics | grep -E '^(latency_reader_|trace_collector_)' | grep -v _bucket | head -20
  ```
- [ ] **Step 15** [W] ~2m: Log Steps 10-14 in `$PROG` ("Before"): bpf/s, map entries (key 40 bytes, by Step 13's `key` size), latency events published. Expected: none on west (Grounding, D-17).
- [ ] **Step 16** [C][V] ~1m: **PHASE 0 EXIT GATE**: `$PROG` has Decisions and a "Before" with CPU, bpf/s and map entries for west, and CPU for east.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && git add docs/battle-plans/progress/S-WATCH-TRUE-progress.md && git commit -m "[PLAN S-WATCH-TRUE] Steps 1-16: decisions and the before reading"
  ```

---

## PHASE 1: LATENCY-PROBE: BOUNDED MAP, COUNTED FAILURES (Steps 17-36)

**Goal**: LATENCY_MAP is an LRU hash, so it cannot fill and new threads keep getting samples. A failed insert is counted in `LATENCY_STATS`. The comments say what the map holds.
**Prerequisite**: Phase 0 gate.
**Time**: 60 min.
**Agent**: Coordinator (verifier needs root).

- [ ] **Step 17** [R] ~3m: `ebpf/latency-probe/src/main.rs` lines 14, 24, 63-85, 302-388 and `src/common.rs`. Confirm that `aya-ebpf` in the workspace has `LruHashMap` (`grep -n 'pub struct LruHashMap' ~/.cargo/registry/src/*/aya-ebpf-0.1.1/src/maps/hash_map.rs`). It is the same crate, so nothing new is added.
- [ ] **Step 18** [W] ~5m: In `common.rs` add `/// Stats key: a LATENCY_MAP insert the kernel refused.` and `pub const STAT_MAP_INSERT_FAILED: u32 = 9;`. Add a host test `test_insert_failed_stat_is_next_free_key` that asserts it is 9 and below 16, the size of `LATENCY_STATS`.
- [ ] **Step 19** [B] ~2m: Red: run the host tests before `main.rs` uses the constant (the S-WIRE-HONEST recipe: there is no cargo path for these tests on the bpf target).
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE/ebpf/latency-probe/src && rustc --edition 2021 --test common.rs -o /tmp/claude-lp-common-test 2>&1 | tail -3 && /tmp/claude-lp-common-test 2>&1 | tail -3
  ```
- [ ] **Step 20** [V] ~1m: 5 passed (4 old, 1 new). This test only pins the key; the behaviour is proven by the verifier load and the map spec (Steps 25-29). Log it.
- [ ] **Step 21** [W] ~10m: `main.rs`:
  - `use aya_ebpf::maps::{HashMap, LruHashMap, RingBuf}`; `static LATENCY_MAP: LruHashMap<LatencyTraceKey, LatencyEntry> = LruHashMap::with_max_entries(MAX_LATENCY_MAP_ENTRIES, 0);`
  - Both `let _ = LATENCY_MAP.insert(...)` become `if LATENCY_MAP.insert(&map_key, &x, 0).is_err() { increment_stat(STAT_MAP_INSERT_FAILED); }`. Keep `increment_stat(STAT_MAP_UPDATES)` for successful updates only.
  - Line 14: "Aggregates per-thread RTT statistics in LATENCY_MAP (LRU: the least recently updated thread makes room)". Line 302: "Send latency event to userspace and update the thread's LATENCY_MAP entry."
  - Remove the local `STAT_*` duplicates the common constant replaces, if any. Leave `INFLIGHT` and `SOCK_TRACE` alone (Marshal: parked).
- [ ] **Step 22** [B] ~5m: Build into the load-gate target dir (never a plain `cargo build` in `ebpf/`: it replaces the xv6 boot ELF).
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE/ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p latency-probe 2>&1 | tail -3
  ```
- [ ] **Step 23** [V] ~1m: `Finished`, with no warning except the known toolchain `libLLVM ... dlopen` line.
  - If `LruHashMap` does not resolve → Step 24.
- [ ] **Step 24** [D] ~5m: `grep -rn 'LruHashMap' ~/.cargo/registry/src/*/aya-ebpf-*/src/maps/mod.rs`. Use the path the crate exports (`aya_ebpf::maps::hash_map::LruHashMap`). Still failing after two tries → `[STUCK]`; Phases 2-3 are `[BLOCKED]`; go to Phase 5.
- [ ] **Step 25** [S][B] ~15m: Verifier gate, all programs (needs root; take the shared lock).
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && P=/tmp/claude-swt && mkdir -p $P && flock /var/tmp/unheaded-sprint/ebpf-load.lock env CARGO_BUILD_JOBS=4 EBPF_INSNS_OUT=$P/swt.tsv sudo -E scripts/check-ebpf-loads.sh 2>&1 | tail -2 && grep '^latency-probe' $P/swt.tsv
  ```
- [ ] **Step 26** [V] ~2m: `[PASS] ... pass the verifier`, all six latency-probe programs load.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && scripts/check-ebpf-insns.sh --from /tmp/claude-swt/swt.tsv 2>&1 | tail -3
  ```
  - `[PASS]` → Step 28.
  - A latency-probe row grows more than 10 % → Step 27.
- [ ] **Step 27** [D] ~5m: Log before and after for the row under "For Stevie" in `$PROG` (S-WIRE-HONEST rule 17: a row over +10 % moves only on his word). Do not `--update` the baseline. Phases 2-3 go on; the merge at Step 90 waits for his word.
- [ ] **Step 28** [B] ~2m: Read the built object's map spec (no root): type LRUHash, key 24, value 40, max 65536.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && LATENCY_PROBE_OBJ=$PWD/ebpf/target/load-gate/default/bpfel-unknown-none/release/latency-probe go test ./cmd/trace-collector-go -run TestBuiltLatencyProbeSpec -count=1 2>&1 | tail -3
  ```
  (This test is written in Phase 2, Step 39. On a first pass, run this step after Step 41.)
- [ ] **Step 29** [V] ~1m: `ok`.
- [ ] **Step 30** [C] ~1m: **COMMIT**
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && git add ebpf/latency-probe/src/main.rs ebpf/latency-probe/src/common.rs && git commit -m "[PLAN S-WATCH-TRUE] Steps 17-30: LATENCY_MAP is an LRU hash; a refused insert is counted"
  ```
- [ ] **Step 31** [B] ~2m: `cargo clippy` on the probe crate, as the workspace allows for bpf (`cd ebpf && CARGO_TARGET_DIR=target/load-gate/default cargo clippy -p latency-probe --release 2>&1 | tail -5`).
- [ ] **Step 32** [V] ~1m: No warning that develop does not already have: run the same command in `/home/govan/tmp/unheaded/ebpf` and compare the warning counts.
- [ ] **Step 33** [B] ~1m: `grep -n 'per-flow' ebpf/latency-probe/src/*.rs cmd/trace-collector-go/latency_*.go pkg/protocol/bpfschema/bpfschema.go`
- [ ] **Step 34** [V] ~1m: No hit that describes LATENCY_MAP as per-flow. (Hits about flow-tracker's `FLOWS` map are right and stay.)
- [ ] **Step 35** [W] ~2m: Log in `$PROG`: verifier result, insn rows, map spec.
- [ ] **Step 36** [C][V] ~1m: **PHASE 1 EXIT GATE**: the probe builds, all 36 programs pass the verifier, the insn gate passes (or one row waits on Stevie, logged), the map is LRUHash 24/40/65536. Commit `$PROG` and any comment fix.

---

## PHASE 2: TRACE-COLLECTOR: BATCHED, SLOWER, LOUD (Steps 37-72)

**Goal**: One sweep every 2 s that reads LATENCY_MAP in batches (a few `bpf()` calls per sweep, not two per entry). A mismatched object refuses to load. Decode errors, map size, sweep time and the probe's own stats are all on `/metrics`.
**Prerequisite**: Phase 1 gate (Steps 39-41 need only the spec, not the verifier).
**Time**: 2.5 h.
**Agent**: Coordinator or one Developer agent.

### Spec check at load

- [ ] **Step 37** [R] ~5m: `cmd/trace-collector-go/latency_kprobe.go` (load path, lines 44-133), `latency_reader.go` (`LatencyTraceKeySize`, `rttTracker` 455-485), `main.go` 716-871.
- [ ] **Step 38** [W] ~10m: Red. `latency_kprobe_test.go`: `TestCheckLatencyMapSpec` table:
  - `{LRUHash, 24, 40}` → nil.
  - `{Hash, 24, 40}` → error naming "LRU".
  - `{LRUHash, 40, 40}` → error naming "key 40 bytes, want 24" (the west case).
  - `{LRUHash, 24, 32}` → error naming the value size.
  - nil spec → error.
- [ ] **Step 39** [W] ~5m: Red. `TestBuiltLatencyProbeSpec`: when `LATENCY_PROBE_OBJ` is set, load that ELF's `CollectionSpec` and run `checkLatencyMapSpec` on `LATENCY_MAP`. A set variable naming a missing file **fails** (never pass by skipping). An unset variable skips with a message that says how to run it.
- [ ] **Step 40** [B] ~2m: `cd /home/govan/tmp/wt/S-WATCH-TRUE && go test ./cmd/trace-collector-go -run 'TestCheckLatencyMapSpec|TestBuiltLatencyProbeSpec' -count=1 2>&1 | tail -5`
- [ ] **Step 41** [V] ~1m: Red: `undefined: checkLatencyMapSpec`.
- [ ] **Step 42** [W] ~10m: Green. `checkLatencyMapSpec(ms *ciliumebpf.MapSpec) error` in `latency_kprobe.go`. `LoadLatencyKprobes` calls it on `spec.Maps["LATENCY_MAP"]` **before** `NewCollection` and returns its error. main.go's existing branch then logs "latency_probe NOT loaded" and `programs_loaded{program="latency_probe"}` stays 0. A mismatched object is now one loud error at start, not silence.
- [ ] **Step 43** [B] ~2m: Step 40 again, then Step 28 with `LATENCY_PROBE_OBJ` set.
- [ ] **Step 44** [V] ~1m: Both `ok`. Also run it against the **installed** object, which must be refused:
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && LATENCY_PROBE_OBJ=/usr/local/lib/unheaded/ebpf/latency-probe go test ./cmd/trace-collector-go -run TestBuiltLatencyProbeSpec -count=1 2>&1 | grep -E 'key 40|FAIL' | head -2
  ```
  Expected: FAIL naming "key 40 bytes". That is the proof the check catches west's current state. Log it.
- [ ] **Step 45** [C] ~1m: **COMMIT** `latency_kprobe.go latency_kprobe_test.go`: "Steps 37-45: the collector refuses a LATENCY_MAP that is not the LRU 24/40 layout".

### Batched sweep

- [ ] **Step 46** [W] ~15m: Red. New `latency_sweep_test.go`. A `latencyMap` interface, `{ BatchLookup(*ciliumebpf.MapBatchCursor, any, any, *ciliumebpf.BatchOptions) (int, error); Iterate() *ciliumebpf.MapIterator; KeySize() uint32; ValueSize() uint32 }`, or the narrower function seam the Developer prefers. A fake returns N entries in batches. Tests:
  - `TestSweepBatches`: 10,000 entries with batch size 4096 → three `BatchLookup` calls, 10,000 entries visited.
  - `TestSweepFallsBackWhenBatchUnsupported`: `ErrNotSupported` → one `Iterate` walk, same entries, and the fallback is logged once.
  - `TestSweepCountsDecodeErrors`: entries with a 40-byte key → `trace_collector_latency_decode_errors_total` += N, nothing published.
  - `TestSweepPartialErrorKeepsTracker`: a mid-batch error → `endSweep` is not called (the existing rule at main.go:866).
  - `TestSweepPublishesOnlyFresh`: the existing `rtts.fresh` behaviour, kept.
  - `TestSweepSetsEntriesGauge`: `trace_collector_latency_map_entries` equals the entries visited.
- [ ] **Step 47** [B] ~2m: `go test ./cmd/trace-collector-go -run TestSweep -count=1 2>&1 | tail -5`
- [ ] **Step 48** [V] ~1m: Red, undefined symbols.
- [ ] **Step 49** [W] ~25m: Green. `latency_sweep.go`: `sweepLatency(ctx, m, rtts, publish func([]byte), now func() time.Time) error`. Reuse one key and one value buffer of 4096 entries across sweeps (no per-sweep allocation of the batch). Decode, `fresh`, marshal and publish as main.go does today. Metrics: `trace_collector_latency_sweep_seconds` (histogram, buckets 1 ms to 5 s), `trace_collector_latency_map_entries` (gauge), `trace_collector_latency_decode_errors_total` (counter). Register them through `unheaded/pkg/metrics/prom` as the file's neighbours do.
- [ ] **Step 50** [B] ~2m: Step 47 again, with `-race`.
- [ ] **Step 51** [V] ~1m: `ok`, 6 tests.
- [ ] **Step 52** [W] ~10m: main.go: replace the inline goroutine (lines 828-870) with a ticker on a new flag `--latency-sweep-interval` (default **2s**, minimum 500ms, refused below that at flag parse) that calls `sweepLatency`. Remove the now-duplicate start-up log line (one line: "latency sweep started", with interval and batch size). The type of `directLatencyMap` becomes `*ciliumebpf.Map`.
- [ ] **Step 53** [W] ~5m: Red then green: `main_test.go` `TestLatencySweepIntervalFlag`: default 2s; `--latency-sweep-interval=100ms` refused with a message naming the minimum.
- [ ] **Step 54** [B] ~3m: Full package, race on.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE && gofmt -l cmd/trace-collector-go; go vet ./cmd/trace-collector-go && go test -race -count=1 ./cmd/trace-collector-go/... ./pkg/protocol/bpfschema/... 2>&1 | tail -4
  ```
- [ ] **Step 55** [V] ~1m: gofmt prints nothing, vet clean, both `ok`.
  - Fail → Step 56.
- [ ] **Step 56** [D] ~5m: Read the first failing test. A data race on the reused buffers means a publish kept a slice past the sweep: copy before publishing. Two attempts, then `[STUCK]`.
- [ ] **Step 57** [C] ~1m: **COMMIT** `latency_sweep.go latency_sweep_test.go main.go main_test.go`: "Steps 46-57: LATENCY_MAP read in batches every 2 s; decode errors, entries and sweep time on /metrics".

### Probe stats on /metrics

- [ ] **Step 58** [W] ~10m: Red. `latency_stats_test.go`: a `prom.Collector` over a fake `LATENCY_STATS` reader emits `trace_collector_latency_probe_events_total{stat}` as **counters** for keys 0-9 by fixed names (`send_enter`, `send_exit`, `recv_enter`, `recv_exit`, `connect_enter`, `connect_exit`, `events_sent`, `events_dropped`, `map_updates`, `map_insert_failed`). An unknown key is ignored, never minted as a label. A read error emits nothing and counts `trace_collector_latency_stats_read_errors_total`.
- [ ] **Step 59** [W] ~10m: Green. `LoadLatencyKprobes` also returns `coll.Maps["LATENCY_STATS"]`. The loader struct gets a field instead of a third return value, if that reads cleaner. Register the collector when the probe loads.
- [ ] **Step 60** [B] ~2m: Step 54's command again.
- [ ] **Step 61** [V] ~1m: `ok`.
- [ ] **Step 62** [C] ~1m: **COMMIT** `latency_stats.go latency_stats_test.go latency_kprobe.go main.go`: "Steps 58-62: latency-probe's own counters on /metrics, map_insert_failed among them".

### Benchmark and doc

- [ ] **Step 63** [W] ~10m: `benchmark_test.go`: `BenchmarkSweepLatency65536` against the fake map, full and all fresh, then all stale.
- [ ] **Step 64** [B] ~3m: `go test -run '^$' -bench SweepLatency -benchmem ./cmd/trace-collector-go 2>&1 | tail -4`
- [ ] **Step 65** [V] ~1m: Logged in `$PROG` (ns/op, B/op). No bar here: the live bar is Phase 4. Allocations per sweep must not scale with batches (one buffer).
- [ ] **Step 66** [W] ~5m: The unit file comment (`deploy/systemd/unheaded-trace-collector.service` lines 6-12): "build the binary **and** `latency-probe` from the same commit; install both, then restart. The collector refuses an object whose LATENCY_MAP is not the LRU 24/40 layout." Add the install line for the probe object from `ebpf/target/load-gate/default/bpfel-unknown-none/release/`.
- [ ] **Step 67** [W] ~5m: `wiki/Profiling.md` (or the trace-collector wiki page if one exists: `ls wiki | grep -i trace`): the three new series, `--latency-sweep-interval`, and how to read `map_insert_failed`.
- [ ] **Step 68** [B] ~1m: Doc gates the repo runs on wiki and units: `scripts/check-spdx.sh 2>&1 | tail -1; ls scripts/check-unit-*.sh | xargs -n1 bash 2>&1 | grep -E 'PASS|FAIL' | head`
- [ ] **Step 69** [V] ~1m: No FAIL that names a file this phase touched.
- [ ] **Step 70** [C] ~1m: **COMMIT** `benchmark_test.go deploy/systemd/unheaded-trace-collector.service wiki/<page>.md`.
- [ ] **Step 71** [B] ~2m: `go build -o /tmp/claude-swt/trace-collector-go ./cmd/trace-collector-go && /tmp/claude-swt/trace-collector-go --help 2>&1 | grep -A1 latency-sweep-interval`
- [ ] **Step 72** [V] ~1m: **PHASE 2 EXIT GATE**: the package is green under `-race`. The built binary shows the flag. `TestBuiltLatencyProbeSpec` passes on the new object and fails on the installed one. Commit `$PROG`.

---

## PHASE 3: THE RECORD: FND-044 AND ADR-144 (Steps 73-82)

**Goal**: The defect is in the findings register, and ADR-144 carries a dated section with the corrections and the decision for item 1.
**Prerequisite**: Phase 2 gate (the fix exists to point at).
**Time**: 30 min.
**Agent**: Coordinator.

- [ ] **Step 73** [B] ~1m: Next free id: `grep -o 'FND-[0-9]*' compliance/findings/register.yaml | sort -u | tail -1` → FND-043, so FND-044. Run `grep -rn 'FND-044' --exclude-dir=.claude .` and expect nothing.
- [ ] **Step 74** [W] ~10m: Append FND-044 after FND-043, schema as the file header says. Fields:
  - **title**: "The eBPF trace collector sweeps LATENCY_MAP two system calls per entry every 200 ms; the map never shrinks and drops new threads silently when full; west's object and binary disagree on the key"
  - **verify**: "on each host, `rate(process_cpu_seconds_total{job="trace-collector"}[1h])` under 0.05 after 6 h up; `trace_collector_latency_map_entries` present; `trace_collector_latency_decode_errors_total` 0; the installed latency-probe passes `checkLatencyMapSpec`"
  - **severity**: medium. Availability and observability: one host loses most of a core, and latency monitoring is blind on west. No confidentiality impact.
  - **class**: app; **opened**: 2026-11-12; **lockout**: none; **step**: 2
  - **exposure**: the measurements from ADR-144 and Grounding
  - **plan**: "Stevie 2026-11-12: fix (ADR-144 item 1, freeze lifted for it) and record. S-WATCH-TRUE Phases 1-4; east in Phase 4B, attended."
- [ ] **Step 75** [B] ~2m: Register validator: `go test -count=1 ./cmd/compliance-evidence/... 2>&1 | tail -3`
- [ ] **Step 76** [V] ~1m: `ok`.
  - Fail → read the schema error, fix the field. Two tries.
- [ ] **Step 77** [W] ~10m: ADR-144, new section `## Update 2026-11-12 (S-WATCH-TRUE)`. The earlier text is a dated record and is not edited. Content:
  - one reader, not two (the two log lines are one branch);
  - west's object/binary key mismatch, so west published no latency events;
  - `LATENCY_EVENTS` written and never read;
  - `INFLIGHT` and `SOCK_TRACE` also drop on full (parked);
  - the decision for item 1: LRU plus batched 2 s sweep plus counters, with the ring buffer reader parked as the next step;
  - the status line stays Proposed (parked) for items 2-7.
- [ ] **Step 78** [B] ~1m: `grep -n 'FND-044' compliance/findings/register.yaml docs/adr/ADR-144*.md`
- [ ] **Step 79** [V] ~1m: Two or more hits.
- [ ] **Step 80** [B] ~2m: Index row unchanged in status: `grep -n 'ADR-144' docs/adr/ADR-INDEX.md`. Still "Proposed". Leave it as it is: only item 1 moved.
- [ ] **Step 81** [C] ~1m: **COMMIT** `compliance/findings/register.yaml docs/adr/ADR-144-*.md docs/battle-plans/progress/S-WATCH-TRUE-progress.md`: "Steps 73-81: FND-044 recorded; ADR-144 dated section".
- [ ] **Step 82** [V] ~1m: **PHASE 3 EXIT GATE**: FND-044 validates; ADR-144 has the dated section.

---

## PHASE 4: DEPLOY ON WEST AND THE "AFTER" READING (Steps 83-110)

**Goal**: West runs the new binary and object from one commit, and the pass bar is met.
**Prerequisite**: Phases 1-3 gates. Stevie's word if Step 27 is waiting on him.
**Time**: 45 min of work plus a 6 h soak (do Phases 5-9 meanwhile).
**Agent**: Coordinator, sudo.

**Pass bar (west).** Taken at least 6 h after the restart:

| Reading | Before (2026-11-12) | Bar |
|---|---|---|
| `rate(process_cpu_seconds_total{job="trace-collector",host="west"}[1h])` | 0.81 core at 19 h, rising with uptime | **≤ 0.05** |
| Growth: 1 h rate at 6 h ÷ 1 h rate at 1 h | ~3x (0.16 → 0.48 over 3 h, ADR-144) | **≤ 1.5** |
| `bpf()` calls per second (strace, 10 s) | ~71,000 | **≤ 2,000** |
| latency events published (`trace_collector_latency_map_entries` > 0 and Wotan `traces.latency` / panel non-empty) | 0 (decode mismatch) | **> 0** while traffic flows |
| `trace_collector_latency_decode_errors_total` | not counted | **0** |
| `trace_collector_latency_probe_events_total{stat="map_insert_failed"}` | not counted | **present; 0** |
| `trace_collector_latency_map_entries` | ~43,900 (ADR-144), 40-byte keys | **≤ 65,536** |

- [ ] **Step 83** [B] ~2m: Merge to develop (the live configs are bind-mounted from the main checkout, so live means merged).
  ```bash
  cd /home/govan/tmp/unheaded && git switch develop && git merge --ff-only sprint/s-watch-true && git log --oneline -1
  ```
- [ ] **Step 84** [D] ~3m: Not fast-forward (S-RESIDUE merged first) → `git -C /home/govan/tmp/wt/S-WATCH-TRUE rebase develop`, rerun Step 54, then Step 83.
- [ ] **Step 85** [B] ~3m: Build both from that one commit.
  ```bash
  cd /home/govan/tmp/unheaded && C=$(git rev-parse --short HEAD) && go build -o /tmp/claude-swt/trace-collector-go ./cmd/trace-collector-go && (cd ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p latency-probe 2>&1 | tail -1) && echo "commit $C"
  ```
- [ ] **Step 86** [V] ~1m: `LATENCY_PROBE_OBJ=/home/govan/tmp/unheaded/ebpf/target/load-gate/default/bpfel-unknown-none/release/latency-probe go test ./cmd/trace-collector-go -run TestBuiltLatencyProbeSpec -count=1` → `ok`.
- [ ] **Step 87** [S][B] ~2m: Keep the old pair for rollback.
  ```bash
  sudo install -d -m 0750 /var/backups/trace-collector-2026-11-12 && sudo cp -p /usr/local/bin/trace-collector-go /usr/local/lib/unheaded/ebpf/latency-probe /var/backups/trace-collector-2026-11-12/ && sudo ls -la /var/backups/trace-collector-2026-11-12/
  ```
- [ ] **Step 88** [S][B] ~2m: Install both (root-owned, as the loader requires), then restart.
  ```bash
  sudo install -m 0755 /tmp/claude-swt/trace-collector-go /usr/local/bin/trace-collector-go && sudo install -m 0644 /home/govan/tmp/unheaded/ebpf/target/load-gate/default/bpfel-unknown-none/release/latency-probe /usr/local/lib/unheaded/ebpf/latency-probe && sudo systemctl restart unheaded-trace-collector && date -u +%FT%TZ
  ```
- [ ] **Step 89** [W] ~1m: Restart time in `$PROG`.
- [ ] **Step 90** [V] ~2m: Started clean, all three programs loaded.
  ```bash
  systemctl is-active unheaded-trace-collector; journalctl -u unheaded-trace-collector --since -2min --no-pager | grep -E 'latency|NOT loaded|error' | head -8; curl -s 127.0.0.1:16671/metrics | grep -E '^programs_loaded|^trace_collector_programs_loaded' | head
  ```
  - `active`, "latency sweep started", latency_probe loaded = 1 → Step 92.
  - "NOT loaded" or restart loop → Step 91.
- [ ] **Step 91** [D] ~5m: Read the error. Spec refused → the wrong object was installed (rerun Step 86 against `/usr/local/lib/unheaded/ebpf/latency-probe`). Verifier refused under the unit's capabilities → compare with Step 25's root load. Not fixed in two tries → **roll back**:
  ```bash
  sudo cp -p /var/backups/trace-collector-2026-11-12/trace-collector-go /usr/local/bin/ && sudo cp -p /var/backups/trace-collector-2026-11-12/latency-probe /usr/local/lib/unheaded/ebpf/ && sudo systemctl restart unheaded-trace-collector
  ```
  Then `[STUCK]`.
- [ ] **Step 92** [B] ~2m: First readings after 10 minutes.
  ```bash
  curl -s 127.0.0.1:16671/metrics | grep -E '^trace_collector_latency_(map_entries|decode_errors_total|probe_events_total)' | head -14
  ```
- [ ] **Step 93** [V] ~1m: `decode_errors_total 0`. `map_entries` above 0. `probe_events_total{stat="map_updates"}` rising between two reads a minute apart.
- [ ] **Step 94** [B] ~2m: Latency reaches Wotan subscribers. Use the dashboard's latency view, or whichever check `wiki/Profiling.md` names for `traces.latency` (no Wotan wildcard read as govan: FND-043).
- [ ] **Step 95** [V] ~1m: Non-empty. Log it.
- [ ] **Step 96** [C] ~1m: **COMMIT** `$PROG` (on develop or the sprint branch, whichever is checked out in the main checkout; named file only): "Steps 83-96: west runs the probe and collector from one commit".
- [ ] **Step 97** [W] ~1m: Note in `docs/battle-plans/progress/S-E1-LIVE-progress.md`, log store week section: "trace-collector restarted <time> (S-WATCH-TRUE); not a log store component".
- [ ] **Step 98** [B] ~2m: **1 h reading.** Step 8's query with `[1h]`, west only.
- [ ] **Step 99** [W] ~1m: Logged.
- [ ] **Step 100** [S][B] ~2m: bpf() rate after (Step 10's command).
- [ ] **Step 101** [V] ~1m: ≤ 2,000/s. Logged.
  - Over → Step 102.
- [ ] **Step 102** [D] ~10m: `ErrNotSupported` fallback active? `journalctl -u unheaded-trace-collector | grep -i fallback`. If the fallback is on, find out why the batch call is refused on a 6.17 kernel; the likely cause is the map type flags. Other `bpf()` sources (flow_reader at 1 s, packet ring) are counted in the total: separate them with `strace -e trace=bpf -e bpf=...` or by `-k` stacks. A batch path that is on and still over the bar → `[STUCK]` with the numbers.
- [ ] **Step 103** [B] ~1m: Wait (Phases 5-9 run now). **6 h reading**: CPU `[1h]` and the growth ratio against Step 98.
- [ ] **Step 104** [V] ~2m: **Pass bar met** (every row of the table). Log each reading against its bar in `$PROG`.
  - A row fails → Step 105.
- [ ] **Step 105** [D] ~10m: CPU over the bar with bpf/s under it → profile: `--pprof` per `wiki/Profiling.md` for 30 s. The likely costs are JSON marshal and publish per fresh entry. Log the top 5 frames. If it stays over the bar after one change, `[STUCK]`. Parked fix: ADR-144's ring-buffer reader.
- [ ] **Step 106** [C] ~1m: **COMMIT** `$PROG`: "Steps 97-106: west after-reading, pass bar <met|not met>".
- [ ] **Step 107** [R] ~2m: Grafana trace/latency panels: no query names a series this phase removed (none was removed; the new ones are additive).
- [ ] **Step 108** [B] ~1m: `sudo rm -r /var/backups/trace-collector-2026-11-12` only after Phase 4B, or after 3 days of the bar holding. Not now: write it under "For Stevie / housekeeping".
- [ ] **Step 109** [W] ~2m: FND-044's verify needs both hosts: note "west met <date>; east Phase 4B".
- [ ] **Step 110** [V] ~1m: **PHASE 4 EXIT GATE**: the pass bar is met on west with logged numbers. The rollback copy is kept.

## PHASE 4B: EAST (ATTENDED) (Steps 111-118)

**Goal**: East runs the same pair; its 1.93 cores come back.
**Prerequisite**: Phase 4 gate. Stevie present (east host change, his rule).
**Time**: 20 min plus a 6 h soak.
**Agent**: Coordinator with Stevie.

- [ ] **Step 111** [STEVIE] ~1m: "Deploy the trace collector fix on east now?" A no → the steps below are `[BLOCKED]`; FND-044 stays open for east.
- [ ] **Step 112** [B] ~2m: Copy the pair to east: `scp /tmp/claude-swt/trace-collector-go /home/govan/tmp/unheaded/ebpf/target/load-gate/default/bpfel-unknown-none/release/latency-probe east:/tmp/`
- [ ] **Step 113** [S][B] ~3m: On east: back up the old pair as Step 87 did, install both as Step 88 did, restart. `TRACE_IFACE=lo` stays in its env file.
- [ ] **Step 114** [V] ~2m: Step 90's checks over `ssh east`.
- [ ] **Step 115** [D] ~5m: Same as Step 91, on east; roll back on east the same way.
- [ ] **Step 116** [B] ~1m: 6 h later: Step 8's query, east row.
- [ ] **Step 117** [V] ~1m: East ≤ 0.10 core (4-core host, busier loopback). Logged against 1.93.
- [ ] **Step 118** [C] ~1m: **COMMIT** `$PROG` and FND-044's verify note. The entry is not removed by hand: the register says evidence decides. **PHASE 4B EXIT GATE**.

---

## PHASE 5: WOTAN REFUSAL ALERT (Steps 119-142) [P]

**Goal**: A refused topic read or publish names its caller on `/metrics` and raises a ticket within 15 minutes, with a runbook that says what to do.
**Prerequisite**: Phase 0 gate. Independent of Phases 1-4.
**Time**: 1.5 h.
**Agent**: Developer agent `[P]` (own worktree `/home/govan/tmp/wt/S-WATCH-TRUE-p5` off `sprint/s-watch-true`, merged back at Step 141).

- [ ] **Step 119** [R] ~5m: `services/wotan/internal/api/topic_permissions.go` 180-260, `topic_permissions_test.go`, `configs/wotan.yaml` `topics.permissions`.
- [ ] **Step 120** [W] ~10m: Red. `TestRefusalCounterNamesCaller`: a refused service identity listed in `topics.permissions` counts under `caller="<its name>"`. An identity not in the list (a user token, an API key) counts under `caller="other"`. Label values never come from the pattern or the request.
- [ ] **Step 121** [B] ~2m: `cd /home/govan/tmp/wt/S-WATCH-TRUE-p5 && go test -race -count=1 ./services/wotan/internal/api -run TestRefusalCounterNamesCaller 2>&1 | tail -3`
- [ ] **Step 122** [V] ~1m: Red (label count mismatch).
- [ ] **Step 123** [W] ~10m: Green. Labels `{action, outcome, caller}`; `caller` comes from a `compiledPermissions` method `knows(subject) bool`. The Help text says the label is bounded by the config's identities.
- [ ] **Step 124** [B] ~2m: The package and its dependants: `go test -race -count=1 ./services/wotan/... 2>&1 | tail -5`
- [ ] **Step 125** [V] ~1m: All `ok`.
- [ ] **Step 126** [C] ~1m: **COMMIT** `topic_permissions.go topic_permissions_test.go`: "Steps 119-126: Wotan's refusal counter names the caller (bounded)".
- [ ] **Step 127** [W] ~10m: `monitoring/prometheus/rules/wotan.yml` (new; header comment in the house style: producer, scrape job, why):
  ```yaml
  groups:
    - name: wotan-permissions
      rules:
        - alert: WotanTopicPermissionRefused
          expr: sum by (job, instance, action, outcome, caller) (increase(unheaded_wotan_topic_permission_refusals_total[15m])) > 0
          labels: {severity: warning, urgency: ticket}
          annotations:
            summary: "Wotan refused {{ $labels.caller }} a topic {{ $labels.action }} ({{ $labels.outcome }})"
            description: "A service asked for a topic outside configs/wotan.yaml topics.permissions; under enforce it gets nothing and retries"
            runbook_url: "runbooks/observe/wotan-topic-refused.yaml"
  ```
  This is Stevie's expression, scoped by `sum by` so one ticket names one caller. The `audited` outcome alerts too: in audit mode that is the warning before enforce.
- [ ] **Step 128** [W] ~15m: `monitoring/prometheus/rules/tests/wotan_test.yml`, four cases:
  - a flat counter (like west's `2` for 16 h) → quiet;
  - one step up → fires within 15 m with `caller` set;
  - a counter reset on restart → no false fire (`increase` handles resets; prove it);
  - `audited` → fires with `outcome: audited`.
- [ ] **Step 129** [W] ~15m: `runbooks/observe/wotan-topic-refused.yaml`, in the format of `zhen-scheduler-stalled.yaml` (`status: draft`, `authored_by: claude`, empty approvers, `requires_sudo: false`). Steps:
  1. which caller, from `ALERTS`;
  2. Wotan's log lines for that caller in the last 30 min (`docker logs unheaded-wotan --since 30m 2>&1 | grep -F "<caller>"`; the log names the pattern);
  3. the caller's list in `configs/wotan.yaml`;
  4. decide: the service is new or changed → add its prefix in the file **in place** (never `sed -i`) and `docker kill -s HUP unheaded-wotan`; not expected → `runbooks/security/incident-response.yaml`.
  The notes tell the 2026-11-12 Sophia story in two lines.
- [ ] **Step 130** [W] ~3m: `observability/alerting/policy.yaml`: `WotanTopicPermissionRefused` / `urgency: ticket` / reason: "Under unlisted: deny a missing topic is silent: Sophia was refused 314 times in 2.5 h on 2026-11-12 and nothing showed it but Wotan's log. No SLO sees a service that is up and starved."
- [ ] **Step 131** [B] ~2m: `cd /home/govan/tmp/wt/S-WATCH-TRUE-p5 && scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256`
- [ ] **Step 132** [V] ~1m: One line added (the new runbook), nothing else changed.
- [ ] **Step 133** [B] ~5m: Gates.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE-p5 && scripts/check-prometheus-rules.sh 2>&1 | tail -3 && /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/runbooks/test_alert_runbooks.py 2>&1 | tail -2 && go test -count=1 ./cmd/alert-policy/... 2>&1 | tail -2
  ```
- [ ] **Step 134** [V] ~1m: All pass. The new rule file is covered by its test (`check-prometheus-rules.sh` fails an untested file).
  - Fail → Step 135.
- [ ] **Step 135** [D] ~5m: Untested-file FAIL → the test's `rule_files` line must be exactly `  - ../wotan.yml`. Runbook-link FAIL → the path in `runbook_url`. Policy FAIL → the alert name matches exactly. Two tries.
- [ ] **Step 136** [C] ~1m: **COMMIT** `monitoring/prometheus/rules/wotan.yml monitoring/prometheus/rules/tests/wotan_test.yml runbooks/observe/wotan-topic-refused.yaml runbooks/MANIFEST.sha256 observability/alerting/policy.yaml`.
- [ ] **Step 137** [B] ~2m: Merge p5 into the sprint branch: `cd /home/govan/tmp/wt/S-WATCH-TRUE && git merge --ff-only sprint/s-watch-true-p5 || git merge -S sprint/s-watch-true-p5`. Then merge the sprint branch into develop as in Step 83.
- [ ] **Step 138** [S][B] ~5m: Live. Rebuild and recreate Wotan (the caller label) the way the night session did (`sudo docker compose build wotan && sudo docker compose up -d wotan`), then restart Prometheus. A single-file bind mount keeps the old inode after a git checkout, and a restart re-binds the path: `sudo docker restart unheaded-prometheus`.
- [ ] **Step 139** [V] ~3m: Wotan healthy and every client reconnected (`docker ps --filter name=unheaded- --format '{{.Names}} {{.Status}}' | grep -vc healthy` prints 0 or the known non-healthchecked ones). The rule is loaded: `curl -s localhost:9090/api/v1/rules | grep -c WotanTopicPermissionRefused` ≥ 1. The counter has `caller` (Step 8's `q`).
- [ ] **Step 140** [V] ~1m: The alert is quiet (no refusal since 06:46Z).
- [ ] **Step 141** [C] ~1m: **COMMIT** `$PROG`. Note Wotan's recreate time in the S-E1-LIVE log store week section.
- [ ] **Step 142** [V] ~1m: **PHASE 5 EXIT GATE**: rule loaded, tested, runbook in the manifest (unsealed), quiet on west, label live.

---

## PHASE 6: SCRAPE TRUTH (D-13) (Steps 143-158) [P]

**Goal**: No scrape job for a thing that is not meant to run. The scheduler alert says which case it is.
**Prerequisite**: Phase 0 gate (W1, W2).
**Time**: 45 min.
**Agent**: Agent `[P]`, worktree `-p6`.

- [ ] **Step 143** [W] ~5m: W1 (answered 2026-11-12): delete the `sword` job and its comment from `config/victoria/scrape.yml`. First confirm nothing reads it: `grep -rn 'sword_\|job="sword"' config/grafana monitoring/prometheus/rules observability` prints nothing.
- [ ] **Step 144** [W] ~10m: W2 (answered 2026-11-12; heimdall's code stays): delete the `heimdall` group from `monitoring/prometheus/rules/collector.yml` and its cases from `tests/collector_test.yml`. Delete the `heimdall` job from `config/victoria/scrape.yml` and `monitoring/prometheus/prometheus.yml`. In `collector.yml`'s header, add: "heimdall's two rules were removed 2026-11-12 (S-WATCH-TRUE, W2): no host runs heimdall-daemon. A rule returns with its producer and a test (decision 25). Runbooks kept." Keep `runbooks/infra/heimdall-*.yaml`.
- [ ] **Step 145** [W] ~3m: `scrape.yml`'s "three below read up == 0" comment: syslog-relay is up since 2026-11-12 (`8efe36e2`) and zhen-agentd is up since 2026-11-12. Rewrite the comment to say what is true now. With heimdall gone, no job is expected down.
- [ ] **Step 146** [W] ~10m: `zhen.yml`: split `ZhenSchedulerStalled` into two rules with the same alert name, one with `labels: {reason: absent}` (`absent_over_time(...[15m])`) and one with `labels: {reason: stale}` (the `time() - max ... > 900` arm). Descriptions say which. In the runbook's notes, the first step reads `reason`.
- [ ] **Step 147** [W] ~5m: `tests/zhen_test.yml`: the existing stale case expects `reason: stale`; add an absent case expecting `reason: absent`.
- [ ] **Step 148** [B] ~3m: `scripts/runbook-manifest.sh --write` (zhen-scheduler-stalled changed), then Step 133's gate line.
- [ ] **Step 149** [V] ~1m: Pass. The manifest diff is one changed line.
- [ ] **Step 150** [B] ~2m: Search for anything else that counts `heimdall`/`sword` targets: `grep -rn 'heimdall\|19101\|19011' tests/ scripts/check-*.sh observability/ | grep -v runbooks | head`
- [ ] **Step 151** [V] ~1m: No remaining reader. A test that pins the job list (if one exists) is updated in the same commit.
- [ ] **Step 152** [C] ~1m: **COMMIT** the touched files by name.
- [ ] **Step 153** [B] ~2m: Merge as in Step 137.
- [ ] **Step 154** [S][B] ~2m: `sudo docker restart unheaded-victoria unheaded-prometheus` (re-binds the single-file mounts).
- [ ] **Step 155** [V] ~3m: No down targets in either.
  ```bash
  for u in http://127.0.0.1:8428 http://127.0.0.1:9090; do curl -s -m5 "$u/api/v1/targets" | python3 -c 'import json,sys; d=json.load(sys.stdin)["data"]["activeTargets"]; print(sys.argv[1], len(d), "targets,", [t["labels"].get("job") for t in d if t["health"]!="up"])' $u; done
  ```
  Expected: an empty list on both.
- [ ] **Step 156** [D] ~5m: A target is down that is not heimdall or sword → it is a real outage, not this phase's: log it under "For Stevie", don't fix it here.
- [ ] **Step 157** [V] ~1m: `ZhenSchedulerStalled` is quiet (zhen-agentd is beating, paused).
- [ ] **Step 158** [C][V] ~1m: **PHASE 6 EXIT GATE**: zero down targets on both, both rule files tested. Commit `$PROG`.

---

## PHASE 7: DASHBOARDS AND THE CUT-OVER CHECKS (D-14) (Steps 159-172) [P]

**Goal**: No panel queries a series that is never emitted. `09_compliance` shows the baseline's numbers. The three cut-over checks the agent skipped are run.
**Prerequisite**: Phase 0 gate.
**Time**: 45 min.
**Agent**: Agent `[P]`, worktree `-p7`; Step 166 needs sudo.

- [ ] **Step 159** [W] ~10m: `00_kingdom_services.json`: remove the panel or panels whose targets are `wotan_idempotency_entries` and `rate(wotan_idempotency_hits_total[1m])`, around line 662. Edit the JSON with a short Python script (load, filter `panels`, dump with the file's indent), not by hand. Keep the `gridPos` of the panels below consistent: shift `y` only if the panel was alone on its row.
- [ ] **Step 160** [B] ~1m: `python3 -c 'import json;json.load(open("config/grafana/provisioning/dashboards/kingdom/00_kingdom_services.json"))' && grep -c wotan_idempotency config/grafana/provisioning/dashboards/kingdom/00_kingdom_services.json`
- [ ] **Step 161** [V] ~1m: Valid JSON, count 0.
- [ ] **Step 162** [W] ~5m: `deploy/systemd/unheaded-baseline.service`: `ExecStart=... -textfile-dir /var/lib/unheaded/textfile`, `ReadWritePaths=/var/lib/unheaded/textfile` (if the unit uses `ProtectSystem=strict`, check `grep -n ProtectSystem`), and an install comment like `unheaded-ci-status.service:10`: `sudo setfacl -m u:unheaded-baseline:rwx /var/lib/unheaded/textfile`.
- [ ] **Step 163** [B] ~3m: Unit gates and the baseline's own tests: `go test -race -count=1 ./cmd/unheaded-baseline/... 2>&1 | tail -2; ls scripts/check-unit-*.sh | xargs -n1 bash 2>&1 | grep -E 'FAIL' | head`
- [ ] **Step 164** [V] ~1m: `ok`, no FAIL naming the unit.
- [ ] **Step 165** [C] ~1m: **COMMIT** `00_kingdom_services.json deploy/systemd/unheaded-baseline.service`.
- [ ] **Step 166** [S][B] ~3m: Live: merge (Step 137 pattern), then:
  ```bash
  sudo setfacl -m u:unheaded-baseline:rwx /var/lib/unheaded/textfile && sudo cp /home/govan/tmp/unheaded/deploy/systemd/unheaded-baseline.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl start unheaded-baseline.service && sleep 5 && ls -la /var/lib/unheaded/textfile/ | grep baseline
  ```
- [ ] **Step 167** [V] ~3m: `unheaded_baseline.prom` exists, owned by `unheaded-baseline`. Within two scrapes VictoriaMetrics has `unheaded_baseline_*` for `host="west"` (Step 8's `q` with `count by (__name__) ({__name__=~"unheaded_baseline_.*"})`). 8 series, or as many as `09_compliance` queries.
  - Missing → Step 168.
- [ ] **Step 168** [D] ~5m: `journalctl -u unheaded-baseline -n 20`: permission denied → the ACL, or `ReadWritePaths`. The file is there but no series → huginn reads another directory: `systemctl cat huginn | grep -i textfile`. Two tries.
- [ ] **Step 169** [B] ~5m: The three cut-over checks from todo 19:
  ```bash
  cd /home/govan/tmp/unheaded && /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_image_digests.py 2>&1 | tail -1; scripts/check-prometheus-rules.sh 2>&1 | tail -1; scripts/check-gitleaks.sh 2>&1 | tail -1
  ```
- [ ] **Step 170** [V] ~1m: Three passes, each read on its own line before going on.
  - A FAIL → log it under "For Stevie" with the line. Fix it in this phase only if it is this plan's file.
- [ ] **Step 171** [W] ~2m: Log in `$PROG`.
- [ ] **Step 172** [C][V] ~1m: **PHASE 7 EXIT GATE**: idempotency panels gone; `unheaded_baseline_*` in VictoriaMetrics; the three checks recorded. Commit `$PROG`.

---

## PHASE 8: WHAT THE GATES DO NOT READ (D-15) (Steps 173-184) [P]

**Goal**: The load-balancer compose file names its bytes and the digest gate reads it. `bootstrap-llm-lab.sh` is pinned, and `lxd/telemetry/` is gone with every live reference to it (W4). The override file is on record (W3).
**Prerequisite**: Phase 0 gate (W3, W4). Network to the registry for the digests.
**Time**: 1 h.
**Agent**: Agent `[P]`, worktree `-p8`.

- [ ] **Step 173** [B] ~5m: The digest the registry serves for each tag, read and not chosen (Stevie 2026-11-06):
  ```bash
  for i in haproxy:2.9.15-alpine nginx:1.25.5-alpine; do printf '%s ' $i; docker buildx imagetools inspect "docker.io/library/$i" --format '{{json .Manifest.Digest}}'; echo; done
  ```
- [ ] **Step 174** [V] ~1m: Two `sha256:` values of 64 hex.
  - buildx missing → `docker manifest inspect --verbose` (no install).
- [ ] **Step 175** [W] ~5m: `docker/docker-compose.loadbalancers.yml`: all 8 lines to `tag@sha256:<digest>`. Write the file in place with a short Python replace, never `sed -i`.
- [ ] **Step 176** [W] ~10m: `scripts/check-image-digests.sh`: `files=(docker-compose.yml monitoring/docker-compose.yml docker/docker-compose.loadbalancers.yml)`, and the header comment updated. Add a red-then-green case in `tests/scripts/test_image_digests.py`: the load-balancer file's third-party images all carry a digest. ADR-102: a `## Update 2026-11-12` section saying the load-balancer file is now read, and why: Stevie's 11-06 answer covers compose files, and the registry digest was read, not chosen. Dockerfile `FROM` lines and sophia-eye/telemetry compose stay out, as Decision 4 says.
- [ ] **Step 177** [B] ~3m: `bash scripts/check-image-digests.sh 2>&1 | tail -2; bash scripts/check-image-tags.sh 2>&1 | tail -1; /var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/scripts/test_image_digests.py 2>&1 | tail -1; bash scripts/check-haproxy-cfg.sh 2>&1 | tail -1`
- [ ] **Step 178** [V] ~1m: All pass.
- [ ] **Step 179** [W] ~15m: W4 (answered 2026-11-12).
  - `scripts/bootstrap-llm-lab.sh` lines 979-981: prometheus and grafana go to the exact `tag@sha256` that `monitoring/docker-compose.yml` uses (copy, don't re-resolve). Drop the Loki pull line and any later use of that image in the script. Read the script around the line first; if Loki is used beyond the pull, `[STUCK]` and ask.
  - Delete `lxd/telemetry/` outright: `git rm -r lxd/telemetry` (history keeps it).
  - `lxd/Makefile`: remove the `launch-telemetry` target (lines 104-106), its `.PHONY` entry (line 5) and both help lines (32, 58). Renumber the "3. Run:" list if a step number is left out.
  - Live references found 2026-11-12: `lxd/README.md` (81-82, 119, the "Telemetry Stack Issues" section from 488), `lxd/FILES_SUMMARY.txt` (10, 32, 95, 131, 160) and `lxd/NEW_FILES_CREATED.md` (13, 64-78, 211, 271, 281, 354, 377-378). Remove those lines or sections. If a file is a dated record (a date in its name or its header), don't edit it: add a one-line "superseded 2026-11-12: lxd/telemetry deleted (S-WATCH-TRUE W4)" note at the top instead.
  - The `unheaded-telemetry` LXD **profile** (`lxd/deploy.sh:134`, `lxd/LAUNCH_EXAMPLES.sh`, `lxd/hosts/`) is not one of the deleted files. Leave it.
- [ ] **Step 180** [B] ~2m: Syntax and target checks.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE-p8 && bash -n scripts/bootstrap-llm-lab.sh && grep -n ':latest' scripts/bootstrap-llm-lab.sh | head; test ! -e lxd/telemetry && echo gone; make -C lxd -n help >/dev/null && echo make-ok; make -C lxd -n launch-telemetry 2>&1 | grep -c 'No rule'
  ```
  Expected: no `:latest` among the three lines, `gone`, `make-ok`, `1`.
- [ ] **Step 181** [V] ~2m: **No live reference to the deleted files.** Dated records and generated scans are excluded: `docs/sweeps/`, the `docs/policy/*-2026-*` inventories, `docs/legal/scancode-full.json`, `docs/archive/`, and the plans and progress files that describe this change.
  ```bash
  cd /home/govan/tmp/wt/S-WATCH-TRUE-p8 && git grep -n -E 'lxd/telemetry|telemetry/setup-telemetry|setup-telemetry\.sh|launch-telemetry|(grafana|loki|prometheus|victoriametrics)-lxd\.yaml' -- . ':!docs/sweeps' ':!docs/policy/*-2026-*' ':!docs/legal/scancode-full.json' ':!docs/archive' ':!docs/battle-plans' | wc -l
  ```
  Must print `0`.
  - Anything else → remove that reference, or add a superseded note if the file is a dated record, and rerun.
- [ ] **Step 182** [B] ~5m: W3: what the override changes, key names only (no values printed).
  ```bash
  cd /home/govan/tmp/unheaded && python3 - <<'PY'
  import yaml
  d = yaml.safe_load(open("docker-compose.override.yml"))
  for s, v in sorted((d.get("services") or {}).items()):
      print(s, sorted((v or {}).keys()))
  PY
  ```
  Then `sudo docker compose config --hash '*' > /tmp/claude-swt/with.txt && sudo docker compose -f docker-compose.yml config --hash '*' > /tmp/claude-swt/without.txt && diff /tmp/claude-swt/with.txt /tmp/claude-swt/without.txt | grep -c '^<'` gives the number of services the override really changes.
- [ ] **Step 183** [W] ~3m: Log in `$PROG` "For Stevie": the per-service key list and the count. The ask: fold the needed keys into the tracked compose file or delete the override. Nothing is changed.
- [ ] **Step 184** [C][V] ~1m: **PHASE 8 EXIT GATE**: gates green with the load-balancer file read; W4 applied (Step 181 prints 0); the override recorded. **COMMIT** `docker/docker-compose.loadbalancers.yml scripts/check-image-digests.sh tests/scripts/test_image_digests.py docs/adr/ADR-102-*.md scripts/bootstrap-llm-lab.sh lxd/Makefile lxd/README.md lxd/FILES_SUMMARY.txt lxd/NEW_FILES_CREATED.md docs/battle-plans/progress/S-WATCH-TRUE-progress.md`. The `lxd/telemetry/` removal is already staged by `git rm`; check that `git status --short lxd/telemetry` shows only `D` lines before committing.

---

## PHASE 9: HOST EVENTS FOR THE IOC AUDIT (D-18, option A) (Steps 185-212) [P]

**Goal**: Vector counts account changes, sudo commands, crontab edits and kernel events by kind, from trusted journal fields, with no text. The ioc-audit reads them per W5.
**Prerequisite**: Phase 0 gate.
**Time**: 2 h.
**Agent**: Developer agent `[P]`, worktree `-p9`.

- [ ] **Step 185** [R] ~5m: `config/vector/vector.yaml` lines 156-222 (`journal_ssh`, the pattern to copy: raw journal, `_COMM` plus `_UID == 0`, never `SYSLOG_IDENTIFIER`) and `config/vector/tests/ssh.yaml`.
- [ ] **Step 186** [W] ~20m: Red. `config/vector/tests/host_events.yaml`, cases:
  - `useradd` (`_COMM=useradd`, `_UID=0`, "new user: name=x, UID=1001...") → `{host, kind: account, op: useradd, user: invalid|x}`: no text kept, `user` kept only if it matches `^[a-z_][a-z0-9_-]{0,31}$`.
  - `usermod`, `userdel`, `groupadd`, `gpasswd`, `passwd`, `chpasswd`, `systemd-sysusers` "Creating user" → `kind: account`, `op` = the command.
  - `sudo` (`_COMM=sudo`, `_UID=0`, "govan : TTY=pts/0 ; PWD=/x ; USER=root ; COMMAND=/usr/bin/ls") → `{kind: sudo_command, user: govan, runas: root}`. No `PWD`, no `COMMAND` text.
  - `sudo` pam session lines ("pam_unix(sudo:session): session opened") → dropped (not a command).
  - `crontab` (`_COMM=crontab`, "(govan) BEGIN EDIT (govan)" / "REPLACE" / "END EDIT") → `{kind: cron_edit, user: govan}` on REPLACE only: one edit counts once.
  - kernel (`_TRANSPORT=kernel`): "Out of memory: Killed process" or "oom-kill:" → `{kind: kernel, class: oom_kill}`; "segfault at" → `class: segfault`; "module verification failed" or "loading out-of-tree module taints kernel" → `class: module_untrusted`; other kernel lines → dropped.
  - a forged line: `_COMM=logger` or `_UID=1000` writing "new user: name=x" → dropped (the `journal_ssh` rule).
  - a crafted user "a b;rm" → `user: invalid`.
  Every case asserts the event has exactly its listed keys (the `length(keys(.))` idiom from `ssh.yaml`).
- [ ] **Step 187** [B] ~3m: `cd /home/govan/tmp/wt/S-WATCH-TRUE-p9 && scripts/test-vector-config.sh 2>&1 | tail -4`
- [ ] **Step 188** [V] ~1m: Red: the transform `journal_host_events` does not exist.
- [ ] **Step 189** [W] ~25m: Green. In `vector.yaml`, after `journal_ssh_metrics`: a `journal_host` filter on `journal` (trusted `_COMM` in the fixed list and `_UID == "0"`, or `_TRANSPORT == "kernel"`), a `journal_host_events` remap (`drop_on_abort: true`, fixed regexes, `abort` on anything unclassified), and `journal_host_metrics` (log_to_metric) to **one** counter, `unheaded_host_events_total{host, kind, op, class, user, runas}`. Fields a kind does not use are `""`. Comment block above it in the house voice: what is counted, that text never leaves the transform, why trusted fields only.
- [ ] **Step 190** [B] ~3m: Step 187 again.
- [ ] **Step 191** [V] ~1m: All Vector tests pass, old and new.
  - Fail → Step 192.
- [ ] **Step 192** [D] ~5m: VRL errors name the line. A fallible call needs `?? ""` or `!`. Two tries, then `[STUCK]`; Steps 193-212 are `[BLOCKED]`.
- [ ] **Step 193** [C] ~1m: **COMMIT** `config/vector/vector.yaml config/vector/tests/host_events.yaml`: "Steps 185-193: Vector counts account, sudo, crontab and kernel events by kind, no text".
- [ ] **Step 194** [B] ~2m: East parity, if `vector-east.yaml` mirrors west's transforms: `grep -n journal_ssh config/vector/vector-east.yaml`. If it has them, copy the new block there too (east ships nothing yet, B-22, but the file stays in step). If not, leave it and log why.
- [ ] **Step 195** [W] ~20m: Red. `tests/runbooks/test_ioc_audit.py`: extend `FakeVictoria`'s table with a `unheaded_host_events_total` key placed **before** the broader keys (first match wins). New tests:
  - an account change → indicator, run exits 1, line names op, host and user;
  - a crontab edit → indicator;
  - kernel `module_untrusted` → indicator;
  - sudo commands → a note with counts per host/user/runas, run exits 0;
  - `oom_kill`/`segfault` → notes;
  - `IOC_ACCOUNT_CHANGE_IS_INDICATOR=0` → account changes become notes (W5 tunable);
  - the series absent while Vector shipped → a note "host events not counted on <host> (Vector older than S-WATCH-TRUE?)", **not** a blind failure, and not clean either: it is listed.
- [ ] **Step 196** [B] ~2m: `/var/tmp/unheaded-sprint/civenv/bin/pytest -q tests/runbooks/test_ioc_audit.py 2>&1 | tail -3`
- [ ] **Step 197** [V] ~1m: Red on the new tests, 18 old pass.
- [ ] **Step 198** [W] ~25m: Green. `runbooks/security/ioc-audit.yaml`:
  - one query, `sum by (host, kind, op, class, user, runas) (increase(unheaded_host_events_total[%s])) > 0`;
  - a printed section "Host events (host kind op/class user runas count)";
  - judgment per W5;
  - new parameters `IOC_ACCOUNT_CHANGE_IS_INDICATOR` (default 1), `IOC_CRON_EDIT_IS_INDICATOR` (1), `IOC_MODULE_UNTRUSTED_IS_INDICATOR` (1);
  - `safe()` on every label;
  - the description and notes say what is newly covered.
  `status` stays `draft`.
- [ ] **Step 199** [B] ~2m: Step 196 again.
- [ ] **Step 200** [V] ~1m: All pass (18 old and the new ones).
- [ ] **Step 201** [B] ~1m: `scripts/runbook-manifest.sh --write && git diff --stat runbooks/MANIFEST.sha256`
- [ ] **Step 202** [V] ~1m: One line changed (ioc-audit).
- [ ] **Step 203** [C] ~1m: **COMMIT** `runbooks/security/ioc-audit.yaml runbooks/MANIFEST.sha256 tests/runbooks/test_ioc_audit.py`.
- [ ] **Step 204** [B] ~2m: Merge (Step 137 pattern).
- [ ] **Step 205** [S][B] ~2m: `sudo docker restart unheaded-vector && date -u +%FT%TZ`. Note the time in S-E1-LIVE's log store week section (Busboy: Vector resumes from `vector-data`; a few seconds of journal lag, no loss).
- [ ] **Step 206** [V] ~3m: Vector up. `unheaded_log_lines_total` still rising (Step 8's `q`, `rate(...[5m])`). `JournalNotShipping` quiet.
- [ ] **Step 207** [S][B] ~2m: A canary of each kind that changes nothing lasting:
  ```bash
  sudo true; sudo -u nobody true; (crontab -l 2>/dev/null; echo '# s-watch-true canary') | crontab - && crontab -l | grep -v 's-watch-true canary' | crontab -
  ```
  (No account or kernel canary: those would change the host. Their tests are the Vector unit tests.)
- [ ] **Step 208** [V] ~3m: Within two scrapes, `unheaded_host_events_total{kind="sudo_command"}` and `{kind="cron_edit"}` exist for `host="west"`.
- [ ] **Step 209** [B] ~3m: Run the audit by hand against the live VictoriaMetrics, as the night session did: `bash -c "$(python3 -c 'import yaml;print(yaml.safe_load(open("runbooks/security/ioc-audit.yaml"))["steps"][0]["command"])')"`. Then `echo rc=$?`.
- [ ] **Step 210** [V] ~1m: The new section prints. The canary's crontab edit shows as an **indicator** (rc=1): the audit sees what it should. Log that rc=1 is the canary, by design, and that the next scheduled run's window will still hold it for 2 days (the default `IOC_WINDOW`). Write that under "For Stevie" so the first card in Review is expected.
- [ ] **Step 211** [W] ~3m: `wiki/` page for the IOC audit (find it with `grep -rl ioc-audit wiki`): the new kinds and their judgment.
- [ ] **Step 212** [C][V] ~1m: **PHASE 9 EXIT GATE**: Vector tests, audit tests and manifest green; the live counters exist on west; the hand run reads them. Commit the wiki page and `$PROG`.

---

## PHASE 10: CLOSE (Steps 213-226)

**Goal**: Everything merged on develop, gates green together, records updated, next.md written.
**Prerequisite**: Phases 1-9 gates (or their `[STUCK]` markers).
**Time**: 45 min.
**Agent**: Coordinator.

- [ ] **Step 213** [B] ~2m: `cd /home/govan/tmp/unheaded && git switch develop && git log --oneline origin/develop..develop | grep -c 'S-WATCH-TRUE'`
- [ ] **Step 214** [B] ~15m: The full local CI with the CI venv **last** on PATH (reference_ci_replay_gotchas): `PATH=$PATH:/var/tmp/unheaded-sprint/civenv/bin scripts/ci-local.sh 2>&1 | tail -25`
- [ ] **Step 215** [V] ~2m: Read each gate's result on its own line. All pass, or each FAIL is shown to be on develop before this sprint (re-run that gate in a clean clone of `a31be33b`).
- [ ] **Step 216** [D] ~10m: A FAIL this sprint caused → fix it in its phase's files, commit, rerun Step 214. Two tries.
- [ ] **Step 217** [W] ~5m: `docs/sweeps/BACKLOG-SWEEP-2026-11-12.md` is a dated record: **do not edit it**. Instead write a closing table in `$PROG`: D-13 … D-18, each with the commit that closed it, or `[STUCK]` with the reason.
- [ ] **Step 218** [W] ~5m: Librarian ripple. In `wiki/` (Monitoring or Alerts page, Profiling, Systemd-Units for the baseline unit change), say what changed, in terse style. Check that no doc still says "two readers" or calls heimdall a scrape target.
- [ ] **Step 219** [B] ~1m: `grep -rn 'heimdall' wiki/*.md | grep -i 'scrape\|target' | head`
- [ ] **Step 220** [V] ~1m: None claim a live target.
- [ ] **Step 221** [C] ~1m: **COMMIT** wiki files and `$PROG` by name.
- [ ] **Step 222** [B] ~1m: Remove the lane worktrees once merged: `for w in -p5 -p6 -p7 -p8 -p9 ""; do git worktree remove /home/govan/tmp/wt/S-WATCH-TRUE$w 2>/dev/null; done; git worktree list | grep -c S-WATCH-TRUE`
- [ ] **Step 223** [V] ~1m: 0.
- [ ] **Step 224** [W] ~10m: `~/tmp/next.md`: a section at the top for this sprint: the before and after numbers, FND-044, what waits on Stevie (east Phase 4B if not done, W3's override decision, the ioc-audit approval, which still is A-1, any `[STUCK]`), the expected canary card, and the push count.
- [ ] **Step 225** [V] ~1m: `git status --short` shows no tracked change left. `git log origin/develop..develop --format='%G? %s' | grep -vc '^G'` is 0, or the unsigned ones are listed for `scripts/resign-unpushed.py`.
- [ ] **Step 226** [V] ~1m: **PHASE 10 EXIT GATE**: develop holds the sprint, gates green, next.md written. Stevie pushes.

---

## Appendix A: Emergency procedures

1. **The collector will not start after Step 88.** Roll back with Step 91's two `cp` lines and the restart. The old pair is a known state: it burns CPU but loads. Then read `journalctl -u unheaded-trace-collector -n 50`.
2. **Verifier rejects the LRU map (Step 25).** LRU hash maps load on 6.17 with no special flag. A rejection names the program and the instruction. Revert `main.rs` to `HashMap` and keep only the counter (`is_err` → `STAT_MAP_INSERT_FAILED`). The bar's "≤ 65,536 entries" then still holds, but new threads are refused (counted). Log it as `[STUCK]` for the LRU half.
3. **Wotan does not come back after Step 138.** `docker logs unheaded-wotan --tail 50`. A config refused at start means `configs/wotan.yaml` was touched: it was not, in this plan, so check the image build. The previous image is still tagged locally: `sudo docker compose up -d --no-build wotan` after `docker tag` back.
4. **A single-file bind mount shows the old file.** `git` writes a new inode. A restart of the container re-binds the path; a SIGHUP does not. Restart, never HUP, after a merge.
5. **Vector refuses the new config after Step 205.** `docker logs unheaded-vector --tail 30`. Revert `config/vector/vector.yaml` on develop to the parent commit (`git revert`, signed) and restart Vector. Shipping first: `JournalNotShipping` pages nobody, but the log store week is running.
6. **`check-ebpf-loads.sh` needs sudo and Stevie is away.** It fails (exit 2) by design. Phase 1 then cannot pass its gate, so Phases 2-4 are `[BLOCKED]`. Go to Phases 5-9.
7. **A gate was already red on develop before the sprint.** Prove it in a clean clone at `a31be33b`, log it under "For Stevie", and don't fix it here.

## Appendix B: Agent assignment and dependencies

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preconditions | Coordinator | no | — | 30m |
| 1 Probe | Coordinator (root) | no | 0 | 1h |
| 2 Collector | Coordinator / Developer | no | 1 | 2.5h |
| 3 Record | Coordinator | no | 2 | 30m |
| 4 West deploy | Coordinator (sudo) | soak runs beside 5-9 | 3 | 45m + 6h soak |
| 4B East | Coordinator + Stevie | — | 4 | 20m + 6h |
| 5 Wotan alert | Developer | [P] | 0 | 1.5h |
| 6 Scrape truth | Agent | [P] | 0 | 45m |
| 7 Dashboards | Agent (one sudo step) | [P] | 0 | 45m |
| 8 Gates | Agent | [P] | 0 | 1h |
| 9 Host events | Developer | [P] | 0 | 2h |
| 10 Close | Coordinator | no | all | 45m |

**Critical path**: 0 → 1 → 2 → 3 → 4 (6 h soak) → 10. That is about 5 h of work plus the soak. Phases 5-9 fit inside the soak. Lanes merge one at a time onto `sprint/s-watch-true`, then to develop. Every merge reruns the merged lane's own gate.

## Appendix C: Quick reference

**New series**

| Series | Type | Labels | From |
|---|---|---|---|
| `trace_collector_latency_sweep_seconds` | histogram | — | Phase 2 |
| `trace_collector_latency_map_entries` | gauge | — | Phase 2 |
| `trace_collector_latency_decode_errors_total` | counter | — | Phase 2 |
| `trace_collector_latency_probe_events_total` | counter | `stat` (10 fixed values) | Phase 2 |
| `trace_collector_latency_stats_read_errors_total` | counter | — | Phase 2 |
| `unheaded_wotan_topic_permission_refusals_total` | counter | `action, outcome, caller` (caller bounded by config) | Phase 5 (label added) |
| `unheaded_host_events_total` | counter | `host, kind, op, class, user, runas` (fixed sets, validated names) | Phase 9 |

**New alert**: `WotanTopicPermissionRefused`, urgency ticket, `runbooks/observe/wotan-topic-refused.yaml`. **Changed**: `ZhenSchedulerStalled` gains `reason`. **Removed**: heimdall's two rules (W2).

**Paths**: installed collector `/usr/local/bin/trace-collector-go`; objects `/usr/local/lib/unheaded/ebpf/`; rollback `/var/backups/trace-collector-2026-11-12/`; built object `ebpf/target/load-gate/default/bpfel-unknown-none/release/latency-probe`; collector metrics `127.0.0.1:16671`; VictoriaMetrics `127.0.0.1:8428`; Prometheus `127.0.0.1:9090`; textfile dir `/var/lib/unheaded/textfile`.

**Layouts**: `LatencyTraceKey` 24 bytes (`trace_id[16]`, `pid u32`, `tid u32`); `LatencyEntry` 40 bytes; `LATENCY_STATS` keys 0-9, 9 = `map_insert_failed` (new); old west object key 40 bytes (5-tuple `LatencyKey`, pre-`262e8786`).

**Gates**: `scripts/check-ebpf-loads.sh` (root), `scripts/check-ebpf-insns.sh --from`, `scripts/check-prometheus-rules.sh`, `scripts/test-vector-config.sh`, `scripts/runbook-manifest.sh --write`, `scripts/check-image-digests.sh`, `scripts/check-gitleaks.sh`, `go test ./cmd/compliance-evidence/...`, `scripts/ci-local.sh`.

---

*S-WATCH-TRUE Battle Plan, forged 2026-11-12; amended 2026-11-12 with Stevie's rulings on W1-W4 (W4: lxd/telemetry deleted)*
*12 Phases. 226 Steps. The watch reads what runs, and the watcher stops eating the house.*
