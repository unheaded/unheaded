# S77 — Interface Contracts (sprint index)

**Sprint:** S77 — Age 2 Acceleration Campaign
**Phase:** 5 — Interchangeability Documentation
**Status:** Shipped — IaC Renderer + Observability Adapter interfaces in-tree
**Canonical doc:** [`docs/INTERFACE-CONTRACTS-S77.md`](../INTERFACE-CONTRACTS-S77.md)
**Gate test:** [`tests/s77/s77_verification_test.go::TestPhase5_*`](../../tests/s77/s77_verification_test.go)

---

## Purpose

Sprint-folder index for the S77 interface-contracts inventory. The
canonical doc covers the two trait-shaped Go interfaces — `Renderer`
(`pkg/iac/iac.go`) and `Adapter` (`pkg/observability/observability.go`)
— that enforce anti-lock-in across IaC backends and observability
backends. **This file is the broader service-to-service contract
matrix** for the Kingdom: who talks to whom, over what transport, with
what key types.

The Kingdom's transport priority is `Wotan gRPC streaming → HTTP/3 →
HTTP/2 → HTTP/1.1` (the gRPC-first cascade implemented in
`pkg/transport/`). Service discovery is four-layer (Wotan registration
→ port scan → convention scan → static fallback) via `pkg/discovery/`.

---

## Transport tiers

| Tier | Transport | When used |
|------|-----------|-----------|
| **Tier 1 — bus** | Wotan gRPC topics (`services/wotan/proto/topic.proto`, `chat.proto`, `replication.proto`) | service-to-service async events, log aggregation, alerts |
| **Tier 2 — RPC** | gRPC (port 18001 for Wotan; 17001 for unheaded-daemon; 19004 for monad; 19005 for sophia) | sync request/response between control-plane services |
| **Tier 3 — REST** | HTTP/3 → HTTP/2 → HTTP/1.1 (`pkg/transport/cascade.go`) | user-facing APIs, dashboard, kanban |
| **Tier 4 — sidecar** | nginx per-app proxies (ports 18080, 19080, 19081, 20080, 20081, 21090) | per-service load balancing + telemetry |
| **Tier 5 — edge** | HAProxy (21080/21443 edge; 21081 internal) | TLS termination, rate limiting, service routing |

---

## Contract matrix (caller → callee)

| Caller | Callee | Transport | Key types / topics |
|--------|--------|-----------|--------------------|
| any service | wotan | Tier 1 (gRPC streaming) | `TopicPublishRequest`, `TopicEvent` (ML-DSA-65 signed on `config.*`) |
| any service | wotan | Tier 2 (gRPC) | `system.discovery` for registration; `system.outage.reports` for health consensus |
| any service | logagg | Tier 1 (Wotan topic) | `logs.<service>.<level>` — zerolog hook in `pkg/logagg/` |
| dashboard-backend | timeguru | Tier 3 (REST) | `GET /api/v1/timeline` → `Timeline` JSON |
| dashboard-backend | architect | Tier 3 (REST) | `GET /api/v1/design/*` |
| dashboard-backend | captain | Tier 3 (REST) | `GET /api/v1/strategy/*` |
| dashboard-backend | micromanager | Tier 3 (REST) | `GET /api/v1/execution/*` |
| dashboard-backend | monad | Tier 2 (gRPC, port 19004) | unified state CRUD |
| dashboard-backend | sophia | Tier 2 (gRPC, port 19005) | knowledge-graph queries |
| dashboard-backend | trace-collector | Tier 1 (Wotan topic `flow.cross_host`) | Monad flow tuples |
| kanban-app | timeguru | Tier 3 (REST) | timeline rendering for the meta moment |
| gateway | any service | Tier 5 → Tier 3 | TLS-terminated routing |
| trace-collector | wotan | Tier 1 | `flow.cross_host`, `flow.local` Monad packet streams |
| unheaded-daemon | any service | Tier 2 (gRPC, port 17001) | drift-detect + reconcile RPCs |
| zhen-web-ui | zhen-inference | Tier 3 (REST, port 20100) | inference requests |
| heimdall-daemon | enkrateia | Tier 1 (Wotan topic) | drift alerts (alerts-only, zero FS mutations per ADR-043) |
| gjallarhorn-sender | UPC | Tier 1 (Monad 20-byte trigger packet) | bootstrap multicast triggers |

The `services/` tree currently holds **34 active services** (per the
S78 LoC audit in CLAUDE.md). The matrix above captures the Tier 1
through Tier 5 *patterns* without enumerating every adjacency; the
authoritative caller list per callee lives in each service's
`/health/dependencies` endpoint and is reported into the
`system.outage.reports` Wotan topic per the percentage-based consensus
table in CLAUDE.md.

---

## Service discovery surface

`pkg/discovery/` provides the four-layer discovery used by every
service on startup:

1. **Wotan registration** — `system.discovery` topic, TTL-based reaping.
2. **Port scan** — verifies expected ports are listening on the host.
3. **Convention scan** — `/opt/unheaded/<service>/config.yaml` lookup.
4. **Static fallback** — `configs/services.yaml` for known-good defaults.

`pkg/discovery/setup.go::SetupServiceDiscovery()` is called by every
service `main.go`, so every Tier-2 / Tier-3 contract above is
discoverable rather than hardcoded.

---

## Cross-cutting contracts

- **Health protocol** — every service implements `GET /health` (HTTP
  200 if healthy) and `GET /ready` (HTTP 200 if ready to serve). gRPC
  services additionally implement the `grpc.health.v1.Health` service.
- **Metrics protocol** — every service exposes `GET /metrics` with the
  Prometheus `unheaded_http_requests_total`,
  `unheaded_http_request_duration_seconds`,
  `unheaded_wotan_messages_published_total` baseline (CLAUDE.md §
  Metrics).
- **Auth protocol** — every service installs
  `auth.Middleware(authenticator)` on its HTTP router; Authenticator
  is pluggable (Noop / APIKey / JWT) per `pkg/auth/`.
- **Triple-mirror data format** — services with persistent state emit
  MD (human source of truth), JSON (API), YAML (IaC) — CLAUDE.md §
  Triple Mirror Strategy.

---

## PROPOSED / TBD per S77 close-out

- A machine-readable manifest (`references/service-contracts.yaml`)
  that the dashboard could render as a live caller-callee graph. The
  matrix above is the human-readable seed; emitting it from each
  service's startup registration is a S78+ task.

---

Free to use. Free to share.
