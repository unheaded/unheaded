# draft-bellis-unheaded-protocol-foundation-06

**Status:** IETF Experimental · Independent Submission · March 2026

The Unheaded Protocol defines a mapped data bus model that transforms IPv6 packets into addressable memory by encoding a small register file directly in the IPv6 Hop-by-Hop Options extension header. A 20-byte Monad (register file) carries program state through the network; at each hop, a BPF program (the Shim) performs computation on the Monad. The packet itself becomes the working memory, using exponent-encoded fields to pack rich metadata into the IPv6 option while remaining fully backward-compatible with existing networks. This memo also specifies Kingdom Mode Address Reclamation and Post-Quantum Cryptographic Identity Binding.

## Key Sections

- **Introduction** — Problem statement (computation vs. data separation), scope (Limited Domains only), cross-references to Sophia and Wotan companion specs
- **Terminology** — Monad, Shim, Shield, Wotan, Anamnesis, Sophia, Exponent Encoding, Limited Domain, Kingdom Mode
- **Protocol Overview** — Per-hop processing model: Shield stamps at ingress, Shim runs at each hop, Shield strips at egress
- **Packet Format** — IPv6 Hop-by-Hop extension header layout; 20-byte Monad register file (WIRE FORMAT FROZEN at v0x01); Extended Register Option
- **Monad Register File** — Full field table: version, src/dst_service_id, hop_count, qos_class, flow_action, circuit_state, flags, latency_hint, deploy_ring, mesh_flags, src/dst_prefix_lo, scratch[0-3], checksum
- **Flags Bitfield** — CHAOS, CANARY, TRACED, ENCRYPT, SAMPLED, MIRROR, CUSTOM, RSVD
- **Checksum Field** — CRC-16/CCITT-FALSE over all 20 bytes; computation and verification procedures
- **Exponent Encoding** — Compositional scheme: decoded = base^exponent × multiplier; Sophia dictionary system; concrete examples
- **Sophia Dictionary System (Extended)** — Tree architecture; BPF map implementation; minimum required dictionary entries
- **IANA Registration Procedures** — Step-by-step guide for the 12 registries; UNHEADED_METRIC_V1 example registration (Type 0x2A)
- **Security Considerations** — Wire format immutability threat model; parser divergence attacks; BPF containment; integrity; trust boundary; PQC and Kingdom Mode threat models
- **Backwards Compatibility Statement (draft-05 → draft-06)** — Wire format unchanged; registry additions additive; interoperability confirmed
- **IANA Considerations** — 12 registries: Monad Version Numbers, Monad Flags, Flow Actions, Kingdom Mode Values, IPv6 HbH Option Type, Next Header Type, Sophia Dictionary Namespace, Anamnesis Event Types, Error Codes, TLV Types, PQC Algorithm Identifiers, Wotan Topic Namespace; UPC MBC Opcode/Syscall/Memory/Event registries; PQC Authentication Value Format; UPCFlat Binary Format; UNFS Filesystem

## Related

- [[Sophia Dictionaries|Sophia-Dictionaries]]
- [[Wotan Memory Model|Wotan-Memory-Model]]
- [[MBC ISA Reference|MBC-ISA-Reference]]
- [[PQC Authentication|PQC-Authentication]]
- [[Protocol Foundation|Protocol-Foundation]]
- [[Drafts Index|Drafts-Index]]

---

> **Source:** [docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md)
