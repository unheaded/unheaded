# IANA Considerations — draft-bellis-unheaded-protocol-foundation-06
## Drafted: 2026-02-27 | For Integration into Foundation Spec Section 14

---

This document requests the following IANA actions.

## 14.1. IPv6 Hop-by-Hop Option Type

This document requests IANA to assign a value from the "Destination
Options and Hop-by-Hop Options" registry [IANA-HBH] for the Monad
option defined in Section 5.2.

The option type encoding follows the format defined in [RFC8200]
Section 4.2:

```
   act = 00 (skip over this option and continue processing)
   chg = 1  (option data may change en-route)
```

The resulting option type format is 001xxxxx.

| Name | Value | Reference |
|------|-------|-----------|
| Monad Register File | 0x3E (suggested) | [this document], Section 5.2 |

The "act" bits are set to 00 because nodes that do not recognize
the Monad option MUST skip it without disrupting packet forwarding.
The "chg" bit is set to 1 because the Monad register is modified by
per-hop BPF programs (Shim processing) as defined in Section 5.

## 14.2. Monad Protocol Version Registry

This document requests IANA to create a new registry entitled
"Monad Protocol Version Numbers" in a new "Unheaded Protocol
Parameters" registry group.

Registration Policy: Standards Action [RFC8126]

| Value | Description | Reference |
|-------|-------------|-----------|
| 0x00 | Reserved (MUST NOT be used) | [this document] |
| 0x01 | Unheaded Protocol v1 | [this document] |
| 0x02-0xEF | Unassigned | |
| 0xF0-0xFE | Reserved for Experimental Use | [this document] |
| 0xFF | Reserved (MUST NOT be used) | [this document] |

The version field occupies offset 0x00 of the Monad register file
(Section 5.3). Receivers MUST drop packets with unknown version
values as specified in Section 5.3.

## 14.3. Monad Flags Bitfield Registry

This document requests IANA to create a new registry entitled
"Monad Flags" in the "Unheaded Protocol Parameters" registry group.

Registration Policy: Specification Required [RFC8126]

The flags field occupies offset 0x07 of the Monad register file
(Section 5.5). Each flag occupies a single bit position.

| Bit | Name | Description | Reference |
|-----|------|-------------|-----------|
| 7 (0x80) | CHAOS (C) | Chaos injection active (Yaldabaoth resilience testing) | [this document], Section 5.5 |
| 6 (0x40) | CANARY (Y) | Canary deployment path marker | [this document], Section 5.5 |
| 5 (0x20) | TRACED (T) | Full trace active (all hops emit to Anamnesis) | [this document], Section 5.5 |
| 4 (0x10) | ENCRYPT (E) | Payload encrypted (intra-Kingdom TLS) | [this document], Section 5.5 |
| 3 (0x08) | SAMPLED (S) | Statistical sampling active | [this document], Section 5.5 |
| 2 (0x04) | MIRROR (M) | Mirror copy (not original packet) | [this document], Section 5.5 |
| 1 (0x02) | CUSTOM | Scratch and checksum fields carry exponent-encoded values | [this document], Section 5.5 |
| 0 (0x01) | RSVD | Reserved. Senders MUST set to zero. Receivers MUST ignore. | [this document], Section 5.5 |

Designated Expert Guidelines: The designated expert(s) SHOULD verify
that new flag assignments do not conflict with existing bit semantics,
that the specification clearly defines when the flag is set and cleared,
and that the flag serves a purpose not achievable through existing
Sophia dictionary entries.

Note: The flags byte is fully allocated. Future flag assignments
require either repurposing bit 0 (RSVD) or use of the Extended
Register Option [MONAD-EXT-REG].

## 14.4. Flow Action Registry

This document requests IANA to create a new registry entitled
"Monad Flow Actions" in the "Unheaded Protocol Parameters" registry
group.

Registration Policy: Expert Review [RFC8126]

The flow_action field occupies offset 0x05 of the Monad register
file (Section 5.3). Values are exponent-encoded; the registry tracks
the semantic meaning of decoded values as defined by Sophia dictionaries.

| Value | Name | Description | Reference |
|-------|------|-------------|-----------|
| 0x00 | FORWARD | Normal forwarding (default action) | [this document] |
| 0x01 | TRACE | Full event logging to Anamnesis | [this document] |
| 0x02 | SAMPLE | Probabilistic event logging | [this document] |
| 0x03 | DROP | Discard packet | [this document] |
| 0x04 | MIRROR | Clone packet to monitoring interface | [this document] |
| 0x05 | RATE_LIMIT | Apply rate limiting policy | [this document] |
| 0x06 | REDIRECT | Redirect to alternate destination | [this document] |
| 0x07 | INJECT | Inject synthetic packet | [this document] |
| 0x08-0x0F | Unassigned | | |
| 0x10 | KEY_ROTATE | PQC key rotation event | [this document] |
| 0x11 | KEY_REVOKE | PQC key revocation event | [this document] |
| 0x12 | KEY_DISTRIBUTE | PQC key distribution event | [this document] |
| 0x13 | KEY_VERIFY | PQC key verification event | [this document] |
| 0x14 | KEY_CHALLENGE | PQC challenge-response | [this document] |
| 0x15-0xEF | Unassigned | | |
| 0xF0-0xFE | Reserved for Experimental Use | | [this document] |
| 0xFF | Reserved | MUST NOT be used | [this document] |

Designated Expert Guidelines: The designated expert(s) SHOULD verify
that the proposed action has clear packet-level semantics (what happens
to the packet when this action is set), does not duplicate existing
actions, and includes a reference to the Sophia dictionary entry
that maps the exponent value to this action.

## 14.5. Kingdom Mode Registry

This document requests IANA to create a new registry entitled
"Kingdom Mode Values" in the "Unheaded Protocol Parameters" registry
group.

Registration Policy: Standards Action [RFC8126]

Kingdom Mode is encoded in the two low-order bits of the flags byte
(K1|K0, which in the current allocation corresponds to CUSTOM and
RSVD bits). When Kingdom Mode is active (see Section 10), these bits
indicate the operational mode within the Limited Domain.

NOTE: Kingdom Mode is signaled out-of-band (via Sophia configuration)
and the K1|K0 encoding is carried in the Extended Register Option
[MONAD-EXT-REG], not in the base Monad flags byte. This avoids
overloading the CUSTOM and RSVD bits.

| Value | Name | Description | Reference |
|-------|------|-------------|-----------|
| 0b00 | NORMAL | Standard processing, no address reclamation | [this document], Section 10 |
| 0b01 | PRIORITY | Priority processing with address-aware routing | [this document], Section 10 |
| 0b10 | EXPERIMENTAL | Experimental mode for development/testing | [this document], Section 10 |
| 0b11 | RESERVED | Reserved for future use. MUST NOT be used. | [this document] |

The Kingdom Mode registry is intentionally small (2 bits, 4 values)
because mode transitions are infrequent and high-impact operations.
Expansion beyond 4 values requires a wire format revision.

---

## Registry Group Summary

All registries defined in this document are placed in a new
"Unheaded Protocol Parameters" registry group, consistent with
IANA practice for protocol families (cf. "QUIC Transport Parameters"
for [RFC9000]).

| Registry | Size | Policy | Initial Entries |
|----------|------|--------|----------------|
| Monad Protocol Version Numbers | 8-bit (0-255) | Standards Action | 2 |
| Monad Flags | 8-bit bitfield | Specification Required | 8 |
| Monad Flow Actions | 8-bit (0-255) | Expert Review | 13 |
| Kingdom Mode Values | 2-bit (0-3) | Standards Action | 4 |
| IPv6 HbH Option Type | per RFC 8200 | Standards Action | 1 |

---

## Security Implications of IANA Registrations

The Monad Flags and Flow Actions registries have direct security
implications:

- The CHAOS flag (C) enables fault injection. Unauthorized use could
  disrupt service. Implementations MUST verify CHAOS is only set by
  authorized Shield instances.

- The DROP flow action terminates packet forwarding. Unauthorized
  injection of DROP actions constitutes a denial-of-service vector.

- Kingdom Mode EXPERIMENTAL bypasses production routing policy.
  This mode MUST be restricted to designated test domains.

These considerations are detailed in the Security Considerations
section (Section 15).

---

*IANA Considerations Draft — Forged 2026-02-27*
*5 registries. 1 registry group. Wire format protected.*
