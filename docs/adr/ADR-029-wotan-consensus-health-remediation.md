# ADR-029: Wotan Consensus Health + Automatic Remediation — Every Node a Watchdog

**Status:** Planned
**Date:** 2026-04-03
**Deciders:** Captain, Architect, BlackMage, Micromanager
**Priority:** HIGH — core Kingdom resilience

## Context

Currently, health checking is centralized in dashboard-backend. If the dashboard goes down, nobody knows anything is broken. This is a single point of failure for the entire monitoring system.

The Kingdom doctrine (CLAUDE.md) already defines consensus-based severity:

| % Reporting Failure | Severity | Action |
|---|---|---|
| 0 - 12.49% | OK | Healthy |
| 12.50 - 37.49% | WARN | Log + email |
| 37.50 - 62.49% | ERROR | Log + 2nd email |
| 62.50 - 87.49% | CRITICAL | Auto-remediate |
| 87.50 - 100% | PANIC | All hands |

**New rule: The magic number is 66.666...% (two-thirds consensus).**

If two-thirds or more of connected Wotan subscribers report a service as unhealthy, that service gets automatically restarted. No human approval needed. This is hardcoded into Wotan itself — not a skill, not a runbook, not optional.

## Decision

### Every Node is a Watchdog

Every service connected to Wotan MUST:
1. Health-check every other service it depends on (every 30 seconds)
2. Publish health reports to Wotan topic `system.health.reports`
3. Subscribe to `system.health.consensus` for remediation commands

This is not optional. It's compiled into every service binary via `pkg/health/watchdog.go`.

### Wotan is the Ballot Box

Wotan aggregates health reports and computes consensus:

```
Formula: failure_rate = unique_reporters_failing / total_dependent_services

If failure_rate >= 0.6667 (two-thirds):
  → Publish remediation command to system.health.remediate
  → Log to system.health.events
  → Zhenai Champion notified via system.health.alerts
```

### Remediation Chain

When consensus triggers remediation:

```
1. Wotan publishes: {service: "timeguru", action: "restart", consensus: 0.72, reporters: [...]}
2. The HOST running that service receives the message
3. Local watchdog agent executes: systemctl restart unheaded-timeguru
4. Watchdog reports restart result back to Wotan
5. Health checks resume — if still failing after 3 restarts → ESCALATE to human
```

### Hardcoded, Not Configurable

This behavior is baked into:
- `pkg/health/watchdog.go` — health check loop + Wotan publisher
- `pkg/health/consensus.go` — aggregation + threshold logic
- `services/wotan/internal/health/` — Wotan-side consensus engine
- Every service's `main.go` via `pkg/service/` template

The 66.67% threshold is a constant, not a config value. Changing it requires a code change + review. This prevents accidental misconfiguration from weakening the Kingdom's immune system.

```go
// pkg/health/consensus.go
const (
    // ConsensusThreshold is the fraction of reporters that must agree
    // a service is unhealthy before automatic remediation triggers.
    // Two-thirds (Byzantine fault tolerance).
    // This is intentionally hardcoded — not configurable.
    ConsensusThreshold = 2.0 / 3.0 // 0.666666...

    // MaxAutoRestarts before escalating to human
    MaxAutoRestarts = 3

    // HealthCheckInterval between checks
    HealthCheckInterval = 30 * time.Second
)
```

### Wotan Topics

```
system.health.reports    — individual service health reports (published by every node)
system.health.consensus  — computed consensus state (published by Wotan)
system.health.remediate  — remediation commands (published by Wotan when threshold hit)
system.health.events     — audit log of all health events
system.health.alerts     — high-severity alerts for Zhenai Champion
```

### Node Agent

Every Unheaded host runs a monitoring agent (part of the `unheaded-configs` .deb package):

```ini
# /etc/systemd/system/unheaded-watchdog.service
[Unit]
Description=Unheaded Kingdom Watchdog — Health Monitor + Auto-Remediation
After=network.target unheaded-wotan.service
Requires=unheaded-wotan.service

[Service]
Type=simple
ExecStart=/opt/unheaded/bin/watchdog
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

The watchdog:
1. Connects to local Wotan
2. Discovers all services on this host (via port scan + convention)
3. Health-checks every service every 30 seconds
4. Publishes reports to `system.health.reports`
5. Subscribes to `system.health.remediate`
6. Executes restart commands for local services
7. Reports restart results back to Wotan

### Zhenai's Role

Zhenai Champion is notified of all health events but does NOT make the restart decision — the consensus algorithm does. Zhenai's role:
- Monitor `system.health.alerts` for patterns
- Escalate to human when MaxAutoRestarts exceeded
- Run diagnostic runbooks when consensus keeps triggering
- Log everything to The Well for post-mortem analysis

The Champion is the brain, but the watchdog is the immune system. The immune system doesn't wait for the brain to decide — it acts on reflex (consensus threshold), and the brain reviews after.

## Implementation Phases

### Phase 1: pkg/health/watchdog.go + consensus.go
- Health check loop with Wotan publishing
- Consensus computation with 66.67% threshold
- Remediation command publishing

### Phase 2: Watchdog binary (cmd/watchdog/)
- Systemd-managed daemon
- Port scan discovery
- Local service restart capability
- Included in unheaded-configs .deb

### Phase 3: Integration
- Wire into pkg/service/ template (every service auto-registers)
- Wire into Wotan (consensus engine)
- Wire into Zhenai (alert subscription)

### Phase 4: Cross-host
- WEST watchdog monitors EAST services (via P2P link)
- EAST watchdog monitors WEST services
- Consensus computed across all hosts

## Consequences

### Positive
- No single point of failure for health monitoring
- Automatic recovery from common failures (crashed processes)
- Byzantine-tolerant: minority of bad reporters can't trigger false restarts
- Scales from 2 hosts to 200+ without architecture change
- Immune system analogy: reflexive response, brain reviews after

## Ping/Pong Full Mesh Health Protocol (Raven)

**Naming**: The health daemon is **Raven** — Odin's raven of memory. Flies over the Kingdom, remembers the state of all things, reports back to Wotan.

**Core concept**: Every service with a REST API or gRPC endpoint includes `pkg/health` and participates in the full mesh. This is NOT optional — it's compiled into every service via the `pkg/service/` template.

### Protocol

```
Service A pushes to Wotan:
  topic: system.health.report
  payload: { "service": "timeguru", "status": "ok", "timestamp": 1712188800 }

Wotan responds with aggregated state:
  { "services": {
      "wotan": {"status": "ok", "last_seen": 1712188795},
      "timeguru": {"status": "ok", "last_seen": 1712188800},
      "captain": {"status": "degraded", "last_seen": 1712188750},
      ...
    },
    "consensus": { "captain": { "failure_rate": 0.72, "action": "restart" } }
  }
```

Every service:
1. **Pushes** `health:ok:$TIMESTAMP` to Wotan at a configurable interval (default 30s)
2. **Receives** the aggregated health state of ALL other services in the response
3. **Acts** on consensus decisions (restart commands for local services)

This is a **ping/pong model** — the health push IS the ping, the aggregated state IS the pong. No separate health check loop needed — the reporting IS the checking.

### Configuration

Per-service config (in `/etc/unheaded/<service>.yaml` or env vars):

```yaml
health:
  report_interval: 30s        # How often to push health report (default 30s)
  dependencies:                # Services this service depends on (checked in response)
    - wotan
    - postgresql
  alert_threshold: 0.6667     # Consensus threshold (default: 2/3, NOT configurable in code but configurable which services to watch)
```

Default dependencies are auto-discovered from the service registry (`configs/services.yaml`). Custom dependencies can override for services that only care about specific peers.

### Full Mesh Visualization

```
     timeguru ──push──> WOTAN ──pong──> timeguru (sees all)
      captain ──push──> WOTAN ──pong──> captain (sees all)
    architect ──push──> WOTAN ──pong──> architect (sees all)
 micromanager ──push──> WOTAN ──pong──> micromanager (sees all)
    dashboard ──push──> WOTAN ──pong──> dashboard (sees all)
       kanban ──push──> WOTAN ──pong──> kanban (sees all)
       zhenai ──push──> WOTAN ──pong──> zhenai (sees all)
```

Every service knows the health of every other service. Wotan is the hub but NOT the decision-maker — consensus is computed by each service locally from the aggregated data. If Wotan itself goes down, services lose visibility but continue operating.

### Negative
- Restart storms possible if multiple services fail simultaneously (mitigate: rate limit restarts)
- Hardcoded threshold means no per-service tuning (feature: consistency > flexibility)
- Every service now has health-check overhead (mitigate: 30s interval is lightweight)
