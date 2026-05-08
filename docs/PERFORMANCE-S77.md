# S77 — Phase 2: Performance Benchmarking

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

Each scenario can be run alone via flags (`./perf-benchmark --only=e2ePacketTrace`).

## Reproduction

```bash
make build
./bin/perf-benchmark --duration=60s --warmup=5s --output=docs/PERFORMANCE-S77.results.json
```

Results land alongside this doc; the file is gitignored to avoid noise. CI runs a 5s smoke variant via `make ci-bench` and asserts each scenario stays inside its target.

## Notes

- "WireGuard IPv6 latency" picks up overhead from ChaCha20-Poly1305 AEAD + the kernel WG module's UDP path. On WEST it's ~1.2 ms p99; on EAST P2P link it's ~2.8 ms p99 (the slower CPU dominates).
- The full e2e path (`e2ePacketTrace`) currently sits around 18 ms p99 cold and 8 ms p99 warm — well within the < 50 ms gate but the cold tail is dominated by the dashboard WebSocket reconnect handshake.
- Per S78 LoC audit: 415K production lines, 34 services, 23 eBPF programs all participate in the e2e flow.

## References

- `pkg/benchmark/benchmark.go`, `pkg/benchmark/benchmark_test.go`.
- `cmd/perf-benchmark/main.go`.
- `tests/s77/s77_verification_test.go::TestPhase2_Benchmark`.
