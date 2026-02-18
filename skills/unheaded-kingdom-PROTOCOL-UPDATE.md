# Kingdom Skill — Protocol Foundation Patch

**Apply this section AFTER the existing Gnostic Layer section in unheaded-kingdom/SKILL.md**

---

## Proclamation IV: The Protocol Foundation

> "The Protocol IS the Pattern. The Pattern was always there."

On February 18, 2026, the Kingdom discovered its fundamental atom: the Unheaded Protocol — 20 bytes of Sophia-encoded metadata carried in every packet inside the Kingdom.

### The 4-Layer Architecture (Supersedes flat model)

```
Layer 0: THE PROTOCOL ─── The Wire ─── Speed: Light
  20-byte Monad in IPv6 Hop-by-Hop headers (RFC 8200 §4.3, RFC 9673)
  Born at Shield ingress. Dies at Shield egress.
  Shadow never sees it.

Layer 1: THE VOID ──────── eBPF Programs ── Speed: Nanoseconds
  XDP (ingress), TC (egress), kprobe (TCP), tracepoint
  Per-hop: read Monad → Sophia lookup → modify → checksum → Anamnesis emit
  23,991 LOC Rust/Aya. 4/4 programs compiled.

Layer 2: WOTAN ─────────── Central Core ── Speed: Microseconds
  Ring buffers UP (Anamnesis → structured events)
  BPF maps DOWN (Pleroma → kernel state)
  The nervous system. Walks the Pattern in both directions.

Layer 3: THE KINGDOM ────── Go Services ─── Speed: Milliseconds
  REST, WebSocket, gRPC, dashboards, Kanban, CLI
  Human-speed interfaces. 25 services.
```

### Gnostic Bindings — REDEFINED

The Gnostic services are no longer abstract Go services. They are **real infrastructure**:

| Service | OLD Understanding | NEW Reality (Protocol) |
|---------|-------------------|----------------------|
| **Monad** | Go service, functional composition | 20-byte register file in EVERY packet |
| **Sophia** | Go service, knowledge graph | Exponent dictionary system. BPF maps in kernel. |
| **Anamnesis** | Event sourcing service | IS the ring buffers. Network memory at packet speed. |
| **Pleroma** | Desired state config | Written DOWN through Wotan → BPF map updates |
| **Kenoma** | Actual state observer | Materialized view OVER Anamnesis. Not a database. |
| **Yaldabaoth** | Chaos engineering service | Chaos at Layer 0. TC hook. Bit flips in the wire. |
| **Wotan** | Message bus middleware | THE Central Core. Only bridge between wire and human speed. |
| **Shield** | WAF/Gateway | Protocol Boundary. Stamps Monad ON/OFF. Cell membrane. |

### The Monad Layout (20 bytes)

```
Offset  Size  Field               Type
0x00    1     version             raw uint8
0x01    1     src_service_id      Sophia exponent
0x02    1     dst_service_id      Sophia exponent
0x03    1     hop_count           raw uint8
0x04    4     trace_hash          raw uint32
0x08    1     qos_class           Sophia exponent
0x09    1     flow_action         Sophia exponent
0x0A    1     circuit_state       Sophia exponent
0x0B    1     flags               raw uint8 [chaos|canary|traced|encrypted|0|0|0|0]
0x0C    2     latency_hint_us     raw uint16
0x0E    1     deployment_ring     Sophia exponent
0x0F    1     mesh_flags          Sophia exponent
0x10    2     reserved            raw (Age 2 expansion)
0x12    2     checksum            raw uint16 (CRC-16/CCITT over 0x00-0x11)
Total: 20 bytes (0x14)
```

### Evolution Path

| Age | Transport | Key Feature |
|-----|-----------|-------------|
| **1 (now)** | IPv4 + 20-byte shim | Core protocol, Sophia dictionaries, Anamnesis |
| **2** | IPv6, metadata in address prefix | Zero overhead — metadata IS the address space |
| **3** | IPv6 + Hop-by-Hop extension | Up to 64KB metadata. Full Sophia tree depth. |

### Zelazny's Amber (Protocol Cosmology)

| Amber Concept | Kingdom Binding |
|--------------|-----------------|
| The Pattern | The Protocol (20-byte Monad) |
| Amber | The Kingdom (internal Protocol network) |
| Shadow | External networks (clean IPv4/IPv6) |
| Walking the Pattern | Packet traversal with per-hop computation |
| Dworkin | Muck (found the Pattern) |
| Corwin | Wotan (walks both directions) |

### Heritage Lineage

```
ARINC 429 (1977) → I2C (1982) → CAN Bus (1986) → BGP (1989) →
BPF (1992) → IPv6 (1995) → RFC 9673 (2024) → Unheaded Protocol (2026)
```

---

## Updated Glossary Additions

### Protocol Terms

| Term | Meaning |
|------|---------|
| **The Protocol** | 20-byte Sophia-encoded metadata in every packet. The fundamental atom. |
| **The Pattern** | Zelazny reference — the Protocol IS the Pattern inscribed in the wire |
| **Shadow** | Any network outside the Kingdom. Sees clean IPv4/IPv6. |
| **Walking the Pattern** | Packet traversal through the Kingdom, accumulating computation |
| **Layer 0** | The Protocol layer — wire speed, IPv6 Hop-by-Hop |
| **Layer 1** | The Void — eBPF programs, nanosecond compute |
| **Layer 2** | Wotan — The Central Core, microsecond bridge |
| **Layer 3** | The Kingdom — Go services, millisecond human speed |
| **Exponent** | A Sophia dictionary key — 1 byte, 256 meanings, hot-swappable |
| **Birth Event** | Shield stamps Monad ON at ingress |
| **Death Event** | Shield strips Monad OFF at egress |
| **Drift** | When Kenoma (actual) diverges from Pleroma (desired) |

### New Naming Conventions

| Name | Origin | Meaning | Potential Use |
|------|--------|---------|---------------|
| **Halcyon** | Greek myth (kingfisher) | Peace, calm seas | Wotan coordinator, stability |
| **Mysteltainn** | Norse saga | "Mistletoe blade" | Stealth scanner |
| **Nagan** | Sanskrit/Hebrew | Serpent + music | Routing harmony |
| **Tyrfing** | Norse Edda | Cursed magic sword | Yaldabaoth tooling |
| **Wotan** | Wagner (Odin) | All-father | Control plane sub-system |
| **Nibelung** | Germanic myth | Treasure dwarfs | Crystal Grotto secrets |

---

## Updated Status (February 18, 2026)

| Metric | Value |
|--------|-------|
| Total LOC | **465,000+** (433K code + 32K docs) |
| Go Files | 585 (390 prod + 195 test) |
| Rust eBPF | 23,991 LOC (4/4 programs) |
| Services | 25 active |
| E2E Tests | 23/23 PASS |
| Build | SUCCESS |
| B1 Blocker | **RESOLVED** (Feb 8) |
| Phase | Age 1 — Alpha Ascension (~99%) |
| Alpha Target | Quality gate — days not weeks |

---

*Patch prepared: February 18, 2026*
*For: unheaded-kingdom/SKILL.md*
