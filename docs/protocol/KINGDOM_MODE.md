# Kingdom Mode: Flow State Machine in the Network Header

## What It Is

Kingdom Mode is a four-state flow machine encoded in two bits (K1|K0) within the Monad register. Every packet processed by Unheaded carries Kingdom Mode state, enabling the network layer itself to be stateful and aware of flow lifecycle without requiring userspace connection tracking tables.

The innovation: flow state is not hidden in kernel connection tables. It is explicit in the packet header, auditable, and enforced in the kernel via BPF programs. The infrastructure always knows whether a flow is idle, active, closing, or closed.

## The Four States

Kingdom Mode uses 2 bits to encode exactly 4 distinct flow states:

### State 1: IDLE (K1=0, K0=0)

**Binary**: `00`

A flow has been established and authenticated but is not actively transferring data. The connection is open and ready, but neither endpoint is sending packets at the moment.

**Characteristics**:
- Sophia dictionary entry is active (the flow is recognized)
- Wotan ephemeral ring buffer exists and holds flow history
- Port Authority policies are enforced
- No data plane traffic occurs
- Timeout: 30 seconds of inactivity before automatic transition to CLOSING

**Application scenario**: A gRPC connection in between RPC calls, or a TCP connection waiting for the client to initiate a request.

**Transitions to**:
- ACTIVE (when data transmission begins)
- CLOSING (if timeout or explicit close request)

### State 2: ACTIVE (K1=0, K0=1)

**Binary**: `01`

The flow is actively transmitting or receiving data. Both endpoints are engaged in packet exchange. Anamnesis is recording events at full fidelity. Shield WAF rules are actively evaluating each packet.

**Characteristics**:
- Both endpoints are sending packets
- Anamnesis event rate is highest (hundreds of events per second per flow)
- Wotan tracking is continuous
- Load balancing and failover detection is active
- Timeout: 5 seconds without a packet triggers automatic transition to CLOSING (abnormal close)

**Application scenario**: An RPC in flight, streaming data transmission, or interactive command exchange.

**Transitions to**:
- IDLE (when one side stops transmitting)
- CLOSING (on FIN, RST, or timeout)

### State 3: CLOSING (K1=1, K0=0)

**Binary**: `10`

The flow is in the process of being torn down. One or both endpoints have initiated closure, but resources have not yet been reclaimed. The flow is in a grace period where packets may still arrive (e.g., retransmitted FINs, buffered data).

**Characteristics**:
- New data is not accepted (Shield WAF rejects new payloads)
- Existing in-flight packets may complete
- Sophia dictionary entry is marked for deletion
- Wotan ring buffer records final state transitions
- Timeout: 2 seconds (default) before automatic transition to CLOSED
- Grace period allows peer acknowledgment without hard socket closure

**Application scenario**: TCP FIN sequence, gRPC graceful shutdown, or explicit flow teardown initiated by Port Authority.

**Transitions to**:
- CLOSED (after timeout or all pending packets acknowledged)

### State 4: CLOSED (K1=1, K0=1)

**Binary**: `11`

The flow has been fully terminated and resources are being reclaimed. The Sophia dictionary entry is deleted. The Wotan ring buffer is deallocated. No further packets belong to this flow.

**Characteristics**:
- All Wotan resources released
- Sophia entry removed
- Anamnesis records final teardown event
- Any packets arriving with CLOSED state are logged as anomalies
- Timeout: 1 second before ring buffer memory is reclaimed

**Application scenario**: Post-teardown cleanup, final event recording, memory reclamation.

**Transitions from**: CLOSING only. Flows do not resurrect from CLOSED.

## State Machine Diagram

```
                    +--------+
                    |  IDLE  |
                    |        |
                    +---+----+
                        |
                        | (data starts)
                        v
                    +--------+
          +--------->| ACTIVE |
          |          |        |
          |          +---+----+
          |              |
          |              | (no data for 5s)
          |              | or (FIN/RST)
          |              v
          |          +--------+
          |          |CLOSING |
          |          |        |
          |          +---+----+
          |              |
          |              | (2s timeout)
          |              v
          |          +--------+
          |          | CLOSED |
          |          |        |
          |          +--------+
          |              |
          |              | (1s timeout)
          |              | (memory freed)
          |              v
          |          [End of flow]
          |
          |
          +---------+ (IDLE: no data for 30s)
                    +--------+
                    |CLOSING |
                    | state  |
                    +--------+
```

### Transitions Matrix

| From State | To State | Trigger | Allowed | Notes |
|------------|----------|---------|---------|-------|
| IDLE | ACTIVE | Data transmission starts | Yes | Normal flow activation |
| IDLE | CLOSING | 30s timeout or admin close | Yes | Garbage collection |
| ACTIVE | IDLE | No data for a brief period | Yes | Flow quiescence |
| ACTIVE | CLOSING | FIN, RST, or 5s timeout | Yes | Abnormal or graceful close |
| CLOSING | CLOSED | 2s grace period expires | Yes | Final resource cleanup |
| CLOSED | * | Any | No | Dead flow; no resurrection |

## How Wotan Memory Interacts with Kingdom Mode

Wotan maintains an ephemeral per-flow ring buffer. The ring buffer's lifecycle is tied to Kingdom Mode state:

### Ring Buffer Lifecycle

**IDLE state**:
- Ring buffer exists and is retained
- Memory footprint is minimal (128 bytes for state flags + timeout counters)
- No active event recording unless policy requires it

**ACTIVE state**:
- Ring buffer expands to hold flow history (up to 1 KB per flow)
- Last 32 state transitions are recorded
- Resource allocation metadata is updated continuously
- Packet timestamps and sequence numbers are tracked

**CLOSING state**:
- Ring buffer remains resident during grace period (2 seconds)
- No new history is recorded
- Final events are flushed to Anamnesis
- Memory is marked for deallocation

**CLOSED state**:
- Ring buffer deallocated within 1 second
- Memory returned to kernel pool
- Flow identifier is released for reuse
- Prevents memory leaks in long-running BPF programs

### Memory Constraints

Wotan enforces memory constraints per-host:
- Maximum 1 MB per active host for all IDLE ring buffers
- Maximum 10 MB per active host for all ACTIVE ring buffers
- When limits are exceeded, oldest IDLE flows are force-transitioned to CLOSING
- ACTIVE flows cannot be force-closed; instead, alerts are generated

This prevents a single compromised application from exhausting host memory via connection exhaustion attacks.

## How Sophia Dictionaries Consult Kingdom Mode

Sophia dictionary lookups are state-aware. When a packet arrives, Wotan queries Sophia with the flow label as the key, but the returned dictionary entry depends on the current Kingdom Mode state.

### Dictionary Entry Versioning

Each logical flow has up to 4 Sophia entries, one per Kingdom Mode state:

```go
// In services/sophia/dictionary.go
type SophiaEntry struct {
    FlowLabel         [16]byte
    KingdomModeState  uint8  // 0=IDLE, 1=ACTIVE, 2=CLOSING, 3=CLOSED
    Policy            PolicyRules
    CapabilityFlags   uint32
    TimeoutSeconds    uint32
    WafRules          []WafRule
    RatelimitMbps     uint32
}

// Lookup in Sophia returns different entries based on state
entry, err := sophia.GetByFlowAndState(flowLabel, kingdomMode)
```

### Policy Variation by State

Different policies apply depending on Kingdom Mode state:

```yaml
flow_label: 0x20010db812345678...
policies:
  idle:
    rate_limit_mbps: 10          # Low rate for idle flows
    timeout_seconds: 30
    log_events: [state_change]   # Minimal logging

  active:
    rate_limit_mbps: 1000        # Full rate during active
    timeout_seconds: 5
    log_events: [packet, state_change, anomaly]  # Full logging

  closing:
    rate_limit_mbps: 100         # Restricted during close
    timeout_seconds: 2
    log_events: [packet, state_change]
    reject_new_data: true        # No new streams

  closed:
    timeout_seconds: 1
    log_events: [final_event]
```

### Load Balancing and Failover

When ACTIVE, Sophia entries can include multiple valid endpoints. On packet loss or timeout, Wotan can select a different endpoint from the Sophia entry without returning to IDLE:

```
ACTIVE (endpoint1)
    |
    | (loss detected)
    v
ACTIVE (endpoint1) → consult Sophia
    |
    | (Sophia returns endpoints: [endpoint1, endpoint2, endpoint3])
    v
ACTIVE (endpoint2, seamless failover)
```

## How Anamnesis Records State Transitions

Every Kingdom Mode state transition generates an Anamnesis event. These events are recorded to the ring buffer and forwarded to the observability backend.

### State Transition Events

```go
type KingdomModeTransitionEvent struct {
    Timestamp           uint64       // nanoseconds since epoch
    FlowLabel           [16]byte
    FromState           uint8        // Previous K1|K0
    ToState             uint8        // New K1|K0
    Trigger             string       // "data_start", "timeout", "fin", "closing_complete"
    SophiaEntryVersion  uint32       // Which dictionary version was active
    WotanRingbufAddr    uint64       // Ring buffer memory address
    CausePacketSeq     uint32        // Sequence number of triggering packet
}
```

Example event stream for a complete flow lifecycle:

```
Time=0us:   IDLE → ACTIVE (trigger: data_start, seq=1)
Time=100us: ACTIVE (packet received, seq=2)
Time=150us: ACTIVE (packet received, seq=3)
Time=5.0s:  ACTIVE → CLOSING (trigger: timeout, seq=4)
Time=5.1s:  CLOSING (final packet received, seq=5)
Time=7.1s:  CLOSING → CLOSED (trigger: closing_complete)
Time=8.1s:  [ring buffer deallocated]
```

These events are queryable by observability backends for:
- Flow duration analysis
- Timeout troubleshooting
- State machine auditing
- Performance profiling

## Security Implications: Shield WAF Integration

Shield is Unheaded's Web Application Firewall. It uses Kingdom Mode state to enforce different security policies:

### CLOSING/CLOSED State Restrictions

When a flow enters CLOSING state, Shield applies stricter rules:

```yaml
waf_rule: reject_new_streams_in_closing
condition:
  kingdom_mode_state: CLOSING
  http_method: ["POST", "PUT", "DELETE"]
action:
  reject: true
  log: true
  metric: "shield.closing_state.new_stream_rejected"
```

Rationale: During graceful shutdown, clients should not initiate new operations. New requests during CLOSING are treated as anomalous.

### IDLE State Timeout Enforcement

Shield enforces aggressive timeouts for IDLE flows to prevent connection exhaustion:

```yaml
waf_rule: close_idle_connections
condition:
  kingdom_mode_state: IDLE
  duration_without_packet_ms: 30000
action:
  close_flow: true
  log_level: info
```

This prevents attackers from holding thousands of idle connections open.

## Wire Format: Monad Register Encoding

The Kingdom Mode bits appear in the Monad register at a fixed location:

### Monad Register Byte Layout

```
Offset   Field                          Bits
------   -----                          ----
0x10     Kingdom Mode (K1|K0)           2 bits (bits 6-7 of byte 0x10)
0x11     Inverse Mask Indicator         1 bit  (bit 5 of byte 0x11)
0x12     Sequence Number                14 bits
0x14     Reserved                       15 bits
0x16     CRC-16/CCITT-FALSE             16 bits
```

### Bit Positions Within Byte

```
Byte 0x10:
  Bit 7: K1 (most significant)
  Bit 6: K0 (less significant)
  Bits 5-0: Unused (reserved for future use)

Example:
  IDLE state:     0b00xxxxxx (K1=0, K0=0)
  ACTIVE state:   0b01xxxxxx (K1=0, K0=1)
  CLOSING state:  0b10xxxxxx (K1=1, K0=0)
  CLOSED state:   0b11xxxxxx (K1=1, K0=1)
```

### Wire Example

A packet in ACTIVE state with inverse mask enabled:

```
Hex:     0x20 0x01 0x0d 0xb8 0x12 0x34 0x56 0x78
         ...
Hex:     0x9a 0xbc 0xde 0xf0 0x11 0x22 0x33 0x44
         [Flow Label = 128 bits, displayed in two rows]

Hex:     0x56 0x78 ...
         ^^
         Byte 0x10: Kingdom Mode bits
         Binary: 0101xxxx = ACTIVE (01), rest reserved

Hex:     ... 0x78 0x42 ...
         Binary (0x78): 0111 1000
         Bit 5: 1 = Inverse Mask enabled
```

## Implementation: Go Encoding/Decoding Pattern

Go code from `pkg/protocol/monad.go` demonstrates the standard encoding/decoding pattern:

```go
package monad

import (
	"encoding/binary"
	"fmt"
)

// KingdomModeState represents the 2-bit K1|K0 state
type KingdomModeState uint8

const (
	KingdomModeIDLE    KingdomModeState = 0b00
	KingdomModeACTIVE  KingdomModeState = 0b01
	KingdomModeCLOSING KingdomModeState = 0b10
	KingdomModeCLOSED  KingdomModeState = 0b11
)

// String returns the human-readable state name
func (k KingdomModeState) String() string {
	switch k {
	case KingdomModeIDLE:
		return "IDLE"
	case KingdomModeACTIVE:
		return "ACTIVE"
	case KingdomModeCLOSING:
		return "CLOSING"
	case KingdomModeCLOSED:
		return "CLOSED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", k)
	}
}

// MonadRegister is the 20-byte header
type MonadRegister struct {
	FlowLabel     [16]byte
	StateAndFlags byte      // Bits 7-6: K1|K0, Bit 5: InverseMask
	Sequence      [2]byte
	Reserved      [2]byte
	CRC16         [2]byte
}

// SetKingdomMode sets the K1|K0 bits
func (m *MonadRegister) SetKingdomMode(state KingdomModeState) {
	m.StateAndFlags = (m.StateAndFlags & 0x3f) | ((byte(state) & 0x03) << 6)
}

// KingdomMode retrieves the K1|K0 bits
func (m *MonadRegister) KingdomMode() KingdomModeState {
	return KingdomModeState((m.StateAndFlags >> 6) & 0x03)
}

// ValidTransition checks if a state transition is allowed
func ValidTransition(from, to KingdomModeState) bool {
	switch from {
	case KingdomModeIDLE:
		return to == KingdomModeACTIVE || to == KingdomModeCLOSING
	case KingdomModeACTIVE:
		return to == KingdomModeIDLE || to == KingdomModeCLOSING
	case KingdomModeCLOSING:
		return to == KingdomModeCLOSED
	case KingdomModeCLOSED:
		return false  // No transition from CLOSED
	default:
		return false
	}
}

// Example: Encoding a packet
func encodeMonadPacket(flowLabel [16]byte, state KingdomModeState) *MonadRegister {
	m := &MonadRegister{FlowLabel: flowLabel}
	m.SetKingdomMode(state)
	// CRC-16 would be computed here over bytes 0x00-0x11
	return m
}

// Example: Decoding a packet
func decodeMonadPacket(buf []byte) (*MonadRegister, error) {
	if len(buf) < 20 {
		return nil, fmt.Errorf("insufficient data for Monad register")
	}
	m := &MonadRegister{
		FlowLabel:     [16]byte(buf[0:16]),
		StateAndFlags: buf[16],
		Sequence:      [2]byte{buf[17], buf[18]},
		CRC16:         [2]byte{buf[19], buf[20]},
	}
	return m, nil
}
```

## Why This Is Innovative

### 1. Stateful Networking Without Connection Tables

Traditional networking requires userspace connection tables (TCP/IP stack, netfilter connections, etc.). Kingdom Mode eliminates this requirement by encoding state in the packet header itself.

**Benefit**: No connection table exhaustion attacks, no GC pauses from connection cleanup, no per-connection memory overhead.

### 2. Kernel-Space Policy Enforcement

Because state is in the packet, BPF programs can enforce state-dependent policies without syscalls or userspace interaction.

**Benefit**: Sub-microsecond policy enforcement, no context switches, auditable in the network header.

### 3. Distributed State Machine

In a multi-datacenter deployment, every endpoint knows the flow state by reading the packet. No global state server required.

**Benefit**: No distributed consensus overhead, no eventual consistency issues, strong consistency via the packet itself.

### 4. Automatic Audit Trail

Every state transition is an Anamnesis event. The complete flow lifecycle is logged by default.

**Benefit**: Full operational observability, compliance audit readiness, post-incident analysis capability.

## Testing: State Machine Correctness

Kingdom Mode correctness is validated via:

### Unit Tests (pkg/protocol/monad_test.go)
- All valid and invalid transitions
- Bit manipulation correctness
- Timeout boundary conditions

### Integration Tests (services/wotan/tests/)
- State machine under load (1000s flows/sec)
- Sophia dictionary consultation on state change
- Anamnesis event generation

### Property Tests
- For all transitions, `ValidTransition(from, to)` is consistent
- Kingdom Mode bits round-trip without corruption
- CRC-16 validation after state changes

## Summary

Kingdom Mode is a four-state flow machine that makes network flows stateful at the protocol layer. By encoding state in the packet header (just 2 bits in the Monad register), Unheaded achieves:

- Stateful networking without userspace connection tables
- Kernel-space policy enforcement
- Distributed state awareness
- Full audit trail of every state transition
- Integration with Wotan ephemeral memory and Sophia dictionaries

This is how infrastructure becomes aware without becoming complicated.
