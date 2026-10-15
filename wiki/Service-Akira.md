# Akira — Kingdom Health Daemon

Akira checks every Kingdom service, publishes its reports to Wotan, tallies every reporter's votes, and publishes the health band. It implements the percentage-consensus doctrine (ADR-029). It's named after the film.

## Running

| | |
|---|---|
| Container | `unheaded-akira` (compose service `akira`, live on west) |
| Port | 19100 (`pkg/ports.Akira`) |
| Wotan topics | publishes `system.health.reports` (each reporter's checks) and `system.health.consensus` (band changes) |
| Remediation | off by default; `--remediate` restarts `unheaded-<name>` when two-thirds of reporters see it failing, at most 3 times per outage, then escalates |

## Bands

`failing_reporters / reporters x 100`, where each reporter's latest report from the last three sweeps counts as one vote:

| % | Band |
|---|---|
| 0 – 12.49 | OK |
| 12.50 – 37.49 | WARN |
| 37.50 – 62.49 | ERROR |
| 62.50 – 87.49 | CRITICAL (auto-remediate from 66.67%) |
| 87.50 – 100 | PANIC |

Health state lands in The Well (`unheaded_ops.service_health_current` / `_transitions`) via dashboard-backend.

## Endpoints and flags

| | |
|---|---|
| `GET /health` | liveness |
| `GET /api/v1/status` | current band per service |
| `akira --once` | one sweep, print, exit |
| `--config`, `--node`, `--port`, `--wotan` | config file, node id (default hostname), API port, Wotan HTTP address |

---

> **Source:** [cmd/akira/main.go](../cmd/akira/main.go) · [ADR-029](../docs/adr/ADR-029-wotan-consensus-health-remediation.md) · [pkg/health/consensus.go](../pkg/health/consensus.go)
