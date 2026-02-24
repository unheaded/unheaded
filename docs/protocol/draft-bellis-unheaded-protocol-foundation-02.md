---
title: "The Unheaded Protocol Foundation: A Mapped Data Bus over IPv6 Hop-by-Hop Options"
abbrev: "Unheaded Protocol Foundation"
docname: draft-bellis-unheaded-protocol-foundation-02
category: exp
ipr: trust200902
area: Internet
workgroup: Independent Submission
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
  - post-quantum-cryptography
  - address-reclamation

author:
  - ins: S. Bellis
    name: Stevie Bellis
    org: Unheaded
    email: stevie@bellis.tech
    country: US

pi:
  toc: yes
  symrefs: yes
  sortrefs: yes
  compact: yes
  subcompact: no

normative:
  RFC2119:
  RFC8174:
  RFC8200:
  RFC9673:

informative:
  RFC1918:
  RFC4193:
  RFC8300:
  RFC8754:
  RFC8799:
  RFC9098:
  RFC9180:
  RFC9288:
  RFC9370:
  RFC9486:
  RFC9631:
  RFC9669:
  FIPS203:
    title: "Module-Lattice-Based Key-Encapsulation Mechanism Standard"
    author:
      org: National Institute of Standards and Technology
    date: 2024-08
    target: https://csrc.nist.gov/pubs/fips/203/final
  FIPS204:
    title: "Module-Lattice-Based Digital Signature Standard"
    author:
      org: National Institute of Standards and Technology
    date: 2024-08
    target: https://csrc.nist.gov/pubs/fips/204/final
  FIPS205:
    title: "Stateless Hash-Based Digital Signature Standard"
    author:
      org: National Institute of Standards and Technology
    date: 2024-08
    target: https://csrc.nist.gov/pubs/fips/205/final

--- abstract

The Unheaded Protocol Foundation defines a mapped data bus model
that transforms IPv6 packets into addressable memory by encoding a
small register file directly in the IPv6 Hop-by-Hop Options
extension header.

We introduce a 20-byte Monad (register file) that carries program
state through the network.  At each hop, an eBPF program (the
Shim) performs computation on the Monad.  The packet itself becomes
the working memory, using exponent-encoded fields to pack rich
metadata into the IPv6 option while remaining fully backward-
compatible.

To support programs larger than what fits in a single Monad, we
introduce Wotan, a memory and I/O bus that bridges Monad
computation to per-flow ring-buffer storage and external topics.
This decouples the Monad (pure, 20-byte compute) from memory
(Wotan's configurable data planes).

This memo extends the packet format with two additional
capabilities: (1) Kingdom Mode Address Reclamation, which recovers
up to 224 bits of deterministic address space from IPv6
addresses within a controlled L2 fabric for use as extended
computational and cryptographic registers; and (2) Post-Quantum
Cryptographic Identity Binding, which cryptographically binds each
service identifier in the Monad to a post-quantum keypair via the
Sophia dictionary system, providing quantum-resistant authentication
of per-packet metadata without increasing wire overhead.

This memo defines the packet format, exponent-encoding scheme,
per-hop processing semantics, address reclamation model, post-
quantum identity binding, optional chaos injection for resilience
testing, and the complete computational model (Turing-complete with
memory paging).

--- middle

# Introduction

## Problem Statement

Classical networking separates computation from data.
Computation happens in applications.  Data flows through the
network as opaque byte streams.  Between them lies an expensive
impedance mismatch of serialization, deserialization, protocol
translation, middleware, sidecars, and proxies.

The Unheaded Protocol Foundation inverts this model: the packet
IS the memory.  A 20-byte register file (the Monad) is written
and read by eBPF programs at each hop.  The packet itself is the
working storage of a distributed computation that executes at
wire speed across every node in the path.

Within a Limited Domain [RFC8799] — a single AS, corporation, or
container fleet where every hop is operator-controlled — the
protocol provides:

-  Zero-copy pass-through: Application state travels with the
   packet; no serialization boundaries.

-  Hop-local compute: Each hop reads the latest Monad state
   without network round-trips.

-  Deterministic causality: Events are timestamped in packet
   traversal order, not wall-clock order.

-  Quantum-resistant identity: Service identifiers are
   cryptographically bound to post-quantum keypairs.

-  Address reclamation: Deterministic address prefix bits
   within the Limited Domain are reclaimed as extended
   computational registers.

## Terminology

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL
NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described
in BCP 14 [RFC2119] [RFC8174] when, and only when, they appear
in all capitals, as shown here.

This document uses the following terms:

Monad:
: The 20-byte register file embedded in the IPv6 Hop-by-Hop
  option header.  It travels with the packet.

Shim:
: The stateless eBPF program that runs at each hop, reading
  the Monad from the packet, performing computation, and writing
  back to the packet.

Shield:
: The packet boundary — the first and last hop logic that
  stamps the packet into existence (ingress) and commits it
  (egress).

Wotan:
: The memory and I/O bus, providing per-flow ring buffers,
  persistent storage (WAL), and bridging to external topics.

Anamnesis:
: The non-blocking event log, recording packet events for
  observability and historical replay via ring buffers.

Sophia:
: The exponent dictionary system.  BPF maps in kernel space,
  structured tables in userspace.  Hot-swappable vocabulary
  that gives meaning to raw exponent bytes.

Exponent Encoding:
: A compositional semantic for packing metadata: each field
  stores a signed exponent, and the actual value is
  reconstructed as base^exponent * multiplier.

Limited Domain:
: A network boundary (typically a single AS, corporation, or
  container fleet) where the Unheaded Protocol is deployed
  end-to-end.  All hops within the domain are operator-
  controlled.  Defined in [RFC8799].

Kingdom Mode:
: An operational mode within a Limited Domain where the IPv6
  ULA prefix is known a priori, enabling deterministic address
  prefix bits to be reclaimed as extended computational space.

Pleroma:
: The complete internal desired state of the system.

Kenoma:
: The observed actual state of the network.

# The Mapped Data Bus Model

## The Packet Is the Memory

The Monad (register file) is written and read by eBPF programs
at each hop.  The packet itself is the working storage.

~~~~~
Traditional model:
  [App] -> serialize -> [packet] -> deserialize -> [App]
  State lives in application memory.  Packets are dumb pipes.

Mapped data bus model:
  [Shield: birth] -> [Hop 1: compute] -> [Hop 2: compute] ->
  ... -> [Hop N: compute] -> [Shield: death]
  Registers live in the packet.  The network is the compute bus.
  Memory lives in Wotan ring buffers.  Anamnesis is the trace log.
~~~~~

## Design Principles

1.  Minimalism in the packet: The Monad is 20 bytes.  Rich
    metadata is encoded via exponent notation.

2.  Separation of concerns: Registers (Monad) are compute;
    memory (Wotan) is I/O.

3.  Non-blocking observability: Anamnesis records events without
    blocking the Shim.

4.  Backward compatibility: The Hop-by-Hop option is processed
    per [RFC8200] and [RFC9673].  Unaware routers skip the option.

5.  Cryptographic identity: Service identifiers are bound to
    post-quantum keypairs via Sophia.

## Architecture Layers

~~~~~
Layer 0 - The Protocol:
  IPv6 packets with Hop-by-Hop extension headers.
  Exponent-encoded key-value metadata.
  Sophia dictionaries compiled to BPF maps.
  The atom.  The wire itself.
  Speed: light.

Layer 1 - The Void:
  eBPF programs at XDP, TC, kprobe, tracepoint hooks.
  Per-hop compute.  Each program is a CPU core in a
  distributed computer.
  Speed: nanoseconds.

Layer 2 - Wotan (The Central Core):
  Encode/decode bridge.  Sophia table lookups.
  Reads ring buffers UP from the Void.
  Writes BPF maps DOWN to the Void.
  The Rosetta Stone.  The nervous system.
  Speed: microseconds.

Layer 3 - The Kingdom:
  Go services, REST, gRPC, WebSocket, dashboards.
  Human-speed interfaces for human-speed decisions.
  Speed: milliseconds.
~~~~~

# IPv6 Hop-by-Hop Option Format

## Header

The IPv6 Hop-by-Hop Options extension header (Next Header = 0)
carries the Monad in a single option TLV.

~~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
|                     Option TLV(s)                             |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~~

The Hop-by-Hop extension header precedes all other extension
headers.  It is processed at each hop per [RFC8200] Section 4.3
and [RFC9673].

## Option Type

~~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Type = TBD   |   Len = 20+   |                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
|                     Monad (20 bytes)                          |
|                                                               |
|                                                               |
|                                                               |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Optional: Metadata, Kingdom Flags, Chaos payload            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~~

Type:
: To be assigned by IANA.  The high-order two bits MUST be
  00 (skip on unrecognized) and the third bit MUST be 0
  (option data does not change en-route), yielding a format
  00Txxxxx.  This ensures backward compatibility: unaware
  routers skip the option per [RFC8200].

Len:
: The length of the option payload in octets (not including
  the Type and Len octets).  Minimum value is 20 (Monad
  only); maximum is 255.

## Monad Register File

The Monad is a 20-byte register file organized as five 32-bit
words:

~~~~~
Offset  Name    Type    Purpose
------  ------  ------  --------------------------------
 0x00   R0      u32     Accumulator
 0x04   R1      u32     Argument / secondary value
 0x08   R2      u32     Result / output
 0x0C   R3      u32     Counter / loop variable
 0x10   R4      u32     Caller ID / error code
------  ------  ------  --------------------------------
TOTAL:  20 bytes        5 registers x 32 bits = 160 bits
~~~~~

All fields are in network byte order (big-endian).  The
semantics of each register are program-defined; the protocol
does not prescribe their use.

## Flags Bitfield

Following the Monad, a 1-byte flags field controls behavior:

~~~~~
 7   6   5   4   3   2   1   0
+---+---+---+---+---+---+---+---+
| C | Y | T | E | S | M | K | R |
+---+---+---+---+---+---+---+---+

C (0x80): Chaos injection active (Yaldabaoth)
Y (0x40): Canary deployment path
T (0x20): Full trace active (all hops emit to Anamnesis)
E (0x10): Payload encrypted (intra-Kingdom TLS)
S (0x08): Statistically sampled
M (0x04): Mirror copy (not original)
K1 (0x02): Kingdom Mode selector (high bit)
K0 (0x01): Kingdom Mode selector (low bit)

Kingdom Mode selector (K1:K0):
  00 = Default IPv6 (no address reclamation)
  01 = /8 mode  (10.0.0.0/8 equivalent, 24-bit host)
  10 = /12 mode (172.16.0.0/12 equivalent, 20-bit host)
  11 = /16 mode (192.168.0.0/16 equivalent, 16-bit host)
~~~~~

## Trace Correlation

~~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                 Trace ID (32 bits)                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~~

Trace ID:
: A correlation token shared by all packets in a logical
  flow.  Set by Shield at packet birth.  Anamnesis uses
  (Trace ID, Flow Label) as the composite key for event
  recording.

## Checksum

~~~~~
checksum = CRC-16/CCITT(Monad || metadata || Trace_ID)
~~~~~

Polynomial: x^16 + x^12 + x^5 + 1 (0x1021).  Initial value:
0xFFFF.  Covers the 20-byte Monad, all metadata fields, and
the 4-byte Trace ID.

Detection capability: 100% of all single-bit, double-bit, and
odd-number-of-bit errors; 100% of burst errors up to 16 bits.
Computation cost: approximately 50 ns for 20 bytes on modern
x86-64.

# Exponent Encoding

## Overview

Exponent encoding is a compositional scheme for representing
rich metadata in compact form:

~~~~~
value = base^exponent * multiplier
~~~~~

Where base is a constant (typically 2), exponent is a signed
byte (-128 to +127), and multiplier is a scaling factor.

## Compositional Semantics (Sophia)

Sophia dictionaries are trees, not tables.  Each byte narrows
the context for the next:

~~~~~
byte_0 = 0x01 -> sophia_root -> dict_id = 1 (service_identity)
byte_1 = 0x03 -> dict_1      -> "architect"

The SAME byte_1 value in DIFFERENT contexts:
[0x01, 0x03] -> service = "architect"
[0x02, 0x03] -> action  = "sample"
[0x03, 0x03] -> qos     = "realtime"
~~~~~

With K key positions, the expressible meaning space is:

~~~~~
M = 256^K

K=1:  256 meanings           (8 bits)
K=2:  65,536 meanings        (16 bits)
K=3:  16,777,216 meanings    (24 bits)
K=8:  1.844 x 10^19 meanings (64 bits)
~~~~~

## BPF Map Implementation

Sophia dictionaries are stored as BPF hash maps in kernel
space.  Each lookup is O(1).  A two-level lookup (root map
to sub-dictionary) costs two hash table hits — still
nanoseconds.  Dictionary updates propagate cluster-wide in
under 10 ms via atomic BPF map replacement through Wotan.

# Kingdom Mode: Address Reclamation

## Motivation

Within a Limited Domain [RFC8799], the IPv6 source and
destination addresses use Unique Local Addresses (ULA)
[RFC4193] or IPv4-mapped IPv6 addresses that share a common,
operator-assigned prefix.  The prefix bits are deterministic —
they carry zero information because every node in the domain
knows them a priori.

Kingdom Mode reclaims these deterministic prefix bits as
extended computational and cryptographic register space,
dramatically increasing the Monad's effective bit budget
without adding any bytes to the packet.

## Applicability

Kingdom Mode is OPTIONAL.  It provides additional
computational and cryptographic register space for
deployments that benefit from it.  Deployments that do
not require the Extended Register Space operate normally
with the 20-byte Monad and standard IPv6 addressing.

## Address Space Analysis

An IPv6 packet carries two 128-bit addresses (source and
destination) for a total of 256 address bits.  Within a
Limited Domain, IPv6 is the wire protocol, but the actual
addressing requirements are bounded by the Kingdom's fleet
size — typically equivalent to a single RFC 1918 [RFC1918]
private address block.

A single Unheaded Kingdom will never need more than
16,777,216 addresses (equivalent to 10.0.0.0/8).  Many
user Kingdoms require far fewer — a /24 (256 hosts)
or /16 (65,536 hosts).  The fewer hosts required, the
more address bits become deterministic and reclaimable.

Within a Limited Domain operating on an L2 overlay (e.g.,
EVPN-VXLAN), the inner IPv6 addresses are not used for
Layer 3 routing — the outer encapsulation handles
forwarding.  The inner addresses serve only as node
identifiers.  Therefore, every bit beyond the host
identifier is deterministic (known to all Kingdom nodes)
and carries zero information.

The reclamation formula is:

~~~~~
  Reclaimed bits = 2 * (128 - host_bits)
~~~~~

Where host_bits is the number of bits required to
uniquely identify each node in the fleet.  The factor
of 2 accounts for both source and destination addresses.

The following table shows reclaimed space by fleet size:

~~~~~
Fleet Size    Host Bits    Free Bits     Reclaimed     Reclaimed
(hosts)       (per addr)   (per addr)    (both addrs)  Bytes
----------    ----------   ----------    -----------   ---------
256           8            120           240           30 bytes
4,096         12           116           232           29 bytes
65,536        16           112           224           28 bytes
1,048,576     20           108           216           27 bytes
16,777,216    24           104           208           26 bytes
~~~~~

The minimum case (256 hosts, 8-bit addressing) yields
240 reclaimed bits (30 bytes) of Extended Register Space
— more than doubling the Monad's effective register
budget from 20 bytes to 50 bytes, with zero additional
wire overhead.  Even the maximum case (16.7M hosts, full
10/8 equivalent) yields 208 reclaimed bits (26 bytes).

Note: In a routed Kingdom without L2 overlay, the ULA
prefix bits must be preserved for Layer 3 forwarding.
The reclaimed space is reduced to:

~~~~~
  Reclaimed bits (routed) = 2 * (128 - prefix_len - host_bits)
~~~~~

For example, with a /48 ULA prefix and 24-bit host
addressing: 2 * (128 - 48 - 24) = 112 reclaimed bits.
L2 overlay (EVPN-VXLAN) is RECOMMENDED for Kingdom Mode
deployments to maximize the Extended Register Space.

## Kingdom Mode Selector

Bits 1-0 (K1:K0) of the flags bitfield (Section 3.5)
select the Kingdom Mode.  The 2-bit field maps directly
to the three RFC 1918 [RFC1918] private address block
sizes, plus a default mode:

~~~~~
K1:K0  Mode     Host Bits  Equivalent          Reclaimed
                (per addr) RFC 1918 Block       (both addrs)
-----  -------  ---------  -----------------   -----------
  00   Default  N/A        (standard IPv6)     0 bits
  01   /8       24         10.0.0.0/8          208 bits
  10   /12      20         172.16.0.0/12       216 bits
  11   /16      16         192.168.0.0/16      224 bits
~~~~~

Mode 00 (Default):
: Standard IPv6 addressing.  No reclamation.  The Monad
  provides 160 bits of register state.  This is the
  baseline for all deployments.

Mode 01 (/8):
: Large Kingdoms (up to 16,777,216 hosts).  Each address
  reserves 24 bits for host addressing.  The remaining
  104 bits per address (208 bits total) are reclaimed as
  Extended Register Space.

Mode 10 (/12):
: Medium Kingdoms (up to 1,048,576 hosts).  Each address
  reserves 20 bits for host addressing.  108 bits per
  address (216 bits total) are reclaimed.

Mode 11 (/16):
: Small to medium Kingdoms (up to 65,536 hosts).  Each
  address reserves 16 bits for host addressing.  112 bits
  per address (224 bits total) are reclaimed.

When K1:K0 is non-zero:

1.  The source and destination IPv6 addresses MUST use the
    Kingdom's ULA prefix.

2.  The Shim at each hop MAY interpret all bits beyond
    the host identifier in the source and destination
    addresses as Extended Register Space (ERS).

3.  Shield (ingress) MUST populate the ERS bits according
    to the Kingdom Mode layout defined in the active Sophia
    dictionary.

4.  Shield (egress) MUST restore standard ULA formatting
    before any packet exits the Limited Domain.

Operators whose Kingdoms require fewer hosts than the
selected mode provides (e.g., 256 hosts using /16 mode)
gain additional unused host bits that MAY be repurposed
as ERS at the operator's discretion, further increasing
the reclaimed bit budget.

## Extended Register Space Layout

When Kingdom Mode is active on an L2 overlay, the entire
IPv6 address beyond the host identifier is available as
Extended Register Space.  The ULA prefix bits are
deterministic (known to all Kingdom nodes via Sophia)
and carry zero information — they are overwritten with
register data in transit and restored by Shield at egress.

~~~~~
 0                                                              127
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Extended Registers (Source)                   | Host ID (src) |
|  (128 - H reclaimed bits)                     | (H bits)      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

Source Address (128 bits):
  Bits [127..H]   = Extended Source Registers (reclaimed)
  Bits [H-1..0]   = Source Host ID (actual addressing)

Destination Address (128 bits):
  Bits [127..H]   = Extended Destination Registers (reclaimed)
  Bits [H-1..0]   = Destination Host ID (actual addressing)

Where H = host_bits per the Kingdom Mode selector:
  /8 mode:  H = 24  →  104 ERS bits per address
  /12 mode: H = 20  →  108 ERS bits per address
  /16 mode: H = 16  →  112 ERS bits per address
~~~~~

Example: /16 mode (K1:K0 = 11), 65,536-host fleet:

~~~~~
Source Address:
  Bits [127..16]  = 112 bits of Extended Source Registers
  Bits [15..0]    = 16-bit Source Host ID

Destination Address:
  Bits [127..16]  = 112 bits of Extended Dest Registers
  Bits [15..0]    = 16-bit Destination Host ID

Total Extended Register Space: 224 bits (112 + 112)
                             = 28 bytes
~~~~~

In a routed Kingdom (no L2 overlay), the ULA prefix
MUST be preserved for Layer 3 forwarding.  The ERS
is reduced to the bits between the prefix and host ID:

~~~~~
Source Address (routed mode):
  Bits [127..P]   = ULA Prefix (P bits, MUST preserve)
  Bits [P-1..H]   = Extended Source Registers (reduced)
  Bits [H-1..0]   = Source Host ID
~~~~~

## Extended Register Semantics

The Extended Register bits are program-defined and mapped
through Sophia dictionaries, just as the Monad registers are.
Recommended allocations for the 224-bit ERS (/16 mode on
L2 overlay, 112 bits per address):

~~~~~
ERS Field               Bits  Location          Purpose
---------------------   ----  ----------------  ---------------------
PQC Key Epoch (src)      8    src[127..120]     Key rotation counter
PQC Fingerprint (src)   32    src[119..88]      SHA3-256[0:32] of
                                                 src service pubkey
Flow Sequence Number    16    src[87..72]       Per-flow monotonic
                                                 sequence counter
Sophia Dict Version      8    src[71..64]       Active dictionary
                                                 version at stamp time
Extended QoS (src)       8    src[63..56]       Fine-grained QoS
Src Reserved            40    src[55..16]       Future use (Age 2+)

PQC Key Epoch (dst)      8    dst[127..120]     Key rotation counter
PQC Fingerprint (dst)   32    dst[119..88]      SHA3-256[0:32] of
                                                 dst service pubkey
Deterministic Latency   16    dst[87..72]       DetNet latency
                                                 budget (microseconds)
Extended Flow Action     8    dst[71..64]       Additional flow
                                                 directives
Dst Reserved            48    dst[63..16]       Future use (Age 2+)

---------------------   ----  ----------------  ---------------------
TOTAL ALLOCATED:       224 bits (of 224 available in /16 mode)
  Source ERS:          112 bits (72 allocated + 40 reserved)
  Dest ERS:           112 bits (64 allocated + 48 reserved)
~~~~~

Implementations MAY define alternative layouts via Sophia
dictionary entries keyed by program name.  The layout above
is RECOMMENDED for general-purpose /16 mode deployments.
Kingdoms using /8 or /12 modes have fewer ERS bits per
address (104 or 108 respectively) and SHOULD adjust
allocations accordingly, reducing reserved space first.

## Interaction with IPv6-Mapped IPv4

For Kingdom deployments that operate IPv4 internally
(per [RFC1918]), the IPv4 addresses can be mapped to
IPv6 using the ::ffff:0:0/96 prefix.  In this mode:

~~~~~
IPv4 Address    IPv4 Fixed Bits   IPv6 Mapped Free Bits
                                  (per addr, in 128-bit
                                   IPv6 representation)
-----------     ---------------   ---------------------
10.0.0.0/8           8            128 - 96 - 8  = 24
172.16.0.0/12       12            128 - 96 - 12 = 20
192.168.0.0/16      16            128 - 96 - 16 = 16
~~~~~

However, the 96-bit ::ffff:0:0 prefix is fixed, leaving
only the 32-bit IPv4 address portion for reclamation.
Native IPv6 ULA addressing (Section 5.3) provides
substantially more reclaimed bits and is RECOMMENDED for
Kingdom Mode deployments.

## Security Considerations for Kingdom Mode

The Extended Register bits are only meaningful within the
Limited Domain.  Shield MUST:

-  Strip or zero all Extended Register bits on egress
   before any packet leaves the Limited Domain.

-  Reject any ingress packet from outside the Limited
   Domain that claims Kingdom Mode (K flag set) or
   carries a Kingdom ULA prefix.

-  Validate that the ULA prefix matches the Kingdom's
   configured prefix at every hop when K flag is set.

Failure to enforce these rules would allow an external
attacker to inject forged Extended Register values.

# Post-Quantum Cryptographic Identity Binding

## Motivation

The Monad's src_service_id and dst_service_id fields identify
which services originated and should receive a packet.  In a
standard deployment, these are Sophia dictionary lookups —
opaque integers mapped to service names.

Post-Quantum Cryptographic (PQC) Identity Binding extends
this model by cryptographically binding each service_id to a
post-quantum keypair.  Every packet carrying src_service_id =
0x42 implicitly asserts: "this packet was stamped by Shield
on behalf of the service holding the private key for
service 0x42."

This provides:

-  Quantum-resistant authentication of per-packet metadata.

-  Zero additional wire overhead — the binding lives in
   Sophia, not in the packet.

-  Defense against "harvest now, decrypt later" attacks
   on intra-domain traffic metadata.

-  Cryptographic non-repudiation of service identity at
   every hop.

## Sophia Key Store

The Sophia dictionary is extended with cryptographic key
material for each service_id:

~~~~~
Sophia Entry for service_id 0x42:

  service_name:       "kanban-app"
  endpoint:           "fd00:3f:75::1007:8080"
  pqc_algorithm:      ML-KEM-768       (FIPS 203)
  pqc_pubkey:         <1184 bytes>     (ML-KEM-768 public key)
  pqc_fingerprint:    <32 bytes>       (SHA3-256 of pubkey)
  classical_pubkey:   <32 bytes>       (X25519, for hybrid mode)
  hybrid_mode:        CONCATENATE      (PQ/T hybrid per RFC 9370)
  key_epoch:          7                (rotation counter)
  key_issued:         2026-02-18T...   (issuance timestamp)
  key_expires:        2026-03-18T...   (expiration timestamp)
  signature_algo:     ML-DSA-65        (FIPS 204)
  signature_pubkey:   <1952 bytes>     (ML-DSA-65 public key)
~~~~~

The public keys are stored in Sophia's userspace tables
and, where space permits, cached in BPF maps as
fingerprints (32-byte SHA3-256 truncations).

## Key Distribution

Key distribution uses the Monad's existing flow_action
field to signal key lifecycle events:

~~~~~
flow_action    Meaning
-----------    ------------------------------------
0x01           NORMAL (data packet)
0x10           KEY_ANNOUNCE (new service pubkey)
0x11           KEY_ROTATE (epoch increment)
0x12           KEY_REVOKE (emergency revocation)
0x13           KEM_ENCAPS (ML-KEM encapsulation)
0x14           KEM_DECAPS (ML-KEM decapsulation)
~~~~~

When flow_action is KEY_ANNOUNCE, KEY_ROTATE, or KEY_REVOKE,
the packet payload carries the relevant key material.  The
Monad's R0-R4 registers carry control metadata (key epoch,
fingerprint prefix, etc.).  Shield MUST enforce that
KEY_ANNOUNCE packets originate only from authorized
provisioning nodes.

Wotan handles the actual key exchange ceremony:

1.  Provisioning node generates ML-KEM-768 keypair.

2.  Provisioning node sends KEY_ANNOUNCE packet through
    Shield with the public key as payload.

3.  Shield stamps the packet with the new service_id and
    a KEY_ANNOUNCE flow_action.

4.  Wotan receives the ring buffer event, updates Sophia
    userspace tables, and pushes the fingerprint to BPF
    maps on all Kingdom nodes.

5.  Subsequent packets carrying that service_id are now
    cryptographically bound to the announced keypair.

## PQ/T Hybrid Mode

During the transition period before Cryptographically
Relevant Quantum Computers (CRQCs) are available, the
Sophia key store supports Post-Quantum/Traditional (PQ/T)
hybrid mode per the approach described in [RFC9370]:

~~~~~
Sophia Hybrid Entry:

  classical_key:  X25519 public key (32 bytes)
  pqc_key:        ML-KEM-768 public key (1184 bytes)
  hybrid_mode:    CONCATENATE | PQC_ONLY | CLASSICAL_ONLY
~~~~~

In CONCATENATE mode, both the classical and PQC key
exchanges must succeed for authentication.  This ensures
security against both classical and quantum adversaries.

Transitioning from CONCATENATE to PQC_ONLY is a Sophia
dictionary update — a single BPF map write propagated
by Wotan in under 10 ms.  Zero packet format changes.
Zero downtime.

## Per-Hop Fingerprint Verification

When Kingdom Mode (Section 5) is active and the Extended
Register Space carries PQC fingerprints (Section 5.6),
each hop MAY verify the fingerprint:

1.  Extract PQC Fingerprint (src) from Extended Registers
    (32 bits = truncated SHA3-256).

2.  Look up the full fingerprint for src_service_id in
    the local Sophia BPF map.

3.  Compare the 32-bit truncation.

4.  If mismatch: emit ANOMALY event to Anamnesis, set
    flow_action = DROP (configurable).

This provides per-hop authentication at O(1) cost per
packet.  The full cryptographic verification (ML-DSA-65
signature check) is performed only at Shield boundaries,
where computational cost is amortized.

## Security Considerations for PQC Binding

Key material in Sophia userspace tables MUST be protected
by operating system access controls.  BPF maps containing
fingerprints are in kernel space and are not directly
accessible by user-space applications.

Key rotation MUST be performed before key_expires.  If a
key epoch mismatch is detected (packet carries epoch N,
Sophia has epoch N+1), the Shim SHOULD log a warning but
MUST NOT drop the packet during the grace period
(configurable, default: 60 seconds after rotation).

Private keys MUST NOT be stored in Sophia or any BPF map.
Private keys reside only in the provisioning node's secure
enclave or HSM.

Side-channel mitigation: BPF map lookups for fingerprint
comparison MUST use constant-time comparison functions to
prevent timing attacks.

# Shield

## Ingress

At ingress, Shield performs:

1.  Receive packet from outside the Limited Domain.

2.  Apply WAF checks (blocklist, rate limit, geo).

3.  Construct Hop-by-Hop extension header with Monad:
    initialize R0-R4 per program, set metadata fields,
    set flags, assign Trace ID.

4.  If Kingdom Mode: populate Extended Register Space
    in source and destination addresses (PQC fingerprints,
    key epochs, sequence numbers, Sophia version).

5.  Compute CRC-16 checksum over Monad and metadata.

6.  Update IPv6 Next Header to 0 (Hop-by-Hop).

7.  Emit BIRTH event to Anamnesis ring buffer.

8.  Forward packet into the Limited Domain.

## Egress

At egress, Shield performs:

1.  Parse Hop-by-Hop extension header; extract Monad.

2.  Verify CRC-16 checksum.  If failed: discard, log.

3.  If Kingdom Mode: zero all Extended Register bits,
    restore standard ULA formatting.

4.  Emit DEATH event to Anamnesis (full Monad snapshot,
    final state after all hops).

5.  Strip Hop-by-Hop extension header.

6.  Restore original IPv6 Next Header.

7.  Forward clean IPv6 packet out of the Limited Domain.

## Lifecycle

~~~~~
[Shield birth] -> [Shim 1] -> [Shim 2] -> ... -> [Shield death]
      |               |           |                     |
      +-> BIRTH       +-> COMPUTED events               +-> DEATH
          event           (per-hop)                         event
~~~~~

# Per-Hop Processing

At each hop within the Limited Domain:

1.  Parse Hop-by-Hop option; extract Monad and metadata.

2.  Verify CRC-16 checksum.

3.  If Kingdom Mode (K flag set): extract Extended
    Registers from source/destination addresses.

4.  Optionally verify PQC fingerprint against Sophia.

5.  Look up Shim program via Sophia (keyed by program
    name or default).

6.  Execute Shim eBPF program with Monad as input.
    The Shim reads/writes Wotan memory via BPF helpers.

7.  Write updated Monad back to packet.

8.  Increment hop_count.  Update metadata.

9.  Recompute CRC-16 checksum.

10. Emit event to Anamnesis (non-blocking ring buffer
    write).

11. Forward packet to next hop.

All per-hop processing MUST be stateless and deterministic
given the input Monad.  External state is accessed only
through Wotan.

# Anamnesis

## Event Structure

~~~~~
struct anamnesis_event {
  u64 timestamp_ns;        // bpf_ktime_get_ns()
  u8  event_type;          // BIRTH=0, COMPUTED=1, DEATH=6, etc.
  u8  hop_index;
  u16 reserved;
  u32 input_monad[5];      // 20 bytes: Monad before Shim
  u32 output_monad[5];     // 20 bytes: Monad after Shim
  u32 trace_id;
  u32 wotan_addr;          // Wotan memory access (if any)
  u32 wotan_value;         // Value read/written (if any)
};
// Total: 64 bytes per event
~~~~~

## Event Types

~~~~~
Code   Name            Description
----   --------------  ---------------------------------
0x00   EVENT_BORN      Packet created at Shield (birth)
0x01   EVENT_COMPUTED  Shim executed, Monad updated
0x02   EVENT_WOTAN_RD  Wotan ring-buffer read
0x03   EVENT_WOTAN_WR  Wotan ring-buffer write
0x04   EVENT_CHAOS     Chaos mode applied
0x05   EVENT_ROLLBACK  Monad rolled back
0x06   EVENT_DIED      Packet reached Shield (death)
0x07   EVENT_KEY_OP    PQC key lifecycle event
0x08   EVENT_ANOMALY   Fingerprint mismatch / integrity
~~~~~

## Sampling

Anamnesis supports adaptive sampling via Sophia:

~~~~~
qos_class = realtime    -> always trace    (S = 1.0)
qos_class = interactive -> 10% sample      (S = 0.1)
qos_class = bulk        -> 1% sample       (S = 0.01)
flags & CHAOS           -> always trace    (audit trail)
flags & KEY_OP          -> always trace    (key lifecycle)
~~~~~

# Wotan Memory Model

## Memory Hierarchy

~~~~~
Level   Name                  Size           Latency
-----   --------------------  -------------  ----------
L0      Monad Registers       20 bytes       wire speed
        (in the packet)       (fixed)        (~ns)

L1      Per-hop BPF map       variable       ~100-200 ns
        cache (prefetched)

L2      Wotan ring buffer     --ring-size    ~1-10 us
        (per-flow RAM)        (configurable)

L3      Wotan WAL             disk-bounded   ~100 us-1 ms
        (persistent storage)

L4      Sophia dictionaries   BPF maps       ~100-200 ns
        (instruction decode)
~~~~~

## Address Space Layout

~~~~~
0x00000000 +------------------+
           |  Data Memory     |  <- Wotan ring buffer
           |  (general RAM)   |     per-flow, keyed by
0x0000BFFF +------------------+     Flow Label
0x0000C000 |  Screen / Output |  <- Wotan I/O topic
           |  (I/O region)    |
0x0000FFFE +------------------+
0x0000FFFF |  Input (1 word)  |  <- Wotan I/O topic
           +------------------+
~~~~~

# Chaos Injection (Yaldabaoth)

When bit C of the flags field is set, the Shim applies
deterministic perturbations to the Monad or Wotan state
for resilience testing.

Chaos modes (selected via Sophia):

~~~~~
Mode     Name           Action
------   -----------    ----------------------------
0x01     BIT_FLIP       Flip random bit in R0
0x02     VALUE_MUTATE   Increment R0 mod 2^32
0x03     MEMORY_FAULT   Wotan reads return error
0x04     LATENCY        Inflate hop latency 10x
0x05     LOSS           Drop subsequent packets (10%)
0x06     CHAOS_MARKER   Set chaos bit, visible to
                        all downstream hops
~~~~~

All chaos events are recorded in Anamnesis with
EVENT_CHAOS type.  Yaldabaoth never hides.

# Computational Completeness

The Monad (5 registers) paired with Wotan (unbounded
addressable memory via ring buffers) forms a Turing-
complete system.  A proof sketch:

1.  Tape: Wotan ring buffer (address i holds symbol at
    position i).

2.  State: Monad R3 holds current state q.  R4 holds
    head position.

3.  Transition: Shim implements delta via Sophia lookup
    table.

4.  Iteration: Packet circulation via BPF_REDIRECT.
    Each circulation = one Turing machine step.

5.  Halting: if R3 == halt_state, stop circulating.

Effective clock speed: approximately 3.7 MHz single-
instruction (one Shim execution per ~270 ns including
BPF_REDIRECT overhead), approximately 15-30 MHz with
batched multi-instruction Shim execution per hop.

With Kingdom Mode (Section 5) on an L2 overlay, the
Extended Register Space provides an additional 208 to
224 bits of register state per packet (depending on
fleet size), further increasing the computational
bandwidth of each hop.

# Security Considerations

## Extension Header Sanitization

Packets entering the Limited Domain MUST have their
Hop-by-Hop option replaced or removed.  Packets exiting
MUST have their Hop-by-Hop option stripped.  ULA
prefixes from external sources MUST be rejected.

These requirements apply to both standard Monad fields
and Kingdom Mode Extended Registers.

## eBPF Containment

Shim programs are verified by the kernel BPF verifier,
which ensures: no out-of-bounds access, bounded loop
execution, no unauthorized kernel function calls, and
stack safety.  Wotan enforces per-program access control
on ring buffers and topics.

## Integrity

The CRC-16 checksum detects accidental corruption.
For stronger guarantees, an optional HMAC-SHA256
signature can be appended:

~~~~~
signature = HMAC-SHA256(shared_secret,
                        Monad || metadata || Trace_ID)
~~~~~

For post-quantum integrity, ML-DSA-65 [FIPS204]
signatures can replace HMAC at Shield boundaries.

## Trust Boundary

The trust boundary is the Limited Domain.  Within it,
all Shim programs and Wotan instances are operator-
controlled.  Cross-domain routing is not supported;
inter-domain traffic MUST be encapsulated (IP-in-IP or
VPN tunnel) with the Hop-by-Hop option stripped and
reconstructed at each boundary.

## Post-Quantum Threat Model

The PQC identity binding (Section 6) protects against:

-  Harvest-now-decrypt-later attacks on metadata
   correlation.

-  Service identity spoofing by a quantum-capable
   adversary within the Limited Domain.

-  Key compromise via classical cryptanalysis of
   traditional key agreement algorithms.

The hybrid PQ/T mode ensures security during the
transition period while CRQCs are not yet available.

## Kingdom Mode Threat Model

Kingdom Mode Extended Registers (Section 5) are only
valid within the Limited Domain.  An external attacker
who can inject packets with forged ULA prefixes and
K flags could:

-  Forge PQC fingerprints to bypass per-hop verification.

-  Inject false Extended Register values to manipulate
   Shim program behavior.

Shield ingress MUST validate ULA prefix provenance and
reject Kingdom Mode packets from outside the domain.

# IANA Considerations

## IPv6 Hop-by-Hop Option Type

A new IPv6 Hop-by-Hop option type is requested:

~~~~~
Type:               TBD (suggested: 0x42)
Name:               Unheaded Monad Option
Change Controller:  IESG
Reference:          This document
~~~~~

The option type has high-order bits 00 (skip on
unrecognized) and third bit 0 (unchanged on transit).

## Sophia Dictionary Namespace Registry

IANA should create a new registry:

~~~~~
Registry Name:  Unheaded Sophia Dictionary Namespace
Template:       Dictionary Name, Program Name, Version,
                Organization, Contact Email
Policy:         First Come First Served
~~~~~

## Anamnesis Event Type Registry

~~~~~
Registry Name:  Unheaded Anamnesis Event Types
Template:       Event Name, Code (u8), Description,
                Reference
Policy:         Specification Required

Initial entries:
  EVENT_BORN (0x00), EVENT_COMPUTED (0x01),
  EVENT_WOTAN_RD (0x02), EVENT_WOTAN_WR (0x03),
  EVENT_CHAOS (0x04), EVENT_ROLLBACK (0x05),
  EVENT_DIED (0x06), EVENT_KEY_OP (0x07),
  EVENT_ANOMALY (0x08)
~~~~~

## PQC Algorithm Registry

~~~~~
Registry Name:  Unheaded PQC Algorithm Identifiers
Template:       Algorithm Name, Code (u8), Key Size,
                FIPS Reference
Policy:         Specification Required

Initial entries:
  ML-KEM-768 (0x01, 1184 bytes, FIPS 203)
  ML-KEM-1024 (0x02, 1568 bytes, FIPS 203)
  ML-DSA-65 (0x03, 1952 bytes, FIPS 204)
  ML-DSA-87 (0x04, 2592 bytes, FIPS 204)
  SLH-DSA-SHA2-128s (0x05, 32 bytes, FIPS 205)
~~~~~

--- back

# Heritage

The Unheaded Protocol builds on a lineage of metadata-
riding-with-data designs:

~~~~~
Year  Technology         Pattern Element
----  -----------------  ---------------------------
1977  ARINC 429          Self-contained 32-bit words,
                         every bit meaningful
1982  I2C                Two-wire bus, address in
                         first byte
1986  CAN Bus            Identifier = address +
                         priority, bus as backplane
1989  BGP (RFC 1105)     Path attributes riding with
                         routes, hop-by-hop
                         accumulation
1992  BPF                Packet filter in kernel,
                         evolved to eBPF VM
1995  IPv6 (RFC 1883)    Extension headers, typed,
                         extensible computation
                         space
2001  uIP (Dunkels)      Full TCP/IP in 5KB, minimal
                         protocol on constrained
                         hardware
2024  RFC 9673           Hop-by-Hop processing
                         rehabilitated
2024  FIPS 203/204/205   Post-quantum cryptographic
                         standards (ML-KEM, ML-DSA,
                         SLH-DSA)
2026  Unheaded Protocol  Mapped data bus, packet as
                         memory, 20-byte register
                         file, PQC identity binding,
                         address reclamation
~~~~~

# Prior Art and Relationship to Other Work

## IOAM (RFC 9197, RFC 9486)

In-situ OAM defines data fields for recording
operational telemetry in IPv6 Hop-by-Hop options.
IOAM is observation-only: it records what happened.
The Monad is intent-driven: it declares what SHOULD
happen.  IOAM traces grow with each hop (variable
length).  The Monad is fixed at 20 bytes.

## APN (Application-Aware Networking)

APN carries application identity in IPv6 headers.
APN is a framework defining what COULD be carried,
with TLV-based variable encoding.  The Monad is a
fixed-size computational atom with dictionary-based
semantic compression (Sophia).

## MPLS MNA (Network Actions)

MNA carries per-packet action directives in MPLS
label stacks.  MNA operates in MPLS networks; the
Unheaded Protocol is IPv6-native.

## NSH (RFC 8300)

The Network Service Header provides metadata for
Service Function Chaining.  NSH uses a separate
encapsulation; the Unheaded Protocol uses the
standard IPv6 Hop-by-Hop extension header mechanism.

## Key Differentiators

The Unheaded Protocol is distinguished from all of
the above by the combination of:

1.  Fixed 20-byte register file (vs. variable-length
    formats in IOAM, APN, NSH).

2.  Sophia exponent dictionary for semantic
    compression (unique).

3.  eBPF-native processing model designed from the
    start for programmable dataplanes (unique).

4.  Intent-driven computation, not observation-only
    (unique vs. IOAM).

5.  Post-quantum cryptographic identity binding of
    service identifiers (unique).

6.  Kingdom Mode address reclamation of deterministic
    prefix bits (unique).

7.  Computational completeness (Turing-complete with
    Wotan memory, unique).

# Acknowledgments

The Linux kernel eBPF community (Alexei Starovoitov,
Daniel Borkmann) for creating the infrastructure that
made this design possible.

Adam Dunkels for demonstrating with uIP and Contiki
that full protocol stacks operate in impossibly
constrained spaces — proving that the constraint IS
the design.

Roger Zelazny (1937-1995) for the Chronicles of Amber,
which taught us that the one true reality casts infinite
shadows, and that walking the Pattern is always worth
the fire.

Erik Baar, for a copy of the Chronicles and many
garage beers.
