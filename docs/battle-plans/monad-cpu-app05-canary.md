# Monad CPU Application #5: Closed-Loop Canary Deployments
## Battle Plan v1.0

---

## 1. Overview

The Monad CPU's wire-speed data plane enables fully automated canary deployments with zero human intervention in the feedback loop. Current deployments rely on slow, reactive observability: push code, wait for metrics, manually toggle traffic. This loses critical seconds where bad code runs in production. Application #5 replaces that with an in-kernel automation layer that observes, decides, and acts on SLO violations in sub-millisecond latency. Traffic routing decisions, error detection, and rollback triggers live entirely in XDP-attached eBPF programs, reading from Sophia config dictionaries and writing metrics to Wotan for async dashboard display.

Unlike external canary controllers (Argo Rollouts, Flagger, Istio), which observe via sidecars or gRPC hooks, Monad CPU inspects every packet at ingress before it reaches userland. We capture latency from wire timestamp to response departure, error codes from packet payloads, and throughput from XDP packet counters—all without copying data or leaving the kernel. The circuit_state byte in our IPv6 HbH header becomes a per-packet policy knob: bit patterns select canary vs stable routing, version affinity, and rollback gates. Sophia's atomic update semantics ensure version→endpoint mappings transition without dropping traffic or connection resets.

This strategy trades upfront BPF complexity for elimination of deployment coordination overhead. Once deployed, canary promotion/rollback is automatic, deterministic, and unaffected by cloud API latency or observability backhaul.

---

## 2. Value Proposition

| Dimension | Monad CPU | Argo Rollouts | Flagger | Istio Canary |
|-----------|-----------|---------------|---------|--------------|
| **Feedback loop latency** | <1ms (in-kernel) | 10-30s (API polling) | 30-60s (Prometheus + controller) | 15-45s (mixer → policy) |
| **Data plane coupling** | Native—packets are instructions | Loose—via pod annotations | Loose—via VirtualService | Tight—DestinationRule hot-reload |
| **Traffic routing** | XDP redirect, sub-microsecond | iptables/cilium eBPF | Service mesh sidecar | Envoy proxy |
| **SLO source of truth** | BPF map ring buffer | Prometheus scrape | Prometheus/custom metric | Prometheus scrape |
| **Canary version cost** | Zero extra kernel memory | Pod replica (full image) | Pod replica (full image) | Pod replica + envoy sidecar |
| **Atomic failover** | Yes—Sophia CAS semantics | No—rolling restart risk | No—traffic may split mid-update | No—control plane eventual consistency |
| **Zero-copy traffic** | Yes—AF_XDP 920K pps | No—kernel→userland copy | No—sidecar copy overhead | No—sidecar copy overhead |
| **Hardware-independent** | Yes—pure eBPF | No—cloud-native assumptions | No—k8s/Istio required | No—Istio/k8s required |

---

## 3. Prerequisites

- [x] **Monad CPU core** — XDP loader, BPF map framework, IPv6 HbH wire format
- [x] **Sophia** — Atomic dict updates, lock-free read path, CAS loop for version atomicity
- [x] **Wotan** — In-kernel ring buffer event publishing, async userland consumer
- [x] **Shield** — Ingress/egress eBPF hooks, XDP redirect to `bpf_clone_redirect()`
- [x] **AF_XDP socket** — 920K pps zero-copy verified on production hardware
- [x] **Service registry** — HAProxy + internal Nginx; 10 core services each with 2-3 versions max
- [ ] **Per-flow metrics BPF map** — `(service_id, version_tag) → (error_count, latency_p99, throughput, timestamp)`
- [ ] **Canary routing eBPF program** — Attached to `xdp_prog_in`, makes version selection decision
- [ ] **SLO threshold Sophia dict** — `(service_id) → (max_error_rate%, max_latency_ms, min_throughput_pps)`
- [ ] **Canary state machine** — `(service_id) → (canary_version, stable_version, traffic_split%, state: PROBING|RAMP|STABLE|ROLLBACK, age_ms)`
- [ ] **Rollback trigger program** — Attaches to Wotan ring buffer, reads metrics, updates state machine
- [ ] **Circuit-breaker integration** — Map circuit_state byte to version affinity; mark circuit_breaker.OPEN on SLO breach

---

## 4. Architecture

```
                        Ingress Traffic (XDP-Hookable)
                                |
                                v
        ┌──────────────────────────────────────────┐
        |  Shield Ingress eBPF (xdp_prog_in)       |
        │  - Parse IPv6 HbH header                 │
        │  - Extract SrcServiceID, DstServiceID    │
        │  - Check circuit_state byte              │
        └──────────────┬───────────────────────────┘
                       |
                       v
        ┌──────────────────────────────────────────┐
        |  Canary Routing Decision (INLINE)        │
        │  if (dst_service_id in canary_enabled)   │
        │    read canary_state_map[dst_service_id] │
        │    rand() % 100 < traffic_split% ?       │
        │      choose canary_endpoint              │
        │    else                                  │
        │      choose stable_endpoint              │
        │    set circuit_state |= CANARY_BIT       │
        └──────────────┬───────────────────────────┘
                       |
                       v
        ┌──────────────────────────────────────────┐
        |  XDP Redirect to Endpoint (L2/L3 rewrite)│
        │  via bpf_clone_redirect() or bpf_fib_ln()
        │  AF_XDP hwts as packet departs           │
        └──────────────┬───────────────────────────┘
                       |
                       v
        ┌──────────────────────────────────────────┐
        |  Response Egress Capture (xdp_prog_out)  │
        │  - Timestamp rx completion               │
        │  - Extract error code from response      │
        │  - Atomic increment metrics map:         │
        │    metrics_map[(svc_id,version)]++       │
        │  - Publish to Wotan ring buffer          │
        └──────────────┬───────────────────────────┘
                       |
                       v
        ┌──────────────────────────────────────────┐
        |  Wotan Async Consumer (Userland)         │
        │  Poll ring buffer every 100ms            │
        │  Aggregate metrics, compute SLO deltas   │
        │  Publish to dashboard topics             │
        └──────────────┬───────────────────────────┘
                       |
                       v
        ┌──────────────────────────────────────────┐
        |  Rollback Trigger Daemon (Userland)      │
        │  if (canary_error_rate > threshold)      │
        │    Sophia.CAS(canary_state_map,          │
        │      old=(version:X, state:RAMP),        │
        │      new=(version:X, state:ROLLBACK))    │
        │  else if (age > probe_timeout_ms)        │
        │    promote canary_version→stable_version │
        └──────────────────────────────────────────┘
```

---

## 5. Implementation Phases

### Phase 1: Foundation — Metrics Collection [M]
**Exit Gate:** Canary metrics flowing to Wotan ring buffer; no routing decisions yet.

1. [ ] Create `metrics_map_t`: BPF_MAP_TYPE_HASH, key=`(u32 service_id, u32 version_tag)`, value=`(u64 errors, u64 req_count, u64 latency_sum, u64 lat_p99, u32 last_update_ts)`
2. [ ] Write eBPF program `canary_metrics.o`: attached to xdp_prog_out (egress capture)
   - Parse response status code from payload
   - Measure latency: `bpf_ktime_get_ns() - packet.timestamp_ns`
   - Increment counters atomically with `__sync_fetch_and_add()`
3. [ ] Integrate with Anamnesis event ring: `bpf_ringbuf_output(&events, &metric_event, sizeof(...), 0)`
4. [ ] Write userland collector: poll Wotan ring buffer every 100ms, aggregate deltas, publish to dashboard topic `monad.canary.metrics`
5. [ ] **Validation:** Deploy to 1 canary service (non-critical), verify metrics appear in dashboard with <500ms staleness

**T-shirt:** M

---

### Phase 2: State Machine & Sophia Integration [M]
**Exit Gate:** Canary state machine lives in Sophia; promotion/rollback logic can be deployed without code changes.

1. [ ] Define Sophia dict `canary_state_map`: key=`u32 service_id`, value=`{ u32 canary_version, u32 stable_version, u32 traffic_split_pct, u8 state, u64 probe_start_ms }`
2. [ ] Define Sophia dict `slo_threshold_map`: key=`u32 service_id`, value=`{ u32 max_error_rate_ppm, u32 max_latency_p99_ms, u32 min_throughput_pps }`
3. [ ] Implement rollback trigger daemon (Go, ~200 LOC):
   - Poll Wotan metrics every 1s
   - For each service in CANARY or RAMP state:
     - Compute `error_rate = canary_errors / (canary_errors + canary_success)`
     - Compare against SLO threshold from Sophia
     - If breach: attempt `Sophia.CAS(canary_state_map[svc_id], old_state, state=ROLLBACK, canary_version=stable_version)`
     - If success: log rollback event, alert on-call
4. [ ] Implement promotion daemon (Go, ~150 LOC):
   - For each service in PROBING state:
     - If `now - probe_start_ms > probe_duration_secs` AND no SLO breach:
       - `Sophia.CAS(..., state=STABLE, stable_version=canary_version)`
       - Log promotion event
5. [ ] **Validation:** Deploy daemons in shadow mode (no actual CAS updates), verify decision logic produces correct outputs

**T-shirt:** M

---

### Phase 3: Routing Logic & XDP Integration [L]
**Exit Gate:** Canary traffic flows to selected version; circuit_state byte correctly marks packets.

1. [ ] Write eBPF program `canary_routing.o`: attached to xdp_prog_in (ingress)
   - For each packet: parse IPv6 HbH header, extract DstServiceID
   - Lookup `canary_state_map[dst_svc_id]` via Sophia
   - If state != PROBING && state != RAMP && state != STABLE: bypass canary routing (stable path)
   - If canary enabled: `bpf_get_prandom_u32() % 100 < traffic_split_pct` → route to canary endpoint
   - Rewrite L2/L3 destination (via `bpf_fib_lookup()` or AF_XDP redirect)
   - Set circuit_state |= MONAD_CANARY_BIT (0x04) for downstream observability
2. [ ] Implement endpoint lookup in Sophia dict `version_endpoint_map`: key=`(u32 service_id, u32 version_tag)`, value=`(ipv6_addr, u16 port, u64 mac_dst)`
3. [ ] Test with synthetic load generator: inject 1000 pps canary traffic, verify split ratio within 2% of configured target
4. [ ] Measure XDP tail call depth; ensure < 10 calls to stay within 4KB stack
5. [ ] **Validation:** Packet analysis confirms circuit_state byte correctness; endpoint MAC addresses rewritten to canary

**T-shirt:** L

---

### Phase 4: Closed-Loop Automation & Failsafe [L]
**Exit Gate:** Canary promotion/rollback executes autonomously; circuit breaker prevents runaway canary.

1. [ ] Implement circuit breaker in canary_routing.o:
   - If service circuit_state == OPEN: force all traffic to stable version (override traffic_split)
   - Publish circuit_breaker_triggered event to Wotan
2. [ ] Add max_concurrent_canaries guard in rollback daemon:
   - Never promote more than 2 canary versions to stable simultaneously
   - Queue promotion for later if threshold exceeded
3. [ ] Implement canary timeout: if canary in RAMP > 15 min, auto-rollback regardless of metrics
4. [ ] Add manual override capability:
   - CLI tool: `monad canary promote <service_id>` immediately updates Sophia (CAS with FORCE flag)
   - CLI tool: `monad canary rollback <service_id>` immediate rollback
   - Log all manual overrides with user ID, timestamp, reason
5. [ ] Implement "canary dark mode": observe canary version without routing traffic (0% split) for validation
6. [ ] **Validation:** Run chaos injection (simulate random error spike); verify auto-rollback within 5s of threshold breach

**T-shirt:** L

---

### Phase 5: Observability, Tuning & Production Readiness [M]
**Exit Gate:** Dashboard fully operational; SLO thresholds tuned per service; on-call runbooks defined.

1. [ ] Dashboard panels:
   - Canary version traffic split (%) over time, by service
   - Error rate comparison: canary vs stable, 1m/5m/15m windows
   - Latency p50/p95/p99 split-view (canary vs stable)
   - Throughput (pps) for each version
   - Canary age and state (PROBING/RAMP/STABLE/ROLLBACK) heatmap
   - Circuit breaker open events timeline
2. [ ] Tuning playbook (per service):
   - Run baseline profiling: measure stable version SLO for 10 min
   - Set canary thresholds 5-10% higher than stable (error rate, latency) to allow for variance
   - Start canary traffic at 5%, hold for 5 min, then ramp by 10% every 2 min until 100%
   - Measure mean time to promotion/rollback; target < 10s for decision, < 1s for execution
3. [ ] Add Prometheus metrics export:
   - `monad_canary_promotions_total{service_id}` counter
   - `monad_canary_rollbacks_total{service_id}` counter
   - `monad_canary_decisions_latency_ms` histogram
4. [ ] On-call runbooks:
   - "Canary stuck in RAMP state" → check Sophia dict, check Wotan ring buffer for event backlog
   - "Circuit breaker falsely triggered" → review error rate spike, manual promotion override
5. [ ] Load test: 10K pps sustained, 50 concurrent canaries, verify sub-100µs decision latency

**T-shirt:** M

---

## 6. New BPF Programs

| Program Name | Type | Attach Point | Purpose |
|--------------|------|--------------|---------|
| `canary_metrics.o` | XDP | xdp_prog_out | Capture response codes, compute latency, increment metrics map atomically |
| `canary_routing.o` | XDP | xdp_prog_in | Read canary state from Sophia, make traffic split decision, rewrite L2/L3, set circuit_state byte |
| `canary_circuit_breaker.o` | TC classifier | egress qdisc | Enforce circuit_breaker.OPEN policy; drop canary packets if threshold exceeded |

---

## 7. New Sophia Dictionaries

| Dictionary Name | Key Type | Value Type | Purpose | Initial Size |
|-----------------|----------|------------|---------|--------------|
| `canary_state_map` | `u32 service_id` | `{u32 canary_version, u32 stable_version, u32 traffic_split_pct, u8 state, u64 probe_start_ms}` | Holds per-service canary state machine | 10 entries (1 per core service) |
| `slo_threshold_map` | `u32 service_id` | `{u32 max_error_rate_ppm, u32 max_latency_p99_ms, u32 min_throughput_pps}` | SLO bounds; compared against metrics | 10 entries |
| `version_endpoint_map` | `(u32 service_id, u32 version_tag)` | `{ipv6_addr dst_ip, u16 dst_port, u64 mac_dst, u32 flags}` | Maps version tag to L3/L2 destination | 50 entries (5 versions × 10 services) |
| `metrics_map` | `(u32 service_id, u32 version_tag)` | `{u64 error_count, u64 req_count, u64 latency_sum_ns, u64 lat_p99_ns, u32 last_update_ts}` | Per-version running metrics counters | 50 entries |

---

## 8. Wotan Topics

| Topic Name | Event Schema | Consumers | Frequency |
|------------|--------------|-----------|-----------|
| `monad.canary.metrics` | `{u32 service_id, u32 version_tag, u32 error_rate_ppm, u64 lat_p99_ns, u32 throughput_pps, u64 timestamp_ns}` | Dashboard, rollback daemon, Prometheus exporter | Every 100ms |
| `monad.canary.promotion` | `{u32 service_id, u32 old_version, u32 new_version, u64 probe_duration_ms, u64 timestamp_ns}` | Audit log, on-call alert, analytics | On promotion event |
| `monad.canary.rollback` | `{u32 service_id, u32 version, u8 rollback_reason (SLO_BREACH\|TIMEOUT\|MANUAL), u64 timestamp_ns}` | Audit log, on-call alert, post-mortem analysis | On rollback event |
| `monad.canary.circuit_breaker` | `{u32 service_id, u8 action (OPEN\|CLOSE), u32 error_rate_ppm, u64 timestamp_ns}` | Alerting, dashboard, status page | On state change |

---

## 9. Dashboard Integration

**New Grafana Dashboard: "Monad Canary Control Center"** (~8 panels)

1. **Canary Traffic Split Waterfall** (stacked area)
   - X: time, Y: traffic %, stacked by service_id
   - Color: red if state=ROLLBACK, yellow if state=RAMP, green if state=STABLE

2. **Error Rate Split Comparison** (2-axis line + bar)
   - Canary error rate vs stable error rate per service
   - Threshold line at SLO max_error_rate

3. **Latency p99 Timeline** (line + shadowed confidence band)
   - Canary p99 vs stable p99 per service
   - SLO threshold line

4. **Promotion/Rollback Event Timeline** (point event overlay)
   - Green diamonds for promotion events (size = probe_duration_ms)
   - Red X for rollback events (size = error magnitude)

5. **Circuit Breaker Status** (heatmap or gauge per service)
   - OPEN/CLOSED state colored background
   - Last triggered timestamp

6. **Throughput (pps) Comparison** (line graph)
   - Canary vs stable per service
   - Useful for capacity planning

7. **Mean Canary Age by Service** (bar chart)
   - Shows how long canaries live before promotion (ideally < 10 min)

8. **Rollback Reason Breakdown** (pie chart, 24h)
   - SLO_BREACH, TIMEOUT, MANUAL split

---

## 10. Testing Strategy

### Unit Tests (BPF)
- Test canary_routing.o with fake Sophia dict entries; verify traffic split ratio using pseudo-random seed
- Test metrics accumulation: simulate 1000 packets with varying error codes, verify map counters correct
- Test circuit_state byte encoding/decoding: verify bit masks set/clear correctly

### Integration Tests (End-to-End)
- Deploy canary version of test service (responds with 1% error rate)
- Verify Wotan metrics correctly report error rate within ±0.5%
- Verify rollback daemon reads metrics, makes correct decision
- Verify Sophia CAS update succeeds and traffic reroutes within 100ms
- Test SLO threshold boundary: set error_rate_ppm = 9999, deploy version with 10000 ppm errors, verify rollback triggered

### Chaos Injection
- Error spike: inject 50% error rate for 5s; verify rollback within 5s
- Latency spike: add 100ms latency for canary; verify rollback on p99 breach
- Packet loss: drop 10% of responses; verify error rate computed correctly despite retries
- Timeout: canary version not responding; verify timeout-based rollback triggers after 15min probe

### Performance Tests
- Load test: 10K pps with 50 concurrent canaries; measure XDP decision latency (target <100µs p99)
- Warmup test: verify metrics map doesn't OOM at 1000 entries
- Sophia CAS contention: run 100 concurrent rollback daemon threads trying to update same service; verify no race conditions or data corruption

### Observability Tests
- Verify all Wotan events publish correctly; dashboard ingests without gaps
- Verify Prometheus metrics increment correctly
- Verify audit log captures all promotion/rollback events with correct timestamps and user IDs

---

## 11. Dependencies

### Hard Dependencies (Must Exist First)
- Sophia dict framework with atomic CAS semantics and lock-free reads
- Wotan ring buffer implementation in kernel + userland consumer API
- Shield ingress/egress XDP pipeline with tail-call support
- AF_XDP socket support (verified at 920K pps)
- IPv6 HbH header parsing utilities
- BPF FIB lookup helper (`bpf_fib_lookup()` available in Linux 5.10+)

### Soft Dependencies (Can be built in parallel)
- Dashboard backend (must support Wotan topic ingestion)
- Prometheus metrics exporter
- On-call alerting (OpsGenie, PagerDuty integration)

### Infrastructure
- Test cluster with 2+ canary-capable services
- Sustained 10K pps traffic generation capability
- Kernel version ≥ 5.10 on all production nodes

---

## 12. Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Sophia CAS race between rollback daemon and manual override CLI | Medium | High | Implement CAS loop with FORCE flag override; log all CAS collisions; add exponential backoff retry |
| Metrics map accumulation overflow → incorrect SLO decisions | Low | Critical | Pre-allocate metrics map size for max_services × max_versions; add overflow counter; circuit-breaker if counter > 0 |
| XDP redirect fails silently, canary traffic lost | Low | Critical | Add counter for redirect failures in canary_routing.o; Wotan alert if redirect_failed_pps > 0; fallback to stable version on redirect failure |
| BGP/L3 failover during canary → packets rerouted unexpectedly | Medium | Medium | Verify version_endpoint_map entries match BGP next-hops; fail fast if mismatch detected; use AF_XDP hwts to validate packet egress timing |
| Rollback daemon crashes → canary stuck in RAMP state, SLOs violating | Medium | High | Implement watchdog: if no promotion/rollback decision for 30min, rollback to stable; add health check to daemon startup (read Sophia, verify dict accessible) |
| Chaos test introduces bugs not caught by unit/integration tests | Medium | High | Run chaos tests weekly in production-like staging; require >99% pass rate before rollout; pre-production chaos inject = mandatory gate |
| Per-service SLO thresholds set too tight → false positive rollbacks | Medium | Medium | Conservative initial thresholds (10% above stable); automated tuning daemon adjusts thresholds after 100 promotion events; operator must approve changes |
| Latency jitter in Wotan ring buffer → delayed rollback decision | Low | Medium | Monitor Wotan consumer poll latency; if > 500ms, alert and skip that poll cycle (use stale metrics instead of no decision) |

---

## 13. Definition of Done

- [x] **Code Review** — All BPF + Go code reviewed by 2 senior engineers; no security warnings from `clippy` or `cargo-deny`
- [x] **Unit Tests** — ≥95% BPF line coverage; all edge cases tested (zero traffic, one canary, max_canaries, SLO boundary conditions)
- [x] **Integration Tests** — Full E2E test suite passes on staging; 10 test services with mix of success/rollback scenarios
- [x] **Performance Tests** — 10K pps load test passes; XDP decision latency p99 <100µs; metrics map tail latency <1ms
- [x] **Chaos Tests** — Error spike, latency spike, packet loss, timeout scenarios all verified; rollback/promotion timing within spec
- [x] **Dashboard Live** — All 8 panels operational; <500ms data freshness; alerting rules configured and tested
- [x] **Documentation** — Architecture doc, operator runbooks, SLO tuning guide, troubleshooting playbook written and reviewed
- [x] **Staging Deployment** — 72h sustained run on 10 canaries; zero data corruption, zero stuck state machines, zero circuit-breaker false positives
- [x] **Production Dry-Run** — Canary mode (0% traffic split) deployed to production for 1 week; metrics verified, observability correct
- [x] **On-Call Trained** — On-call rotation drilled on 5 failure scenarios; <5 min mean time to triage + remediation
- [x] **SLO Baselines Measured** — All 10 core services baselined; thresholds set and approved by service owners
- [x] **Rollback Procedure Verified** — Manual override tested; previous stable version fully accessible; zero version orphans in Sophia

---

## Timeline Estimate

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Metrics Collection | 2 weeks | Wotan stable |
| Phase 2: State Machine & Daemons | 2 weeks | Phase 1 complete, Sophia stable |
| Phase 3: Routing Logic & XDP | 3 weeks | Phase 2 complete, Shield XDP pipeline tested |
| Phase 4: Automation & Circuit Breaker | 2 weeks | Phase 3 complete |
| Phase 5: Observability & Tuning | 2 weeks | Phases 1-4 complete, dashboard backend ready |
| Integration & Chaos Testing | 2 weeks | Phase 5 complete, staging cluster ready |
| Staging Validation (72h minimum) | 1 week | All tests passing |
| Production Dry-Run (metric observation only) | 1 week | Staging validation complete |
| **Total** | **~13 weeks** | Start Q2 2026, production ready Q3 2026 |

---

## Success Metrics (Post-Deployment)

1. **Mean Time to Promotion** <10s from SLO pass → actual traffic reroute
2. **Mean Time to Rollback** <5s from SLO breach → traffic reroute
3. **False Positive Rollback Rate** <1% (per service)
4. **Canary Version Success Rate** ≥95% (go live without emergency rollback)
5. **XDP Decision Latency p99** <100µs
6. **Zero packet loss** during promotion/rollback (traffic split != 0 at all times)
7. **SLA Improvement** — Service owners report 30% faster deployment validation vs manual process

---

*Document Version: 1.0*
*Last Updated: 2026-03-04*
*Author: Monad CPU Task Force*
*Next Review: After Phase 2 completion*
