# Unheaded Kingdom - Prometheus Metrics Reference

This document maps dashboard panels to their underlying Prometheus metrics.

## 1. Host Overview Dashboard (01_host_overview.json)

### Panel: CPU Usage % by Host
```promql
100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)
```
**Metric:** `node_cpu_seconds_total`  
**Labels:** instance, mode (idle, user, system, iowait, etc.)  
**Type:** Counter

### Panel: Memory Usage % by Host
```promql
(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100
```
**Metrics:** `node_memory_MemAvailable_bytes`, `node_memory_MemTotal_bytes`  
**Type:** Gauge  
**Unit:** bytes

### Panel: Disk I/O (read/write bytes/sec)
```promql
rate(node_disk_read_bytes_total[5m])
rate(node_disk_write_bytes_total[5m])
```
**Metrics:** `node_disk_read_bytes_total`, `node_disk_write_bytes_total`  
**Labels:** device (sda, nvme0n1, etc.)  
**Type:** Counter

### Panel: Network Throughput
```promql
rate(node_network_receive_bytes_total[5m])
rate(node_network_transmit_bytes_total[5m])
```
**Metrics:** `node_network_receive_bytes_total`, `node_network_transmit_bytes_total`  
**Labels:** device (eth0, wg0, etc.), instance  
**Type:** Counter

### Panel: Load Average
```promql
node_load1
node_load5
node_load15
```
**Metrics:** `node_load1`, `node_load5`, `node_load15`  
**Type:** Gauge

### Panel: PSI CPU/Memory Pressure Stall
```promql
node_pressure_cpu_waiting_seconds_total
node_pressure_memory_waiting_seconds_total
```
**Metrics:** `node_pressure_cpu_waiting_seconds_total`, `node_pressure_memory_waiting_seconds_total`  
**Type:** Counter  
**Note:** Requires Linux kernel 4.20+ with PSI enabled

### Panel: Process Count
```promql
node_processes_running
```
**Metric:** `node_processes_running`  
**Type:** Gauge

### Panel: Uptime
```promql
node_boot_time_seconds
```
**Metric:** `node_boot_time_seconds`  
**Type:** Gauge (Unix timestamp)

---

## 2. eBPF Pipeline Dashboard (02_ebpf_pipeline.json)

### Panel: XDP Packets/sec by Action
```promql
rate(unheaded_xdp_packets_total{action="pass"}[5m])
rate(unheaded_xdp_packets_total{action="drop"}[5m])
rate(unheaded_xdp_packets_total{action="redirect"}[5m])
rate(unheaded_xdp_packets_total{action="tx"}[5m])
rate(unheaded_xdp_packets_total{action="aborted"}[5m])
```
**Metric:** `unheaded_xdp_packets_total`  
**Labels:** action (pass, drop, redirect, tx, aborted)  
**Type:** Counter  
**Source:** Custom eBPF exporter

### Panel: XDP Drop Rate %
```promql
(rate(unheaded_xdp_packets_total{action="drop"}[5m]) / rate(unheaded_xdp_packets_total[5m])) * 100
```
**Calculation:** Drops / Total × 100  
**Unit:** percent

### Panel: Flow Latency P50/P95/P99
```promql
histogram_quantile(0.5, unheaded_flow_latency_ns_bucket)
histogram_quantile(0.95, unheaded_flow_latency_ns_bucket)
histogram_quantile(0.99, unheaded_flow_latency_ns_bucket)
```
**Metric:** `unheaded_flow_latency_ns_bucket` (histogram)  
**Labels:** le (bucket boundaries)  
**Type:** Histogram  
**Unit:** nanoseconds

### Panel: Ring Buffer Events/sec
```promql
rate(unheaded_ring_events_total[5m])
```
**Metric:** `unheaded_ring_events_total`  
**Type:** Counter  
**Note:** Tracks perf ring buffer events from BPF programs

### Panel: BPF Map Errors Total
```promql
unheaded_bpf_map_errors_total
```
**Metric:** `unheaded_bpf_map_errors_total`  
**Labels:** error_type (overflow, lookup_failed, update_failed, delete_failed)  
**Type:** Counter

### Panel: eBPF Exporter Status
```promql
up{job="unheaded-ebpf-exporter"}
```
**Metric:** `up`  
**Label:** job  
**Type:** Gauge (1=up, 0=down)

---

## 3. WireGuard East-West Dashboard (03_wireguard.json)

### Panel: WireGuard Bytes Transferred
```promql
rate(node_network_receive_bytes_total{device="wg0"}[5m])
rate(node_network_transmit_bytes_total{device="wg0"}[5m])
```
**Metrics:** `node_network_receive_bytes_total`, `node_network_transmit_bytes_total`  
**Labels:** device="wg0", instance  
**Type:** Counter

### Panel: Outpost → Forge RTT
```promql
probe_duration_seconds{probe="wireguard_rtt"} * 1000
```
**Metric:** `probe_duration_seconds`  
**Labels:** probe (wireguard_rtt)  
**Type:** Gauge  
**Unit:** milliseconds (converted)  
**Source:** Prometheus blackbox exporter

### Panel: WireGuard Handshake Timestamp
```promql
unheaded_wireguard_latest_handshake_seconds
```
**Metric:** `unheaded_wireguard_latest_handshake_seconds`  
**Labels:** public_key (peer identifier)  
**Type:** Gauge (Unix timestamp)

### Panel: WireGuard Interface Status
```promql
node_network_up{device="wg0"}
```
**Metric:** `node_network_up`  
**Labels:** device="wg0"  
**Type:** Gauge (1=up, 0=down)

### Panel: MTU
```promql
node_network_mtu_bytes{device="wg0"}
```
**Metric:** `node_network_mtu_bytes`  
**Labels:** device="wg0"  
**Type:** Gauge  
**Expected Value:** 1380

### Panel: Active Peers
```promql
count(unheaded_wireguard_active_peers)
```
**Metric:** `unheaded_wireguard_active_peers`  
**Type:** Gauge

---

## 4. LLM Inference Dashboard (04_llm_inference.json)

### Panel: vLLM Tokens/second
```promql
rate(vllm_tokens_generated_total[5m])
```
**Metric:** `vllm_tokens_generated_total`  
**Type:** Counter  
**Source:** vLLM metrics endpoint

### Panel: VRAM Utilization %
```promql
(vllm_vram_used_bytes / vllm_vram_total_bytes) * 100
```
**Metrics:** `vllm_vram_used_bytes`, `vllm_vram_total_bytes`  
**Type:** Gauge  
**Unit:** bytes

### Panel: Concurrent Sequences
```promql
vllm_concurrent_sequences
```
**Metric:** `vllm_concurrent_sequences`  
**Type:** Gauge  
**Description:** Number of active inference sequences

### Panel: Request Queue Depth
```promql
vllm_request_queue_depth
```
**Metric:** `vllm_request_queue_depth`  
**Type:** Gauge  
**Description:** Pending requests waiting for processing

### Panel: Time to First Token
```promql
histogram_quantile(0.5, vllm_time_to_first_token_ms_bucket)
histogram_quantile(0.95, vllm_time_to_first_token_ms_bucket)
histogram_quantile(0.99, vllm_time_to_first_token_ms_bucket)
```
**Metric:** `vllm_time_to_first_token_ms_bucket` (histogram)  
**Type:** Histogram  
**Unit:** milliseconds

### Panel: GPU/CPU Contention
```promql
process_cpu_seconds_total{job="vllm"}
100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)
```
**Metrics:**
- `process_cpu_seconds_total` (vLLM process CPU)
- `node_cpu_seconds_total` (system CPU)

---

## 5. Unheaded Services Dashboard (05_services.json)

### Panel: Service Health Grid
```promql
up{job="unheaded-services"}
```
**Metric:** `up`  
**Labels:** job="unheaded-services", instance (per service)  
**Type:** Gauge (1=up, 0=down)

### Panel: gRPC Request Rate by Service
```promql
rate(grpc_server_handled_total{job="unheaded-services"}[5m])
```
**Metric:** `grpc_server_handled_total`  
**Labels:** service, method, grpc_code  
**Type:** Counter  
**Source:** gRPC metrics middleware

### Panel: gRPC Error Rate by Service
```promql
rate(grpc_server_handled_total{job="unheaded-services",grpc_code!="OK"}[5m])
```
**Metric:** `grpc_server_handled_total`  
**Filters:** grpc_code != "OK" (errors only)  
**Type:** Counter

### Panel: gRPC Latency P99 by Service
```promql
histogram_quantile(0.99, rate(grpc_server_handling_seconds_bucket{job="unheaded-services"}[5m])) * 1000
```
**Metric:** `grpc_server_handling_seconds_bucket` (histogram)  
**Type:** Histogram  
**Unit:** milliseconds (converted)

### Panel: Wotan Event Bus Throughput
```promql
rate(unheaded_wotan_event_bus_messages_total[5m])
```
**Metric:** `unheaded_wotan_event_bus_messages_total`  
**Labels:** instance  
**Type:** Counter  
**Source:** Custom event bus exporter

### Panel: Service Restart Count
```promql
unheaded_service_restart_count
```
**Metric:** `unheaded_service_restart_count`  
**Labels:** service  
**Type:** Counter  
**Description:** Total restarts per service

---

## Exporter Configuration

### Node Exporter
Collects from: `/metrics` port 9100  
Provides: node_cpu, node_memory, node_disk, node_network, node_boot_time, node_processes, node_load, node_pressure

### eBPF Exporter
Collects from: `/metrics` port 9500  
Provides: unheaded_xdp_packets, unheaded_flow_latency, unheaded_ring_events, unheaded_bpf_map_errors

### vLLM Metrics
Collects from: `:8000/metrics` (built-in Prometheus endpoint)  
Provides: vllm_tokens_generated, vllm_vram_*, vllm_concurrent_sequences, vllm_request_queue_depth, vllm_time_to_first_token

### gRPC Service Metrics
Collects from: `:9091/metrics` (per-service instrumentation)  
Provides: grpc_server_handled_total, grpc_server_handling_seconds, up

### Blackbox Exporter
Collects from: `/probe` with targets  
Provides: probe_duration_seconds, probe_success for WireGuard RTT

---

## Label Conventions

**Instance Labels:**
- `outpost` - Outpost node (primary)
- `forge` - Forge node (secondary)

**Service Labels:**
- 25 services in total
- Examples: auth-service, data-service, compute-service, etc.

**Job Labels:**
- `unheaded-services` - All microservices
- `unheaded-ebpf-exporter` - eBPF exporter
- `vllm` - LLM inference engine

---

## Query Performance Tips

1. **Use rate() for counters:** `rate(metric[5m])` not just the raw counter
2. **Filter early:** Always use label filters in queries
3. **Adjust time window:** Use `[5m]` for 1-hour dashboards, `[1m]` for real-time
4. **Histogram quantiles:** Always use `histogram_quantile()` for latency metrics
5. **Avoid full table scans:** Include job/instance labels in all queries

---

**Last Updated:** 2026-02-26  
**Prometheus Version:** 2.40+  
**Grafana Version:** 10.0+
