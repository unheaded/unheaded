# Port Registry — The Doom Range

All Unheaded services use high ports (16666-26666) to avoid conflicts with common dev tools.

## Port Allocation Table

From `pkg/ports/ports.go`:

| Tier | Range | Services |
|------|-------|----------|
| Infrastructure | 16666-16999 | doom-bridge 16666, doom-go-injector 16667, trace-collector 16670 (HTTP) / 16671 (metrics) |
| Control Plane | 17000-17999 | unheaded-daemon 17000 / 17001 (gRPC), CLI server 17010, protocol-api 17100 / 17101 (gRPC) |
| Wotan | 18000-18099 | wotan 18000 (HTTP) / 18001 (gRPC) |
| Core Services | 19000-19099 | timeguru 19000, architect 19001, captain 19002, micromanager 19003, monad 19004, sophia 19005, cuirass 19006, key-mgmt 19007, pqc-verifier 19008 (reserved, ADR-119), shield 19009, firewall 19010, sword 19011 (was canary), chaos 19012, QoS 19013, backup-mgr 19014, compliance 19015, NFV 19016, version 19017, anomaly 19018, telemetry-agg 19019, unheaded-auth 19020 ([[Service-Auth]]) |
| Health | 19100 | akira |
| Health | 19101 | heimdall-daemon `/metrics` (`-metrics-addr`, default `127.0.0.1`; empty disables) |
| Threat feeds | 19120 | kev-poller (ADR-055); 19110 is the loadbalancer's xdpsync (loopback only) |
| Alerting | 19130 | alert-mailer (ADR-100), `127.0.0.1` in `monitoring/docker-compose.yml` |
| Alerting | 19131 | alert-mailer `/heartbeat/{host}` only (dead-man switch); loopback until `HEARTBEAT_LISTEN` |
| Log store | 19140 | logstore-writer: Vector's `well_logs` sink into the Well (S-E1-LIVE Evening 16); compose networks only, no published port |
| Packages | 18888 | signed apt repo (reprepro) behind the `nginx` sysuser, `deploy/nginx/unheaded-apt.conf`: 127.0.0.1 and 192.168.13.2 only (S-IRON-GATE Phase 14; installs in E3) |
| Applications | 20000-20099 | dashboard-backend 20000, kanban-app 20001, wiki-server 20002 |
| AI | 20100-20106 | vLLM DeepSeek 20100, vLLM Qwen 20101, Qdrant 20102, Zhen web UI 20103 (TLS 1.3 hybrid-only, `zhen-ui-front`, ADR-138; `127.0.0.1` in the shipped unit, the app behind it is a Unix socket), BGE-M3 20104, Sophia-Eye 20105, AI web UI 20106 |
| Zhen | 20110-20112 | vor 20110, zhen-agentd 20111, zhen-ui-front `/metrics` `/health` `/ready` 20112 (`127.0.0.1`, plain HTTP) (20107 and 20108 are sophia-eye) |
| Gateway | 21000-21443 | gateway HTTP 21000, HTTPS 21443 (in compose, Traefik's dashboard is on `127.0.0.1:21000`) |
| Tools | 22000 | cert-gen |
| User Apps | 26000-26666 | reserved; UPC tty bridge 26100, upc-cartridgectl metrics 26610 (ADR-108) |

Outside the Doom Range (infrastructure on standard ports):

| Service | Port | Bound to |
|---------|------|----------|
| The Well (PostgreSQL) | 5432 | `127.0.0.1` only, TLS ([[The-Well]]) |
| Huginn | 9110 | host ([[Service-Huginn]]) |
| VictoriaMetrics | 8428 | `127.0.0.1`, plus `UNHEADED_P2P_BIND` and `UNHEADED_OVERLAY_BIND` |
| Grafana | 3001 | `127.0.0.1`, plus `UNHEADED_LAN_BIND`; `monitoring/docker-compose.yml` on 3000: `GRAFANA_BIND`, default `127.0.0.1` |
| ClickHouse | 8123 / 9000 | `127.0.0.1` |
| Vector metrics | 9598 | `172.17.0.1` (override `VECTOR_METRICS_BIND`) |
| syslog-relay (ADR-124) | 514 udp and tcp | `10.10.20.1`, bound by `unheaded-syslog-relay.socket`; LAN after D7; no wildcard; not installed |
| syslog-relay metrics | 9599 | `127.0.0.1` and `[fd00:dead:beef::1]` |
| CoreDNS | 5353 | `127.0.0.1` (override `COREDNS_PORT`) |
| Traefik edge | 80 / 443 | all IPv4 on purpose, narrowed by `TRAEFIK_BIND` (override `TRAEFIK_HTTP_PORT`, `TRAEFIK_HTTPS_PORT`) |
| Traefik gRPC entrypoint | 50051 | `127.0.0.1`, no router uses it (override `TRAEFIK_GRPC_PORT`) |
| Prometheus (monitoring stack) | 9090 | `127.0.0.1` (`monitoring/docker-compose.yml`) |

### Compose publishes

Docker publishes through its own NAT rules, so a host firewall's INPUT chain
does not cover them. Every compose publish therefore names an internal address,
and `scripts/check-compose-publish-bind.sh` fails one that does not. The
exceptions are in `docs/security/compose-publish-allowlist.txt`.

All first-party ports in `docker-compose.yml` publish on `127.0.0.1`. The twelve
that VictoriaMetrics scrapes also publish on docker0's gateway, `172.17.0.1`,
because `config/victoria/scrape.yml` reaches them at `host.docker.internal`
(`UNHEADED_GATEWAY_BIND` if a host's docker0 differs;
`scripts/check-scrape-publish.sh` holds it). A host that serves peers sets the
variables below in `.env`. Unset, they fall to spare loopback addresses, because
docker will not start a container whose published address is missing.

| Variable | West's value | Publishes | Who needs it |
|----------|--------------|-----------|--------------|
| `UNHEADED_P2P_BIND` | `192.168.13.2` | 8428 | east's huginn |
| `UNHEADED_OVERLAY_BIND` | `[fd00:dead:beef::1]` | 8428, 18000, 19006, 20000 | Prometheus remote-write and scrape |
| `UNHEADED_LAN_BIND` | `192.168.69.184`, only if wanted | 3001, 20000, 20001 | browsers on the LAN using the pages' nav links |

The victoria Ansible role sets the first two for its own `up`.

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
