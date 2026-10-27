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

## Host and Container Logs (Vector)

The Wotan log bus above is the live view. The durable copy is the journal, shipped by Vector:

```
west: host journal + first-party container stdout (compose journald driver)
      → Vector (compose service vector) → ClickHouse unheaded_logs.logs
                                        → Prometheus/VictoriaMetrics (log volume, auth failures)
east: host journal → Vector (host unit vector-east) → west's ClickHouse over 192.168.13.2
```

- **Compose driver**: the twelve first-party services log with `driver: journald` (anchor `x-logging-journald` in `docker-compose.yml`). Vector labels each line `unit = CONTAINER_NAME`, e.g. `unheaded_log_lines_total{unit="unheaded-wotan"}`. Third-party images and Vector itself keep json-file 3x10MB.
- **Cap**: journald `SystemMaxUse=1G`, `RuntimeMaxUse=200M` (compliance fix `journald-caps`). `scripts/check-compose-log-caps.sh` accepts journald only while every host's profile applies that fix.
- **Tests**: `scripts/test-vector-config.sh` validates both configs, runs `config/vector/tests/`, and fails if `vector-east.yaml`'s transforms differ from `vector.yaml`'s.
- **Decision record**: ADR-092 (amended 2026-10-27). ADR-086 Muninn is superseded by Vector; ADR-076's topology map is re-scoped to one dashboard-backend endpoint.

### East shipping (host window)

Prepared, not installed. Needs ClickHouse on west reachable at `192.168.13.2:8123` from east only.

```bash
# on east, from a checkout
source scripts/lib/tool-pins.sh
curl -fsSL -o /tmp/vector.deb "$VECTOR_DEB_URL"
echo "$VECTOR_DEB_SHA256  /tmp/vector.deb" | sha256sum -c -
sudo apt-get install -y /tmp/vector.deb
sudo systemctl disable --now vector.service   # the package unit uses its own vendor account and /etc/vector; ours replaces it
sudo install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/ && sudo systemd-sysusers
sudo install -d -m 0750 /etc/unheaded/vector
sudo install -m 0644 config/vector/vector-east.yaml /etc/unheaded/vector/
sudo install -m 0600 /dev/null /etc/unheaded/vector/vector-east.env   # add CLICKHOUSE_PASSWORD=...
sudo cp deploy/systemd/vector-east.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now vector-east
```

Check: `systemctl status vector-east`, then from west `curl -s http://192.168.13.1:9598/metrics | grep -c vector_component_sent_events_total`.
