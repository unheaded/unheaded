# Kingdom Lore — Naming Conventions & Mythology

This directory documents the relationship between Unheaded's internal naming
conventions (drawn from Gnostic cosmology, medieval armory, and Norse mythology)
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

See individual files for the full mythology and rationale behind each name.
