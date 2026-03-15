# ADR-002: Kingdom Naming Convention (Armor-Themed Services)

## Status: Accepted

## Date: 2026-01-28

## Context

Unheaded is a large platform with dozens of packages, services, and infrastructure layers. The codebase at alpha already exceeds 160,000 lines of code across Go, Rust, Nix, JavaScript, and documentation. As the system grew, generic names like "controller," "manager," "proxy," and "service" became ambiguous. Multiple components legitimately needed names like "gateway" or "monitor," and conversations about architecture became unclear ("which manager?" "the load balancer or the health manager?").

The team needed a naming convention that would:

1. Give every component a unique, memorable name that conveys its role at a glance.
2. Provide a unifying metaphor that makes the full system architecture easy to visualize and discuss.
3. Scale to new components without name collisions or forced acronyms.
4. Reflect the project's identity and culture (Unheaded -- "the headless kingdom").

Alternatives considered:

- **Generic technical names** (gateway, controller, scheduler) -- already causing confusion; too many collisions in a system this size.
- **Animal/element themes** (popular in open source) -- no structural relationship between names; a "falcon" tells you nothing about its relationship to an "otter."
- **Military rank hierarchy** -- implies a strict command chain that does not match the peer-to-peer pub/sub architecture.

## Decision

We adopt a **medieval armor / kingdom** naming metaphor where every component is a piece of a knight's armor or a location within a castle. The metaphor provides two complementary taxonomies:

### Armor Pieces (Infrastructure Components)

| Armor Piece | Domain | Primary Package |
|-------------|--------|-----------------|
| **Cuirass** | Control Plane | `cmd/unheaded-daemon/` |
| **Shield** | WAF / API Gateway | `pkg/waf/`, `services/gateway/` |
| **Sword** | Deployment Engine | `pkg/deploy/` |
| **Hauberk** | Service Mesh | `pkg/mesh/` |
| **Pauldrons** | Load Balancer | `pkg/loadbalancer/` |
| **Vambraces** | Observability | `pkg/tracing/`, `cmd/dashboard-backend/` |
| **Gauntlets** | CLI + API | `cmd/unheaded-cli/` |
| **Tassets** | Storage | `pkg/storage/` |
| **Sabatons** | Bare Metal Provisioning | `pkg/baremetal/` |
| **Helm** | Gateway (nginx) | `services/gateway/` |
| **Greaves** | Observation Layer | `cmd/dashboard-backend/`, `cmd/trace-collector/` |
| **Cape/Cloak** | Dashboard UI | `dashboard/` |

### Arcane Hollows (Hidden Infrastructure)

| Hollow | Domain | Package |
|--------|--------|---------|
| **Whispering Void** | eBPF Layer | `pkg/ebpf/`, `ebpf/` (Rust) |
| **Crystal Grotto** | Secrets Management | `pkg/secrets/` |
| **Seer's Antre** | Timeline Service | `services/timeguru/` |
| **Fae Chamber** | Message Bus (Wotan) | `pkg/wotan-client/`, `pkg/events/` |
| **Sage's Lair** | Architecture Decisions | `services/architect/`, `docs/adr/` |
| **Mythic Abyss** | Telemetry Underworld | `cmd/trace-collector/` |
| **Daemon's Den** | Control Plane HQ | `cmd/unheaded-daemon/` |

### Naming Rules

1. Every infrastructure component maps to exactly one armor piece or hollow.
2. Documentation always pairs the Kingdom name with its plain-English function on first use (e.g., "The Hauberk (Service Mesh)").
3. Code identifiers (package names, binary names) use the plain-English name for IDE discoverability (`pkg/mesh/`, not `pkg/hauberk/`). The Kingdom name is used in architecture docs, diagrams, and team communication.
4. New components must be assigned a Kingdom name before implementation begins.

## Consequences

### Positive

- **Unique and memorable**: "Cuirass" is unambiguous in a way that "controller" never will be. Team members can discuss architecture without qualifying every noun.
- **Structural metaphor**: Armor pieces have inherent spatial relationships (helm protects the head, sabatons protect the feet). This maps well to the layered architecture: Helm (gateway) faces the outside world; Sabatons (bare metal) touch the hardware.
- **Visual architecture**: The "Complete Knight" diagram in `docs/KINGDOM_ARCHITECTURE.md` makes the full system instantly comprehensible as a single armored figure, with each piece labeled.
- **Cultural identity**: The naming convention reinforces the "Unheaded Kingdom" brand and makes the project distinctive in a landscape of generic infrastructure tools.
- **Extensibility**: Medieval armor has dozens of named pieces (gorget, couter, poleyn, fauld, etc.), providing ample room for future components.

### Negative

- **Onboarding cost**: New contributors must learn the mapping between Kingdom names and technical functions. The `docs/PROJECT_STRUCTURE.md` armor mapping table is essential reading.
- **External communication**: When discussing the system with users or partners, Kingdom names may confuse rather than clarify. External-facing documentation should prefer plain-English names.
- **Code vs. docs disconnect**: Package names use plain English (`pkg/mesh/`) while architecture docs use Kingdom names ("Hauberk"). This split is intentional but requires discipline to maintain consistently.
- **Metaphor limitations**: Not every infrastructure concept maps cleanly to armor. The "Arcane Hollows" secondary metaphor (castle locations) was needed to cover non-armor components, adding a second layer of vocabulary.

## References

- `docs/KINGDOM_ARCHITECTURE.md` -- The Complete Knight diagram and full nomenclature
- `docs/PROJECT_STRUCTURE.md` -- Armor Mapping table (code locations)
- `docs/SYSTEM_DIAGRAM.md` -- Visual system overview using Kingdom names
