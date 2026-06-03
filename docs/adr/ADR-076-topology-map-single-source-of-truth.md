# ADR-076: Single Source of Truth for the Live Topology Map

**Status:** PROPOSED (planning)
**Date:** 2026-06-02
**Deciders:** Architect (4 minds), Muck
**Tags:** observability, dashboard, wotan, victoriametrics, topology, read-plane, eBPF

> Planning ADR. It decides *where the live topology map gets its data* and, in doing so,
> resolves a pre-existing divergence in how operational views read telemetry. Free to use,
> free to share — GPL-3.0. No commercial framing.

---

## Context and Problem Statement

We built a live topology map (`demos/unheaded-topology-enterprise.html`, dual-skin
Enterprise/Kingdom) that renders holds (services), Monad packet flows, health, and the
three live agents (Chaos / Yaldabaoth, Adversary / Lich, AI-Ops / ZhenAI). Today it runs on
**simulated** telemetry behind a single `stepSim()` seam.

Before wiring it to real data, one question must be answered correctly, because getting it
wrong produces a dashboard that *lies*:

> **Where does the dashboard service pull from? Where does Grafana pull from? Where will the
> map pull from? They must all be the same source.**

### Current wiring (traced from the repo, not assumed)

| View / consumer | Reads from | Evidence |
|---|---|---|
| **Grafana** | **VictoriaMetrics** (PromQL) + **Loki** (logs) | `config/grafana/provisioning/datasources/victoriametrics.yml` → `http://victoria:8428` (isDefault); `monitoring/grafana/.../datasources.yaml` lists Prometheus, Loki, VictoriaMetrics |
| **VictoriaMetrics** | scrapes each service `/metrics` | `config/victoria/scrape.yml` → `wotan:18000`, `timeguru:19000`, `architect:19001`, `captain:19002`, `micromanager:19003`, `dashboard-backend:16667`, `kanban-app:16668` |
| **Prometheus** (second metrics stack) | scrapes the IPv6 bare-metal fabric | `monitoring/prometheus/prometheus.yml` → `[fd00:dead:beef::1/2]` node_exporter `:9100`, daemon `:9090`, frr `:9342`, bird `:9324`, routing_health `:8080`, doom-range `16666–16689` |
| **dashboard-backend** (metrics) | its **own** scraper polling service `/metrics` | `cmd/dashboard-backend/internal/scraper/scraper.go` (`RegisterTarget` → `GET http://host:port/metrics`, in-memory aggregator, 1h / 1000-sample retention) |
| **dashboard-backend** (flows/latency/packets) | **Wotan** topics | `internal/ebpf/ingestor.go` subscribes `ebpf.packet.events`, `ebpf.flow.events`, `ebpf.latency.events`, `ebpf.syscall.events` |
| **dashboard-backend** (logs) | `pkg/logagg` ring buffer (fed by Wotan `logs.<svc>.<lvl>`) | `internal/logs/handler.go` → `GET /api/v1/logs`, `WS /ws/logs` |
| **dashboard-backend** (events) | **Wotan** subscription | `internal/events/events.go` (metrics/health/alert/timeline/task/decision/system) |
| **Topology map** | nothing yet — `stepSim()` simulation | `demos/unheaded-topology-enterprise.html` |

### The problem, stated plainly

There is **one origin per data class**, but **three independent metrics scrape loops** over it:

1. VictoriaMetrics scrapes `/metrics` (this is what Grafana shows).
2. Prometheus (`monitoring/`) scrapes the same fabric on a *different* port map.
3. dashboard-backend scrapes `/metrics` *again* into its own aggregator (this is what the
   existing dashboard UI shows).

Because each loop has its own cadence, retention, and target list, **the same metric can
read differently in Grafana vs. the dashboard UI.** The configs don't even agree on identity:
`wotan` is `:18000` in `config/victoria/scrape.yml` but labelled on `[fd00:dead:beef::1]:16671`
in `monitoring/prometheus/prometheus.yml`. If the map naively adds a *fourth* scrape path, we
institutionalize the drift.

Origins that are **already single** and correct:
- **Flows / packets / latency / chaos / security / remediation events → Wotan** (event bus +
  ring buffer; fed by the eBPF data plane via `trace-collector`).
- **Durable state / health history → Postgres "The Well"** (`unheaded_app/ops/config`, ADR-016 /
  ADR-001 gnostic stores).

---

## Decision Drivers

- **One truth per data class.** A number on the map must equal the same number in Grafana and
  in VictoriaMetrics, within one scrape interval.
- **The map is a *view*, not a collector.** It must not scrape services, hold credentials to
  service `/metrics`, or invent a parallel path. (Customer-data isolation + blast radius.)
- **Reuse what exists.** dashboard-backend already aggregates metrics + eBPF flows + logs +
  events and already serves `/viz/`. No new long-running service should be required to ship.
- **One identity space.** Map node ⇔ dashboard service ⇔ VM `service=` label ⇔ Wotan
  discovery ⇔ Sophia service identity must share one canonical join key.
- **Reliability by construction** (per the Unheaded checklist): fail fast (timeouts/circuit
  breakers), recover automatically (health checks/retries/backoff), eventual consistency
  (idempotent event apply), know why it broke (Monad-tagged distributed tracing).

---

## Considered Options

### For "where the map pulls from"

**Option A — Map scrapes/queries services directly.**
Map fetches each service `/metrics` and the eBPF endpoints itself.
*Rejected:* a fourth scrape loop; new auth surface; guarantees drift; violates "view, not collector."

**Option B — Map queries VictoriaMetrics (PromQL) directly.**
Map talks to VM for metrics and to Wotan for flows.
*Partial:* metrics would match Grafana, but the map would re-implement flow/event/health
fusion that dashboard-backend already does, and would need its own Wotan client + auth. Two
read paths to maintain; topology/health logic forks from the dashboard UI.

**Option C — Map binds to the dashboard-backend read-plane. ✅ CHOSEN**
Map consumes only dashboard-backend HTTP + WS. dashboard-backend remains the single
aggregation point; the map is a pure view over the *same* aggregate the dashboard UI uses.

### Sub-decision — "what does dashboard-backend use for metrics?"

**C1 — Keep the in-process scraper.** *Rejected long-term:* this is the drift source.
**C2 — dashboard-backend queries VictoriaMetrics. ✅ CHOSEN (phased).**
Replace the internal scraper with PromQL queries to VM (`/api/v1/query`, `/query_range`).
Then dashboard UI, map, and Grafana all derive metrics from the **same VM store**.

---

## Decision

1. **One read-plane for live operational views: the dashboard-backend API.**
   The topology map binds **only** to dashboard-backend:
   `GET /api/v1/aggregated`, `/api/v1/flows`, `/api/v1/services`, `/api/v1/health`,
   `/api/v1/hosts`, `/api/v1/stats`; `WS /api/v1/stream` (filtered live metrics) and
   `WS /ws/logs` (Chronicle / Event Log). The map never scrapes a service.

2. **One metrics system-of-record: VictoriaMetrics.**
   dashboard-backend stops re-scraping `/metrics` and instead **queries VM** (PromQL). VM is
   the single scrape pipeline. Grafana and the map then show identical metric values.

3. **One event/flow system-of-record: Wotan.**
   Flows, packet stamps, latency, and agent activity (chaos-controller, lich-security,
   zhen-agent) all arrive via Wotan topics, surfaced by dashboard-backend. The map's three
   agents bind to the event stream filtered by source — they stop being simulated.

4. **One identity space: the canonical service name.**
   The join key across map node / `/api/v1/services` / VM `service=` label / Wotan discovery /
   Sophia identity is the service name. Port maps in scrape configs must be reconciled to it.

5. **Serve the map from dashboard-backend `/viz/`.**
   Drop the HTML into the configured `VizDir` (`server.go` already mounts `/viz/` and
   `/dashboard/` via `http.FileServer`). Same-origin with the API → no CORS, shared
   auth/mTLS, no new service.

6. **Consolidate the metrics plane (follow-up).**
   VM is canonical. Prometheus (`monitoring/`) either `remote_write`s into VM or is retained
   only for host/routing exporters (node/frr/bird) that *also* land in VM. One Grafana, one store.

### Principle: fan out from one source — do not scrape per consumer

"Single source" vs. "scraper" is a false choice: **the scraper is the single source.** Pull
`/metrics` exactly once (VictoriaMetrics) and ingest the data plane exactly once (eBPF →
`trace-collector` → Wotan). Every consumer then **fans out by reading** that source. No view,
no service, and not the map runs its own collector.

| | Fan-out from one source ✅ | Per-consumer scrapers ❌ (today) |
|---|---|---|
| Scrape load per service | 1× (VM) | N× (VM + Prometheus + dashboard-backend + map…) |
| Value consistency | identical everywhere | drifts by cadence / retention / timing |
| Failure domain | one HA'd source (VM cluster; Wotan active-active per ADR-064) | unbounded distributed inconsistency |
| Cost of a new consumer | add a reader | add and maintain another scrape loop |
| Identity / labels | one canonical set | each loop re-derives (today's port drift) |

The data plane already does this correctly: eBPF ring buffer → `trace-collector` → **Wotan** →
many subscribers. Mirror it for metrics: services → **VictoriaMetrics** (one scrape) → many
readers. Read fan-out may be **layered** (VM → dashboard-backend read-plane → UI + map; VM →
Grafana directly) — that's fine, because *ingest stays single*. Decouple read fan-out from
ingest so a slow or greedy view can never backpressure the scrape (cap `/api/v1/stream`
clients, cache aggregated reads).

### …then fan in to a sink (durable retention, same source)

Live fan-out answers *"what's happening now."* For *"what happened,"* each stream also **fans
in to a durable sink** — and the sink is a **downstream consumer of the same single source**,
never a parallel collector. That invariant keeps **live == historical**: the map's live number
and the recorded number share one origin.

| Data class | Single source (live) | Durable sink (history) |
|---|---|---|
| Metrics | VictoriaMetrics (one scrape) | **VictoriaMetrics TSDB** — the source *is* the sink (retains; PromQL for history) |
| Flows / events | Wotan bus (`ebpf.*`, events) | **Anamnesis → Postgres "The Well"** (`unheaded_ops` Kenoma + audit) |
| Logs | Wotan `logs.*` → `logagg` (live tail) | **Vector → ClickHouse** (`unheaded_logs`); Loki is a duplicate to consolidate |
| State | control plane / Wotan | **Postgres** Pleroma (desired) / Kenoma (actual) |

```
                          ┌── FAN-OUT (live) ─► dashboard-backend ─► Dashboard UI + Topology map
 ONE SOURCE ─────────────►┤                  ─► Grafana (PromQL)
 (VM scrape / Wotan bus)  └── FAN-IN → SINK ─► VM TSDB · ClickHouse · Anamnesis → The Well
```

**Reconcile before trusting history:** today the durable log sink (`Vector` ← container
stdout, `config/vector/vector.yaml`) collects *independently* of the live log path
(`Wotan logs.*` → `logagg`). Same disease as the triple metrics scrape, time-shifted — the
live tail and ClickHouse can disagree. Hang the sink off the **same** source (a
`Wotan logs.* → ClickHouse` sink) so tailed and stored logs are one truth.

### Target data flow

```
ORIGINS (systems of record)         STORES                 READ-PLANE                 VIEWS
---------------------------         ------                 ----------                 -----
eBPF data plane (Monad-tagged)
   trace-collector ──► Wotan ─┐
                              ├─► Wotan topics ──────┐
service /metrics (exposition) │   ebpf.*.events,     │
   └──► VictoriaMetrics ◄─────┘   logs.*, events     ├─► dashboard-backend ─┬─► Dashboard UI
            │  (single scrape, PromQL)               │   /api/v1/aggregated │
            │                                         │   /api/v1/flows,     ├─► Topology map (/viz/)
   Postgres "The Well" (ops/config, health history) ──┘   /hosts, /stream,  │
            │                                              /ws/logs          │
            └──────────────────────────────────────► VictoriaMetrics ───────┴─► Grafana (PromQL)
                                                          + Loki (logs)
```

Everything a live view shows traces back to **VictoriaMetrics (metrics)** and **Wotan
(flows/events)**, joined on the **canonical service name**. The map and the dashboard UI read
the *identical* aggregate; Grafana reads the same VM store.

---

## Build & Wire Plan

### Phase 0 — Serve & bind read-only (no backend changes)
- Set `VizDir` and place the map at `<VizDir>/topology.html` → served at
  `http://<dashboard-backend>:20000/viz/topology.html`.
- Replace `stepSim()` with an `adapter` module:
  - **Bootstrap:** `GET /api/v1/aggregated` (services + health), `GET /api/v1/flows` (edges),
    `GET /api/v1/hosts` (WEST/EAST selector → makes the EAST outpost real).
  - **Live:** `WS /api/v1/stream` (metrics → pps/p99/err/flows tiles), `WS /ws/logs`
    (→ Chronicle / Event Log).
  - Map node id = `service` from `/api/v1/services`. Health ring = `/api/v1/health`.
- Reliability: 3s fetch timeout + circuit breaker; WS auto-reconnect w/ exponential backoff;
  idempotent `applyEvent(evt.id)`; show a "STALE" badge if no frame in N seconds.
- **Acceptance:** map nodes/edges/health/log match the dashboard UI 1:1 (same backend).

### Phase 1 — Agents become real (no backend changes)
- Bind the three agents to `WS /api/v1/stream` / `GET /api/v1/events` filtered by `source`:
  - Chaos (Yaldabaoth) ⇐ `chaos-controller` fault-injection events.
  - Adversary (Lich) ⇐ `lich-security` / shield WAF events.
  - AI-Ops (ZhenAI) ⇐ `zhen-agent` remediation events.
- Demo buttons switch from "simulate" to "trigger via API" where an action endpoint exists,
  else become event-driven only.
- **Acceptance:** triggering a real chaos experiment animates the corresponding hold.

### Phase 2 — Kill the metrics divergence (backend change)
- In `cmd/dashboard-backend`: replace `internal/scraper` polling with a VM client querying
  `GET {VM}/api/v1/query` and `/query_range`; keep the same `/api/v1/metrics*` response shape.
- Feature-flag (`METRICS_SOURCE=victoria|scraper`) for safe rollout; remove scraper after burn-in.
- **Acceptance (the "same source" test):** for a fixed metric+service,
  `map value == dashboard /api/v1/metrics == Grafana panel == VM PromQL`, within one scrape
  interval. Document the check in `runbooks/`.

### Phase 3 — Consolidate stores & identity (infra)
- Reconcile port/identity drift between `config/victoria/scrape.yml` and
  `monitoring/prometheus/prometheus.yml`; one canonical `service` label per service.
- Prometheus `remote_write` → VM (or retire duplicate jobs); single Grafana datasource of record.
- Optionally promote the map into `cmd/dashboard-backend/static/` as a first-class view and add
  a Grafana text/link panel so operators land on the same map.

---

## Consequences

**Positive**
- One number, everywhere. Map = dashboard = Grafana = VM. The map can be trusted on-call.
- The map ships in Phase 0 with **zero backend changes** (pure view over existing endpoints).
- Same-origin under `/viz/` inherits auth, mTLS, CORS, rate-limiting already in dashboard-backend.
- Agents reflect real chaos/security/remediation, not theatre.

**Negative / risks**
- dashboard-backend becomes a **critical read-plane** → needs HA, response caching, and
  backpressure on the WS fan-out. Mitigate: cache aggregated reads; cap stream clients
  (`/api/v1/stream` already tracks client count).
- Phase 2 migration risk (scraper → VM): mitigate with the `METRICS_SOURCE` flag + burn-in +
  the parity test as a gate.
- VM/Loki/Wotan outage now visibly degrades the map → must **fail soft** (STALE badge, last-known
  state, no spinner-of-death).

**Neutral**
- Postgres "The Well" remains the durable state-of-record; the read-plane stays in-memory/stream
  for live views and queries The Well/VM for history.

---

## Definition of Done (verification)

- [ ] Map served from `/viz/`, binds only to dashboard-backend (no direct service scrape).
- [ ] Node/edge/health/log parity with the dashboard UI.
- [ ] Parity test green: map == dashboard == Grafana == VM for a sampled metric.
- [ ] One agent path proven end-to-end (real chaos experiment → hold animates).
- [ ] Graceful degradation verified (kill a feed → STALE, no crash).
- [ ] Port/identity drift between scrape configs reconciled (or ticketed with owner).

---

## References

- **ADRs:** ADR-005 (Wotan message backbone), ADR-001 (Gnostic state), ADR-003 (eBPF Rust/Aya),
  ADR-016 (Postgres "The Well"), ADR-034 (gRPC + mTLS default transport), ADR-006 (Vanilla JS frontend).
- **Code:** `cmd/dashboard-backend/internal/{server,scraper,metrics,ebpf,logs,events}`,
  `pkg/{logagg,wotan-client,observability,telemetry}`, `cmd/trace-collector-go`.
- **Config:** `config/victoria/scrape.yml`, `config/grafana/provisioning/datasources/victoriametrics.yml`,
  `monitoring/prometheus/prometheus.yml`, `monitoring/grafana/provisioning/datasources/datasources.yaml`,
  `config/vector/vector.yaml`.
- **Artifact:** `demos/unheaded-topology-enterprise.html` (the view being wired).

*Add to `docs/adr/ADR-INDEX.md` on acceptance.*
