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

## Dashboard Endpoints

- `GET /api/v1/logs` — Query logs with filters (service, level, search, from, to, limit, offset)
- `SSE /ws/logs` — Live tail via Server-Sent Events (filtered by service/level)
- `GET /logs` — Log viewer UI

## Topic Format

`logs.<service>.<level>` — e.g., `logs.timeguru.info`, `logs.captain.error`

## Integration

Services with zerolog: full hook wiring (timeguru, architect, micromanager, trace-collector)
Services with stdlib logger: publisher created for future upgrade
