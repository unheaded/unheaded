# Dashboard Gap Analysis — S41

**Date**: 2026-02-24
**Compared against**: Coroot, Hubble (Cilium), Pixie, Grafana

## What We Have Now

| Feature | Status | Notes |
|---------|--------|-------|
| Flow table (sortable, real-time) | DONE | Overview page, auto-refresh |
| Latency percentiles (P50/P90/P99) | DONE | Per-operation bar charts |
| Event stream (live, filterable) | DONE | WebSocket, topic filtering |
| Service health monitoring | DONE | Multi-service scraper |
| eBPF program stats | DONE | Packets, flows, latency, compute, anamnesis |
| Packet flow diagram | DONE | Canvas-based in /viz/ |
| Doom visualization | DONE | Framebuffer viewport in /viz/ |
| Log viewer | DONE | SSE-based live tail |
| System resource gauges | DONE | CPU, memory, goroutines |
| Dark theme | DONE | Infrastructure engineer standard |

## What Coroot Has That We Don't

| Feature | Coroot Name | Priority | Target Age |
|---------|-------------|----------|------------|
| Service map topology | Service Map | P1 | Age 2 |
| SLO tracking dashboard | SLO Dashboard | P2 | Age 2 |
| Predefined inspections (auto-detect issues) | Inspections | P2 | Age 2 |
| Cost allocation per service | Cost Attribution | P3 | Age 3 |
| Anomaly detection with ML | Anomaly Detection | P3 | Age 3 |
| Log pattern analysis | Log Patterns | P2 | Age 2 |
| Deployment tracking | Deployments | P2 | Age 2 |

## What Hubble Has That We Don't

| Feature | Hubble Name | Priority | Target Age |
|---------|-------------|----------|------------|
| Network policy visualization | Policy Viewer | P2 | Age 2 |
| DNS request tracking | DNS Monitor | P2 | Age 2 |
| HTTP flow inspection | L7 Flows | P1 | Age 2 |
| Identity-based flow filtering | Identity Filter | P3 | Age 3 |

## Our Unique Advantages (Not in Competitors)

| Feature | Description |
|---------|-------------|
| Wire-level Monad register file | 20-byte state carried IN the packet |
| eBPF-to-browser pipeline | XDP → TC → kprobe → Wotan → dashboard |
| Doom-over-IPv6 compute | BPF-powered compute in network namespace ring |
| Anamnesis event streaming | Lifecycle event tracking per flow |
| Multi-backend observability | Interchangeable output adapters |
| Zero customer data access | Architectural isolation at every layer |

## Priority Roadmap

### Age 2 (Next)
1. Service map topology (canvas-based, auto-discovered from flow data)
2. L7 HTTP flow inspection (parse HTTP headers in TC program)
3. DNS monitoring (hook into kprobe for DNS resolution)
4. Log pattern analysis (cluster similar log lines)
5. Deployment tracking (detect binary version changes)

### Age 3 (Later)
1. Anomaly detection (statistical baseline + deviation alerts)
2. Cost attribution (per-service resource accounting)
3. Identity-based filtering (Cilium-style identity labels)
4. SLO dashboards (define + track service level objectives)
