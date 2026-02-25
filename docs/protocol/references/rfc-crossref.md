# RFC Cross-Reference Matrix for Unheaded Protocol

## Overview

This document provides a comprehensive cross-reference matrix mapping Unheaded protocol components to relevant RFCs. It serves as a definitive guide for RFC citation in Unheaded specifications and ensures consistency across the protocol documentation suite.

---

## 1. Normative References

These RFCs **MUST** be cited in Unheaded specifications as they define required behavior essential to protocol implementation.

| RFC | Title | Use Case | Component |
|-----|-------|----------|-----------|
| RFC 8200 | Internet Protocol, Version 6 (IPv6) Specification | Defines IPv6 packet structure and Hop-by-Hop (HbH) extension headers used by Monad for metadata propagation | Monad HbH Extension |
| RFC 9669 | BPF Instruction Set Architecture (ISA) | Defines eBPF instruction set architecture (ISA) used by Sophia/Shim eBPF programs for packet processing | Sophia/Shim eBPF Programs |
| RFC 2119 | Key words for use in RFCs to Indicate Requirement Levels | Defines requirement level keywords (MUST, SHOULD, MAY, etc.) for normative language in Unheaded specs | All Components |
| RFC 8174 | Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words | Clarifies capitalization rules for RFC 2119 keywords to ensure unambiguous requirement specification | All Components |
| RFC 8126 | Guidelines for Writing IANA Considerations Sections in RFCs | Defines procedures for IANA registry creation and management for Unheaded registries | Registry Management |
| RFC 8949 | Concise Binary Object Representation (CBOR) | Defines CBOR encoding used for Sophia dictionary serialization and protocol data representation | Sophia Dictionaries |

---

## 2. Informative References by Domain

### 2.1 Overlay and Fabric Networking

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 7348 | Virtual eXtensible Local Area Network (VXLAN): A Framework for Overlaying Virtualized Layer 2 Networks over Layer 3 Networks | Defines VXLAN tunnel encapsulation mechanism used in Unheaded fabric networking for multi-tenant isolation |
| RFC 8365 | A YANG Data Model for Ethernet VPN (EVPN) | Extends VXLAN with EVPN control plane procedures for dynamic MAC/IP learning and failover |

### 2.2 Routing Protocols

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 4271 | A Border Gateway Protocol 4 (BGP-4) | Defines BGP-4 routing protocol used by Kingdom Mode for inter-fabric route distribution and EVPN control |
| RFC 4456 | BGP Route Reflection: An Alternative to Full Mesh Internal BGP (iBGP) | Describes route reflection architecture for BGP EVPN control plane scaling |
| RFC 2328 | OSPF Version 2 | Alternative intra-fabric routing protocol option; reference for link-state algorithms |
| RFC 3031 | Multiprotocol Label Switching Architecture | Describes MPLS label switching used in advanced routing scenarios; baseline for segment routing |

### 2.3 EVPN Control Plane

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 7432 | BGP MPLS-Based Ethernet VPN | Foundational EVPN specification defining EVPN address families and route types |
| RFC 9135 | BGP EVPN Integrated Routing and Bridging (IRB) | Extends EVPN for inter-VLAN routing and IP mobility in Unheaded fabric environments |
| RFC 9136 | IP Prefix Advertisement in BGP EVPN | Defines EVPN IP Prefix Route (Type 5) for prefix reachability in multi-tenant networks |

### 2.4 Transport Layer Protocols

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 9000 | QUIC: A UDP-Based Multiplexed and Secure Transport | Modern transport protocol option for Unheaded inter-node communication with reduced latency |
| RFC 9001 | Using TLS 1.3 with QUIC | Defines TLS 1.3 integration with QUIC for secure Unheaded control plane communication |
| RFC 9002 | QUIC Loss Detection and Congestion Control | Specifies congestion control algorithms for QUIC-based Unheaded fabric communication |

### 2.5 Application Layer Protocols

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 9114 | HTTP/3 | HTTP/3 over QUIC for REST API access to Unheaded control plane and management interfaces |
| RFC 9204 | QPACK: Header Compression for HTTP/3 | Header compression for HTTP/3 based Unheaded management protocols |

### 2.6 Security and Cryptography

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 8446 | The Transport Layer Security (TLS) Protocol Version 1.3 | Defines TLS 1.3 for securing control plane communication in Unheaded deployments |
| RFC 4033 | DNS Security Introduction and Requirements | Defines DNSSEC framework for securing Unheaded DNS-based service discovery |
| RFC 4034 | Resource Records for the DNS Security Extensions | Specifies DNSSEC resource record types for Unheaded security |
| RFC 4035 | Protocol Modifications for the DNS Security Extensions | Defines DNSSEC protocol mechanics for Unheaded DNS security |

### 2.7 Checksums and Data Integrity

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 1071 | Computing the Internet Checksum | Defines checksum algorithms for Monad and Shim packet header validation |

### 2.8 RFC Process and Documentation

| RFC | Title | Relevance |
|-----|-------|-----------|
| RFC 7322 | RFC Style Guide | Guidelines for formatting and style in Unheaded RFC submissions |
| RFC 7841 | RFC Streams, Headers, and Boilerplate | Defines RFC document structure and mandatory boilerplate for Unheaded specifications |
| RFC 8729 | The RFC Series and RFC Editor | Overview of RFC publication process and standards for Unheaded protocol documentation |

---

## 3. Component-to-RFC Matrix

Comprehensive mapping of each Unheaded protocol component to applicable RFCs with classification and rationale.

### 3.1 Monad HbH Extension Header

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 8200 | **Normative** | Defines IPv6 Hop-by-Hop extension header structure and processing rules |
| RFC 1071 | Informative | Checksum computation for Monad metadata validation |
| RFC 2119/8174 | Normative | Requirement keywords for Monad specification |

### 3.2 Sophia Dictionary Serialization

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 8949 | **Normative** | CBOR encoding format for Sophia dictionary representation |
| RFC 2119/8174 | Normative | Requirement keywords for Sophia specification |
| RFC 8126 | Informative | IANA registry procedures for Sophia type allocations |

### 3.3 Shim eBPF Programs

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 9669 | **Normative** | eBPF instruction set architecture and program semantics |
| RFC 2119/8174 | Normative | Requirement keywords for Shim specification |
| RFC 1071 | Informative | Checksum computation in eBPF packet programs |

### 3.4 VXLAN Fabric Encapsulation

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 7348 | **Normative** | VXLAN tunnel encapsulation and VNI allocation |
| RFC 8365 | Informative | EVPN-VXLAN integration for dynamic tunnel management |
| RFC 2119/8174 | Normative | Requirement keywords for fabric specification |
| RFC 8126 | Informative | IANA registry for VNI allocation procedures |

### 3.5 BGP Routing (Kingdom Mode)

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 4271 | **Normative** | BGP-4 protocol specification and route distribution |
| RFC 4456 | Informative | Route reflection for iBGP scalability in Kingdom deployments |
| RFC 2119/8174 | Normative | Requirement keywords for Kingdom specification |
| RFC 3031 | Informative | MPLS labeling for advanced routing topologies |

### 3.6 EVPN Control Plane

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 7432 | **Normative** | EVPN address families, route types, and procedures |
| RFC 9135 | Informative | Integrated Routing and Bridging (IRB) for inter-VLAN communication |
| RFC 9136 | Informative | IP Prefix routes for multi-tenant IP reachability |
| RFC 2119/8174 | Normative | Requirement keywords for EVPN specification |
| RFC 4271 | Normative | BGP base protocol for EVPN route distribution |

### 3.7 Anamnesis Protocol State

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 8949 | Informative | CBOR serialization for state snapshots |
| RFC 8446 | Informative | TLS 1.3 for secure state replication channels |
| RFC 2119/8174 | Normative | Requirement keywords for Anamnesis specification |

### 3.8 Shield Security Framework

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 8446 | **Normative** | TLS 1.3 for encrypted control plane communication |
| RFC 4033/4034/4035 | Informative | DNSSEC for authenticated service discovery |
| RFC 2119/8174 | Normative | Requirement keywords for Shield specification |
| RFC 8126 | Informative | IANA procedures for Shield policy registries |

### 3.9 Wotan Memory Protocol

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 2119/8174 | Normative | Requirement keywords for consensus specification |
| RFC 8949 | Informative | CBOR for consensus message encoding |
| RFC 8446 | Informative | TLS 1.3 for secure consensus communication |

### 3.10 Interconnect and Inter-Fabric Communication

| RFC | Classification | Reason |
|-----|-----------------|--------|
| RFC 9000 | Informative | QUIC transport for low-latency inter-fabric links |
| RFC 9001 | Informative | QUIC-TLS integration for encrypted inter-fabric tunnels |
| RFC 9002 | Informative | QUIC congestion control for multi-fabric topologies |
| RFC 8446 | Informative | TLS 1.3 as alternative encrypted transport |
| RFC 4271 | Informative | BGP for multi-fabric route distribution |

---

## 4. Citation Tag Reference

Standard citation tags for commonly referenced RFCs in Unheaded documentation:

```markdown
[RFC2119]      Bradner, S., "Key words for use in RFCs to Indicate
               Requirement Levels", BCP 14, RFC 2119,
               DOI 10.17487/RFC2119, March 1997.

[RFC1071]      Braden, R., Borman, D., Partridge, C., and W. Plummer,
               "Computing the Internet Checksum", RFC 1071,
               DOI 10.17487/RFC1071, September 1988.

[RFC2328]      Moy, J., "OSPF Version 2", STD 54, RFC 2328,
               DOI 10.17487/RFC2328, April 1998.

[RFC3031]      Rosen, E., Viswanathan, A., and R. Callon,
               "Multiprotocol Label Switching Architecture",
               RFC 3031, DOI 10.17487/RFC3031, January 2001.

[RFC4033]      Arends, R., Austein, R., Larson, M., Massey, D., and
               S. Rose, "DNS Security Introduction and Requirements",
               RFC 4033, DOI 10.17487/RFC4033, March 2005.

[RFC4034]      Arends, R., Austein, R., Larson, M., Massey, D., and
               S. Rose, "Resource Records for the DNS Security
               Extensions", RFC 4034, DOI 10.17487/RFC4034,
               March 2005.

[RFC4035]      Arends, R., Austein, R., Larson, M., Massey, D., and
               S. Rose, "Protocol Modifications for the DNS Security
               Extensions", RFC 4035, DOI 10.17487/RFC4035,
               March 2005.

[RFC4271]      Rekhter, Y., Ed., Li, T., Ed., and S. Hares, Ed.,
               "A Border Gateway Protocol 4 (BGP-4)", RFC 4271,
               DOI 10.17487/RFC4271, January 2006.

[RFC4456]      Bates, T., Chen, E., and R. Chandra, "BGP Route
               Reflection: An Alternative to Full Mesh Internal BGP
               (iBGP)", RFC 4456, DOI 10.17487/RFC4456, April 2006.

[RFC7322]      Flanagan, H., Ed. and S. Gingerich, Ed., "RFC Style
               Guide", RFC 7322, DOI 10.17487/RFC7322, September 2014.

[RFC7348]      Mahalingam, M., Dutt, D., Duda, K., Agarwal, P.,
               Kreeger, L., Sridhar, T., Bursell, M., and C. Wright,
               "Virtual eXtensible Local Area Network (VXLAN): A
               Framework for Overlaying Virtualized Layer 2 Networks
               over Layer 3 Networks", RFC 7348,
               DOI 10.17487/RFC7348, August 2014.

[RFC7432]      Sajassi, A., Ed., Aggarwal, R., Bitar, N., Isaac, A.,
               Przygienda, T., Drake, J., and W. Henderickx,
               "BGP MPLS-Based Ethernet VPN", RFC 7432,
               DOI 10.17487/RFC7432, February 2015.

[RFC7841]      Halpern, J., Ed., Daigle, L., Ed., and O. Kolkman,
               Ed., "RFC Streams, Headers, and Boilerplate",
               RFC 7841, DOI 10.17487/RFC7841, May 2016.

[RFC8126]      Cotton, M., Leiba, B., and T. Narten, "Guidelines for
               Writing IANA Considerations Sections in RFCs",
               BCP 26, RFC 8126, DOI 10.17487/RFC8126, June 2017.

[RFC8174]      Leiba, B., "Ambiguity of Uppercase vs Lowercase in
               RFC 2119 Key Words", BCP 14, RFC 8174,
               DOI 10.17487/RFC8174, May 2017.

[RFC8200]      Deering, S. and R. Hinden, "Internet Protocol, Version
               6 (IPv6) Specification", STD 86, RFC 8200,
               DOI 10.17487/RFC8200, July 2017.

[RFC8365]      Sajassi, A., Ed., Drake, J., Ed., Bitar, N., Knight,
               S., and W. Henderickx, "A YANG Data Model for
               Ethernet VPN (EVPN)", RFC 8365,
               DOI 10.17487/RFC8365, March 2018.

[RFC8446]      Rescorla, E., "The Transport Layer Security (TLS)
               Protocol Version 1.3", RFC 8446,
               DOI 10.17487/RFC8446, August 2018.

[RFC8729]      Farrel, A., Ed. and P. Hoffman, Ed., "The RFC Series
               and RFC Editor", RFC 8729, DOI 10.17487/RFC8729,
               February 2020.

[RFC8949]      Bormann, C. and P. Hoffman, "Concise Binary Object
               Representation (CBOR)", STD 94, RFC 8949,
               DOI 10.17487/RFC8949, December 2020.

[RFC9000]      Iyengar, J., Ed. and M. Thomson, Ed., "QUIC: A
               UDP-Based Multiplexed and Secure Transport",
               RFC 9000, DOI 10.17487/RFC9000, May 2021.

[RFC9001]      Thomson, M., Ed. and S. Turner, Ed., "Using TLS 1.3
               with QUIC", RFC 9001, DOI 10.17487/RFC9001, May 2021.

[RFC9002]      Iyengar, J., Ed. and I. Swett, Ed., "QUIC Loss
               Detection and Congestion Control", RFC 9002,
               DOI 10.17487/RFC9002, May 2021.

[RFC9114]      Bishop, M., Ed., "HTTP/3", RFC 9114,
               DOI 10.17487/RFC9114, June 2022.

[RFC9135]      Brissette, P., Sajassi, A., and D. Smith, "BGP EVPN
               Integrated Routing and Bridging (IRB)", RFC 9135,
               DOI 10.17487/RFC9135, October 2021.

[RFC9136]      Rekhter, Y., Ed., Rosen, E., Ed., Uttaro, J., Drake,
               J., Fragassi, G., and W. Lin, "IP Prefix Advertisement
               in BGP EVPN", RFC 9136, DOI 10.17487/RFC9136,
               October 2021.

[RFC9204]      Krasic, C., Tyson, G., Campos, R., Rashid, Z., and M.
               Bishop, "QPACK: Header Compression for HTTP/3", RFC 9204,
               DOI 10.17487/RFC9204, June 2022.

[RFC9669]      "BPF Instruction Set Architecture (ISA)",
               RFC 9669, DOI 10.17487/RFC9669,
               August 2024.
```

---

## 5. RFC Evolution Chains

Historical evolution of key standards referenced in Unheaded specifications:

### 5.1 IPv6 Evolution

```
RFC 1883 (IP Version 6 Specification)
    ↓ obsoleted by ↓
RFC 2460 (Internet Protocol, Version 6 Specification)
    ↓ obsoleted by ↓
RFC 8200 (Internet Protocol, Version 6 Specification)
    [CURRENT - Normative Reference for Monad]
```

**Rationale:** RFC 8200 supersedes previous IPv6 specifications and includes clarifications on extension header processing essential for Monad HbH implementation.

### 5.2 BGP Evolution

```
RFC 1105 (Border Gateway Protocol)
    ↓ obsoleted by ↓
RFC 1163 (Border Gateway Protocol (BGP))
    ↓ obsoleted by ↓
RFC 1267 (Border Gateway Protocol 3 (BGP-3))
    ↓ obsoleted by ↓
RFC 1654 (BGP Protocol Analysis)
    ↓ obsoleted by ↓
RFC 1771 (A Border Gateway Protocol 4 (BGP-4))
    ↓ obsoleted by ↓
RFC 4271 (A Border Gateway Protocol 4 (BGP-4))
    [CURRENT - Normative Reference for Kingdom Mode]
```

**Rationale:** RFC 4271 is the authoritative BGP-4 specification. Earlier RFCs are obsolete and should not be referenced in new Unheaded specifications.

### 5.3 OSPF Evolution

```
RFC 1131 (The OSPF Specification)
    ↓ obsoleted by ↓
RFC 1247 (OSPF Version 2 Management Information Base)
    ↓ obsoleted by ↓
RFC 1583 (OSPF Version 2)
    ↓ obsoleted by ↓
RFC 2178 (OSPF Version 2)
    ↓ obsoleted by ↓
RFC 2328 (OSPF Version 2)
    [CURRENT - Informative Reference for Alternative Routing]
```

**Rationale:** RFC 2328 is the current OSPF-v2 specification. Used informatively for comparison with BGP approaches.

### 5.4 QUIC Evolution

```
RFC 9000 (QUIC: A UDP-Based Multiplexed and Secure Transport)
    [FIRST PUBLICATION - Informative Reference for Modern Transport]

    Related: RFC 9001 (Using TLS 1.3 with QUIC)
    Related: RFC 9002 (QUIC Loss Detection and Congestion Control)
```

**Rationale:** RFC 9000 is the baseline QUIC specification. No predecessors. Used informatively for inter-fabric communication optimization.

### 5.5 eBPF/BPF Evolution

```
RFC 9669 (BPF Instruction Set Architecture (ISA))
    [CURRENT - Normative Reference for Sophia/Shim eBPF Programs]

    Note: RFC 9669 defines the BPF ISA
          in IANA registries and external documentation.
```

**Rationale:** RFC 9669 is the authoritative reference for eBPF instruction set used in Unheaded packet processing programs.

### 5.6 TLS Evolution

```
RFC 2246 (TLS Protocol Version 1.0)
    ↓ obsoleted by ↓
RFC 3261 (TLS Protocol Version 1.1)
    ↓ obsoleted by ↓
RFC 5246 (The Transport Layer Security (TLS) Protocol Version 1.2)
    ↓ obsoleted by ↓
RFC 8446 (The Transport Layer Security (TLS) Protocol Version 1.3)
    [CURRENT - Normative Reference for Shield and Inter-Fabric Security]
```

**Rationale:** RFC 8446 (TLS 1.3) is the current standard. TLS 1.2 (RFC 5246) is acceptable for legacy deployments only.

---

## 6. Usage Guidelines

### 6.1 When to Cite as Normative

Cite an RFC as normative when:
- The specification defines required structures or procedures that Unheaded components must implement
- Interoperability depends on conformance to the RFC
- The RFC defines protocol constants, algorithms, or state machines
- The RFC establishes security requirements

### 6.2 When to Cite as Informative

Cite an RFC as informative when:
- The specification provides background or context
- The RFC describes a related but optional feature
- The RFC provides implementation guidance or best practices
- The RFC is included for historical context

### 6.3 Citation Format in Unheaded RFCs

Use the following format in reference sections:

```
1. Normative References

   [RFC2119]   Bradner, S., "Key words for use in RFCs to Indicate
               Requirement Levels", BCP 14, RFC 2119, March 1997.

   [RFC8200]   Deering, S. and R. Hinden, "Internet Protocol, Version
               6 (IPv6) Specification", STD 86, RFC 8200, July 2017.

   [RFC8949]   Bormann, C. and P. Hoffman, "Concise Binary Object
               Representation (CBOR)", STD 94, RFC 8949,
               December 2020.

2. Informative References

   [RFC7348]   Mahalingam, M., et al., "Virtual eXtensible Local Area
               Network (VXLAN): A Framework for Overlaying
               Virtualized Layer 2 Networks over Layer 3 Networks",
               RFC 7348, August 2014.
```

---

## 7. Maintenance and Updates

This cross-reference matrix should be reviewed and updated when:
- New Unheaded protocol components are added
- New RFC versions obsolete existing references
- RFC errata or clarifications affect Unheaded specifications
- Unheaded implementations reveal new RFC relevance

**Last Updated:** 2026-02-20
**Maintained by:** Unheaded RFC Editor

---

## Appendix A: Quick Reference Tables

### A.1 RFC Index by Number

| RFC | Title | Category |
|-----|-------|----------|
| RFC 1071 | Internet Checksum | Checksums |
| RFC 2119 | Key Words (BCP 14) | Process |
| RFC 2328 | OSPF Version 2 | Routing |
| RFC 3031 | MPLS Architecture | Routing |
| RFC 4033 | DNSSEC Intro | Security |
| RFC 4034 | DNSSEC RRs | Security |
| RFC 4035 | DNSSEC Protocol | Security |
| RFC 4271 | BGP-4 | Routing |
| RFC 4456 | BGP Route Reflection | Routing |
| RFC 7322 | RFC Style Guide | Process |
| RFC 7348 | VXLAN | Overlay |
| RFC 7432 | BGP EVPN | EVPN |
| RFC 7841 | RFC Streams/Boilerplate | Process |
| RFC 8126 | IANA Considerations | Process |
| RFC 8174 | RFC 2119 Clarification | Process |
| RFC 8200 | IPv6 | Protocols |
| RFC 8365 | EVPN-VXLAN | EVPN |
| RFC 8446 | TLS 1.3 | Security |
| RFC 8729 | RFC Series | Process |
| RFC 8949 | CBOR | Serialization |
| RFC 9000 | QUIC | Transport |
| RFC 9001 | QUIC-TLS | Transport |
| RFC 9002 | QUIC Loss Detection | Transport |
| RFC 9114 | HTTP/3 | Application |
| RFC 9135 | EVPN IRB | EVPN |
| RFC 9136 | EVPN IP Prefix | EVPN |
| RFC 9204 | QPACK | Application |
| RFC 9669 | BPF ISA | Processing |

### A.2 Component Quick-Link Reference

| Component | Primary RFC | Category |
|-----------|-------------|----------|
| Monad | RFC 8200 | IPv6 HbH |
| Sophia | RFC 8949 | CBOR |
| Shim | RFC 9669 | eBPF |
| Wotan | RFC 9669 | Memory |
| Shield | RFC 8446 | TLS 1.3 |
| Anamnesis | RFC 8949 | State |
| Kingdom Mode | RFC 4271 | BGP |
| VXLAN Fabric | RFC 7348 | Overlay |
| EVPN Control | RFC 7432 | EVPN |
| IRB | RFC 9135 | EVPN |
| IP Prefixes | RFC 9136 | EVPN |

