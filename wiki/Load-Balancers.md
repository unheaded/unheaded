# Load Balancers — The Pauldrons

Three-layer load balancing protecting every shoulder of the Kingdom.

## Architecture

| Layer | Technology | Port | Role |
|-------|-----------|------|------|
| Edge | HAProxy 2.8+ | :21443/:21080/:21404 | TLS termination, rate limiting, trace injection |
| Internal | HAProxy 2.8+ | :21081/:21405 | Service routing, health checks, circuit breaking |
| Per-App | Nginx 1.25+ | :18080-:21090 | App-level LB, WebSocket upgrade, upstream failover |

## Deployment Targets

All services are documented for 4 deployment methods:

| Target | Method | Config Source |
|--------|--------|--------------|
| Docker Compose | `docker compose -f docker-compose.loadbalancers.yml up` | `docker/` |
| Bare Metal | systemd units + apt packages | Manual install |
| LXD | System containers with proxy devices | `lxc launch` |
| containerd | OCI containers with `ctr run` | `ctr` CLI |
| NixOS | Declarative modules | `nix/modules/haproxy-*.nix`, `nix/modules/nginx-sidecar.nix` |

## Telemetry

| Signal | Pipeline | Dashboard |
|--------|----------|-----------|
| Metrics | HAProxy PROMEX + Nginx stub_status → Prometheus → VictoriaMetrics | `06_loadbalancers.json` |
| Logs | JSON format → Promtail → Loki | Grafana Explore |
| Alerts | 7 rules in `loadbalancers.yml` → AlertManager | PagerDuty/Slack (TBD) |

## Interchangeable Alternatives

Follows the Kingdom's interchangeability pattern — any component swappable if it exports Prometheus metrics + JSON logs with `trace_id`.

| Role | Default | Alternatives |
|------|---------|-------------|
| Edge/Internal LB | HAProxy | Envoy, Traefik, Caddy, IPVS/LVS, Keepalived, Katran (XDP), Cilium |
| Per-App Sidecar | Nginx | Envoy, Caddy, Pingora (Rust), OpenResty, Sozu (Rust), linkerd2-proxy |

See [full comparison table](../docs/infrastructure/LOAD_BALANCERS.md#interchangeable-alternatives) for strengths, metrics endpoints, and trade-offs.

## Nginx Sidecars

| Sidecar | Port | Upstream Service | Upstream Port |
|---------|------|-----------------|---------------|
| nginx-wotan | 18080 | wotan | 18000 |
| nginx-monad | 19080 | monad | 19004 |
| nginx-sophia | 19081 | sophia | 19005 |
| nginx-dashboard | 20080 | dashboard | 20000 |
| nginx-kanban | 20081 | kanban | 20001 |
| nginx-gateway | 21090 | gateway | 21000 |

---

> **Source:** [docs/infrastructure/LOAD_BALANCERS.md](../docs/infrastructure/LOAD_BALANCERS.md) · [docker/haproxy/](../docker/haproxy/) · [docker/nginx/](../docker/nginx/)
