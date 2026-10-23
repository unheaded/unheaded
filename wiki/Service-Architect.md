# Architect — Infrastructure Design Service

Fusion of 4 staff-level architect minds: Systems, Network, Infrastructure, Security.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check |
| `GET /ready` | Readiness probe |
| `GET /metrics` | Prometheus metrics |
| `GET /infrastructure` | Services and decisions |
| `POST /infrastructure/services` | Add a service |
| `GET /network` | Network topology |
| `POST /network/nodes` | Add a network node |
| `GET`, `POST /design` | List or log design decisions |
| Port: 19001 | |

## State

Services, nodes and decisions live in `architect-state.json` under `-data-dir` / `ARCHITECT_DATA_DIR` (compose: the `architect-data` volume at `/data`; deb: `/var/lib/unheaded/architect`). Saves are atomic; a save that fails returns 503 and the change is undone; with no data dir every POST returns 503 (read-only). A damaged file stops the service at startup. Until 2026-10-23 all of it was in memory and a restart lost it.

---

> **Source:** [services/architect/](../services/architect/)
