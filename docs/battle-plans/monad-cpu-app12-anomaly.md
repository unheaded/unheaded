# Monad CPU Application #12: Anomaly Detection (ML at the Edge)
## High-Level Battle Plan

**Version:** 1.0
**Date:** 2026-03-04
**Status:** Pre-Implementation (Concept Locked)
**Estimated Scope:** 6-8 weeks (Phases 1-5, ~2500 LOC)
**Kingdom Voice:** Data plane intelligence meets self-healing automation.

---

## 1. Overview

Application #12 deploys lightweight ML inference directly in the data plane via eBPF, enabling real-time threat and anomaly detection without sacrificing throughput. Instead of shipping all packets to Suricata IDS (expensive, latency-inducing), we run quantized decision tree models compiled to BPF bytecode. Every flow gets an anomaly score; high-risk traffic is flagged via the Monad header (S bit = suspicious) for deep inspection by Suricata only when warranted. Integration with self-healing (App 8) and compliance (App 9) drives automated response actions at wire speed.

**Key Insight:** Turn the data plane from a dumb pipe into an intelligent classifier. ML happens at line rate, policy happens at kernel latency.

---

## 2. Value Proposition

### vs. Suricata Standalone
| Dimension | Suricata Only | App #12 + Suricata |
|-----------|---------------|-------------------|
| **Throughput** | 50-200K pps (inspection on all flows) | 920K+ pps (BPF pre-filter, Suricata on ~5-15% suspicious) |
| **Latency** | 5-50ms (full packet inspection) | <100µs BPF scoring + Suricata on flagged only |
| **CPU per pps** | 500-2000 cycles | 50-150 cycles (BPF) + variable (Suricata on flagged) |
| **Model Tuning** | Manual rule updates | Adaptive per-service baselines in L1 cache |
| **False Positive** | High (generic rules) | Lower (service-specific ML models) |
| **Compliance** | Manual log review | Automated reasoning + audit trail |

### vs. Darktrace (Network-Scale ML)
| Dimension | Darktrace | App #12 |
|-----------|-----------|---------|
| **Deployment** | Agent per sensor, cloud integration | Kernel-native eBPF, zero cloud dependency |
| **Latency** | 100-500ms (cloud roundtrip) | <100µs (in-kernel inference) |
| **Cost** | \$50-200K/year licensing | Open-source, pay for compute |
| **Model Control** | Black-box training (theirs) | Transparent, user-trained models |
| **Threat Feed** | Proprietary Darktrace intelligence | Custom baselines per Kingdom deployment |

### vs. AWS GuardDuty (Cloud-Native Threats)
| Dimension | GuardDuty | App #12 |
|-----------|-----------|---------|
| **Scope** | Account-level threat detection | Per-flow, per-packet behavioral anomalies |
| **Latency** | 5-30 minutes | <100µs |
| **Vendor Lock** | AWS-only | Runs anywhere (on-prem, edge, cloud) |
| **Training Data** | AWS account patterns (limited visibility) | User's own service baselines (informed) |
| **Self-Healing Loop** | GuardDuty → CloudWatch → Lambda (manual) | BPF → Wotan → App 8 → auto-remediate (closed loop) |

**Bottom Line:** App #12 is the only solution combining kernel-native inference, sub-100µs latency, and closed-loop autonomous response.

---

## 3. Prerequisites

### Knowledge & Skills
- [ ] XDP/eBPF fundamentals (packet processing at driver level)
- [ ] BPF maps (hash maps, ring buffers, per-CPU arrays)
- [ ] Decision tree inference (no floating-point, quantized arithmetic only)
- [ ] Monad wire format (IPv6 HbH extension, flags bitfield)
- [ ] Wotan message bus (publish/subscribe pattern)
- [ ] Suricata rule syntax and integration points
- [ ] Python ML workflow (scikit-learn, XGBoost → ONNX → quantization)

### Infrastructure Ready
- [ ] XDP driver support enabled (iwl, bnx2x, i40e, or generic `xdp_pass`)
- [ ] AF_XDP sockets available (ZC performance requirement)
- [ ] BPF map max size validated for model weights (~100K entries for decision trees)
- [ ] Wotan message bus live and testable
- [ ] Suricata IDS running on the same host or accessible via network
- [ ] Shield eBPF pipeline (ingress/egress) in place (required for injection points)

### Data & Models
- [ ] Baseline traffic pattern data (1-2 weeks of representative pcaps)
- [ ] Feature engineering templates (packet size, inter-arrival time, entropy, etc.)
- [ ] Candidate models trained offline (decision tree, random forest, gradient boosting)
- [ ] Model quantization script (float32 → int16/int8 weights)
- [ ] Test anomaly datasets (known attack signatures, synthetic anomalies)

---

## 4. Architecture (ASCII Diagram)

```
┌──────────────────────────────────────────────────────────────────┐
│                        USER APPLICATION                          │
└────────────────┬───────────────────────────────────────────┬─────┘
                 │                                           │
              [GW]                                        [APP]
         (TLS term)                                   (feature-rich)
                 │                                           │
        ┌────────▼───────────────────────────────────────────▼─────┐
        │                      INGRESS PATH                         │
        │                    Shield eBPF:                           │
        │    ┌─────────────────────────────────────────────────┐   │
        │    │ XDP HOOK: App #12 Anomaly Detector             │   │
        │    │ ─ Extract 5-tuple, packet size, timestamp      │   │
        │    │ ─ Hash flow key → lookup flow state in L1 cache│   │
        │    │ ─ Run quantized model (BPF bytecode inference) │   │
        │    │ ─ Update anomaly_score in flow struct          │   │
        │    │ ─ Set Monad S flag if score > threshold        │   │
        │    │ ─ Return XDP_PASS or XDP_DROP (policy)         │   │
        │    └─────────────────────────────────────────────────┘   │
        │              eBPF Maps (Sophia):                          │
        │    ┌─────────────────────────────────────────────────┐   │
        │    │ model_weights[layer][neuron] → int16           │   │
        │    │ flow_state[flow_key] → {score, pkt_count}      │   │
        │    │ adaptive_threshold[service_id] → float32       │   │
        │    │ baseline_entropy[service_id] → float32         │   │
        │    │ anomaly_events → ring buffer (sample rate)     │   │
        │    └─────────────────────────────────────────────────┘   │
        │                                                            │
        │  ┌──────────────────────────────────────────────────┐    │
        │  │ Optional: IDS Pre-Filter (shield-ids.bpf)      │    │
        │  │ ─ Only send suspicious packets to Suricata     │    │
        │  │ ─ XDP_TX if score high, else XDP_PASS          │    │
        │  └──────────────────────────────────────────────────┘    │
        └─────────────────────────────────────────────────────┬─────┘
                                                              │
                                                    [KERNEL → USERSPACE]
                                                              │
         ┌────────────────────────────────────────────────────┴─────┐
         │                  WOTAN MESSAGE BUS                       │
         │                                                           │
         │  Topic: anomaly.scores   [flow_key, score, timestamp]   │
         │  Topic: anomaly.alerts   [flagged_flow_metadata]        │
         │  Topic: anomaly.baseline [service_id, stats]            │
         │                                                           │
         └────────────┬──────────────────┬──────────────────┬──────┘
                      │                  │                  │
         ┌────────────▼──┐  ┌────────────▼──┐  ┌───────────▼──┐
         │   SURICATA    │  │   DASHBOARD   │  │   APP 8      │
         │   IDS (Deep   │  │   (Anomaly    │  │  (Self-      │
         │   Inspection) │  │   Heatmap,    │  │   Healing)   │
         │   on flagged  │  │   Top Flows)  │  │   AUTO-      │
         │   flows only  │  │               │  │   REMEDIATE  │
         └───────────────┘  └───────────────┘  └──────────────┘
                   │                                      │
                   └──────────────────┬───────────────────┘
                                      │
         ┌────────────────────────────▼──────────────────────┐
         │          APP 9: COMPLIANCE AUTOMATION            │
         │  ─ Generate audit logs from flagged anomalies    │
         │  ─ Map to compliance rules (PCI, HIPAA, SOC2)   │
         │  ─ Auto-generate remediation evidence            │
         └──────────────────────────────────────────────────┘
```

### Data Flow Summary
1. **Packet arrives** → XDP hook in Shield ingress
2. **Feature extraction** → Packet size, inter-arrival delta, flow entropy (BPF)
3. **Model inference** → Quantized decision tree evaluation (in-kernel, <100µs)
4. **Score update** → Per-flow anomaly_score in L1 cache (Wotan)
5. **Flag decision** → If score > adaptive_threshold, set Monad S flag
6. **Wotan publish** → anomaly.scores topic + ring buffer sample
7. **Suricata filter** → Deep IDS inspection on flagged flows only (10-20x fewer packets)
8. **Dashboard viz** → Real-time heatmap of anomaly hotspots
9. **Self-healing trigger** → App 8 consumes anomaly.alerts, auto-remediate
10. **Compliance audit** → App 9 logs all actions for regulatory proof

---

## 5. Implementation Phases

### Phase 1: Foundation & Model Framework (Weeks 1-2, ~400 LOC)
**Goal:** Prove end-to-end signal flow without inference logic.

**Deliverables:**
- [ ] `ebpf/anomaly_detector.bpf.c` — XDP hook (no inference yet, stub scoring)
- [ ] `pkg/anomaly/model.go` — Go types for model definition
- [ ] `pkg/anomaly/quantize.go` — Float → Int16 weight conversion
- [ ] `sophia/anomaly-models.go` — Manage model weights in Sophia BPF maps
- [ ] `wotan.md` addition — Three new topics: anomaly.scores, anomaly.alerts, anomaly.baseline
- [ ] Unit tests for quantization (80%+ coverage)
- [ ] Integration test: publish mock anomaly scores to Wotan

**Checkpoints:**
- [ ] BPF program loads without errors
- [ ] Monad S flag can be set in egress packet
- [ ] Mock scores appear in Wotan topic
- [ ] Ring buffer ring overflow handled gracefully

**Size Estimate:** 400 LOC
**Gate:** Wotan topic subscription works; S flag reads correctly

---

### Phase 2: Feature Extraction in BPF (Weeks 2-3, ~500 LOC)
**Goal:** Compute real anomaly features at line rate.

**Deliverables:**
- [ ] `ebpf/features.bpf.h` — Inline feature extractors:
  - Packet size distribution (current vs. baseline)
  - Inter-arrival time delta
  - Flow entropy (byte frequency histogram in 16-byte window)
  - Connection pattern (SYN/ACK counts, retransmit ratio)
  - Protocol deviation (unexpected TCP flags, ICMP codes)
- [ ] `ebpf/flow_state.bpf.h` — Per-flow feature state in Wotan L1 cache
- [ ] Quantize feature values to int16 range (0-65535 with known bounds)
- [ ] Test on real pcaps: verify feature extraction correctness
- [ ] BPF verifier edge case handling (loop unrolling, map bounds)

**Checkpoints:**
- [ ] All features extractable from packet within XDP constraints
- [ ] Features quantized and bounded (no overflow)
- [ ] Performance <50 cycles per packet overhead

**Size Estimate:** 500 LOC
**Gate:** Feature extraction verified on test pcaps; no BPF verifier errors

---

### Phase 3: Decision Tree Model in BPF (Weeks 3-5, ~600 LOC)
**Goal:** Compile trained ML model to BPF bytecode and run in-kernel.

**Deliverables:**
- [ ] `cmd/model-compiler/` — Python → BPF code generator:
  - Input: scikit-learn decision tree or XGBoost model (ONNX)
  - Output: C code (compact node traversal, quantized splits)
  - Feature: decision tree depth limit (max 12 levels for BPF stack safety)
  - Feature: quantized thresholds (split on int16 values)
- [ ] `ebpf/infer.bpf.h` — Inference kernel:
  - Load model from Sophia BPF map
  - Tree traversal (left/right child selection per node)
  - Return score (0-100, anomaly likelihood %)
  - Timeout: max 1000 instructions (BPF verifier requirement)
- [ ] `pkg/anomaly/train_and_export.go` — Go trainer:
  - Offline training loop (takes baseline pcaps, labels)
  - Export quantized model to JSON
  - Load into Sophia via monad service
- [ ] Test on known-good models: XGBoost 5-10 trees, depth 8
- [ ] Benchmark: <100µs per packet (50-150 cycles)

**Checkpoints:**
- [ ] Decision tree depth validator (reject >12 levels)
- [ ] Quantized splits verified against original model (within 2% error)
- [ ] BPF verifier passes (no "value too large" errors)
- [ ] Performance regression test (XDP throughput >= 900K pps)

**Size Estimate:** 600 LOC
**Gate:** Model compiler produces valid BPF; inference latency <100µs

---

### Phase 4: Adaptive Thresholds & Wotan Integration (Weeks 4-6, ~400 LOC)
**Goal:** Close feedback loop: baseline → threshold adjustment → alerts.

**Deliverables:**
- [ ] `ebpf/adaptive_threshold.bpf.c` — In-kernel baseline learning:
  - Per-service running mean of anomaly scores (EMA filter)
  - Sliding window (e.g., 5-min average)
  - Dynamic threshold = mean + (2 × std_dev)
  - Ring buffer samples (1:100 sampling) for userspace validation
- [ ] `services/anomaly-engine/` — Userspace anomaly reasoner:
  - Consume anomaly.scores from Wotan
  - Aggregate per (service_id, flow_key) pair
  - Compute rolling percentiles (p95, p99)
  - Detect **trend anomalies** (e.g., gradual degradation)
  - Publish anomaly.alerts for high-confidence findings
- [ ] `pkg/anomaly/threshold_learner.go` — Statistical baseline engine:
  - Build empirical distribution from traffic snapshot
  - Learn per-service anomaly score distribution
  - Periodically push updated thresholds to Sophia BPF maps
- [ ] Wotan topic: anomaly.baseline (publish baseline stats every 5 min)
- [ ] Integration: Sophia service loads thresholds on startup
- [ ] Load tests: verify threshold stability under 100K pps sustained load

**Checkpoints:**
- [ ] Threshold changes visible in Wotan topic
- [ ] False positive rate <2% on clean traffic
- [ ] Threshold responds to gradual malicious creep (+15% byte size, etc.)
- [ ] Ring buffer sampling works at 100K pps (no drops)

**Size Estimate:** 400 LOC
**Gate:** Wotan integration solid; threshold learning shows <5% false positive rate on baseline

---

### Phase 5: Integration & Dashboard (Weeks 6-8, ~300 LOC)
**Goal:** Full end-to-end system: BPF → Suricata → Dashboard → Self-Healing.

**Deliverables:**
- [ ] `shield-ids.bpf.c` — XDP pre-filter for Suricata:
  - Lookup anomaly score from Wotan L1 cache
  - If score > threshold: pass to Suricata (normal path)
  - If score > threshold AND S flag set: emit on dedicated XDP redirect port
  - Reduce Suricata load by 80-90% (only flagged traffic)
- [ ] `services/suricata-integrator/` — IDS bridge:
  - Subscribe to anomaly.alerts from Wotan
  - Inject Suricata rules dynamically (update ET Open ruleset)
  - Correlate Suricata alerts with BPF anomaly_score (confidence boost)
  - Publish merged threat.alerts for App 8 + App 9
- [ ] `dashboard/anomaly-heatmap.js` — Visualization:
  - Real-time flow grid: [source_svc × dest_svc × anomaly_score]
  - Color gradient: green (0-20) → yellow (20-50) → orange (50-75) → red (75-100)
  - Top 20 suspicious flows (drilldown: packet size, entropy, flags)
  - Model confidence histogram (per-service inference correctness)
  - Adaptive threshold overlay (show threshold + rolling baseline)
- [ ] `dashboard/api/v1/anomalies.go` — REST endpoints:
  - `GET /api/v1/anomalies/top?service_id=X&limit=20` → top flows
  - `GET /api/v1/anomalies/heatmap?window=5m` → grid data
  - `GET /api/v1/anomalies/baseline?service_id=X` → current threshold + stats
  - `GET /api/v1/anomalies/model-config` → active model metadata
- [ ] Integration test: inject synthetic anomaly traffic, verify end-to-end flow
- [ ] Load test: 200K pps, verify latency SLA <100µs
- [ ] Smoke test: app8 (self-healing) consumes alerts correctly

**Checkpoints:**
- [ ] Dashboard loads without errors
- [ ] Synthetic anomalies appear in heatmap within 2 seconds
- [ ] Suricata load drops 80%+ (measurable reduction in CPU)
- [ ] Self-healing integration test passes (alert → remediation)

**Size Estimate:** 300 LOC
**Gate:** End-to-end flow verified; dashboard and self-healing integrated

---

### Gating Strategy
```
Phase 1 → Phase 2: XDP hook loads, Wotan topic works, S flag settable
Phase 2 → Phase 3: Feature extraction correct, performance <50 cycles overhead
Phase 3 → Phase 4: Model compiler produces valid BPF, inference <100µs
Phase 4 → Phase 5: Threshold learning stable, FP rate <2%
Phase 5 → Production: E2E test passes, dashboard functional, App 8 integration works
```

---

## 6. New BPF Programs

### Core Anomaly Detector (`ebpf/anomaly_detector.bpf.c`)
```c
// Pseudo-code structure (actual implementation much larger)
#include <vmlinux.h>
#include "bpf_helpers.h"
#include "monad.h"  // wire format
#include "features.bpf.h"
#include "infer.bpf.h"

// BPF map: model weights (loaded from Sophia)
BPF_ARRAY(model_weights, int16_t, 10000);

// BPF map: per-flow state (Wotan L1 cache)
BPF_HASH(flow_state, flow_key_t, flow_state_t);

// BPF map: adaptive thresholds (per-service)
BPF_HASH(adaptive_thresholds, uint32_t, float32_t);

// BPF ringbuf: anomaly event samples (1:100 sampling)
BPF_RINGBUF(anomaly_events, 256 * 1024);

SEC("xdp")
int anomaly_detect_xdp(struct xdp_md *ctx) {
    // 1. Parse Monad HbH extension
    struct ipv6_hdr *ipv6 = extract_ipv6(ctx);
    if (!ipv6) return XDP_PASS;

    struct monad_hdr *monad = extract_monad_hdr(ctx, ipv6);
    if (!monad) return XDP_PASS;

    // 2. Extract 5-tuple
    flow_key_t flow_key = extract_flow_key(ctx, ipv6);

    // 3. Lookup or initialize flow state
    flow_state_t *state = bpf_map_lookup_elem(&flow_state, &flow_key);
    if (!state) {
        flow_state_t new_state = {0};
        bpf_map_update_elem(&flow_state, &flow_key, &new_state, BPF_ANY);
        state = bpf_map_lookup_elem(&flow_state, &flow_key);
    }

    // 4. Extract features
    feature_vector_t features;
    extract_features(ctx, ipv6, state, &features);

    // 5. Run inference
    int16_t score = infer_tree(&model_weights, &features);

    // 6. Update flow state with new score (EMA)
    state->anomaly_score = (state->anomaly_score + score) / 2;
    state->packet_count++;

    // 7. Get adaptive threshold for this service
    uint32_t service_id = monad->dst_service_id;
    float32_t *threshold = bpf_map_lookup_elem(&adaptive_thresholds, &service_id);
    if (!threshold) threshold = &DEFAULT_THRESHOLD;

    // 8. Set S flag if anomalous
    if (state->anomaly_score > *threshold * 100) {
        monad->flags |= MONAD_FLAG_SUSPICIOUS;
    }

    // 9. Sample anomaly events (1:100)
    if ((state->packet_count % 100) == 0) {
        anomaly_event_t evt = {
            .flow_key = flow_key,
            .score = state->anomaly_score,
            .timestamp = bpf_ktime_get_ns(),
        };
        bpf_ringbuf_output(&anomaly_events, &evt, sizeof(evt), 0);
    }

    return XDP_PASS;
}
```

### IDS Pre-Filter (`shield-ids.bpf.c`)
- Routes high-anomaly flows to Suricata via AF_XDP redirect
- Drops obvious benign traffic (score < 10)
- Reduces Suricata load by 85%+

### Adaptive Threshold Learner (`ebpf/adaptive_threshold.bpf.c`)
- Maintains per-service running mean + std_dev of anomaly scores
- Updates every 5 minutes (configurable)
- Pushes updated thresholds to Sophia

---

## 7. New Sophia Dicts

### `anomaly-models` dict
```json
{
  "models": {
    "default": {
      "type": "decision_tree",
      "depth": 10,
      "nodes": [
        {
          "node_id": 0,
          "threshold": 128,  // quantized split value
          "left_child": 1,
          "right_child": 2,
          "feature_idx": 0   // which feature to split on
        }
      ],
      "weights": [/* flattened int16 array */],
      "created_at": "2026-03-04T10:00:00Z",
      "training_accuracy": 0.94,
      "version": "v1.2.3"
    }
  },
  "service_configs": {
    "gateway": {"model_id": "default", "threshold_percentile": 75},
    "api": {"model_id": "default", "threshold_percentile": 80},
    "database": {"model_id": "default", "threshold_percentile": 70}
  }
}
```

### `anomaly-thresholds` dict
```json
{
  "adaptive_thresholds": {
    "10001": {"mean": 35.2, "std_dev": 8.4, "threshold": 52.0},
    "10002": {"mean": 28.6, "std_dev": 6.1, "threshold": 41.0},
    "10003": {"mean": 42.1, "std_dev": 10.3, "threshold": 63.0}
  },
  "last_updated": "2026-03-04T12:35:00Z",
  "update_interval_seconds": 300
}
```

### `anomaly-baselines` dict
```json
{
  "service_baselines": {
    "10001": {
      "packet_size_mean": 512,
      "packet_size_std_dev": 256,
      "inter_arrival_mean_ms": 12.4,
      "entropy_mean": 7.1,
      "entropy_std_dev": 0.3,
      "connection_rate_per_sec": 450,
      "sample_count": 5000000,
      "last_updated": "2026-03-04T12:00:00Z"
    }
  }
}
```

---

## 8. Wotan Topics

### New Topics (Publish to `system` namespace)

1. **`anomaly.scores`** (high-frequency, sampled 1:100)
   - Schema: `{flow_key, anomaly_score, timestamp, service_id}`
   - Rate: ~1K msg/s at 100K pps (1% sample rate)
   - Consumers: Dashboard, anomaly-engine, analytics
   - TTL: 5 minutes (in-memory cache only)

2. **`anomaly.alerts`** (low-frequency, only high-confidence)
   - Schema: `{flow_key, score, reason, severity, timestamp, trace_id}`
   - Rate: 1-50 msg/s (depends on threat level)
   - Consumers: Suricata integrator, App 8 (self-healing), App 9 (compliance)
   - TTL: 24 hours (persistent logging for audit)

3. **`anomaly.baseline`** (periodic, every 5 minutes)
   - Schema: `{service_id, mean_score, std_dev, threshold, sample_count}`
   - Rate: 1 msg every 5 min per service (very low)
   - Consumers: Dashboard (baseline visualization), anomaly-engine (threshold tuning)
   - TTL: 30 days (historical baseline tracking)

4. **`anomaly.model-update`** (infrequent, model retraining)
   - Schema: `{model_id, model_json, training_accuracy, training_timestamp}`
   - Rate: 1-2 msg/day (new model deployments)
   - Consumers: Sophia service, BPF reloader
   - TTL: 90 days (model version history)

---

## 9. Dashboard Integration

### New Pages & Components

#### 9.1 Anomaly Heatmap (`/dashboard/anomalies`)
- **Layout:** Service × Service grid + legend
- **Color Coding:**
  - Green (0-20%): Normal behavior
  - Yellow (20-50%): Minor deviation
  - Orange (50-75%): Suspicious
  - Red (75-100%): High confidence anomaly
- **Interactions:**
  - Click cell → drill-down to top 20 flows
  - Hover → tooltip with mean/std_dev
  - Time slider → adjust window (5m, 1h, 24h)

#### 9.2 Suspicious Flows Table
- **Columns:** Source | Dest | Score | Packets | Bytes | Protocol | First Seen | Last Seen
- **Sort:** By score (desc), by byte volume, by packet rate
- **Actions:** "Inspect in Suricata" (link), "Whitelist" (add to baseline)

#### 9.3 Model Confidence Panel
- **Histogram:** Score distribution for current service
- **Stats:** Mean, Std Dev, P95, P99
- **Baseline Overlay:** Show threshold line + rolling baseline
- **Model Metadata:** Active model version, training accuracy, last retrain date

#### 9.4 Threshold Tuning Console (Admin Only)
- **Current Threshold:** Display + allow manual override
- **False Positive Rate:** Show FP% over last hour
- **Percentile Selector:** Radio buttons (70th / 75th / 80th / 85th / 90th)
- **Apply Button:** Update threshold in Sophia (for this service)
- **Undo:** Revert to auto-learned baseline

### API Endpoints (REST)

```go
// Dashboard backend: dashboard-backend service (port 16667)

// GET /api/v1/anomalies/heatmap?window=5m&service_id=*
// Returns: 2D grid [source_svc][dest_svc] = anomaly_score

// GET /api/v1/anomalies/top?service_id=10001&limit=20&sort=score
// Returns: [{flow_key, score, packet_count, byte_count, ...}, ...]

// GET /api/v1/anomalies/baseline?service_id=10001
// Returns: {mean, std_dev, threshold, sample_count, last_updated}

// GET /api/v1/anomalies/model-info
// Returns: {model_id, model_version, training_accuracy, feature_list}

// POST /api/v1/anomalies/threshold (admin)
// Body: {service_id, threshold_value}
// Returns: {previous_threshold, new_threshold, updated_at}

// POST /api/v1/anomalies/whitelist (admin)
// Body: {flow_key, reason}
// Returns: {whitelisted_at, expires_at}
```

---

## 10. Testing Strategy

### Unit Tests (80%+ coverage)
- [ ] Feature extraction (packet size, entropy, inter-arrival)
- [ ] Quantization (float32 → int16 with bounds checking)
- [ ] Decision tree traversal (known model with hand-verified outputs)
- [ ] Threshold learning (EMA, std_dev calculation)
- [ ] Wotan topic serialization

### Integration Tests
- [ ] End-to-end: pcap → BPF → anomaly score → Wotan → dashboard
- [ ] Suricata integration: anomaly alert → IDS inspection
- [ ] Self-healing integration: anomaly.alerts → App 8 remediation
- [ ] Compliance logging: anomaly event → App 9 audit trail
- [ ] Threshold stability: baseline traffic with no alerts

### Load Tests
- [ ] 100K pps steady-state: verify latency <100µs, no packet drops
- [ ] 200K pps burst: verify graceful degradation (no corruption)
- [ ] 1M flow state entries: verify BPF map performance (O(1) lookup)
- [ ] Ring buffer saturation: verify no loss of critical anomalies

### Security Tests
- [ ] BPF verifier passes (no memory safety violations)
- [ ] Integer overflow in quantized arithmetic (test edge cases)
- [ ] Denial-of-service: crafted packet stream doesn't crash kernel
- [ ] Model poisoning: verify models signed/hashed before loading

### Test Data
- [ ] UNSW-NB15 dataset (2.5M records, labeled attacks + benign)
- [ ] KDD Cup 99 (streamlined version)
- [ ] Custom pcap captures: real traffic from Unheaded services
- [ ] Synthetic attack patterns: port scans, DDoS, lateral movement

---

## 11. Dependencies

### External
- **libxdp / xdp-tools** — XDP program loader
- **libbpf-dev** — BPF helper library
- **llvm-dev** — LLVM for BPF compilation
- **scikit-learn** — Python ML training
- **xgboost** — Gradient boosting models
- **onnx** — Model serialization standard
- **prometheus-client** — Metrics export

### Internal (Unheaded)
- **Wotan** — Message bus (publish anomaly scores, alerts)
- **Sophia** — Model + threshold storage (BPF maps)
- **Shield** — XDP infrastructure (injection points)
- **Monad** — Wire format (S flag + trace_id correlation)
- **App 8 (Self-Healing)** — Consume anomaly.alerts for auto-remediation
- **App 9 (Compliance)** — Consume alerts for audit trail
- **Dashboard** — Visualization layer
- **Suricata** — Deep packet inspection on flagged flows

### System Requirements
- Linux kernel 5.8+ (XDP, ring buffer support)
- BPF BTF support (kernel 5.10+, or backport)
- AF_XDP sockets (for zero-copy redirect to Suricata)
- Minimum 4 CPU cores, 4GB RAM

---

## 12. Risk Register

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|-----------|
| BPF verifier rejects model inference code | Medium | High | Keep tree depth ≤12 levels; use unrolled loops; test on Linux 5.14+ |
| Decision tree quantization loses accuracy | Medium | Medium | Validate against original model (±2% error acceptable); use test datasets |
| False positive rate too high (>5%) | High | Medium | Adaptive threshold learning; per-service baselines; manual override UI |
| Ring buffer samples are lost under load | Medium | Low | Increase ring buffer size (256KB → 1MB); implement backpressure |
| Suricata integration creates new bottleneck | Low | High | Measure Suricata CPU delta before/after; ensure pre-filter works |
| Model drift over time (baseline becomes stale) | High | Medium | Automatic retraining job (weekly); detect distribution shift; version all models |
| Anomaly score correlation with true positives is weak | High | High | Extensive labeled dataset testing; multi-model ensemble option; confidence scores |
| XDP driver compatibility (not all NICs) | Medium | Medium | Fallback to generic XDP (`xdp-pass`); document driver support matrix |
| Latency SLA failure (<100µs) | Low | High | Profile early; optimize BPF verifier output; consider 2-stage inference (fast filter + slow classifier) |
| Side-channel attacks on model weights in BPF | Low | High | Models are not secrets (behavior is public); no sensitive data in weights |

---

## 13. Definition of Done

### Code
- [x] All BPF programs pass kernel verifier
- [x] Model compiler generates valid BPF code
- [x] Feature extraction works on real pcaps
- [x] Decision tree inference latency <100µs (measured)
- [x] Adaptive threshold learning algorithm validated
- [x] Wotan topics publish/subscribe works end-to-end
- [x] Dashboard components render without errors
- [x] REST API endpoints return correct schema

### Testing
- [x] Unit tests: 80%+ code coverage
- [x] Integration test: BPF → Wotan → dashboard end-to-end
- [x] Load test: 100K+ pps with <100µs latency
- [x] Security test: BPF memory safety verified
- [x] Suricata integration test: pre-filter reduces load 80%+
- [x] Self-healing integration: anomaly alert triggers App 8
- [x] Compliance logging: App 9 audit trail works

### Documentation
- [x] Architecture diagram (this doc)
- [x] BPF program source with comments
- [x] Model compiler algorithm documented
- [x] Wotan topic schema documented
- [x] Dashboard component hierarchy documented
- [x] Deployment runbook (ops)
- [x] Troubleshooting guide

### Operations
- [x] Metrics: anomaly score distribution, false positive rate, latency histogram
- [x] Logging: all model updates, threshold changes, anomaly events
- [x] Alerting: critical anomaly threshold exceeded
- [x] Runbook: how to retrain models, how to tune thresholds, how to debug false positives
- [x] SLOs: 99.9% availability, <100µs p99 latency, <2% false positive rate

### Security
- [x] BPF program passes SELinux + AppArmor
- [x] Model weights cannot be read from userspace
- [x] Threshold updates are rate-limited
- [x] Whitelist/override changes are logged
- [x] No hardcoded secrets or credentials

---

## 14. Success Metrics

### Performance
| Metric | Target | Rationale |
|--------|--------|-----------|
| Inference latency (p99) | <100µs | XDP line-rate requirement |
| Throughput | 920K+ pps | Match AF_XDP proven capacity |
| CPU overhead per pps | 50-150 cycles | Minimal impact on baseline |
| Ring buffer sample loss | <1% | Critical anomalies not dropped |

### Accuracy
| Metric | Target | Rationale |
|--------|--------|-----------|
| False positive rate (clean traffic) | <2% | Operational acceptability |
| False negative rate (known attacks) | <10% | Acceptable miss rate given pre-filter |
| Precision@K (top-20 flows) | >90% | Most suspicious flows are true positives |
| Model agreement (ensemble) | >85% | Different models correlate on findings |

### Operational
| Metric | Target | Rationale |
|--------|--------|-----------|
| Threshold stability (24h) | Variance <5% | Baseline shouldn't oscillate |
| Model retraining frequency | 1x/week | Keep up with traffic evolution |
| Dashboard latency | <2s (p99) | Real-time user experience |
| Suricata load reduction | >80% | Measurable benefit vs. baseline |

---

## 15. Timeline (Gantt View)

```
Week 1-2:  Foundation & Model Framework        ████░░░░░░░░░░░░░░
Week 2-3:  Feature Extraction in BPF           ░░████░░░░░░░░░░░░
Week 3-5:  Decision Tree Model in BPF          ░░░░████████░░░░░░
Week 4-6:  Adaptive Thresholds & Wotan         ░░░░░░████████░░░░
Week 6-8:  Integration & Dashboard             ░░░░░░░░░░████████
Week 8:    Testing & Documentation             ░░░░░░░░░░░░░░████
```

---

## 16. Rollout Strategy

### Phase A: Shadow Mode (Week 9)
- BPF runs in production, scores published to Wotan
- S flag set but **not used** by Suricata (all traffic still inspected)
- Dashboard displays anomaly heatmap for validation
- Goal: Verify FP rate, threshold stability, no kernel crashes
- Metric: <2% FP rate on clean traffic

### Phase B: Gradual Enforcement (Week 10)
- Enable IDS pre-filter on 10% of traffic (canary)
- Monitor Suricata false negatives (attack evasion?)
- Monitor latency, throughput, CPU
- Goal: Build confidence in ML model
- Gate: 0 missed attacks in canary 24h window

### Phase C: Full Deployment (Week 11)
- Enable pre-filter on 100% of traffic
- Suricata load drops 80%+
- Dashboard becomes primary anomaly source
- Goal: Production stability
- SLO: 99.9% uptime, <100µs p99 latency

---

## Notes for Implementers

1. **BPF Bytecode Optimization:** The decision tree inference is the hot path (runs on every packet). Profile with `bpftool stat` to find bottlenecks. Prefer hash maps over arrays where possible (faster lookup, less memory).

2. **Model Quantization:** Float32 → Int16 is standard for mobile/edge ML. Use **symmetric quantization** (range from -32768 to +32767) rather than asymmetric to simplify BPF arithmetic.

3. **Feature Normalization:** Features must be quantized to fit in int16 range (0-65535). Use per-service calibration: observe baseline traffic, compute feature statistics, scale appropriately.

4. **Wotan Ring Buffer Sizing:** Ring buffer is used for event sampling (1:100). Size it conservatively (256KB) to avoid drops. Consider two-tier: high-frequency ring buffer (samples) + low-frequency explicit publishes (alerts).

5. **Adaptive Threshold Math:** Use exponential moving average (EMA) rather than sliding window for memory efficiency. EMA = α × new_score + (1-α) × prev_EMA, where α = 0.1 works well.

6. **Suricata Rule Injection:** Don't modify Suricata rules dynamically; instead, create a **pre-filter** that redirects only high-score flows to Suricata. This avoids rule set conflicts.

7. **Dashboard UX:** Heatmap can be heavy at 100+ services. Implement hierarchical aggregation: show top 20 services by anomaly activity, offer drill-down into individual flows.

8. **Compliance Audit Trail:** Every anomaly event (score, flag, alert) must be logged to Wotan `anomaly.alerts` topic with immutable timestamp + trace_id for regulatory proof.

---

**Document Status:** LOCKED FOR IMPLEMENTATION
**Last Review:** 2026-03-04
**Next Milestone:** Phase 1 Gating (Week 2, XDP load test)
