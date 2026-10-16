# System Diagram

How the pieces connect: the planes, the hosts, and what is live today. Layer-by-layer design: [[Architecture]]. Ports: [[Port Registry|Port-Registry]].

## Planes

```
 browser ──HTTPS/HTTP3──▶ edge (traefik in compose; HAProxy+nginx design, see Load-Balancers)
                              │
          ┌───────────────────┼──────────────────────────────────────────┐
          │ application       ▼                                          │
          │   dashboard-backend :20000   kanban-app :20001   wiki :20002 │
          │   timeguru 19000 · architect 19001 · captain 19002 ·         │
          │   micromanager 19003 · monad 19004 · sophia 19005            │
          └───────────┬──────────────────────────────────┬───────────────┘
                      │ publish / subscribe              │ SQL (TLS 1.3)
          ┌───────────▼───────────┐          ┌───────────▼───────────┐
          │ Wotan :18000 / 18001  │          │ The Well (Postgres 16)│
          │ topics, ring buffer,  │          │ unheaded_app / _ops / │
          │ protocol RAM          │          │ _config               │
          └───────────▲───────────┘          └───────────▲───────────┘
                      │ traces.*, anamnesis.*            │ health rows
          ┌───────────┴───────────┐          ┌───────────┴───────────┐
          │ trace-collector(s)    │          │ Akira :19100          │
          │ read eBPF ring buffers│          │ health sweep +        │
          └───────────▲───────────┘          │ consensus (ADR-029)   │
                      │                      └───────────────────────┘
          ┌───────────┴──────────────────────────────────────────────┐
          │ kernel: XDP / TC eBPF (shield, hop, firewall, maglev,    │
          │ qos, tracing…)  ·  monad-cpu-ebpf = the UPC              │
          └──────────────────────────────────────────────────────────┘
 control plane: unheaded-daemon :17000 (drift detection + reconcile)
 host metrics:  Huginn :9110 on each host → VictoriaMetrics :8428 → Grafana
```

## Hosts (2026-10-16)

| Host | Role | Live |
|------|------|------|
| west | Dev + test host, compose stack | The Well, kanban-app, dashboard-backend, Wotan, Akira (compose); Huginn (systemd); backup + drill timers. VictoriaMetrics is down, so Huginn's pushes fail (FND-001) |
| east | Staging, bare metal (4 core, 8 GB) | Offsite backup target + daily check; Huginn + VictoriaMetrics units |

West ↔ east is a point-to-point link (192.168.13.1 ↔ 192.168.13.2). West is not up 24/7.

## Request and trace flow

1. Browser → edge → `dashboard-backend` or `kanban-app`.
2. Services publish state changes to Wotan topics ([[Fae Chamber Contracts|Fae-Chamber-Contracts]]); the dashboard streams them to every viewer over WebSocket (live only, no replay).
3. eBPF programs mark and track packets; collectors publish `traces.*` / `anamnesis.*` to Wotan; the dashboard correlates by `trace_id`.
4. Durable state (tasks, audit events, health transitions, compliance evidence, config history) lives in The Well.

## Trust zones

| Zone | Members | Rule |
|------|---------|------|
| External | Browsers, internet | TLS 1.3 only, through the edge |
| Edge / DMZ | traefik / HAProxy, Shield WAF | TLS termination, rate limiting |
| Application | Court services, dashboard, kanban | Talk to each other via Wotan |
| Infrastructure | Wotan, collectors, The Well | Restricted; Well accepts TLS only, per-role `pg_hba` |
| User | The user's app ("the head") | Deny all except the gateway: zero access to Unheaded internals (NIST 800-207 Zero Trust Architecture) |

Internal mTLS is the default design (ADR-034) but not everywhere yet; see [[Security]].

---

> **Source:** [docs/architecture/SYSTEM_DIAGRAM.md](../docs/architecture/SYSTEM_DIAGRAM.md) (2026-02 alpha design) · `docker-compose.yml` · `~/tmp/next.md` live state
