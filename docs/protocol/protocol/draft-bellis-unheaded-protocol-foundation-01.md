---
title: "The Unheaded Protocol Foundation: A Mapped Data Bus over IPv6 Hop-by-Hop Options"
abbrev: "Unheaded Protocol Foundation"
docname: draft-bellis-unheaded-protocol-foundation-01
category: exp
ipr: trust200902
area: Internet
date: 2026-02-18
stand_alone: yes

keyword:
  - ebpf
  - bpf-isa
  - ipv6
  - hop-by-hop
  - mapped-data-bus
  - metadata
  - packet-tagging
  - observability
  - exponent-encoding
  - limited-domain
  - computational-completeness

author:
  - ins: S. Bellis
    name: Steven Bellis
    org: Unheaded
    email: stevenrbellis@gmail.com
    country: US

pi:
  toc: yes
  symrefs: yes
  sortrefs: yes
  compact: yes
  subcompact: no

--- abstract

The Unheaded Protocol Foundation defines a mapped data bus model that transforms IPv6 packets into addressable memory by encoding a small register file directly in the IPv6 Hop-by-Hop Options extension header.

We introduce a 20-byte Monad (register file) that carries program state through the network.  At each hop, an eBPF program (the Shim) performs computation on the Monad.  The packet itself becomes the working memory, using exponent-encoded fields to pack rich metadata into the IPv6 option while remaining fully backward-compatible.

To support programs larger than what fits in a single Monad, we introduce Wotan, a memory and I/O bus that bridges Monad computation to per-flow ring-buffer storage and external topics.  This decouples the Monad (pure, 20-byte compute) from memory (Wotan's configurable data planes).

The Architecture Layers section describes how Shield (packet birth) frames the entire compute trace, Shim (at each hop) manages eBPF execution on the Monad, and Anamnesis (the event log) provides non-blocking historical replay for observability, chaos injection, and state reconciliation.

This memo defines the packet format, exponent-encoding scheme, per-hop processing semantics, optional chaos injection for resilience testing, and the complete computational model (Turing-complete with memory paging).

--- middle

# Introduction

## Terminology

The Unheaded Protocol Foundation uses the following terminology:

- **Monad**: The 20-byte register file embedded in the IPv6 Hop-by-Hop option header.  It travels with the packet.

- **Shim**: The stateless eBPF program that runs at each hop, reading the Monad from the packet, performing computation, and writing back to the packet.

- **Shield**: The packet boundary — the first and last hop logic that stamps the packet into existence (ingress) and commits it (egress).

- **Wotan**: The memory and I/O bus, providing per-flow ring buffers (configured via --ring-size), persistent storage (WAL), and bridging to external topics.

- **Anamnesis**: The non-blocking event log, recording packet events for observability and historical replay.

- **Exponent Encoding**: A compositional semantic for packing metadata: each field stores a signed exponent, and the actual value is reconstructed as `base^exponent * multiplier`.

- **Limited Domain**: A network boundary (typically a single AS, corporation, or Kubernetes cluster) where Unheaded is deployed end-to-end.

- **Kenoma**: The state of the network observed from outside (by external systems).

- **Pleroma**: The complete internal state of the system (Monads + Wotan buffers + WAL + Anamnesis).

- **Drift**: Divergence between Pleroma and Kenoma; detected and corrected via periodic reconciliation.

## Requirements Language

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in RFC 2119 and RFC 8174.

# The Mapped Data Bus Model

## The Packet Is the Memory

Classical networking separates computation from data:

- Computation happens in applications (servers, routers).
- Data flows through the network (packets are opaque).
- Serialization and deserialization boundaries are expensive and defeat optimization.

The Unheaded Protocol Foundation inverts this: **the packet is the memory**.  The Monad (register file) is written and read by eBPF programs at each hop.  The packet itself is the working storage.

This enables:

1. **Zero-copy pass-through**: Application state travels with the packet; no de/serialization.
2. **Hop-local compute**: Each hop can read the latest Monad state without network round-trips.
3. **Deterministic causality**: Events are timestamped (in Anamnesis) in packet traversal order, not wall-clock order.

## Proof of Concept: Packet as RAM/Cache

The intuition is captured in the following model:

~~~~~
Traditional model:
  [App] -> serialize -> [packet] -> deserialize -> [App]
  State lives in application memory.  Packets are dumb pipes.

Mapped data bus model:
  [Shield: birth] -> [Hop 1: compute] -> [Hop 2: compute] ->
  ... -> [Hop N: compute] -> [Shield: death]
  Registers live in the packet.  The network is the compute bus.
  Memory lives in Wotan ring buffers.  Anamnesis is the trace log.

Memory hierarchy:
  L0: Monad Scratch registers (20 bytes, wire speed)
  L1: Per-hop BPF map cache (prefetched by Wotan)
  L2: Wotan ring buffer (configurable via --ring-size)
  L3: Wotan WAL (persistent storage)
  L4: Sophia dictionaries (instruction decode only)
~~~~~

The Monad is the fast path (inline in the packet); Wotan is the slow path (out-of-line memory accessed by Flow Label).  Together, they form a complete memory hierarchy.

## Design Principles

1. **Minimalism in the packet**: The Monad is 20 bytes.  Rich metadata is encoded via exponent notation.
2. **Separation of concerns**: Registers (Monad) are compute; memory (Wotan) is I/O.
3. **Non-blocking observability**: Anamnesis records events without blocking the Shim.
4. **Optional chaos**: Chaos injection is an optional feature for resilience testing.
5. **Backward compatibility**: The Hop-by-Hop option is recognized by standards-compliant IPv6 routers.

## Architecture Layers

The Unheaded architecture is organized into distinct layers:

Layer 1 - The Packet:
: The IPv6 datagram carrying the Hop-by-Hop option with the Monad and metadata.

Layer 2 - Wotan:
: The memory and I/O bus, providing per-flow addressable storage, persistent WAL, and topic-based I/O bridges.  Wotan is configured at Unheaded deployment time (--ring-size, --wal-path, etc.) and persists across packet lifetimes.

Layer 2.5 - The Memory Bus:
: Wotan ring buffers repurposed as addressable memory.
  Dedicated channels keyed by Flow Label provide per-flow
  RAM (hot ring buffer, configurable via --ring-size) and
  persistent storage (WAL).  The Monad stays pure compute --
  20 bytes of registers at wire speed.  Memory access travels
  through Wotan, not through the Monad.

Layer 3 - The Shim:
: The per-hop eBPF program.  At each Unheaded node, the Shim is loaded into the kernel (via BPF subsystem), extracted from the Hop-by-Hop option, and executed with the Monad as input.  The Shim updates the Monad in-place and returns it to the packet.  Shim execution is stateless and deterministic given the input Monad.

Layer 4 - Shield:
: The ingress and egress boundaries.  Shield (birth) stamps the packet with a Monad; Shield (death) commits it to the application.  Between birth and death, the packet is an in-flight computation.

Layer 5 - Anamnesis:
: The non-blocking event log.  As the packet flows through hops, events are recorded in a ring buffer (keyed by packet ID).  Anamnesis provides the historical trace for debugging, chaos injection, and state reconciliation.

# IPv6 Hop-by-Hop Option Format

## Header

The IPv6 Hop-by-Hop Options extension header (Next Header = 0) carries our Monad in a single option TLV (Type-Length-Value).

~~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
|                                                               |
|                     Option TLV(s)                            |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~~

The Hop-by-Hop extension header precedes all other headers (IPv6 Destination Options, Routing, etc.).  It is processed at each hop per RFC 8200, section 4.2.

## Option Type

The Monad is carried in a single Unheaded Option:

~~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Type = TBD   |       Len     |          Payload              |
|               |               |        (20+ bytes)            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        Monad (20 bytes)                       |
|                                                               |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Optional: Metadata fields, Wotan routing, Chaos payload    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~~

Type:
: The option type value (to be assigned by IANA).  The high-order two bits MUST be `00` (skip on unrecognized) and the third bit MUST be 0 (unchanged on transit), yielding a format like `00Txxxxx`.  This ensures backward compatibility: unaware routers skip the option and leave it unchanged.

Len:
: The length of the option payload in octets (not including the Type and Len octets themselves).  Minimum value is 20 (Monad only); maximum is 255.

Payload:
: The option data, starting with the 20-byte Monad register file, followed by optional metadata fields.

## Monad Register File

The Monad is a 20-byte register file organized as five 32-bit words:

~~~~~
Offset  Name      Type      Purpose
------  ----      ----      -------
 0      R0        u32       Accumulator
 4      R1        u32       Argument
 8      R2        u32       Result
12      R3        u32       Counter
16      R4        u32       Caller / Error code
~~~~~

All fields are in network byte order (big-endian).  The Shim (eBPF program at each hop) reads and updates these fields.

- **R0**: General-purpose accumulator.
- **R1**: Argument or secondary value.
- **R2**: Result or output register.
- **R3**: Counter or loop variable.
- **R4**: Caller ID or error code.

The semantics of each field are program-defined; the protocol does not prescribe their use.

## Field Definitions and Exponent Encoding

Following the Monad, optional metadata fields are packed using exponent encoding.  Each field occupies a variable number of bits and is interpreted as:

~~~~~
value = base^exponent * multiplier
~~~~~

The encoding allows compressing rich metadata (latency, loss rate, queue depth, etc.) into the 255-byte option payload.

Exponent-encoded fields are defined per-program (in the program metadata or Sophia dictionary).  The protocol does not mandate specific fields; instead, it provides the encoding scheme.

Example metadata fields (illustrative only):

- **Hop Count** (1 byte, unsigned): Number of hops since Shield birth.
- **Latency** (2 bytes, exponent): `base = 2, exponent = u8, multiplier = 1ns`.  A value of 0x10 represents 2^16 ns = 65.5 us.
- **Loss Rate** (1 byte, exponent): `base = 2, exponent = i8 (signed), multiplier = 1/256`.  A value of -8 represents 2^-8 / 256 = 1/65536.
- **Queue Depth** (2 bytes, exponent): `base = 2, exponent = u8, multiplier = 1 packet`.  A value of 0x0A represents 2^10 = 1024 packets.

Metadata field layout and encoding rules are specified in the Shim program's metadata block (stored in Sophia, keyed by program name).

## Flags Bitfield

Inside the Hop-by-Hop option payload, after the Monad, a 1-byte flags field controls optional behavior:

~~~~~
 0 1 2 3 4 5 6 7
+-+-+-+-+-+-+-+-+
|A|B|C|D|E|F|G|H|
+-+-+-+-+-+-+-+-+
~~~~~

Where:

- **A** (0x80): Enable chaos injection (if bit is set, Shim applies chaos modes).
- **B** (0x40): Enable memory tracing (Wotan records all ring-buffer operations to Anamnesis).
- **C** (0x20): Enable deterministic replay (Shim uses Anamnesis event stream instead of live state).
- **D-H** (0x1F): Reserved for future use.

## Trace Correlation

The Hop-by-Hop option includes a 4-byte trace ID (correlation token):

~~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|     Trace ID (4 bytes)         |     Packet ID (4 bytes)      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~~

- **Trace ID**: A correlation token shared by all packets in a logical flow (e.g., a request and its responses).  Set by Shield at packet birth.
- **Packet ID**: A unique identifier for this packet within the trace.  Assigned sequentially by Shield.

Anamnesis uses (Trace ID, Packet ID) as the key to record events.

## ULA Prefix

The IPv6 source address SHOULD use a Unique Local Address (ULA) prefix (RFC 4193) to signal that the packet is part of an Unheaded computation.  The recommended prefix is:

~~~~~
fd00:3f:unheaded::/48
~~~~~

Where:

- `fd00` is the ULA prefix.
- `3f` is an organization identifier (chosen to avoid collisions).
- `unheaded` (as hex: `756e68` = "uh") is the protocol identifier.

Packets originating outside the limited domain (e.g., from the public Internet) MUST be encapsulated or rewritten to use the ULA prefix before entering Shield.

# Exponent Encoding

## Overview

Exponent encoding is a compositional scheme for representing rich metadata in compact form.  Each value is encoded as a pair of signed bytes:

~~~~~
value = base^exponent * multiplier
~~~~~

Where:

- **base**: A constant (typically 2, sometimes 10).
- **exponent**: A signed byte (-128 to +127).
- **multiplier**: A scaling factor (unit-dependent).

## Compositional Semantics

Exponent-encoded fields are recursively composable:

- A field may itself be the result of arithmetic on two exponent-encoded sub-fields.
- The Shim eBPF programs use BPF map lookups (Sophia dictionaries) to retrieve encoding rules per field.
- Decoders (e.g., observability systems) apply the same rules in reverse.

Example: Latency is encoded as:

~~~~~
latency_us = 2^exponent_latency * 1us
~~~~~

If exponent_latency = 10, then latency_us = 2^10 = 1024 us.

## BPF Map Implementation

Exponent encoding rules are stored in a Sophia BPF array map:

~~~~~
struct exponent_rule {
  u8 base;           // 2, 10, or 256
  u8 fractional_bits; // for base 2^f
  u32 multiplier;    // in units (e.g., 1000 for ms -> us)
  u32 min_exponent;
  u32 max_exponent;
};
~~~~~

At each hop, the Shim looks up the rule for a field (by field ID), extracts the exponent from the packet, and reconstructs the value.

## Dictionary Updates

Exponent rules may change over time (e.g., as programs are updated).  Wotan maintains a versioned Sophia dictionary (keyed by program version and field ID) and provides a subscription mechanism for Shims to be notified of rule changes.

# Shield

## Overview

Shield is the packet boundary logic:

- **Shield (birth)**: Ingress logic that creates a packet, stamps it with a Monad, assigns trace and packet IDs, and hands it to the first Shim.
- **Shield (death)**: Egress logic that receives the final Monad, computes a checksum, and passes the packet to the application or forward path.

Shield MUST run on the source node (ingress) and the sink node (egress) of the Unheaded domain.

## Ingress

At ingress, Shield performs:

1. **Packet creation**: The application provides a program name and initial arguments.
2. **Monad initialization**: The Monad is allocated and populated with initial register values (program-defined).
3. **Trace ID assignment**: A 4-byte trace ID is generated (can be pseudorandom or a hash of source/dest).
4. **Packet ID assignment**: A sequential 4-byte packet ID is assigned within the trace.
5. **Hopcount initialization**: The Hop-by-Hop option's hop count field is set to 0.
6. **Hop-by-Hop option construction**: The Monad, metadata fields, flags, and trace/packet IDs are packed into the option.
7. **IPv6 header construction**: The packet is given an IPv6 source (ULA) and destination (ULA) address; the Hop-by-Hop Next Header is set.
8. **Forwarding**: The packet is handed to the routing layer for delivery to the next hop.

Shield may optionally preload Wotan memory (e.g., initialize ring-buffer state) before creating the packet.

## Egress

At egress, Shield performs:

1. **Monad extraction**: The Hop-by-Hop option is parsed, and the Monad is extracted.
2. **Checksum verification**: The Monad is checksummed (see below); if the checksum fails, the packet is discarded.
3. **Anamnesis finalization**: Shield signals to Anamnesis that the packet has reached its destination; Anamnesis finalizes the event log.
4. **Packet hand-off**: The Monad (or application-level result) is passed to the application.
5. **Cleanup**: Wotan memory for this (Trace ID, Packet ID) is deallocated after a grace period.

If the checksum fails, the packet is logged as an error and discarded. The application is notified of the failure (via an exception or status field).

## Lifecycle

A packet's lifecycle:

~~~~~
[Shield birth] -> [Shim 1] -> [Shim 2] -> ... -> [Shim N] -> [Shield death]
     |                |          |                  |              |
     +-> Anamnesis event: "born" |                  |              |
         (Trace ID, Packet ID)   +-> Anamnesis events: "computed" |
                                                                    +-> Anamnesis event: "died"
~~~~~

Each Shim execution is recorded in Anamnesis with the (Trace ID, Packet ID), the hop index, the input Monad, the output Monad, and the wall-clock timestamp.

# Per-Hop Processing

At each hop (routing node, Unheaded-aware), the following occurs:

1. **Hop-by-Hop option parsing**: The IPv6 extension header is parsed; the Monad and metadata are extracted.
2. **Program lookup**: The program name (from the packet metadata or a default) is resolved (via Wotan) to locate the Shim eBPF bytecode.
3. **Shim loading**: The Shim is attached to the XDP (eXpress Data Path) or TC (traffic class) hook if not already loaded.
4. **eBPF execution**: The Shim is invoked with the Monad as input.  The Shim reads from Wotan (ring buffer memory, I/O topics), modifies the Monad, and returns the updated Monad.
5. **Monad update**: The updated Monad is written back to the packet.
6. **Metadata update**: Hop count and exponent-encoded metadata (latency, loss, queue depth) are updated.
7. **Anamnesis logging**: The (Trace ID, Packet ID, hop index, input Monad, output Monad) tuple is recorded non-blockingly.
8. **Forwarding**: The packet is forwarded to the next hop (or to Shield death if it's the last hop).

All per-hop processing MUST be stateless and deterministic given the input Monad.  If the Shim needs to access external state, it MUST retrieve it from Wotan (e.g., via a ring-buffer lookup or a topic read).

# Anamnesis

## Event Structure

Anamnesis records packet events in a ring buffer, keyed by (Trace ID, Packet ID).  Each event is a fixed-size entry:

~~~~~
struct anamnesis_event {
  u32 timestamp_ns;    // Nanosecond timestamp (relative to Shield birth)
  u32 hop_index;       // Which hop (0 = Shield birth, 1..N-1 = intermediate hops, N = Shield death)
  u8 event_type;       // See event types below
  u8 flags;            // Event flags (e.g., "chaos applied", "rollback")
  u16 reserved;        // For alignment

  u32 input_monad[5];  // Input Monad state
  u32 output_monad[5]; // Output Monad state
  u32 wotan_addr;     // Address of Wotan memory access (if applicable)
  u32 wotan_value;    // Value read or written (if applicable)
};
~~~~~

Total size: 56 bytes per event (or less if packed).

## Event Types

- **EVENT_BORN** (0): Packet created at Shield (birth).
- **EVENT_COMPUTED** (1): Shim executed and Monad updated.
- **EVENT_WOTAN_READ** (2): Wotan ring-buffer read.
- **EVENT_WOTAN_WRITE** (3): Wotan ring-buffer write.
- **EVENT_CHAOS** (4): Chaos mode applied (bit flip, value mutation, etc.).
- **EVENT_ROLLBACK** (5): Monad was rolled back (e.g., due to chaos).
- **EVENT_DIED** (6): Packet reached Shield (death).

## Ring Buffer Sizing

Anamnesis uses a per-trace ring buffer, sized at deployment time (--anamnesis-ring-size).  Default: 16 KB per trace (sufficient for ~300 events).  If the ring buffer overflows, the oldest events are overwritten.

## Sampling

To reduce overhead, Anamnesis supports probabilistic sampling:

~~~~~
--anamnesis-sample-rate 0.1  # Sample 10% of events
~~~~~

Only sampled events are written to the ring buffer.  Non-sampled events are discarded.

## Historical Replay

Anamnesis events are persisted to the Wotan WAL at Shield (death).  The Unheaded debugging toolchain can replay a packet's computation by re-executing the Shim against the recorded Monad sequence:

~~~~~
$ unheaded-replay --trace-id 0x12345678 --packet-id 0x01
[Hop 0] Input Monad: R0=0x00000000 R1=0x00000001 ...
[Hop 0] Output Monad: R0=0x00000002 R1=0x00000001 ...
[Hop 1] Input Monad: R0=0x00000002 R1=0x00000001 ...
...
[Hop N] Output Monad: R0=0x00000123 R1=0x00000456 ...
~~~~~

This enables deterministic debugging and fault diagnosis without live packet capture.

# Checksum

The Monad is protected by a 32-bit checksum computed at Shield (death).  The checksum algorithm is:

~~~~~
checksum = CRC32(Monad || hop_metadata || Trace_ID || Packet_ID)
~~~~~

Where:

- CRC32 is the polynomial 0x1EDC6F41 (Castagnoli, used in iSCSI).
- `Monad || hop_metadata` is the concatenation of the 20-byte Monad register file and any exponent-encoded hop metadata.
- `Trace_ID || Packet_ID` are the 4-byte correlation tokens.

The checksum is computed at the last hop (Shield death) and appended to the Hop-by-Hop option.  Intermediate hops do NOT verify the checksum; only Shield (death) does.

If the checksum fails, the packet is considered corrupted and is discarded.  The application is notified via an error code in the packet hand-off.

# Chaos Injection

## Overview

Chaos injection is an optional feature for resilience testing.  When enabled (via the flags bitfield, bit A), the Shim applies deterministic perturbations to the Monad or Wotan state.

The goal is to test how the application (and the Shim program) handles transient faults:

- Bit flips in registers.
- Value mutations (e.g., increment, saturate).
- Memory access failures.
- Latency increases.

Chaos is applied deterministically: given the same input Monad and the same chaos mode, the same output is produced. This ensures reproducibility and makes debugging possible.

## Chaos Modes

The flags bitfield reserves 5 bits for chaos mode selection:

~~~~~
Bits 5-1 (after the chaos-enable bit):
00000 -> No chaos (or chaos is disabled)
00001 -> Bit flip chaos (flip random bit in R0)
00010 -> Value mutation chaos (increment R0 mod 2^32)
00011 -> Memory fault chaos (all Wotan reads fail)
00100 -> Latency chaos (inflate hop latency by 10x)
00101 -> Loss chaos (drop 10% of subsequent packets)
00110-11111 -> Reserved for future modes
~~~~~

The chaos mode is selected by Shield (birth) based on the deployment configuration or application request.

Example:

~~~~~
flags = 0x80 | (0x01 << 1)  # Chaos enabled, bit flip mode
~~~~~

## Audit Trail

All chaos-induced mutations are recorded in Anamnesis with EVENT_CHAOS type.  The audit trail includes:

- The mode that was applied.
- The input value and the mutated value.
- The hop index and timestamp.

This allows operators to correlate application failures with chaos events.

## Activation

Chaos injection is activated via:

1. **Deployment flag**: `--chaos-mode <mode>` at Unheaded startup.
2. **Per-packet flag**: The application (or Shield) can set the chaos bit in the flags field to enable chaos for a single packet.
3. **Probabilistic activation**: A random percentage of packets can be subjected to chaos (--chaos-rate 0.1 for 10%).

# State Reconciliation

## Pleroma

Pleroma is the complete internal state of the Unheaded system:

- The Monads of all in-flight packets.
- The Wotan ring buffers (per-flow memory state).
- The Wotan WAL (persistent storage).
- The Anamnesis event logs.

Pleroma is authoritative: it is the source of truth for all computation within the limited domain.

## Kenoma

Kenoma is the state of the network observed from outside (by external systems):

- Application requests and responses (as seen by clients).
- Metrics and observability signals (CPU, memory, latency).
- Consensus protocols (e.g., consensus state from an external raft or paxos cluster).

Kenoma is partial: it does not include internal Wotan memory or Monad state.

## Drift Detection

Over time, Pleroma and Kenoma may diverge due to:

- Lost packets (Anamnesis events not delivered to external observers).
- Application crashes or restarts.
- Wotan WAL corruption.
- Network partitions.

Drift is detected via periodic reconciliation:

~~~~~
1. Select a set of (Trace ID, Packet ID) tuples from Anamnesis.
2. Replay their computation via unheaded-replay.
3. Compare the final output Monad to the application-level result (from Kenoma).
4. If mismatch, trigger a recovery action (e.g., rollback, re-execute).
~~~~~

The reconciliation frequency and the sample size are configured at deployment time.

# Information Density

The Unheaded Protocol Foundation achieves high information density by encoding state directly in the packet.

The 20-byte Monad carries:

- **5 registers × 4 bytes = 20 bytes**

The exponent encoding allows compressing metric-rich metadata into the remaining option space:

- Example: 10 fields × 2 bytes (each exponent-encoded) = 20 bytes

Total: 40 bytes of state in a 40+ byte option, with zero separate control messages.

Comparison to traditional approaches:

- **gRPC**: Requires message serialization, deserialization, and separate RPC calls. Overhead: hundreds of bytes, milliseconds of latency.
- **NETCONF**: Similar overhead; designed for device configuration, not packet-level state.
- **In-band telemetry (IPoD/INT)**: Records metadata at each hop but does not provide a computation model. Overhead: tens to hundreds of bytes per packet.
- **Unheaded**: Encodes registers and exponent-encoded metrics inline. Overhead: 20-40 bytes per packet, nanoseconds of per-hop latency.

# Security Considerations

## Extension Header Sanitization

The IPv6 Hop-by-Hop option MUST be sanitized at domain boundaries:

- Packets entering the limited domain from the public Internet MUST have their Hop-by-Hop option replaced (or removed).
- Packets exiting the limited domain to the public Internet MUST have their Hop-by-Hop option removed or rewritten to hide internal state.
- ULA prefixes (Section ULA Prefix) MUST be detected and either accepted (if internal) or rejected (if external).

Failure to sanitize allows attackers to inject malicious Monads or eavesdrop on internal state.

## Containment

The Shim eBPF programs are loaded and verified by the kernel BPF subsystem (verifier), which ensures:

- No out-of-bounds memory access.
- Finite loop execution (bounded by instruction count).
- No kernel function calls (except whitelisted helpers).
- Stack safety (bounded stack depth).

However, the Shim does have access to Wotan memory (ring buffers) and topics. Wotan MUST enforce per-program access control:

- A Shim program can only read/write memory and topics that are explicitly whitelisted in its metadata.
- A Shim program cannot forge or modify packet headers (Wotan prevents this).

## Integrity

The Monad is checksummed at Shield (death) to detect corruption.  However, intermediate hops do NOT verify the checksum; they trust that the Shim programs are correct (verified by the BPF verifier).

To provide stronger integrity guarantees, an optional HMAC-based signature can be added:

~~~~~
signature = HMAC-SHA256(shared_secret, Monad || Trace_ID || Packet_ID)
~~~~~

The signature would be stored in an extended metadata field and verified at Shield (death).  This prevents tampering by untrusted intermediate hops.

Shared secrets would be provisioned via Wotan (stored in the WAL with appropriate ACLs).

## Dictionary Isolation

The Sophia BPF maps (dictionaries) used by the Shim are in kernel space and are not directly accessible by user-space applications.  However, the exponent encoding rules stored in these maps could be a vector for side-channel attacks (e.g., timing attacks via map lookup latency).

To mitigate:

- Use constant-time map lookup implementations (e.g., cuckoo hashing).
- Audit all dictionary operations for timing side channels.
- Monitor map lookup latency and alert on anomalies.

## Chaos Controls

Chaos injection can be abused to cause denial of service:

- An attacker could enable chaos on all packets, causing computation failures.
- An attacker could selectively enable chaos on a subset of packets to trigger application logic bugs.

Mitigation:

- Chaos mode is activated only via operator configuration (--chaos-mode) or explicit application request (setting the chaos bit).
- Per-packet chaos activation is NOT exposed to untrusted applications; it is operator-controlled.
- Anamnesis records all chaos events, allowing operators to audit and detect abuse.

## Trust Boundary

The trust boundary of the Unheaded Protocol Foundation is the limited domain (e.g., a single AS, corporation, or Kubernetes cluster).

Within the limited domain:

- All Shim programs are trusted (loaded by the operator).
- All Wotan instances are trusted.
- All nodes run Unheaded.

At the domain boundary:

- Packets from external sources are untrusted.
- Hop-by-Hop options from external sources are sanitized (removed or rewritten).
- ULA prefixes from external sources are suspicious and should be treated as attacks.

Cross-domain packet routing is not supported by this specification. If packets must cross domain boundaries, they should be encapsulated (e.g., using IP-in-IP or a VPN tunnel) and decapsulated at the boundary, with the Hop-by-Hop option removed and reconstructed if needed.

# IANA Considerations

This memo defines the following IANA registrations:

## IPv6 Hop-by-Hop Option Type

A new IPv6 Hop-by-Hop option type is required:

Type:
: TBD (to be assigned by IANA)

Name:
: Unheaded Monad Option

Change Controller:
: IESG

Reference:
: This document

The option type value should have the high-order two bits set to `00` (skip on unrecognized) and the third bit set to 0 (unchanged on transit), resulting in a format like `00Txxxxx`.  Suggested value: `0x42` (66 in decimal).

## Sophia Dictionary Namespace

Unheaded programs may register Sophia dictionaries (BPF maps) with a namespace registry to avoid collisions.  IANA should create a new registry:

Registry Name:
: Unheaded Sophia Dictionary Namespace

Template:
: Dictionary Name, Program Name, Version, Organization, Contact Email

Suggested allocation policy:
: First Come First Served

This allows operators to coordinate dictionary updates across Unheaded deployments.

## Event Type Registry

Anamnesis event types are expandable.  IANA should create a registry:

Registry Name:
: Unheaded Anamnesis Event Types

Template:
: Event Name, Event Code (u8), Description, RFC/Memo Reference

Current entries:
: EVENT_BORN (0), EVENT_COMPUTED (1), EVENT_WOTAN_READ (2), EVENT_WOTAN_WRITE (3), EVENT_CHAOS (4), EVENT_ROLLBACK (5), EVENT_DIED (6)

Allocation policy:
: Specification Required

--- back

# Heritage

The Unheaded Protocol Foundation builds on historical and contemporary networking research:

- **Packet as Memory**: Inspired by in-band network telemetry (INT) and programmable packet processing (P4, eBPF).
- **Exponent Encoding**: Similar to variable-length integer encoding (VLQ) and IEEE floating-point exponent notation.
- **Hop-by-Hop Metadata**: Extends the IPv6 extension header model (RFC 8200) and Destination Options (RFC 8250).
- **eBPF State Machine**: Builds on the Linux kernel's eBPF virtual machine (XDP, TC hooks) and the BPF instruction set architecture (ISA).
- **Ring Buffer Observability**: Uses the kernel ring buffer infrastructure (perf, BPF ring buffer) for non-blocking event logging.
- **Sophia Dictionaries**: Inspired by embedded key-value stores (LevelDB, RocksDB) adapted for in-kernel use.

The term **Monad** is borrowed from functional programming (Haskell, Scala) to evoke the notion of a computation context that carries state through a series of transforms.

The term **Wotan** (restaurant staff who clears tables and manages supplies) is chosen for metaphorical resonance: Wotan manages memory and I/O resources, freeing the Monad to focus on pure computation.

The term **Anamnesis** (recollection, memory) is chosen to evoke the Platonic notion of recovering truth from latent knowledge; here, it recovers the packet computation trace from the event log.

# BPF Edge Codec

## CUSTOM Override

The BPF Edge Codec subsystem allows programs to override the standard Hop-by-Hop option serialization format with a program-specific (custom) codec.

Rationale:
: Different programs may have different requirements for metadata layout, compression, and field ordering.  Rather than forcing all programs into a single canonical format, the codec subsystem allows each program to define its own serialization rules.

Implementation:
: At Shield (birth), the program's metadata (stored in Sophia under the key `program:<name>:codec`) is consulted.  If a custom codec is defined, it is applied to serialize the Monad and metadata into the Hop-by-Hop option.  At each Shim execution, the codec is applied in reverse (deserialize) to extract the Monad.

Interface:
: A custom codec is a pair of eBPF functions (encode, decode):

~~~~~
struct bpf_edge_codec {
  u32 (*encode)(const struct monad *in, u8 *out, u32 out_len);
  // Returns the number of bytes written to out
  // Returns 0 on error

  u32 (*decode)(const u8 *in, u32 in_len, struct monad *out);
  // Returns the number of bytes read from in
  // Returns 0 on error
};
~~~~~

Example:
: A program that uses 16-bit half-precision floats for latency and loss rate might define a custom codec that packs these fields using FP16 format (IEEE 754) instead of the standard exponent encoding.

Versioning:
: If a program's codec changes (e.g., due to a protocol upgrade), the program name or version must be incremented, and a new codec must be registered.  Backward compatibility is the program's responsibility.

# Computational Completeness

This appendix argues that the Unheaded Protocol Foundation is Turing-complete when paired with Wotan memory.

## The Halting Problem

A computational system is Turing-complete if it can simulate any Turing machine.  The halting problem (can we decide if an algorithm halts?) is undecidable on a Turing-complete system.

The Shim eBPF programs are NOT Turing-complete in isolation:

- The kernel BPF verifier enforces bounded loop execution (bounded instruction count, no unbounded loops).
- The eBPF ISA does not provide unbounded memory access.

However, when the Shim is paired with Wotan (which provides unbounded addressable memory), the system becomes Turing-complete.

## The Shim

The Shim is an eBPF program that:

1. Receives the Monad (5 registers) as input.
2. Reads and writes memory via Wotan (addressable via Flow Label).
3. Returns the updated Monad.

The Shim is compiled from a high-level source language (e.g., C, Rust) via LLVM to eBPF bytecode.  The kernel verifier checks the bytecode for safety but does not restrict the algorithm's expressiveness.

Example Shim: Fibonacci

~~~~~
uint32_t compute_fibonacci(struct monad *m) {
  uint32_t n = m->r0;
  uint32_t a = 0, b = 1;
  for (uint32_t i = 0; i < n; i++) {
    uint32_t temp = a + b;
    a = b;
    b = temp;
  }
  m->r0 = a;
  return 0;
}
~~~~~

This Shim computes the nth Fibonacci number. The loop is bounded by the input n, and the verifier accepts it.

## Wotan as the Memory Model

Wotan provides addressable memory via ring buffers keyed by Flow Label:

~~~~~
uint32_t wotan_read(uint32_t address) {
  // Wotan BPF helper function
  // Reads from the per-flow ring buffer at the given address
  return bpf_wotan_read(address);
}

void wotan_write(uint32_t address, uint32_t value) {
  // Wotan BPF helper function
  // Writes to the per-flow ring buffer at the given address
  bpf_wotan_write(address, value);
}
~~~~~

The per-flow ring buffer is allocated at Shield (birth) and is large enough to hold any reasonable program state (configurable via --ring-size).

With Wotan, the Shim can implement:

1. **Unbounded loops**: The Shim can write a loop counter to memory and read it back, allowing loops that depend on memory state.
2. **Recursive function calls**: Not directly (eBPF does not support function pointers), but via a software stack in Wotan memory.
3. **Arbitrary data structures**: Linked lists, trees, hash tables, etc., all implemented in Wotan memory.

## Proof Sketch

We sketch a proof that Unheaded + Wotan is Turing-complete:

**Claim**: Given any Turing machine M with alphabet A, states Q, and transition function δ, we can construct an Unheaded Shim program that simulates M.

**Proof**:

1. **Tape representation**: The Wotan ring buffer represents the Turing machine's infinite tape.  Address `i` in Wotan memory holds the symbol at tape position `i`.

2. **State representation**: The Monad register R3 holds the current state q ∈ Q.  R4 holds the head position (tape offset).

3. **Transition function**: The Shim implements δ as a lookup table (or a decision tree):
   ~~~~~
   current_symbol = wotan_read(R4);        // Read tape at head
   next_action = lookup_transition(R3, current_symbol);
   write_symbol = get_write_symbol(next_action);
   next_state = get_next_state(next_action);
   head_offset = get_head_move(next_action);

   wotan_write(R4, write_symbol);           // Write tape
   R3 = next_state;                          // Update state
   R4 = R4 + head_offset;                    // Move head
   ~~~~~

4. **Iteration**: The packet's Trace ID acts as a "time step." Each Shim execution performs one transition. By repeatedly injecting packets (via Shield or an external loop), the Turing machine is simulated step-by-step.

5. **Halting**: The Shim can check if R3 reaches a halt state and, if so, return 0 (no further computation).  Anamnesis records the sequence of states and can be used to recover the final output.

**Conclusion**: The Unheaded + Wotan system can simulate any Turing machine; therefore, it is Turing-complete.

**Note**: The system is subject to practical constraints (packet lifetime, ring buffer size), but these are resource limits, not fundamental limitations.

## Memory Model {#memory-model}

The Monad carries registers.  Wotan carries memory.  This
separation keeps the Monad fixed at 20 bytes (pure compute bus)
while allowing memory to scale independently via Wotan's
configurable ring buffer allocation.

Program memory (ROM equivalent):
: A Sophia BPF array map holds the assembled program as an
  array of instruction words.  The PC indexes into this map.
  ROM stays in BPF maps because it is read-only and benefits
  from O(1) kernel-space access at every hop.

Data memory (RAM equivalent):
: A dedicated Wotan ring buffer channel, keyed by Flow Label,
  provides per-flow addressable memory.  The eBPF program at
  each hop reads and writes memory through the local BPF map
  cache, which Wotan pre-stages from the ring buffer before
  the packet arrives.  Memory allocation is controlled by the
  --ring-size argument to Wotan, allowing operators to scale
  from kilobytes (embedded use) to gigabytes (Doom) without
  changing the protocol or the eBPF programs.

Persistent storage (disk equivalent):
: Wotan's Write-Ahead Log (WAL) provides durable storage.
  Memory pages that have not been accessed within a
  configurable TTL are flushed from the ring buffer to the
  WAL.  On access, Wotan promotes them back to the ring
  buffer.  This is the swap partition of the mapped data bus.

Memory-mapped I/O:
: Designated address ranges in the Wotan memory channel
  map to I/O topics that Wotan bridges to external
  interfaces.  A write to a screen-region address publishes
  to a screen topic that Wotan renders to the dashboard.
  A read from an input-region address retrieves the latest
  value from an input topic written by an external source
  (keyboard, sensor, network).

~~~~~
Memory hierarchy:

  L0  Monad Scratch[0..3]         4 bytes    wire speed
      (CPU registers, travel with the packet)

  L1  Per-hop BPF map cache       variable   ~100-200 ns
      (prefetched by Wotan before packet arrives)

  L2  Wotan ring buffer          --ring-size  ~1-10 us
      (in-memory, per-flow, configurable)

  L3  Wotan WAL                  disk       ~100 us - 1 ms
      (persistent, durable, swappable)

  L4  Sophia dictionaries         BPF maps   ~100-200 ns
      (instruction decode only, not data memory)

  Address space layout (configurable per program):

  0x00000000 +------------------+
             |  Data Memory     |  <- Wotan ring buffer channel
             |  (general RAM)   |     per-flow, keyed by Flow Label
  0x0000BFFF +------------------+
  0x0000C000 |  Screen / Output |  <- Wotan I/O topic
             |  (I/O region)    |     Wotan reads -> dashboard
  0x0000FFFE +------------------+
  0x0000FFFF |  Input (1 word)  |  <- Wotan I/O topic
             |  (keyboard/etc)  |     Wotan writes <- external
             +------------------+
~~~~~
{: #fig-memory-layout title="Memory Hierarchy and Address Space"}

# Acknowledgments

This document is the output of a thought experiment in programmable packet processing and emerged from conversations on functional programming, observability, and the boundaries between the network and the application.

Special thanks to the Linux kernel eBPF community (Alexei Starovoitov, Daniel Borkmann, and others) for creating the infrastructure that made this design possible.
