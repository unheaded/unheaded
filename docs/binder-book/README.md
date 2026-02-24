# The Unheaded Binder Book

**A graduated reading path from zero to formal specification.**

---

## Purpose

This binder book is a preamble to the formal RFC specifications for the
Unheaded Protocol Foundation (`draft-bellis-unheaded-protocol-foundation-01`).
Its goal is to make the ideas accessible to anyone, regardless of technical
background, and then progressively deepen the treatment until the reader is
ready for the formal protocol documents.

The binder book exists because protocol specifications are written for
implementors, not for understanding. You cannot learn *why* a system exists by
reading its wire format. These documents fill that gap.

---

## Reading Guide

Start at the level that matches your current understanding. Each level builds
on the one before it. If something in a higher level does not make sense, drop
back one level and re-read.

### Level 1: ELI5 (Explain Like I'm 5)

No jargon. No assumed knowledge. Analogies from the physical world.

| Document | What You Will Learn |
|----------|---------------------|
| [What Is Unheaded?](eli5/what-is-unheaded.md) | The post office analogy: what the platform does and why it matters, explained with stamps, envelopes, and address books. |
| [How Packets Work](eli5/how-packets-work.md) | What a digital envelope is, what goes inside it, how it gets where it needs to go, and what Unheaded adds (a tracking sticker). |

**After this level you will understand:** Unheaded watches and protects the
mail system that computers use to talk to each other, without reading the
letters.

### Level 2: Middle School

Technical concepts introduced gently. ASCII diagrams. Real terminology with
definitions inline.

| Document | What You Will Learn |
|----------|---------------------|
| [Networking Basics](middle-school/networking-basics.md) | IP addresses, ports, TCP and UDP, clients and servers, firewalls, and monitoring. Enough to understand how the internet actually works. |
| [What Unheaded Does](middle-school/what-unheaded-does.md) | The three pillars (networking, observability, security), what eBPF is and why it matters, the wire protocol concept, and why this approach is different. |

**After this level you will understand:** How computers communicate over
networks, what observability means, and how Unheaded fits into that picture.

### Level 3: High School

Full technical detail without formal mathematics. Protocol layers, data flow,
component roles.

| Document | What You Will Learn |
|----------|---------------------|
| [Architecture Overview](high-school/architecture-overview.md) | The full stack from XDP to dashboard. IPv6 Hop-by-Hop headers. BPF maps. 5-tuple correlation. Protocol layer diagrams. How all the pieces connect. |
| [Protocol Primer](high-school/protocol-primer.md) | The five protocol components: Monad (register file), Sophia (BPF map state), Wotan (memory persistence), Anamnesis (event streaming), and Shield (XDP filtering). How they compose into a system. |

**After this level you will understand:** How Unheaded's protocol works at a
component level, what each piece does, and how data flows from packet to
dashboard.

### Level 4: PhD

Formal treatment. Mathematical models. Competitive analysis. Security proofs.

| Document | What You Will Learn |
|----------|---------------------|
| [Formal Specification Introduction](phd/formal-specification-intro.md) | Stateful stream transducer model. Information-theoretic analysis. Comparison to Apache Flink checkpoint model. BPF verifier constraints as a formal safety proof. Wire format security analysis. |
| [Competitive Landscape](phd/competitive-landscape.md) | Where Unheaded fits in the ecosystem of 72 projects across 16 categories. Formal capability matrix. Why wire-level depth is the differentiator. |

**After this level you will understand:** The theoretical foundations of the
protocol, its formal safety properties, and how it compares to every relevant
system in the industry.

---

## How This Connects to the RFC

The formal specification (`draft-bellis-unheaded-protocol-foundation-01`)
defines:

- The Monad wire format (20 octets, IPv6 Hop-by-Hop Option Type 0x3E)
- The HopByHop Options Header layout (24 octets total)
- The Anamnesis event format (32 bytes per event)
- The Sophia dictionary encoding (exponent-encoded field values)
- Shield ingress/egress boundary semantics
- CRC-16/CCITT integrity verification
- Flow action, circuit state, and flag field semantics

The binder book explains *why* each of those choices was made. The RFC
explains *what* the choices are.

---

## Conventions

- All file sizes and byte counts refer to octets (8-bit bytes).
- Wire format offsets are given in hexadecimal (e.g., `0x03` for hop count).
- Network byte order means big-endian unless stated otherwise.
- "Kingdom" refers to the Unheaded-managed network domain. "Shadow" refers to
  the external network.
- Service names (Monad, Sophia, Wotan, Anamnesis, Shield) are protocol
  components, not marketing terms. Each maps to a specific crate, BPF program,
  or Go service in the codebase.

---

## Contributing

If you find a section unclear, the problem is with the document, not with you.
File an issue or submit a pull request. The whole point of graduated
documentation is that each level should be self-contained and comprehensible
to its target audience.

---

*"Self-hosting is proof, not marketing."*
