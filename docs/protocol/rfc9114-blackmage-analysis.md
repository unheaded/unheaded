# RFC 9114 (HTTP/3) Security Analysis for Unheaded Protocol
## Black Mage Adversarial Review

**Document:** RFC 9114 - Hypertext Transfer Protocol Version 3 (HTTP/3)
**Analysis Date:** 2026-02-20
**Scope:** Sections 1-7 (Connection Setup, HTTP Framing Layer, Stream Types, Frame Layout, Request/Response Lifecycle, Connection Management)
**Analyst:** Black Mage Security Reviewer

---

## EXECUTIVE SUMMARY

HTTP/3 presents **8 major architectural patterns** highly relevant to Unheaded's hop-by-hop protocol design. The most critical findings concern:

1. **Capability Negotiation via SETTINGS** - Unheaded lacks any per-connection negotiation mechanism
2. **Stream-Level vs Connection-Level Error Distinction** - Unheaded collapses these to per-hop anomaly events
3. **Graceful Flow Termination via GOAWAY** - Unheaded currently has no mechanism to signal clean shutdown
4. **Typed Flow Classification** - Unheaded treats all flows uniformly; HTTP/3 uses stream types for explicit semantics
5. **Request Cancellation Without Connection Collapse** - Unheaded cannot cancel individual flows without affecting peers
6. **Proactive Resource Distribution** - Parallels Sophia/Wotan prefetch but lacks explicit signaling
7. **Intermediary-Aware Protocol Design** - Every Unheaded hop is an intermediary; HTTP/3 has explicit expectations
8. **Frame Format Extensibility** - Unheaded's fixed 20-byte register format vs HTTP/3's variable-length integers

---

## FINDING 1: HTTP/3 Frame Format vs Monad Register Format

### HTTP/3 Architecture (RFC 9114, Sections 7.1, 7.2)

**Frame Layout (Section 7.1):**
```
HTTP/3 Frame Format {
  Type (i),              // variable-length integer
  Length (i),            // variable-length integer
  Frame Payload (..)     // variable-length
}
```

**Key Properties:**
- **Type field:** Variable-length integer encoding (QUIC definition), enables 62-bit type space
- **Length field:** Explicitly states frame payload length, self-consistent verification required
- **Extensibility:** New frame types can be registered without disrupting existing implementations
- **Forward compatibility:** Unknown frame types are simply ignored per Section 9

### Monad Register Format (Unheaded)

**Current Design:**
```
Monad Register: 20 bytes
  version:  1 byte
  flags:    1 byte
  dict_id:  2 bytes
  field_id: 2 bytes
  value:    12 bytes
  crc16:    2 bytes
```

### Applicability: **HIGH**

**Black Mage Finding F1:**

The 20-byte fixed register format is a **single point of extension failure**. HTTP/3's variable-length integer encoding provides:
1. Backward compatibility via unknown-type ignoring
2. Granular extensibility per frame
3. IANA-registry-based governance

**Reinforces Finding:** M2 (Monad lacks extensibility framework)

---

## FINDING 2: HTTP/3 Stream Types vs Uniform Unheaded Flows

### HTTP/3 Stream Type Classification (RFC 9114, Sections 6.1, 6.2)

**Typed Streams:**
1. **Control Streams (0x00):** Carry connection-level frames
2. **Push Streams (0x01):** Carry server-initiated responses
3. **Unknown Types:** Safely ignored without connection failure

**Stream Type Semantics:**
- Clear separation of concerns: control vs data vs pushed resources
- Unknown types are safely ignored
- Allows independent flow control limits per type

### Unheaded Current Design

**Monadic Flow Model:**
- All flows are uniform; no stream type classifier
- No way to distinguish control flows vs data flows vs management flows
- All flows routed to same Wotan topics regardless of purpose

### Applicability: **HIGH**

**Black Mage Finding F2:**

Unheaded's uniform flow model is a **semantic loss**:
- Control flows can starve data flows in shared Wotan WAL
- Malformed flow affects all subsequent flows
- DDoS: no per-flow type quotas to limit damage

**Reinforces Finding:** M7 (Monad monolithic state machine)

**Recommendation:** Introduce stream types with per-type quotas via SETTINGS (Finding F3).

---

## FINDING 3: HTTP/3 SETTINGS Frame & Capability Negotiation

### HTTP/3 SETTINGS Mechanism (RFC 9114, Sections 7.2.4, 7.2.4.2)

**SETTINGS Frame Structure:**
- First frame on control stream (mandatory)
- Connection-scoped (applies to entire connection)
- Asymmetric: each peer advertises different limits
- Unknown identifiers silently ignored (forward compatible)
- Enables 0-RTT with remembered settings compatibility checking

**Defined Settings:**
- `SETTINGS_MAX_FIELD_SECTION_SIZE` (0x06): advertises maximum header size
- Reserved identifiers for forward compatibility testing

### Unheaded Current Design

**Zero Capability Negotiation:**
- No per-connection handshake
- Shield has hardcoded limits (compile-time constants)
- No way for peer hops to communicate constraints
- No version negotiation for Sophia dictionary encodings

### Applicability: **HIGH**

**Black Mage Finding F3:**

The absence of capability negotiation is a **critical deployment gap**:
- New hop joins path with different limits → silent misconfiguration
- Ring buffer exhaustion → no backpressure signal
- Dictionary version mismatch → corrupted value decoding
- Cannot add new encodings without updating all hops

**Reinforces Finding:** S3 (Sophia lacks version negotiation)

**Recommendation:** Implement Unheaded SETTINGS handshake with:
```
  SETTINGS_MAX_DICT_ID
  SETTINGS_MAX_FIELD_ID
  SETTINGS_WOTAN_BUFFER_SIZE
  SETTINGS_SOPHIA_CACHE_SIZE
  SETTINGS_FLOW_TIMEOUT_MS
  SETTINGS_L1_CACHE_WINDOW
```

---

## FINDING 4: HTTP/3 GOAWAY & Graceful Connection Shutdown

### HTTP/3 GOAWAY Mechanism (RFC 9114, Sections 5.2, 7.2.6)

**GOAWAY Frame Structure:**
```
GOAWAY Frame {
  Type (i) = 0x07,
  Length (i),
  Stream ID/Push ID (i)
}
```

**Semantics:**
1. Either endpoint can send GOAWAY to stop accepting new requests
2. Carries range: requests on streams >= N are rejected
3. Requests on streams < N may or may not be processed
4. Multiple GOAWAYs allowed with decreasing IDs (progressive shutdown)
5. Enables client/server to agree on which requests were processed

### Unheaded Current Design

**Zero Shutdown Signaling:**
- No GOAWAY equivalent
- Orphaned flows loop indefinitely in Wotan WAL
- When path changes (hop crashes), in-flight flows hang
- No way to signal "stop accepting new flows, finish existing ones"

### Applicability: **HIGH**

**Black Mage Finding F4:**

The lack of graceful shutdown is **operational reliability gap**:
- Flow completion guarantee absent (sender doesn't know what won't be processed)
- Exactly-once semantics broken (can't distinguish processed vs unprocessed)
- Orphan detection timeout = minutes (poor UX)
- Duplicate processing on restart if flow state unckeckpointed

**Reinforces Finding:** Q8 (Unheaded stateless reset; no shutdown signaling)

**Recommendation:** Implement Unheaded GOAWAY:
```c
Unheaded_GOAWAY {
  header: Monad (version, flags, stream_type=0x00)
  last_flow_id: 4 bytes
}
```
- Control stream tracks monotonically increasing flow IDs
- On shutdown: send GOAWAY(current_flow_id)
- Receiver rejects flows >= that ID

---

## FINDING 5: HTTP/3 Connection Errors vs Stream Errors

### HTTP/3 Error Hierarchy (RFC 9114, Section 8)

**Stream-Level Errors:**
- `H3_REQUEST_CANCELLED`: Flow cancelled by either endpoint
- `H3_REQUEST_REJECTED`: Server rejected without processing
- Reset specific stream only (other streams unaffected)

**Connection-Level Errors:**
- `H3_FRAME_UNEXPECTED`: Frame not allowed
- `H3_SETTINGS_ERROR`: Malformed SETTINGS payload
- Terminate entire connection; all streams reset

**Policy:** Endpoints MAY escalate stream errors to connection errors "under certain circumstances" but should consider impact on outstanding requests.

### Unheaded Current Design

**Undifferentiated Error Model:**
- All validation failures → per-hop anomaly events in Anamnesis ring buffer
- No stream-level vs connection-level distinction
- BPF map lookup failure? All flows on hop affected
- CRC check failure? Ring buffer integrity suspect

### Applicability: **MEDIUM**

**Black Mage Finding F5:**

Unheaded's monolithic error model is **visibility and recovery gap**:
- Cannot distinguish recoverable errors (flow-local) from fatal errors (hop-wide)
- No backpressure to peer hops
- Cascading data corruption possible if misaligned value decoded

**Reinforces Finding:** M5 (Monad lacks error classification), M10 (BPF map race conditions)

**Recommendation:** Stratify error codes:
```c
UNHEADED_ERR_FLOW     0x00  // Stream-level (recovered)
UNHEADED_ERR_DOMAIN   0x01  // Hop-level (affects all flows from ingress)
UNHEADED_ERR_SYSTEM   0x02  // System-level (ring buffer, BPF runtime)
```

---

## FINDING 6: HTTP/3 Request Cancellation Without Connection Collapse

### HTTP/3 Request Cancellation (RFC 9114, Section 4.1.1)

**Cancellation Semantics:**
- Either endpoint can cancel request stream
- Reset specific stream only (other streams unaffected)
- Error codes: `H3_REQUEST_CANCELLED`, `H3_REQUEST_REJECTED`, `H3_REQUEST_INCOMPLETE`
- Idempotency rules: distinguish completed vs partial responses

### Unheaded Current Design

**Monolithic Flow Model:**
- All flows share Wotan ring buffer state
- No per-flow reset mechanism
- Cancelling flow F1 requires:
  - Removing F1 from ring buffer (compaction or marker)
  - OR waiting for natural timeout (minutes)
  - OR hop crash (affects all flows)

### Applicability: **MEDIUM-HIGH**

**Black Mage Finding F6:**

The inability to cancel individual flows is an **efficiency and fairness gap**:
- Hop wastes CPU cycles on cancelled flows
- L1 cache polluted with stale data
- Dictionary entries exhausted faster (no early cleanup)
- Can't guarantee exactly-once semantics

**Reinforces Finding:** M8 (Monad state machine doesn't support cancellation), M10 (BPF map race on concurrent access)

**Recommendation:** Implement request cancellation:
```c
Unheaded_CANCEL_FLOW {
  header: Monad (version, flags, stream_type=0x00)
  flow_id: 4 bytes
}
```
- Per-flow tracking in Sophia with refcount
- On CANCEL_FLOW: decrement dict refcount, evict if zero
- Remove from ring buffer processing queue

---

## FINDING 7: HTTP/3 Server Push & Proactive Resource Distribution

### HTTP/3 Server Push (RFC 9114, Section 4.6)

**Push Semantics:**
- Server sends `PUSH_PROMISE` before main response (announces intent)
- Client can reject via `CANCEL_PUSH`
- Resources budgeted via `MAX_PUSH_ID` frame
- Ordering guarantee: PUSH_PROMISE before main response

**Benefits:**
- Reduces client rerequests (spatial locality)
- Explicit intent declaration
- Client rejection control
- Resource budgeting

### Unheaded Design: Wotan L1 Prefetch & Sophia Pre-Distribution

**Wotan L1 Cache Spatial Prefetch:**
- Cache hints based on access patterns
- Preload neighboring cache lines

**Sophia Dictionary Pre-Distribution:**
- Could send dictionary entries to downstream hops before needed

### Applicability: **MEDIUM**

**Black Mage Finding F7:**

Unheaded's prefetch mechanisms are **tactically similar to HTTP/3 push** but lack:
1. Intent signal (PUSH_PROMISE equivalent)
2. Client rejection (CANCEL_PUSH equivalent)
3. Resource quota (MAX_PUSH_ID equivalent)
4. Ordering guarantees

**Impact:**
- Receiving hop doesn't know if prefetch was intentional
- Downstream hop can't reject dictionary pre-distribution
- No limit on prefetch hints → attacker can flood Wotan L1
- Race condition: main flow arrives after prefetch eviction

**Recommendation:** Make prefetch explicit:
```c
Unheaded_PREFETCH_HINT {
  header: Monad (version, flags, stream_type=0x00)
  prefetch_id: 4 bytes
  dict_ids: variable array
  expected_latency_ms: 2 bytes
}

Unheaded_CANCEL_PREFETCH {
  header: Monad (version, flags, stream_type=0x00)
  prefetch_id: 4 bytes
}
```

---

## FINDING 8: HTTP/3 Intermediaries & Hop-by-Hop Semantics

### HTTP/3 Intermediary Model (RFC 9114, Sections 4.1.2, 4.2, 4.4, 10.3)

**Intermediary Responsibilities:**
- MUST NOT forward malformed requests/responses
- Malformed detections = stream error (`H3_MESSAGE_ERROR`)
- Field transformation rules (remove connection-specific headers)
- Authority negotiation (CONNECT method)
- Security rules for cross-proxy attacks (CRLF injection protection)

**Malformed Detections (Section 4.1.2):**
- Prohibited fields/pseudo-header fields
- Mandatory pseudo-header fields absent
- Invalid values for pseudo-header fields
- Invalid field sequences

### Unheaded as Hop-Agnostic Network

**Reality: Every Unheaded Hop IS an Intermediary**

Each hop:
- Receives Monad registers (requests)
- Performs Sophia dictionary translation (transformation)
- Forwards to next hop (intermediation)
- Makes forwarding decisions (authority)

**Critical Gaps:**
1. No hop-by-hop vs end-to-end distinction
2. No tunnel mode (no transparent pass-through)
3. No malformed detection rules spec
4. No authority negotiation mechanism

### Applicability: **MEDIUM**

**Black Mage Finding F8:**

Unheaded's hop model is **implicitly intermediary-centric but lacks explicit rules**:
- No spec for what constitutes malformed Monad register
- Hop1 and Hop2 may disagree on malformation
- Routing loops possible
- Sensitive information (dict hints) leaks end-to-end

**Recommendation:** Define malformation rules and formalize intermediary validation:
```c
enum unheaded_malformed_reason {
  UNHEADED_INVALID_VERSION = 0x01,
  UNHEADED_INVALID_CRC = 0x02,
  UNHEADED_UNKNOWN_DICT_ID = 0x04,
  UNHEADED_INVALID_FIELD_ID = 0x05,
  UNHEADED_DICT_AUTH_VIOLATION = 0x07,
};

// Intermediary rule:
//   "Intermediaries that detect malformed Monad registers MUST NOT
//    forward them. Malformed registers detected MUST be treated as
//    stream error of type UNHEADED_MESSAGE_ERROR."
```

---

## FINDING 9: HTTP/3 Extensions Framework

### HTTP/3 Extensibility (RFC 9114, Section 9, Section 11.2)

**Extension Points:**
- New frame types (62-bit space via IANA registry)
- New settings (variable-length integer IDs)
- New error codes (variable-length integer codes)
- New stream types (variable-length integer type IDs)

**Backward Compatibility:**
- Unknown values ignored in all extensible elements
- Unknown stream types: discard data without error
- Unknown frame types at location-specific positions: error

**Negotiation:**
- SETTINGS mechanism: both peers set value → extension enabled
- Default: disabled if setting omitted (backward compatible)

### Unheaded Extensibility

**Current State: No Extensibility Framework**
- No registry for new Monad frame types
- No mechanism for new Sophia encoding variants
- No error code space allocated
- Protocol changes = version bump (version field saturating)

### Applicability: **MEDIUM-HIGH**

**Black Mage Finding F9:**

The absence of extensibility framework is a **future-proofing gap**:
- Safe unknown handling absent (new extension frames crash old hops)
- Negotiation mechanism absent (all hops must upgrade simultaneously)
- IANA governance absent (type space collisions possible)
- Backward compatibility broken (no graceful degradation)

**Reinforces Finding:** M2 (Monad lacks extensibility framework)

**Recommendation:** Design extension framework with IANA registries:
```c
enum unheaded_frame_type {
  UNHEADED_FRAME_SETTINGS = 0x00,
  UNHEADED_FRAME_GOAWAY = 0x01,
  UNHEADED_FRAME_CANCEL_FLOW = 0x02,
  UNHEADED_FRAME_PREFETCH_HINT = 0x03,
  // 0x04-0x7F reserved for future standard frames
  // 0x80-0xFF for extensions
};

enum unheaded_setting_id {
  SETTINGS_MAX_DICT_ID = 0x01,
  SETTINGS_MAX_FIELD_ID = 0x02,
  // 0x03-0x7F reserved
  // 0x80-0xFF for extensions
};
```

---

## SUMMARY TABLE: Findings & Priority

| Finding | Title | Applicability | Related Gaps | Priority |
|---------|-------|---------------|-------------|----------|
| F1 | Frame Format Extensibility | HIGH | M2 | P2 |
| F2 | Stream Types & Multiplexing | HIGH | M7 | P1 |
| F3 | Capability Negotiation (SETTINGS) | HIGH | S3 | P1 |
| F4 | Graceful Shutdown (GOAWAY) | HIGH | Q8 | P1 |
| F5 | Error Stratification | MEDIUM | M5, M10 | P2 |
| F6 | Request Cancellation | MEDIUM-HIGH | M8, M10 | P2 |
| F7 | Proactive Resource Distribution | MEDIUM | W1 | P3 |
| F8 | Intermediary Rules | MEDIUM | (implicit) | P2 |
| F9 | Extension Framework | MEDIUM-HIGH | M2 | P1 |

---

## FINAL ASSESSMENT

**RFC 9114 Applicability to Unheaded:** VERY HIGH

HTTP/3 provides **proven solutions** to Unheaded's current architectural gaps:
1. **F3, F4, F9** (P1) enable operational correctness
2. **F1, F2, F5, F6, F8** (P2) ensure robustness
3. **F7** (P3) optimizes performance

**Current Risk:** MEDIUM (tactical) → HIGH (strategic at scale)
- Works in PoC (Doom demo)
- Fails in production (multi-vendor, graceful shutdown, feature additions)

**Recommended Timeline:** 12-18 months phased implementation following HTTP/3 patterns.

