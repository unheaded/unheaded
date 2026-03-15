# IANA Registration Strategy: Unheaded Protocol IPv6 Extension Headers

## Section 1: Background

IPv6 Hop-by-Hop and Destination Options extension headers are standardized in **RFC 8200** (IPv6 Specification) and **RFC 9008** (IPv6 Hop-by-Hop Options Processing Update). Option type assignments are managed by **IANA** under the "IPv6 Parameters" registry.

### Current Experimental Usage

The Unheaded Kingdom protocol currently uses **experimental option types** (not IANA-registered) for distributed tracing:

- **Type 0x2A**: `UNHEADED_METRIC_V1` — Primary Metrics Hop-by-Hop option
- **Type 0x2B**: `UNHEADED_AUDIT_TRAIL_V1` — Destination Options audit trail

Both option types are RFC-compliant:
- **High two bits**: `00` (skip if unknown, don't change option on route forward)
- **Compliance**: Follows RFC 8200 § 4.2 processing rules
- **Scope**: Experimental phase; for use on controlled networks only

### Option Type Encoding

```
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|H|C|    Type (6 bits)          |  Type field structure (8 bits)
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

H (1 bit): Home Agent (not used in Unheaded)
C (1 bit): Change mutable field (0 = don't change, 1 = may change)
Type (6 bits): Option identifier

Experimental range: 0x00–0x3F (first 64 types)
Standard/IANA range: 0x40–0xFF (remaining 192 types)

0x2A (binary: 00101010) → High bits = 00 (skip, don't change)
0x2B (binary: 00101011) → High bits = 00 (skip, don't change)
```

---

## Section 2: Registration Path

The Unheaded protocol will follow a **4-phase approach** to transition from experimental to permanent IANA allocations:

### Phase 1: Experimental Phase (Current — Alpha)

**Duration**: Until Beta release (Q3 2026 estimated)

**Actions**:
- Use experimental option types (0x2A, 0x2B) as assigned
- Document types in repository and Internet-Draft skeleton
- Deploy only on networks under Unheaded project control
- Test interoperability with RFC 8200 implementations

**Compliance**:
- Follow RFC 4727: "Experimental Options" — Types in this phase may be deprecated or realloc'd without notice
- All deployments must be prepared to renumber

### Phase 2: Internet-Draft Phase (Beta)

**Duration**: Q3 2026 — Q1 2027 (estimated)

**Actions**:
- Submit `draft-bellis-unheaded-metric-header-00` to IETF
- Request IANA expert review for provisional type allocations
- Coordinate with IETF 6MAN or INTAREA working group
- Publish revised Internet-Draft with proposed permanent types

**IANA Request**:
- Request two permanent IPv6 Hop-by-Hop/Destination Option Types
- Proposed names: `UNHEADED_METRIC_V1` and `UNHEADED_AUDIT_TRAIL_V1`
- Request specific type numbers (e.g., 0x50, 0x51) or accept IANA assignment

### Phase 3: Standards Track (Release/Stable)

**Duration**: Q1 2027 — Q4 2027 (estimated)

**Actions**:
- Advance Internet-Draft to RFC status via IETF standards track
- Receive permanent IANA type allocations
- Update all code to use allocated type numbers
- Deploy on production networks

**Deliverables**:
- RFC published (e.g., RFC XXXX)
- IANA registry updated
- Type numbers locked

### Phase 4: Permanent Assignment (Post-Release)

**Duration**: Q4 2027 and beyond

**Actions**:
- Maintain backward compatibility with published types
- Monitor for issues and publish errata if needed
- Transition from experimental to operational deployments

---

## Section 3: Internet-Draft Skeleton

### Document Title

**"IPv6 Hop-by-Hop and Destination Options for Distributed Tracing Metrics"**

*or*

**"IPv6 Extension Headers for Wire-Speed Distributed Tracing"**

### Authors

- Stevie Bellis (stevie@bellis.tech)
- [Future contributors to be added]

### Abstract

```
This document defines two IPv6 extension header options for embedding
distributed tracing metadata directly in IPv6 packets, enabling 
wire-speed observability without out-of-band telemetry systems.

Option Type 0xTBD1 (UNHEADED_METRIC_V1) carries complete W3C Trace
Context-compatible trace IDs, span IDs, timestamps, and service
metadata within a Hop-by-Hop option.

Option Type 0xTBD2 (UNHEADED_AUDIT_TRAIL_V1) records per-hop latency
measurements and routing metadata as packets traverse network
segments, enabling precise path analysis and bottleneck detection.

Both options are designed for use with eBPF-based observability
systems operating at kernel wire speed, providing sub-microsecond
latency measurements and zero-copy metadata collection.

The options are compatible with RFC 8200 processing rules and
operate safely in networks that do not recognize them.
```

### Table of Contents

```
1. Introduction
   1.1. Motivation
   1.2. Use Cases
   1.3. Design Principles

2. Conventions and Definitions
   2.1. Terminology
   2.2. Requirements Language

3. IPv6 Extension Header Options
   3.1. UNHEADED_METRIC_V1 (Hop-by-Hop)
        3.1.1. Option Format and Layout
        3.1.2. Trace ID and Span ID Format (W3C TraceContext)
        3.1.3. Timestamp Encoding
        3.1.4. Service Metadata
   
   3.2. UNHEADED_AUDIT_TRAIL_V1 (Destination)
        3.2.1. Option Format and Layout
        3.2.2. Per-Hop Latency Records
        3.2.3. Hop Count and Routing Metadata
        3.2.4. Timestamp Precision

4. Processing Rules
   4.1. Sender Processing (Originating Host)
   4.2. Intermediate Router Processing
   4.3. Receiver Processing (Destination Host)
   4.4. Handling Unknown Options

5. Security Considerations
   5.1. Denial of Service Prevention
   5.2. Information Disclosure
   5.3. Spoofing and Authentication
   5.4. IPv6 Extension Header Fragmentation

6. Privacy Considerations
   6.1. Trace Context Information Leakage
   6.2. Path Correlation Attacks
   6.3. Recommendation: Encryption with IPsec

7. IANA Considerations
   7.1. IPv6 Hop-by-Hop Option Types Registry
   7.2. IPv6 Destination Options Registry

8. References
   8.1. Normative References
   8.2. Informative References

Appendices:
A. Example Deployments (eBPF observability)
B. Interoperability Testing
C. Trace Context to UNHEADED_METRIC_V1 Mapping
```

### Key Sections Details

**Section 3.1: UNHEADED_METRIC_V1 Format**

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |  Option Type  | Option Len    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Trace ID (16 bytes)                   |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         Span ID (8 bytes)                     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   Trace Flags |    Reserved   | Timestamp (8 bytes)          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    Service ID (4 bytes)       |    Baggage (variable length) |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

Total minimum length: 40 octets
Variable length: up to 255 octets (IPv6 option length constraint)
```

**Section 3.2: UNHEADED_AUDIT_TRAIL_V1 Format**

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |  Option Type  | Option Len    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    Hop Cnt    |Timestamp Prec.|    Reserved   |   Flags      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Hop Latency Record 1 (16 bytes per hop)                     |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Hop Latency Record 2...                                      |
...

Each hop record contains:
  - Source IPv6 (16 bytes) or IPv4-mapped IPv6 (16 bytes)
  - Per-hop latency delta (8 bytes, nanosecond precision)
  - Hop identifier (4 bytes: interface ID or AS number)
```

**Section 4: Processing Rules**

- Hosts MUST silently ignore unknown option types per RFC 8200 § 4.2
- Intermediate routers MUST NOT modify options marked as immutable (C bit = 0)
- Receivers MUST validate timestamp ordering and detect loops
- Overflow handling: If audit trail exceeds IPv6 option max (255 bytes), truncate with warning flag

**Section 5: Security**

- Options do NOT provide authentication or integrity protection; use IPsec/TLS for security
- DoS risk: Malicious senders can craft options with timestamps far in future; routers SHOULD rate-limit processing
- Information disclosure: Trace IDs and service metadata are unencrypted; use IPv6 encryption for confidential deployments
- Recommendations: Encrypt with IPsec AH/ESP, validate timestamps, use filtering on trust boundaries

---

## Section 4: Experimental Usage Notice

**Until IANA permanently allocates option types, users of the Unheaded protocol MUST adhere to the following:**

1. **Deployment Scope**: Only deploy on networks under your direct control or with explicit network operator consent.

2. **No Public Registration**: Do not advertise or register experimental types in any IANA registry or public namespace.

3. **Prepared for Renumbering**: Be prepared to update all deployments when permanent type allocations are received. The Unheaded project will provide migration tools and documentation.

4. **RFC 4727 Compliance**: Understand that experimental option types may be reclaimed or reallocated without notice, per RFC 4727.

5. **Testing Only**: Use experimental types for development, testing, and internal deployments. Do not rely on type numbers 0x2A, 0x2B for production commercial services until IANA allocation is confirmed.

6. **Documentation**: All experimental deployments MUST include documentation stating:
   ```
   "This deployment uses experimental IPv6 option types 0x2A (UNHEADED_METRIC_V1)
    and 0x2B (UNHEADED_AUDIT_TRAIL_V1). These types are not IANA-registered and
    may be reallocated. This is for experimental use only."
   ```

---

## Section 5: Related Registrations

Beyond IPv6 extension header types, the Unheaded protocol may require registration in several IANA registries:

### IPFIX Information Elements

**Rationale**: When metrics are exported to IPFIX collectors, custom Information Elements (IEs) may be needed.

**Planned registrations**:
- `unheaded.metric.trace_id` (128-bit value)
- `unheaded.metric.span_id` (64-bit value)
- `unheaded.metric.service_id` (32-bit identifier)
- `unheaded.metric.hop_latency` (64-bit nanosecond value)

**Timeline**: Phase 3 (Standards Track)

### Prometheus Metric Namespace

**Rationale**: eBPF collectors export metrics in Prometheus format.

**Planned names**:
- `unheaded_packet_count_total` (counter)
- `unheaded_latency_seconds_bucket` (histogram)
- `unheaded_packet_loss_ratio` (gauge)
- `unheaded_span_active_total` (gauge)

**Timeline**: Phase 2 (Internet-Draft)

### gRPC Service Names

**Rationale**: Collection and management services use gRPC.

**Planned services**:
- `unheaded.v1.MetricsCollector` (metrics export)
- `unheaded.v1.TraceContext` (trace query)
- `unheaded.v1.ConfigManagement` (deployment config)

**Timeline**: Phase 1 (Experimental)

### QUIC Application Protocol (ALPN)

**Rationale**: Future QUIC-based transport may need ALPN registration.

**Proposed ALPN**:
- `unheaded/1.0` (Unheaded metrics protocol v1)

**Timeline**: Phase 3 (Standards Track)

---

## Section 6: Contact & Governance

### IANA Registration Inquiries

**Primary Contact**:
- Name: Stevie Bellis
- Email: stevie@bellis.tech
- Organization: Unheaded Kingdom Protocol Project

**Secondary Contact**: TBD

### Protocol Working Group

**Current Status**: Not yet chartered by IETF

**Future**: A protocol working group will be established at:
- **Website**: bellis.tech/unheaded/wg (placeholder)
- **GitHub**: github.com/stevebellis/unheaded

**Participation**: Open to all contributors and interested parties

### Process & Timeline

1. **Q2 2026**: Publish Internet-Draft skeleton (`draft-bellis-unheaded-metric-header-00`)
2. **Q3 2026**: Request IANA expert review for provisional allocations
3. **Q4 2026**: Advance to IETF working group (if chartered)
4. **Q1 2027**: RFC publication and permanent allocation
5. **Q2 2027**: Update all code; begin production deployment

---

## Section 7: Related Documents

**Internal References**:
- `CONTRIBUTOR-GUIDE.md` — Development and protocol change procedures
- `docs/protocol/MONAD-SPECIFICATION.md` — Monad protocol (IPv6 extension context)
- `docs/architecture/PHYLACTERY.md` — System architecture
- `pkg/protocol/ipv6options.go` — Current type 0x2A, 0x2B implementation

**External References**:
- RFC 8200 — Internet Protocol, Version 6 (IPv6) Specification
- RFC 9008 — IPv6 Hop-by-Hop Options Processing Update
- RFC 4727 — IPv6 Global Unicast Address Format
- RFC 3692 — Assigning Experimental and Testing Numbers Considered Useful
- W3C Trace Context — https://www.w3.org/TR/trace-context/

---

## Section 8: Version History

| Date | Version | Status | Changes |
|------|---------|--------|---------|
| 2026-02-25 | 1.0 | Draft | Initial strategy document |
| TBD | 1.1 | Proposed | IANA expert review feedback |
| TBD | 2.0 | Final | Post-RFC publication |

---

**Last Updated**: 2026-02-25  
**Sprint**: S52 Legal Sprint  
**Maintained By**: Stevie Bellis (stevie@bellis.tech)
