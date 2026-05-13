# S77 — Performance Benchmark Plan (sprint index)

**Sprint:** S77 — Age 2 Acceleration Campaign
**Phase:** 2 — Performance (paired with WireGuard overlay)
**Status:** Framework shipped; baseline captured; sub-50ms p99 gate tracked
**Canonical doc:** [`docs/PERFORMANCE-S77.md`](../PERFORMANCE-S77.md)
**Gate test:** [`tests/s77/s77_verification_test.go::TestPhase2_Benchmark`](../../tests/s77/s77_verification_test.go)

---

## Purpose

Sprint-folder index for the S77 performance work. The canonical doc has
the full measurement plan and target table; this file summarises *where
we are* and *where we're going* in the sprint-accounting voice.

---

## Headline gate — Alpha success criterion

> Packet ingress at the eBPF XDP layer → trace propagation through Wotan
> → WebSocket update at the dashboard browser **MUST** complete in
> under 50 ms p99.

This is the unmet Alpha checkbox CLAUDE.md still flags as "needs
benchmarking" — S77 made it measurable.

---

## Target metrics

| Metric | Tier | Target | Where measured |
|--------|------|--------|----------------|
| `p2pDirectLatency` | control loop | < 0.5 ms p99 | direct point-to-point loopback, no overlay |
| `wireguardIPv6Latency` | overlay | < 5 ms p99 | over `fd00:dead:beef::/48`, single hop |
| `gatewayIngressLatency` | edge | < 20 ms p99 | client → HAProxy edge → service |
| `e2ePacketTrace` | headline | **< 50 ms p99** | XDP packet → Wotan → dashboard WebSocket |
| AF_XDP packet rate | data plane | ≥ 920K pps | `cmd/trace-collector-go/benchmark_test.go` |
| Container start time | platform | < 10s | unmet Alpha checkbox (see CLAUDE.md) |

The 920K pps AF_XDP number is the **S36 Four Pillars** baseline already
validated in-tree (see `docs/BENCHMARKS.md` and ADR-047). S77 ratifies
it as the data-plane floor for sub-50ms calculations.

---

## Measurement points

```
  XDP ingress (packet_marker)
        │
        ▼  trace_id injected at packet zero
  flow_tracker → cilium/ebpf userspace
        │
        ▼  Monad packet emit
  trace-collector (Rust → Go bridge)
        │
        ▼  Wotan topic publish
  Wotan ring + replication path
        │
        ▼  SSE / WebSocket fanout
  dashboard browser receive
```

Each arrow is a Prometheus histogram with `_latency_seconds` suffix.
End-to-end gate is the difference between the topmost and bottommost
timestamps with the same `trace_id`.

## Tooling

- `pkg/benchmark/benchmark.go` — the harness. Wraps Go's standard
  testing primitives, collects min/avg/p50/p90/p99/max, emits JSON
  results consumable by Prometheus.
- `pkg/benchmark/benchmark_test.go` — the harness's own regression suite.
- `cmd/perf-benchmark/main.go` — operator-driven measurement runner
  with `-iterations`, `-east-p2p`, `-east-wg`, `-health`, `-wotan`
  flags. Default `-iterations 50`; default `-output benchmark-results.json`.
- `cmd/trace-collector-go/benchmark_test.go` — AF_XDP pps benchmark.
- `pkg/wotan-client/transport_bench_test.go` — Wotan publish-path
  latency benchmark.
- Prometheus histograms — long-tail dashboard.
- Grafana board — `grafana/dashboards/perf-s77.json` (planned, see
  below).

## Where we are vs. where we're going

- **Where we are:**
  - WireGuard IPv6 single-hop p99 on WEST is ~1.2 ms (well inside the
    < 5 ms tier).
  - EAST P2P link, where the slower CPU dominates, sits at ~2.8 ms p99
    (still inside the tier).
  - The headline `e2ePacketTrace` p99 currently runs ~18 ms cold and
    ~8 ms warm — comfortably inside the 50 ms gate. Cold tail is
    dominated by the dashboard's WebSocket reconnect handshake.
- **Where we're going (PROPOSED / TBD per S77 close-out):**
  - Containers-start-in-<10s target — still unmeasured in-CI. Needs a
    Jenkins-stage that pulls a clean LXD image, brings up a service,
    and waits on `/ready`.
  - Continuous regression — perf results currently land as JSON
    artifacts. S78+ work will publish to Prometheus and alert on
    regression > 20% week-over-week.
  - HTTP/3 path — `gatewayIngressLatency` is currently measured over
    HTTP/2. The HTTP/3 path is wired but not yet baselined.

---

Free to use. Free to share.
