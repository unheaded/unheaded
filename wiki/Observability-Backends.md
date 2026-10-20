# Observability Backends — The All-Seeing Eye

Unheaded emits OpenTelemetry-compatible signals (metrics, logs, traces). Users plug in their preferred observability stack via interchangeable output adapters. Same pattern as [[Containers]] and [[IaC Backends|IaC-Backends]] — your tools, our data model.

## Supported Backends

| Category | Drop-In Backends | Unheaded Default (Future) |
|----------|-----------------|--------------------------|
| **Metrics** | Prometheus, Grafana, Datadog, InfluxDB, Nagios | Custom Wotan metrics store |
| **Logging** | ELK (Elasticsearch/Logstash/Kibana), Fluentd/Fluent Bit, Flume, Splunk, Loki, Graylog | Custom Wotan log aggregator |
| **Tracing** | Jaeger, Zipkin, Tempo, Datadog APM | Custom eBPF-native tracer |
| **Alerting** | Grafana Alerting, PagerDuty, OpsGenie, Nagios, Prometheus Alertmanager | Custom Wotan alert engine |
| **Dashboards** | Grafana, Kibana, Datadog, custom | Unheaded Dashboard (vanilla JS) |
| **SIEM** | Elastic SIEM, Splunk Enterprise Security, Wazuh | Custom Wotan SIEM integration |

## What runs today (west, 2026-10-18)

| Piece | State |
|---|---|
| `unheaded_http_*` metrics on every service | live (`httputil.ServiceMetrics`) |
| [[Service-Huginn]] host metrics, The Well, backup textfiles | live on `:9110/metrics`; its push to VictoriaMetrics fails (not running) |
| VictoriaMetrics, Grafana | in `docker-compose.yml`, running when the stack is up (Victoria scrapes 10 targets). Victoria's admin endpoints (delete_series, snapshots, force merge, cache reset, reload, pprof, flags) need the key volume-init makes: `docker exec unheaded-victoria cat /victoria-auth/admin.key`, passed as `?authKey=`; writes, queries and `/metrics` stay open |
| ClickHouse | running with the stack; its own system log tables trimmed to fit 2G (`config/clickhouse/system-logs.xml`: text/trace/metric logs off, the rest 7-day TTL) |
| Vector → ClickHouse logs | **Host journal: built, not yet deployed** (ADR-092 step 8): Vector reads `/var/log/journal` read-only through the `adm` group into `unheaded_logs.logs` (audit rows kept 455 days, the rest 90), schema applied by `clickhouse-schema`. Alerts: `monitoring/prometheus/rules/logs.yml` (JournalNotShipping, JournalShipErrors, VectorDown) on Vector's metrics, `host.docker.internal:9598`. **Container stdout does not ship**: open choice between the journald logging driver and a read-only socket proxy. East has no shipper |
| Prometheus alert rules (`monitoring/prometheus/rules/`) | `promtool`-tested in CI and evaluated by the monitoring Prometheus on west. Only SLO error-budget burn notifies (ADR-100): policy, urgency labels, the capped mailer and how to switch it from dry-run to Gmail are in [[Alerting]]. Alertmanager's port is loopback-only (its API takes silences without auth); `monitoring/.env` needs `GRAFANA_ADMIN_PASSWORD` (the stack refuses to start without it) |
| Logs | zerolog → Wotan `logs.<service>.<level>` → dashboard live tail ([[Log-Aggregation]]) |
| Dashboard | live on `:20000`: overview (req/s, 5xx, p95), flow graph, logs, compliance |

## How It Works

```
eBPF Data Plane → Wotan Ring Buffer → ObservabilityAdapter interface
                                              │
                                              ├── prometheus/    → scrape configs, recording rules
                                              ├── grafana/       → dashboard JSON, datasources
                                              ├── elk/           → Logstash pipelines, Kibana index patterns
                                              ├── fluentd/       → Fluent Bit configs, parsers
                                              ├── jaeger/        → collector config, trace export
                                              ├── nagios/        → check configs, NRPE integration
                                              ├── flume/         → agent configs, channel definitions
                                              ├── loki/          → Promtail config, log labels
                                              └── alertmanager/  → alert rules, routing trees
```

Each adapter is a config generator — it consumes Unheaded's signal schema and produces valid, working configs for the target tool. No code changes needed. Drop in, start collecting.

## CLI Usage

```bash
# Generate Prometheus scrape config + Grafana dashboards
unheaded observe --backend=prometheus,grafana   # cmd/unheaded-cli --output=./observability/

# Generate ELK stack configs
unheaded observe --backend=elk --output=./observability/

# Generate everything
unheaded observe --backend=all --output=./observability/
```

## Phased Roadmap

**Phase 1 — Adapter Configs (Alpha/Beta):** Drop-in config mirrors so users hook in their own Grafana, ELK, Prometheus, etc. Pure config generation, no custom tooling.

**Phase 2 — Wotan-Native Defaults (Release):** Purpose-built replacements leveraging the eBPF data plane. Wotan's ring buffer feeds directly into custom metrics/log/trace stores — zero serialization overhead, wire-speed observability.

**Phase 3 — Full Suite (Scale):** Unheaded's own Grafana/ELK/Jaeger-class tools. Built from scratch, tailored to the protocol, integrated with Monad/Sophia/Wotan at the packet level.

## Licensing

All supported backends are open source (Apache-2.0, MIT, GPL, or similar). Unheaded adapter configs are MIT-licensed. See [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md) for attribution.

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · [references/timeline.md](../references/timeline.md)
