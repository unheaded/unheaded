# Captain — Strategy & Vision Service

Elite executive fusion: CEO + CMO + CFO + CHRO + CPO. Translates engineering milestones into strategic narrative.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check |
| `GET /ready` | Readiness probe |
| `GET /metrics` | Prometheus metrics |
| `GET /api/v1/strategy` | Strategy data |
| `GET /api/v1/vision` | Vision |
| `GET`, `POST /api/v1/decisions` | List (`limit`, `offset`) or record decisions |
| `GET`, `PATCH /api/v1/decisions/{id}` | Read or update one decision |
| Port: 19002 | |

## State

Decisions are one JSON file each under `DATA_PATH` (compose: the `captain-data` volume at `/data`; deb: `/var/lib/unheaded/captain`). Until 2026-10-23 compose mounted a tmpfs at `/data`, so they vanished with the container.

---

> **Source:** [services/captain/](../services/captain/)
