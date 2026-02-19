---
title: "Sophia Dictionary Format for the Unheaded Protocol"
abbrev: "Sophia Dictionary Format"
docname: draft-bellis-unheaded-sophia-dictionary-00
category: exp
ipr: trust200902
area: Internet
workgroup: Independent Submission
date: 2026-02-19
stand_alone: yes

author:
  - ins: S. Bellis
    name: Steven Bellis
    org: Unheaded
    email: stevenrbellis@gmail.com
    country: US

normative:
  RFC2119:
  RFC8174:
  RFC8949:
  RFC9669:
  UNHEADED-FOUNDATION:
    title: "The Unheaded Protocol Foundation"
    author:
      - ins: S. Bellis
    date: 2026-02
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-protocol-foundation-03

informative:
  RFC8799:
  RFC9197:

--- abstract

The Sophia Dictionary Format defines the serialization, storage, and distribution
mechanism for semantic metadata that accompanies the Unheaded Protocol.  Sophia
dictionaries are exponent-decoding tables that translate compact byte values
(0x00-0xFF) into meaningful human-readable categories (service identifiers,
QoS classes, flow actions, etc.) and their associated metadata.

This memo specifies the CBOR serialization format for dictionary entries,
the BPF map representation for in-kernel storage, the atomic update protocol
for cluster-wide distribution via the Wotan memory bus, and the minimum
required dictionary entries for any conformant Unheaded deployment.

Sophia dictionaries are hot-swappable: updates propagate to all nodes in
under 10 milliseconds without packet loss or restart.

--- middle

# Introduction

## Problem Statement

The Unheaded Protocol Foundation [UNHEADED-FOUNDATION] defines a 20-byte
register file (the Monad) that travels with every packet.  Each byte in the
Monad is exponent-encoded: the actual value is reconstructed as base^exponent
* multiplier.  But where do the base, multiplier, and the semantic meaning of
each byte position come from?

The answer is Sophia: a distributed, versioned dictionary system that maps
byte values to meanings.  Sophia is the semantic layer.  Without it, the
Monad is just bytes.  With it, a simple 0x03 byte becomes "architect" or
"realtime" or "forward" or "open" depending on context.

This memo specifies:

1. How Sophia dictionaries are represented on the wire (CBOR format per RFC 8949)
2. How they are stored in BPF maps for nanosecond-latency lookups
3. How they are distributed to all nodes atomically via Wotan
4. The minimum dictionary entries that all implementations MUST support
5. Version negotiation and backward-compatibility rules

## Terminology

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this
document are to be interpreted as described in BCP 14 [RFC2119] [RFC8174]
when, and only when, they appear in all capitals, as shown here.

Root Dictionary:
: The top-level BPF map (type BPF_MAP_TYPE_HASH) keyed by root entry ID
  (0x00-0xFF) that points to sub-dictionaries.

Sub-Dictionary:
: A BPF map (type BPF_MAP_TYPE_ARRAY_OF_MAPS) indexed by sub-entry ID
  (0x00-0xFF) that contains semantic metadata.

Sophia Lookup:
: A two-level hash table traversal: root_map[key0] → sub_dict_id, then
  sub_dict[key0][key1] → value.

Dictionary Version:
: An unsigned 8-bit counter (0-255) that increments with each dictionary
  update.  Used for consistency validation across nodes.

Atomic Update:
: The act of replacing an entire BPF map and updating the array-of-maps
  reference in a single atomic kernel operation.

Wotan Topic:
: A publish-subscribe channel through which dictionary updates are
  broadcast.  Format: sophia.dictionary.v{N} where N is the version number.

BPF (Berkeley Packet Filter):
: Per RFC 9669, the in-kernel virtual machine and map storage system.
  This memo uses "BPF" not "eBPF" per RFC 9669 conventions.

# Dictionary Model

## Tree Structure

Sophia dictionaries are trees, not flat tables.  The root level maps
entry categories to sub-dictionaries.  Each sub-dictionary maps specific
values within that category to metadata.

Example:

~~~~~
Root entry 0x01 -> "service_identity" -> sub-dict #1
  Sub-dict #1[0x01] -> {name: "captain", ...}
  Sub-dict #1[0x02] -> {name: "timeguru", ...}
  Sub-dict #1[0x03] -> {name: "architect", ...}

Root entry 0x02 -> "flow_action" -> sub-dict #2
  Sub-dict #2[0x01] -> {name: "forward", ...}
  Sub-dict #2[0x02] -> {name: "trace", ...}
  Sub-dict #2[0x03] -> {name: "sample", ...}

The SAME byte 0x03 means:
  [0x01, 0x03] = service "architect"
  [0x02, 0x03] = action "sample"
  [0x03, 0x03] = qos "realtime"
~~~~~

This compositional structure provides 256^K total expressible meanings
with K key positions, using only 2*K bytes on the wire (K bytes per lookup).

## Root Dictionary

The root dictionary is a single BPF hash map with 256 slots.  Each key
(0x00-0xFF) maps to a root entry structure.

Root entries occupy the following key ranges:

- 0x00: Reserved (MUST NOT be used by any implementation)
- 0x01-0x0F: Standard categories (see Section 8)
- 0x10-0xFE: Available for operator assignment
- 0xFF: Reserved (Yaldabaoth chaos injection)

## Sub-Dictionaries

Each root entry points to a sub-dictionary (a separate BPF map).  Sub-
dictionaries use array-of-maps indirection: the root entry contains a
sub_dict_id (0-255) that is used as a key into the array-of-maps.

Sub-dictionary entries are indexed by a secondary key (0x00-0xFF) and
contain category-specific metadata.

## Lookup Chain

A complete Sophia lookup performs:

1. Look up root_key in sophia_root map
2. Extract sub_dict_id from the result
3. Look up sub_dict_id in sophia_dicts (array of maps)
4. Look up sub_key in the obtained sub-dictionary map
5. Return the final value

Cost: Two BPF hash lookups (~200ns total on modern systems) plus
one array-of-maps indirection (~100ns).  Total: ~300ns per double lookup.

# Serialization Format (CBOR)

Sophia dictionaries are serialized as CBOR per RFC 8949 for distribution
over the Wotan topics.

## Dictionary Entry Structure

Each dictionary entry is a CBOR map with the following fields:

~~~~~
sophia_entry = {
  "type": tstr,                    ; Category name
  "sub_dict_id": uint,             ; Pointer to sub-dictionary
  ? "base": uint,                  ; Exponent base (default: 2)
  ? "multiplier": uint,            ; Scaling factor (default: 1)
  ? "unit": tstr,                  ; Unit string ("ns", "us", "ms", etc.)
  ? "description": tstr,           ; Human-readable description
}
~~~~~

The ? prefix indicates optional fields.  Required fields: type, sub_dict_id.

Type is a text string describing the semantic category.  Examples:
"service_identity", "flow_action", "qos_class", "deployment_ring",
"circuit_state", "mesh_flags".

## Root Entry Schema

Root entries map category names to sub-dictionary indices:

~~~~~
root_entry = {
  "type": tstr,           ; "service_identity", "flow_action", etc.
  "sub_dict_id": uint,    ; Index into sophia_dicts array (0-255)
}
~~~~~

Example encoding (CBOR):

~~~~~
{
  "type": "flow_action",
  "sub_dict_id": 2
}

CBOR hex: A2 64747970652A666C6F775F616374696F6E 0B 6C7375625F646963745F6964 02
         (A2 = map with 2 pairs)
         (647479706520 = "type")
         (6A666C6F775F616374696F6E = "flow_action")
         (0B = integer 11, not used here - replace with actual mapping)
~~~~~

## Sub-Dictionary Entry Schema

Sub-dictionary entries contain category-specific metadata:

~~~~~
sub_dict_entry = {
  "name": tstr,                    ; Human-readable name
  "endpoint": tstr,                ; Service endpoint (optional)
  "pqc_algorithm": uint,           ; PQC algorithm ID (optional)
  "pqc_pubkey": bstr,              ; Public key bytes (optional)
  "pqc_fingerprint": bstr,         ; SHA3-256 truncation (optional)
  "key_epoch": uint,               ; Rotation counter (optional)
  "key_expires": tstr,             ; ISO 8601 timestamp (optional)
  ? "description": tstr,           ; Additional metadata
}
~~~~~

Example: service_identity entry for "captain":

~~~~~
{
  "name": "captain",
  "endpoint": "fd00:3f:75::1007:8080",
  "pqc_algorithm": 1,                    ; ML-KEM-768
  "pqc_pubkey": h'1184bytes...',         ; Base16 or base64url
  "pqc_fingerprint": h'3257A8...',       ; SHA3-256[0:32]
  "key_epoch": 7,
  "key_expires": "2026-03-19T00:00:00Z"
}
~~~~~

## Exponent Rule Entry Schema

Exponent-encoding rules are stored as root dictionary metadata:

~~~~~
exponent_rule = {
  "field": tstr,             ; Field name (e.g., "latency_hint_us")
  "byte_position": uint,     ; Offset in Monad (0-19)
  "base": uint,              ; Exponent base (typically 2 or 10)
  "multiplier": uint,        ; Scaling factor (typically 1)
  "unit": tstr,              ; Unit string ("ns", "us", "ms", "packets")
  "min_value": int,          ; Minimum decoded value (inclusive)
  "max_value": int,          ; Maximum decoded value (inclusive)
}
~~~~~

Example: latency_hint_us field:

~~~~~
{
  "field": "latency_hint_us",
  "byte_position": 12,
  "base": 2,
  "multiplier": 1,
  "unit": "microseconds",
  "min_value": 0,
  "max_value": 2147483647
}
~~~~~

## PQC Key Entry Schema

When PQC identity binding is enabled, key material is stored with metadata:

~~~~~
pqc_key_entry = {
  "service_id": uint,            ; Service identifier
  "pqc_algorithm": uint,         ; Algorithm ID (ML-KEM-768, etc.)
  "pqc_pubkey": bstr,            ; Public key material
  "pqc_fingerprint": bstr,       ; SHA3-256 truncation
  "signature_algorithm": uint,   ; Signature algo ID (ML-DSA-65, etc.)
  "signature_pubkey": bstr,      ; Signature verification key
  "key_epoch": uint,             ; Rotation counter
  "key_issued": tstr,            ; ISO 8601 issuance timestamp
  "key_expires": tstr,           ; ISO 8601 expiration timestamp
  "hybrid_mode": tstr,           ; "CONCATENATE", "PQC_ONLY", or
                                 ; "CLASSICAL_ONLY"
  ? "classical_pubkey": bstr,    ; X25519 key (hybrid mode)
}
~~~~~

# BPF Map Representation

BPF maps are the kernel-space runtime representation of Sophia dictionaries.
They provide O(1) lookup with nanosecond-scale latency.

## Root Map (BPF_MAP_TYPE_HASH)

~~~~~
struct sophia_root_entry {
    u32 sub_dict_id;      // Index into sophia_dicts array
    u8  entry_type;       // 0=identity, 1=action, 2=qos, etc.
    u8  base;             // Exponent base (2, 10, or 256)
    u16 reserved;         // Padding to 8-byte alignment
};

BPF map definition:
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, u8);              // Root key 0x00-0xFF
    __type(value, struct sophia_root_entry);
} sophia_root SEC(".maps");
~~~~~

## Sub-Dictionary Maps (BPF_MAP_TYPE_ARRAY_OF_MAPS)

Each sub-dictionary is itself a BPF hash map.  An array-of-maps indirection
allows atomic replacement of entire sub-dictionaries.

~~~~~
struct sophia_sub_entry {
    u8  name[32];         // Null-terminated name string
    u32 endpoint_ip;      // Service IPv6 last 32 bits (for lookup)
    u16 endpoint_port;    // Service port
    u8  pqc_algo;         // PQC algorithm ID
    u8  key_epoch;        // Key rotation counter
    u8  fingerprint[32];  // SHA3-256 of PQC public key
    u16 reserved;
};  // Total: 80 bytes per entry

BPF map definition:
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY_OF_MAPS);
    __uint(max_entries, 256);        // Up to 256 sub-dictionaries
    __type(key, u32);                // Sub-dictionary index
    __type(value, u32);              // Map file descriptor
} sophia_dicts SEC(".maps");

Each slot in sophia_dicts contains a BPF_MAP_TYPE_HASH:
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);        // 256 entries per sub-dict
    __type(key, u8);                 // Sub-entry key 0x00-0xFF
    __type(value, struct sophia_sub_entry);
} sophia_dict_{N} SEC(".maps");
~~~~~

## Fingerprint Cache Map

For fast PQC fingerprint verification without full key lookup:

~~~~~
struct fingerprint_cache_entry {
    u8  fingerprint[32];  // Truncated SHA3-256
    u8  key_epoch;        // Associated key epoch
    u16 reserved;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u8);                 // Service ID
    __type(value, struct fingerprint_cache_entry);
} sophia_fingerprint_cache SEC(".maps");
~~~~~

## Map Pinning Paths

BPF maps are pinned to the filesystem for persistence across reboots and
for communication between userspace and kernel:

~~~~~
/sys/fs/bpf/unheaded/sophia_root
/sys/fs/bpf/unheaded/sophia_dicts
/sys/fs/bpf/unheaded/sophia_dict_1
/sys/fs/bpf/unheaded/sophia_dict_2
...
/sys/fs/bpf/unheaded/sophia_fingerprint_cache
/sys/fs/bpf/unheaded/sophia_version  (u32 map with single entry)
~~~~~

The sophia_version map contains a single entry (key 0) with the current
dictionary version counter (0-255).

# Dictionary Distribution

Sophia dictionaries are distributed to all nodes via the Wotan publish-
subscribe topics.  The distribution model ensures atomic, cluster-wide
updates with zero packet loss.

## Wotan Distribution Channel

Dictionaries are published on a versioned topic:

~~~~~
Topic: sophia.dictionary.v{N}

Where N = version number (0-255).

Each topic publication contains:
  1. Complete serialized dictionary (CBOR)
  2. Version number (repeated for idempotence)
  3. Timestamp (ISO 8601)
  4. Signature (ML-DSA-65, optional for integrity)
~~~~~

Publishers:
- A designated provisioning node updates Pleroma
- Wotan control plane publishes the new dictionary to sophia.dictionary.v{N}
- All Shim/Shield nodes subscribe to sophia.dictionary.v*

## Version Negotiation

Implementations MUST support at least 2 concurrent dictionary versions.
When a new version is published:

1. Subscriber receives CBOR dictionary on sophia.dictionary.v{N+1}
2. New maps are created (sophia_dict_1_v{N+1}, etc.)
3. Old version maps remain active
4. Array-of-maps references are atomically updated
5. Old version maps are retained for grace_period (default: 60 seconds)
6. After grace_period, old maps are deleted

This ensures that packets in flight using the old dictionary version do
not cause errors or inconsistencies.

## Atomic Update Protocol

The update sequence is:

~~~~~
1. [Provisioning] Publish new dictionary to sophia.dictionary.v{N+1}
2. [Wotan] Receive on subscriber
3. [Wotan] Deserialize CBOR → in-memory dictionary
4. [Wotan] Create new BPF maps with suffix _v{N+1}
5. [Wotan] Load all entries into new maps
6. [Wotan] Update sophia_dicts[0..255] pointers atomically
   (single atomic map write per sub-dict)
7. [Wotan] Update sophia_version map (key 0, value = N+1)
8. [Wotan] Retain old maps for grace_period
9. [Wotan] After grace_period, delete old maps
~~~~~

All Shim/Shield nodes see the update within one polling cycle
(typically <10ms) of the Wotan write.

## Consistency Model

The consistency model is eventual with bounded staleness:

- Write-then-read consistency: A node that writes its local dictionary
  update MAY not immediately see it in other nodes' views.
- Staleness bound: All nodes see the same version within grace_period.
- Packet coherence: All packets originating in a single epoch use the
  same dictionary version (set by Shield at birth).

During version transitions, packets carrying old version numbers in the
Monad (Section 5 of [UNHEADED-FOUNDATION]) are processed using the
retained old dictionary map.  Packets carrying new version numbers use
the new map.

# Minimum Required Dictionary

A conformant Unheaded implementation MUST support at least the following
dictionary entries.

## Reserved Root Keys (0x00-0x0F)

~~~~~
0x00  RESERVED (MUST NOT be used)
0x01  service_identity (REQUIRED)
0x02  flow_action (REQUIRED)
0x03  qos_class (REQUIRED)
0x04  deployment_ring (RECOMMENDED)
0x05  circuit_state (RECOMMENDED)
0x06  mesh_flags (OPTIONAL)
0x07-0x0E  RESERVED for future standardization
0x0F  RESERVED for future standardization
~~~~~

## Standard Service Identity Entries

Sub-dictionary #1 (service_identity) MUST define at least:

~~~~~
0x00  RESERVED (catch-all for unknown service)
0x01  captain (primary ingress/egress)
0x02  timeguru (time synchronization)
0x03  architect (policy engine)
0x04  micromanager (QoS enforcement)
0x05  wotan (memory/event broker)
0x06  dashboard (observability UI)
0x07  kanban (orchestration)
0x08-0xFF  Available for operator assignment
~~~~~

Each entry MUST include at minimum: name (string), endpoint (string or
IPv6 address).  MAY include PQC key material if PQC identity binding
is enabled.

## Standard Flow Action Entries

Sub-dictionary #2 (flow_action) MUST define:

~~~~~
0x00  forward (REQUIRED; default action)
0x01  trace (REQUIRED; full packet trace to Anamnesis)
0x02  sample (REQUIRED; probabilistic trace)
0x03  mirror (REQUIRED; packet clone to mirror port)
0x04  rate_limit (OPTIONAL; token bucket enforcement)
0x05  drop (RECOMMENDED; explicit packet drop)
0x06-0x0F  RESERVED for future standardization

0x10  key_announce (PQC key lifecycle)
0x11  key_rotate (epoch increment)
0x12  key_revoke (emergency key revocation)
0x13  kem_encaps (ML-KEM encapsulation request)
0x14  kem_decaps (ML-KEM decapsulation request)
0x15-0xFF  Available for operator assignment
~~~~~

## Standard QoS Class Entries

Sub-dictionary #3 (qos_class) MUST define:

~~~~~
0x00  default (REQUIRED)
0x01  realtime (REQUIRED; lowest latency, no sampling)
0x02  interactive (REQUIRED; medium priority)
0x03  bulk (REQUIRED; background traffic)
0x04-0xFF  Available for operator assignment
~~~~~

# Dictionary Versioning

Sophia dictionaries have an explicit version number that increments with
each update.  Version numbers are 8-bit unsigned integers (0-255) that wrap.

## Version Counter

- Maintained in sophia_version BPF map (key 0, value = current version)
- Incremented monotonically (mod 256) on each update
- Stamped into Extended Register Space by Shield (if Kingdom Mode is active)
- Allows per-hop verification that the packet was stamped with the current
  dictionary version

## Backward Compatibility Rules

When a new version is published:

1. All new dictionary entries in version N+1 MUST be defined in version N
   (no gaps).  That is, if version N+1 defines flow_action 0x03, version N
   MUST also define it with the same meaning.

2. Existing entries (0x01-0x0F) MAY have extended metadata in version N+1,
   but their core meaning MUST NOT change.

3. New root keys (0x10-0xFE) MAY be added without affecting existing keys.

4. Deprecated entries (those with changed semantics) MUST be handled in a
   new root key, not by redefinition.

## Grace Period for Version Transitions

When a new version N+1 is published, the old version N dictionary maps are
retained for a configurable grace_period (default: 60 seconds).

- Packets arriving with version N in their headers are processed using version N maps
- Packets arriving with version N+1 use version N+1 maps
- Shield SHOULD stamp new packets with version N+1 immediately
- After grace_period, version N maps are deleted

Grace period is configurable per deployment via Pleroma:

~~~~~
versioning:
  grace_period_seconds: 60    # Default
  max_concurrent_versions: 2  # Always support current + previous
~~~~~

# Security Considerations

## Dictionary Integrity

Sophia dictionaries MUST be protected against tampering.  Recommended
mechanisms:

1. Access control: Dictionary maps reside in kernel BPF space and are not
   directly accessible from userspace.  Updates require CAP_BPF and
   CAP_NET_ADMIN.

2. Signature verification: Dictionary publications on Wotan topics SHOULD
   be signed with ML-DSA-65 [FIPS204] by the provisioning node.

3. Version monotonicity: Implementations MUST reject dictionary updates where
   the version number decreases (version wrap at 255->0 is allowed).

4. Source authentication: Only authenticated provisioning nodes SHOULD be
   able to publish dictionary updates.

## Side-Channel Mitigation

BPF map lookups for fingerprint verification MUST use constant-time
comparison functions:

~~~~~c
// Constant-time fingerprint comparison
static int constant_memcmp(const u8 *a, const u8 *b, size_t len) {
    u8 cmp = 0;
    #pragma unroll
    for (int i = 0; i < len; i++) {
        cmp |= a[i] ^ b[i];
    }
    return cmp;
}
~~~~~

Implementations MUST NOT optimize fingerprint lookups in ways that leak
timing information to an observer.

## Access Control on BPF Maps

Sophia maps MUST be pinned with restrictive file permissions:

~~~~~
/sys/fs/bpf/unheaded/*  mode 0600  (user read/write only)
~~~~~

CAP_BPF and CAP_NET_ADMIN are required to modify dictionary maps.  Standard
users (including observability agents) SHOULD NOT have write access.

# IANA Considerations

## Sophia Root Key Registry

IANA should establish a new registry:

~~~~~
Registry Name:  Unheaded Sophia Root Dictionary Keys
Template:       Root Key (0x00-0xFF), Category Name, Type,
                Specification Reference
Policy:         0x00-0x0F: Specification Required
                0x10-0xFE: First Come First Served
                0xFF: Specification Required (reserved)

Initial entries:
  0x00: RESERVED
  0x01: service_identity
  0x02: flow_action
  0x03: qos_class
  0x04: deployment_ring
  0x05: circuit_state
  0x06: mesh_flags
  0x07-0x0F: RESERVED
  0xFF: RESERVED (Yaldabaoth)
~~~~~

## Standard Sub-Dictionary Registry

~~~~~
Registry Name:  Unheaded Sophia Sub-Dictionary Entry Values
Template:       Root Key, Sub-Entry Key (0x00-0xFF), Entry Name,
                Description, Specification Reference
Policy:         0x00-0x0F: Specification Required
                0x10-0xFF: First Come First Served

Initial entries for service_identity (root 0x01):
  0x00: unknown
  0x01: captain
  0x02: timeguru
  0x03: architect
  0x04: micromanager
  0x05: wotan
  0x06: dashboard
  0x07: kanban

Initial entries for flow_action (root 0x02):
  0x00: forward
  0x01: trace
  0x02: sample
  0x03: mirror
  0x04: rate_limit
  0x05: drop
  0x10: key_announce
  0x11: key_rotate
  0x12: key_revoke
  0x13: kem_encaps
  0x14: kem_decaps

Initial entries for qos_class (root 0x03):
  0x00: default
  0x01: realtime
  0x02: interactive
  0x03: bulk
~~~~~

--- back

# Appendix: CBOR Schema Definitions (Informative)

This appendix provides CDDL [RFC8610] schema definitions for Sophia
dictionaries.  These are informative only; the normative specification
is in Sections 4 and 5.

## Root Dictionary CDDL Schema

~~~~~cddl
sophia_dictionary = {
  "version": uint,
  "entries": [+ root_entry],
}

root_entry = {
  "key": uint,            ; 0x00-0xFF
  "type": tstr,
  "sub_dict_id": uint,
  ? "base": uint,         ; default: 2
  ? "multiplier": uint,   ; default: 1
}
~~~~~

## Sub-Dictionary CDDL Schema

~~~~~cddl
sub_dictionary = {
  "version": uint,
  "entries": [+ sub_entry],
}

sub_entry = {
  "key": uint,                     ; 0x00-0xFF
  "name": tstr,
  ? "endpoint": tstr,
  ? "pqc_algorithm": uint,
  ? "pqc_pubkey": bstr,
  ? "pqc_fingerprint": bstr,
  ? "key_epoch": uint,
  ? "key_expires": tstr,
  ? "description": tstr,
}
~~~~~

# Appendix: Example Dictionary Update Workflow

This section illustrates a complete dictionary update cycle from
provisioning to deployment.

~~~~~
[Operator] Edits Pleroma YAML:
  services:
    captain: id 0x01 -> add pqc_algorithm: ML-KEM-768

[Wotan control] Detects drift:
  Kenoma (observed) != Pleroma (desired)
  Triggers dictionary update

[Wotan] Publishes to sophia.dictionary.v48:
  CBOR:
  {
    "version": 48,
    "timestamp": "2026-02-19T15:30:00Z",
    "entries": [
      {
        "key": 0x01,
        "type": "service_identity",
        "sub_dict_id": 1
      },
      ...
    ]
  }

[All nodes] Subscribe to sophia.dictionary.v48:
  1. Receive CBOR
  2. Create new maps: sophia_dict_1_v48
  3. Load all entries
  4. Atomically update sophia_dicts[1] -> new map
  5. Update sophia_version = 48

[Shield] On next packet:
  Stamps new packets with version 48 in Extended Registers

[60 seconds later] Grace period expires:
  Old version 47 maps are deleted
  All nodes now on version 48
~~~~~

---

*This memo specifies the Sophia Dictionary Format as a companion to
draft-bellis-unheaded-protocol-foundation-03.  Sophia is the semantic
layer that transforms raw exponent bytes into meaningful metadata.*
