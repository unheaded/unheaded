# Protocol Primer

*High School Level*

---

## The Five Components

The Unheaded Protocol Foundation consists of five named components that work
together as a distributed system. Each has a specific role, a specific
implementation, and a specific place in the packet lifecycle.

```
                PACKET LIFECYCLE

    External    +-------+   Internal   +-------+   External
    Network --->| SHIELD|-->  hops  -->| SHIELD|---> Network
     (Shadow)   | (XDP) |   +-----+   |  (TC) |    (Shadow)
                +---+---+   | HOP |   +---+---+
                    |        +--+--+       |
                    |           |           |
              BIRTH |     HOP  |     DEATH |
              event |    event |     event |
                    v           v           v
                +------------------------------+
                |          ANAMNESIS            |
                |     (Ring Buffer Events)      |
                +------+-----------------------+
                       |
                       v
                +------+-----------+     +---------+
                | trace-collector  |---->|  WOTAN  |
                | (Rust userspace) |     | (gRPC)  |
                +------------------+     +----+----+
                                              |
                                    +---------+---------+
                                    |                   |
                              +-----+-----+    +-------+------+
                              | dashboard  |    | other subs   |
                              +------------+    +--------------+

  +----------+
  |  SOPHIA  |  (Loaded into BPF maps, consulted at each hop)
  +----------+
```

Let us examine each component.

---

## 1. Monad: The Register File

The Monad is the core data structure of the protocol. It is a 20-byte
register file that travels with every packet inside the Unheaded-managed
network ("the Kingdom").

### What It Is

A fixed-layout block of bytes that eBPF programs read and write at each hop.
Think of it as a tiny, standardized form that gets stamped and updated as the
packet passes through each checkpoint.

### Wire Format

```
Byte Offset  Field            Size  Encoding       Purpose
-----------  ---------------  ----  -----------    ---------------------------
0x00         version          1B    raw u8         Protocol version (0x01)
0x01         src_service_id   1B    exponent       Who sent this packet
0x02         dst_service_id   1B    exponent       Intended destination service
0x03         hop_count        1B    raw u8         Hops traversed (incremented per-hop)
0x04         qos_class        1B    exponent       Quality of service tier
0x05         flow_action      1B    exponent       Per-hop instruction
0x06         circuit_state    1B    exponent       Circuit breaker state
0x07         flags            1B    bitfield       8 boolean flags
0x08-0x09    latency_hint     2B    raw u16 (BE)   Expected delivery time
0x0A         deploy_ring      1B    exponent       Deployment environment
0x0B         mesh_flags       1B    exponent       Service mesh routing
0x0C         src_prefix_lo    1B    raw u8         Low byte of source subnet
0x0D         dst_prefix_lo    1B    raw u8         Low byte of destination subnet
0x0E-0x11    scratch[0..3]    4B    exponent       General-purpose registers
0x12-0x13    checksum         2B    CRC-16/CCITT   Integrity verification
```

### Exponent Encoding

Most Monad fields use **exponent encoding**: each byte value represents a
power of 2 or a named constant from a lookup table. This means a single byte
can encode 256 distinct states without complex parsing.

For example, the `flow_action` field:

| Byte Value | Meaning | Description |
|-----------|---------|-------------|
| 0x00 | FORWARD | Default: pass packet to next hop |
| 0x01 | SAMPLE | Select for statistical sampling |
| 0x02 | TRACE | Force full event emission at every downstream hop |
| 0x03 | MIRROR | Create a copy for analysis |
| 0x04 | DROP | Drop this packet (after state write-back) |

The `circuit_state` field:

| Byte Value | Meaning | Description |
|-----------|---------|-------------|
| 0x00 | CLOSED | Circuit healthy, traffic flows normally |
| 0x01 | HALF_OPEN | Probing: limited traffic to test recovery |
| 0x02 | OPEN | Circuit tripped: service is unhealthy |

### The Flags Bitfield (offset 0x07)

Eight independent boolean flags packed into one byte:

```
Bit 7: CHAOS    Packet under chaos injection (Yaldabaoth)
Bit 6: CANARY   Canary deployment path
Bit 5: TRACED   Force full Anamnesis event at every hop
Bit 4: ENCRYPT  Intra-Kingdom TLS payload
Bit 3: SAMPLED  Selected for statistical sampling
Bit 2: MIRROR   This is a mirror copy
Bit 1: CUSTOM   Scratch/Checksum carry exponent values, not defaults
Bit 0: RSVD     Reserved, must be zero
```

### Checksum

The checksum field (bytes 0x12-0x13) contains a CRC-16/CCITT computed over
bytes 0x00 through 0x11 (18 bytes). Every hop verifies the checksum before
processing and recomputes it after mutation.

If the checksum does not match:
- The hop emits an ANOMALY event to Anamnesis
- The packet is passed through (fail-open) -- corrupted metadata should not
  cause packet drops
- The anomaly is visible on the dashboard

If the CUSTOM flag (bit 1) is set, the checksum field carries exponent-encoded
values instead of CRC. In this mode, checksum verification is skipped.

---

## 2. Sophia: The Dictionary

Sophia is the knowledge layer. It maps the exponent-encoded values in the
Monad to human-readable metadata and operational parameters.

### What It Is

A BPF HashMap (`SOPHIA`) populated by userspace and consulted by the hop-ebpf
program at every packet. Key encoding packs dictionary ID and field value into
a single u16:

```
Key = (dict_id << 8) | field_value

Example:
  dict_id = 0x01 (QoS dictionary)
  field_value = 0x03 (Gold tier)
  Key = 0x0103
```

### Dictionary Categories

| Dict ID | Name | Maps From | Maps To |
|---------|------|-----------|---------|
| 0x01 | QoS | qos_class values | Scheduling weight, DSCP mark |
| 0x02 | Circuit | circuit_state values | Human-readable state metadata |
| 0x03 | Service | service_id values | Service name, tier, capabilities |

### How It Works

1. **Userspace** populates the SOPHIA BPF map with known values at startup
   (and updates it dynamically as services register/deregister).

2. **hop-ebpf** reads the Monad fields (qos_class, circuit_state,
   src_service_id, dst_service_id) and looks them up in SOPHIA.

3. If a lookup succeeds, the hop increments `STAT_SOPHIA_HITS` and can use
   the returned metadata for routing decisions.

4. If a lookup fails (unknown value), the hop passes the packet through.
   Unknown values are not errors -- they indicate a new service or
   configuration that has not been registered yet.

### Why a BPF Map?

The alternative would be to hard-code value meanings into the eBPF program
itself. But eBPF programs run in the kernel and cannot be updated without
reload. By putting the dictionary in a BPF map, userspace can add, remove,
or update entries without reloading the eBPF program. The kernel program
stays generic; all domain knowledge lives in the map.

---

## 3. Wotan: Memory Persistence

Wotan is the message bus and the memory layer of the system. It provides
pub/sub messaging, event persistence, and state distribution.

### What It Is

A Go service running on port 18001 (gRPC) and 18000 (HTTP). All services
communicate through Wotan rather than directly with each other.

### Architecture

```
+-----------------------------------------------------------+
|                        WOTAN                               |
|                                                           |
|  +----------+  +-----------+  +----------+  +----------+ |
|  | Topic:   |  | Topic:    |  | Topic:   |  | Topic:   | |
|  | network  |  | timeline  |  | alerts   |  | logs.*   | |
|  | .traces  |  | .updates  |  | .critical|  |          | |
|  +----+-----+  +-----+-----+  +----+-----+  +----+-----+ |
|       |               |             |             |        |
|  Subscribers:    Subscribers:  Subscribers:  Subscribers:  |
|  - dashboard     - dashboard   - ALL         - dashboard   |
|                  - kanban                                   |
+-----------------------------------------------------------+
```

### Capabilities

1. **Pub/Sub Topics**: Services publish messages to named topics. Other
   services subscribe and receive messages asynchronously. Publishers do not
   know or care who is listening.

2. **Ring Buffer**: Wotan maintains an internal ring buffer for each topic.
   Recent messages are kept in memory for late-joining subscribers.

3. **gRPC Streaming**: Subscribers receive messages via gRPC server-side
   streaming. This provides low-latency push delivery without polling.

4. **Protocol RAM**: The Sophia dictionary values and runtime configuration
   are distributed through Wotan. When a service registers, its metadata
   propagates to all Sophia maps via Wotan topic `system.discovery`.

### Topic Hierarchy

| Topic Pattern | Publisher | Subscribers | Content |
|---------------|-----------|-------------|---------|
| `network.traces` | trace-collector | dashboard-backend | Anamnesis events |
| `timeline.updates` | timeguru | dashboard, kanban | Roadmap changes |
| `strategy.decisions` | captain | all services | Strategic guidance |
| `tasks.assignments` | micromanager | all services | Task tracking |
| `design.proposals` | architect | all services | Architecture decisions |
| `alerts.critical` | any service | all services | Emergency notifications |
| `logs.<svc>.<level>` | all services | dashboard-backend | Structured log entries |
| `system.discovery` | all services | all services | Service registration |
| `system.outage.reports` | all services | all services | Health failure reports |

---

## 4. Anamnesis: Event Streaming

Anamnesis (Greek: "recollection") is the event streaming subsystem. It
captures the complete lifecycle of every packet that passes through the
Kingdom.

### What It Is

A per-CPU BPF ring buffer (8 MiB) shared between eBPF programs and the
trace-collector userspace reader. Events are 32 bytes each (one CPU cache
line for optimal performance).

### Event Types

| Type | Value | Emitter | Meaning |
|------|-------|---------|---------|
| Birth | 0x01 | Shield XDP | Packet entered Kingdom, Monad inserted |
| Hop | 0x02 | hop-ebpf | Packet processed at interior node |
| Death | 0x03 | Shield TC | Packet leaving Kingdom, Monad stripped |
| Anomaly | 0x04 | Any program | CRC failure, unknown key, decode error |
| Chaos | 0x05 | Yaldabaoth | Deliberate corruption for testing |

### Event Format

```
AnamnesisEvent (32 bytes):

Offset  Field          Size  Description
------  -----------    ----  -----------
0x00    timestamp_ns   8B    Kernel monotonic clock (nanoseconds)
0x08    event_type     1B    Birth/Hop/Death/Anomaly/Chaos
0x09    hop_id         1B    Which node emitted this event
0x0A    flow_label_lo  2B    Low 16 bits of IPv6 Flow Label
0x0C    monad          20B   Full Monad register snapshot
```

### Event Flow

1. eBPF program reserves a 32-byte slot in the ANAMNESIS ring buffer.
2. Program writes the event using `core::ptr::write` (no copy, direct write).
3. Program calls `submit()` to make the event visible to userspace.
4. If the ring buffer is full, the event is dropped and
   `STAT_EVENTS_DROPPED` is incremented.
5. trace-collector reads submitted events asynchronously.
6. Events are correlated by Flow Label and published to Wotan.

### Performance

At 32 bytes per event and 8 MiB ring buffer capacity, the buffer holds
262,144 events before wraparound. At a sustained rate of 100,000 events/sec,
the buffer provides 2.6 seconds of slack before drops.

The ring buffer is per-CPU, so on a 4-core system the total capacity is
1,048,576 events.

---

## 5. Shield: XDP Filtering

Shield is the boundary enforcement layer. It runs at the Kingdom's edge --
the point where the internal network ("Kingdom") meets the external network
("Shadow").

### What It Is

Two eBPF programs in one binary:

1. **shield_xdp** (XDP ingress): Runs at the earliest possible point in the
   network stack, before the kernel allocates an sk_buff for the packet.

2. **shield_tc** (TC egress): Runs after the kernel has processed the
   outbound packet but before it hits the wire.

### Security Properties

These are not aspirational goals. They are enforced by the programs at every
packet:

1. **No external extension header enters the Kingdom.** Shield strips all
   IPv6 extension headers from inbound packets before inserting the Monad.
   This prevents header injection attacks.

2. **No Monad exits the Kingdom.** Shield TC strips the HBH header from
   every outbound packet. External observers cannot see Unheaded's internal
   metadata.

3. **Source blocklisting at wire speed.** The BLOCKLIST BPF map is checked
   at XDP before sk_buff allocation. Blocked sources receive XDP_DROP with
   zero memory allocation, making Shield resistant to DDoS flooding.

4. **Rate limiting.** The RATE_TOKENS map provides per-source token bucket
   rate limiting, also at XDP speed.

5. **Anti-double-stamp.** If a packet already carries a Monad HBH header
   (which would indicate either a misconfiguration or an attack), Shield
   passes it through without re-stamping.

### Packet Transformation

**Ingress** (Shadow to Kingdom):

```
BEFORE:  [Eth][IPv6][ext hdrs...][Transport][Payload]
AFTER:   [Eth][IPv6 nh=0][HBH+Monad][Transport][Payload]
                     ^         ^
                     |         +-- 24 bytes inserted
                     +-- Next Header changed to 0 (HBH)
```

**Egress** (Kingdom to Shadow):

```
BEFORE:  [Eth][IPv6 nh=0][HBH+Monad][Transport][Payload]
AFTER:   [Eth][IPv6 nh=6/17][Transport][Payload]
                     ^
                     +-- Next Header restored to original (TCP/UDP)
```

---

## How They Compose

The five components form a pipeline:

```
1. SHIELD inserts MONAD at Kingdom ingress
        |
2. HOP programs read/write MONAD, consulting SOPHIA for context
        |
3. Every program emits events to ANAMNESIS ring buffer
        |
4. trace-collector reads ANAMNESIS, publishes to WOTAN
        |
5. SHIELD strips MONAD at Kingdom egress, emits final DEATH event
        |
6. WOTAN distributes events to dashboard and other subscribers
```

The Monad is the thread that ties them together. It is born at Shield
ingress, mutated at every hop, observed by Anamnesis, distributed by Wotan,
and destroyed at Shield egress. Its 20-byte lifecycle captures the entire
distributed computation that the packet participated in.

---

## Design Constraints

### BPF Verifier

Every eBPF program must pass the kernel's BPF verifier before loading. The
verifier enforces:

- **Bounded loops**: Every loop must have a compile-time upper bound. No
  infinite loops are possible.
- **Memory safety**: Every pointer dereference must be preceded by a bounds
  check. Out-of-bounds access is rejected at load time.
- **No null dereferences**: Pointers from map lookups must be checked before
  use.
- **Stack limit**: eBPF programs have a 512-byte stack. All state that does
  not fit must go in BPF maps.
- **Instruction limit**: Programs are limited to ~1 million verified
  instructions (not runtime instructions -- verified paths).

These constraints are not limitations. They are safety guarantees. A verified
eBPF program cannot crash the kernel, cannot leak memory, and cannot enter an
infinite loop. This is provable, not merely tested.

### Fail-Open Design

All Unheaded eBPF programs are fail-open:

- If Shield fails to insert a Monad, the packet passes through as-is.
- If a hop fails to parse the Monad, the packet passes through.
- If a CRC check fails, the packet passes through (an anomaly is recorded).
- If a ring buffer is full, events are dropped (packets are not).

The only exception is explicit DROP actions (Shield blocklist, flow_action
DROP). Network connectivity is never sacrificed for observability.

### No Packet Resize at Hops

The hop-ebpf program writes the mutated Monad back to the same 20 bytes it
read from. There is no packet resize, no memory allocation, no buffer copy.
This is a pure in-place overwrite, which is the fastest possible mutation
path in XDP.

---

*Next: [Formal Specification Introduction](../phd/formal-specification-intro.md)
-- The mathematical foundations.*
