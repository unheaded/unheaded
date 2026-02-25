# Unheaded Telemetry Stack

## 1. Architecture

```
Host-A & Host-B (data sources)
├── node_exporter :9100        → CPU, RAM, disk IOPS, net, process count
├── rocm_smi_exporter :9400    → GPU util, VRAM, temp, power (Host-A only)  
├── vllm /metrics :20101       → tok/s, queue depth, latency (Host-A only)
├── unheaded services /metrics → per-service latency, error rate, queue depth
└── eBPF ring buffer           → per-packet latency, drop rates, XDP action counts
      └── ebpf-exporter :9435  → converts BPF maps → Prometheus metrics

Host-A (collector)
├── Prometheus :9090           → scrapes all exporters (both hosts via WireGuard)
├── VictoriaMetrics :8428      → receives from Prometheus remote_write, 90-day retention
├── Loki :3100                 → receives structured logs from promtail (both hosts)
└── Grafana :3000              → unified dashboard (queries VM + Loki)
```

## 2. node_exporter Configuration

Collectors to enable (non-default ones matter):

```bash
node_exporter \
  --collector.cpu \
  --collector.cpufreq \
  --collector.diskstats \
  --collector.filesystem \
  --collector.meminfo \
  --collector.netdev \
  --collector.netstat \
  --collector.processes \
  --collector.perf \
  --collector.interrupts \
  --collector.buddyinfo \
  --collector.sockstat \
  --collector.pressure \
  --web.listen-address=:9100
```

Key metrics to alert on:
- `node_pressure_cpu_waiting_seconds_total` — PSI CPU pressure (>0.1 = saturated)
- `node_memory_MemAvailable_bytes` — not MemFree. Available is what matters.
- `node_disk_io_time_seconds_total` — IOPS saturation (rate > 0.8 = saturated)
- `node_network_receive_drop_total` — packet drops at NIC (should be 0)
- `node_procs_running` — runnable processes (sustained >ncores = CPU saturated)
- `process_open_fds` / `process_max_fds` — FD leak detection

## 3. Prometheus Configuration

Write the exact prometheus.yml:

```yaml
global:
  scrape_interval: 5s
  evaluation_interval: 5s
  external_labels:
    cluster: unheaded-poc

remote_write:
  - url: http://localhost:8428/api/v1/write
    queue_config:
      max_samples_per_send: 10000
      capacity: 100000

scrape_configs:
  - job_name: node_host_a
    static_configs:
      - targets: ['localhost:9100']
        labels:
          host: host-a
          role: forge

  - job_name: node_host_b
    static_configs:
      - targets: ['fd00:dead:beef:wg::b:9100']
        labels:
          host: host-b
          role: outpost

  - job_name: unheaded_services_a
    metrics_path: /metrics
    static_configs:
      - targets:
        - 'localhost:16667'
        - 'localhost:16669'
        - 'localhost:16681'
        labels:
          host: host-a

  - job_name: unheaded_services_b
    metrics_path: /metrics
    static_configs:
      - targets:
        - '[fd00:dead:beef:wg::b]:16667'
        - '[fd00:dead:beef:wg::b]:16669'
        - '[fd00:dead:beef:wg::b]:16681'
        labels:
          host: host-b

  - job_name: ebpf_host_a
    static_configs:
      - targets: ['localhost:9435']
        labels:
          host: host-a

  - job_name: rocm_host_a
    static_configs:
      - targets: ['localhost:9400']
        labels:
          host: host-a

  - job_name: vllm_host_a
    metrics_path: /metrics
    static_configs:
      - targets: ['localhost:20101']
        labels:
          host: host-a
```

## 4. VictoriaMetrics Configuration

Why VictoriaMetrics over Thanos or Cortex:
- Single binary, no object storage required for PoC
- ~7x compression vs Prometheus TSDB
- Handles 5s scrape interval at 2-host scale with <100MB RAM
- Remote write compatible — drop-in from Prometheus perspective

```bash
victoria-metrics \
  -storageDataPath=/var/lib/victoria-metrics \
  -retentionPeriod=90d \
  -httpListenAddr=:8428 \
  -search.maxQueryDuration=60s \
  -search.maxConcurrentRequests=4
```

Disk estimate: 2 hosts × ~500 metrics × 5s interval × 90 days × ~1.5 bytes/point ≈ 11.7 GB. Use 20GB partition with headroom.

## 5. eBPF Metrics Exporter

The Unheaded eBPF programs write to BPF maps. Bridge from BPF maps to Prometheus via `cmd/ebpf-exporter/main.go`:

```
Every 5 seconds:
1. Read XDP_ACTION_COUNT map → counter by (ifname, action: XDP_PASS/DROP/TX/REDIRECT)
2. Read LATENCY_HISTOGRAM map → histogram buckets by (hop_count)
3. Read MONAD_REGISTER_COUNT map → counter by (service_id, status_code)
4. Expose as Prometheus metrics on :9435
```

Metric names:
- `unheaded_xdp_action_total{ifname, action}` — counter
- `unheaded_packet_latency_ns_bucket{le, hop}` — histogram
- `unheaded_monad_register_total{service_id, status}` — counter
- `unheaded_bpf_map_entries{map_name}` — gauge

Implementation uses `cilium/ebpf` library (already in go.mod).

## 6. Loki + Promtail for Log Aggregation

All Unheaded services use zerolog → structured JSON logs to stdout. Promtail captures and ships to Loki.

promtail config (each host):

```yaml
server:
  http_listen_port: 9080

clients:
  - url: http://fd00:dead:beef:a::3100/loki/api/v1/push

scrape_configs:
  - job_name: unheaded
    static_configs:
      - targets: [localhost]
        labels:
          host: __HOSTNAME__
          job: unheaded
    pipeline_stages:
      - json:
          expressions:
            level: level
            service: service
            trace_id: trace_id
      - labels:
          level:
          service:
      - timestamp:
          source: time
          format: RFC3339Nano
```

Loki config (Host-A):

```yaml
auth_enabled: false

server:
  http_listen_port: 3100

ingester:
  chunk_idle_period: 1h
  max_chunk_age: 1h

schema_config:
  configs:
    - from: 2026-01-01
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h

storage_config:
  boltdb_shipper:
    active_index_directory: /var/lib/loki/boltdb-shipper-active
    cache_location: /var/lib/loki/boltdb-shipper-cache
    shared_store: filesystem
  filesystem:
    directory: /var/lib/loki/chunks

limits_config:
  retention_period: 90d
  ingestion_rate_mb: 16
  max_streams_per_user: 10000
```

## 7. Grafana Dashboards

Pre-provision these dashboards (JSON in `monitoring/grafana/dashboards/`):

1. **Host Overview** — CPU per-core heat map, RAM available, disk IOPS, net throughput
2. **Unheaded Services** — per-service P99 latency, error rate, request rate
3. **eBPF Pipeline** — XDP action counts, packet latency distribution, BPF map pressure
4. **LLM Inference** — tok/s, queue depth, VRAM used, GPU utilization
5. **WireGuard Bridge** — RTT, throughput, handshake age
6. **Process Audit** — process count by host, goroutine count, FD count, OOM events

## 8. Alert Rules

Write Prometheus alert rules for `monitoring/alerts/bare_metal.yml`:

```yaml
groups:
  - name: unheaded_bare_metal
    rules:
      - alert: HighMemoryPressure
        expr: node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes < 0.15
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Host {{ $labels.host }} memory < 15% available"

      - alert: DiskIOSaturation
        expr: rate(node_disk_io_time_seconds_total[1m]) > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Host {{ $labels.host }} disk I/O saturated"

      - alert: XDPDropRateHigh
        expr: rate(unheaded_xdp_action_total{action="XDP_DROP"}[1m]) > 100
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "XDP dropping >100 packets/sec on {{ $labels.host }}"

      - alert: VLLMThroughputLow
        expr: vllm:generation_tokens_total:rate5m < 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "vLLM throughput dropped on {{ $labels.host }}"

      - alert: WireGuardHandshakeStale
        expr: wireguard_latest_handshake_seconds > 180
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "WireGuard handshake stale on {{ $labels.host }}"

      - alert: ServiceDown
        expr: up{job=~"unheaded.*"} == 0
        for: 30s
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.job }} down on {{ $labels.host }}"
```

## 9. Historical Audit Queries

Useful queries for post-incident review.

PromQL:

```promql
# RAM peak in last 24h per host
max_over_time(node_memory_MemAvailable_bytes[24h]) by (host)

# IOPS peak by device
max_over_time(rate(node_disk_reads_completed_total[1m])[24h:1m]) by (device, host)

# XDP drops over time (packet loss audit)
rate(unheaded_xdp_action_total{action="XDP_DROP"}[5m])

# Service error rate 7-day trend
rate(http_requests_total{status=~"5.."}[5m]) offset 7d vs rate(http_requests_total{status=~"5.."}[5m])
```

LogQL:

```logql
# All ERROR logs from all services in last hour
{job="unheaded"} | json | level="error"

# Trace correlation: find all logs for a trace_id
{job="unheaded"} | json | trace_id="<UUID>"

# OOM events
{job="kernel"} |= "Out of memory"
```

## 10. NixOS Module Sketch

For reproducible deployment, wrap this in a NixOS module at `nixos/modules/telemetry.nix`:

```nix
{ config, pkgs, ... }: {
  services.prometheus = {
    enable = true;
    port = 9090;
    scrapeConfigs = [ ... ];
    globalConfig.scrape_interval = "5s";
  };
  services.prometheus.exporters.node = {
    enable = true;
    enabledCollectors = [
      "cpu"
      "diskstats"
      "meminfo"
      "netdev"
      "processes"
      "pressure"
    ];
  };
  services.loki = {
    enable = true;
    configFile = ./loki.yaml;
  };
  services.grafana = {
    enable = true;
    port = 3000;
  };
}
```

## 11. Deployment Checklist

1. **Host-A (Collector)**
   - [ ] node_exporter on :9100
   - [ ] Prometheus on :9090 with prometheus.yml
   - [ ] VictoriaMetrics on :8428 with 20GB /var/lib/victoria-metrics
   - [ ] Loki on :3100 with boltdb-shipper
   - [ ] Grafana on :3000 with pre-provisioned dashboards
   - [ ] Promtail on :9080 configured to scrape /var/log/unheaded/*.log

2. **Host-B (Outpost)**
   - [ ] node_exporter on :9100
   - [ ] Promtail on :9080 forwarding to Host-A Loki
   - [ ] Unheaded services exposing /metrics endpoints
   - [ ] ebpf-exporter on :9435 (if eBPF programs active)

3. **WireGuard Connectivity**
   - [ ] Verify Host-B can reach Host-A on fd00:dead:beef:wg::a (DNS + connectivity)
   - [ ] Test Prometheus scrape via: `curl http://fd00:dead:beef:wg::b:9100/metrics`

4. **Storage**
   - [ ] `/var/lib/victoria-metrics` on 20GB+ partition
   - [ ] `/var/lib/loki` on 10GB+ partition
   - [ ] `/var/lib/prometheus` (short-term only) on 5GB partition

5. **Validation**
   - [ ] Prometheus targets all healthy in http://localhost:9090/targets
   - [ ] VictoriaMetrics shows remote_write bytes in http://localhost:8428
   - [ ] Grafana data source configured: http://localhost:8428 (VictoriaMetrics)
   - [ ] Loki data source configured: http://localhost:3100 (Loki)
   - [ ] Dashboard queries return data (not "no data")

## 12. Rationale Notes

**Why 5s scrape interval?** 
Bare metal warrants tight observability. 5s is aggressive but sustainable at 2-host, ~500-metric scale. Prometheus can handle it. CPU cost is <2% on a modern host.

**Why VictoriaMetrics?**
Long-term storage without external services. Single binary means fewer moving parts on a PoC. Compression is real (11.7 GB vs ~80 GB for raw Prometheus).

**Why Loki, not ELK?**
Loki is label-based, not full-text-indexed. Matches the Prometheus label paradigm. Lighter memory footprint. Perfect for correlated tracing via trace_id label.

**Why eBPF exporter?**
Unheaded already has the BPF programs. This layer just exposes the kernel-generated data via Prometheus, making it queryable and auditable alongside system metrics.

**Why WireGuard labels?**
Host-B is remote. WireGuard assumes full IPv6 connectivity. Labels on scrape configs allow filtering/alerting by physical location (forge vs outpost).

**Historical auditability:**
90-day retention at 5s = ~15.5 million points per metric. VictoriaMetrics compresses this to disk. No Anamnesis coupling needed — both systems feed independent data (eBPF events + telemetry metrics).
