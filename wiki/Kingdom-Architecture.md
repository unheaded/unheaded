# Kingdom Architecture

The Kingdom's components as one picture: the six technical layers, the armor pieces each layer wears, and the Gnostic state services alongside them. Naming rules: [[Naming Map|Naming-Map]], [[Medieval Armory|Medieval-Armory]], [[Sacred Hierarchy|Sacred-Hierarchy]].

## The hierarchy

```
 Layer 5  User interface      The Crown ─ dashboard, kanban            Cloak (user dashboard)
 Layer 4  Application         Captain · Timeguru (vision)              Sword (deploy pipeline)
                              Micromanager · Architect (execution)     Cape (internal web framework)
 Layer 3  Infrastructure svcs Wotan, the Fae Chamber (message bus)     Hauberk (mesh, mTLS)
                              Monad · Sophia (protocol)                Tassets (data layer → The Well)
 Layer 2  Control plane       unheaded-daemon                          Cuirass · Pauldrons (LB)
                              Pleroma (desired) ⇄ Kenoma (actual) → drift
 Layer 1  Data plane          eBPF, the Whispering Void                Vambraces (observability)
                              Shield (BIRTH at XDP, DEATH at TC)       Shield (WAF / boundary)
 Layer 0  Infrastructure      host OS, bare metal, network fabric      Sabatons (bare-metal agent)

 Operator grip on every layer: Gauntlets (CLI + API, cmd/unheaded-cli)
 Outside the hierarchy:       Yaldabaoth (chaos injection), the LICH (red team)
```

Each armor piece is a Go package under `services/<name>/`.

## Arcane Hollows

The hollows group components by domain rather than layer:

| Hollow | Components |
|--------|------------|
| Crown | Captain, Timeguru |
| Forge | Architect, Micromanager, IaC renderers |
| Gnostic | Pleroma, Kenoma, Anamnesis (event memory) |
| Fae | Wotan, topic contracts ([[Fae-Chamber-Contracts]]) |
| Protocol | Monad, Sophia, Shield, Shim |
| Void | eBPF programs ([[eBPF-Programs]]) |
| Sabaton | Host OS, network fabric, BGP (Sleipnir) |

## Gnostic state loop

```
 Pleroma (desired state, from Git) ──┐
                                     ├─▶ Cuirass reconciles ──▶ publish state.changed
 Kenoma  (actual state, observed) ───┘          │
                                                └─▶ Anamnesis records every transition
```

## The Meta Moment

The Kingdom hosts its own development: the kanban board reads the timeline from Timeguru and its cards from The Well, every request is traced by eBPF, and the dashboard shows it live. See [[The Meta Moment|The-Meta-Moment]].

## Legacy names

The 2026-01 source doc used an earlier mapping (Breastplate = Wotan, Gauntlets = agent services, Greaves = observation layer, 10.10.10.x addresses on ports 8000–8083). The table above follows the current `services/` packages and the Doom Range ports ([[Port Registry|Port-Registry]]).

---

> **Source:** [docs/architecture/KINGDOM_ARCHITECTURE.md](../docs/architecture/KINGDOM_ARCHITECTURE.md) (2026-01, full ASCII + Mermaid renderings) · `services/*/` package docs
