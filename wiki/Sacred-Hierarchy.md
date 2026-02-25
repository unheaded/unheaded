# Sacred Hierarchy — Component Architecture

Full Unheaded component hierarchy mapping lore names to technical layers.

## Layer Mapping

| Layer | Technical Name | Lore Name |
|-------|---------------|-----------|
| 5 | User Interface | The Crown (dashboard, kanban) |
| 4 | Application Services | Vision & Execution layers |
| 3 | Infrastructure Services | Fae Chamber (Wotan), Protocol Layer |
| 2 | Control Plane | Cuirass (unheaded-daemon) |
| 1 | Data Plane | The Whispering Void (eBPF) |
| 0 | Infrastructure | The Sabatons (host OS, EVPN-VXLAN) |

## Arcane Hollows (Domain Groupings)

| Hollow | Domain | Key Components |
|--------|--------|---------------|
| **Crown Hollow** | Leadership & strategy | Captain, Timeguru |
| **Forge Hollow** | Infrastructure & execution | Architect, Micromanager, IaC renderers |
| **Gnostic Hollow** | State management | Pleroma, Kenoma, Anamnesis |
| **Fae Hollow** | Messaging & communication | Wotan, topic contracts |
| **Protocol Hollow** | Wire format & dictionaries | Monad, Sophia, Shield, Shim |
| **Void Hollow** | eBPF data plane | All eBPF programs |
| **Sabaton Hollow** | Host infrastructure | OS, network fabric, BGP |

## Adversarial Layer

Yaldabaoth (chaos injection) orbits outside the hierarchy. It injects faults at any layer to test resilience — the adversary is not part of the system, it is the force that tests it.

---

> **Source:** [docs/lore/SACRED_HIERARCHY.md](../docs/lore/SACRED_HIERARCHY.md)
