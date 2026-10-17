# IANA Guide

How the Unheaded drafts write their IANA Considerations sections (per RFC 8126), and the registries they ask IANA to create. Error codes: [[Error Registry|Error-Registry]]. Drafts: [[Drafts-Index]].

## Registries requested (Foundation draft-06 Section 11: 13 registries + 2 code points)

| Registry | Policy | Notes |
|----------|--------|-------|
| Monad Protocol Version Numbers | Standards Action | `0x01` only; frozen |
| Monad Flags | Specification Required | Byte 0x07: CHAOS, CANARY, TRACED, ENCRYPT, SAMPLED, MIRROR, CUSTOM, RSVD |
| Monad Flow Actions | Expert Review | FORWARD 0x00, TRACE, SAMPLE, DROP, MIRROR, RATE_LIMIT, REDIRECT, INJECT 0x07; PQC key ops 0x10–0x14; experimental 0xF0–0xFE |
| Kingdom Mode Values | Standards Action | NORMAL, PRIORITY, EXPERIMENTAL, RESERVED |
| Sophia Dictionary Namespace | First Come First Served | Root keys `0x10–0xFE` |
| Anamnesis Event Types | Specification Required | BORN, COMPUTED, WOTAN_RD/WR, CHAOS, ROLLBACK, DIED, KEY_OP, ANOMALY (0x00–0x08) |
| Error Codes | Specification Required | `0x00–0x0C` normative |
| TLV Type Allocations | Specification Required | |
| PQC Algorithm Identifiers | Specification Required | ML-KEM-768/1024, ML-DSA-65/87, SLH-DSA-SHA2-128s |
| UPC MBC Opcode Numbers | Specification Required | Bits [31:24] of each 32-bit MBC word |
| UPC MBC Syscall Numbers | Expert Review | Native 0x01–0x0F; Linux-compatible via INT 0x80 |
| UPC Memory Region Types | Specification Required | RAM, KBD_IO, SCREEN, DEBUG, WAD, HEAP, STACK, RAMDISK |
| UPC Event Types | Specification Required | 0x10–0x19 compute events |

Plus two code points in existing IANA registries: the IPv6 Hop-by-Hop Option Type (suggested `0x3E`, `001xxxxx`: skip if unknown, may change en route) and IPv6 Next Header `0xFE`. 254 is an RFC 3692 experimentation value, so a real request would need another number.

Companion drafts add their own: Wotan Topic Namespace, Expert Review, `<domain>.<event>` (Wotan-03, [[Fae-Chamber-Contracts]]); Sophia sub-dictionary types and QPACK static table (Sophia-03).

## Writing the section

- Say whether IANA action is needed at all; if none, write "This document has no IANA actions."
- For each new registry: name, registry group ("Unheaded Protocol Parameters"), policy, value range, initial values with references, and which ranges are reserved / private use (guide: 25–50% of a namespace for private use).
- Use placeholders `TBA1`, `TBA2`, … for values IANA assigns, and tell the RFC Editor what each one stands for.
- Expert Review needs written review criteria and a named expert role.
- State change control explicitly for every range.

## Common mistakes

Missing TBA replacement instructions · gaps in a namespace description · ambiguous policy · Expert Review without criteria · undefined initial values · namespaces that can't grow cleanly · unstated change control · ignoring backward compatibility.

## Open question

The PQC draft signals compliance tiers with Kingdom Mode bits "K1|K0 in the Monad flags byte", but all 8 flag bits are assigned and neither draft-06 nor the code places the Kingdom Mode bits anywhere on the wire (`pkg/policy` holds the tier as config).

---

> **Source:** [docs/protocol/references/iana-guide.md](../docs/protocol/references/iana-guide.md) (§7 re-synced with draft-06 on 2026-10-17) · [draft-06](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) IANA section
