# Kingdom Lore — Naming Conventions & Mythology

This directory documents the relationship between Unheaded's internal naming
conventions (drawn from Gnostic cosmology, medieval armory, Norse mythology, and the
contemplative traditions)
and the technical components they represent.

The lore exists for three reasons: it makes the codebase memorable, it maps
cleanly to architectural concepts, and it's fun. Public-facing documentation
uses technical language; these files are the Rosetta Stone between the two.

## Files

| File | Contents |
|------|----------|
| [NAMING_MAP.md](NAMING_MAP.md) | Complete name → component mapping table |
| [GNOSTIC_ARCHITECTURE.md](GNOSTIC_ARCHITECTURE.md) | Gnostic cosmology → state management architecture |
| [MEDIEVAL_ARMORY.md](MEDIEVAL_ARMORY.md) | Armor pieces → infrastructure components |
| [NORSE_MYTHOLOGY.md](NORSE_MYTHOLOGY.md) | Norse/Wagnerian → protocol and messaging |
| [SACRED_HIERARCHY.md](SACRED_HIERARCHY.md) | The full component hierarchy with ASCII art |
| [HERITAGE.md](HERITAGE.md) | Protocol lineage: ARINC 429 → Unheaded |
| [TOMB_OF_KNOWLEDGE.md](TOMB_OF_KNOWLEDGE.md) | The Tomb: LICH, Grimoire, Cerydwyn |
| [PHYLACTERY.md](PHYLACTERY.md) | Encrypted state persistence |
| [binary-lore-names.md](binary-lore-names.md) | Binary aliases (S41) — see NAMING_MAP → Naming Debt for collisions |

## Quick Reference

| Lore Name | Technical Component |
|-----------|-------------------|
| The Crown | Project leadership (Muck) |
| Captain | Strategy/vision service |
| Timeguru | Timeline tracking service |
| Architect | Infrastructure design service |
| Micromanager | Execution/QA service |
| Wotan | Message bus (gRPC, ring buffer, BPF map substrate) |
| Sophia | BPF dictionary service (exponent-encoded maps) |
| Monad | 20-byte register file in IPv6 HbH Options |
| Anamnesis | Event sourcing / audit trail service |
| Shield | Ingress/egress boundary (WAF, Monad stamp/strip) |
| Shim | Per-hop eBPF program that reads/writes the Monad |
| Pleroma | Desired state (configuration truth) |
| Kenoma | Actual state (observed reality, drift detection) |
| Yaldabaoth | Chaos injection / adversarial testing |
| Phylactery | Encrypted state persistence layer |
| The Whispering Void | eBPF data plane (XDP/TC/kprobe programs) |
| Dark Grimoire | Attack surface taxonomy |
| LICH | Lethal Infrastructure Chaos Hunter (fuzzing framework) |
| Fae Chamber | Service interface contracts (Wotan topics) |
| Kingdom Mode | Extended register space via EVPN-VXLAN address reclamation |
| Hauberk | Service mesh (circuit breakers, mTLS) |
| Cuirass | Control plane (unheaded-daemon) |
| Pauldrons | Load balancer |
| Vambraces | Observability layer |
| Sabatons | Host OS / bare metal |
| Gauntlets | CLI tooling |
| Huginn | Host metrics agent (native on every host, ADR-084) |
| Muninn | Observability fan-out (reserved; superseded by Vector) |
| Kvasir | IPFIX / NetFlow collector (planned, ADR-087) |
| Heimdall / Gjallarhorn / Mjölnir / Gungnir / Mímir | Mímir's Law: UPC baseline delivery + drift detection (ADR-043) |
| Enkrateia | Alerts-only drift aggregator (restoration verb) |
| Akira | Health consensus monitor (ADR-029) |
| The Well | PostgreSQL 16 — app / ops / config databases |
| Zhenai / Champion | Local LLM agent; Seer → Champion ascension (ADR-019, -104, -106) |
| Cerydwyn | Blue-team triage LLM over the Grimoire (counter to LICH) |
| Steel Echo | Game-day workload for Sword + Yaldabaoth (S-REAL-STEEL) |
| Cartridge | Signed MBC network function run per flow on the UPC (ADR-108) |
| EVOLUTION-1 / Rung | Waking the owned xv6 kernel; one gated flip per rung (ADR-109/110) |
| Unheaded Linux / unheadnix | Own minimal OS on the UPC (ADR-081) / ephemeral single-.mbc node (ADR-082) |
| Sleipnir / Yggdrasil / Gleipnir | BGP daemon / hardened OS image + evidence seals / config convergence (Age 2) |
| WEST / EAST | The two bare-metal Kingdom hosts |

See individual files for the full mythology and rationale behind each name.
Dated additions since 2026-05-29 are in NAMING_MAP.md → Addendum.

Last reconciled with `develop`: 2026-10-28 (`1db358a0`).
