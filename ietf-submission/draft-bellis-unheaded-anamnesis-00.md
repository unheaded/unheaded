---
title: "Anamnesis: Event Records for the Unheaded Protocol"
abbrev: "Unheaded Anamnesis"
docname: draft-bellis-unheaded-anamnesis-00
category: exp
ipr: trust200902
submissionType: independent
area: Internet
workgroup: Independent Submission
date: 2026-10-27
stand_alone: yes
v: 3
keyword:
  - Unheaded Protocol
  - Anamnesis
  - event log
  - BPF ring buffer
  - tracing

author:
  - ins: S. Bellis
    name: Stevie Bellis
    org: Unheaded
    email: stevie@bellis.tech
    country: US

normative:
  RFC2119:
  RFC8174:
  RFC8200:
  RFC6437:
  UNHEADED-FOUNDATION:
    title: "Unheaded: Protocol Foundation"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-protocol-foundation-00

informative:
  RFC9669:
  UNHEADED-WOTAN:
    title: "Wotan Memory Protocol for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-03
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-wotan-memory-00
  UNHEADED-ADR108:
    title: "ADR-108: MBC Cartridges"
    author:
      - ins: S. Bellis
    date: 2026-10
    target: "https://github.com/unheaded/unheaded/blob/develop/docs/adr/ADR-108-mbc-cartridges.md"

--- abstract

This document specifies Anamnesis, the event log of the Unheaded
Protocol.  BPF programs that process packets carrying a Monad register
write fixed 32-octet event records into ring buffers on the host where
they run, without blocking the packet path.  The document defines the
record format, the rules for producers and consumers, and how records
are correlated into a trace.  Event Type values are registered by the
Unheaded Protocol Foundation; this document creates no registry.

--- middle

# Introduction

The Unheaded Protocol Foundation {{UNHEADED-FOUNDATION}} carries a
20-octet Monad register in an IPv6 Hop-by-Hop option {{RFC8200}}.  The
programs that insert, process and strip that register run as BPF
programs {{RFC9669}} attached to the network path.  Anamnesis is how
they report what they did: each program writes an event record when a
packet is born at a Shield, crosses a hop, is changed by the chaos
subsystem, fails a check, or leaves the Kingdom.

Anamnesis has three properties that the rest of this document
preserves:

1. It never slows the packet.  A producer that cannot write a record
   drops the record, not the packet, and never waits.

2. It carries metadata only.  A record holds a timestamp, an Event
   Type, a hop identifier, part of the flow label and a copy of the
   Monad register.  It never holds packet payload.

3. It is local.  Records stay on the host that produced them.  A
   host agent may forward what it reads (for example to Wotan
   {{UNHEADED-WOTAN}}), but the 32-octet record is not a wire format
   and is never placed in a packet.

## Requirements Language

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and
"OPTIONAL" in this document are to be interpreted as described in
BCP 14 {{RFC2119}} {{RFC8174}} when, and only when, they appear in all
capitals, as shown here.

## Terminology

Producer:
: A BPF program that writes event records.

Ring:
: A BPF ring buffer (BPF_MAP_TYPE_RINGBUF) owned by one producer
  program, into which it writes event records.

Consumer:
: A host process that reads records from one or more rings.

Event record:
: The fixed 32-octet structure defined in {{record}}.

# Event Record Format {#record}

An event record is 32 octets:

~~~~
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
+                     Timestamp (64 bits)                       +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Event Type   |    Hop ID     |   Flow Label (low 16 bits)    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
+                                                               +
|                                                               |
+                 Monad register snapshot (20 octets)           +
|                                                               |
+                                                               +
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
~~~~

Timestamp (octets 0-7):
: Nanoseconds from the producing host's monotonic clock at the time of
  the event, in the producing host's native byte order.  Timestamps
  from different hosts are not comparable.

Event Type (octet 8):
: A value from the "Unheaded Anamnesis Event Types" registry
  {{UNHEADED-FOUNDATION}}.

Hop ID (octet 9):
: The identifier of the hop that emitted the record, as configured on
  that hop.

Flow Label (octets 10-11):
: The low 16 bits of the packet's IPv6 Flow Label {{RFC6437}}, in
  network byte order.

Monad register snapshot (octets 12-31):
: The 20-octet Monad register as it stood when the event occurred, in
  the octet order of the wire format {{UNHEADED-FOUNDATION}}.

A producer MUST write all 32 octets of every record it commits.  A
record has no padding and no field that is left unwritten.

# Event Types

The Event Type octet takes its value from the "Unheaded Anamnesis
Event Types" registry created by {{UNHEADED-FOUNDATION}}.  This
document does not repeat that registry and assigns no value.

Some producers emit EVENT_COMPUTED records larger than 32 octets, for
example the cartridge trace records of {{UNHEADED-ADR108}}.  Those
records begin with the Event Type octet followed by a Sub-kind octet,
are carried on a stream separate from the rings of this document, and
are governed by the "EVENT_COMPUTED Sub-kinds" rules and registry in
{{UNHEADED-FOUNDATION}}.  A consumer of 32-octet records MUST NOT
interpret any octet of such a record as a Sub-kind.

# Producer Rules

A producer:

1. MUST NOT block, retry or loop while reserving space in a ring.  If
   the reservation fails, the producer MUST drop the record and SHOULD
   increment a drop counter that a consumer can read.

2. MUST NOT copy packet payload octets into a record or into any other
   field it emits to Anamnesis.

3. MUST emit a record at every hop for a packet whose Monad TRACED
   flag is set {{UNHEADED-FOUNDATION}}, subject to rule 1.

4. SHOULD size its ring from the expected event rate, the record size
   and the longest interval a consumer may fall behind, so that drops
   are rare in normal operation.

# Consumer Rules

A consumer:

1. MUST discard a record that is not exactly 32 octets long.

2. MUST count and discard a record whose Event Type it does not
   recognize, and MUST continue reading the ring.  An unknown Event
   Type is not an error of the ring.

3. MUST NOT assume that the Monad snapshot carries a valid checksum:
   an EVENT_ANOMALY record may report exactly the packet whose
   checksum failed.

4. SHOULD expose the drop counters of the producers it reads, so that
   a gap in a trace can be told apart from a quiet flow.

# Correlating Records into a Trace

Records that belong to the same packet's journey share the trace ID in
their Monad snapshots {{UNHEADED-FOUNDATION}}; the Flow Label bits
narrow the search when trace IDs are reused.  Within one host,
timestamps order the records.  Across hosts, ordering comes from the
hop sequence (birth, hops, death) and from the Monad hop count, not
from timestamps.

# Security Considerations

Anamnesis records carry flow metadata.  The Monad snapshot holds
service identifiers, a trace ID, QoS and flow-action values and flags;
the Flow Label bits identify a flow.  No record carries payload
(producer rule 2), and no field holds a key or a credential.  Access to
the rings MUST be limited to the host agents that need them, since a
reader learns which services talk to which and when.

A producer that overflows its ring loses records, not packets.  An
attacker who floods a hop can therefore blind tracing for that hop but
cannot use Anamnesis to stall the data path.  Consumers SHOULD alert on
drop counters for this reason.

Records are written by BPF programs and read by host processes.  A
consumer MUST treat every field as untrusted input: a record may
describe a malformed packet, and the Event Type may be one the
consumer does not know (consumer rule 2).

Producer rule 2 and the requirement to write all 32 octets together
keep kernel memory and packet payload out of the log.

# IANA Considerations

This document has no IANA actions.  The registries it uses (Anamnesis
Event Types and EVENT_COMPUTED Sub-kinds) are created by
{{UNHEADED-FOUNDATION}}.

--- back

# Acknowledgments
{:numbered="false"}

The record format follows the implementation in the Unheaded
repository (`ebpf/monad-common`, `pkg/protocol/bpfschema`), whose
size and offset tests fix it at 32 octets.
