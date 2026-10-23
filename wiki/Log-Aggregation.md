# Log Aggregation — The Chronicler's Well

Centralized structured logging via Wotan message bus.

## Architecture

1. Services publish structured JSON logs via `zerolog.Hook`
2. Logs flow to Wotan topic `logs.<service>.<level>`
3. Dashboard subscriber feeds logs into a ring buffer (10K default capacity)
4. Dashboard serves logs via REST API and SSE live tail

## Package: `pkg/logagg/`

- `ringbuffer.go` — Thread-safe circular buffer with query/filter support
- `publisher.go` — zerolog.Hook that publishes to Wotan topics
- `subscriber.go` — Wotan topic listener feeding into ring buffer
- `query.go` — LogEntry, LogQuery types with filtering (service, level, time range, search)
- `setup.go` — `SetupServiceLogger()` one-liner for services
- `loggerhook.go` — adapts a Publisher to `pkg/logger`'s Hook interface
- `metrics.go` — log-forwarding health counters for `/metrics`

## Dashboard Endpoints

- `GET /api/v1/logs` — Query logs with filters (service, level, search, from, to, limit, offset). Results are oldest first; `newest=true` counts limit/offset back from the newest entry (the Logs page and `unheaded service logs` use it). Without it a full buffer answered "the last 500" with its first 500.
- `SSE /ws/logs` — Live tail via Server-Sent Events (filtered by service/level). Live only, no replay on connect. The broadcast loop follows the ring buffer by sequence number; it used to compare lengths, which stop growing at capacity, so the live tail went silent after the first 10,000 entries.
- `GET /logs` — Log viewer UI

## Topic Format

`logs.<service>.<level>` — e.g., `logs.timeguru.info`, `logs.captain.error`

## Integration

Publishing to `logs.<service>.<level>`: timeguru, architect, micromanager, monad, sophia, kanban-app, dashboard-backend, unheaded-daemon, trace-collector-go. Captain has no structured logger yet, so it publishes no logs (`services/captain/cmd/captain/main.go`).
