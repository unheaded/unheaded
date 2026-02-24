# Architecture Overview

*High School Level*

---

## The Full Stack

Unheaded's architecture spans from raw packets on the wire to a JavaScript
dashboard in the browser. This document traces the complete data path,
explains every component, and shows how they connect.

---

## Protocol Layer Model

```
+================================================================+
|  LAYER 5: USER INTERFACE                                        |
|  Dashboard (vanilla JS, port 20000)                             |
|  Kanban App (vanilla JS, port 20001)                            |
+================================================================+
        |  WebSocket / HTTP
        v
+================================================================+
|  LAYER 4: APPLICATION SERVICES                                  |
|  dashboard-backend  (Go, port 20000)                            |
|  kanban-app         (Go, port 20001)                            |
|  timeguru           (Go, port 19000)                            |
|  captain            (Go, port 19002)                            |
|  architect          (Go, port 19001)                            |
|  micromanager       (Go, port 19003)                            |
|  monad              (Go, port 19004)                            |
|  sophia             (Go, port 19005)                            |
+================================================================+
        |  gRPC / HTTP
        v
+================================================================+
|  LAYER 3: INFRASTRUCTURE SERVICES                               |
|  wotan              (Go, gRPC port 18001, HTTP port 18000)      |
|  trace-collector    (Rust, port 16670/16671)                    |
|  gateway            (nginx, HTTPS port 21443, HTTP port 21000)  |
+================================================================+
        |  gRPC / Ring Buffer
        v
+================================================================+
|  LAYER 2: CONTROL PLANE                                         |
|  unheaded-daemon    (Go, HTTP port 17000, gRPC port 17001)      |
|  - Container orchestration (LXD/Docker/containerd)              |
|  - eBPF program loading/unloading                               |
|  - State enforcement and drift detection                        |
|  - Telemetry reporting to Wotan                                 |
+================================================================+
        |  BPF syscalls / Ring Buffer
        v
+================================================================+
|  LAYER 1: DATA PLANE (eBPF)                                     |
|  shield-ebpf        XDP ingress: insert Monad, emit BIRTH      |
|  shield-ebpf        TC egress:   strip Monad, emit DEATH       |
|  hop-ebpf           XDP interior: process Monad per-hop        |
|  packet-marker      XDP: trace ID injection (IPv4 legacy)      |
|  flow-tracker       TC:  connection state tracking              |
|  latency-probe      kprobes: tcp_sendmsg/recvmsg timing        |
+================================================================+
        |  Direct packet access
        v
+================================================================+
|  LAYER 0: INFRASTRUCTURE                                        |
|  Linux kernel 5.8+ (eBPF support)                               |
|  LXD / containerd / Docker (container runtime)                  |
|  Network: lxdbr0 bridge (10.10.10.0/24)                         |
|  NixOS container definitions (declarative, immutable)           |
+================================================================+
```

---

## IPv6 Hop-by-Hop Headers: The Wire Format

Unheaded uses IPv6 Hop-by-Hop (HBH) extension headers to carry the Monad
register file through the network stack. This is the critical design decision
that enables per-hop computation at wire speed.

### Packet Layout

```
+------------------------------------------------------+
| Ethernet Header (14 bytes)                            |
|   Dst MAC [6] | Src MAC [6] | EtherType=0x86DD [2]  |
+------------------------------------------------------+
| IPv6 Fixed Header (40 bytes)                          |
|   Version=6 [4b] | TC [8b] | Flow Label [20b]       |
|   Payload Length [2] | Next Header=0 (HBH) [1]       |
|   Hop Limit [1] | Source Addr [16] | Dest Addr [16]  |
+------------------------------------------------------+
| Hop-by-Hop Options Header (24 bytes)                  |
|   Next Header (original transport) [1]                |
|   Hdr Ext Len = 2 [1]                                |
|   Opt Type = 0x3E [1]                                 |
|   Opt Data Len = 20 [1]                               |
|   +--------------------------------------------------+|
|   | MONAD REGISTER FILE (20 bytes)                   ||
|   |  0x00: version          (u8)                     ||
|   |  0x01: src_service_id   (u8, exponent-encoded)   ||
|   |  0x02: dst_service_id   (u8, exponent-encoded)   ||
|   |  0x03: hop_count        (u8)                     ||
|   |  0x04: qos_class        (u8, exponent-encoded)   ||
|   |  0x05: flow_action      (u8, exponent-encoded)   ||
|   |  0x06: circuit_state    (u8, exponent-encoded)   ||
|   |  0x07: flags            (u8, bitfield)           ||
|   |  0x08: latency_hint     (u16, big-endian)        ||
|   |  0x0A: deploy_ring      (u8, exponent-encoded)   ||
|   |  0x0B: mesh_flags       (u8, exponent-encoded)   ||
|   |  0x0C: src_prefix_lo    (u8)                     ||
|   |  0x0D: dst_prefix_lo    (u8)                     ||
|   |  0x0E: scratch[0..3]    (4 x u8)                ||
|   |  0x12: checksum         (u16, CRC-16/CCITT)     ||
|   +--------------------------------------------------+|
+------------------------------------------------------+
| Transport Header (TCP/UDP/etc.)                       |
+------------------------------------------------------+
| Application Payload                                   |
+------------------------------------------------------+
```

### Option Type Encoding (0x3E)

The Option Type byte follows RFC 8200 Section 4.2:

```
  Bit 7-6 (action):          00 = skip if unrecognized
  Bit 5   (change-en-route): 1  = option data MAY change at each hop
  Bits 4-0 (number):         0x1E (30, locally assigned)

  Binary: 0b0011_1110 = 0x3E
```

The `change-en-route` bit is set because the Monad is explicitly designed to
be read and written by eBPF programs at every hop. The `skip if unrecognized`
action means non-Unheaded routers will pass the packet through without
disruption.

### Flow Label Correlation

The IPv6 Flow Label (20 bits) is set by Shield at ingress using
`bpf_get_prandom_u32()`. All Anamnesis events for a given packet carry this
same Flow Label, which acts as a trace correlation key. The 20-bit space
provides 1,048,576 concurrent trace IDs before collision, which is sufficient
for the target throughput.

---

## BPF Maps: The State Layer

eBPF programs are stateless by design -- they process one packet and exit.
**BPF maps** provide persistent key-value storage that eBPF programs can read
and write, and that userspace programs can also access.

### Map Inventory

| Map Name | Program | Type | Key | Value | Max Entries |
|----------|---------|------|-----|-------|-------------|
| `ANAMNESIS` | shield, hop | RingBuf | N/A | AnamnesisEvent (32B) | 8 MiB |
| `SOPHIA` | hop | HashMap | u16 (dict_id << 8 \| value) | SophiaEntry | 65,536 |
| `CIRCUIT_ERRORS` | hop | HashMap | u16 (src << 8 \| dst) | u32 error count | 65,536 |
| `CONFIG` | hop | HashMap | u32 key | u64 value | 16 |
| `STATS` | all | HashMap | u32 key | u64 counter | 32 |
| `BLOCKLIST` | shield | HashMap | u64 (src addr low 64) | u8 flag | 4,096 |
| `RATE_TOKENS` | shield | HashMap | u64 (src addr low 64) | u32 token count | 4,096 |
| `FLOWS` | flow_tracker | LruHashMap | FlowKey (16B) | FlowState (72B) | 65,536 |
| `FLOW_STATE` | packet_marker | HashMap | FlowKey (16B) | FlowState (72B) | 65,536 |
| `INFLIGHT` | latency_probe | HashMap | InflightKey (16B) | InflightValue (16B) | 8,192 |
| `LATENCY_MAP` | latency_probe | HashMap | LatencyKey | LatencyEntry | 8,192 |

### Ring Buffer vs. HashMap

**Ring buffers** (ANAMNESIS, PACKET_EVENTS, FLOW_EVENTS, LATENCY_EVENTS) are
used for unidirectional eBPF-to-userspace event streaming. They are
lock-free, per-CPU, and fixed-size. When full, new events are dropped (not
queued), and a dropped counter is incremented.

**HashMaps** are used for bidirectional state. eBPF programs read and write
them. Userspace programs can also read and write them (to populate Sophia
dictionaries, update blocklists, adjust configuration).

**LruHashMaps** (FLOWS) automatically evict the least-recently-used entry when
full, providing natural expiration of stale connections without explicit
cleanup.

---

## 5-Tuple Correlation

The flow tracker normalizes every connection into a canonical 5-tuple:

```
FlowKey (16 bytes, repr(C)):
  src_addr:  u32  (smaller IP always first)
  dst_addr:  u32  (larger IP)
  src_port:  u16  (port matching src_addr)
  dst_port:  u16  (port matching dst_addr)
  protocol:  u8   (TCP=6, UDP=17)
  _pad:      [u8; 3]
```

Normalization ensures that packets in both directions of a connection map to
the same FlowKey. The convention is: the smaller IP address goes in
`src_addr`. If IPs are equal, the smaller port goes in `src_port`.

The flow tracker maintains full TCP state machine tracking:

```
  Unknown --> SynSent --> SynReceived --> Established
                                              |
                          +------- FIN -------+
                          |                   |
                          v                   v
                      FinWait1            CloseWait
                       |    |                 |
                  ACK  |    | FIN         FIN |
                       v    v                 v
                  FinWait2  Closing       LastAck
                       |       |              |
                   FIN |   ACK |          ACK |
                       v       v              v
                     TimeWait--+          Closed
                       |
                   (timeout)
                       v
                     Closed

  RST at any state --> Closed (immediately)
```

State transitions are emitted as FlowEvent records to the FLOW_EVENTS ring
buffer, allowing userspace to track connection lifecycle without polling.

---

## The Anamnesis Event Pipeline

Every eBPF program emits events to the **Anamnesis** ring buffer. Events are
exactly 32 bytes (one CPU cache line), structured as:

```
AnamnesisEvent (32 bytes):
  timestamp_ns:  u64     Kernel timestamp (bpf_ktime_get_ns)
  event_type:    u8      Birth(0x01), Hop(0x02), Death(0x03),
                         Anomaly(0x04), Chaos(0x05)
  hop_id:        u8      Which node emitted this event
  flow_label_lo: [u8; 2] Low 16 bits of IPv6 Flow Label
  monad:         [u8; 20] Full Monad register file snapshot
```

The event pipeline:

```
eBPF programs                    Userspace
+----------+     Ring Buffer     +-----------------+     gRPC      +-------+
| shield   |---->|          |    | trace-collector |-------------->| Wotan |
| hop      |---->| ANAMNESIS|--->| (Rust)          |               |       |
| etc.     |---->|          |    +-----------------+               +---+---+
+----------+     +----------+                                         |
                                                            gRPC sub  |
                                              +-------------------+   |
                                              | dashboard-backend |<--+
                                              | (Go)              |
                                              +--------+----------+
                                                       | WebSocket/SSE
                                                       v
                                              +-------------------+
                                              | Dashboard (JS)    |
                                              +-------------------+
```

The trace-collector reads the ring buffer using aya's `AsyncPerfEventArray`,
correlates events by Flow Label, and publishes them to Wotan topic
`network.traces` via gRPC streaming. The dashboard-backend subscribes and
pushes updates to connected browsers.

Target latency: under 50ms from packet arrival to dashboard rendering.

---

## Shield: The Boundary Enforcer

Shield runs two programs:

### Ingress (XDP)

1. Parse Ethernet + IPv6 headers
2. Check source against BLOCKLIST map -- XDP_DROP if blocked
3. Reject packets already carrying a Monad HBH header (anti-double-stamp)
4. Strip any existing IPv6 extension headers from inbound traffic
5. Call `bpf_xdp_adjust_head(-24)` to extend packet by 24 bytes
6. Shuffle Ethernet + IPv6 headers forward
7. Write the 24-byte HBH Options header with initialized Monad
8. Set IPv6 Next Header = 0 (HBH), update Payload Length
9. Set Flow Label from `bpf_get_prandom_u32()`
10. Emit BIRTH event to Anamnesis
11. XDP_PASS -- hand to kernel network stack

### Egress (TC)

1. Load EtherType, check for IPv6
2. Verify packet carries a Monad HBH header
3. Load the full Monad from SKB data
4. Read original Next Header from HBH[0]
5. Read Flow Label for correlation
6. Verify Monad checksum integrity
7. Emit DEATH event to Anamnesis
8. Call `bpf_skb_adjust_room(-24)` to remove the 24-byte HBH header
9. Restore IPv6 Next Header to original transport protocol
10. Decrement Payload Length by 24
11. TC_ACT_OK -- packet exits as clean standard IPv6

**Security invariant**: No Monad ever exits the Kingdom boundary. No external
extension header enters unstripped. Shield enforces containment at wire speed.

---

## Hop: The Interior Processor

The hop-ebpf program runs at every interior node. Its processing pipeline:

```
Parse IPv6 + HBH --> Find Monad option (scan up to 16 TLV options)
    |
    v
Verify CRC-16/CCITT (skip if CUSTOM flag set)
    |
    v
Check CHAOS flag --> skip if set (Yaldabaoth owns this packet)
    |
    v
Check hop limit --> emit ANOMALY if exceeded
    |
    v
Sophia dictionary lookups:
  - QoS class --> scheduling weight
  - Circuit state --> human-readable metadata
  - Service ID --> service name/tier
    |
    v
Circuit breaker enforcement:
  - Read CIRCUIT_ERRORS[src|dst]
  - If errors >= threshold --> set circuit_state = OPEN
    |
    v
Increment hop_count (saturating)
    |
    v
Apply flow_action:
  - FORWARD: pass through (default)
  - TRACE: set TRACED flag, force full event emission downstream
  - SAMPLE: set SAMPLED flag if flow_label % divisor == 0
  - MIRROR: set MIRROR flag for TC clone_redirect
  - DROP: mark for XDP_DROP after write-back
    |
    v
Recompute CRC-16/CCITT (skip if CUSTOM flag)
    |
    v
Write mutated Monad back in-place (20-byte overwrite, no resize)
    |
    v
Emit HOP event (if TRACED, SAMPLED, or head-based sample hit)
    |
    v
XDP_PASS or XDP_DROP (based on flow_action)
```

---

## Transport Priority

All inter-service communication follows the gRPC-first cascade:

```
1. Primary:  gRPC streaming (Wotan port 18001)
2. Fallback: HTTP/3 (QUIC)
3. Fallback: HTTP/2
4. Fallback: HTTP/1.1
```

The transport package (`pkg/transport/`) implements automatic fallback. All
10 services are wired through this package for unified connection management.
Services also expose both gRPC health check and HTTP `/health` endpoints.

---

## State Management

```
Desired State (Git):
  nix/containers/*.nix     Container definitions
  configs/services.yaml    Service topology
  references/*.md          Source-of-truth documents

Actual State (Filesystem):
  /var/lib/unheaded/state/containers.json
  /var/lib/unheaded/state/ebpf-programs.json
  /var/lib/unheaded/state/metrics.json

Drift Detection:
  unheaded-daemon polls every 30 seconds
  Compares desired state (Nix definitions) vs actual state (LXD/Docker)
  On drift: log to Wotan, auto-remediate (restart with correct config), alert dashboard
```

---

## Network Topology

```
                        INTERNET
                            |
                    +-------+-------+
                    |   Gateway     |
                    | 10.10.10.100  |
                    | :21443 HTTPS  |
                    +-------+-------+
                            |
              +-------------+-------------+
              |         lxdbr0            |
              |     10.10.10.0/24         |
              +--+--+--+--+--+--+--+--+--+
                 |  |  |  |  |  |  |  |
  +---------+   +--+  +--+  +--+  +--+   +---------+
  | Wotan   |   |TG|  |CA|  |AR|  |MM|   | Dash    |
  |.10:18001|   |.20|  |.21|  |.23|  |.22|   |.30:20000|
  +---------+   +--+  +--+  +--+  +--+   +---------+
                                            |
                                          +---------+
                                          | Kanban  |
                                          |.200:20001|
                                          +---------+

  TG=timeguru  CA=captain  AR=architect  MM=micromanager
```

---

*Next: [Protocol Primer](protocol-primer.md) -- The five protocol components
and how they compose.*
