# RFC Cross-Reference

Which RFCs the Unheaded drafts build on, normative vs informative, by component. Drafts: [[Drafts-Index]]. Style rules for citing them: [[Wire Format Patterns|Wire-Format-Patterns]].

## What Foundation draft-06 cites

| Normative | For |
|-----------|-----|
| RFC 2119, RFC 8174 | Requirement keywords |
| RFC 8200 | IPv6 and Hop-by-Hop Options, which carry the Monad |
| RFC 9673 | Hop-by-Hop Options processing procedures |
| RFC 6437 | IPv6 Flow Label |
| RFC 8949 | CBOR (Sophia serialization) |
| RFC 9669 | BPF ISA (the drafts say "BPF", not "eBPF", per 9669) |
| RFC 9000, RFC 9114 | QUIC, HTTP/3 |

Informative: RFC 768 / 791 / 792 / 793 (UDP, IPv4, ICMP, TCP), 1918 and 4193 (private and ULA addressing), 4303 (ESP), 8126 (IANA considerations), 8300 (NSH), 8754 (Segment Routing Header), **8799 (Limited Domains: the scope the protocol is defined for)**, 9098 and 9288 (extension headers in operation and filtering at transit routers), 9180 (HPKE), 9197 and 9486 (IOAM data and its IPv6 options: the closest prior art), 9370, 9631, plus the companion Monad Extended Register draft.

## By component

| Component | Normative | Informative |
|-----------|-----------|-------------|
| Monad (HbH option) | 8200, 9673, 2119/8174 | 1071 checksum, 9197/9486 IOAM |
| Sophia | 8949, 2119/8174 | 8126, 9204 QPACK |
| Shim (BPF programs) | 9669 | 1071 |
| Kingdom Mode fabric | 7348 VXLAN, 4271 BGP-4, 7432 EVPN | 8365 EVPN overlay, 4456 route reflection, 9135 IRB, 9136 IP prefix routes, 3031 MPLS, 2328 OSPF |
| Transport | 9000 QUIC, 9114 HTTP/3 | 9001 TLS for QUIC, 9002 loss/congestion, 9204 QPACK |
| Security | 8446 TLS 1.3 | 4033–4035 DNSSEC, 9180 HPKE |
| Anamnesis | — | 8949, 8446 |
| Drafting | — | 7322 style guide, 7841 boilerplate, 8126 IANA |

## Notes

- The source matrix lists RFC 8365 as "A YANG Data Model for EVPN"; RFC 8365 is the EVPN network virtualization overlay (VXLAN) RFC.
- The source matrix predates draft-06 and does not include 8799, 9673, 6437, the IOAM RFCs or the extension-header operations RFCs; the table above adds them from draft-06's reference list.

---

> **Source:** [docs/protocol/references/rfc-crossref.md](../docs/protocol/references/rfc-crossref.md) (2026-02) · [draft-06](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) references
