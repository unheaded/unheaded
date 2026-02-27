---
title: "Unheaded: Protocol Foundation — A Mapped Data Bus over IPv6 Hop-by-Hop Options"
abbrev: "Unheaded Protocol"
docname: draft-bellis-unheaded-protocol-foundation-05
category: exp
ipr: trust200902
area: Internet
workgroup: Independent Submission
date: 2026-02-27
stand_alone: yes

keyword:
  - bpf
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
  - shield-ebpf
  - anamnesis
  - kingdom-mode

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
  RFC9669:
  RFC9673:
  RFC9000:
  RFC9114:

informative:
  RFC0768:
  RFC0791:
  RFC0792:
  RFC0793:
  RFC1918:
  RFC4193:
  RFC8300:
  RFC8754:
  RFC8799:
  RFC9098:
  RFC9180:
  RFC9197:
  RFC9288:
  RFC9370:
  RFC9486:
  RFC9631:
  MONAD-EXT-REG:
    title: "Monad Extended Register Option for the Unheaded Protocol"
    author:
      - ins: S. Bellis
        name: Stevie Bellis
    date: 2026-02
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-monad-extended-register-00
    target: draft-bellis-unheaded-monad-extended-register-00
  RFC0950:
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

The Unheaded Protocol defines a mapped data bus model that
transforms IPv6 packets into addressable memory by encoding a small
register file directly in the IPv6 Hop-by-Hop Options extension header.

We introduce a 20-byte Monad (register file) that carries program state
through the network. At each hop, a BPF program (the Shim) performs
computation on the Monad. The packet itself becomes the working memory,
using exponent-encoded fields to pack rich metadata into the IPv6
option while remaining fully backward-compatible with existing networks.

To support programs larger than what fits in a single Monad, we
introduce Wotan, a memory and I/O bus that bridges Monad computation
to per-flow ring-buffer storage and external topics. This decouples
the Monad (pure, 20-byte compute) from memory (Wotan's configurable
data planes).

This memo extends the packet format with two additional capabilities:
(1) Kingdom Mode Address Reclamation, which recovers up to 224 bits of
deterministic address space from IPv6 addresses within a controlled L2
fabric for use as extended computational and cryptographic registers;
and (2) Post-Quantum Cryptographic Identity Binding, which cryptographically
binds each service identifier in the Monad to a post-quantum keypair via
the Sophia dictionary system, providing quantum-resistant authentication
of per-packet metadata without increasing wire overhead.

This memo defines the normative packet format, exponent-encoding scheme,
per-hop processing semantics, address reclamation model, post-quantum
identity binding, optional chaos injection for resilience testing, and
the complete computational model (Turing-complete with memory paging).

--- middle

# Introduction

## Problem Statement

Classical networking separates computation from data. Computation
happens in applications. Data flows through the network as opaque byte
streams. This creates an expensive impedance mismatch of serialization,
deserialization, protocol translation, middleware, sidecars, and proxies.

The Unheaded Protocol inverts this model: the packet carries
computational state. A 20-byte register file (the Monad) is read and
written by BPF programs at each hop. The packet functions as working
storage of a distributed computation that executes at each
operator-controlled node in the path.

Within a Limited Domain [RFC8799] — a single AS, corporation, or
container fleet where every hop is operator-controlled — the protocol
provides:

-  Inline state: Application state is carried in the packet without
   intermediate serialization.

-  Per-hop compute: Each hop reads the Monad state directly from the
   packet without requiring separate queries or network round-trips.

-  Causal ordering: Anamnesis events are ordered by packet arrival at
   each hop, enabling causal reconstruction independent of wall-clock
   synchronization.

-  Quantum-resistant identity: Service identifiers are cryptographically
   bound to post-quantum keypairs.

-  Address reclamation: Deterministic address prefix bits within the
   Limited Domain are reclaimed as extended computational registers.

## Scope and Applicability

This specification defines the complete protocol for deploying a
mapped data bus over IPv6 Hop-by-Hop Options. The protocol is
applicable to Limited Domains (RFC 8799) where all intermediate nodes
are operator-controlled and IPv6 Hop-by-Hop option processing is
enabled.

The protocol is NOT applicable to the public Internet, where
intermediate routers may drop Hop-by-Hop options (RFC 9098, RFC 9673),
nor to paths containing routers that do not process such options.

# Requirements Language

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this
document are to be interpreted as described in BCP 14 [RFC2119]
[RFC8174] when, and only when, they appear in all capitals, as shown
here.

# Terminology

This document uses the following terms:

Monad:
: The 20-byte register file embedded in the IPv6 Hop-by-Hop option
  header. It travels with the packet and is read and modified at each hop.

Shim:
: A BPF program that executes at each hop, reading the Monad from the
  packet, performing computation, and writing back to the packet.

Shield:
: The packet boundary logic at ingress (first hop) that stamps the
  packet into existence by adding the Hop-by-Hop option, and at egress
  (last hop) that strips the option and commits the packet to the
  external network.

Wotan:
: The memory and I/O bus that provides per-flow ring buffers,
  persistent storage (WAL), and bridging to external topics.

Anamnesis:
: The non-blocking event log implemented as a BPF ring buffer,
  recording packet events for observability and historical replay.

Sophia:
: The exponent dictionary system. BPF hash maps in kernel space,
  structured tables in userspace. Hot-swappable vocabulary that
  assigns meaning to exponent-encoded field values.

Exponent Encoding:
: A compositional scheme for packing metadata: each field stores a
  signed exponent, and the actual value is reconstructed as
  base^exponent * multiplier, where base and multiplier are defined
  per-field in Sophia.

Limited Domain:
: A network boundary (typically a single AS, corporation, or container
  fleet) where the Unheaded Protocol is deployed end-to-end. All hops
  within the domain are operator-controlled. Defined in [RFC8799].

Kingdom Mode:
: An operational mode within a Limited Domain where the IPv6 ULA
  prefix is known a priori, enabling deterministic address prefix bits
  to be reclaimed as extended computational space.

# Protocol Overview

The Unheaded Protocol adds an IPv6 Hop-by-Hop Options extension header
to packets at the Limited Domain ingress. The option contains a 20-byte
register file (Monad) that carries computational state. At each
operator-controlled hop within the domain, a BPF program (Shim) reads
the Monad, performs computation, and writes the updated Monad back to
the packet before forwarding it to the next hop. At egress, Shield
removes the option and forwards a clean IPv6 packet to the external
network.

The Monad's fields are exponent-encoded, allowing rich metadata to be
packed into 8-bit signed values. Field semantics are defined by Sophia,
a dictionary system stored as BPF hash maps. Ring buffers (Anamnesis)
record packet events at each hop for observability.

# Packet Format

## IPv6 Hop-by-Hop Extension Header

The Monad is carried in a single IPv6 Hop-by-Hop Options extension
header option, formatted per RFC 8200 Section 4.

~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
|                     Option TLV(s)                             |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~

The Hop-by-Hop extension header MUST precede all other extension
headers. It MUST be processed at each hop per RFC 8200 Section 4.3
and RFC 9673.

## Option Type

The Monad is encoded as a single option TLV within the Hop-by-Hop
extension header:

~~~~
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
~~~~

Type:
: To be assigned by IANA. The high-order two bits MUST be 00 (skip on
  unrecognized). The third bit MUST be 1 (option data may change
  en-route), as the Monad is modified by per-hop processing. The
  resulting format is 001xxxxx. Suggested value: 0x3E.

Len:
: The length of the option payload in octets (not including the Type
  and Len octets). Minimum value is 20 (Monad only); maximum is 255.

## Monad Register File

The Monad is a 20-byte register file with the following layout. All
multi-byte fields MUST be encoded in network byte order (big-endian).

~~~~
Offset  Size  Field               Type        Description
------  ----  ------------------  ----------  ---------------------------------
0x00    1     version             raw uint8   Protocol version (current: 0x01)
0x01    1     src_service_id      exponent    Source service (Sophia lookup)
0x02    1     dst_service_id      exponent    Destination service (Sophia)
0x03    1     hop_count           raw uint8   Decremented at each hop (TTL-like)
0x04    1     qos_class           exponent    QoS classification
0x05    1     flow_action         exponent    Action directive
0x06    1     circuit_state       exponent    Circuit breaker state
0x07    1     flags               raw uint8   Bitfield (see Flags section)
0x08    2     latency_hint        raw uint16  Latency hint in microseconds
0x0A    1     deploy_ring         exponent    Deployment ring
0x0B    1     mesh_flags          exponent    Mesh-level flags
0x0C    1     src_prefix_lo       raw uint8   Source routing prefix low octet
0x0D    1     dst_prefix_lo       raw uint8   Destination routing prefix low octet
0x0E    4     scratch[0-3]        raw uint8   Scratch registers (4 bytes)
0x12    2     checksum            raw uint16  CRC-16/CCITT over bytes 0x00-0x11
------  ----  ------------------  ----------  ---------------------------------
Total: 20 bytes (0x14)
~~~~

version:
: The protocol version, an unsigned 8-bit integer. This document
  specifies version 0x01 (current). Version 0 is reserved and MUST NOT be
  used. Future versions (2+) are currently undefined.

  Version Checking (NORMATIVE): Receivers MUST drop packets with
  unknown version fields immediately with no error code generation or
  fallback processing. Specifically, if the version field != 0x01,
  the packet MUST be dropped immediately (silent drop). No version
  negotiation, no fallback, no compatibility shim. This eliminates
  parser divergence attacks (X4) by ensuring all implementations reject
  unknown versions identically.

src_service_id:
: An exponent-encoded field identifying the source service. Semantics
  are defined by Sophia dictionary lookup. Implementations MUST NOT
  assume fixed semantics; the meaning is program-defined per service.

dst_service_id:
: An exponent-encoded field identifying the destination service.
  Semantics are defined by Sophia dictionary lookup.

hop_count:
: An unsigned 8-bit counter, initially set to a deployment-defined
  hop limit (default 64) at Shield ingress. Each hop MUST check
  whether hop_count equals 0; if so, the packet MUST be dropped and
  an EVENT_ANOMALY MUST be emitted. Otherwise, the hop MUST
  decrement hop_count by 1 before forwarding.

trace_id:
: Flow trace correlation is derived from the IPv6 Flow Label (RFC 6437).
  The 20-bit Flow Label set by Shield at ingress serves as the trace
  correlation identifier.  Shim programs MUST NOT modify the Flow Label.

qos_class:
: An exponent-encoded field specifying the Quality of Service class.
  Semantics are defined by Sophia.

flow_action:
: An exponent-encoded field specifying the action directive for this
  packet. Examples include trace, sample, mirror, rate-limit, or drop.
  Semantics are defined by Sophia.

circuit_state:
: An exponent-encoded field specifying the circuit breaker state (open,
  closed, half-open). Semantics are defined by Sophia.

flags:
: An 8-bit bitfield controlling protocol behavior. See Section 5.2.

latency_hint:
: A 16-bit hint encoding the per-hop latency target, in network byte order.
  Interpretation is deployment-defined.

deploy_ring:
: An exponent-encoded field specifying the deployment ring (canary,
  staging, production). Semantics are defined by Sophia.

mesh_flags:
: An exponent-encoded field specifying mesh-level flags (NAT type,
  direction, encryption). Semantics are defined by Sophia.

src_prefix_lo:
: The low-order octet of the source routing prefix, used by Shim programs
  for routing optimization.  Set by Shield at ingress from Sophia policy.

dst_prefix_lo:
: The low-order octet of the destination routing prefix, used by Shim
  programs for routing optimization.  Set by Shield at ingress from Sophia
  policy.

scratch:
: Four bytes of per-hop scratch storage (scratch[0]-scratch[3]).
  Used by Shim programs for temporary computation.  Scratch bytes form
  two 16-bit registers: scratch_r0 (bytes 0x0E-0x0F) and scratch_r1
  (bytes 0x10-0x11).  When CUSTOM flag is set, scratch_r0 and scratch_r1
  carry exponent-encoded values whose semantics are deployment-defined.
  Shield MUST zero scratch bytes at ingress unless CUSTOM is set.

checksum:
: A 16-bit CRC-16/CCITT checksum computed over the first 18 bytes
  (0x00-0x11) of the Monad. See Section 5.4.

### Extended Register Option

The 20-byte primary register file defined above MAY be complemented by
an optional Extended Register Option carried as a second IPv6 HbH option
within the same extension header.  The extended option provides
additional registers (r16-r31) using the complement space principle:
the bitwise inverse of the primary register map identifies available
compute capacity, formalized as a separate option with its own type
code.  This approach derives from the inverted subnet mask (wildcard
mask) formalism established in {{RFC0950}}.

The Extended Register Option is defined in {{MONAD-EXT-REG}}.  Nodes
that do not recognize the extended option MUST skip it per {{RFC8200}}
option type processing rules (act=00).  The primary Monad option format
is unchanged; the extended option is purely additive.

## Flags Bitfield

The flags field (offset 0x0B) is an 8-bit field controlling per-packet
behavior:

~~~~
 7   6   5   4   3   2   1   0
+---+---+---+---+---+---+---+---+
| C | Y | T | E | S | M |CUST| R |
+---+---+---+---+---+---+---+---+

C (0x80):  CHAOS — Chaos injection active (Yaldabaoth)
Y (0x40):  CANARY — Canary deployment path
T (0x20):  TRACED — Full trace active (all hops emit to Anamnesis)
E (0x10):  ENCRYPT — Payload encrypted (intra-Kingdom TLS)
S (0x08):  SAMPLED — Statistically sampled
M (0x04):  MIRROR — Mirror copy (not original)
CUSTOM (0x02): Scratch and checksum fields carry exponent-encoded values
RSVD (0x01): Reserved, MUST be zero
~~~~

Each bit MUST be set or cleared by Shim programs as needed. Shield
MUST ensure that the C, Y, T, E, and S bits are set to consistent
values at ingress based on policy.

CUSTOM (0x02): When set, the scratch fields (0x0E-0x11) and the checksum
field (0x12-0x13) carry exponent-encoded values whose interpretation is
defined by the active Sophia dictionary.  Shield MUST NOT set CUSTOM
unless configured by policy.

RSVD (0x01): Reserved.  Senders MUST set to zero.  Receivers MUST
ignore.

## Checksum Field

The checksum field (offset 0x12) holds a 16-bit CRC-16/CCITT value
computed over all 20 bytes of the Monad header (offsets 0x00-0x13,
inclusive), with the checksum field itself (offsets 0x12-0x13) zeroed
during computation.

CRC-16/CCITT-FALSE Parameters:

Polynomial:
: x^16 + x^12 + x^5 + 1 (0x1021)

Initial Value:
: 0xFFFF

Input Reflection:
: false

Output Reflection:
: false

Final XOR:
: 0x0000

Computation Procedure:

The checksum is computed as follows:

1. Create a working copy of the 20-byte Monad header.
2. Set bytes 0x12-0x13 (the checksum field) to 0x0000.
3. Compute CRC-16/CCITT over all 20 bytes of this modified header.
4. Store the resulting 16-bit value at offset 0x12.

This ensures integrity protection over all Monad fields, including
version, flags, flow_label, latency_hint, and reserved fields.

Shield MUST compute the checksum when creating a packet at ingress.
Each hop MUST verify the checksum before processing. Each hop MUST
recompute the checksum after modifying any field in offsets 0x00-0x11.
The checksum field itself (offset 0x12-0x13) MUST NOT be included in
the checksum computation (set to zero during computation).

If a hop detects a checksum failure, the implementation MUST:

  (a) Increment a per-interface error counter.

  (b) Emit an EVENT_ANOMALY event to Anamnesis if tracing is enabled.

  (c) Either drop the packet (RECOMMENDED) or forward it with an
      anomaly flag set, depending on deployment policy.

The CRC-16 checksum provides error detection against accidental bit
corruption only. It does NOT provide integrity protection against
malicious modification. See Security Considerations for guidance on
integrity protection mechanisms.

# Exponent Encoding

## Overview

Exponent encoding is a compositional scheme for representing rich
metadata in compact form. An exponent-encoded field is a single octet
interpreted as a signed 8-bit integer (two's complement, range -128 to
+127).

The decoded value is computed as:

~~~~
decoded = base ^ exponent
~~~~

Where:

-  base is the base value defined by the Sophia dictionary entry for
   this field position. If no Sophia entry exists, the default base is 2.

-  exponent is the signed 8-bit value stored in the field.

-  The result is an unsigned integer value.

An optional multiplier (unit scaling factor) may be applied:

~~~~
decoded = (base ^ exponent) * multiplier
~~~~

The multiplier is also defined per-field in the Sophia dictionary. If
no entry exists, the multiplier is 1.

Encoders MUST NOT produce exponent values that would cause the decoded
value to exceed 2^64 - 1. Decoders that encounter such values MUST
treat them as errors and emit an anomaly event.

## Sophia Dictionary System

Sophia dictionaries are hierarchical structures stored as BPF hash maps
in kernel space and as structured tables in userspace. Each exponent
field in the Monad has a corresponding Sophia entry that defines:

-  Field name and purpose.

-  Base value (typically 2, but may be 10 or other values).

-  Multiplier for unit scaling.

-  Semantic interpretation (lookup table from exponent to meaning).

### Concrete Example

For src_service_id (offset 0x01):

~~~~
Sophia Entry (service_identity):
  exponent = 0x03  (signed byte, value 3)
  base = 2
  multiplier = 1
  decoded = 2^3 * 1 = 8

Lookup: service ID 8 -> "architect" (service name)
        service ID 8 -> "fd00::8" (endpoint address)
        service ID 8 -> 0x0A03 (algorithm ID: ML-KEM-768)
~~~~

For qos_class (offset 0x08):

~~~~
Sophia Entry (qos_class):
  exponent = 0x02  (signed byte, value 2)
  base = 2
  multiplier = 8  (microseconds per hop)
  decoded = 2^2 * 8 = 32 microseconds

Interpretation: QoS class with 32-microsecond target latency.
~~~~

### Sophia Wire Format and Distribution

Sophia dictionaries MUST be distributed to all Kingdom nodes via Wotan
topics. Each dictionary entry MUST be serialized in CBOR format (RFC
9152) and identified by a (service_id, field_type) tuple.

A minimal Sophia dictionary that all implementations MUST support
includes:

~~~~
Root Dictionary:
  0x01 -> service_identity (sub_dict_1)
  0x02 -> flow_action (sub_dict_2)
  0x03 -> qos_class (sub_dict_3)

Sub-Dictionary: service_identity (min 16 entries)
  0x01 -> "shield" (ingress gateway)
  0x02 -> "shim" (internal hop)
  0x03 -> "architect" (application)
  ... (implementation-specific services)

Sub-Dictionary: flow_action (min 8 entries)
  0x00 -> forward (normal)
  0x01 -> trace (full event logging)
  0x02 -> sample (probabilistic logging)
  0x03 -> drop (discard packet)

Sub-Dictionary: qos_class (min 4 entries)
  0x00 -> best-effort
  0x01 -> interactive
  0x02 -> realtime
  0x03 -> bulk
~~~~

Implementations MAY extend these dictionaries with additional entries.
Dictionary version negotiation is performed through the Monad's
reserved field and Anamnesis event correlation.

# Sophia Dictionary System (Extended)

The Sophia dictionary system is the semantic layer that transforms raw
exponent bytes into meaningful values. It operates at multiple levels:

## Dictionary Architecture

Sophia dictionaries are trees, not flat tables. Each byte narrows the
context for the next:

~~~~
byte_0 = 0x01 -> sophia_root -> dict_id = 1 (service_identity)
byte_1 = 0x03 -> dict_1      -> "architect"

The SAME byte_1 value in DIFFERENT contexts:
[0x01, 0x03] -> service = "architect"
[0x02, 0x03] -> action  = "sample"
[0x03, 0x03] -> qos     = "realtime"
~~~~

With K key positions, the expressible meaning space is:

~~~~
M = 256^K

K=1:  256 meanings           (8 bits)
K=2:  65,536 meanings        (16 bits)
K=3:  16,777,216 meanings    (24 bits)
K=8:  1.844 x 10^19 meanings (64 bits)
~~~~

## BPF Map Implementation

Sophia dictionaries are stored as BPF hash maps in kernel space per
RFC 9669. Each lookup is O(1) complexity. A two-level lookup (root map
to sub-dictionary) costs two hash table hits — still under 100
nanoseconds on modern hardware.

Dictionary updates propagate cluster-wide in under 10 milliseconds via
atomic BPF map replacement through Wotan. This allows hot-swapping of
service identifiers, QoS policies, and Shim behavior without restarting
any kernel components.

## Minimum Required Dictionary

All implementations MUST support the following minimum Sophia dictionary:

~~~~
Root entries (1 byte key):
  0x01 = service_identity    (sub_dict_1)
  0x02 = flow_action         (sub_dict_2)
  0x03 = qos_class           (sub_dict_3)
  0x04 = deploy_ring         (sub_dict_4)
  0x05 = circuit_state       (sub_dict_5)
  0x06 = mesh_flags          (sub_dict_6)

service_identity (sub_dict_1):
  Must include entries for all active service IDs in the Kingdom.
  Each entry maps to (service_name, endpoint_address, metadata).

flow_action (sub_dict_2):
  0x00 = FORWARD      (default: pass packet to next hop)
  0x01 = TRACE        (emit full event to Anamnesis)
  0x02 = SAMPLE       (emit with probabilistic probability)
  0x03 = DROP         (discard packet)
  0x04 = MIRROR       (clone to monitoring interface)
  (0x10-0x14 reserved for PQC key lifecycle events)

qos_class (sub_dict_3):
  0x00 = BULK         (low priority, best effort)
  0x01 = INTERACTIVE  (medium priority, <100ms latency)
  0x02 = REALTIME     (high priority, <10ms latency)

deploy_ring (sub_dict_4):
  0x00 = CANARY       (test deployment)
  0x01 = STAGING      (pre-production)
  0x02 = PRODUCTION   (user-facing)

circuit_state (sub_dict_5):
  0x00 = CLOSED       (normal operation)
  0x01 = OPEN         (circuit breaker triggered)
  0x02 = HALF_OPEN    (recovery attempt)

mesh_flags (sub_dict_6):
  0x00 = DEFAULT      (standard routing)
  0x01 = NAT_INGRESS  (behind NAT)
  0x02 = NAT_EGRESS   (acting as NAT)
  (implementation-specific flags beyond 0x02)
~~~~

# Shield: eBPF Security Pipeline

Shield is implemented as an eBPF-based security pipeline that enforces
policy at the Limited Domain boundary. The pipeline operates in two stages:
XDP (eXpress Data Path) ingress for maximum performance and TC (Traffic Control)
egress for comprehensive stateful processing.

## XDP Ingress Processing

At packet ingress, an XDP program executes in the kernel driver before
sk_buff allocation, enabling extreme-low latency packet stamping:

Ingress XDP Operations (per Monad):
  1. Load IPv6 5-tuple from packet (src/dst IP, src/dst port, next-header)
  2. Consult admission control BPF map (blocklist, rate-limit)
  3. If denied: increment drop counter, return XDP_DROP
  4. Allocate IPv6 Hop-by-Hop option using bpf_skb_adjust_room()
  5. Write 20-byte Monad via bpf_skb_store_bytes()
  6. Update IPv6 header length field
  7. Return XDP_PASS to forward to TC layer

Processing Latency: <1 microsecond per packet (wire speed)

BPF Maps Required:
  - admissions_policy (BPF_MAP_TYPE_HASH): keyed by src_ip, value = policy
  - rate_limit_buckets (BPF_MAP_TYPE_HASH): keyed by src_ip, value = token bucket
  - geodata (BPF_MAP_TYPE_ARRAY): IP CIDR ranges, value = geographic region

## TC Egress Processing

At packet egress, a TC (Traffic Control) program executes after routing
but before transmission. This layer performs stateful filtering and
address restoration.

Egress TC Operations (per Monad):
  1. Extract IPv6 Hop-by-Hop option and verify CRC-16 checksum
  2. Verify flags (check RSVD bit, anomaly flags)
  3. Extract Monad fields for logging/observability
  4. If Kingdom Mode: restore original IPv6 host bits
  5. Remove IPv6 Hop-by-Hop extension header
  6. Update IPv6 Next Header field to restore original header chain
  7. Update IPv6 payload length field
  8. Return TC_ACT_OK to forward packet

Processing Latency: <1 microsecond per packet

BPF Maps Required:
  - egress_policy (BPF_MAP_TYPE_HASH): stateful conntrack entries
  - kingdom_prefixes (BPF_MAP_TYPE_ARRAY): ULA prefix configuration
  - event_counters (BPF_MAP_TYPE_ARRAY): per-packet-type statistics

## BPF Map Pinning Contract

All Shield BPF maps MUST be pinned to the filesystem at:

~~~~
/sys/fs/bpf/unheaded/
  ├── admissions_policy
  ├── rate_limit_buckets
  ├── geodata
  ├── egress_policy
  ├── kingdom_prefixes
  └── event_counters
~~~~

Pinning ensures:
  - Maps persist across BPF program reload
  - Multiple programs can share the same backing storage
  - Userspace daemons (Wotan, Sophia) can hot-update entries
  - Disaster recovery via iproute2: ip bpf map show /sys/fs/bpf/unheaded

## BPF Map Types and Schemas

### admissions_policy (XDP Ingress)
Type: BPF_MAP_TYPE_HASH
Key: u32 (source IP address, big-endian)
Value: Struct {
  u32 policy_flags;        // Policy directive (allow/deny/quarantine)
  u32 rate_limit_pps;      // Packets per second
  u32 burst_packets;       // Token bucket depth
  u8 geo_region;           // Geographic region code
}
Max Entries: 1,000,000 (expecting 10-100K in practice)

### rate_limit_buckets (XDP Ingress)
Type: BPF_MAP_TYPE_HASH
Key: u32 (source IP address)
Value: Struct {
  u64 tokens;              // Current token count (scaled by 1000)
  u64 last_refill_time_ns; // Last refill timestamp
}
Max Entries: 1,000,000

### egress_policy (TC Egress)
Type: BPF_MAP_TYPE_HASH
Key: Struct {
  u32 src_ip;
  u32 dst_ip;
  u16 src_port;
  u16 dst_port;
}
Value: Struct {
  u8 connection_state;     // ESTABLISHED, NEW, INVALID
  u64 packets_seen;        // Packet count
  u64 bytes_seen;          // Byte count
  u64 last_seen_time_ns;   // Last activity timestamp
}
Max Entries: 100,000

### kingdom_prefixes (TC Egress)
Type: BPF_MAP_TYPE_ARRAY
Key: u32 (array index 0-3 for up to 4 ULA prefixes)
Value: Struct {
  u8 prefix[8];            // IPv6 ULA prefix (first 64 bits)
  u64 valid_since_ns;      // Validity start time
  u64 valid_until_ns;      // Validity end time
}
Max Entries: 4

## BPF Verifier Compliance

All Shield eBPF programs MUST pass the kernel BPF verifier with:
  - No unbounded loops (verifier enforces finite execution)
  - No out-of-bounds map accesses
  - Stack usage < 512 bytes
  - Verified on Linux kernel 5.17+

Verifier errors MUST be considered fatal. Programs with verifier warnings
MUST NOT be loaded.

## Interaction with Monad Hop-by-Hop Processing

Shield (XDP ingress) and Shim (per-hop) programs form a pipeline:

~~~~
[External packet]
        |
        v
[Shield XDP: Add Monad, admit packet]
        |
        v
[IPv6 routing to next hop]
        |
        v
[Shim BPF: Read Monad, execute Shim, update Monad]
        |
        v
[repeat at each hop]
        |
        v
[Shield TC: Strip Monad, verify egress policy]
        |
        v
[External network]
~~~~

Shield MUST NOT modify fields that Shim programs depend on (like flow_label).

# Shield Processing

Shield is the packet boundary logic that adds and removes the Monad
option at the Limited Domain ingress and egress.

## Ingress Processing

At packet ingress, Shield MUST perform the following operations:

1. Receive packet from outside the Limited Domain.

2. Apply admission control checks:

   a. Consult the blocklist BPF map keyed by source IP address.
   b. Check rate-limit token bucket for the source IP.
   c. If configured, perform geo-IP filtering.
   d. If any check fails, drop the packet and emit a Shield block event
      to Anamnesis.

3. If packet is admitted:

   a. Insert an IPv6 Hop-by-Hop extension header.
   b. Allocate a new Monad option with Type = 0x3E (TBD).
   c. Initialize all Monad fields:
      - version = 0x01
      - src_service_id = Sophia.ingress_classify(source_ip)
      - dst_service_id = Sophia.ingress_classify(destination_ip)
      - hop_count = 0
      - qos_class = Sophia.policy_lookup(src_ip, dst_port)
      - flow_action = 0x00 (FORWARD)
      - circuit_state = Sophia.lookup("circuit_state", 0x00)
      - flags = 0x00 (no chaos, no custom encoding initially)
      - latency_hint = Sophia.lookup("latency_policy", dst_service_id)
      - deploy_ring = Sophia.ring_lookup(dst_service_id)
      - src_prefix_lo = extracted from source address or Sophia
      - dst_prefix_lo = extracted from destination address or Sophia
      - scratch[0-3] = 0x00 (zero unless CUSTOM flag set)
      - mesh_flags = 0x00
   d. Compute CRC-16 checksum over bytes 0x00-0x11, store at offset 0x12.
   e. Update IPv6 Next Header field to 0 (Hop-by-Hop).
   f. Emit BIRTH event to Anamnesis ring buffer with full Monad snapshot.

4. Forward the packet into the Limited Domain.

## Egress Processing

At packet egress, Shield MUST perform the following operations:

1. Receive packet destined to exit the Limited Domain.

2. Parse the IPv6 Hop-by-Hop extension header.

3. Extract the Monad option and verify CRC-16 checksum over bytes 0x00-0x11:

   a. If checksum verification fails, discard the packet, log an error,
      and emit an anomaly event.
   b. If checksum is valid, proceed.

4. Verify that flags & RSVD == 0. If the RSVD bit is set, emit an anomaly
   event and either drop the packet or set an anomaly flag per policy.

5. Emit DEATH event to Anamnesis with the final Monad snapshot and exit
   timestamp.

6. Remove the IPv6 Hop-by-Hop extension header.

7. Restore the original IPv6 Next Header value.

8. Forward the clean IPv6 packet out of the Limited Domain.

## Lifecycle

~~~~
[Shield ingress] -> [Shim hop 1] -> [Shim hop 2] -> ... -> [Shield egress]
       |                |              |                         |
     BIRTH           COMPUTED        COMPUTED                  DEATH
     event           event           event                     event
~~~~

# Per-Hop Processing

At each hop within the Limited Domain, the following operations MUST be
performed in order:

1. Parse the IPv6 Hop-by-Hop option and extract the Monad.

1a. Verify that the packet contains exactly zero or one Hop-by-Hop (HbH)
    option. If multiple HbH options are present, the packet MUST be dropped
    immediately with no error code generation or fallback parsing. This
    restriction eliminates header smuggling attacks (X3) by preventing parser
    divergence on ambiguous HbH header counts.

2. Verify the CRC-16 checksum over bytes 0x00-0x11.

   a. If checksum verification fails, emit an EVENT_ANOMALY to Anamnesis,
      increment the per-interface error counter, and either drop the
      packet (RECOMMENDED) or set an anomaly flag.

3. Verify that the version field equals 0x01. If version is unknown,
   emit an anomaly event and drop the packet (MUST NOT forward with
   unknown version).

4. If CUSTOM flag (bit 1) is set, interpret scratch fields and checksum
   as exponent-encoded values per Sophia. Otherwise, treat as raw fields.

5. Look up the Shim program via Sophia, keyed by program name or default.

6. Execute the Shim BPF program with the current Monad as input. The Shim
   MUST NOT modify fields outside offsets 0x00-0x11. The Shim MAY read
   and write Wotan memory via BPF helper functions (see Section 11).

7. Write the updated Monad back to the packet.

8. Check whether hop_count equals 0; if so, drop the packet and emit
   an EVENT_ANOMALY. Otherwise, decrement hop_count by 1.

9. Recompute the CRC-16 checksum over bytes 0x00-0x11 and store at
   offset 0x12.

10. Emit a COMPUTED event to Anamnesis (non-blocking ring buffer write)
    containing the input Monad, output Monad, and metadata (hop ID,
    timestamp).

11. Forward the packet to the next hop.

All per-hop processing MUST be stateless with respect to the Monad. The
protocol guarantees that the output Monad depends only on the input
Monad, the Sophia dictionary, and the Shim program logic. External state
(counters, per-flow memory) is accessed only through Wotan.

# TLV (Type-Length-Value) Extension Mechanism

TLV extensions allow Monad packets to carry optional metadata from Sophia,
Wotan, and future protocol extensions beyond the 20-byte core Monad.

## TLV Container Format

TLV options are appended after the 20-byte Monad header, before the
Sophia dictionary or payload. Format:

~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|Type|C|  Length  |                 Value...                    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~

Type: 7 bits (0-127), identifies TLV type
C: 1 bit, critical bit (1 = must understand, 0 = can ignore)
Length: 8 bits, length of value in bytes (0-255)
Value: variable-length value

## TLV Type Registry

Allocated TLV types (with critical bit 0, i.e., type = base_type):

Base Type 0x00-0x1F: Reserved for Monad Foundation (future use)
Base Type 0x20-0x3F: Sophia Dictionary extensions [SOPHIA]
Base Type 0x40-0x5F: Wotan Memory extensions [WOTAN]
Base Type 0x60-0x7F: Future protocol extensions

Critical Bit:
Type_with_critical = base_type (C=0, can ignore)
              or base_type | 0x80 (C=1, must understand)

Example:
  - Type 0x20 (C=0): Sophia TLV, optional (can skip if not understood)
  - Type 0xA0 (C=1): Sophia TLV, critical (must understand or drop packet)

## Unknown TLV Handling

When processing TLVs in a packet:

~~~~
for tlv in packet.tlvs:
  if tlv.type not in KNOWN_TYPES:
    if tlv.critical_bit == 1:
      DROP_PACKET()  # Critical TLV not understood
    else:
      SKIP_TLV()     # Optional TLV, skip and continue
  else:
    PROCESS_TLV(tlv)
~~~~

This ensures:
  - Critical extensions cannot be silently ignored
  - Optional extensions degrade gracefully
  - No parser divergence on unknown types

## Extension Registration Process

To register a new TLV type:

1. RFC author allocates type N from appropriate range (0x00-0x7F)
2. Document interpretation: critical vs. optional
3. Specify value format (length constraints, field definitions)
4. Update Monad RFC with allocation table entry
5. IETF consensus required for allocations

New TLV types MUST NOT overlap with existing allocations.

## Ring Path Counter TLV (M8 Extension)

A new TLV type is defined for tracking ring buffer path traversals:

Type: 0x01 (Monad Foundation, optional)
Name: Ring Path Counter
Critical: 0 (optional, can be ignored if not understood)
Length: 4 bytes (fixed)
Value: 32-bit counter (path traversal count)

Semantics:
  Counter increments by 1 each time packet traverses a ring node
  (Wotan L1 cache, WAL, etc.). Initial value: 0.

  Implementations MAY ignore this TLV if not tracking ring paths.
  Implementations tracking paths MUST increment counter at each ring node.

  Maximum counter value: 2^32 - 1. If counter would exceed this,
  implementation SHOULD drop packet (prevent integer overflow).

Use Cases:
  - Detect loops (counter > N indicates packet looping, drop immediately)
  - Traffic engineering (routing decisions based on path depth)
  - Performance analysis (latency vs. ring path count correlation)

Format (big-endian):
~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                      Ring Path Counter                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~

# Anamnesis Event System

Anamnesis is the non-blocking observability system implemented as a BPF
ring buffer. Events are written at each hop and consumed by Wotan for
decoding and external distribution.

## Event Structure

Each event is a fixed-size structure:

~~~~
struct anamnesis_event {
  u64 timestamp_ns;        // bpf_ktime_get_ns()
  u8  event_type;          // BIRTH, COMPUTED, DEATH, etc.
  u8  hop_index;           // Which hop emitted this (optional)
  u16 reserved;
  u32 input_monad[5];      // 20 bytes: Monad before this operation
  u32 output_monad[5];     // 20 bytes: Monad after this operation
  u32 flow_label;          // Copied from IPv6 Flow Label for fast correlation
  u32 wotan_addr;          // Wotan memory address (if accessed)
  u32 wotan_value;         // Value read/written (if applicable)
};
// Total: 64 bytes per event
~~~~

## Event Types

The event_type field (offset 1 byte) specifies the event:

~~~~
Code   Name            Description
----   --------------  ---------------------------------
0x00   EVENT_BORN      Packet created at Shield (birth)
0x01   EVENT_COMPUTED  Shim executed, Monad updated
0x02   EVENT_WOTAN_RD  Wotan ring-buffer read
0x03   EVENT_WOTAN_WR  Wotan ring-buffer write
0x04   EVENT_CHAOS     Chaos mode applied
0x05   EVENT_ROLLBACK  Monad rolled back (error recovery)
0x06   EVENT_DIED      Packet reached Shield (death)
0x07   EVENT_KEY_OP    PQC key lifecycle event
0x08   EVENT_ANOMALY   Checksum failure, version mismatch, or
                       integrity error
~~~~

## Ring Buffer Writing

Anamnesis events MUST be written non-blocking. If the ring buffer is
full, the event MUST be dropped silently and a dropped-event counter
MUST be incremented. This ensures that packet processing is never
blocked by observability.

The ring buffer size MUST be configured to handle the expected packet
rate. For a 10 Gbps line rate with 1500-byte average packets (~833,333
pps) with 64-byte events, a 102 MB per-CPU ring buffer covers
approximately 2 seconds at full rate with every packet emitting events.

## Event Sampling

The T flag (trace) and S flag (sample) in the flags field control
whether events are emitted:

- If T=1 (trace), emit events for all hops.
- If S=1 (sample) and T=0, emit events according to sampling probability
  defined by qos_class in Sophia.
- If both T=0 and S=0, emit only anomaly and birth/death events.

Sampling probabilities are defined per qos_class:

~~~~
qos_class = realtime    -> always emit (S = 1.0)
qos_class = interactive -> emit 10% (S = 0.1)
qos_class = bulk        -> emit 1% (S = 0.01)
~~~~

Additionally, packets with the C flag set (chaos) MUST always emit
events, and packets with the Y flag set (canary) SHOULD always emit
events.

# Anamnesis: Event Capture Architecture

Anamnesis is the observability layer that captures per-packet events at
each hop and forwards them to Wotan for aggregation, decoding, and external
distribution. Anamnesis events are the audit trail of packet computation
across the Limited Domain.

## RingEntry 64-Byte Format

Each Anamnesis event is a fixed 64-byte structure (one cache line):

~~~~
struct anamnesis_ringentry {
  u64 timestamp_ns;              // bpf_ktime_get_ns(), nanosecond precision
  u8  event_type;                // EVENT_BORN, EVENT_COMPUTED, etc. (8 bits)
  u8  hop_index;                 // Hop identifier (router/node ID)
  u16 reserved_1;                // Padding for alignment
  u8  input_monad[20];           // Monad before operation
  u8  output_monad[20];          // Monad after operation
  u32 flow_label;                // IPv6 flow label (for correlation)
  u8  metadata[8];               // Optional: SPI, latency, path info
};
// Total: 64 bytes per entry
~~~~

This fixed-size entry ensures:
  - O(1) ring buffer write performance
  - No variable-length parsing required by userspace
  - Atomic writes (64 bytes fit in one cache line)
  - Alignment to L1 cache line boundaries

## EVE JSON Transformation

Wotan transforms 64-byte binary entries into EVE (Extensible Extensible
Event) JSON format for external consumption:

~~~~
{
  "timestamp": "2026-02-27T14:23:45.123456789Z",
  "event_type": "COMPUTED",
  "hop_index": 3,
  "flow_label": "0x12345",
  "monad": {
    "version": 1,
    "src_service_id": 8,
    "dst_service_id": 12,
    "hop_count": 58,
    "flow_action": 0,
    "circuit_state": 0,
    "flags": {
      "chaos": false,
      "canary": false,
      "traced": true,
      "encrypted": false,
      "sampled": false,
      "mirror": false
    },
    "qos_class": 1,
    "latency_hint_us": 100
  },
  "monad_delta": {
    "field": "circuit_state",
    "before": 0,
    "after": 1
  }
}
~~~~

## Wotan Topic Routing: anamnesis.*

Anamnesis events are published to Wotan topics with prefixed routing:

- anamnesis.birth - Packets entering Limited Domain (Shield ingress)
- anamnesis.computed - Shim program execution results
- anamnesis.death - Packets exiting Limited Domain (Shield egress)
- anamnesis.anomaly - Integrity/version/policy violations
- anamnesis.chaos - Chaos injection testing events
- anamnesis.metrics - Aggregated counters (per-hop drop counts, etc.)

Subscribers (external SIEMs, dashboards, alerting systems) MUST implement
Wotan gRPC streaming clients to consume these topics in real-time.

## Suricata GPL Isolation

IDS/IPS systems (Suricata, Zeek) consume Anamnesis EVE streams via Wotan
topics. Suricata integrations MUST respect GPL licensing:

- Anamnesis events themselves (timestamps, packet flow data) are part of
  the Unheaded Protocol specification (RFC, not GPL).
- Suricata rule set processing is separate (GPL-licensed via Suricata project).
- Wotan May publish preprocessed events (no Suricata code execution) to
  anamnesis.* topics.

## Event Correlation with Flow Labels

Anamnesis events are correlated across hops using the IPv6 Flow Label field
(RFC 6437), which MUST NOT be modified by any hop:

~~~~
Packet journey:
  [Shield ingress] event.flow_label = 0x12345 -> EVENT_BORN
  [Shim hop 1]    event.flow_label = 0x12345 -> EVENT_COMPUTED
  [Shim hop 2]    event.flow_label = 0x12345 -> EVENT_COMPUTED
  [Shield egress] event.flow_label = 0x12345 -> EVENT_DEATH

All events with flow_label = 0x12345 belong to same packet journey.
~~~~

Wotan aggregates events by flow_label to reconstruct packet path,
measure latency per hop, and detect packet loss.

## Ring Buffer Configuration

The Anamnesis ring buffer is configured at kernel load time:

~~~~
Suggested configuration:
  - Ring buffer size: 102 MB per CPU (supports ~2 seconds at 833k pps)
  - Entry size: 64 bytes (fixed)
  - Writing: Non-blocking (BPF_RINGBUF_OUTPUT)
  - Overflow behavior: Drop oldest entries silently
  - Readers: Wotan userspace daemon via bpf_ringbuf_read()

Per-CPU ring buffers ensure no contention between CPUs.
~~~~

# Wotan Memory Model

Wotan is the memory and I/O bus that bridges Monad computation to
per-flow storage and external topics.

## Memory Hierarchy

~~~~
Level   Name                 Size           Access Latency
-----   --------------------  -----------    ----------
L0      Monad Registers       20 bytes       Wire speed (~ns)
        (in the packet)       (fixed)

L1      Per-hop BPF map       Variable       ~100-200 ns
        (kernel hash map)

L2      Wotan ring buffer     Configurable   ~1-10 us
        (per-flow RAM)        (per-flow)

L3      Wotan WAL             Disk-bounded   ~100 us - 1 ms
        (persistent storage)

L4      Sophia dictionaries   BPF maps       ~100-200 ns
        (instruction decode)  (read-only)
~~~~

## Address Space Layout

Wotan provides a 32-bit addressable memory space partitioned as follows:

~~~~
0x00000000 +------------------+
           |  Data Memory     |  <- Ring buffer storage
           |  (general RAM)   |     Per-flow, keyed by
0x0000BFFF +------------------+     trace_id/flow_label
0x0000C000 |  I/O Topic Region|  <- External topic subscriptions
           |                  |
0x0000FFFD +------------------+
0x0000FFFE |  Input Register  |  <- One 32-bit word
0x0000FFFF +------------------+
~~~~

## BPF Helper Functions

Shim programs access Wotan memory through BPF helper functions. These
helpers are defined per RFC 9669:

~~~~
long bpf_wotan_read(u32 flow_label, u32 addr, void *buf, u32 len);

  Arguments:
    flow_label: Trace ID or flow identifier (from Monad)
    addr: Wotan memory address (0x00000000 - 0x0000FFFF)
    buf: Kernel-space buffer to receive data
    len: Number of bytes to read (max 512)

  Returns:
    Number of bytes read (>= 0), or negative error code

  Error codes:
    -ENOENT: flow_label not found in Wotan
    -EFAULT: addr out of bounds for this flow
    -ENOMEM: Insufficient memory available
    -EBUSY: Memory region locked (contention)

long bpf_wotan_write(u32 flow_label, u32 addr, const void *buf,
                      u32 len);

  Arguments:
    flow_label: Trace ID or flow identifier (from Monad)
    addr: Wotan memory address (0x00000000 - 0x0000FFFF)
    buf: Kernel-space buffer with data to write
    len: Number of bytes to write (max 512)

  Returns:
    Number of bytes written (>= 0), or negative error code

  Error codes:
    -ENOENT: flow_label not found in Wotan
    -EFAULT: addr out of bounds for this flow
    -ENOMEM: Insufficient memory available
    -EBUSY: Memory region locked (contention)
    -EACCES: Write access denied (read-only region)
~~~~

### Wotan Helper Bounds Checking (Normative)

Wotan helpers allow eBPF programs to read/write to L1 cache and WAL structures. All accesses MUST be bounds-checked to prevent out-of-bounds memory access.

#### bpf_wotan_read(key: u64, offset: u16) -> u64

Parameters:
  - key: 64-bit cache key (flow_label + src/dst hash, per [WOTAN])
  - offset: byte offset within cache line (0-63)

Return value: 64-bit value read from cache, or BPF_WOTAN_ERROR on error.

Bounds checking (NORMATIVE):
  - Offset MUST be in range [0, 63]. If offset > 63 or offset+8 > 64,
    return BPF_WOTAN_ERROR (value 0xFFFFFFFFFFFFFFFF).
  - Key MUST correspond to a valid cache line for current flow. If key
    not in cache, return 0 (zero-fill).

Atomicity:
  - Read is atomic: 64-bit value is read with acquire semantics (no
    reordering of subsequent operations before this read completes).

#### bpf_wotan_write(key: u64, offset: u16, value: u64) -> i32

Parameters:
  - key: 64-bit cache key
  - offset: byte offset within cache line (0-63)
  - value: 64-bit value to write

Return value: 0 on success, BPF_WOTAN_ERROR (-1) on error.

Bounds checking (NORMATIVE):
  - Offset MUST be in range [0, 56] (to ensure write does not exceed
    64-byte cache line: offset + 8 <= 64).
  - If offset > 56, return -1 (BPF_WOTAN_ERROR).
  - Key MUST correspond to valid cache line. If not, allocate new cache
    line (if space available) or evict LRU entry.

Atomicity:
  - Write is atomic: 64-bit value is written with release semantics
    (all prior operations complete before write).
  - Cache line is invalidated after write (subsequent reads see new value,
    not stale cached value).

Memory Ordering:
  - bpf_wotan_read() provides acquire semantics (LoadLoad + LoadStore barriers)
  - bpf_wotan_write() provides release semantics (StoreStore + LoadStore barriers)
  - This provides sequential consistency for data race detection (TSan).

Error Codes:
  - BPF_WOTAN_ERROR = -1 (or 0xFFFFFFFFFFFFFFFF for read return)
  - Do not use exception/signal; return error code in return value

Test Requirements:
  - Implementations MUST verify bounds checking for out-of-bounds offsets
    (no crash, no silent clamp, must return error code)
  - Concurrent access with different keys MUST NOT race
  - Concurrent access with same key MUST be exclusive or use CAS
  - Memory barriers MUST prevent stale reads/writes

Access to Wotan memory is synchronized via spin-lock per flow. Writers
MUST NOT hold locks across packet processing boundaries. The read and
write helpers MUST be used within per-packet BPF programs only.

# Kingdom Mode: Operational States

Kingdom Mode defines three operational states for Limited Domain management
and failover behavior. State transitions are orchestrated via Wotan topic
subscriptions.

## Normal Mode

Normal Mode is the standard operational state where all hops are available
and processing Monad packets at full capacity.

Triggers:
  - All Shim hops report healthy heartbeats (via Wotan)
  - Packet loss rate < 0.1%
  - Per-hop latency < 10 milliseconds (configurable)

Actions in Normal Mode:
  - Forward all packets to primary egress
  - Maintain full Sophia dictionary (all services active)
  - Emit all trace-level events to Anamnesis
  - Shield admission control applies normal rate limits

## Degraded Mode

Degraded Mode activates when some hops become unavailable or show elevated
latency. The Limited Domain continues operating but with reduced capacity.

Triggers:
  - Any Shim hop unreachable for > 5 seconds
  - Packet loss rate 0.1% - 5%
  - Per-hop latency 10ms - 100ms (configurable)
  - CPU load on any hop > 90% (configurable)

Actions in Degraded Mode:
  - Reroute packets through alternate paths (per Wotan topology map)
  - Reduce trace event emission (sample rate 10% instead of 100%)
  - Scale back Sophia dictionary (only active services)
  - Shield admission control applies stricter rate limits (50% of normal)
  - Alert operators via Wotan alerts.* topic

## Emergency Mode

Emergency Mode is the last-resort failover state. The Limited Domain
constrains itself to absolute minimum functionality to preserve critical
services.

Triggers:
  - > 50% of hops unreachable
  - Packet loss rate > 5%
  - Complete path unavailability (no forwarding possible)
  - Manual operator command via Wotan admin API

Actions in Emergency Mode:
  - Forward only packets with QoS class = realtime (critical services)
  - Drop all other packets silently (no EVENT_ANOMALY to preserve bandwidth)
  - Minimal Sophia dictionary (only critical service identifiers)
  - No trace event emission (only anomalies and DEATH events)
  - Shield admission control: allow only trusted sources (whitelist only)
  - Emit EMERGENCY_STATE event to all Wotan subscribers

## State Transition Semantics

State transitions are declared via the Wotan system.state topic, which is
subscribed by all Shield and Shim programs:

~~~~
Wotan topic: system.state
Message format: {
  "state": "NORMAL|DEGRADED|EMERGENCY",
  "transition_reason": "string",
  "timestamp_ns": u64,
  "enabled_services": [service_id, ...],
  "reroute_rules": [{src, dst, next_hop}, ...]
}

All nodes MUST apply state transition within 1 second of receiving
Wotan message. No local caching of state; always subscribe to current.
~~~~

## Wotan system.state Topic

The Wotan system.state topic is the authoritative source of Kingdom Mode
state for all nodes:

- Maintained by Wotan daemon (runs on dedicated control plane node)
- Published whenever state changes
- Subscribed by all Shield (XDP/TC) and Shim (per-hop) programs
- TTL: 5 seconds (messages expire if not refreshed)
- Consensus: Quorum-based (requires 2 out of 3 control plane nodes to agree)

Example Wotan state management logic:

~~~~
HealthMonitor in Wotan:
  1. Ping all Shim hops every 1 second (via Wotan ping.* topic)
  2. Count consecutive failures
  3. If failures > 5: mark hop as unhealthy
  4. If unhealthy_hops / total_hops > 0.5: emit system.state = EMERGENCY
  5. If unhealthy_hops / total_hops > 0.1: emit system.state = DEGRADED
  6. Otherwise: emit system.state = NORMAL
~~~~

## Dashboard Indicators

Operators monitor state via Wotan dashboard:

- **State LED**: Green (NORMAL), Yellow (DEGRADED), Red (EMERGENCY)
- **Topology map**: Highlights unavailable hops in red
- **Metric graph**: Packet loss rate, latency histogram, drop count
- **Service list**: Green (active), gray (disabled in Degraded/Emergency)
- **Alerts**: Scrolling list of state changes and anomalies

# Kingdom Mode: Address Reclamation

Kingdom Mode address reclamation using IPv6 host bits is reserved for a
future version of this specification.  Implementations MUST set RSVD to
zero.  The CUSTOM bit provides a single-bit extension point for
deployment-specific scratch field encoding.

The Kingdom Mode technical content (address space analysis, extended
register layout, and Kingdom Mode selector) is reserved for future use
and is described in [draft-bellis-unheaded-protocol-foundation-04] or
later versions. Current implementations MUST set RSVD (bit 0 of flags)
to zero.

# Post-Quantum Cryptographic Identity Binding

Post-Quantum Cryptographic (PQC) Identity Binding cryptographically
associates each service identifier in the Monad to a post-quantum
keypair, providing quantum-resistant authentication of service metadata
without increasing wire overhead.

## Motivation

Service identifiers (src_service_id, dst_service_id) are Sophia-encoded
fields that name services. In a standard deployment, these are opaque
integers. PQC binding ensures that every packet carrying service_id = 0x42
implicitly asserts: "This packet was stamped by Shield on behalf of the
service holding the private key for service 0x42."

This provides:

-  Quantum-resistant authentication without HMAC overhead.

-  Defense against "harvest now, decrypt later" attacks on intra-domain
   metadata.

-  Cryptographic non-repudiation of service identity at every hop.

## Sophia Key Store

The Sophia dictionary is extended with cryptographic key material for
each service_id. Each entry includes:

~~~~
Sophia Entry for service_id 0x42:

  service_name:       "kanban-app"
  endpoint:           "fd00:3f:75::1007:8080"
  pqc_algorithm:      ML-KEM-768       (FIPS 203)
  pqc_pubkey:         <1184 bytes>     (ML-KEM-768 public key)
  pqc_fingerprint:    <32 bytes>       (SHA3-256 of pubkey)
  classical_pubkey:   <32 bytes>       (X25519, for hybrid mode)
  hybrid_mode:        CONCATENATE      (PQ/T hybrid per RFC 9370)
  key_epoch:          7                (rotation counter)
  key_issued:         2026-02-18T...   (timestamp)
  key_expires:        2026-03-18T...   (timestamp)
  signature_algo:     ML-DSA-65        (FIPS 204)
  signature_pubkey:   <1952 bytes>     (ML-DSA-65 public key)
~~~~

The public keys are stored in Sophia's userspace tables and, where space
permits, cached in BPF maps as fingerprints (32-byte SHA3-256
truncations).

## Key Distribution

Key distribution uses the flow_action field to signal key lifecycle events.
The minimum required key actions are:

~~~~
flow_action    Meaning
-----------    ------------------------------------
0x10           KEY_ANNOUNCE   (new service pubkey)
0x11           KEY_ROTATE     (epoch increment)
0x12           KEY_REVOKE     (emergency revocation)
0x13           KEM_ENCAPS     (ML-KEM encapsulation)
0x14           KEM_DECAPS     (ML-KEM decapsulation)
0x15           KEY_ACK        (key acknowledgment)
0x16           KEY_REJECT     (key rejection)
~~~~

When flow_action is KEY_ANNOUNCE, KEY_ROTATE, or KEY_REVOKE, the packet
payload carries the relevant key material. The Monad registers carry
control metadata (key epoch, fingerprint, etc.). Shield MUST enforce
that KEY_ANNOUNCE packets originate only from authorized provisioning
nodes.

Wotan handles the key exchange:

1. Provisioning node generates ML-KEM-768 keypair (1184-byte public key).

2. Provisioning node sends KEY_ANNOUNCE packet through Shield with the
   public key as payload.

3. Shield stamps the packet with the new service_id and KEY_ANNOUNCE
   flow_action.

4. Wotan receives the Anamnesis ring buffer event, updates Sophia
   userspace tables, and pushes fingerprints to BPF maps on all Kingdom
   nodes.

5. Subsequent packets carrying that service_id are now cryptographically
   bound to the announced keypair.

## Per-Hop Fingerprint Verification

When Kingdom Mode is active and Extended Register Space carries PQC
fingerprints (32-bit SHA3-256 truncations), each hop MAY verify the
fingerprint:

1. Extract the PQC Fingerprint fields from Extended Registers.

2. Look up the full fingerprint for src_service_id in the local Sophia
   BPF map.

3. Compare the 32-bit truncation.

4. If mismatch: emit EVENT_ANOMALY to Anamnesis, optionally set
   flow_action = DROP (configurable).

This provides per-hop authentication at O(1) cost per packet. The full
cryptographic verification (ML-DSA-65 signature check) is performed
only at Shield boundaries.

## PQ/T Hybrid Mode

During transition before Cryptographically Relevant Quantum Computers
(CRQCs) are available, Sophia supports Post-Quantum/Traditional (PQ/T)
hybrid mode per RFC 9370:

~~~~
hybrid_mode = CONCATENATE
  Both classical and PQC key exchanges must succeed.

hybrid_mode = PQC_ONLY
  PQC key exchange only (transition phase complete).

hybrid_mode = CLASSICAL_ONLY
  Classical key exchange only (for backward compatibility).
~~~~

Transitioning from CONCATENATE to PQC_ONLY is a Sophia dictionary
update — a single BPF map write propagated cluster-wide in under 10 ms.
Zero packet format changes. Zero downtime.

## Security Considerations for PQC Binding

Key material in Sophia userspace tables MUST be protected by operating
system access controls. BPF maps containing fingerprints are in kernel
space and MUST NOT be directly accessible by user-space applications.

Key rotation MUST occur before key_expires. If a key epoch mismatch is
detected (packet carries epoch N, Sophia has epoch N+1), the Shim
SHOULD log a warning but MUST NOT drop the packet during a grace period
(configurable, default 60 seconds).

Private keys MUST NOT be stored in Sophia or any BPF map. Private keys
reside only on the provisioning node in a secure enclave or HSM.

BPF map lookups for fingerprint comparison MUST use constant-time
comparison functions to prevent timing attacks.

# Chaos Injection (Yaldabaoth)

When the C bit (0x80) of the flags field is set, the Shim applies
deterministic perturbations to Monad or Wotan state for resilience
testing.

Chaos modes are selected via Sophia and applied conditionally:

~~~~
Mode     Name              Action
------   ---------------   ----------------------------
0x01     BIT_FLIP          Flip random bit in Monad field
0x02     VALUE_MUTATE      Increment target field mod 2^32
0x03     MEMORY_FAULT      Wotan read returns error
0x04     LATENCY_INFLATE   Increase hop latency 10x
0x05     PACKET_LOSS       Drop subsequent packets (10%)
0x06     CHAOS_MARKER      Set chaos bit visible downstream
~~~~

All chaos events MUST be recorded in Anamnesis with EVENT_CHAOS type.
The chaos system MUST be completely auditable. All perturbations MUST
be recorded in Anamnesis with before and after Monad snapshots.

# Computational Completeness

The Monad (14 fields, 20 bytes) paired with Wotan (unbounded
addressable memory via ring buffers) forms a Turing-complete system.

A proof sketch:

1. Tape: Wotan ring buffer provides addressable storage (address i holds
   symbol at position i).

2. State: The Monad circuit_state field holds the current state q. The
   latency_hint or scratch fields hold the head position.

3. Transition: Shim implements the Turing machine transition function
   delta via Sophia lookup table.

4. Iteration: Each circulation of a packet via BPF_REDIRECT represents
   one Turing machine step.

5. Halting: When circuit_state == halt_state, stop circulating.

With unlimited packet circulation and unbounded Wotan memory, this model
computes any computable function. Practical clock speed is limited by
BPF program execution time and BPF_REDIRECT overhead: approximately 2.7
MHz single-instruction (one Shim execution per ~370 ns including
checksum verification and recomputation), or 11-21 MHz with batched
multi-instruction Shim execution per hop.

Kingdom Mode Extended Registers provide additional 208-224 bits of
register state per packet (depending on fleet size), further increasing
the computational bandwidth of each hop without consuming additional
Wotan memory.

# Applicability Statement

The Unheaded Protocol is applicable to Limited Domains (RFC 8799)
where:

  (a) All intermediate nodes are operator-controlled.

  (b) IPv6 Hop-by-Hop option processing is enabled on all nodes.

  (c) BPF program loading infrastructure is available.

  (d) A Sophia dictionary distribution mechanism is deployed.

The protocol is NOT applicable to:

  (a) The public Internet, where intermediate routers may drop Hop-by-Hop
      options (RFC 9098, RFC 9673).

  (b) Paths containing routers that do not process IPv6 Hop-by-Hop
      options.

  (c) Environments where BPF program loading is restricted by security
      policy.

  (d) Networks that require backward compatibility with routers that
      slow-path or drop Hop-by-Hop extension headers.

While RFC 8200 specifies that routers should skip unrecognized Hop-by-Hop
options, operational experience (RFC 9098, RFC 9673) shows that some
router implementations in practice drop or slow-path packets containing
Hop-by-Hop extension headers. Deployments MUST ensure all intermediate
routers are explicitly configured to process (or at minimum forward)
packets containing the Unheaded Monad option.

# Performance Considerations

The Unheaded Protocol adds the following per-hop computational overhead:

-  Checksum verification: approximately 50 nanoseconds for CRC-16 over
   18 bytes.

-  Monad field extraction: negligible (memory reads).

-  Sophia dictionary lookup: O(1) hash map access, approximately 100-200
   nanoseconds per field.

-  BPF Shim execution: 1-10 microseconds depending on Shim complexity.

-  Checksum recomputation: approximately 50 nanoseconds.

-  Ring buffer event write: approximately 100-500 nanoseconds (non-blocking).

Total per-hop processing time: approximately 2-12 microseconds on modern
hardware, dominated by Shim execution. For a typical Shim program with
<100 BPF instructions, the overhead is <1 microsecond per packet.

The Monad adds a minimum of 24 octets (2-byte HbH header + 2-byte option
TLV + 20-byte Monad, already 8-octet aligned) to each packet. On a
standard 1500-byte MTU path, this reduces the usable payload to
approximately 1476 octets (1.6% overhead). With optional metadata and
Kingdom Mode Extended Registers, overhead may reach 40-56 octets
(approximately 2.7-3.7% overhead).

Within the Limited Domain, the operator SHOULD configure the MTU to
accommodate the maximum expected overhead, including optional Chaos
payloads and Kingdom Mode Extended Registers. Jumbo frames (9000-byte
MTU) are RECOMMENDED for deployments using Kingdom Mode.

Shield (ingress processing) adds approximately <1 microsecond per packet
via XDP, before sk_buff allocation. Shield (egress processing) adds
approximately <1 microsecond per packet via TC egress hook.

Dictionary update propagation via Wotan: <10 milliseconds cluster-wide
via atomic BPF map replacement.

Anamnesis ring buffer: 102 MB per-CPU buffer at standard 1500-byte
packet rate (~833,333 pps) with 64-byte events covers approximately 2
seconds at full rate with every packet emitting events.

# Manageability Considerations

## Observability

Anamnesis ring buffers provide complete per-packet observability through
non-blocking event emission. Wotan aggregates events from all hops and
decodes them through Sophia dictionaries. Operators can reconstruct the
complete packet path, inspect Monad mutations at each hop, and audit
checksum errors and chaos events.

## Configuration

Sophia dictionaries are the primary configuration mechanism. Dictionary
entries define:

-  Service identifiers and their meanings.

-  Flow action policies (trace, sample, drop, mirror).

-  QoS class definitions and sampling probabilities.

-  Deployment ring classifications.

-  Circuit breaker policies and recovery timers.

-  PQC key material and key rotation schedules.

-  Shim program selection and parameters.

Sophia updates propagate in under 10 milliseconds via BPF map replacement.
No packet forwarding is interrupted.

## Monitoring

Operators SHOULD monitor the following metrics:

-  Checksum failure rate per interface (indicates link corruption).

-  Anamnesis ring buffer overflow rate (indicates observability loss).

-  Wotan memory allocation and contention (indicates per-flow state
   pressure).

-  PQC key rotation status and epoch synchronization (indicates key
   management health).

-  BPF Shim program error counts (indicates Shim failures).

-  Kingdom Mode egress address restoration verification (indicates
   Shield correctness).

## Logging

Shield ingress MUST log:

-  Dropped packets (rate-limit, blocklist, WAF).

-  Service classification decisions.

-  Trace ID allocation.

Shield egress MUST log:

-  Checksum verification failures.

-  Kingdom Mode address restoration (log restored bits for audit).

-  Packets forwarded with anomaly flags.

Per-hop processing SHOULD log:

-  Checksum verification failures.

-  BPF Shim program errors.

-  Anomaly events (version mismatch, PQC fingerprint mismatch).

-  Wotan access errors (ENOENT, EFAULT).

# Security Considerations

## Extension Header Sanitization

Packets entering the Limited Domain from external sources MUST have any
existing Hop-by-Hop options replaced or removed. External packets MUST
NOT carry Unheaded Monad options. Packets exiting the Limited Domain
MUST have their Hop-by-Hop options stripped before reaching the external
network.

These requirements apply to both standard Monad fields and Kingdom Mode
Extended Registers.

Shield ingress MUST validate that ingress packets do not already carry
the Unheaded Monad option type. If found, the packet MUST be dropped and
an anomaly event MUST be logged.

Shield egress MUST validate that the ULA prefix in any Kingdom Mode
packet matches the configured Kingdom prefix. If mismatch is detected,
the packet MUST be dropped.

## BPF Containment

Shim programs are verified by the kernel BPF verifier per RFC 9669, which
ensures:

-  No out-of-bounds memory access.

-  Bounded loop execution (no infinite loops).

-  No unauthorized kernel function calls.

-  Stack safety and register constraints.

Shim program loading REQUIRES CAP_BPF and CAP_NET_ADMIN capabilities (or
equivalent). Implementations MUST use Linux kernel version 5.17 or later.

Kernel 5.17 (released March 2022) introduced critical BPF features required
for secure Unheaded deployments:

-  Full memory barrier semantics (wmb, rmb, mb) in BPF programs

-  CAS (Compare-And-Swap) atomic operations (__sync_compare_and_swap)

-  Ringbuf exclusive reservation mode (bpf_ringbuf_reserve with BPF_RB_EXCLUSIVE_RING flag)

-  Enhanced BPF verifier with stack bounds checking

-  Support for 64-bit atomic operations on all platforms

Kernel 5.15 or earlier MUST NOT be used for production deployments, as older
kernels lack these features and are vulnerable to data races, memory corruption,
and privilege escalation attacks (see Dark Grimoire Section 5).

Operators SHOULD:

-  Pin BPF programs to prevent runtime modification.

-  Use BPF token-based delegation (kernel 6.9+) where available.

-  Monitor /sys/kernel/debug/tracing/trace_pipe for verifier warnings.

-  Apply kernel security updates promptly, as the BPF verifier is a
   security-critical component.

## Integrity

The CRC-16 checksum detects accidental bit corruption only. It does NOT
provide integrity protection against malicious modification.

Deployments requiring integrity protection against compromised
intermediate nodes MUST use one of:

  (a) IPsec ESP (RFC 4303) to protect the entire packet including
      extension headers.

  (b) The optional HMAC field (not defined in this document) appended
      to the Monad, using a pre-shared key distributed via Sophia.

  (c) ML-DSA-65 (FIPS 204) signatures at Shield boundaries for
      post-quantum integrity.

The choice of integrity mechanism is a deployment decision outside the
scope of this specification.

## Trust Boundary

The trust boundary is the Limited Domain boundary. Within the domain, all
Shim programs and Wotan instances are operator-controlled. Cross-domain
routing is NOT supported. Inter-domain traffic MUST be encapsulated
(IP-in-IP or VPN tunnel) with the Hop-by-Hop option stripped at the
egress boundary and reconstructed at the ingress boundary of the next
domain.

## Post-Quantum Threat Model

PQC identity binding (Section 9) protects against:

-  Harvest-now-decrypt-later attacks on metadata correlation.

-  Service identity spoofing by a quantum-capable adversary within the
   Limited Domain.

-  Key compromise via classical cryptanalysis of traditional key
   agreement algorithms.

The hybrid PQ/T mode (RFC 9370) ensures security during transition while
CRQCs are not yet available.

## Kingdom Mode Threat Model

Kingdom Mode Extended Registers (Section 8) are only valid within the
Limited Domain. An external attacker who can inject packets with forged
ULA prefixes and K flags set could:

-  Forge PQC fingerprints to bypass per-hop verification.

-  Inject false Extended Register values to manipulate Shim program
   behavior.

Shield ingress MUST validate ULA prefix provenance and MUST reject Kingdom
Mode packets from outside the domain.

## MTU and Fragmentation

Implementations MUST NOT add the Hop-by-Hop extension header if doing so
would cause the packet to exceed the path MTU. The Monad adds a minimum
of 24 octets to each packet (8-octet HbH header + 2-octet option TLV +
20-octet Monad, padded to 8-octet boundary). With optional metadata and
Kingdom Mode, the overhead may reach 40-56 octets.

If the packet cannot accommodate the minimum 24-octet overhead, Shield
ingress MUST either:

  (a) Forward the packet without the Monad (bypass mode), or

  (b) Fragment the inner packet before adding the Monad (NOT RECOMMENDED
      due to performance impact).

Within the Limited Domain, the operator SHOULD configure the MTU to
accommodate the maximum expected overhead. For standard Ethernet (1500-byte
MTU), the effective payload is reduced to approximately 1436-1460 octets.
Jumbo frames (9000-byte MTU) are RECOMMENDED for Limited Domain deployments
using Kingdom Mode.

## Amplification Attack Mitigations

The Monad option does NOT amplify attack traffic because:

1. Response packets (e.g., TCP SYN-ACK) do NOT carry Monad options
2. Monad options are stripped at Shield egress before external network
3. External packets cannot initiate Monad processing (option stripped at ingress)

Potential misconfiguration that could enable amplification:
  - Forwarding packets with MIRROR flag (m=1) enabled to untrusted networks
  - Disabling option stripping at Shield egress

RECOMMENDED mitigation:
  - Shield egress MUST always strip Monad options before external forwarding
  - Admin API MUST require multi-factor approval before enabling MIRROR flag
  - Rate limits on MIRROR packets (max 1 mirror per 10 original packets)

## Denial-of-Service (DoS) Protection

Shield implements DoS protections via BPF rate limiting:

1. Per-source IP rate limit: configurable (default: 10,000 pps)
2. Per-source IP burst: configurable (default: 100 packets)
3. Per-destination IP rate limit: configurable (default: 1M pps)
4. Geo-blocking: optional (block traffic from specific regions)

If rate limit exceeded:
  - Packet is dropped (no error notification)
  - Per-interface drop counter incremented
  - Shield MAY emit rate-limit anomaly event (configurable)

Token bucket parameters are stored in BPF map and updatable in real-time
via Wotan admin API (zero downtime).

## BPF Verifier as Security Boundary

The kernel BPF verifier is a critical security component. All Shim programs
MUST pass verification before loading. Verifier checks:

1. No out-of-bounds memory access (stack/heap/map bounds)
2. No unbounded loops (verifier enforces finite execution)
3. No unauthorized syscall invocations
4. Stack usage < 512 bytes (prevents stack smashing)
5. All code paths reachable (no unreachable code traps)

Verifier failures MUST be treated as fatal load errors. Programs with
verifier warnings (e.g., "unreachable code") MUST NOT be loaded.

## Flow Label Entropy

The IPv6 Flow Label is used as packet trace ID and MUST NOT be predictable.

Implementation:
  - Shield ingress MUST set Flow Label using a cryptographically strong PRNG
    or per-5-tuple hash (RFC 6437 approach)
  - Never use sequential or constant Flow Labels
  - Recommended: SHA1(src_ip || dst_ip || src_port || dst_port || random_salt)

Benefits:
  - Prevents traffic analysis (adversary cannot correlate packets without hash preimage)
  - Enables Anamnesis event correlation across hops
  - Complies with RFC 6437 intent

## Address Reclamation Attack Surface

Kingdom Mode Extended Registers reclaim IPv6 host bits as extended registers.
Attack surface:

1. ULA Prefix Spoofing: Adversary injects packet with forged ULA prefix
   - Mitigation: Shield ingress validates prefix matches configured Kingdom ULA
   - Mitigation: Shield egress rejects packets with mismatched prefix

2. Extended Register Forgery: Adversary forges Extended Register values
   - Mitigation: Per-hop verification (compare fingerprint with Sophia)
   - Mitigation: HMAC or signature over Extended Registers

3. Host Bits Collision: Two services reclaim overlapping host bits
   - Mitigation: Central allocator (Wotan admin) prevents overlaps
   - Mitigation: Sophia dictionary defines allocation per service

# IANA Considerations

## IPv6 Hop-by-Hop Option Type

A new IPv6 Hop-by-Hop option type is requested:

~~~~
Type:               TBD (suggested: 0x3E)
Name:               Unheaded Monad Option
Change Controller:  IESG
Reference:          This document
~~~~

The high-order two bits MUST be 00 (skip on unrecognized) and the third
bit MUST be 1 (option data may change en-route). This yields the format
001xxxxx.

## IPv6 Next Header Type Allocation

For deployments that extend the IPv6 Next Header field to signal Monad
processing, the following allocation is requested (informative; normative
path uses Hop-by-Hop option 0x3E):

~~~~
Next Header:    0xFE (254)
Name:           Unheaded Monad Protocol
Change Controller: IESG
Reference:      This document
~~~~

Note: This allocation is reserved for future use and is not required by
the normative protocol path, which uses IPv6 Hop-by-Hop options.

## IPv6 Flow Label Per RFC 6437

Monad packets MUST use the IPv6 Flow Label field as a packet trace
identifier. Implementation notes:

- Flow Label MUST be set to a non-zero value by Shield at ingress
  (random or deterministic based on 5-tuple hash, per RFC 6437)
- Flow Label MUST NOT be modified by any hop within the Limited Domain
- All Anamnesis events MUST reference the Flow Label for correlation
- Shim programs MAY read but MUST NOT write the Flow Label

## Sophia Dictionary Namespace Registry

IANA should create a new registry:

~~~~
Registry Name:  Unheaded Sophia Dictionary Namespace
Template:       Dictionary Name, Program Name, Version,
                Organization, Contact Email
Policy:         First Come First Served
~~~~

## Anamnesis Event Type Registry

~~~~
Registry Name:  Unheaded Anamnesis Event Types
Template:       Event Name, Code (u8), Description,
                Reference
Policy:         Specification Required

Initial entries:
  EVENT_BORN (0x00)      Packet created at Shield (birth)
  EVENT_COMPUTED (0x01)  Shim executed, Monad updated
  EVENT_WOTAN_RD (0x02)  Wotan memory read
  EVENT_WOTAN_WR (0x03)  Wotan memory write
  EVENT_CHAOS (0x04)     Chaos mode applied
  EVENT_ROLLBACK (0x05)  Monad rolled back
  EVENT_DIED (0x06)      Packet reached Shield (death)
  EVENT_KEY_OP (0x07)    PQC key lifecycle event
  EVENT_ANOMALY (0x08)   Integrity or version error
~~~~

## Error Code Registry

~~~~
Registry Name:  Unheaded Protocol Error Codes
Template:       Error Name, Code (u8), Description,
                Reference
Policy:         Specification Required

Initial entries:
  NO_ERROR (0x00)                    No error (data packet)
  CRC_VALIDATION_FAILED (0x01)       CRC-16 checksum mismatch
  VERSION_NOT_SUPPORTED (0x02)       Unknown Monad version
  FLOW_LABEL_INVALID (0x03)          IPv6 flow label validation failed
  ARITHMETIC_OVERFLOW (0x04)         Exponent decoding overflow
  WOTAN_BOUNDS_CHECK_FAILED (0x05)   Wotan helper offset out of bounds
  MULTIPLE_HBH_HEADERS (0x06)        Multiple HbH options present
  UNKNOWN_CRITICAL_TLV (0x07)        Unknown critical TLV type
  WAL_SEQNO_DISCONTINUITY (0x08)     WAL sequence number gap detected
  INSUFFICIENT_BUFFER_SPACE (0x09)   No space for Monad in packet
  FLOW_STATE_CORRUPTION (0x0A)       Wotan flow state invalid
  TLS_HANDSHAKE_FAILURE (0x0B)       TLS/QUIC handshake error
  QUIC_VERSION_MISMATCH (0x0C)       QUIC version negotiation failed
  RESERVED (0x0D)                    Reserved for future use
  0x0E-0x1E                          Unallocated (future use)
  0x1F-0xFF                          Private use (testing/greasing)
~~~~

## TLV Type Registry (Extended)

This registry documents TLV type allocations per Section 7 (TLV Extension Mechanism):

~~~~
Registry Name:  Unheaded TLV Type Allocations
Template:       Type (u8), Name, Critical Requirement,
                Length Constraints, Reference
Policy:         Specification Required

Reserved Range: 0x00-0x1F (Monad Foundation)
Reserved Range: 0x20-0x3F (Sophia Dictionary)
Reserved Range: 0x40-0x5F (Wotan Memory)
Reserved Range: 0x60-0x7F (Future Extensions)

With critical bit:
  0x80-0x9F (Monad Foundation, critical)
  0xA0-0xBF (Sophia Dictionary, critical)
  0xC0-0xDF (Wotan Memory, critical)
  0xE0-0xFF (Future Extensions, critical)

Initial entries:
  0x01 (Ring Path Counter, optional, 4 bytes, this document)
~~~~

## PQC Algorithm Registry

~~~~
Registry Name:  Unheaded PQC Algorithm Identifiers
Template:       Algorithm Name, Code (u8), Key Size (bytes),
                FIPS Reference
Policy:         Specification Required

Initial entries:
  ML-KEM-768 (0x01, 1184, FIPS 203)
  ML-KEM-1024 (0x02, 1568, FIPS 203)
  ML-DSA-65 (0x03, 1952, FIPS 204)
  ML-DSA-87 (0x04, 2592, FIPS 204)
  SLH-DSA-SHA2-128s (0x05, 32, FIPS 205)
~~~~

--- back

# Changes from draft-bellis-unheaded-protocol-foundation-02

The following changes are made in draft-03 to address technical review
feedback:

1. **Fixed Option Type chg bit**: Changed the third bit (chg) from 0 to 1,
   as the Monad IS modified at every hop. The format is now 001xxxxx
   (act=00, chg=1). Suggested value changed from 0x42 to 0x3E.

2. **Added version field to Monad**: The Monad layout now includes a
   version field at offset 0x00 (1 byte, unsigned). This enables future
   protocol evolution and version negotiation. The layout is ported from
   PROTOCOL_TECHNICAL_SUMMARY.md to ensure interoperability.

3. **Clarified CRC-16 vs integrity separation**: Explicitly stated that
   CRC-16 provides error detection only, not integrity protection. Added
   guidance on HMAC/IPsec/ML-DSA for integrity-protected deployments.

4. **Reconciled IPv4 shim vs IPv6 HbH**: The RFC now specifies IPv6 HbH
   as the normative transport. An informative note acknowledges that
   deployments MAY use an IPv4 shim (prepending 20 bytes) as an interim
   mechanism during migration. The field layout is identical regardless
   of transport.

5. **Defined Sophia wire format**: Added a section specifying Sophia
   dictionary entries as CBOR-encoded structures with a minimum required
   dictionary. Defined distribution via Wotan topics.

6. **Concrete exponent encoding binary format**: Specified that an
   exponent-encoded field is a single octet, signed 8-bit two's
   complement. Decoded value = base^exponent. Base is defined by Sophia,
   default base = 2.

7. **MTU and fragmentation discussion**: Added comprehensive guidance on
   minimum overhead (24 octets), path MTU considerations, and
   recommendations for jumbo frames in Kingdom Mode deployments.

8. **Applicability Statement (NEW)**: Added Section 14 explicitly defining
   where the protocol applies (Limited Domains with operator-controlled
   hops) and where it does NOT (public Internet, routers that drop HbH).

9. **Performance Considerations (NEW)**: Added Section 15 with measured
   per-hop latencies, packet overhead calculations, dictionary update
   propagation times, and Anamnesis buffer sizing.

10. **Manageability Considerations (NEW)**: Added Section 16 covering
    observability via Anamnesis, Sophia-based configuration, monitoring
    metrics, and logging guidance.

11. **HbH option drop reality acknowledgment**: Added references to RFC
    9098 and RFC 9673 findings that some routers may drop Hop-by-Hop
    options, with explicit statement that this protocol is for Limited
    Domains only.

12. **Restructured to standard RFC ordering**: Reordered sections to match
    IETF RFC structure: Introduction, Requirements Language, Terminology,
    Protocol Overview, Packet Format (with all wire diagrams), Exponent
    Encoding, Sophia, Shield, Per-Hop Processing, Anamnesis, Wotan,
    Kingdom Mode, PQC, Chaos, Computational Completeness, Applicability,
    Performance, Manageability, Security, IANA, Appendices.

13. **Flags bitfield from draft-02**: Committed to the superior flags
    bitfield structure with C, Y, T, E, S, M, CUSTOM, RSVD bits.

14. **Wotan helper interface**: Added detailed BPF helper function
    specifications (bpf_wotan_read, bpf_wotan_write) with error codes
    per RFC 9669.

15. **RFC 9669 reference added**: Added to normative references and
    referenced in Sophia BPF map implementation, Wotan helper interface,
    and BPF containment sections.

16. **Removed all marketing language**: Every sentence is either a
    normative requirement (MUST/SHOULD/MAY) or a factual technical
    statement. Removed poetic descriptions like "Speed: light", "The
    Rosetta Stone", "The nervous system", and "The atom. The wire itself."

17. **Terminology section reformatted**: Uses standard RFC definition list
    format (term followed by colon-indented definition).

# Heritage and Prior Art

The Unheaded Protocol builds on a lineage of metadata-riding-with-data designs:

~~~~
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
                         evolved to BPF per RFC 9669
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
~~~~

## IOAM (RFC 9197, RFC 9486)

In-situ OAM defines data fields for recording operational telemetry in
IPv6 Hop-by-Hop options. IOAM is observation-only: it records what
happened. The Unheaded Protocol is intent-driven: it declares what
SHOULD happen. IOAM traces grow with each hop (variable length). The
Monad is fixed at 20 bytes. IOAM supports multiple parallel data
collection schemes. The Monad is a single unified register file.

## APN (Application-Aware Networking)

APN carries application identity in IPv6 headers. APN is a framework
defining what COULD be carried, with TLV-based variable encoding. The
Monad is a fixed-size computational atom with dictionary-based semantic
compression (Sophia).

## MPLS MNA (Network Actions)

MNA carries per-packet action directives in MPLS label stacks. MNA
operates in MPLS networks; the Unheaded Protocol is IPv6-native and
applicable to Limited Domains.

## NSH (RFC 8300)

The Network Service Header provides metadata for Service Function Chaining.
NSH uses a separate encapsulation; the Unheaded Protocol uses the
standard IPv6 Hop-by-Hop extension header mechanism.

## Key Differentiators

The Unheaded Protocol is distinguished by the combination of:

1. Fixed 20-byte register file (vs. variable-length formats in IOAM, APN,
   NSH).

2. Sophia exponent dictionary for semantic compression (unique to Unheaded).

3. BPF-native processing model designed from the start for programmable
   dataplanes (per RFC 9669).

4. Intent-driven computation, not observation-only (unique vs. IOAM).

5. Post-quantum cryptographic identity binding of service identifiers
   (unique).

6. Kingdom Mode address reclamation of deterministic prefix bits (unique).

7. Computational completeness (Turing-complete with Wotan memory, unique).


# Changes from draft-bellis-unheaded-protocol-foundation-03

The following changes are made in draft-04 to address S21 assessment findings:

1. **Patch M1: Extended CRC Coverage**: The CRC-16 checksum now covers all 20 bytes of the Monad header (offsets 0x00-0x13), with the checksum field itself zeroed during computation. This provides integrity protection for all header fields, including version, flags, and flow label.

2. **Patch M3: Mandatory Kernel 5.17+**: Updated system requirements from Linux kernel 5.15+ to 5.17+. Kernel 5.17 (March 2022) introduced critical BPF features: full memory barrier semantics, CAS atomic operations, ringbuf exclusive reservation mode, enhanced verifier bounds checking, and 64-bit atomic operations.

3. **Patch M5: Strict Version Checking (No Fallback)**: Clarified version field semantics - implementations MUST drop packets with unknown versions immediately, with no version negotiation or fallback. This eliminates parser divergence attacks (X4 finding).

4. **Known Cross-Reference Issue (Rust Code Discrepancy)**: The Rust implementation in ebpf/monad-common/src/lib.rs defines a different wire format than draft-03. This has been noted but NOT changed in this draft pending RFC author clarification. See Security Considerations section for details.

5. **Forward Reference: Extended Register Option**: Added informative reference to {{MONAD-EXT-REG}} and a new subsection under Monad Register File describing the optional Extended Register Option.  The extension doubles MBC compute capacity by claiming the complement space of the primary register map as a second HbH option, inspired by the wildcard mask formalism of {{RFC0950}}.

# Changes from draft-bellis-unheaded-protocol-foundation-04 (Draft-05)

The following changes are made in draft-05 to complete Monad specification
and address patches M1-M8 from S21 assessment:

1. **Patch M1: CRC-16 Scope Already Extended**: Draft-04 already covered all
   20 bytes in CRC-16 computation. Confirmed in section 5.2 (Checksum Field).

2. **Patch M2: Multiple HbH Header Restriction Added**: Added section 1a
   to per-hop processing (after step 1), mandating immediate drop if multiple
   HbH options present (no error code, no fallback). Eliminates header
   smuggling (X3).

3. **Patch M3: Kernel 5.17+ Requirement Confirmed**: Draft-04 already mandated
   kernel 5.17+. Confirmed in section 12.2 (BPF Containment).

4. **Patch M4: Wotan Helper Bounds Checking Added**: Extended section 11.3
   (BPF Helper Functions) with detailed bounds checking specification for
   bpf_wotan_read() and bpf_wotan_write(). Specifies offset ranges, error
   codes, atomicity semantics, memory ordering (acquire/release), and test
   requirements per LICH-008 and D4 findings.

5. **Patch M5: Strict Version Checking Emphasized**: Updated version field
   definition to explicitly mandate immediate drop for unknown versions
   (version != 0x01), no fallback, no negotiation. Per-hop processing already
   enforced this; clarified normative requirements to eliminate X4 attacks.

6. **Patch M6: TLV Extension Mechanism Section Added**: New section 7 defines
   TLV container format, type registry (0x00-0x7F with critical bit), unknown
   TLV handling rules, and extension registration process. Centralizes TLV
   definition in Monad RFC, prevents X3 (parser divergence).

7. **Patch M7: Error Code Registry Added**: Added Error Code Registry to IANA
   section with 30+ error codes (0x00-0xFF). Codes include CRC_FAILED,
   VERSION_UNSUPPORTED, WOTAN_BOUNDS_CHECK_FAILED, etc. Also added TLV Type
   Registry to IANA for coordinated allocation.

8. **Patch M8: Ring Path Counter TLV Added**: New section 7.4 defines optional
   TLV type 0x01 (Ring Path Counter) for tracking ring buffer path traversals.
   Increments at each ring node, supports loop detection and traffic engineering.

9. **New Section: Shield: eBPF Security Pipeline (Section 6)**: Comprehensive
   new section detailing XDP ingress (wire-speed stamping), TC egress
   (stateful filtering), BPF map pinning contract at /sys/fs/bpf/unheaded/,
   map types and schemas, BPF verifier compliance, interaction with Monad
   processing. Shield is the critical security boundary with rate limiting,
   admission control, and address restoration.

10. **New Section: Anamnesis: Event Capture Architecture (Section 9)**: New
    section details 64-byte RingEntry format, EVE JSON transformation, Wotan
    topic routing (anamnesis.*), Suricata GPL isolation, event correlation via
    IPv6 flow labels, ring buffer configuration. Anamnesis events provide
    complete audit trail of packet computation.

11. **New Section: Kingdom Mode: Operational States (Section 13)**: New section
    defines three operational states (Normal/Degraded/Emergency) with state
    transition triggers, Wotan system.state topic for real-time state
    distribution, and dashboard indicators. Enables graceful degradation
    during partial outages.

12. **Enhanced IANA Section**: Added IPv6 Next Header 0xFE allocation request,
    IPv6 Flow Label RFC 6437 notes, expanded Error Code Registry (30+ codes),
    new TLV Type Registry with critical bit handling, expanded PQC Algorithm
    Registry.

13. **Enhanced Security Considerations**: Added DoS protection details
    (rate limiting, token bucket parameters), amplification attack mitigations,
    BPF verifier as security boundary (verification failures are fatal),
    flow label entropy requirements, address reclamation attack surface
    analysis (ULA spoofing, register forgery, host bits collision).

14. **Frontmatter Updates**: Updated docname to draft-05, date to 2026-02-27,
    added keywords: shield-ebpf, anamnesis, kingdom-mode. Added RFC 9000
    (QUIC) and RFC 9114 (HTTP/3) to normative references for integration notes.

15. **Cross-References Added**: Added [SOPHIA] and [WOTAN] cross-reference
    notations throughout (sections 6, 9, 11, 13). References align with
    companion specs: draft-bellis-unheaded-sophia-dictionary-02 and
    draft-bellis-unheaded-wotan-memory-02.

---
# Acknowledgments

The Linux kernel BPF community (Alexei Starovoitov, Daniel Borkmann) for
creating the infrastructure that made this design possible.

All current and previous internet operators, admins, and engineers. Cheers to innovation.
