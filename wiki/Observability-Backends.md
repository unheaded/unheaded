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
unheaded observe --backend=prometheus,grafana --output=./observability/

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

All supported backends are open source (Apache-2.0, MIT, GPL, or similar). Unheaded adapter configs are MIT-licensed. See [[Third Party Licenses|Third-Party-Licenses]] for attribution.

---

> **Source:** [CLAUDE.md](../CLAUDE.md) · [references/timeline.md](../references/timeline.md)
