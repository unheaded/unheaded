# Unheaded Kingdom — Hypothesis Verdicts Log

Pre-registered hypotheses (H1-H8) from BARE_METAL_QA_PLAN.md.
Strong Inference methodology (Platt 1964). Bootstrap CI N=10,000. α=0.05.

## Verdict History

| Date | H1 | H2 | H3 | H4 | H5 | H6 | H7 | H8 | Score |
|------|----|----|----|----|----|----|----|----|-------|
| PENDING | — | — | — | — | — | — | — | — | 0/8 |
| 2026-10-27 | UNTESTED | UNTESTED | UNTESTED | UNTESTED | SUPPORTED (deviation) | UNTESTED | UNTESTED | UNTESTED (pending Step 217) | 1/8 supported, 0 refuted, 7 untested |

## Hypothesis Definitions

### H1 — XDP Latency

**Claim**: XDP packet processing P99 latency < 1µs at 100K pps.

**Null Hypothesis**: H1₀: P99 ≥ 1µs.

**Measurement**: `unheaded_flow_latency_ns` histogram P99 at sustained 100K pps load using synthetic packet generation (pktgen).

**Success Criterion**: P99 latency consistently < 1000 ns (1µs) across 30-second windows.

**Verdict**: UNTESTED: needs pktgen at a sustained 100K pps and the `unheaded_flow_latency_ns` histogram over 30 s windows. No run exists (2026-10-27); the 920K pps figure once quoted was a Python `sendto` ceiling, not XDP (BENCHMARKS.md section 4, M-090).

---

### H2 — eBPF CPU Overhead

**Claim**: eBPF policy enforcement adds < 5% CPU overhead compared to baseline.

**Null Hypothesis**: H2₀: Overhead ≥ 5%.

**Measurement**: CPU utilization (via /proc/stat) with policy programs loaded vs. unloaded, at fixed packet rate (100K pps).

**Success Criterion**: `(CPU_with_policy - CPU_baseline) / CPU_baseline < 0.05`

**Verdict**: UNTESTED: needs /proc/stat CPU with the policy programs loaded and unloaded at a fixed 100K pps. No run exists (2026-10-27).

---

### H3 — Service Recovery

**Claim**: Service restart after forced pod termination < 30 seconds.

**Null Hypothesis**: H3₀: Recovery time ≥ 30s.

**Measurement**: Time from `kill -9 <pod_pid>` to systemd-detected restart and port listening.

**Success Criterion**: `T_restart - T_kill < 30s` in 10/10 trials.

**Verdict**: UNTESTED: needs the 10 pre-registered trials. Two related observations exist, both under 30 s: container kill to Cuirass restart 24.4 s (S-REAL-STEEL Phase 11, 2026-10-26) and 1.7 s (game day 1 B, `docs/gamedays/2026-10-26-real-steel-1.md`). They are LXD instances restarted by Cuirass's enforce loop, not `kill -9` of a systemd service, and the restart waits for Cuirass's 30 s sync, so a trial that lands just after a sync can exceed 30 s: the claim is at risk at the boundary, not shown.

---

### H4 — WireGuard Latency (Inter-Host)

**Claim**: WireGuard tunnel RTT P99 < 5ms between Host-A and Host-B.

**Null Hypothesis**: H4₀: P99 RTT ≥ 5ms.

**Measurement**: `ping6` with 1000 probes over established WireGuard tunnel between forge and outpost.

**Success Criterion**: P99 RTT < 5000 µs (5ms).

**Verdict**: UNTESTED: needs 1000 probes (`ping6 -c 1000` over the overlay, or `go run ./cmd/perf-benchmark -iterations 1000` without `-scenario`, whose `-east-wg` probe pings6 once per iteration). The only data is 10 probes on 2026-03-25, max 0.891 ms WEST to EAST (BENCHMARKS.md section 3): consistent with the claim, too few for a P99.

---

### H5 — LLM Inference Throughput

**Claim**: vLLM serving > 30 tokens/second on AMD RX 7700 XT (Host-A only).

**Null Hypothesis**: H5₀: Throughput ≤ 30 tok/s.

**Measurement**: vLLM benchmarking with 7B parameter model via `python -m vllm.entrypoints.openai.api_server`.

**Success Criterion**: Sustained throughput > 30 tok/s measured over 60s window.

**Verdict**: SUPPORTED, with a protocol deviation: llama.cpp, not vLLM, Mistral-7B on the RX 7700 XT, 58.3 to 59.7 tok/s average across five context sizes on 2026-03-16 (ADR-017 results table); one run per size, no 60 s window or bootstrap recorded. The margin (about 2x the 30 tok/s bar) is why the deviation is not judged to flip it; a vLLM run per the protocol would make it clean.

**Note**: Optional for Host-B (outpost); GPU not required.

---

### H6 — Monad Register Overhead

**Claim**: Per-packet overhead for Monad register/unregister < 2µs.

**Null Hypothesis**: H6₀: Overhead ≥ 2µs.

**Measurement**: eBPF tracepoint `sys_enter` to hash table lookup latency via kprobes.

**Success Criterion**: P99 register overhead < 2000 ns (2µs).

**Verdict**: UNTESTED: needs the kprobe timing of the Monad register/unregister path. No run exists (2026-10-27).

---

### H7 — WireGuard Encapsulation Overhead

**Claim**: WireGuard encapsulation adds < 1ms overhead over bare LAN latency.

**Null Hypothesis**: H7₀: Overhead ≥ 1ms.

**Measurement**: Bare LAN latency (P99) vs. WireGuard tunnel latency (P99), same 100M Ethernet link.

**Success Criterion**: `(RTT_wg_p99 - RTT_bare_p99) < 1000 µs (1ms)`

**Verdict**: UNTESTED: needs bare P2P and WireGuard P99 from the same run (`go run ./cmd/perf-benchmark` without `-scenario` measures both: `p2p-direct-latency`, `wireguard-ipv6-latency`). BENCHMARKS.md has WireGuard RTT only.

---

### H8 — End-to-End Trace Propagation

**Claim**: Distributed trace context propagation completes < 10ms end-to-end.

**Null Hypothesis**: H8₀: E2E trace propagation ≥ 10ms.

**Measurement**: OpenTelemetry span context from client request to final service response, including WireGuard tunnel traversal.

**Success Criterion**: `T_trace_complete < 10ms` for 95th percentile.

**Verdict**: UNTESTED: needs Step 217's `e2e-trace` p95 (S-WELL-SPRING Phase 10, coordinator run on west), and that run is a partial test: Wotan publish to the dashboard API on one host, no OpenTelemetry span and no WireGuard leg. A full test needs a traced request crossing west to east.

---

## Methodology

### Strong Inference (Platt 1964)

1. **Inductive reasoning**: Form alternative hypotheses
2. **Deduction**: Predict experimental consequences
3. **Experimentation**: Test predictions rigorously
4. **Iteration**: Refine hypotheses based on falsification

### Statistical Framework

- **Confidence Level**: α = 0.05 (95%)
- **Bootstrap Resampling**: N = 10,000 iterations
- **Measurement Duration**: Minimum 30 seconds per test
- **Replication**: Minimum 10 independent runs per hypothesis

---

## Raw Data

Measurement data and supporting logs are located in:

```
/var/lib/unheaded/verdicts/        — JSON verdict files
/var/lib/unheaded/baselines/       — System baseline snapshots
notebooks/03_ebpf_latency_analysis.ipynb  — Analysis notebook
```

Captured with:
- `scripts/capture-baseline.sh`
- `scripts/pre-flight-check.sh`
- `scripts/verdicts-record.sh`

---

## Notes for Experimenters

1. **Before testing**: Run `capture-baseline.sh` to establish system state
2. **Run QA protocol**: Execute full test suite from BARE_METAL_QA_PLAN.md
3. **Record verdicts**: Use `verdicts-record.sh` to log results
4. **Analyze**: Open `notebooks/03_ebpf_latency_analysis.ipynb` for deep-dive
5. **Publish**: Update this file with verdict entries and commit to git

---

Last Updated: 2026-10-27 (S-WELL-SPRING Phase 10: every hypothesis has a verdict line; M-090)
Repository: github.com/unheaded-kingdom/unheaded
