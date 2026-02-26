# Unheaded Kingdom Grafana Dashboards

Comprehensive monitoring dashboards for the Unheaded Kingdom microservices infrastructure, covering host-level metrics, networking, LLM inference, and service health.

## Dashboard Overview

### 1. Host Overview (01_host_overview.json)
**UID:** `unheaded-01`

Tracks infrastructure health across both hosts (Outpost and Forge).

**Panels:**
- CPU Usage % by Host (time series with mean/max)
- Memory Usage % by Host (with thresholds at 75%/90%)
- Disk I/O (read/write bytes/sec)
- Network Throughput (rx/tx bytes/sec)
- Load Average (1m/5m/15m)
- PSI CPU/Memory Pressure Stall (process scheduling delays)
- Process Count (stat panel)
- Uptime (boot time stat panel)

**Key Metrics:**
- `node_cpu_seconds_total` → CPU %
- `node_memory_MemAvailable_bytes` → Memory %
- `node_disk_read_bytes_total` / `node_disk_write_bytes_total`
- `node_network_receive_bytes_total` / `node_network_transmit_bytes_total`
- `node_load1`, `node_load5`, `node_load15`
- `node_pressure_cpu_waiting_seconds_total`, `node_pressure_memory_waiting_seconds_total`

**Thresholds:**
- CPU: Green <70%, Yellow 70-90%, Red >90%
- Memory: Green <75%, Yellow 75-90%, Red >90%
- PSI: Green <50%, Yellow 50-80%, Red >80%

---

### 2. eBPF Pipeline (02_ebpf_pipeline.json)
**UID:** `unheaded-02`

Monitors XDP/TC eBPF packet processing pipeline and kernel-level forwarding performance.

**Panels:**
- XDP Packets/sec by Action (pass, drop, redirect, tx, aborted)
- XDP Drop Rate % (gauge 0-100%)
- Flow Latency P50/P95/P99 (ns)
- Ring Buffer Events/sec
- BPF Map Errors Total (stat)
- eBPF Exporter Status (up/down)

**Key Metrics:**
- `unheaded_xdp_packets_total{action=...}` → XDP action tracking
- Drop rate calculation: `drop_packets / total_packets * 100`
- `unheaded_flow_latency_ns_bucket` → histogram quantiles
- `unheaded_ring_events_total` → perf ring buffer throughput
- `unheaded_bpf_map_errors_total` → BPF errors
- `up{job="unheaded-ebpf-exporter"}` → exporter health

**Thresholds:**
- Drop Rate: Green <50%, Yellow 50-80%, Red >80%
- BPF Errors: Green 0, Yellow ≥1, Red ≥5

---

### 3. WireGuard East-West (03_wireguard.json)
**UID:** `unheaded-03`

Monitors encrypted inter-node (Outpost ↔ Forge) communication.

**Panels:**
- WireGuard Bytes Transferred rx/tx (time series)
- Outpost → Forge RTT (time series, from active probe)
- WireGuard Handshake Latest Timestamp (stat, "X mins ago")
- WireGuard Interface Status (stat: up/down indicator)
- MTU (stat: 1380 bytes)
- Active Peers (stat: count)

**Key Metrics:**
- `node_network_receive_bytes_total{device="wg0"}`
- `node_network_transmit_bytes_total{device="wg0"}`
- `probe_duration_seconds{probe="wireguard_rtt"}` × 1000 → RTT ms
- `unheaded_wireguard_latest_handshake_seconds` → handshake timestamp
- `node_network_up{device="wg0"}` → interface up/down
- `node_network_mtu_bytes{device="wg0"}`
- `unheaded_wireguard_active_peers` → peer count

**Color Coding:**
- Interface status: Green (1) = "Up", Red (0) = "Down"

---

### 4. LLM Inference (04_llm_inference.json)
**UID:** `unheaded-04`

Monitors vLLM inference engine and ROCm GPU utilization.

**Panels:**
- vLLM Tokens/second (throughput)
- VRAM Utilization % (gauge)
- Concurrent Sequences (time series)
- Request Queue Depth (time series)
- Time to First Token (ms histogram: P50/P95/P99)
- GPU/CPU Contention (vLLM CPU % vs System CPU %)

**Key Metrics:**
- `vllm_tokens_generated_total` → tokens/sec (rate 5m)
- `vllm_vram_used_bytes` / `vllm_vram_total_bytes` → VRAM %
- `vllm_concurrent_sequences` → active inference sequences
- `vllm_request_queue_depth` → pending requests
- `vllm_time_to_first_token_ms_bucket` → TTFT histogram
- `process_cpu_seconds_total{job="vllm"}` → vLLM CPU time
- System CPU: `100 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100`

**Thresholds:**
- VRAM: Green <75%, Yellow 75-90%, Red >90%

**Colors:**
- vLLM CPU: Green (#00ff88)
- System CPU: Orange (#ff6b35)

---

### 5. Unheaded Services (05_services.json)
**UID:** `unheaded-05`

Monitors health and performance of all 25 microservices via gRPC instrumentation.

**Panels:**
- Service Health Grid (table: all services with up/down status)
- gRPC Request Rate by Service (requests/sec)
- gRPC Error Rate by Service (errors/sec)
- gRPC Latency P99 by Service (ms)
- Wotan Event Bus Throughput (events/sec)
- Service Restart Count (bar chart: all 25 services)

**Key Metrics:**
- `up{job="unheaded-services"}` → per-service health status
- `grpc_server_handled_total` → RPC count (rate 5m)
- `grpc_server_handled_total{grpc_code!="OK"}` → error count
- `grpc_server_handling_seconds_bucket` → latency histogram (P99 × 1000)
- `unheaded_wotan_event_bus_messages_total` → event bus throughput
- `unheaded_service_restart_count` → restarts per service

**Thresholds:**
- Error Rate: Green 0, Yellow ≥1 err/s, Red ≥5 err/s
- Latency P99: Green <500ms, Yellow 500-1000ms, Red >1000ms
- Restarts: Green 0, Yellow ≥1, Red ≥3

**Color Coding:**
- Service Status: Green (1) = "Up", Red (0) = "Down"

---

## Dashboard Design Standards

All dashboards follow the **Kingdom Dark Theme**:
- Background: `#0a0a0a` (near-black)
- Primary Accent: `#00ff88` (neon green)
- Warning: `#ff6b35` (orange)
- Critical: `#ff2442` (red)
- Info: `#3d9be9` (blue)

### Configuration
- **Datasource:** Prometheus (UID: `prometheus`)
- **Refresh Rate:** 30 seconds
- **Default Time Range:** Last 1 hour (`now-1h` to `now`)
- **Schema Version:** 38 (Grafana v10+)

### Panel Types Used
- **Timeseries:** Host metrics, throughput, latency trends
- **Gauge:** Percentage utilization (CPU, VRAM, drop rate)
- **Stat:** Point-in-time values (process count, uptime, status)
- **Table:** Service health grid
- **Bar Chart:** Restart counts, service performance

---

## Importing Dashboards

### Via Grafana UI
1. Dashboards → Import
2. Upload JSON file
3. Select Prometheus datasource
4. Click Import

### Via API
```bash
curl -X POST http://localhost:3000/api/dashboards/db \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d @01_host_overview.json
```

### Via Provisioning
Copy JSON files to Grafana provisioning directory:
```bash
cp *.json /etc/grafana/provisioning/dashboards/unheaded/
```

---

## Alerting Integration

Recommended alert rules (to be configured in Prometheus):

### Critical Alerts
- Host CPU >90% for 5 min
- Memory >90% for 5 min
- XDP drop rate >80% for 5 min
- Service down (up=0)
- VRAM utilization >90% for 5 min

### Warning Alerts
- Host CPU 70-90% for 10 min
- Memory 75-90% for 10 min
- High gRPC error rate (>5 errors/sec)
- Service restart count ≥3 in 1 hour
- RTT spike >100ms

---

## Troubleshooting

### No data appearing?
1. Verify Prometheus datasource connectivity
2. Check metric names in Prometheus UI: `curl http://prometheus:9090/api/v1/label/__name__/values`
3. Ensure exporters are running and scraping:
   - node-exporter
   - unheaded-ebpf-exporter
   - vllm-metrics (if using LLM)
   - Custom service metrics (gRPC instrumentation)

### Missing panels?
- Verify required metrics exist in Prometheus
- Check datasource UID is correctly set to "prometheus"
- Ensure labels match exporters (instance, job, service, etc.)

### Time synchronization issues?
- Ensure all hosts have NTP synchronized
- Check Grafana server timezone matches data timezone

---

## Export & Alerts

All dashboards are JSON v10.0 compatible and can be:
- Exported to PDF/PNG
- Shared as JSON
- Templated with Grafana variables
- Queried via Grafana API

JSON UIDs for reference:
- `unheaded-01`: Host Overview
- `unheaded-02`: eBPF Pipeline
- `unheaded-03`: WireGuard
- `unheaded-04`: LLM Inference
- `unheaded-05`: Services

---

**Last Updated:** 2026-02-26  
**Project:** Unheaded Kingdom  
**Monitoring Stack:** Prometheus + Grafana v10+
