---
title: "Wotan Memory Protocol for the Unheaded Protocol"
abbrev: "Wotan Memory Protocol"
docname: draft-bellis-unheaded-wotan-memory-03
category: exp
ipr: trust200902
area: Internet
workgroup: Independent Submission
date: 2026-03-05
stand_alone: yes

author:
  - ins: S. Bellis
    name: Stevie Bellis
    org: Unheaded
    email: stevie@bellis.tech
    country: US

normative:
  RFC2119:
  RFC8174:
  RFC9669:
  UNHEADED-FOUNDATION:
    title: "Unheaded: Protocol Foundation"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-protocol-foundation-06
  UNHEADED-SOPHIA:
    title: "Sophia Dictionary Format for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-sophia-dictionary-03

informative:
  RFC0768:
  RFC0791:
  RFC0792:
  RFC0793:
  RFC8799:
  RFC9180:
  FIPS203:
    title: "Module-Lattice-Based Key-Encapsulation Mechanism Standard"
    author:
      org: National Institute of Standards and Technology
    date: 2024-08
    target: https://csrc.nist.gov/pubs/fips/203/final

--- abstract

Wotan is the memory and I/O bus for the Unheaded Protocol, providing
addressable per-flow storage for BPF programs executing within the
Limited Domain [RFC8799].

The Wotan protocol specifies the BPF helper interface for memory access,
the address space layout for per-flow data structures, a three-level
cache hierarchy (L1 BPF hash maps, L2 per-flow ring buffers, L3
persistent Write-Ahead Log), and the topic-based I/O model for
interaction with userspace services.

This memo defines the memory model, helper functions, address space,
cache miss protocol, gRPC streaming contracts, triple-role architecture,
reliability guarantees, and I/O topic naming conventions for systems
implementing the Unheaded Protocol's computational layer.

Draft-03 introduces a structured error code taxonomy with severity
levels, helper return codes for common operations, and error recovery
procedures. Draft-02 security patches W1-W8 are retained.

--- middle

# Introduction

The Unheaded Protocol [UNHEADED-FOUNDATION] specifies a 20-byte register
file (the Monad) that travels with every packet through a Limited Domain.
BPF programs at each hop read and write the Monad, performing stateless
per-packet computation.

Many use cases require state beyond the 20-byte Monad: buffering input,
accumulating results, maintaining per-flow state machines, or storing
scratch memory for complex algorithms.

Wotan provides this state via a hierarchical memory model:

- L0: Monad (20 bytes, in packet, per-hop latency ~320 ns)
- L1: Per-hop BPF map cache (64-byte cache lines, ~100-200 ns latency)
- L2: Per-flow ring buffer RAM (configurable size, ~1-10 us latency)
- L3: Write-Ahead Log (persistent storage, ~100 us-1 ms latency)
- L4: Sophia dictionaries [UNHEADED-SOPHIA] (instruction decode,
  ~100-200 ns latency)

This memo defines the Wotan memory protocol: the BPF helper interface,
address space layout, cache coherency model, and userspace I/O
interaction.

## Cross-References

This document is part of the Unheaded Protocol specification family:

-  **Protocol Foundation** [UNHEADED-FOUNDATION]: Defines the Monad wire
   format (20 bytes, FROZEN at v0x01), per-hop processing, IANA
   registries, IANA registration procedures, and the wire format
   immutability threat model.

-  **Sophia Dictionary Format** [UNHEADED-SOPHIA]: Defines the semantic
   layer including sub-dictionary type systems for hierarchical knowledge
   and QPACK compression headers for dictionary entries.

# Terminology and Language

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this
document are to be interpreted as described in BCP 14 [RFC2119] [RFC8174]
when, and only when, they appear in all capitals, as shown here.

The following terms are used:

Flow Label:
: The IPv6 Flow Label field (20 bits) or derived hash used to key
  per-flow state in Wotan. Maps packets to unique per-flow ring buffers.

Ring Buffer:
: A BPF ring buffer (BPF_MAP_TYPE_RINGBUF) allocated per flow, used as
  L2 memory (general-purpose RAM) with configurable size via --ring-size.

Cache Line:
: A 64-byte unit of L1 cache (per-hop BPF map), with tag, valid, dirty,
  and LRU tracking.

Write-Back:
: Transfer of dirty cache lines from L1 (per-hop map) to L2 (ring buffer)
  for persistence or hand-off.

Memory-Mapped I/O:
: Designated address ranges that publish to or read from Wotan topics
  (e.g., write to address 0x0000C000 publishes to compute.screen).

# Error Code Taxonomy (NEW in draft-03)

## Overview

Draft-02 defined error handling for BPF helpers using standard errno
codes (-ENOENT, -EFAULT, -ENOMEM, -EACCES, -EINVAL, -EAGAIN). Draft-03
introduces a structured error code taxonomy that classifies errors by
severity, origin, and recommended recovery action.

This taxonomy applies to all Wotan operations: BPF helper functions,
gRPC streaming, WAL operations, and control frame exchanges.

## Error Severity Levels

Each error code is assigned a severity level:

~~~~~
Severity    Code    Description                 Action
--------    ----    --------------------------  ---------------------------
INFO        0       Informational event          Log, continue processing
WARNING     1       Degraded but functional      Log, emit metric, continue
ERROR       2       Operation failed             Log, retry or degrade
CRITICAL    3       Subsystem failure            Log, alert, isolate
FATAL       4       Unrecoverable failure        Log, halt, require restart
~~~~~

## Structured Error Code Format

Error codes in draft-03 use a 32-bit structured format:

~~~~~
 31       24 23       16 15        8 7         0
+-----------+-----------+-----------+-----------+
| Severity  |  Origin   | Category  |  Detail   |
| (3 bits)  |  (5 bits) | (8 bits)  |  (8 bits) |
+-----------+-----------+-----------+-----------+
~~~~~

Severity:
: 3-bit severity level (0-4, see above). Bits 31-29.

Origin:
: 5-bit origin identifier. Bits 28-24.

~~~~~
Origin Code   Name              Description
-----------   ----------------  ----------------------------
0x00          WOTAN_CORE        Core Wotan daemon
0x01          WOTAN_L1          L1 cache subsystem
0x02          WOTAN_L2          L2 ring buffer subsystem
0x03          WOTAN_L3          L3 WAL subsystem
0x04          WOTAN_GRPC        gRPC streaming subsystem
0x05          WOTAN_TOPIC       Topic routing subsystem
0x06          WOTAN_SETTINGS    SETTINGS exchange
0x07          WOTAN_GOAWAY      GOAWAY frame processing
0x08          SOPHIA_LOOKUP     Sophia dictionary lookup
0x09          SOPHIA_UPDATE     Sophia dictionary update
0x0A          SHIELD_INGRESS    Shield ingress processing
0x0B          SHIELD_EGRESS     Shield egress processing
0x0C          SHIM_EXEC         Shim program execution
0x0D-0x1E    Reserved
0x1F          VENDOR_SPECIFIC   Vendor-specific origin
~~~~~

Category:
: 8-bit error category. Bits 23-16.

~~~~~
Category    Name                Description
--------    -----------------   ---------------------------------
0x00        NONE                No error
0x01        ACCESS_CONTROL      Authorization / permission error
0x02        BOUNDS_CHECK        Address / offset out of range
0x03        RESOURCE            Memory / buffer / disk exhausted
0x04        INTEGRITY           Checksum / HMAC / CRC failure
0x05        PROTOCOL            Wire format / version mismatch
0x06        TIMEOUT             Operation timed out
0x07        CONCURRENCY         Lock contention / CAS failure
0x08        CONFIGURATION       Settings mismatch / invalid param
0x09        DEPENDENCY          External dependency unavailable
0x0A        DATA_CORRUPTION     Data integrity violation
0x0B        RATE_LIMIT          Rate limit exceeded
0x0C-0xFE   Reserved
0xFF        VENDOR_SPECIFIC     Vendor-specific category
~~~~~

Detail:
: 8-bit error detail code. Bits 7-0. Interpretation depends on the
  category.

## Helper Return Code Mapping

BPF helper functions continue to return standard errno codes for
backward compatibility. The structured error code is available via
an auxiliary error detail mechanism.

### BPF Helper Return Codes

~~~~~
errno           Structured Code              Severity
---------       ---------------------------  --------
-ENOENT (-2)    [ERROR, L2, RESOURCE, 0x01]  ERROR
-EFAULT (-14)   [ERROR, L1, BOUNDS, 0x01]    ERROR
-ENOMEM (-12)   [WARN, L1, RESOURCE, 0x01]   WARNING
-EACCES (-13)   [ERROR, CORE, ACCESS, 0x01]  ERROR
-EINVAL (-22)   [ERROR, CORE, PROTOCOL, 0x01] ERROR
-EAGAIN (-11)   [INFO, L1, CONCURRENCY, 0x01] INFO
-EBUSY (-16)    [WARN, L3, CONCURRENCY, 0x01] WARNING
~~~~~

### Auxiliary Error Detail

When a BPF helper returns a negative errno, implementations SHOULD
write the full 32-bit structured error code to a per-CPU BPF array
map:

~~~~~
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, u32);  // 32-bit structured error code
} wotan_last_error SEC(".maps");
~~~~~

Shim programs MAY read this map after a helper returns an error to
obtain detailed error information:

~~~~~
ret = bpf_wotan_read(flow_label, addr, buf, len);
if (ret < 0) {
    u32 key = 0;
    u32 *err = bpf_map_lookup_elem(&wotan_last_error, &key);
    if (err) {
        u8 severity = (*err >> 29) & 0x7;
        u8 origin = (*err >> 24) & 0x1F;
        u8 category = (*err >> 16) & 0xFF;
        u8 detail = *err & 0xFF;
        // Handle based on severity/category
    }
}
~~~~~

## Common Error Codes

### L1 Cache Errors

~~~~~
Code                                Errno     Description
----------------------------------  --------  -------------------------
[WARN, L1, RESOURCE, 0x01]         -ENOMEM   Cache miss (line not in L1)
[ERROR, L1, BOUNDS, 0x01]          -EFAULT   Offset > 63 (read)
[ERROR, L1, BOUNDS, 0x02]          -EFAULT   Offset > 56 (write, 8-byte)
[INFO, L1, CONCURRENCY, 0x01]      -EAGAIN   CAS comparison failed
[ERROR, L1, BOUNDS, 0x03]          -EFAULT   CAS alignment error (not 8-byte)
[WARN, L1, RATE_LIMIT, 0x01]       -EBUSY    Cache-miss rate exceeded (W6)
~~~~~

### L2 Ring Buffer Errors

~~~~~
Code                                Errno     Description
----------------------------------  --------  -------------------------
[ERROR, L2, RESOURCE, 0x01]         -ENOENT   Flow not found (no ring buf)
[ERROR, L2, RESOURCE, 0x02]         -ENOMEM   Ring buffer full (overflow)
[ERROR, L2, INTEGRITY, 0x01]        N/A       CRC-32 mismatch on entry
[ERROR, L2, INTEGRITY, 0x02]        N/A       Seqno discontinuity (W1)
[ERROR, L2, DATA_CORRUPTION, 0x01]  N/A       Valid flag inconsistency
~~~~~

### L3 WAL Errors

~~~~~
Code                                Errno     Description
----------------------------------  --------  -------------------------
[ERROR, L3, RESOURCE, 0x01]         -ENOSPC   WAL disk full
[ERROR, L3, INTEGRITY, 0x01]        N/A       HMAC-SHA256 mismatch (W4)
[ERROR, L3, INTEGRITY, 0x02]        N/A       CRC-32 mismatch on WAL entry
[ERROR, L3, INTEGRITY, 0x03]        N/A       Seqno gap in WAL (W1)
[WARN, L3, CONCURRENCY, 0x01]       N/A       Compaction lock contention (W5)
[CRITICAL, L3, DATA_CORRUPTION, 0x01] N/A     WAL file corrupted (unrecoverable)
~~~~~

### gRPC Streaming Errors

~~~~~
Code                                Errno     Description
----------------------------------  --------  -------------------------
[ERROR, GRPC, ACCESS, 0x01]         N/A       Unauthorized topic publish
[WARN, GRPC, RESOURCE, 0x01]        N/A       Subscriber buffer overflow
[ERROR, GRPC, TIMEOUT, 0x01]        N/A       Subscription idle timeout (5m)
[ERROR, GRPC, PROTOCOL, 0x01]       N/A       Invalid SETTINGS frame (W7)
[ERROR, GRPC, PROTOCOL, 0x02]       N/A       Invalid GOAWAY frame (W8)
[WARN, GRPC, RATE_LIMIT, 0x01]      N/A       GOAWAY rate limit exceeded
~~~~~

### Sophia Lookup Errors

~~~~~
Code                                Errno     Description
----------------------------------  --------  -------------------------
[ERROR, SOPHIA, RESOURCE, 0x01]     N/A       Dictionary not initialized
[ERROR, SOPHIA, BOUNDS, 0x01]       N/A       Nesting depth exceeded (8 levels)
[ERROR, SOPHIA, DATA_CORRUPTION, 0x01] N/A    Circular reference detected
[ERROR, SOPHIA, INTEGRITY, 0x01]    N/A       ML-DSA-65 signature invalid
[ERROR, SOPHIA, RESOURCE, 0x02]     N/A       Dictionary full (128 entries)
[WARN, SOPHIA, INTEGRITY, 0x02]     N/A       QPACK decompression failure
~~~~~

## Error Recovery Procedures (NEW in draft-03)

### Recovery by Severity Level

~~~~~
Severity    Recovery Procedure
--------    ---------------------------------------------------
INFO        Log event. No recovery action needed. Continue
            processing immediately.

WARNING     Log event. Emit metric (increment warning counter).
            Continue processing with degraded behavior:
            - Cache miss: retry via BPF_TAIL_CALL (3 attempts max)
            - Rate limit: back off (100ms delay)
            - Buffer overflow: drain to L3 before retry

ERROR       Log event. Emit metric. Skip current operation.
            Apply fallback:
            - Access denied: drop packet or use default value
            - Bounds check: skip memory access, use zero-fill
            - Resource exhaustion: degrade to stateless mode
            - Integrity failure: reject data, emit EVENT_ANOMALY

CRITICAL    Log event. Emit alert (Wotan alerts.* topic). Isolate
            affected subsystem:
            - L3 corruption: disable WAL writes, switch to L2-only
            - gRPC failure: disconnect and reconnect (exponential backoff)
            - Sophia failure: use cached dictionary version

FATAL       Log event. Emit emergency alert. Halt affected flow:
            - WAL unrecoverable: drop flow state, reallocate
            - Total memory exhaustion: enter Emergency Mode
              (per [UNHEADED-FOUNDATION] Section 10.3)
            - Require operator intervention for restart
~~~~~

### Automatic Recovery State Machine

Each Wotan subsystem (L1, L2, L3, gRPC, Sophia) maintains an
independent recovery state machine:

~~~~~
States:
  HEALTHY     -> All operations succeeding
  DEGRADED    -> Some operations failing, recovery in progress
  RECOVERING  -> Active recovery procedure executing
  FAILED      -> Recovery exhausted, subsystem disabled

Transitions:
  HEALTHY -> DEGRADED:
    Trigger: error_count > threshold (configurable, default 10)
    Action: Enable recovery procedures

  DEGRADED -> RECOVERING:
    Trigger: Recovery procedure initiated
    Action: Execute recovery steps per severity level

  RECOVERING -> HEALTHY:
    Trigger: 3 consecutive successful operations
    Action: Reset error counters, resume normal operation

  RECOVERING -> FAILED:
    Trigger: Recovery attempts exhausted (max 5)
    Action: Disable subsystem, alert operator

  FAILED -> RECOVERING:
    Trigger: Operator intervention (manual restart command)
    Action: Re-execute recovery from clean state

  DEGRADED -> HEALTHY:
    Trigger: error_count drops below threshold / 2
    Action: Resume normal operation
~~~~~

### Recovery Metrics

Implementations MUST export the following recovery metrics:

~~~~~
wotan_error_total{severity, origin, category}     (counter)
wotan_recovery_attempts_total{subsystem}           (counter)
wotan_recovery_success_total{subsystem}            (counter)
wotan_recovery_failure_total{subsystem}            (counter)
wotan_subsystem_state{subsystem}                   (gauge: 0-3)
wotan_time_in_degraded_seconds{subsystem}          (histogram)
~~~~~

### Cross-Subsystem Recovery

When one subsystem enters FAILED state, dependent subsystems MUST
be notified:

~~~~~
Dependency Graph:
  L1 cache -> L2 ring buffer -> L3 WAL
  gRPC streaming -> L2 ring buffer
  Sophia lookup -> L1 cache

Example: L3 WAL enters FAILED state:
  1. L2 ring buffer: Switch to overflow-drop mode (no L3 drain)
  2. L1 cache: Continue operating (L1 hits unaffected)
  3. gRPC: Continue streaming (events may lack persistence guarantee)
  4. Alert: Emit CRITICAL alert on Wotan alerts.* topic
  5. Dashboard: Display L3 subsystem as red (FAILED)
~~~~~

# Architecture Overview

## Role in the Unheaded Protocol

Wotan bridges Monad computation [UNHEADED-FOUNDATION] to memory and
I/O:

- Shim programs (BPF) running at each hop read/write Wotan memory via
  BPF helpers (bpf_wotan_read, bpf_wotan_write, bpf_wotan_cas).
- Wotan maintains per-flow state keyed by IPv6 Flow Label.
- Wotan interfaces with userspace via ring buffer events and pub/sub
  topics.
- Wotan implements cache miss handling, prefetching, and Write-Ahead
  Log management.

## Memory Hierarchy

~~~~~
Level   Name                Size           Latency      Backing
------  --------------------  ---------      ----------   ----------
L0      Monad (packet)      20 bytes       ~ns          wire
L1      Cache (BPF map)     variable       ~100-200ns   per-hop
L2      Ring Buffer (RAM)   configurable   ~1-10us      per-flow
L3      Write-Ahead Log     disk           ~100us-1ms   persistent
L4      Sophia dictionaries BPF maps       ~100-200ns   instruction decode
~~~~~

Wotan implements transparent L1->L2 promotion on cache miss, L2->L3
flush on overflow, and L3->L2 recovery on process restart.

## Separation of Compute and Memory

The Monad is transient compute state (stateless by design). Wotan is
persistent state machine storage. This separation allows:

- Shim programs to remain stateless with respect to the packet format.
- External state to be accessed in a controlled, measurable manner.
- Cache miss latency to be handled without blocking per-hop logic.
- Memory updates to be tracked in Anamnesis for observability.

# BPF Helper Interface

BPF Shim programs access Wotan memory via three helper functions. All
helpers operate on a 32-bit address space keyed by IPv6 Flow Label.
Error codes follow the taxonomy defined in Section 3.

## bpf_wotan_read

Read from Wotan memory.

~~~~~
long bpf_wotan_read(u32 flow_label, u32 addr, void *buf, u32 len);
~~~~~

**Arguments:**
- flow_label: 20-bit IPv6 Flow Label (zero-extended to u32)
- addr: 32-bit address within the flow's address space
- buf: pointer to destination buffer
- len: number of bytes to read (MUST be 1, 2, or 4)

**Returns:**
- On success: number of bytes read (len)
- -ENOENT (-2): flow_label not found [ERROR, L2, RESOURCE, 0x01]
- -EFAULT (-14): addr out of bounds [ERROR, L1, BOUNDS, 0x01]
- -ENOMEM (-12): L1 cache miss [WARN, L1, RESOURCE, 0x01]
- -EACCES (-13): not authorized [ERROR, CORE, ACCESS, 0x01]
- -EINVAL (-22): len not in {1, 2, 4} [ERROR, CORE, PROTOCOL, 0x01]

## bpf_wotan_write

Write to Wotan memory.

~~~~~
long bpf_wotan_write(u32 flow_label, u32 addr, const void *buf, u32 len);
~~~~~

**Arguments:**
- flow_label: 20-bit IPv6 Flow Label
- addr: 32-bit address within the flow's address space
- buf: pointer to source buffer
- len: number of bytes to write (MUST be 1, 2, or 4)

**Returns:**
- On success: number of bytes written (len)
- -ENOENT (-2): flow_label not found [ERROR, L2, RESOURCE, 0x01]
- -EFAULT (-14): addr out of bounds [ERROR, L1, BOUNDS, 0x02]
- -ENOMEM (-12): L1 cache miss [WARN, L1, RESOURCE, 0x01]
- -EACCES (-13): not authorized [ERROR, CORE, ACCESS, 0x01]
- -EINVAL (-22): len not in {1, 2, 4} [ERROR, CORE, PROTOCOL, 0x01]

## bpf_wotan_cas

Atomic compare-and-swap on Wotan memory.

~~~~~
long bpf_wotan_cas(u32 flow_label, u32 addr, u32 expected, u32 desired);
~~~~~

**Arguments:**
- flow_label: 20-bit IPv6 Flow Label
- addr: 32-bit address (MUST be 8-byte aligned per PATCH W3)
- expected: expected current value (u32)
- desired: value to write if current == expected

**Returns:**
- 0: swap successful [INFO, L1, NONE, 0x00]
- -EAGAIN (-11): current != expected [INFO, L1, CONCURRENCY, 0x01]
- -ENOENT (-2): flow_label not found [ERROR, L2, RESOURCE, 0x01]
- -EFAULT (-14): addr out of bounds or not 8-byte aligned
  [ERROR, L1, BOUNDS, 0x03]
- -EACCES (-13): not authorized [ERROR, CORE, ACCESS, 0x01]

## Error Handling (Enhanced in draft-03)

Implementations MUST handle all specified error codes. The structured
error taxonomy (Section 3) provides additional detail beyond errno:

~~~~~
For each helper return:
  1. Check errno (standard error handling, backward compatible)
  2. Optionally read wotan_last_error map for structured code
  3. Apply recovery procedure per severity level (Section 3.5.1)
  4. Emit metric: wotan_helper_error_total{errno, helper_name}
~~~~~

RECOMMENDED error handling by severity:

- INFO (-EAGAIN): Retry immediately (CAS retry loop, max 10 iterations)
- WARNING (-ENOMEM): Retry via BPF_TAIL_CALL (max 3 attempts)
- ERROR (-ENOENT, -EFAULT, -EACCES, -EINVAL): Skip operation, use
  default value, emit EVENT_ANOMALY

Programs MUST NOT crash on negative returns; they MUST check return
values and branch accordingly.

# Security Considerations

## Topic Injection Attacks

A malicious application could publish events to unauthorized topics.

**Mitigation (MANDATORY)**:
- Topic access control via Sophia [UNHEADED-SOPHIA] (per-program
  topic whitelist)
- Verify publisher identity (program ID from BPF context)
- Enforce least-privilege (default: deny all topics)
- Log unauthorized publish attempts
- Emit ANOMALY event to Anamnesis

## Ring Buffer Memory Exhaustion (PATCH W6)

Per-program cache-miss rate limiting (10K misses/sec budget).
See draft-02 Section 8.2 for complete specification.

## Cross-Flow Memory Access (PATCH W2)

64-bit composite L1 cache keys prevent birthday attack collisions.
See draft-02 Section 4.1 for complete specification.

## CAS Alignment Violations (PATCH W3)

8-byte alignment enforced by BPF verifier at load time.
See draft-02 Section 3.2.1 for complete specification.

## WAL Tampering Detection (PATCH W4)

HMAC-SHA256 authentication on WAL entries.
See draft-02 Section 3.1 for specification.

## WAL Compaction Race Conditions (PATCH W5)

Exclusive mutex during compaction. See draft-02 Section 3.3.

## GOAWAY Frame DoS (PATCH W8)

Frame validation and rate limiting. See draft-02 Section 10.2.

## Error Code Information Leakage

Structured error codes (Section 3) may reveal internal architecture
details to an attacker observing BPF program behavior or Wotan metrics.

**Mitigation**:
- Error detail codes MUST NOT contain sensitive data (keys, addresses)
- Metrics SHOULD aggregate error counts, not individual error details
- The wotan_last_error map is per-CPU and ephemeral (not persisted)
- External-facing error messages SHOULD use generic descriptions

## Cross-Reference with Foundation and Sophia

Security considerations in this memo are aligned with:

1. **[UNHEADED-FOUNDATION] Section 10 - Security Considerations**:
   Wire format immutability threat model, parser divergence attacks,
   BPF containment, and integrity protection mechanisms.

2. **[UNHEADED-SOPHIA] Section 9 - Security Considerations**:
   Dictionary poisoning, nested dictionary security, QPACK
   decompression security, and BPF map access control.

# IANA Considerations

This memo does not request IANA registration of option types or protocol
numbers; those are handled by [UNHEADED-FOUNDATION].

Wotan topic naming uses informal convention (compute.*, sophia.*,
anamnesis.*). If standardization is needed, IANA may create a registry:

~~~~~
Registry Name: Wotan Topic Namespace
Policy: Expert Review
Template: Topic Name, Component, Description, Reference
~~~~~

## Wotan Error Origin Registry (NEW in draft-03)

~~~~~
Registry Name:  Unheaded Wotan Error Origin Codes
Template:       Origin Code (0x00-0x1F), Origin Name, Description,
                Specification Reference
Policy:         Specification Required

Initial entries: See Section 3.2 (16 entries, codes 0x00-0x0C)
~~~~~

## Wotan Error Category Registry (NEW in draft-03)

~~~~~
Registry Name:  Unheaded Wotan Error Category Codes
Template:       Category Code (0x00-0xFF), Category Name, Description,
                Specification Reference
Policy:         Specification Required

Initial entries: See Section 3.2 (12 entries, codes 0x00-0x0B)
~~~~~

--- back

# Changes from draft-bellis-unheaded-wotan-memory-02

The following changes are made in draft-03:

1. **Error Code Taxonomy (NEW)**: Added Section 3 defining a structured
   32-bit error code format with severity levels (INFO, WARNING, ERROR,
   CRITICAL, FATAL), origin codes (16 subsystems), and category codes
   (12 categories). Provides fine-grained error classification beyond
   standard errno codes.

2. **Helper Return Codes (ENHANCED)**: Added structured error code
   annotations to all BPF helper return values (Section 5). Each errno
   return now maps to a full 32-bit structured code. Added auxiliary
   error detail mechanism via per-CPU BPF array map
   (wotan_last_error).

3. **Error Recovery Procedures (NEW)**: Added Section 3.5 defining
   recovery procedures by severity level, automatic recovery state
   machine (HEALTHY -> DEGRADED -> RECOVERING -> FAILED), recovery
   metrics, and cross-subsystem recovery dependency graph.

4. **Cross-References to Foundation draft-06 and Sophia draft-03
   (UPDATED)**: Updated UNHEADED-FOUNDATION reference from draft-04 to
   draft-06. Updated UNHEADED-SOPHIA reference from draft-01 to
   draft-03. Added Section 1.1 documenting the specification family
   structure.

5. **Wotan Error Origin Registry (NEW IANA)**: Added IANA registry for
   error origin codes (0x00-0x1F).

6. **Wotan Error Category Registry (NEW IANA)**: Added IANA registry
   for error category codes (0x00-0xFF).

7. **Error Code Information Leakage (NEW Security)**: Added security
   consideration for structured error codes potentially revealing
   internal architecture details.

8. **Updated Date**: Changed date from 2026-02-27 to 2026-03-05.

All draft-02 content is retained, including security patches W1-W8.
No existing wire format, processing rule, or normative requirement
from draft-02 is modified or removed.

# Acknowledgments

The Linux kernel BPF community (Alexei Starovoitov, Daniel Borkmann,
Song Liu) for the infrastructure enabling per-packet computation in the
kernel datapath.

The authors of RFC 9669 (BPF ISA), RFC 8799 (Limited Domains), and
RFC 9673 (Hop-by-Hop Processing Rehabilitation) for the foundational
protocols that make this design possible.

This document was co-authored with assistance from Claude (Anthropic).

---

# Author's Address

Stevie Bellis
Unheaded
Email: stevie@bellis.tech
