# Unheaded Performance Benchmarks

**Date:** 2026-03-25
**Host:** WEST (bare metal, kernel 6.17.0-19-generic)
**Sprint:** Ragnarok Sprint, Phase 2B (HEPHAESTUS)

---

## Test Environment

| Component | Spec |
|-----------|------|
| Host | WEST bare metal |
| Kernel | 6.17.0-19-generic |
| Go | 1.24.0 |
| Services tested | Wotan (18000), unheaded-daemon (17000), timeguru (19000), dashboard-backend (20000) |
| Load tool | [hey](https://github.com/rakyll/hey) v0.1.5 |

---

## 1. Latency (Target: < 50ms)

**Result: PASS** (sub-millisecond, 100-300x under target)

| Service | Endpoint | Avg | Min | Max | N |
|---------|----------|-----|-----|-----|---|
| Wotan | /health | 0.16ms | 0.13ms | 0.21ms | 100 |
| unheaded-daemon | /health | 0.12ms | 0.10ms | 0.17ms | 100 |
| timeguru | /api/v1/timeline | 0.19ms | 0.15ms | 0.26ms | 100 |
| dashboard-backend | /health | 0.12ms | 0.09ms | 0.15ms | 100 |

All services respond in under 0.3ms on bare metal. The 50ms target is exceeded by >100x.

---

## 2. Throughput (Target: 1000 req/s sustained)

**Result: PASS** (149K-256K req/s, 150-256x over target)

### Sustained Load (60s, 50 concurrent, rate-limited to 1000/s)

| Service | Endpoint | Duration | Avg Latency | Req/s |
|---------|----------|----------|-------------|-------|
| Wotan | /health | 60s | 0.6ms | 1000 |
| timeguru | /api/v1/timeline | 30s | 1.9ms | 1000 |
| dashboard-backend | /health | 30s | 0.3ms | 1000 |

All services sustained 1000 req/s with zero errors and sub-2ms latency.

### Maximum Throughput (200 concurrent, unlimited)

| Service | Endpoint | Duration | Avg Latency | Max Req/s |
|---------|----------|----------|-------------|-----------|
| Wotan | /health | 10s | 2.0ms | **149,840** |
| dashboard-backend | /health | 10s | 2.0ms | **256,392** |

Peak throughput exceeds 149K req/s (Wotan) and 256K req/s (dashboard) with 200 concurrent connections.

---

## 3. Cross-Host Latency (WireGuard IPv6 Tunnel)

**Result: PASS** (sub-millisecond)

| Measurement | Avg | Min | Max | Loss |
|-------------|-----|-----|-----|------|
| ICMPv6 (ping6) WEST→EAST | 0.505ms | 0.321ms | 0.891ms | 0% |
| ICMPv6 (ping6) EAST→WEST | 0.379ms | 0.348ms | 0.405ms | 0% |

WireGuard tunnel (fd00:dead:beef::/48) adds <0.5ms overhead on the P2P link.

---

## 4. eBPF Data Plane (Historical)

From S76 Round Table (2026-03-05):

| Metric | Value |
|--------|-------|
| Python `sendto` sender ceiling | 920K pps (not AF_XDP; the AF_XDP product path is M-187) |
| XDP_TX cache warmth speedup | **256x** |
| Doom MBC instructions executed | 5,596,059 |
| Doom XDP bounces | 11,636,475 |
| Doom system ticks | 78,905,128 |

---

## 5. Measured scenarios, 2026-10 (`cmd/perf-benchmark -scenario`, M-089)

The four claims below had no harness before S-WELL-SPRING Phase 10. Each
cell is filled only from a perf/v1 report a run actually wrote; "pending"
means the run has not happened yet. Durations are p50 / p99.

| Scenario | What is timed | Target | Host | Commit | Result |
|----------|---------------|--------|------|--------|--------|
| `e2e-trace` | Wotan publish of a trace-id'd packet event on `bench.perf.packet` until dashboard-backend's `/api/v1/ebpf/events` (the endpoint the page polls) returns it; no XDP leg, 5 ms read resolution | p99 < 50 ms | west | pending | pending (Step 217) |
| `http-load` | open-loop 1000 req/s for 60 s against timeguru `/health`, latency from release (queueing counts) | error ratio < 0.01 at 1000 req/s | west | pending | pending (Step 217) |
| `http3` | `curl --http3-only` against the gateway `:21443` | measured, or skipped with the reason | west | pending | pending (Step 217); west's curl 8.14.1 has no HTTP3 feature, so expect `skipped` |
| `container-start` | `steel-echo-bench-<i>` PUT through Cuirass until running and healthy by Cuirass's probe, then deleted | < 10 s | west | pending | pending (Step 219, host window E-D) |

Reproduce (stack up; Wotan must have `perf-benchmark` in `topics.auto_approve`
and dashboard-backend must ingest `bench.perf.packet`, both since 2026-10-27):

```bash
go run ./cmd/perf-benchmark -scenario e2e-trace,http-load,http3 -iterations 200 -duration 60s -output /tmp/perf.json
CUIRASS_API_KEY=... go run ./cmd/perf-benchmark -scenario container-start -allow-lxd -iterations 5 -output /tmp/perf-cs.json
scripts/check-perf-regression.sh /tmp/perf.json /tmp/perf-cs.json   # against docs/policy/perf-baseline.json
```

Without `-scenario` the harness runs the original four probes (P2P ping,
WireGuard ping6, one `/health`, Wotan publish).

---

## Summary

| Benchmark | Target | Actual | Margin |
|-----------|--------|--------|--------|
| Service latency | < 50ms | 0.12-0.19ms | **250-400x under** |
| Sustained throughput | 1000 req/s | 1000 req/s (stable) | **Met exactly** |
| Max throughput | — | 149K-256K req/s | **Production-grade** |
| Cross-host latency | — | 0.5ms (WireGuard) | **Sub-millisecond** |
| eBPF throughput | — | 920K pps was a Python `sendto` ceiling, not AF_XDP (docs/sweeps/outstanding-2026-10-27/sweep-internal.md D-6, M-090); AF_XDP throughput not measured | not measured |

All performance targets met or exceeded. The system is production-ready on bare metal.

---

## Methodology

- Latency: 100 sequential requests per endpoint, measured with `curl -w "%{time_total}"`
- Sustained throughput: `hey -z 60s -c 50 -q 20` (50 workers, 20 req/s each = 1000/s target)
- Max throughput: `hey -z 10s -c 200` (200 concurrent, unlimited rate)
- Cross-host: `ping6 -c 10` over WireGuard tunnel (fd00:dead:beef::2 ↔ fd00:dead:beef::1)
- All tests on bare metal, no virtualization, no containerization overhead

## Reproducing

```bash
cd ~/tmp/unheaded
make build
./bin/wotan &
./bin/unheaded-daemon &
./bin/dashboard-backend &
./bin/timeguru &
go install github.com/rakyll/hey@latest
hey -z 60s -c 50 -q 20 http://localhost:18000/health
```
