# The Unheaded Protocol: The High School Version

## Overview

The Unheaded Protocol (`draft-bellis-unheaded-protocol-foundation-04`) defines a **mapped data bus** over IPv6 Hop-by-Hop extension headers. It turns the network into a distributed computer where every packet carries computational state and every hop performs inline processing — all at kernel datapath speed using eBPF.

This document assumes familiarity with IP networking, basic programming, and the concept of key-value stores.

## Part 1: The Wire Format

### The Monad — A Register File in Every Packet

Every packet within the Limited Domain (the private network) carries a 20-byte register file called the **Monad**, embedded as an IPv6 Hop-by-Hop Options extension header:

```
IPv6 Header (40 bytes)
  └─ Hop-by-Hop Options Extension Header
       └─ Option TLV (Type=0x3E, Len=20)
            └─ Monad (20 bytes)
```

The Monad layout:

```
Offset  Size  Field               Type
------  ----  ------------------  ----------
0x00    1     version             raw uint8   Protocol version (0x01)
0x01    1     src_service_id      exponent    Source service
0x02    1     dst_service_id      exponent    Destination service
0x03    1     hop_count           raw uint8   Decremented per hop
0x04    1     qos_class           exponent    Quality of Service
0x05    1     flow_action         exponent    Action (trace/sample/mirror/drop)
0x06    1     circuit_state       exponent    Circuit breaker (open/closed/half)
0x07    1     flags               raw uint8   8-bit control flags
0x08    2     latency_hint        raw uint16  Latency target (microseconds)
0x0A    1     deploy_ring         exponent    Deployment ring (canary/staging/prod)
0x0B    1     mesh_flags          exponent    Mesh routing metadata
0x0C    1     src_prefix_lo       raw uint8   Source routing prefix
0x0D    1     dst_prefix_lo       raw uint8   Dest routing prefix
0x0E    4     scratch[0-3]        raw uint8   Per-hop scratch registers
0x12    2     checksum            raw uint16  CRC-16/CCITT integrity
------  ----  ------------------  ----------
Total: 20 bytes
```

**Key design principle**: 8 of the 14 fields are "exponent-encoded" — their meaning is defined by a runtime dictionary, not hardcoded. This makes the protocol dynamically extensible without changing any code.

### Flags Bitfield

```
 7   6   5   4   3   2   1   0
+---+---+---+---+---+---+---+---+
| C | Y | T | E | S | M |CUS| R |
+---+---+---+---+---+---+---+---+

C (0x80): CHAOS     — Chaos injection active
Y (0x40): CANARY    — Canary deployment path
T (0x20): TRACED    — Full trace (all hops emit events)
E (0x10): ENCRYPT   — Payload encrypted
S (0x08): SAMPLED   — Statistically sampled
M (0x04): MIRROR    — Mirror copy (not original)
CUS (0x02): CUSTOM  — Scratch fields carry exponent values
R (0x01): Reserved
```

### Checksum

CRC-16/CCITT-FALSE over the first 18 bytes (with the checksum field zeroed during computation). Polynomial: `x^16 + x^12 + x^5 + 1` (0x1021). Verified at every hop — corruption is detected immediately and flagged.

### Wire Overhead

On a standard 1500-byte Ethernet frame: 24 bytes overhead (2B Hop-by-Hop header + 2B option TLV + 20B Monad) = **1.6% overhead**. This metadata is present on every packet but only exists within the domain boundary.

## Part 2: Sophia — The Dictionary System

### Exponent Encoding

Each exponent field stores a single byte (0x00-0xFF). The byte is a **key** into a lookup dictionary called Sophia, implemented as BPF hash maps in the kernel.

```
Packet field: src_service_id = 0x03

Sophia dictionary (BPF map):
  0x01 → { name: "captain",      endpoint: "10.0.1.1:8080" }
  0x02 → { name: "timeguru",     endpoint: "10.0.1.2:8080" }
  0x03 → { name: "architect",    endpoint: "10.0.1.3:8080" }
  0x04 → { name: "micromanager", endpoint: "10.0.1.4:8080" }

Decoded: src_service_id = "architect"
```

### Tree-Structured Composition

Dictionaries are trees. The first byte selects a sub-dictionary; the second byte selects within it:

```c
// BPF pseudo-code for 2-level Sophia lookup
u8 key0 = monad->field;
sophia_entry *entry = bpf_map_lookup_elem(&sophia_root, &key0);
// entry->sub_dict_id identifies which sub-dictionary to use

u8 key1 = monad->next_field;
meaning *m = bpf_map_lookup_elem(&sophia_dicts[entry->sub_dict_id], &key1);
// m is the fully decoded meaning
```

**Scaling**: 1 byte = 256 meanings. 2 bytes composed = 65,536. 3 bytes = 16.7M. The 20-byte Monad contains effectively unlimited semantic space via composition.

### Hot-Swappable

Sophia dictionaries are BPF maps. Updating a map entry takes effect **on the next packet** — no restart, no redeployment, no downtime. Add a new service? New QoS class? New deployment ring? Just insert a dictionary entry.

Historical replay is preserved: Anamnesis stores raw exponent keys + timestamps. You can decode old events through any version of the dictionary.

## Part 3: The Processing Pipeline

### Shield — The Protocol Boundary

**Shield** is the XDP program at the domain edge. It manages the Monad lifecycle:

**Ingress (packet enters domain):**
1. Standard WAF checks (blocklist, rate limit, geo)
2. Grow packet by 24 bytes (Hop-by-Hop header + Monad)
3. Initialize Monad: version, service IDs from Sophia policy, trace hash from `bpf_get_prandom_u32()`, QoS from policy lookup, hop_count from config
4. Compute checksum
5. Emit `EVENT_BIRTH` to Anamnesis ring buffer
6. `XDP_PASS` — packet enters the Kingdom

**Egress (packet leaves domain):**
1. Emit `EVENT_DEATH` to Anamnesis (final Monad snapshot after all hops)
2. Strip 24 bytes (remove Monad)
3. Egress security checks
4. Clean IPv6 exits — the n+1 hop sees nothing

### Per-Hop Processing (The Void)

At every internal hop, BPF programs (the "Shim") process the Monad:

```
1. Parse Monad from Hop-by-Hop options header
2. Verify CRC-16 checksum
   → Failure: emit EVENT_ANOMALY, increment error counter, drop packet
3. Check hop_count == 0?
   → Yes: drop (loop protection), emit anomaly
   → No: decrement hop_count
4. Sophia lookup on flow_action field:
   0x01 (trace):  emit full event to Anamnesis
   0x02 (sample): emit with probability P from BPF map
   0x03 (mirror): clone packet to mirror port
   0x04 (rate_limit): check token bucket, stamp result
5. Component-specific processing:
   Shield:    security checks
   Pauldrons: load balancing (read dst, select backend)
   Hauberk:   circuit breaker (read/write circuit_state)
   Vambraces: full observability (emit all fields to Anamnesis)
6. Recompute CRC-16 checksum
7. Forward: XDP_PASS or TC_ACT_OK
```

Each BPF "armor piece" operates at a different kernel hook point:

| Component | Hook | Function |
|-----------|------|----------|
| **Shield** | XDP (ingress) | Firewall, Monad lifecycle |
| **Pauldrons** | TC (egress) | Load balancing, backend selection |
| **Hauberk** | TC (both) | Circuit breakers, mesh routing |
| **Vambraces** | Tracepoints | Full observability, event recording |
| **Cuirass** | TC | Control plane commands |

### Anamnesis — Event Recording

Every significant packet event is written to a per-CPU BPF ring buffer:

```c
struct anamnesis_event {
    u64 timestamp_ns;        // bpf_ktime_get_ns()
    u8  event_type;          // BIRTH=0, COMPUTED=1, DEATH=6, ANOMALY=8
    u8  hop_index;           // Which hop emitted this
    u16 reserved;
    u32 input_monad[5];      // 20 bytes: Monad BEFORE this hop
    u32 output_monad[5];     // 20 bytes: Monad AFTER this hop
    u32 trace_id;            // From IPv6 Flow Label
    u32 wotan_addr;          // Wotan memory address (if accessed)
};  // 64 bytes per event
```

**Capacity**: 102 MB per CPU = ~2 seconds of events at 10 Gbps line rate. Events are consumed by Wotan's ring buffer reader and published to subscribers.

### Wotan — The Central Nervous System

Wotan bridges the kernel (nanosecond, binary) and userspace (millisecond, structured):

**Upward path** (Void → Kingdom):
1. Poll Anamnesis ring buffers (epoll on fd)
2. Batch-read events (up to 256 per poll)
3. Decode exponent fields through Sophia userspace dictionaries
4. Publish structured events via gRPC streaming to subscribers

**Downward path** (Kingdom → Void):
1. Receive policy updates (new Sophia entries, routing changes)
2. Encode as BPF map updates
3. Atomic write to kernel maps
4. Takes effect on next packet (~10ms propagation)

Wotan also serves as the **pub/sub event bus** for all services. Key topics:

| Topic | Content |
|-------|---------|
| `anamnesis.birth` | New packets entering domain |
| `anamnesis.computed` | Per-hop computation events |
| `anamnesis.death` | Packets leaving domain |
| `anamnesis.anomaly` | Checksum failures, loop detection |
| `system.discovery` | Service registration/deregistration |
| `alerts.critical` | P0 alerts from any service |
| `state.drift.*` | Desired vs. actual state divergence |

## Part 4: State Management

### Pleroma (Desired State)

The declared configuration — what the system SHOULD be doing:

```yaml
protocol:
  version: 1
  sophia_version: 47
services:
  captain:
    id: 0x01
    qos: realtime
    deployment_ring: production
    trace: always
policies:
  circuit_breaker:
    threshold: 5_failures_in_10s
    recovery: 30s
```

Pleroma lives in Git. Changes go through version control.

### Kenoma (Observed State)

What the packets ACTUALLY show. Kenoma is a **materialized view** over Anamnesis:

```
Kenoma = fold(Anamnesis events, current Sophia dictionary)
```

For each flow: last hop count, circuit state, QoS class, event count, mean latency, last anomaly.

### Drift Detection

The reconciliation loop runs continuously:

```
if Kenoma[field] != Pleroma[field]:
    emit drift event
    Wotan pushes Pleroma down → BPF map update → packets comply
```

This is **GitOps for the network protocol**. `git diff` between Pleroma and Kenoma tells you exactly where reality has drifted from intent.

## Part 5: Chaos Engineering

### Yaldabaoth — The Adversary

A BPF program at the TC hook that deliberately introduces failures:

| Mode | Action |
|------|--------|
| BIT_FLIP | XOR random byte in Monad (without fixing checksum) |
| DELAY | TC scheduling adds N microseconds |
| DUPLICATE | `bpf_clone_redirect()` creates phantom traffic |
| TRUNCATE | Zero out latency/reserved fields |
| CHAOS_MARKER | Set flags bit 7 (0x80) so downstream can see it |

**All chaos injection is recorded in Anamnesis.** Every perturbation has a before/after snapshot. Chaos is always auditable.

Trigger: `bpf_get_prandom_u32() < configured_threshold`. At 0.1%, roughly 1 in 1000 packets gets chaos injection.

## Part 6: Evolution Path

| Age | Transport | Metadata | Key Innovation |
|-----|-----------|----------|----------------|
| **1 (current)** | IPv4 + 20-byte shim | 20 bytes | Core protocol, Sophia, Anamnesis |
| **2** | IPv6, metadata in mapped-address space | 20 bytes (free) | Zero overhead — metadata IS the address |
| **3** | IPv6 + full Hop-by-Hop extension headers | Up to 64 KB | Arbitrary-depth Sophia trees |

The protocol is **transport-agnostic**. Exponent encoding, Sophia dictionaries, and BPF programs work on any byte sequence at any offset. The architecture outlives any single transport.

---

*Previous: [← Middle School Explanation](02-middle-school.md) | Next: [PhD/Staff-Level Explanation →](04-staff-phd.md)*
