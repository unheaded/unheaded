# Wotan — Message Bus / Ring Buffer / Protocol RAM

Wotan is the memory layer of the [Unheaded Protocol](Unheaded-Protocol): the Monad is the 20-byte register file, Wotan is everything bigger. Two tiers:

- **LOCAL**: BPF maps on each Shim (per-flow state, page tables, framebuffers, TTY rings), sub-microsecond from the XDP path.
- **DISTRIBUTED**: gRPC topic streaming between hosts; `config.*` topics are ML-DSA-65 signed.

The same service is the Kingdom's inter-service message bus. It serves a triple role:

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

- **Each topic holds at most 10,000 messages and 8 MiB of payload** (`--buffer-size`, `--buffer-bytes`); past either, the oldest go first. The count alone let one trace topic (~32 KB batches) reach 320 MB, and Wotan was OOM-killed at its 256 MiB limit within 30 minutes (2026-10-18). Per-topic fill: `unheaded_chat_room_buffer_bytes`. **All topics together hold at most 64 MiB** (`--buffer-bytes-total`, content plus ~192 bytes per message): every 250 ms the oldest messages of the largest topics go first. Per-topic budgets alone did not bound Wotan: ~50 log topics of 10,000 small messages each held 290 MiB (OOM-killed again 2026-10-18, reproduced locally). Total: `unheaded_chat_room_buffers_footprint_bytes`, drops: `unheaded_chat_room_buffer_total_evicted_total`. A reader that falls further behind than the buffer holds sees the gap as a jump in seq.
- **At most 10,000 topics** (`--max-rooms`): a publish naming a new topic past it is refused (`ResourceExhausted` over gRPC, 503 over HTTP); existing topics keep working. Topics are never reaped, and any auto-approved publisher creates one by naming it. An unnamed publisher (both gRPC clients send no sender_id) is one member per topic and name, not one per message: until 2026-10-20 every publish created and approved a new member that was never freed (245 bytes each). A stream's subscriber member goes away with the stream. **At most 100,000 members at once** (`--max-members`; ~25 MB at the cap): past it new subscribes and new publishers are refused (ResourceExhausted / 503). Approved members are never reaped; an allowlisted HTTP subscribe reuses the one member for its topic and name (until 2026-10-23 it made a new one per call, so every client restart left its old ones behind). A subscribe under a name not on the allowlist makes a pending member per request, which expires.
- **Seqs are global across topics.** HTTP and gRPC paging resume from `after_seq`, so a client picks up where it left off across a Wotan restart.
- **Wildcard topics** over HTTP and gRPC: `alerts.*` (one segment), `logs.#` (one or more segments).
- **Join + approval:** services `POST /api/v1/join`; unknown names wait in `/api/v1/admin/pending` until approved. The admin endpoints exist only with `--admin` and `WOTAN_ADMIN_API_KEY` in `/etc/unheaded/wotan/wotan.env` (`deploy/systemd/wotan.env.example`); `--admin` without the key exits at startup. Before 2026-10-27 `--admin` was on by default with no key ever set, so every admin call was refused.
- **Services that start before Wotan still subscribe.** `pkg/wotan-client`'s `SubscribeWithRetry` (1 s doubling to 30 s, jitter) is what micromanager, monad, sophia, gateway, architect, anomaly, loadbalancer, timeguru and yaldabaoth use; a client on HTTP polling re-probes gRPC every 30 s and promotes itself back. Tests: `TestLateWotan_*` (fake Wotan from `pkg/wotan-client/wotantest`).
- **Rate limit is per client IP** (`--rate-limit`, 100/s), not per connection: it keyed on IP:port until 2026-10-27, so reconnecting got a fresh bucket.
- **CORS:** with the default allow-list `*` the answer is `Access-Control-Allow-Origin: *` without credentials; a named list echoes only a matching origin, with credentials. Until 2026-10-27 `*` reflected any origin with `Allow-Credentials: true`.
- **`unheaded_chat_streams_active`** counts gRPC topic streams under `room_id="_topic_streams"` as well as legacy room streams (it used to read 0 with every service subscribed).
- **Topic signing:** `config.`, `drift.`, `audit.`, `alerts.critical.`, `system.health.consensus.` and `system.health.remediate.` topics require ML-DSA-65 signatures ([[Wotan Topic Signing|Wotan-Topic-Signing]]); keys are enrolled per deployment.
- **Health topics:** every discovered service reports itself on `system.health.reports` (unsigned: everyone votes); Akira publishes the verdict on `system.health.consensus` and, on CRITICAL or PANIC, one advisory `system.health.remediate` message (signed; nothing acts on it yet).
- **Durable store (optional):** Postgres via `--store-conn-str` (runbooks pass `$WOTAN_STORE_CONN_STR`), with no default and `sslmode=require` against The Well.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health`, `GET /ready` | Health and readiness |
| `GET /metrics` | Prometheus metrics |
| `POST /api/v1/join` | Register a service |
| `GET /api/v1/topics` | List topics |
| `POST /api/v1/topics/<topic>/subscribe`, `POST …/publish`, `GET …/messages` | Topic pub/sub; `messages` pages with `after_seq` and takes wildcards |
| `POST /api/v1/messages/send`, `GET /api/v1/messages`, `POST`/`DELETE /api/v1/messages/delete` | Room (legacy chat) API |
| `GET /api/v1/admin/pending`, `POST /api/v1/admin/{approve,approve-all,deny}` | Join approval (registered only with `--admin` and `WOTAN_ADMIN_API_KEY` set (default off since S-CLEAN-SLATE); send the key as `X-Admin-API-Key` or `Authorization: Bearer`; `--admin` without the key exits at startup) |
| gRPC `:18001` | Streaming publish/subscribe |

---

> **Source:** [services/wotan/](../services/wotan/) · [ADR-005](../docs/adr/ADR-005-wotan-message-backbone.md) · [ADR-064](../docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md) (active/active, proposed)
