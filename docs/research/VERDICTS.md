# Unheaded Kingdom — Hypothesis Verdicts Log

Pre-registered hypotheses (H1-H8) from BARE_METAL_QA_PLAN.md.
Strong Inference methodology (Platt 1964). Bootstrap CI N=10,000. α=0.05.

## Verdict History

| Date | H1 | H2 | H3 | H4 | H5 | H6 | H7 | H8 | Score |
|------|----|----|----|----|----|----|----|----|-------|
| PENDING | — | — | — | — | — | — | — | — | 0/8 |

## Hypothesis Definitions

### H1 — XDP Latency

**Claim**: XDP packet processing P99 latency < 1µs at 100K pps.

**Null Hypothesis**: H1₀: P99 ≥ 1µs.

**Measurement**: `unheaded_flow_latency_ns` histogram P99 at sustained 100K pps load using synthetic packet generation (pktgen).

**Success Criterion**: P99 latency consistently < 1000 ns (1µs) across 30-second windows.

**Status**: PENDING

---

### H2 — eBPF CPU Overhead

**Claim**: eBPF policy enforcement adds < 5% CPU overhead compared to baseline.

**Null Hypothesis**: H2₀: Overhead ≥ 5%.

**Measurement**: CPU utilization (via /proc/stat) with policy programs loaded vs. unloaded, at fixed packet rate (100K pps).

**Success Criterion**: `(CPU_with_policy - CPU_baseline) / CPU_baseline < 0.05`

**Status**: PENDING

---

### H3 — Service Recovery

**Claim**: Service restart after forced pod termination < 30 seconds.

**Null Hypothesis**: H3₀: Recovery time ≥ 30s.

**Measurement**: Time from `kill -9 <pod_pid>` to systemd-detected restart and port listening.

**Success Criterion**: `T_restart - T_kill < 30s` in 10/10 trials.

**Status**: PENDING

---

### H4 — WireGuard Latency (Inter-Host)

**Claim**: WireGuard tunnel RTT P99 < 5ms between Host-A and Host-B.

**Null Hypothesis**: H4₀: P99 RTT ≥ 5ms.

**Measurement**: `ping6` with 1000 probes over established WireGuard tunnel between forge and outpost.

**Success Criterion**: P99 RTT < 5000 µs (5ms).

**Status**: PENDING

---

### H5 — LLM Inference Throughput

**Claim**: vLLM serving > 30 tokens/second on AMD RX 7700 XT (Host-A only).

**Null Hypothesis**: H5₀: Throughput ≤ 30 tok/s.

**Measurement**: vLLM benchmarking with 7B parameter model via `python -m vllm.entrypoints.openai.api_server`.

**Success Criterion**: Sustained throughput > 30 tok/s measured over 60s window.

**Status**: PENDING

**Note**: Optional for Host-B (outpost); GPU not required.

---

### H6 — Monad Register Overhead

**Claim**: Per-packet overhead for Monad register/unregister < 2µs.

**Null Hypothesis**: H6₀: Overhead ≥ 2µs.

**Measurement**: eBPF tracepoint `sys_enter` to hash table lookup latency via kprobes.

**Success Criterion**: P99 register overhead < 2000 ns (2µs).

**Status**: PENDING

---

### H7 — WireGuard Encapsulation Overhead

**Claim**: WireGuard encapsulation adds < 1ms overhead over bare LAN latency.

**Null Hypothesis**: H7₀: Overhead ≥ 1ms.

**Measurement**: Bare LAN latency (P99) vs. WireGuard tunnel latency (P99), same 100M Ethernet link.

**Success Criterion**: `(RTT_wg_p99 - RTT_bare_p99) < 1000 µs (1ms)`

**Status**: PENDING

---

### H8 — End-to-End Trace Propagation

**Claim**: Distributed trace context propagation completes < 10ms end-to-end.

**Null Hypothesis**: H8₀: E2E trace propagation ≥ 10ms.

**Measurement**: OpenTelemetry span context from client request to final service response, including WireGuard tunnel traversal.

**Success Criterion**: `T_trace_complete < 10ms` for 95th percentile.

**Status**: PENDING

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

Last Updated: 2026-02-26
Repository: github.com/unheaded-kingdom/unheaded
