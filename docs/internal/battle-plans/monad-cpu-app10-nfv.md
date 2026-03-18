# Monad CPU Application #10: NETWORK FUNCTION CHAINING (ZERO-VM NFV) — Battle Plan

**Date**: 2026-03-04
**Application**: #10 — Zero-Overhead Service Function Chains
**Prerequisite**: Monad CPU engine operational, Shield (XDP pipeline) in place, Anamnesis ring buffer working
**Target**: Service function chains executing in single XDP pass (~microseconds), zero VM/container overhead, dynamic function addition via Sophia dictionary update
**Estimated Duration**: 3-4 weeks across 4-5 sprints (implementation + testing + production validation)
**Agent Strategy**: Phase 0-1 sequential (design). Phases 2-4 parallelizable (BPF, Sophia config, testing). Phase 5 sequential (integration). Phase 6 parallelizable (dashboard, docs). Phase 7 sequential (final gate).
**Commit Cadence**: Every 4-5 steps

---

## OVERVIEW

**Network Function Chaining (NFV)** traditionally requires dedicated VMs or containers per function:
- Firewall appliance (VM #1)
- NAT gateway (VM #2)
- Rate limiter (VM #3)
- Load balancer (VM #4)
- Router/egress (VM #5)

Each requires memory overhead (512MB minimum), CPU time (context switches), network overhead (inter-function traffic), and operational complexity (VM orchestration, monitoring, failover).

**Monad CPU App #10 reimagines this as pure packet-instruction execution:** Functions chain as XDP tail-calls in a single pass, with per-chain telemetry logged to Anamnesis. No VM spawning. No context switches. No container orchestration. Entire chain executes in **microseconds**, not milliseconds.

**Core Innovation**: Service function chain defined declaratively in Sophia dictionary → XDP programs loaded dynamically → functions chained via `bpf_tail_call()` → per-function decisions recorded in Anamnesis ring buffer → circuit breaker (circuit_state flag) enables pre-authorized flow bypass.

---

## VALUE PROPOSITION

| Aspect | Traditional NFV | Cilium Service Mesh | Monad NFV |
|--------|-----------------|--------------------|-----------|
| **Per-Function Overhead** | 512MB VM + 50ms boot | Sidecar container + Envoy | Zero (shared XDP context) |
| **Chain Latency** | 20-100ms (context switches) | 5-20ms (Envoy + network) | 10-100μs (single XDP pass) |
| **Function Count** | 3-5 practical limit | 10-20 sidecar limit | 10+ tail-call limit (BPF verifier) |
| **Dynamic Reconfiguration** | VM restart required | Envoy reload (~5s) | Sophia dict update (~1ms, atomic) |
| **Observability** | VM-level metrics | App-level tracing | Per-function timing in Anamnesis |
| **Memory Cost** | ~2.5GB for 5 functions | ~50MB for sidecar fleet | ~5MB for 10 BPF programs in maps |
| **Deployment Model** | IaC + orchestrator | Helm + Kubernetes | Git config → Sophia dict → XDP |
| **Circuit Breaking** | Manual/external | Envoy circuit breaker | `circuit_state` flag in Monad header |

**Monad NFV wins on**: latency (100x), resource cost (500x), reconfiguration speed (5x), observability granularity (per-packet vs per-app).

---

## PREREQUISITES

**Must-Have (Before Phase 1)**:
- [ ] Monad CPU engine: eBPF XDP programs running in Shield (ingress/egress pipeline)
- [ ] Monad wire format: 20-byte IPv6 HbH header with trace_id, circuit_state, flags
- [ ] Sophia service: BPF map dictionaries for config/state (port 19005)
- [ ] Anamnesis: 64-byte event ring buffer receiving packets from XDP
- [ ] BPF tail-call infra: `bpf_tail_call()` working between programs
- [ ] AF_XDP zero-copy socket: 920K pps proven baseline
- [ ] Linux test environment: 5.10+ kernel with BPF JIT enabled

**Build Tools**:
- Rust + Aya (eBPF program compilation)
- Go 1.21+ (control plane)
- bpftool (BPF map inspection)
- bpf2go (if using cilium/ebpf)
- Prometheus + Grafana (metrics dashboard)

---

## ARCHITECTURE

### Wire Format Augmentation
```
IPv6 Hop-by-Hop Extension Header (20 bytes)

Offset  Field                    Bytes   Purpose
0       Version                  1       Protocol version (0x01)
1-2     SrcServiceID             2       Source service (16-bit)
3-4     DstServiceID             2       Destination service (16-bit)
5-12    trace_id                 8       Distributed trace correlation
13      QoS                      1       Quality of Service level
14      circuit_state            1       [AUTH:2 bits | CHAIN_INDEX:6 bits]
15      flags                    1       C|Y|T|E|S|M|K1|K0
16-17   reserved                 2       For future use
18-19   CRC-16                   2       Header integrity check

NEW for NFV:
  - circuit_state.CHAIN_INDEX (6 bits): 0-63 = chain ID in Sophia["chains"]
  - circuit_state.AUTH (2 bits):
      00 = normal (execute all functions)
      01 = priority (skip non-critical functions via circuit_state flag)
      10 = reserved
      11 = experimental (bypass all functions, used for pre-auth flows)
```

### Sophia Dictionary Structure
```
sophia["chains"] = {
  "chain_0": {
    "name": "ingress_default",
    "description": "Default ingress: firewall → NAT → rate-limit → route",
    "functions": [
      {"id": 0, "prog": "firewall_deny", "timeout_us": 100, "required": true},
      {"id": 1, "prog": "nat_xlate", "timeout_us": 150, "required": true},
      {"id": 2, "prog": "rate_limiter", "timeout_us": 50, "required": false},
      {"id": 3, "prog": "lb_route", "timeout_us": 200, "required": true}
    ],
    "next_on_drop": "chain_drop_handler",
    "telemetry_enabled": true
  },
  "chain_1": { ... // fast_path_https
  "chain_drop_handler": { ... // cleanup functions
}

sophia["function_cache"] = {
  "firewall_deny": { "prog_fd": 5, "priority": 100 },
  "nat_xlate": { "prog_fd": 6, "priority": 90 },
  "rate_limiter": { "prog_fd": 7, "priority": 50 },
  "lb_route": { "prog_fd": 8, "priority": 200 }
}
```

### XDP Tail-Call Chaining
```
XDP Ingress Entry Point
  ↓
Load chain definition from Sophia["chains"][circuit_state.CHAIN_INDEX]
  ↓
For each function in chain:
  ├─ Load function prog_fd from Sophia["function_cache"]
  ├─ Call via bpf_tail_call(ctx, prog_array_map, func_id)
  ├─ On return: record decision to Anamnesis (function_id, latency_ns, action)
  └─ If circuit_state.AUTH == BYPASS: skip remaining functions
  ↓
Final action (ALLOW/DROP/REDIRECT) written to packet metadata
  ↓
Return XDP_PASS/XDP_DROP/XDP_REDIRECT
```

### Anamnesis NFV Event (Per Function Execution)
```
struct nfv_event {
  uint8_t   event_type;       // ANAMNESIS_EVENT_NFV_FUNCTION_EXEC = 0x09
  uint8_t   chain_id;         // 0-63 (from circuit_state.CHAIN_INDEX)
  uint8_t   function_index;   // 0-15 (position in chain)
  uint8_t   decision;         // 0=ALLOW, 1=DROP, 2=REDIRECT, 3=RATE_LIMITED
  uint32_t  latency_ns;       // nanoseconds (if < 2^32 ns)
  uint32_t  packets_processed;// counter for this function
  uint64_t  timestamp_ns;     // wall clock
} // 24 bytes
```

### System Diagram
```
                   ┌─────────────────────────────┐
                   │    Incoming Packet          │
                   │  (IPv6 HbH header parsed)   │
                   └──────────────┬──────────────┘
                                  │
                   ┌──────────────▼──────────────┐
                   │  Load Chain ID from         │
                   │  circuit_state.CHAIN_INDEX  │
                   └──────────────┬──────────────┘
                                  │
         ┌────────────────────────▼────────────────────────┐
         │                                                 │
    ┌────▼──────┐  ┌───────────┐  ┌──────────────┐  ┌─────▼──────┐
    │ Function  │  │ Function  │  │ Function     │  │ Function   │
    │ 0: Firewall   │ 1: NAT    │  │ 2: Rate-Lim  │  │ 3: LB/Route│
    │ (deny)    │  │ (xlate)   │  │ (limit)      │  │ (route)    │
    └────┬──────┘  └────┬──────┘  └──────┬───────┘  └─────┬──────┘
         │              │                │                │
         └──────────────┼────────────────┼────────────────┘
                        │
                   ┌────▼──────────────┐
                   │  Record to        │
                   │  Anamnesis        │
                   │  (per-function)   │
                   └────┬──────────────┘
                        │
         ┌──────────────▼──────────────┐
         │ Final Action                │
         │ (ALLOW/DROP/REDIRECT)       │
         └──────────────┬──────────────┘
                        │
         ┌──────────────▼──────────────┐
         │ Return XDP_PASS/DROP/       │
         │ REDIRECT with metadata      │
         └─────────────────────────────┘
```

---

## IMPLEMENTATION PHASES

### PHASE 0: INTELLIGENCE & DESIGN (3 hours)
**Goal**: Understand current XDP pipeline, BPF tail-call infra, Sophia dict structure.

**Deliverables**:
- [ ] BPF tail-call maximum function count verified (usually 32-64)
- [ ] Sophia dict schema for chains + function_cache confirmed
- [ ] Anamnesis event types enumerated (add NFV event type)
- [ ] Design doc: circuit_state field layout (AUTH + CHAIN_INDEX bits)
- [ ] Security model: what prevents malicious function injection?

**Key Questions**:
- Current BPF prog limit in array maps: how many tail-call targets?
- Sophia atomic update latency: what's the max duration for dict change?
- Anamnesis ring buffer: per-function events = 64 bytes × N functions per packet. Overflow risk?

### PHASE 1: BPF INFRASTRUCTURE (4-6 hours)
**Goal**: Implement chain loader, function dispatcher, per-function telemetry.

**New Programs**:
1. **`nfv_chain_loader`** (~200 lines Rust/Aya)
   - Entry point for chain #N
   - Load chain definition from Sophia["chains"][N]
   - Verify all function prog_fds are loaded and valid
   - Set up loop counter (prevent infinite tail-call recursion)
   - Tail-call to first function in chain

2. **`nfv_function_dispatcher`** (~300 lines Rust/Aya)
   - Called after each function returns
   - Record function completion to Anamnesis
   - Check circuit_state.AUTH flag:
     - Normal (00): execute next function
     - Priority (01): skip non-required functions, continue
     - Experimental (11): skip to final action
   - Tail-call to next function or exit

3. **`nfv_final_action`** (~150 lines Rust/Aya)
   - Aggregate all function decisions
   - Determine final action (ALLOW/DROP/REDIRECT)
   - Apply to packet (mark for redirect, etc.)
   - Update Anamnesis with final outcome
   - Return control to main XDP path

**Sophia Maps Needed**:
```
prog_array_map[u32] = u32  // maps function_index → prog_fd
                           // for bpf_tail_call() dispatch
chain_config_map[u32] = chain_definition
                           // maps chain_id → function list
                           // updated atomically from Sophia
```

**Tests**:
- [ ] Tail-call depth test: max 10 functions in chain, all execute
- [ ] Sophia dict update: add new function, verify dispatcher sees it
- [ ] Anamnesis recording: per-function events logged, ring buffer doesn't overflow
- [ ] Circuit state bypass: AUTH=11 skips all functions
- [ ] Latency measurement: total chain latency + per-function breakdown

### PHASE 2: SAMPLE FUNCTIONS (3-4 hours)
**Goal**: Implement 4-5 production-quality function examples.

**Function 1: Firewall (stateless deny-list)**
- Input: packet, source IP
- Logic: Check against Sophia["deny_ips"] set
- Output: ALLOW or DROP
- Latency target: <100μs
- Decision recorded: [ALLOW | DROP]

**Function 2: NAT (IPv6-to-IPv6 address translation)**
- Input: packet with dest IPv6 address
- Logic: Look up in Sophia["nat_mappings"] dict
- Output: Rewritten packet (dst addr updated) or passthrough
- Latency target: <150μs
- Decision recorded: [REWRITE | PASSTHROUGH]

**Function 3: Rate Limiter (token bucket)**
- Input: packet with src IP
- Logic: Check/decrement token count in BPF map
- Output: ALLOW or DROP (if rate exceeded)
- Latency target: <50μs
- Decision recorded: [ALLOW | RATE_LIMITED]

**Function 4: Load Balancer / Router**
- Input: packet with dest IP + destination service ID
- Logic: Select backend from Sophia["backends"] for DstServiceID
- Output: Redirect to backend or DROP
- Latency target: <200μs
- Decision recorded: [REDIRECT | DROP]

**Function 5: Egress Classifier (optional)**
- Input: packet marked by previous functions
- Logic: Apply QoS policy based on Monad QoS field
- Output: Mark for egress queue assignment
- Latency target: <50μs
- Decision recorded: [QoS_SET | PASSTHROUGH]

**Each function file**: ~300-400 lines Rust/Aya
**Total**: ~1500-2000 lines new BPF code

---

### PHASE 3: SOPHIA CONTROL PLANE (2-3 hours)
**Goal**: REST API endpoints for dynamic chain definition + function loading.

**Endpoints**:

1. **POST /api/v1/chains** — Create/update chain definition
   ```json
   {
     "chain_id": 0,
     "name": "ingress_default",
     "functions": [
       {"id": 0, "prog": "firewall_deny", "timeout_us": 100, "required": true},
       {"id": 1, "prog": "nat_xlate", "timeout_us": 150, "required": true}
     ],
     "next_on_drop": "chain_drop_handler"
   }
   ```
   - Validates function prog_fds exist
   - Atomically updates Sophia["chains"] + ["function_cache"]
   - Returns: HTTP 200 + chain_id

2. **GET /api/v1/chains/{chain_id}** — Fetch chain definition

3. **DELETE /api/v1/chains/{chain_id}** — Remove chain (fail if active)

4. **POST /api/v1/functions/reload** — Hot-reload BPF function
   - Validates new prog_fd
   - Updates Sophia["function_cache"]
   - Old function still active for 100ms (in-flight packets)
   - Returns: HTTP 202 Accepted (async)

5. **GET /api/v1/chains/{chain_id}/telemetry** — Per-function stats
   ```json
   {
     "chain_id": 0,
     "functions": [
       {"index": 0, "name": "firewall_deny", "calls": 15000, "avg_latency_us": 45, "drops": 120},
       {"index": 1, "name": "nat_xlate", "calls": 14880, "avg_latency_us": 78, "rewrites": 12000}
     ],
     "total_latency_us": 180,
     "packets_processed": 15000
   }
   ```

**Code Location**: `services/sophia/chains.go` (~400 lines)
**Dependencies**: Sophia dict access, Anamnesis event aggregation

---

### PHASE 4: INTEGRATION WITH EXISTING PIPELINE (2-3 hours)
**Goal**: Wire NFV chain loader into main XDP ingress/egress paths.

**Changes to Shield (XDP pipeline)**:
1. At ingress entry point: check circuit_state.CHAIN_INDEX
2. If valid (0-63): tail-call to `nfv_chain_loader[circuit_state.CHAIN_INDEX]`
3. On return from chain: apply final action (ALLOW/DROP/REDIRECT)
4. At egress: apply any egress-specific chains (optional)

**Integration Points**:
- [ ] Update ingress XDP to check circuit_state before processing
- [ ] Integrate Anamnesis event recording (already in place)
- [ ] Update Monad wire format validation (ensure circuit_state properly formatted)
- [ ] Add metrics: chain_latency_histogram, functions_skipped_counter, circuit_bypass_counter

**Code Location**: `ebpf/shield_ingress.rs` (~100 lines added)

---

### PHASE 5: TESTING (4-5 hours)
**Goal**: Unit + integration + load tests for NFV chains.

**Unit Tests** (Go):
```go
TestLoadChainFromSophia()           // 50 lines
TestFunctionDispatcher()             // 75 lines
TestCircuitStateBypass()             // 60 lines
TestAnamnesisEventRecording()        // 80 lines
TestDynamicFunctionReload()          // 100 lines
```

**Integration Tests** (Go):
```bash
# Test 1: Chain execution with 4 functions
TestFullChainExecution()             // packet → all 4 functions → final action

# Test 2: Dynamic chain update
TestDynamicChainUpdate()             // update chain while traffic flowing

# Test 3: Circuit breaker
TestCircuitBreakerBypass()           // verify AUTH=11 skips all functions

# Test 4: Per-function latency
TestPerFunctionLatency()             // verify Anamnesis records individual timings
```

**Load Test** (Iperf):
- [ ] 100K pps for 60s through 4-function chain
- [ ] Verify no packet loss
- [ ] Verify avg latency < 200μs per chain
- [ ] Verify Anamnesis doesn't drop events

**Performance Baseline**:
- Without NFV chain: 920K pps baseline (AF_XDP proven)
- With NFV chain (4 functions): Target ≥500K pps (54% throughput)
- Per-packet latency: <300μs (acceptable for inline processing)

---

### PHASE 6: DASHBOARD VISUALIZATION (3-4 hours)
**Goal**: Real-time NFV chain telemetry in dashboard.

**New Widgets**:
1. **Chain Execution Flow**
   - Nodes: each function in active chain
   - Edges: packet flow (input → function 0 → 1 → ... → action)
   - Metrics: latency bar per node, decision counts
   - Color: green (ALLOW), red (DROP), yellow (REDIRECT)

2. **Per-Chain Stats Table**
   ```
   Chain ID | Name | Functions | Total Latency | Packets/s | Drop Rate
   0        | ingress_default | 4 | 180μs | 500K | 0.8%
   1        | fast_path_https | 2 | 85μs | 320K | 0.1%
   ```

3. **Function Heatmap** (latency vs function count)
   - Y-axis: function count (1-10)
   - X-axis: time
   - Color intensity: latency (green=fast, red=slow)
   - Anomaly detection: alert if function latency spikes 3x baseline

4. **Circuit Breaker Monitor**
   - Gauge: % of flows in each AUTH state (NORMAL / PRIORITY / EXPERIMENTAL)
   - Table: bypass counts per chain per minute

**Code Location**: `dashboard/js/nfv-chains.js` (~400 lines)

---

### PHASE 7: DOCUMENTATION & HANDOFF (2-3 hours)
**Goal**: Operators can deploy, configure, and debug NFV chains.

**Docs**:
1. **NFV User Guide** (`docs/NFV-USER-GUIDE.md` ~1000 lines)
   - Architecture overview
   - Creating custom functions (template + examples)
   - Deploying chains via Sophia API
   - Troubleshooting guide
   - Performance tuning

2. **BPF Function Development Template** (`ebpf/nfv_function_template.rs` ~200 lines)
   - Boilerplate for new function
   - Telemetry recording pattern
   - Testing harness

3. **Sophia NFV Configuration Reference** (`docs/SOPHIA-NFV-API.md` ~400 lines)
   - JSON schema for chain definitions
   - API endpoint reference
   - Hot-reload semantics
   - Latency SLA configuration

4. **Operational Runbook** (`docs/NFV-OPERATIONS.md` ~500 lines)
   - Monitoring & alerting rules
   - Common issues & recovery
   - Performance baseline & tuning
   - Circuit breaker policy examples

5. **Protocol Spec Update** (`docs/protocol/MONAD-NFV-EXTENSION.md` ~300 lines)
   - circuit_state field semantics
   - Anamnesis NFV event type definition
   - BPF tail-call best practices
   - Security considerations (function injection attacks)

---

## NEW BPF PROGRAMS

| Program | Lines | Purpose | Latency Target |
|---------|-------|---------|----------------|
| `nfv_chain_loader` | 200 | Loads chain def, dispatches to first function | <10μs |
| `nfv_function_dispatcher` | 300 | Records function outcome, tail-calls next | <20μs |
| `nfv_final_action` | 150 | Aggregates decisions, applies final action | <30μs |
| `firewall_deny` | 350 | Stateless deny-list lookup | <100μs |
| `nat_xlate` | 400 | IPv6 address rewrite | <150μs |
| `rate_limiter` | 300 | Token bucket rate limiting | <50μs |
| `lb_route` | 450 | Backend selection + redirect | <200μs |
| **Total NFV Core** | **2150** | | |

---

## NEW SOPHIA DICTS

| Dict | Keys | Value Type | Purpose |
|------|------|------------|---------|
| `chains` | `"chain_0"..N` | Chain def (JSON) | Active chain definitions per CHAIN_INDEX |
| `function_cache` | function name | {prog_fd, priority} | Function prog_fd lookup for tail-call |
| `deny_ips` | IPv6 address | 1 (presence marker) | Firewall deny-list (for firewall_deny function) |
| `nat_mappings` | "src_addr:src_port:proto" | "dst_addr:dst_port" | NAT translation table |
| `rate_limiter_state` | "ip:bucket_id" | {tokens, timestamp} | Token bucket state (per-IP) |
| `backends` | "svc_id:lb_key" | [addr1, addr2, ...] | Load balancer backend pool |

---

## WOTAN TOPICS

| Topic | Message Schema | Purpose |
|-------|---|---------|
| `nfv.chain.created` | {chain_id, name, function_count} | Chain definition added/updated |
| `nfv.chain.deleted` | {chain_id, name} | Chain removed (audit) |
| `nfv.function.loaded` | {function_name, prog_fd, latency_us} | Function hot-reloaded |
| `nfv.stats.per_chain` | {chain_id, packets_processed, drops, total_latency_us} | Periodic chain telemetry (10s interval) |
| `nfv.circuit_breaker` | {flow_id, auth_state, bypass_reason} | Circuit breaker state transitions |

---

## DASHBOARD INTEGRATION

**New Endpoints**:
- `GET /api/v1/nfv/chains` — List all active chains with current stats
- `GET /api/v1/nfv/chains/{id}/functions` — Per-function latency breakdown
- `GET /api/v1/nfv/telemetry/timeline` — Time-series latency + drop-rate
- `WebSocket /ws/nfv/stats` — Real-time chain execution flow visualization

**Dashboard Pages**:
1. **NFV Chains Overview** — Table of all chains + global stats
2. **Chain Detail** — Function execution graph + per-function metrics
3. **Circuit Breaker Monitor** — Auth state distribution + bypass anomalies
4. **Function Performance** — Latency heatmap + outlier detection

---

## TESTING STRATEGY

### Unit Tests (BPF)
- [ ] Chain loader: verify correct function sequence loaded
- [ ] Function dispatcher: verify next function called, decision recorded
- [ ] Tail-call limits: verify graceful exit if >10 levels deep
- [ ] Sophia dict access: verify stale data handled

### Integration Tests (Go + BPF)
- [ ] E2E chain execution: packet traverses 4-function chain, final action applied
- [ ] Dynamic chain update: add/remove function while traffic flowing
- [ ] Latency measurement: Anamnesis events match per-function execution
- [ ] Circuit breaker: AUTH state properly bypasses functions
- [ ] Function hot-reload: new prog_fd takes effect within 100ms

### Load Tests
- [ ] 500K pps for 300s through 4-function chain
- [ ] Measure: max latency, 99th percentile, packet loss
- [ ] Verify Anamnesis doesn't drop events under sustained load
- [ ] Verify function reloading doesn't drop in-flight packets

### Security Tests
- [ ] Malicious function injection: verify only Sophia-registered functions can be tail-called
- [ ] Tail-call bombs: prevent infinite loops via loop counter
- [ ] Buffer overflow: verify Anamnesis ring buffer doesn't overflow during peak load
- [ ] Circuit state corruption: verify auth bits can't be forged by packet (kernel-only write)

---

## DEPENDENCIES

### External Libraries
- `aya` (Rust) — BPF program compilation + verification
- `libbpf` (C) — BPF map access from userspace
- `prometheus` (Go) — Metrics export

### Internal Services
- **Sophia** (port 19005) — Chain definition storage + BPF dict atomicity
- **Anamnesis** — Event ring buffer (64-byte entries)
- **Shield** (XDP) — Main packet processing pipeline
- **Monad wire format** — circuit_state field (chain index + auth bits)

### Kernel Requirements
- Linux 5.10+ (BPF tail calls stable)
- BPF JIT enabled
- BPF verifier updated for complex programs (>10K instructions)

---

## RISK REGISTER

| Risk | Severity | Mitigation |
|------|----------|-----------|
| **BPF verifier rejects complex chain loader** | HIGH | Architect program modularly, test with small chains first, use verifier directives |
| **Tail-call recursion depth exceeded** | MEDIUM | Hard limit at 10 functions per chain, with fallback to drop |
| **Sophia dict update not atomic** | MEDIUM | Use Sophia's atomic update API, validate all chain_fds before commit |
| **Anamnesis ring buffer overflow** | MEDIUM | Size buffer for peak load + margin, monitor ring buffer fullness metric |
| **Function hot-reload drops packets** | MEDIUM | Keep old prog_fd active for 100ms grace period, use "N-1" deployment strategy |
| **Latency SLA exceeded under load** | MEDIUM | Load test with realistic packet patterns, profile function hotspots, optimize critical path |
| **Function injection vulnerability** | HIGH | Only Sophia-registered functions can be tail-called, kernel-only circuit_state writes, audit logs |

---

## DEFINITION OF DONE

All of the following must be true:

- [ ] **Code**
  - [ ] All 3 core programs compile without BPF verifier errors
  - [ ] All 5 function examples functional and tested
  - [ ] Integration with Shield XDP complete
  - [ ] Sophia API endpoints functional

- [ ] **Testing**
  - [ ] Unit tests: 80%+ coverage of BPF programs
  - [ ] Integration tests: all 6 E2E test cases pass
  - [ ] Load test: 500K pps sustained for 5 minutes, zero packet loss
  - [ ] Security tests: function injection blocked, tail-call bombing prevented

- [ ] **Documentation**
  - [ ] NFV User Guide written (1000+ lines)
  - [ ] Protocol spec updated (circuit_state + NFV event types)
  - [ ] Operational runbook written
  - [ ] BPF function template + examples

- [ ] **Dashboard**
  - [ ] Chain execution flow visualization working
  - [ ] Per-function latency breakdown displayed
  - [ ] Circuit breaker monitor active
  - [ ] Real-time WebSocket stream active

- [ ] **Observability**
  - [ ] Anamnesis recording NFV events correctly
  - [ ] Prometheus metrics exported (chain_latency, drops, bypass_count)
  - [ ] Wotan topics being published
  - [ ] Dashboard receiving live data

- [ ] **Validation**
  - [ ] Manual test: add new function via Sophia API, verify it executes
  - [ ] Manual test: hot-reload function, verify new version takes effect
  - [ ] Manual test: circuit breaker bypass, verify functions skipped
  - [ ] Manual test: dashboard shows realistic latency breakdown

- [ ] **Production Readiness**
  - [ ] All 10 services (including Sophia) pass full integration test
  - [ ] eBPF programs verified against production kernel (5.10+)
  - [ ] Performance baseline documented (500K+ pps)
  - [ ] Failure modes documented + recovery steps

---

## APPENDIX A: QUICK REFERENCE

### Monad circuit_state Field (App #10 Semantics)
```
Byte 14 (circuit_state):

Bits 7-6: AUTH (2 bits)
  00 = NORMAL: Execute all functions in chain
  01 = PRIORITY: Skip non-required (required=false) functions
  10 = RESERVED
  11 = EXPERIMENTAL: Skip all functions (pre-auth bypass)

Bits 5-0: CHAIN_INDEX (6 bits)
  0-63: Chain ID to load from Sophia["chains"]
```

### Chain Execution Flow
```
Packet arrives with circuit_state.CHAIN_INDEX = 0
  ↓
Load Sophia["chains"]["chain_0"]
  ↓
For each function in chain:
  Call bpf_tail_call(ctx, prog_array_map, function_index)
  Record to Anamnesis: [function_id, decision, latency_ns]
  ↓
  If circuit_state.AUTH == 11 (EXPERIMENTAL): stop executing
  ↓
Apply final action from last function
Return XDP_PASS/DROP/REDIRECT
```

### Anamnesis NFV Event Type
```
#define ANAMNESIS_EVENT_NFV_FUNCTION_EXEC 0x09

struct nfv_event {
  uint8_t   event_type;         // 0x09
  uint8_t   chain_id;           // 0-63
  uint8_t   function_index;     // 0-15
  uint8_t   decision;           // 0=ALLOW, 1=DROP, 2=REDIRECT, 3=RATE_LIMITED
  uint32_t  latency_ns;         // execution time
  uint32_t  packets_processed;  // counter
  uint64_t  timestamp_ns;       // wall clock
};
```

### Sample Chain Definition (JSON)
```json
{
  "chain_id": 0,
  "name": "ingress_default",
  "functions": [
    {
      "id": 0,
      "prog": "firewall_deny",
      "timeout_us": 100,
      "required": true,
      "description": "Deny-list enforcement"
    },
    {
      "id": 1,
      "prog": "nat_xlate",
      "timeout_us": 150,
      "required": true,
      "description": "IPv6 address rewrite"
    },
    {
      "id": 2,
      "prog": "rate_limiter",
      "timeout_us": 50,
      "required": false,
      "description": "Token bucket rate limiting"
    },
    {
      "id": 3,
      "prog": "lb_route",
      "timeout_us": 200,
      "required": true,
      "description": "Backend selection and redirect"
    }
  ],
  "next_on_drop": "chain_drop_handler",
  "telemetry_enabled": true,
  "sla_latency_us": 500
}
```

---

## APPENDIX B: KEY METRICS

**Baseline Performance** (without NFV):
- Throughput: 920K pps (AF_XDP proven)
- Latency: <10μs per packet (kernel→userspace)

**Target Performance** (with NFV, 4-function chain):
- Throughput: ≥500K pps (54% utilization)
- Per-function latency: 50-200μs (median ~100μs)
- Total chain latency: <400μs (p99)
- Anamnesis event record rate: 500K events/s (4 events per packet)

**Scalability Limits**:
- Max functions per chain: 10 (BPF tail-call depth limit)
- Max active chains: 64 (circuit_state field: 6-bit CHAIN_INDEX)
- Max function registrations: 256 (prog_array_map size)

---

## APPENDIX C: REFERENCE DOCUMENTS

| Document | Location | Purpose |
|----------|----------|---------|
| Monad Wire Format Spec | `docs/protocol/MONAD-FOUNDATION.md` | circuit_state field semantics |
| Sophia Service Spec | `services/sophia/README.md` | Dict update atomicity guarantees |
| Anamnesis Event Types | `docs/protocol/ANAMNESIS-EVENTS.md` | Event ring buffer schema |
| Shield XDP Pipeline | `ebpf/shield_ingress.rs` | Integration point for NFV loader |
| BPF Tail Calls | `docs/BPF-TAIL-CALLS.md` | Kernel limitations + best practices |

---

*Monad CPU Application #10 Battle Plan — Forged 2026-03-04*
*7 Phases. 4-5 weeks. Reimagining network services as pure packet instructions.*
*Zero VMs. Zero containers. Zero context switches. Microseconds, not milliseconds.*
*The NFV revolution, distilled into eBPF.*

