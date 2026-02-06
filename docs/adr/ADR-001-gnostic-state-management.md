# ADR-001: Gnostic State Management (Pleroma/Kenoma Pattern)

## Status: Accepted

## Date: 2026-02-01

## Context

Unheaded is a configuration management automation platform that must continuously reconcile what infrastructure *should* look like (desired state) with what it *actually* looks like (observed state). This reconciliation loop is the heart of the control plane: drift detection, auto-remediation, and audit logging all depend on a clean separation between "intended reality" and "material reality."

Early prototypes conflated desired and actual state in a single data store, which led to ambiguous drift detection (is this the desired state or the current one?), fragile reconciliation logic, and difficulty reconstructing *how* the system arrived at its current state. The team needed a conceptual framework that would:

1. Clearly separate desired state from observed state at the service boundary level.
2. Provide an event-sourced audit trail showing all state transitions.
3. Enable chaos engineering by injecting faults into the gap between desired and actual.
4. Scale from 8 services (alpha) to 800+ services (production) without architectural changes.

Several approaches were considered:

- **Single state store with flags** (desired vs. actual as a column/field) -- too easy to conflate.
- **Kubernetes-style controller pattern** (spec/status on every object) -- requires adopting K8s primitives.
- **Custom reconciliation services with domain-driven names** -- requires inventing a vocabulary.

## Decision

We adopt Gnostic cosmological terminology to name and structure the four core state management microservices:

| Service | Greek Origin | Domain | Role |
|---------|-------------|--------|------|
| **Pleroma** | "fullness" | Configuration Management | The complete desired state -- what the system SHOULD be. Declarative configs, NixOS definitions, version-controlled truth. |
| **Kenoma** | "emptiness/void" | Current State / Reality | The actual observed state -- what the system ACTUALLY is. Drift detection, container health, runtime metrics. |
| **Anamnesis** | "remembrance" | Memory / History | Event sourcing, write-ahead log (WAL), audit trails. How we got from any past state to the present. State reconstruction on demand. |
| **Yaldabaoth** | "The Demiurge" | Chaos / Adversary | Chaos engineering, fault injection, adversarial testing. Tests resilience by deliberately widening the gap between Pleroma and Kenoma. |

The reconciliation loop operates as follows:

```
Pleroma (desired) --> compare --> Kenoma (actual) --> if drift --> remediate
                                                   --> log event --> Anamnesis
                                                   --> Yaldabaoth injects faults for testing
```

State drift events are published to the Busboy topic `state.drift.detected`. Remediation events are published to `state.drift.resolved`. All transitions are recorded in Anamnesis for full auditability.

## Consequences

### Positive

- **Conceptual clarity**: The Gnostic metaphor maps precisely to the configuration management domain. Pleroma (divine fullness = desired state) vs. Kenoma (material void = observed reality) is intuitive once understood, and the vocabulary eliminates ambiguity in design discussions.
- **Clean service boundaries**: Each service has a single, well-defined responsibility. Pleroma never observes; Kenoma never prescribes. This prevents the state conflation bugs that plagued early prototypes.
- **Built-in audit trail**: Anamnesis provides event-sourced state reconstruction without bolting it on after the fact. Any past system state can be replayed from the event log.
- **Chaos engineering as a first-class citizen**: Yaldabaoth is not an afterthought -- it is architecturally embedded, ensuring resilience testing is part of the development workflow from day one.
- **Scalability**: The pattern is service-count-agnostic. Whether Pleroma manages 8 container definitions or 800, the reconciliation contract is identical.

### Negative

- **Onboarding friction**: New engineers must learn Gnostic terminology before understanding the architecture. The names are not self-documenting to someone unfamiliar with the metaphor.
- **Cultural risk**: The theological naming could be seen as pretentious or exclusionary. Documentation must always pair the Gnostic name with its plain-English function (e.g., "Pleroma (Configuration Truth)").
- **Four services where two might suffice**: For the alpha with 8 managed containers, a simpler desired/actual split with inline logging would be functionally adequate. The four-service split is an investment in future scale.
- **Tight coupling to the metaphor**: If the metaphor breaks down for a future domain (e.g., multi-tenant state where "desired" varies per tenant), refactoring the naming may be disruptive.

## References

- `docs/MICROSERVICES.md` -- The Gnostic Microservice Layer specification
- `docs/ARCHITECTURE.md` -- State Management and Drift Detection sections
- `pkg/state/reconciler.go` -- Reconciler implementation (1,850 LOC)
- `cmd/unheaded-daemon/internal/state/` -- Desired vs. actual state management
