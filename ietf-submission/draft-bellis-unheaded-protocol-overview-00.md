---
title: "The Unheaded Protocol Suite: Overview and Reading Guide"
abbrev: "Unheaded Protocol Overview"
docname: draft-bellis-unheaded-protocol-overview-00
category: info
ipr: trust200902
submissionType: independent
area: Internet
workgroup: Independent Submission
date: 2026-10-27
stand_alone: yes
v: 3
keyword:
  - Unheaded Protocol
  - Monad
  - Sophia
  - Wotan
  - overview

author:
  - ins: S. Bellis
    name: Stevie Bellis
    org: Unheaded
    email: stevie@bellis.tech
    country: US

normative:

informative:
  RFC8200:
  RFC9669:
  UNHEADED-FOUNDATION:
    title: "Unheaded: Protocol Foundation"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-protocol-foundation-00
  UNHEADED-SOPHIA:
    title: "Sophia Dictionary Format for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-sophia-dictionary-00
  UNHEADED-WOTAN:
    title: "Wotan Memory Protocol for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-wotan-memory-00
  UNHEADED-MBC-ISA:
    title: "MBC Instruction Set Architecture for the Unheaded Protocol Computer"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-mbc-isa-00
  UNHEADED-SHIM:
    title: "Shim Pipeline Specification for the Unheaded Protocol Computer"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-shim-00
  UNHEADED-PQC:
    title: "Post-Quantum Packet Authentication for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-pqc-authentication-00
  UNHEADED-ANAMNESIS:
    title: "Anamnesis: Event Records for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-10
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-anamnesis-00

--- abstract

The Unheaded protocol suite is specified in seven Internet-Drafts: a
foundation that defines the Monad register carried in an IPv6
Hop-by-Hop option, and six companions for dictionaries, memory,
compute, execution, authentication and event records.  This document
is a reader's map of the suite.  It says what each document defines,
which IANA registries each one creates, how the pieces layer, and
which documents each kind of node implements.  It specifies nothing
itself.

--- middle

# Introduction

The suite grew one document at a time, and a reader who starts with
any single draft meets references to all the others.  This document
gives the order in which they fit together.  It adds no requirement:
where it summarizes another document, that document governs.

# Layering

~~~~
 +-------------------------------------------------------------+
 |  Anamnesis: event records from every program (local log)    |
 +-------------------------------------------------------------+
 |  Shim / UPC compute: MBC programs run per hop (MBC ISA)     |
 +-------------------------------------------------------------+
 |  Wotan: per-flow memory, WAL, topics                        |
 +-------------------------------------------------------------+
 |  Sophia: dictionaries that give Monad bytes their meaning   |
 +-------------------------------------------------------------+
 |  Monad: 20-octet register in an IPv6 Hop-by-Hop option     |
 +-------------------------------------------------------------+
 |  IPv6 [RFC8200]                                             |
 +-------------------------------------------------------------+
         PQC authentication spans the Monad, Sophia and
         Wotan layers at the Kingdom edge (Shield).
~~~~

The Monad register is the only part of the suite that travels in every
packet.  Its format is frozen at version 0x01: 20 octets, with the
field layout, the checksum and the flags octet fixed by
{{UNHEADED-FOUNDATION}}.  Every other layer adds meaning to those 20
octets (Sophia), state behind them (Wotan), computation over them
(Shim and MBC), authentication of them (PQC), or a record of them
(Anamnesis) without changing their layout.

# The Documents

## Protocol Foundation

{{UNHEADED-FOUNDATION}} defines the Monad register, its IPv6
Hop-by-Hop option {{RFC8200}}, exponent encoding, the checksum, per-hop processing
and the Kingdom trust boundary.  It creates the "Unheaded Protocol
Parameters" registry group and in it the registries for Monad protocol
versions, Monad flags, flow actions, Kingdom Mode values, the Sophia
dictionary namespace, Anamnesis Event Types and their EVENT_COMPUTED
Sub-kinds, error codes, TLV types, PQC algorithm identifiers, MBC
syscall numbers, UPC memory region types and UPC event types.  It
requests a Hop-by-Hop option type and a Next Header value.

## Sophia Dictionaries

{{UNHEADED-SOPHIA}} defines how dictionaries are serialized, stored in
BPF maps, nested and distributed.  It creates the registries for Sophia
root dictionary keys, sub-dictionary types and the QPACK static table.

## Wotan Memory

{{UNHEADED-WOTAN}} defines per-flow memory exposed to BPF programs, its
write-ahead log, the TTY subsystem and the structured error code.  It
creates the registries for the Wotan topic namespace, error origins and
error categories.

## MBC Instruction Set

{{UNHEADED-MBC-ISA}} defines the instruction set of the Unheaded
Protocol Computer, including the ISA v2 opcodes for translated RISC-V
code and the memory-mapped control and status registers.  It creates
the one registry of MBC opcodes.

## Shim Pipeline

{{UNHEADED-SHIM}} defines how MBC programs are assembled, verified,
loaded into BPF {{RFC9669}} maps and executed per hop, and the tick packets that
drive them.  It creates registries for pipeline stages, UPC BPF map
types and conformance (Dream Ladder) levels.

## PQC Authentication

{{UNHEADED-PQC}} defines post-quantum packet authentication at the
Kingdom edge: signatures held by reference in Sophia maps, the Monad
fields that point at them, and the verification pipeline at Shield.
Its IANA section also requests the PQC algorithm identifier registry
that the foundation names; the two lists are to be merged into one
before either is assigned.

## Anamnesis

{{UNHEADED-ANAMNESIS}} defines the 32-octet event record that programs
write to local ring buffers, and the rules for producers and
consumers.  It creates no registry; its Event Types live in the
foundation.

# What Each Node Implements

The suite names its node roles in {{UNHEADED-FOUNDATION}}.  This table
lists which documents each role draws on; the documents themselves say
which parts are mandatory.

| Role | Documents |
|---|---|
| Shield (Kingdom edge) | Foundation, Sophia, PQC Authentication, Anamnesis |
| Hop (any node that processes Monad in transit) | Foundation, Sophia, Anamnesis |
| Wotan node | Wotan Memory, Foundation (error codes), Sophia (dictionary distribution) |
| UPC compute node | MBC ISA, Shim, Wotan Memory, Anamnesis |
| Kingdom node (all roles) | All seven |

# Reading Order

A first reading takes the foundation, then Sophia, then Anamnesis.
Implementers of compute read the MBC ISA before the Shim.  Wotan and
PQC Authentication can be read in either order after Sophia.

# Security Considerations

This document defines no protocol behavior and adds no security
consideration of its own.  Each document in the suite has its own
Security Considerations section; the foundation's covers the Monad
register and the Kingdom trust boundary that the others build on.

# IANA Considerations

This document has no IANA actions.

--- back

# Acknowledgments
{:numbered="false"}

This overview was prepared alongside the RFC Editor review of the suite
in October 2026.
