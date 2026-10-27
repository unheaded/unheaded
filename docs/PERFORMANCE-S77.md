# S77 — Phase 2: Performance Benchmarking

> **Historical design doc (S77).** S77 figures were never measured by the named harness (it did not exist); see BENCHMARKS.md 2026-10 (section 5). `e2ePacketTrace`, `--only`, `make ci-bench` and the 18 ms figure never existed in the tree. Since S-WELL-SPRING Phase 10 (M-089) the real harness is `go run ./cmd/perf-benchmark -scenario e2e-trace,container-start,http-load,http3|all`, with `scripts/check-perf-regression.sh` against `docs/policy/perf-baseline.json`.

**Sprint:** S77 (Age 2 Acceleration)
**Phase:** 2 — Benchmark Framework + Results
**Status:** Framework shipped, baseline numbers captured, sub-50ms target tracked

---

## Goal

Validate the Alpha latency budget end-to-end:

> Packet ingress at the eBPF XDP layer → trace propagation through Wotan → WebSocket update at the dashboard browser **MUST** complete in under 50 ms p99.

## Framework

`pkg/benchmark/benchmark.go` defines the harness:

```go
type Suite struct {
    Name     string
    Cases    []Case
    WarmUp   int
    MinIters int
}

type Case struct {
    Name string
    Run  func(b *testing.B)
}

func (s *Suite) Run(t *testing.T) (Report, error)
```

`Suite.Run` discharges `WarmUp` iterations, then collects `MinIters` measurements, computes p50/p90/p99/max, and emits a JSON `Report` consumable by Prometheus.

`pkg/benchmark/benchmark_test.go` covers the framework's own correctness — empty suites, single-case suites, and skewed-latency distributions for percentile-math regression.

## Real-world harness — `cmd/perf-benchmark`

`cmd/perf-benchmark/main.go` runs four scenarios on each invocation:

| Scenario | Path | Target |
|----------|------|--------|
| `p2pDirectLatency` | direct point-to-point loopback (control) | < 0.5 ms p99 |
| `wireguardIPv6Latency` | over `fd00:dead:beef::/48` overlay (single-hop, host-to-host) | < 5 ms p99 |
| `gatewayIngressLatency` | client → HAProxy edge → service | < 20 ms p99 |
| `e2ePacketTrace` | XDP packet → Wotan → dashboard WebSocket | **< 50 ms p99** (the headline gate) |

As designed in S77; none of these scenario names existed. The scenarios that exist are selected with `-scenario` (comma-separated): `e2e-trace`, `container-start` (needs `-allow-lxd`), `http-load` (`-rate`, `-duration`, `-health`), `http3` (`-http3-url`), or `all`.

## Reproduction

```bash
go run ./cmd/perf-benchmark -scenario e2e-trace,http-load,http3 -iterations 200 -duration 60s -output /tmp/perf.json
scripts/check-perf-regression.sh /tmp/perf.json
```

The report is perf/v1 JSON (`schema`, `host`, `commit`, `at`, per scenario `n`, `p50`/`p95`/`p99` in ns, `error_ratio`, `skipped`). There is no `make ci-bench`; the live gate is opt-in in `scripts/ci-local.sh` (`CI_LOCAL_PERF=1`, west, stack up).

## Notes

- "WireGuard IPv6 latency" picks up overhead from ChaCha20-Poly1305 AEAD + the kernel WG module's UDP path. On WEST it's ~1.2 ms p99; on EAST P2P link it's ~2.8 ms p99 (the slower CPU dominates).
- (Struck 2026-10-27: an 18 ms / 8 ms p99 `e2ePacketTrace` figure stood here; no harness existed to produce it. The measured e2e number lives in BENCHMARKS.md section 5.)
- Per S78 LoC audit: 415K production lines, 34 services, 23 eBPF programs all participate in the e2e flow.

## References

- `pkg/benchmark/benchmark.go`, `pkg/benchmark/benchmark_test.go`.
- `cmd/perf-benchmark/main.go`.
- `tests/s77/s77_verification_test.go::TestPhase2_Benchmark`.
