# Microservices Architecture

## The Fragmentation Doctrine (February 1, 2026)

> **"From Monolith to Microservices - Cascading Failures Shall Not Plague Us"**

The Kingdom evolves from monolithic to microservice architecture:
- Each component is isolated, independently deployable, independently restartable
- Circuit breakers at every boundary (Hauberk)
- Bulkheads between services - one service's failure stays contained
- Wotan (Fae Chamber) as the message backbone prevents tight coupling
- gRPC for internal communication, REST for external APIs
- Each service owns its own data store (no shared databases)

---

## The Gnostic Microservice Layer

The Kingdom adopts Gnostic terminology for its core microservice architecture.

### The Four Gnostic Services

| Service | Greek/Origin | Domain | Kingdom Role |
|---------|--------------|--------|--------------|
| **Pleroma** | πλήρωμα ("fullness") | Configuration Management | The complete desired state, configuration truth - what the system SHOULD be |
| **Kenoma** | κένωμα ("emptiness") | Current State/Reality | The actual observed state, drift detection - what the system ACTUALLY is |
| **Anamnesis** | ἀνάμνησις ("remembrance") | Memory/History | Event sourcing, WAL, audit logs, state reconstruction - how we got here |
| **Yaldabaoth** | The Demiurge | Chaos/Adversary | Chaos engineering, fault injection, adversarial testing - testing resilience |

### The Gnostic Architecture Pattern

```
┌─────────────────────────────────────────────────────────────────┐
│                    PLEROMA (Configuration Truth)                 │
│         "The Fullness" - What the Kingdom SHOULD be              │
│     Desired state, declarative configs, intended reality         │
└───────────────────────────────┬─────────────────────────────────┘
                                ↓ Reconciliation Loop
┌─────────────────────────────────────────────────────────────────┐
│                     KENOMA (Current Reality)                     │
│         "The Void" - What the Kingdom ACTUALLY is                │
│   Observed state, drift detection, the deficient material world  │
└───────────────────────────────┬─────────────────────────────────┘
                                ↓ Events & History
┌─────────────────────────────────────────────────────────────────┐
│                   ANAMNESIS (Historical Memory)                  │
│       "Remembrance" - What the Kingdom WAS and HOW we got here   │
│   Event sourcing, WAL, audit trails, state reconstruction        │
└───────────────────────────────┬─────────────────────────────────┘
                                ↓ Testing & Chaos
┌─────────────────────────────────────────────────────────────────┐
│                 YALDABAOTH (The Adversary)                       │
│      "The Demiurge" - Bringer of Chaos, Tester of Resilience     │
│   Chaos engineering, fault injection, adversarial simulation     │
└─────────────────────────────────────────────────────────────────┘
```

**Theological Context for Engineers:**
In Gnostic cosmology, **Pleroma** is the fullness of the divine realm - perfect, complete, the ideal. **Kenoma** is the void, the material world that falls short. **Anamnesis** is the soul's remembrance of divine origin. **Yaldabaoth** is the Demiurge - a false god bringing disorder to test resilience.

---

## The Purity of Interface

> **"No Node.js Shall Touch These Lands - Basic HTML/CSS/JS for All User Interfaces"**

### UI Stack
- **Frontend**: Pure HTML + CSS + vanilla JavaScript (no frameworks, no npm)
- **Backend**: Go for all web backend services
- Go's `embed` directive serves static files directly from binaries
- Single binary deployment - no build step, no node_modules

### Reference Implementations
- `~/tmp/weather-daemon-main/weather.js` - Python backend, vanilla JS frontend
- `~/tmp/rss-daemon-main/frontend.js` - Pure JS RSS display
- `~/tmp/www-main/html/` - Static HTML served by Python

### Go Backend Pattern (THE KINGDOM STANDARD)
```go
//go:embed static/*
var staticFiles embed.FS

func main() {
    http.Handle("/", http.FileServer(http.FS(staticFiles)))
    http.HandleFunc("/api/v1/...", apiHandler)
    log.Fatal(http.ListenAndServe(":19000", nil)) // Doom Range port
}
```

---

## Service Catalog (Royal Court)

### timeguru
- **Domain**: Living Timeline
- **Question**: WHEN/WAS/WILL
- Timeline tracking and milestone management
- Maintains timeline.md with JSON/YAML mirrors
- REST API: GET/POST /api/v1/timeline
- **Hollow**: Oracle's Antre

### captain
- **Domain**: Vision & Strategy
- **Question**: WHY & WHERE
- Strategic decisions and vision alignment
- REST API: GET/POST /api/v1/strategy
- **Hollow**: Commander's Quarters

### micromanager
- **Domain**: Execution & QA
- **Question**: WHAT & WHEN
- Task breakdown and QA oversight
- REST API: GET/POST /api/v1/tasks
- **Hollow**: War Room

### architect
- **Domain**: Technical Design
- **Question**: HOW
- Infrastructure design and tech decisions
- REST API: GET/POST /api/v1/designs
- **Hollow**: Sage's Lair

### developer
- **Domain**: Code & TDD
- **Question**: BUILD
- Security-first implementation
- **Hollow**: The Forge

### wotan
- **Domain**: Coordination
- **Question**: GLUE
- Message bus and cross-service communication
- **Hollow**: The Fae Chamber

---

## Communication Pattern

All services communicate via Wotan (message bus) using the Fae Chamber protocols.

### Topic Naming Convention
```
<domain>.<event_type>[.<subtype>]
```

### Standard Topics
| Topic | Publisher | Subscribers |
|-------|-----------|-------------|
| `tasks.created` | micromanager | timeguru, kanban |
| `tasks.completed` | micromanager | timeguru, captain |
| `timeline.updates` | timeguru | captain, dashboard |
| `decisions.approved` | captain | micromanager, architect |
| `alerts.critical` | any | all services |
| `state.drift.detected` | kenoma | pleroma, anamnesis |

See `FAE_CHAMBER_CONTRACTS.md` for full protocol specification.

---

## Codebase Location

**Primary Repository**: `~/tmp/unheaded/`
- All Go services, packages, and infrastructure code
- The canonical source of truth (115K+ LOC)

**Related Artifacts in `~/tmp/`:**
- `wotan/` - Phase 0 message bus (13.5K LOC)
- UI pattern references: `weather-daemon-main/`, `www-main/`, `rss-daemon-main/`

---

*Last Updated: February 1, 2026*
*The Fragmentation Doctrine and Gnostic Layer proclaimed*
