# ADR-084 — Huginn: Host Metrics Agent

**Status:** Accepted
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

A bare-metal host metrics agent (`cmd/host-agent`) was built during the dashboard
demo sprint (2026-07-24) to give the dashboard real host-level telemetry — CPU,
memory, swap, disk, network connections, process counts — from the actual host
rather than from inside a container namespace where those numbers are unreliable.

The agent:
- reads `/proc` and `statfs` directly on the host
- serves `GET /host-summary` (JSON matching the dashboard's per-host shape)
- serves `GET /metrics` (Prometheus `host_*` series)
- pushes Prometheus-format metrics to VictoriaMetrics on a configurable interval
- serves `GET /health`

It runs natively (not in Docker) on every Kingdom host — currently WEST and EAST.

The binary and package are named `host-agent`, a generic placeholder name that
does not fit the established Kingdom naming convention.

---

## Decision

Rename `cmd/host-agent` → `cmd/huginn` and the resulting binary to `huginn`.

**Rationale for "Huginn":**
Huginn is one of Odin's two ravens (alongside Muninn). Each day Huginn flies
across all the worlds of Yggdrasil, observing and reporting back to Odin.
This is precisely the function of the agent: it flies across all Kingdom hosts,
observes their state, and reports it back to the dashboard (Odin's eye).

The established Kingdom naming convention draws on Norse mythology for
infrastructure roles (Wotan, Monad, Sophia, Heimdall, Akira, Gleipnir, etc.).
Huginn fits the pattern exactly:

| Service   | Origin          | Role                               |
|-----------|-----------------|------------------------------------|
| Wotan     | Norse (Odin)    | Memory / message bus               |
| Sophia    | Greek           | Knowledge graph                    |
| Heimdall  | Norse           | Watchman / drift detection daemon  |
| Akira     | Japanese        | Health monitor                     |
| **Huginn**| Norse (raven)   | Host observer / metrics reporter   |

---

## Deliverables

1. **Rename** `cmd/host-agent/` → `cmd/huginn/` (package `main`, binary `huginn`)
2. **Rename** systemd unit → `huginn.service` (see `deploy/systemd/huginn.service`)
3. **Update** dashboard backend: any hard-coded reference to `host-agent` binary
   or service name → `huginn`
4. **Update** `start-east-services.yaml` runbook to reference `huginn`
5. **Update** ADR-INDEX.md

Binary flags remain unchanged:
```
huginn -host <label> -listen :9110 -vm <victoria-url> -interval 10s
```

---

## Naming Reservation

Muninn (Huginn's twin raven, representing memory) is reserved for a future
service if a complementary role emerges — e.g., a persistent host state store
or long-term metrics archiver.

---

## Consequences

- `host-agent` name disappears from the codebase
- `huginn` appears consistently in: binary, systemd unit, dashboard config,
  runbooks, Prometheus labels (`host` label unchanged — still the hostname)
- EAST and WEST both run `huginn.service` (disabled/manual start by convention)
- VictoriaMetrics push target is host-specific: EAST pushes to WEST's Victoria
  over the P2P link (192.168.13.1:8428); configured via `/etc/huginn.env`
