# Wotan — Message Bus / Ring Buffer / Protocol RAM

Core inter-service communication layer. Wotan serves a triple role:

1. **High-Speed Ring Buffer** — Lock-free eBPF perf_event pattern
2. **Event Bus** — Pub/sub gRPC/HTTP with topic routing
3. **Protocol RAM** — BPF map memory substrate for Monad compute

## Cluster Model

DNS-like hierarchy with subscription mirroring:
- Root → Regional → Edge Wotans
- BGP-style topic advertisement
- n+1 availability, not consensus
- Scale: 1 (dev) → 1000 (global)

## Running

| | |
|---|---|
| Container | `unheaded-wotan` (compose service `wotan`, live on west) |
| Ports | 18000 HTTP, 18001 gRPC |
| Source | `services/wotan/` (in this repo) |

## Behaviour worth knowing

- **Each topic holds at most 10,000 messages and 8 MiB of payload** (`--buffer-size`, `--buffer-bytes`); past either, the oldest go first. The count alone let one trace topic (~32 KB batches) reach 320 MB, and Wotan was OOM-killed at its 256 MiB limit within 30 minutes (2026-10-18). Per-topic fill: `unheaded_chat_room_buffer_bytes`. A reader that falls further behind than the buffer holds sees the gap as a jump in seq.
- **Seqs are global across topics.** HTTP and gRPC paging resume from `after_seq`, so a client picks up where it left off across a Wotan restart.
- **Wildcard topics** over HTTP and gRPC: `alerts.*` (one segment), `logs.#` (one or more segments).
- **Join + approval:** services `POST /api/v1/join`; unknown names wait in `/api/v1/admin/pending` until approved.
- **Topic signing:** `config.*` topics require ML-DSA-65 signatures ([[Wotan Topic Signing|Wotan-Topic-Signing]]).
- **Durable store (optional):** Postgres via `--store-conn-str` (runbooks pass `$WOTAN_STORE_CONN_STR`), with no default and `sslmode=require` against The Well.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health`, `GET /ready` | Health and readiness |
| `GET /metrics` | Prometheus metrics |
| `POST /api/v1/join` | Register a service |
| `GET /api/v1/topics`, `/api/v1/topics/<topic>` | Topics and their messages (`after_seq`, wildcards) |
| `POST /api/v1/messages/send`, `GET /api/v1/messages` | Publish and read |
| `/api/v1/admin/{pending,approve,approve-all,deny}` | Join approval |
| gRPC `:18001` | Streaming publish/subscribe |

---

> **Source:** [services/wotan/](../services/wotan/) · [ADR-005](../docs/adr/ADR-005-wotan-message-backbone.md) · [ADR-064](../docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md) (active/active, proposed)
