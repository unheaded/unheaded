# Error Registry

The protocol's 8-bit error codes: what each means, its blast radius, and what a receiver does. Proposed as an IANA registry in the Foundation draft ([[IANA Guide|IANA-Guide]]).

## Codes

| Code | Level | Name | Receiver action |
|------|-------|------|-----------------|
| `0x00` | — | NO_ERROR | Continue |
| `0x01` | Flow | CRC_VALIDATION_FAILED | Drop, log, optionally alert |
| `0x02` | Domain | VERSION_NOT_SUPPORTED | Drop silently (no response) |
| `0x03` | Flow | FLOW_LABEL_INVALID | Drop, optional error response |
| `0x04` | Flow | ARITHMETIC_OVERFLOW | Reject the operation; never crash |
| `0x05` | Flow | WOTAN_HELPER_BOUNDS_CHECK_FAILED | Helper returns error, memory untouched |
| `0x06` | Domain | MULTIPLE_HBH_HEADERS | Drop immediately, no response |
| `0x07` | Domain | UNKNOWN_CRITICAL_TLV | Drop (critical bit C=1), optional error response |
| `0x08` | Domain | WAL_SEQNO_DISCONTINUITY | Alert, replay from last good seqno |
| `0x09` | System | INSUFFICIENT_BUFFER_SPACE | Reject new entry (Sophia: 128 entries / 1 MB per flow, 100 MB global) |
| `0x0A` | Flow | FLOW_STATE_CORRUPTION | Drop flow, close connection, critical alert |
| `0x0B` | Domain | TLS_HANDSHAKE_FAILURE | Close, log, no immediate retry |
| `0x0C` | Domain | QUIC_VERSION_MISMATCH | Drop, close connection |
| `0x0D` | — | RESERVED | Do not use |
| `0x0E–0x1E` | — | Unallocated | Expert Review to allocate |
| `0x1F`, `0xFF` | — | GREASE | Accept and ignore; never trigger recovery |
| `0x20–0xFE` | — | Private use | Implementation-defined |

**Levels:** Flow = close that flow only. Domain = every flow on the connection/node. System = the whole protocol instance (may need restart or reclamation).

## Allocation and change control

| Range | Change control |
|-------|----------------|
| `0x00–0x0D` | Standards Action (RFC required); all implementations MUST support |
| `0x0E–0x1E` | Specification Required (designated expert checks: clear semantics, no overlap, practical need) |
| `0x1F`, `0xFF` | Fixed grease values, never reassigned |
| `0x20–0xFE` | Uncontrolled |

Implementations send grease values occasionally (fixed and pattern-based) so receivers that choke on unknown codes are found early.

## Notes

- `0x01` covers two checks: the Monad's CRC-16/CCITT-FALSE over octets 0x00–0x11, and the Wotan WAL entry's CRC-32 / HMAC-SHA256.
- Names and meanings follow draft-06 Section 11.10 (e.g. `0x05` is `WOTAN_BOUNDS_CHECK_FAILED`; `0x09` is first of all "no space for the Monad in the packet").
- The registry is a spec artifact; the codes are not yet one shared constant set in `pkg/` or `ebpf/`.

---

> **Source:** [docs/protocol/error-registry.md](../docs/protocol/error-registry.md) (re-aligned with Foundation draft-06 on 2026-10-17; examples and RFC patch cross-refs there)
