# Benchmarks: perf-benchmark scenarios

`cmd/perf-benchmark` measures the performance claims the docs make (M-089): packet to browser under 50 ms p99, container start under 10 s, 1000 req/s sustained, and the HTTP/3 leg. Numbers live in `docs/BENCHMARKS.md` section 5, filled only from reports a run wrote. S-WELL-SPRING Phase 10.

## Scenarios

```
go run ./cmd/perf-benchmark -scenario e2e-trace,http-load,http3 -iterations 200 -duration 60s -output /tmp/perf.json
```

| `-scenario` | What is timed | Touches |
|---|---|---|
| `e2e-trace` | Publish of one packet event with a fresh trace id on Wotan topic `bench.perf.packet`, until dashboard-backend's `/api/v1/ebpf/events` (what the page polls) returns it. Read every `-poll` (5 ms). No XDP leg: the event is synthetic. | Publishes to `bench.perf.packet` as display name `perf-benchmark`; GETs only |
| `http-load` | Open-loop `-rate` req/s (1000) for `-duration` (60 s) against `-health` (timeguru `/health`). Token bucket every 5 ms; latency counts from release, so queueing behind a slow server counts; releases that find workers and queue full are reported as dropped. `rate_achieved` is the real ceiling. | GETs |
| `http3` | `curl --http3-only` against `-http3-url` (gateway `:21443`). Skipped, with the reason, when curl lacks the HTTP3 feature: the module has no Go HTTP/3 client (quic-go is not a dependency). | GETs |
| `container-start` | `PUT` of `steel-echo-bench-<i>` (image `-cs-image`, label `svc=steel-echo-bench`, health 8080 `/health`, no proxy, no chaos label) through Cuirass, until Cuirass reports it running and healthy; then deleted, also after a failure. Refuses without `-allow-lxd`, without `CUIRASS_API_KEY`, and when a `steel-echo-bench-*` is already there (it deletes only what it made). | Creates and deletes LXD instances: host window only |
| `all` | The four; `container-start` is skipped (not refused) without `-allow-lxd`. | |

Without `-scenario` the original suite runs (P2P ping, WireGuard ping6, one `/health`, Wotan publish).

The bench packet runs between RFC 2544 benchmarking addresses (`198.18.0.1` to `198.18.0.2`, UDP 40000 to 9), so it shows on the dashboard as one bench flow and never as a real host's. Wotan approves `perf-benchmark` from `configs/wotan.yaml` `topics.auto_approve`; dashboard-backend ingests `bench.perf.packet` beside `ebpf.packet.events`. A running stack needs both (Wotan restart, dashboard-backend rebuild) before `e2e-trace` can see anything; until then it reports `error_ratio 1` with the reason.

## Report and gate

The report is `perf/v1` JSON: `schema`, `host`, `commit`, `at`, `duration_unit: ns`, and per scenario `n`, `attempts`, `errors`, `p50`/`p95`/`p99`, `error_ratio`, `rate_achieved`, `skipped`, `note`, `first_error`. Percentiles are nearest-rank (an observed value).

`scripts/check-perf-regression.sh REPORT...` compares with `docs/policy/perf-baseline.json`: p99 may grow at most 20 % per scenario, `error_ratio` may not grow; faster passes; skipped or unmeasured scenarios are listed, not failed. `--update REPORT...` records measured results (nothing when nothing was measured). Fixture test `scripts/test-check-perf-regression.sh` runs in the security workflow; the live run is opt-in in `scripts/ci-local.sh` (`CI_LOCAL_PERF=1`, west, stack up).

---

> **Source:** [cmd/perf-benchmark](../cmd/perf-benchmark) · [scripts/check-perf-regression.sh](../scripts/check-perf-regression.sh) · [docs/BENCHMARKS.md](../docs/BENCHMARKS.md) · [docs/research/VERDICTS.md](../docs/research/VERDICTS.md) · [S-WELL-SPRING battle plan](../docs/battle-plans/S-WELL-SPRING-battle-plan.md) (Phase 10)
