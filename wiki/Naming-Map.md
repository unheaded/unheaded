# Naming Map — Lore to Technical Reference

Complete mapping of every internal name in the Unheaded codebase to its technical component.

## Three Naming Pillars

| Pillar | Tradition | Domain | Examples |
|--------|-----------|--------|---------|
| **Gnostic Cosmology** | Valentinian Gnosticism | State management | Pleroma, Kenoma, Anamnesis, Monad, Sophia |
| **Medieval Armory** | European arms & armor | Infrastructure layers | Cuirass, Hauberk, Shield, Vambraces |
| **Norse/Wagnerian** | Norse mythology, Wagner's Ring Cycle | Protocol & messaging | Wotan, Mysteltainn, Tyrfing |

## Quick Reference

| Lore Name | Technical Component | Origin |
|-----------|-------------------|--------|
| Monad | 20-byte register file (5×u32) in IPv6 HbH Options | Gnostic: indivisible unity |
| Sophia | BPF dictionary service (exponent-encoded maps) | Gnostic: divine wisdom |
| Wotan | Message bus (gRPC, ring buffer, BPF map substrate) | Norse: Odin/all-father |
| Pleroma | Desired state service | Gnostic: divine fullness |
| Kenoma | Actual state / drift detection | Gnostic: material void |
| Anamnesis | Event sourcing / audit trail | Gnostic: remembrance |
| Yaldabaoth | Chaos injection service | Gnostic: the Demiurge |
| Shield | WAF / ingress-egress boundary | Medieval: blocks attacks |
| Cuirass | Control plane (unheaded-daemon) | Medieval: chest armor |
| Hauberk | Service mesh (circuit breakers, mTLS) | Medieval: chain mail |
| Vambraces | Observability (eBPF layer) | Medieval: forearm armor |
| Phylactery | Encrypted state persistence | Greek/D&D: soul vessel |
| The Whispering Void | eBPF data plane | Original: silent kernel observation |
| LICH | Fuzzing framework | Lethal Infrastructure Chaos Hunter |
| Fae Chamber | Service interface contracts (Wotan topics) | Fairy court |
| Kingdom Mode | Extended register space via EVPN-VXLAN | Political entity |

---

> **Source:** [docs/lore/NAMING_MAP.md](../docs/lore/NAMING_MAP.md)
