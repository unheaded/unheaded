# Cross-Reference Matrix — S77 Protocol Spec Advancement

**Date:** March 5, 2026
**Sprint:** S77 (Phase 4: Protocol Spec Advancement)

This matrix documents all cross-references between the three Unheaded Protocol specifications at their current draft versions.

## Specification Versions

| Spec | Draft | Date | Status |
|------|-------|------|--------|
| Foundation | draft-06 | 2026-03-05 | Current |
| Sophia | draft-03 | 2026-03-05 | Current |
| Wotan | draft-03 | 2026-03-05 | Current |

## Cross-Reference Matrix

### Foundation draft-06 references to other specs

| Foundation Section | References | Target Spec | Target Section | Purpose |
|---|---|---|---|---|
| 1.2 (Cross-References) | [SOPHIA] | Sophia draft-03 | Entire doc | Introduces spec family |
| 1.2 (Cross-References) | [WOTAN] | Wotan draft-03 | Entire doc | Introduces spec family |
| 4 (Protocol Overview) | [SOPHIA] | Sophia draft-03 | Dictionary Model | Field semantics |
| 4 (Protocol Overview) | [WOTAN] | Wotan draft-03 | Architecture | Per-flow state |
| 5.1 (Monad Register File) | [SOPHIA] | Sophia draft-03 | Lookup Chain | src/dst_service_id lookup |
| 5.2 (Flags Bitfield) | [SOPHIA] | Sophia draft-03 | Dictionary Model | CUSTOM flag interpretation |
| 6.1 (Exponent Encoding) | [SOPHIA] | Sophia draft-03 | Exponent Rules | Base/multiplier definitions |
| 6.2 (Sophia Wire Format) | [WOTAN] | Wotan draft-03 | Topic-Based I/O | Distribution via topics |
| 7 (Extended Dictionary) | [SOPHIA] | Sophia draft-03 | Sub-Dictionary Types | Hierarchical knowledge |
| 7 (Extended Dictionary) | [WOTAN] | Wotan draft-03 | Architecture | BPF map replacement |
| 9 (IANA Registration) | [SOPHIA] | Sophia draft-03 | Observability Entry | Metric type references |
| 9.2 (UNHEADED_METRIC_V1) | [SOPHIA] | Sophia draft-03 | Service Identity | Service ID mapping |
| 9.2 (UNHEADED_METRIC_V1) | [WOTAN] | Wotan draft-03 | Per-Flow State | Flow Label correlation |
| 10.2 (Integrity) | [SOPHIA] | Sophia draft-03 | PQC Key Entry | HMAC key distribution |
| 11 (Backwards Compat) | [SOPHIA] | Sophia draft-03 | Entire doc | Companion spec version |
| 11 (Backwards Compat) | [WOTAN] | Wotan draft-03 | Entire doc | Companion spec version |

### Sophia draft-03 references to other specs

| Sophia Section | References | Target Spec | Target Section | Purpose |
|---|---|---|---|---|
| 1.1 (Cross-References) | [UNHEADED-FOUNDATION] | Foundation draft-06 | Entire doc | Spec family |
| 1.1 (Cross-References) | [WOTAN] | Wotan draft-03 | Entire doc | Spec family |
| 1 (Introduction) | [UNHEADED-FOUNDATION] | Foundation draft-06 | 5.1 (Monad) | 20-byte register file |
| 3.1 (Initialization) | [WOTAN] | Wotan draft-03 | Architecture | Initialization signaling |
| 3.2.3 (Nested Lookup) | [UNHEADED-FOUNDATION] | Foundation draft-06 | Anamnesis | EVENT_ANOMALY emission |
| 5 (QPACK Compression) | [WOTAN] | Wotan draft-03 | Topic-Based I/O | Distribution bandwidth |
| 6 (Distribution) | [WOTAN] | Wotan draft-03 | Topic-Based I/O | sophia.dictionary.v{N} |
| 6.1 (Wotan Channel) | [WOTAN] | Wotan draft-03 | gRPC Streaming | Topic subscription |
| 9.4 (Cross-Reference) | [UNHEADED-FOUNDATION] | Foundation draft-06 | 10 (Security) | Wire format immutability |
| 9.4 (Cross-Reference) | [WOTAN] | Wotan draft-03 | 12 (Security) | Topic injection attacks |

### Wotan draft-03 references to other specs

| Wotan Section | References | Target Spec | Target Section | Purpose |
|---|---|---|---|---|
| 1.1 (Cross-References) | [UNHEADED-FOUNDATION] | Foundation draft-06 | Entire doc | Spec family |
| 1.1 (Cross-References) | [UNHEADED-SOPHIA] | Sophia draft-03 | Entire doc | Spec family |
| 1 (Introduction) | [UNHEADED-FOUNDATION] | Foundation draft-06 | 5.1 (Monad) | 20-byte register file |
| 1 (Introduction) | [UNHEADED-SOPHIA] | Sophia draft-03 | Dictionary Model | L4 dictionaries |
| 3.2 (Error Origins) | [UNHEADED-SOPHIA] | Sophia draft-03 | Lookup Chain | SOPHIA_LOOKUP origin |
| 3.4.5 (Sophia Errors) | [UNHEADED-SOPHIA] | Sophia draft-03 | Sub-Dict Types | Nesting depth / circular ref |
| 3.4.5 (Sophia Errors) | [UNHEADED-SOPHIA] | Sophia draft-03 | QPACK | Decompression failure |
| 3.5.1 (Recovery) | [UNHEADED-FOUNDATION] | Foundation draft-06 | 10.3 (Kingdom Mode) | Emergency Mode reference |
| 4 (Architecture) | [UNHEADED-FOUNDATION] | Foundation draft-06 | 5.1 (Monad) | Monad computation |
| 5.4 (Error Handling) | [UNHEADED-FOUNDATION] | Foundation draft-06 | Anamnesis | EVENT_ANOMALY emission |
| 12.1 (Topic Injection) | [UNHEADED-SOPHIA] | Sophia draft-03 | Access Control | Per-program whitelist |
| 12.8 (Cross-Reference) | [UNHEADED-FOUNDATION] | Foundation draft-06 | 10 (Security) | Wire format immutability |
| 12.8 (Cross-Reference) | [UNHEADED-SOPHIA] | Sophia draft-03 | 9 (Security) | Dictionary poisoning |

## Shared Concepts

The following concepts are defined in one spec and referenced by all three:

| Concept | Defined In | Referenced By |
|---|---|---|
| Monad wire format (20 bytes, FROZEN v0x01) | Foundation 5.1 | Sophia 1, Wotan 1 |
| Exponent encoding | Foundation 6 | Sophia 3, Wotan 1 |
| BPF hash maps (RFC 9669) | Foundation 5 | Sophia 5, Wotan 5 |
| Anamnesis events | Foundation 8 | Sophia 3.2, Wotan 3.5 |
| Limited Domain (RFC 8799) | Foundation 1 | Sophia 1, Wotan 1 |
| Shield ingress/egress | Foundation 7 | Sophia 6, Wotan 4 |
| Sophia dictionary lookup | Sophia 3 | Foundation 6, Wotan 3.4 |
| Sub-dictionary type system | Sophia 3.2 | Foundation 7, Wotan 3.4 |
| QPACK compression | Sophia 5 | Foundation 6, Wotan 6 |
| Wotan topic I/O | Wotan 8 | Foundation 6, Sophia 6 |
| Error code taxonomy | Wotan 3 | Foundation 18, Sophia 9 |
| BPF helper functions | Wotan 5 | Foundation 11, Sophia 5 |
| WAL (Write-Ahead Log) | Wotan 7 | Foundation 11, Sophia 6 |

## IANA Registry Cross-References

| Registry | Defined In | Used By |
|---|---|---|
| Monad Protocol Version Numbers | Foundation 18.2 | Sophia, Wotan |
| Monad Flags Bitfield | Foundation 18.3 | Sophia, Wotan |
| Monad Flow Actions | Foundation 18.4 | Sophia, Wotan |
| Kingdom Mode Values | Foundation 18.5 | Sophia, Wotan |
| Sophia Root Dictionary Keys | Sophia 10.1 | Foundation, Wotan |
| Sophia Sub-Dictionary Types | Sophia 10.2 | Foundation, Wotan |
| Sophia QPACK Static Table | Sophia 10.3 | Foundation, Wotan |
| Anamnesis Event Types | Foundation 18.9 | Sophia, Wotan |
| Error Codes | Foundation 18.10 | Sophia, Wotan |
| TLV Type Allocations | Foundation 18.11 | Sophia, Wotan |
| PQC Algorithm Identifiers | Foundation 18.12 | Sophia |
| Wotan Topic Namespace | Wotan 13.1 | Foundation, Sophia |
| Wotan Error Origin Codes | Wotan 13.2 | Foundation, Sophia |
| Wotan Error Category Codes | Wotan 13.3 | Foundation, Sophia |
