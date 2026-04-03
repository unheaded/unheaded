# ADR-025: Kanban Mobile App — Remote Infrastructure Management via Phone

**Status:** Pipe Dream (Age 4+)
**Date:** 2026-04-03
**Deciders:** Captain, Architect, Developer
**Depends on:** ADR-019 (Zhen Champion), ADR-024 (Runbook Automation), ADR-017 (Hybrid Inference), ADR-018 (RAFT Training), ADR-020 (Kanban Bugs — fixed first)

## Vision

A phone app linked to the Zhen hybrid RAFT/MCP inference server that provides:

1. **Kanban board** — full task management from phone (create, move, approve, review)
2. **Infrastructure notifications** — push alerts from Wotan consensus health system (ADR severity: OK → WARN → ERROR → CRITICAL → PANIC)
3. **Dependency approval workflow** — approve new external dependencies (ADR-004 exceptions) from phone
4. **Runbook execution** — trigger and monitor runbook runs (ADR-024) with step-by-step progress
5. **Zhen chat** — ask Zhen questions, get RAG-powered answers with Mistral-7B or Claude backends
6. **Remote management** — service restart, log tail, health dashboard, all from phone

## Why This Matters

Single developer. Two bare metal hosts. Can't always be at a terminal. The phone becomes the control plane UI for:
- Approving dependency exceptions while on the go
- Getting paged when EAST goes down at 3am
- Kicking off an index rebuild from the couch
- Checking if a deploy succeeded while walking the dog

## Architecture (Sketch)

```
Phone App (React Native or Flutter)
    ↓ HTTPS + WebSocket
Gateway (port 21443, TLS)
    ↓
Kanban API (port 16668) ←→ PostgreSQL (The Well)
    ↓
Zhen Champion MCP Server ←→ Mistral-7B (port 20100) / Claude API
    ↓
Wotan (port 18001) ←→ All Services
    ↓
Runbook Engine ←→ Shell execution + verification gates
```

### Push Notifications
- Wotan `system.outage.reports` topic → Gateway → WebSocket → Phone
- Severity-based notification levels:
  - OK/WARN: silent badge update
  - ERROR: notification
  - CRITICAL: persistent notification + sound
  - PANIC: repeated alarm until acknowledged

### Authentication
- JWT token (ADR-051 auth framework) with phone-specific scope
- Biometric unlock on phone side
- API key rotation via Kanban approval workflow (meta: approve the approval mechanism)

### Approval Workflow
- New dependency PRs create a Kanban task in "Review" column
- Phone gets push notification: "New dependency: google.golang.org/grpc — Approve?"
- Owner taps Approve/Reject
- Champion agent processes the decision, updates ADR-004 register, merges or closes PR

## Implementation Phases

### Phase 1: API Foundation (Age 3)
- Kanban REST API already exists (port 16668)
- Add WebSocket endpoint for real-time task updates
- Add push notification infrastructure (Wotan → WebSocket bridge)
- Add approval workflow endpoints

### Phase 2: Web PWA (Age 3)
- Progressive Web App — works on phone browser, no app store needed
- Service worker for push notifications
- Offline-capable task viewing
- Responsive Kanban board (touch-friendly drag-drop)

### Phase 3: Native App (Age 4)
- React Native or Flutter (or Kotlin/Swift if going fully native)
- Full push notification support (FCM/APNs)
- Biometric auth
- Background health monitoring

### Phase 4: Full MCP Integration (Age 4+)
- Zhen Champion as MCP server accessible from phone
- Natural language infrastructure management: "restart wotan on EAST"
- Runbook execution with live progress streaming
- Voice commands (stretch goal)

## Consequences

### Positive
- Single developer can manage infrastructure 24/7 from anywhere
- Approval workflows don't block on terminal access
- Incident response time drops from "when I get to my desk" to "immediately"
- Dogfooding: Unheaded manages Unheaded from a phone

### Negative
- Mobile dev is a different skill set (mitigated by PWA-first approach)
- Push notification infrastructure adds complexity
- Phone becomes a single point of failure for approvals (mitigated by web fallback)

### Risks
- Security: phone compromise = infrastructure compromise (mitigate with scoped JWT + biometric)
- Notification fatigue: too many alerts → ignored alerts (mitigate with severity filtering)

## Note

This is explicitly a pipe dream / long-term vision. PWA (Phase 2) is the realistic near-term target — it gets 80% of the value with 20% of the effort. Native app is a nice-to-have.
