# Medieval Armory — Infrastructure as Armor

Each infrastructure component maps to a piece of medieval plate armor. The customer's application is "the head" — Unheaded provides everything below the neck.

## Armor Pieces

| Armor Piece | Technical Component | Rationale |
|------------|-------------------|-----------|
| **Sabatons** | Host OS / bare metal | Foundation the knight stands on |
| **Gauntlets** | CLI tooling | How operators interact with the system |
| **Vambraces** | Observability (eBPF) | Fine-grained detail of what's happening |
| **Shield** | WAF / ingress-egress | Blocks incoming attacks |
| **Hauberk** | Service mesh (mTLS, circuit breakers) | Flexible protection between rigid components |
| **Cuirass** | Control plane (unheaded-daemon) | Core body armor — protects vital organs |
| **Pauldrons** | Load balancer | Bears the weight of traffic |
| **Sword** | Deployment pipeline | Offensive capability — how you ship |

## Layer Mapping

| Layer | Technical Name | Armor Piece |
|-------|---------------|-------------|
| 5 | User Interface | — (the head) |
| 4 | Application Services | Sword |
| 3 | Infrastructure Services | Hauberk, Cuirass |
| 2 | Control Plane | Cuirass, Pauldrons |
| 1 | Data Plane | Vambraces, Shield |
| 0 | Infrastructure | Sabatons |

---

> **Source:** [docs/lore/MEDIEVAL_ARMORY.md](../docs/lore/MEDIEVAL_ARMORY.md)
