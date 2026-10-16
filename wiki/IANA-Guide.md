# IANA Guide

How the Unheaded drafts write their IANA Considerations sections (per RFC 8126), and the registries they ask IANA to create. Error codes: [[Error Registry|Error-Registry]]. Drafts: [[Drafts-Index]].

## Registries requested (Foundation draft-06)

| Registry | Policy | Notes |
|----------|--------|-------|
| Monad Protocol Version Numbers | Standards Action | `0x01` only; frozen |
| Monad Flags | Specification Required | Byte 0x07: CHAOS, CANARY, TRACED, ENCRYPT, SAMPLED, MIRROR, CUSTOM, RSVD |
| Monad Flow Actions | Expert Review | FORWARD 0x00, TRACE, SAMPLE, MIRROR, RATE_LIMIT, DROP 0x05; PQC key ops 0x10–0x16; experimental 0xF0–0xFE |
| Kingdom Mode Values | Standards Action | NORMAL, PRIORITY, EXPERIMENTAL, RESERVED |
| IPv6 Hop-by-Hop Option Type | IETF Review | Suggested `0x3E` (`001xxxxx`: skip if unknown, may change en route) |
| IPv6 Next Header Type | Standards Action | |
| Sophia Dictionary Namespace | First Come First Served | Root keys `0x10–0xFE` |
| Anamnesis Event Types | Specification Required | BIRTH, COMPUTED, CHAOS, DEATH, ANOMALY, … |
| Error Codes | Specification Required | `0x00–0x0C` normative |
| TLV Type Allocations | Specification Required | |
| PQC Algorithm Identifiers | Specification Required | ML-DSA, SLH-DSA, ML-KEM |
| Wotan Topic Namespace | Expert Review | `<domain>.<event>` ([[Fae-Chamber-Contracts]]) |

Companion drafts add their own: Sophia sub-dictionary types and QPACK static table (Sophia-03), MBC opcodes (MBC-ISA-00).

## Writing the section

- Say whether IANA action is needed at all; if none, write "This document has no IANA actions."
- For each new registry: name, registry group ("Unheaded Protocol Parameters"), policy, value range, initial values with references, and which ranges are reserved / private use (guide: 25–50% of a namespace for private use).
- Use placeholders `TBA1`, `TBA2`, … for values IANA assigns, and tell the RFC Editor what each one stands for.
- Expert Review needs written review criteria and a named expert role.
- State change control explicitly for every range.

## Common mistakes

Missing TBA replacement instructions · gaps in a namespace description · ambiguous policy · Expert Review without criteria · undefined initial values · namespaces that can't grow cleanly · unstated change control · ignoring backward compatibility.

## Known drift

The source guide's §7 registry tables predate draft-06: its Flags table uses the old meanings (Crypto, Yielding, Token, Emergency, Sequential, Manifest) and its freeze table says K1/K0 for bits 1–0. Draft-06 and `ebpf/monad-common` agree on the names above. Draft-06 itself says "offset 0x0B" in the Flags prose and "offset 0x07" in the registry; the code uses 0x07.

---

> **Source:** [docs/protocol/references/iana-guide.md](../docs/protocol/references/iana-guide.md) (2026-02-28) · [draft-06](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) IANA section
