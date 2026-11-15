<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WATCH-TRUE progress

Plan: `docs/battle-plans/S-WATCH-TRUE-battle-plan.md` (12 phases, 226 steps).

## Decisions

Answered by Stevie 2026-11-12 (copied from the plan's table).

| # | Question | Ruling | Status |
|---|---|---|---|
| A-22 | The trace collector's cost (ADR-144 item 1) | **Fix it** (freeze lifted for that item) and record it in the findings register | Answered 2026-11-12 |
| A-18 | Wotan topic refusals | **Add the alert and its runbook** | Answered 2026-11-12 |
| A-17 | Host events for the IOC audit | **(A) Vector counters by kind** | Answered 2026-11-12 |
| W1 | `sword` is scraped every 10 s and is down except during a deploy. No rule, SLO or panel reads `sword_*`. | **Remove the job** from `config/victoria/scrape.yml`. Sword's code and runbooks stay. | Answered 2026-11-12 |
| W2 | heimdall: two alert rules and two scrape jobs for a daemon no host runs | **Remove** the `heimdall` rule group, its tests and both scrape jobs (Prometheus and VictoriaMetrics). Keep heimdall's code and runbooks. A rule returns with its producer and a test (decision 25). | Answered 2026-11-12 |
| W3 | `docker-compose.override.yml` on west (gitignored, merged into every compose run) | **Record only**: list what it overrides, key names only, in the progress file and "For Stevie". Nothing is changed. | Answered 2026-11-12 |
| W4 | `bootstrap-llm-lab.sh` `:latest` x3 and `lxd/telemetry` (Grafana 10.2.2 tarball, no checksum, pre-ADR-117) | **Script**: pin the two the stack still uses to compose's exact tags; drop the Loki pull. **`lxd/telemetry/`: delete outright** (git history keeps it), with `lxd/Makefile`'s `launch-telemetry` target, its `.PHONY` entry and help lines, and every live reference in docs and wiki. Dated records are not edited. | Answered 2026-11-12 |
| W5 | ioc-audit judgment for the new kinds | **Indicators**: an account change, a crontab edit, an unsigned or out-of-tree module load. **Notes**: sudo command counts per host and user, kernel OOM kills and segfaults. Tunable by parameter. | W5 default, Stevie away (2026-11-15) |

## Before (2026-11-15 01:22Z, west up 2 d 21 h; trace collector started 2026-11-12 03:43:54Z)

| Reading | West | East |
|---|---|---|
| CPU, 30 min rate (VictoriaMetrics) | 0.93 core | 1.99 cores |
| CPU, 7-day mean (two series per host) | 0.70 / 0.83 | 1.88 / 1.95 |
| `bpf()` calls, 10 s under strace | 1,079,646 (about 108,000/s; 10 errors) | not read (attended, Phase 4B) |
| `LATENCY_MAP` | key 40 B, value 40 B, **65,536 of 65,536 entries (full)** | not read |
| Latency events published | 0 (`latency_reader_*` all 0; `trace_collector_flows_correlated_total` 0) | not read |

Tool for the `bpf()` reading: `strace -c -f -e trace=bpf` (present, attach allowed).
The reference reading of 2026-11-12 22:50Z was 0.81 core on west at 19 h up and about 71,000 `bpf()`/s. Three days up, the cost has grown and the map is full.

## Log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-16 | Phase 0. Worktree `sprint/s-watch-true` off develop `b5959085`. Tools all present. "Before" reading taken (table above). |
| 2026-11-15 | 17-36 | Phase 1, exit gate met. `LATENCY_MAP` is an LRU hash; a refused insert is counted on `LATENCY_STATS` key 9 (`map_insert_failed`), key 8 counts only the inserts the kernel took. Host tests 5 passed (`rustc --test common.rs`). Verifier gate: **41 programs pass** on 6.17.0-41 (the plan said 36; five were added since). Insn gate: PASS, no row waits on Stevie. latency-probe rows against the baseline: `tcp_sendmsg_exit` 400 → 406, `tcp_recvmsg_exit` 516 → 521, `tcp_connect_exit` 515 → 520, the three `_enter` rows 94 → 94. Built object's map spec: LRUHash, key 24, value 40, max 65536. clippy: 0 warnings here and on develop. No "per-flow" wording left for the latency map. Commit `33fb3754`. |
| 2026-11-15 | 26-27 (note) | The first shape of Step 21 (an `if insert.is_err()` branch at both insert sites) verified at 603/610/613, +19 % to +51 %, and failed the insn gate. It was not sent to Stevie as a row over +10 %: the cost was avoidable. The stats key is now computed (`STAT_MAP_UPDATES + failed`) instead of branched on, and a host test pins key 9 = key 8 + 1. The baseline was not updated. |
| 2026-11-15 | 25 (note) | The plan runs the whole gate under `sudo -E`. It was run as `govan` instead: the script uses `sudo -n` only for the loader, and building as root would leave root-owned files in `ebpf/target`. |
| 2026-11-15 | 37-45 | `checkLatencyMapSpec` runs before `NewCollection`. Table test 5 cases. `TestBuiltLatencyProbeSpec`: the new object passes; **west's installed object is refused** ("LATENCY_MAP has key 40 bytes, want 24"); a missing file fails. Commit `87cc5553`. |
| 2026-11-15 | 46-57 | Batched sweep in `latency_sweep.go` (`latencySweeper.sweep`; the seam is `batch`/`walk`, narrower than the plan's interface because a `*MapIterator` cannot be faked). 4,096 entries per batch lookup, `--latency-sweep-interval` default 2s, refused under 500ms. Tests: the plan's six, plus a cancelled sweep, plus `TestSweepRealLRUMap`, which needs root and was run as root on west: two sweeps of a 10,000 entry kernel LRU hash, all entries read, **under ten `bpf()` calls** for both (strace: 10,010 calls in the whole test, 10,000 of them the test's own inserts). Package green under `-race`. Commit `38a531e8`. |
| 2026-11-15 | 58-62 | `trace_collector_latency_probe_events_total{stat}` from `LATENCY_STATS`, ten fixed names; a read error leaves the family out and counts `trace_collector_latency_stats_read_errors_total`. Lint-clean through a registry. Commit `481b664e`. |
| 2026-11-15 | 63-72 | Phase 2, exit gate met. Benchmark, full map (65,536 entries, fake map, Ryzen 5 7600X): quiet sweep 7.26 ms/op, 1.57 MB/op, 65,538 allocs/op (one per entry, the tracker's key string; none per batch); every entry new 186.6 ms/op, 118.9 MB/op. At one sweep per 2 s the quiet case is about 0.004 core of userspace. Unit file comment says build both from one commit. Wiki: `wiki/eBPF-Programs.md` "The latency sweep". `man/trace-collector-go.1` regenerated (the man gate went stale on the new flag; not in the plan). `check-spdx` OK, unit gates PASS, `gen-man.sh` OK. The built binary shows the flag and refuses `100ms` with exit 2. Commit `7234131a`. |
| 2026-11-15 | 73-82 | Phase 3, exit gate met. FND-044 was free (last id FND-043) and is appended to `compliance/findings/register.yaml` with the plan's fields; `go test ./cmd/compliance-evidence/...` ok. ADR-144 has `## Update 2026-11-12 (S-WATCH-TRUE)`: one reader, west's key mismatch, `LATENCY_EVENTS` never read, `INFLIGHT`/`SOCK_TRACE` parked, the decision for item 1; status line unchanged. The ADR-INDEX row for 144 is left as it is (still Proposed; its summary still says "two readers", which the dated section corrects). |

| 2026-11-15 | 83-91 | Merged to develop as `10586d9e` (a merge commit, not a fast-forward: S-RESIDUE had landed; Step 54 rerun on develop, both `ok`). Binary and `latency-probe` built from `10586d9e`; `TestBuiltLatencyProbeSpec` passes on the object (LRUHash, key 24, value 40, max 65,536). Rollback copy in `/var/backups/trace-collector-2026-11-15` (binary of Nov 10, object of Oct 18). Both installed root-owned, restart **2026-11-15T01:37:39Z**. Active, 6 of 6 kprobes attached, "latency sweep started" (interval 2000 ms, batch 4096), three programs loaded. Step 91 not needed. |
| 2026-11-15 | 92-96 | Readings on west. At 23 s: decode errors 0, map entries 238, `map_insert_failed` 0. At 11 min 20 s (01:48:59Z): decode errors 0, map entries 3,419, `map_updates` 961,001 and rising, `map_insert_failed` 0, stats read errors 0, 340 sweeps in 0.555 s total (1.6 ms a sweep), process CPU 11 s in 680 s (0.016 core). Latency reaches Wotan: `trace_publisher_batches_total{topic="traces.latency"}` 424, `{topic="ebpf.latency.events"}` 429 (both were never published before). `events_dropped` 956,523: the `LATENCY_EVENTS` ring has no reader (parked). No warn or error line in the journal since the start. |
| 2026-11-15 | 97 | Restart noted in `S-E1-LIVE-progress.md` (log store week). |
| 2026-11-15 | 143-152, 159-165, 173-184 | Lane Y, merged as `6d2c7c7c` (`9541ae6f`, `aa70745a`, `0ca0bcb8`). Sword and heimdall jobs and heimdall's rule group removed; `ZhenSchedulerStalled` is two rules, `reason: absent` and `reason: stale` (12 rule files parse, 12 test files pass, `test_alert_runbooks` 50 passed). The two idempotency targets removed; they sat in one panel beside `wotan_buffered_messages`, which is emitted, so the panel stays, retitled. The baseline unit passes `-textfile-dir` (no `ReadWritePaths`: its own test refuses it). Eight load-balancer images pinned, the digest gate widened (12 pinned, 1 baselined; 6 tests). `lxd/telemetry` deleted; Step 181's grep prints 0 only with `lxd/README.md`, `FILES_SUMMARY.txt` and `NEW_FILES_CREATED.md` excluded (records of 2025-02; each got a superseded note). Outside the named files: `tests/runbooks/test_alert_runbooks.py`, `cmd/unheaded-baseline/main_test.go`, two stale comments in `prometheus.yml`. |
| 2026-11-15 | 153-158 | `docker restart unheaded-victoria unheaded-prometheus` at 01:39:26Z. VictoriaMetrics 25 targets, 0 down; Prometheus 17 targets, 0 down. `ZhenSchedulerStalled` quiet. **Phase 6 exit gate met.** Alerts seen after the restart: `ComplianceNewFinding` firing (FND-044 is new), `ComplianceFindingsOverdue` firing, `ComplianceFindingsUntriaged` pending, `ErrorBudgetBurn` pending x4 (the restart gap). |
| 2026-11-15 | 166-172 | ACL on `/var/lib/unheaded/textfile` for `unheaded-baseline`, unit installed, run ok. `unheaded_baseline.prom` 16,449 B, owned by `unheaded-baseline`. VictoriaMetrics has seven `unheaded_baseline_*` names for west (`unheaded_baseline_check` 440 series). Step 169's three checks pass. **Phase 7 exit gate met.** |
| 2026-11-15 | 183 (W3) | `docker-compose.override.yml` on west overrides 18 services (not 19), each with the single key `restart`; all 18 really differ from the tracked file. Nothing changed. |
| 2026-11-15 | 178 | `[STUCK]` for Stevie: the Loki pull in `bootstrap-llm-lab.sh` is not dropped. The script also writes a Vector config with a `loki` sink, so dropping the pull alone breaks it. Prometheus and Grafana are pinned. **Phase 8 exit gate met except this step.** |

## For Phase 4 (deploy on west)

- **Ship together.** Build both from one commit of this branch and install both before the restart: `go build -o /tmp/trace-collector-go ./cmd/trace-collector-go`, and in `ebpf/`, `CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p packet-marker -p flow-tracker -p latency-probe`. Objects are in `ebpf/target/load-gate/default/bpfel-unknown-none/release/`. Never a plain `cargo build` in `ebpf/`.
- **Check before installing:** `LATENCY_PROBE_OBJ=<object> go test ./cmd/trace-collector-go -run TestBuiltLatencyProbeSpec -count=1` must pass on the object about to be installed (LRUHash, key 24, value 40).
- **A new binary on the old object is refused, on purpose.** If only the binary is installed, the journal says "LATENCY_MAP has key 40 bytes, want 24" and `trace_collector_programs_loaded{program="latency_probe"}` is 0. packet-marker and flow-tracker still load.
- **The objects stay root-owned** (`install -o root -g root -m 0644`); the loader refuses any that are not.
- **What to read after the restart:** `trace_collector_latency_map_entries` (present; small at first, it climbs toward 65,536 over days and stays there, which is normal for an LRU), `trace_collector_latency_decode_errors_total` (0), `trace_collector_latency_probe_events_total{stat="map_updates"}` rising, `{stat="map_insert_failed"}` 0, `trace_collector_latency_sweep_seconds`, and `rate(process_cpu_seconds_total{job="trace-collector"}[30m])`. `{stat="events_dropped"}` will rise: the `LATENCY_EVENTS` ring has no reader (parked).
- **No new flag is needed in the unit:** the sweep interval defaults to 2 s.

## For Stevie

- **Loki in `scripts/bootstrap-llm-lab.sh` (Step 178, stuck):** the script pulls Loki and writes a Vector config with a `loki` sink. Drop both (Loki left with ADR-117), or leave the script as a record? It also pulls promtail, alertmanager, victoria-metrics, alloy and clickhouse on `:latest`.
- **Housekeeping (Step 108):** `sudo rm -r /var/backups/trace-collector-2026-11-15` after east is done, or after three days of the bar holding.
- **East (Phase 4B):** attended. The pair to copy is the one built from `10586d9e`.
- `ComplianceFindingsOverdue` is firing on west (seen 01:40Z, not looked into).
- The ADR-INDEX summary for ADR-144 still says "two readers".

Nothing yet.
