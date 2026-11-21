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

`logs.<service>.<level>` — e.g., `logs.timeguru.info`, `logs.kanban-app.error`

## Integration

Publishing to `logs.<service>.<level>`: timeguru, kanban-app, dashboard-backend, unheaded-daemon, trace-collector-go. architect, captain, micromanager and the monad and sophia services were removed in 2026-11 (ADR-147) and archived in `unheaded-role-services`.

## Host and Container Logs (Vector)

The Wotan log bus above is the live view. The durable copy is the journal, shipped by Vector:

```
west: host journal + first-party container stdout (compose journald driver)
      → Vector (compose service vector) → ClickHouse unheaded_logs.logs
                                        → Prometheus/VictoriaMetrics (log volume, auth failures)
                                        → logstore-writer → the Well, unheaded_ops.log_events (measured week only)
east: host journal → Vector (host unit vector-east) → west's ClickHouse over 192.168.13.2
```

- **Compose driver**: the fourteen first-party services log with `driver: journald` (anchor `x-logging-journald` in `docker-compose.yml`). Vector labels each line `unit = CONTAINER_NAME`, e.g. `unheaded_log_lines_total{unit="unheaded-wotan"}`. Third-party images and Vector itself keep json-file 3x10MB.
- **Cap**: journald `SystemMaxUse=1G`, `RuntimeMaxUse=200M` (compliance fix `journald-caps`), raised on west to 4G/500M (`journald-caps-west`, DP-09). That fix sets its two caps under `hosts: [west]`: applied on east (the `all` profile takes every fix) it writes a file of comments and east keeps 1G/200M; `pkg/compliance/remediation` tests fail a fix named for one host that sets anything on another. `scripts/check-compose-log-caps.sh` accepts journald only while every host's profile applies that fix.
- **Batching**: Vector inserts into ClickHouse once every 30 s (`batch.timeout_secs` on the `clickhouse` sink in `config/vector/vector.yaml`; at most 100,000 rows or 10 MiB a batch), not once a second, so a line is readable in ClickHouse up to 30 s after it was written. The disk buffer holds it meanwhile. One insert a second had made ClickHouse merge 42 GiB a day to keep 127 MiB (ADR-147).
- **Second copy in the Well (S-E1-LIVE Evening 16)**: for one measured week Vector's `well_logs` sink posts west's own redacted journal rows to `logstore-writer` (port 19140, `data` and `observe` networks, no published port), which inserts them into `unheaded_ops.log_events` as the insert-only role `log_writer` (migration 048). ClickHouse stays the record; the Well copy is derived, kept 14 days in daily partitions, and holds no rows relayed from other devices. The sink drops when it cannot deliver (`when_full: drop_newest`), so a slow or dead writer never holds the ClickHouse sink back; the writer refuses past a row cap per minute (429) and a byte ceiling (503). Off switch: remove the `well_logs` sink, `docker compose up -d vector`, `docker compose stop logstore-writer`. The daily reading is `scripts/logstore-measure.sh`: row counts, bytes, query timings and Vector buffer figures for both stores, numbers only, appended to `~/.local/state/unheaded/S-E1-LIVE-logstore.tsv` on the host (week 1) or to the file `LOGSTORE_TSV` names (see "The log store reading" below).
- **Tests**: `scripts/test-vector-config.sh` validates both configs, runs `config/vector/tests/`, and fails if `vector-east.yaml`'s transforms differ from `vector.yaml`'s.
- **Decision record**: ADR-092 (amended 2026-10-27). ADR-086 Muninn is superseded by Vector; ADR-076's topology map is re-scoped to one dashboard-backend endpoint.

### The log store reading (measured weeks)

`scripts/logstore-measure.sh [FILE]` takes one reading of both stores and appends it as `reading_utc<TAB>metric<TAB>value` lines. Read-only on both stores, numbers only, needs `sudo` for docker and for the backup set's size. Exit 0: every value is a number. Exit 1: the block holds an `NA` (a value that could not be read is never written as zero). Exit 2: a bad setting, nothing written. Week 1 (S-E1-LIVE, 2026-11-10 to 11-17) and week 2 (S-LOG-WEEK-2, ADR-147 decision 5) use the same script and the same bar.

What a reading holds, in the order it is taken:

| Metrics | What |
|---|---|
| `{ch,pg}_rows_yesterday`, `{ch,pg}_rows_7d`, `{ch,pg}_bytes`, `{ch,pg}_rows`, `pg_bytes_yesterday` | The row set and its size on disk |
| `well_probe_first_{p50,p95}_ms` | The Well's own latency (one read the Kanban makes) before the three queries load its cache. New in week 2 |
| `{ch,pg}_{q1_last_hour,q2_text_24h,q3_units_7d}_{p50,p95}_ms` and `_runs` | The three queries on each side; `_runs` is how many of the `RUNS` runs finished inside the budget (new in week 2) |
| `well_probe_{p50,p95}_ms` | The same probe after the queries, where week 1 had it |
| `well_calls_total`, `well_exec_ms_total` | `pg_stat_statements` for every statement that does not touch `log_events` (cumulative) |
| `backup_seconds`, `backup_bytes` | The last nightly backup set |
| `vector_record_buffer_events`, `vector_record_buffer_bytes`, `vector_record_sends_over_1s_total`, `vector_well_dropped_total` | From Vector's own page; cumulative since Vector's last start. `vector_well_dropped_total` is `vector_component_discarded_events_total` for the `well_logs` sink: rows the sink itself gave up on |
| `vector_well_buffer_dropped_total` | `vector_buffer_discarded_events_total` for the `well_logs` sink: rows its full buffer turned away (`when_full: drop_newest`), which is what a stopped or slow writer causes and what `VectorSinkDroppingEvents` reads. New in week 2 |
| `vector_record_sends_over_1s_24h`, `vector_well_dropped_24h`, `vector_well_buffer_dropped_24h` | The same three counts as `increase(...[24h])` from VictoriaMetrics, which a Vector restart does not reset. New in week 2. No drop series is read as 0 only while Vector's other series answer; otherwise `NA` |
| `ch_started_unix`, `pg_started_unix`, `vector_started_unix`, `writer_started_unix`, `host_boot_unix` | When each container last started and when the host booted: a value that differs between two readings is a restart between them. `writer_started_unix` is `NA` while the writer is stopped, and that alone does not fail the reading. New in week 2 |
| `io_some_avg300` | `/proc/pressure/io`, `some`, 5-minute average. New in week 2 |

The time budget: each query on each side, and each probe, has `QUERY_BUDGET_S` seconds for its `RUNS` runs. On the Postgres side the deadline is kept inside the psql session and `statement_timeout` is set to what is left before every run, so the server ends a run that would pass it; on the ClickHouse side the client is stopped. The p50 and p95 come from the runs that finished when there are five or more; fewer is `NA`. A slow query is therefore a number with its run count, not a broken reading (the reading of 2026-11-16 held `NA` for a query that took 30 s a run and did not fit the old fixed 600 s).

Settings (environment):

| Setting | Default | What |
|---|---|---|
| `LOGSTORE_TSV` | `~/.local/state/unheaded/S-E1-LIVE-logstore.tsv` | The readings file, when no `FILE` is given. Week 2 passes `~/.local/state/unheaded/S-LOG-WEEK-2-logstore.tsv` |
| `RUNS` | `20` | Runs of each query and probe; 5 or more |
| `QUERY_BUDGET_S` | `900` | Seconds each query (each side) and each probe may take for its runs |
| `QUERY_BUDGET_MIN_S` | `60` | The least `QUERY_BUDGET_S` accepted, so a slip cannot turn every query into `NA` |
| `CLICKHOUSE_CONTAINER`, `WELL_CONTAINER`, `VECTOR_CONTAINER`, `LOGSTORE_WRITER_CONTAINER` | `unheaded-clickhouse`, `unheaded-postgres`, `unheaded-vector`, `unheaded-logstore-writer` | The four containers |
| `VECTOR_METRICS_URL` | `http://172.17.0.1:9598/metrics` | Vector's own metrics page |
| `VICTORIA_URL` | `http://127.0.0.1:8428` | VictoriaMetrics, for the two 24 h counts |
| `WELL_BACKUP_DIR` | `/srv/backups/unheaded-well` | Where the nightly sets are |
| `LOGSTORE_PROC` | `/proc` | Where `/proc` is (the test points it at a made-up one) |

The timer: `deploy/systemd/user/unheaded-logstore-measure.{service,timer}` takes the reading daily at 00:15 UTC as the operator ([[Systemd-Units]]). It is enabled for a measured week and disabled when the week closes. Check it with `systemctl --user status unheaded-logstore-measure.service`; a shorter budget for the rest of a week is a drop-in with `Environment=QUERY_BUDGET_S=300`.

The table against the bar: `scripts/logstore-reckon.py FILE --baseline-until T --start T --end T` prints one Markdown table (test, bar, ClickHouse, the Postgres copy, ratio or movement, verdict) and, under it, the five tests of the bar, the readings used, every `NA` by time and metric, run counts under `--runs`, restarts seen, the highest I/O pressure, the copy's growth against the writer's byte ceiling (`--ceiling-bytes`, default the writer's `--max-bytes` of 4 GiB), and whether a backup ran inside the baseline. The bar's two numbers are options (`--ratio-bar 2`, `--move-bar 10`). The readings before `--baseline-until` are the "before": the Well's probe is compared with their median, the backup with the last of them. The Well's row is decided by the probe before the queries when the file has it. An `NA` is listed and left out of its row, never read as zero.

Tests: `scripts/test-logstore-measure.sh` (the script against stub `sudo`, `docker`, `curl`, `systemctl` and `du`; no container; in `scripts/check-gates-can-fail.sh`), `tests/scripts/test_logstore_reckon.py`, and `scripts/well/test-logstore-budget.sh` (the budget against a throwaway PostgreSQL: needs docker).

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
