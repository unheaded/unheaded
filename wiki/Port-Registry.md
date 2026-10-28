# Port Registry — The Doom Range

All Unheaded services use high ports (16666-26666) to avoid conflicts with common dev tools.

## Port Allocation Table

From `pkg/ports/ports.go`:

| Tier | Range | Services |
|------|-------|----------|
| Infrastructure | 16666-16999 | doom-bridge 16666, doom-go-injector 16667, trace-collector 16670 (HTTP) / 16671 (metrics) |
| Control Plane | 17000-17999 | unheaded-daemon 17000 / 17001 (gRPC), CLI server 17010, protocol-api 17100 / 17101 (gRPC) |
| Wotan | 18000-18099 | wotan 18000 (HTTP) / 18001 (gRPC) |
| Core Services | 19000-19099 | timeguru 19000, architect 19001, captain 19002, micromanager 19003, monad 19004, sophia 19005, cuirass 19006, key-mgmt 19007, pqc-verifier 19008, shield 19009, firewall 19010, sword 19011 (was canary), chaos 19012, QoS 19013, backup-mgr 19014, compliance 19015, NFV 19016, version 19017, anomaly 19018, telemetry-agg 19019, unheaded-auth 19020 ([[Service-Auth]]) |
| Health | 19100 | akira |
| Health | 19101 | heimdall-daemon `/metrics` (`-metrics-addr`, default `127.0.0.1`; empty disables) |
| Threat feeds | 19120 | kev-poller (ADR-055); 19110 is the loadbalancer's xdpsync |
| Alerting | 19130 | alert-mailer (ADR-100), `127.0.0.1` in `monitoring/docker-compose.yml` |
| Alerting | 19131 | alert-mailer `/heartbeat/{host}` only (dead-man switch); loopback until `HEARTBEAT_LISTEN` |
| Packages | 18888 | signed apt repo (reprepro) behind the `nginx` sysuser, `deploy/nginx/unheaded-apt.conf`: 127.0.0.1 and 192.168.13.2 only (S-IRON-GATE Phase 14; installs in E3) |
| Applications | 20000-20099 | dashboard-backend 20000, kanban-app 20001, wiki-server 20002 |
| AI | 20100-20106 | vLLM DeepSeek 20100, vLLM Qwen 20101, Qdrant 20102, BGE-M3 20104, Sophia-Eye 20105, AI web UI 20106 |
| Gateway | 21000-21443 | gateway HTTP 21000, HTTPS 21443 (in compose, Traefik's dashboard is on `127.0.0.1:21000`) |
| Tools | 22000 | cert-gen |
| User Apps | 26000-26666 | reserved; UPC tty bridge 26100, upc-cartridgectl metrics 26610 (ADR-108) |

Outside the Doom Range (infrastructure on standard ports):

| Service | Port | Bound to |
|---------|------|----------|
| The Well (PostgreSQL) | 5432 | `127.0.0.1` only, TLS ([[The-Well]]) |
| Huginn | 9110 | host ([[Service-Huginn]]) |
| VictoriaMetrics | 8428 | all |
| Grafana | 3001 | all |
| ClickHouse | 8123 / 9000 | `127.0.0.1` |
| Vector metrics | 9598 | `172.17.0.1` (override `VECTOR_METRICS_BIND`) |
| CoreDNS | 5353 | all (override `COREDNS_PORT`) |
| Traefik edge | 80 / 443 / 50051 | all (override `TRAEFIK_HTTP_PORT`, `TRAEFIK_HTTPS_PORT`, `TRAEFIK_GRPC_PORT`) |
| Prometheus (monitoring stack) | 9090 | all (`monitoring/docker-compose.yml`) |

## Deploy and chaos ports and identities (S-REAL-STEEL, 2026-10-26)

| What | Port | Account / uid (`pkg/uids`) |
|------|------|-----|
| Cuirass (compose / deb) | 19006 / 17000 | 16740 |
| Sword ([[Service-Sword]]) | 19011 (was Canary; Canary folded into Sword) | 16746 (`uids.Sword`); host unit `unheaded-sword` |
| chaos-controller ([[Chaos-Engineering]]) | 19012 (west and east) | `unheaded-chaos`, 16763 (`uids.Yaldabaoth`) |
| steel-echo inside an instance ([[Steel-Echo]]) | 8080 | n/a (LXD container, user `steel`) |
| steel-echo stable endpoint (Cuirass proxy device) | `127.0.0.1:26601` (user-apps band) | n/a |

## Implementation

Port constants are defined in `pkg/ports/ports.go`. Use `ports.DefaultAddr()` for lookups.

Static fallback configuration: `configs/services.yaml`

## Rationale

High ports avoid conflicts with databases (5432, 3306, 6379), web servers (80, 443, 8080), and common dev tools. The "Doom Range" name comes from the project's Doom-over-eBPF demo that occupies port 16666.

---

> **Source:** [pkg/ports/ports.go](../pkg/ports/ports.go) · [docker-compose.yml](../docker-compose.yml)
