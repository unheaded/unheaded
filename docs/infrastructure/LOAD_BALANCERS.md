# Load Balancer Stack — THE PAULDRONS

> Three-layer load balancing: HAProxy edge → HAProxy internal → Nginx per-app sidecars.
> Every layer exports Prometheus metrics and JSON logs to Loki.

---

## Architecture

```
                    INTERNET
                       │
              ┌────────▼────────┐
              │  HAProxy EDGE   │  TLS 1.3 termination
              │  :21443 HTTPS   │  Rate limiting (100 req/10s per IP)
              │  :21080 HTTP    │  Trace ID injection (X-Trace-Id UUID)
              │  :21404 stats   │  Security headers (HSTS, CSP, XFO)
              └───┬─────────┬───┘
                  │         │
        ┌─────────▼──┐  ┌──▼─────────┐
        │ HAProxy    │  │            │
        │ INTERNAL   │  │  (future   │
        │ :21081     │  │  replicas) │
        │ :21405     │  │            │
        └──┬──┬──┬───┘  └────────────┘
           │  │  │
     ┌─────▼┐ ▼──▼───┐
     │Nginx ││Nginx   │   ... 6 Nginx sidecars total
     │wotan ││monad   │   Each: /stub_status + JSON access logs
     └──┬───┘└──┬─────┘
        │       │
     ┌──▼──┐ ┌──▼──┐
     │wotan│ │monad│     Actual application services
     └─────┘ └─────┘
```

## Port Allocations (Doom Range)

| Service | Port | Purpose |
|---------|------|---------|
| HAProxy Edge HTTP | 21080 | HTTP → HTTPS redirect + ACME |
| HAProxy Edge HTTPS | 21443 | TLS termination, main ingress |
| HAProxy Edge Stats | 21404 | Stats UI + Prometheus /metrics |
| HAProxy Internal | 21081 | Service routing |
| HAProxy Internal Stats | 21405 | Stats UI + Prometheus /metrics |
| Nginx Wotan | 18080 | Sidecar for wotan :18000 |
| Nginx Monad | 19080 | Sidecar for monad :19004 |
| Nginx Sophia | 19081 | Sidecar for sophia :19005 |
| Nginx Dashboard | 20080 | Sidecar for dashboard :20000 |
| Nginx Kanban | 20081 | Sidecar for kanban :20001 |
| Nginx Gateway | 21090 | Sidecar for gateway :21000 |

## Telemetry Integration

### Metrics (Prometheus → VictoriaMetrics)

| Source | Endpoint | Scrape Interval |
|--------|----------|-----------------|
| HAProxy Edge | :21404/metrics | 5s (native PROMEX) |
| HAProxy Internal | :21405/metrics | 5s (native PROMEX) |
| Nginx sidecars (×6) | :PORT/stub_status | 5s (nginx-exporter) |

### Logs (JSON → Promtail → Loki)

All HAProxy and Nginx instances emit structured JSON logs:

**HAProxy fields:** timestamp, client, frontend, backend, server, status, bytes_read, time_request, time_queue, time_connect, time_response, time_total, retries, trace_id

**Nginx fields:** timestamp, remote_addr, request_method, request_uri, status, body_bytes_sent, request_time, upstream_response_time, upstream_addr, upstream_status, trace_id, service

### Alerts (7 rules in loadbalancers.yml)

| Alert | Severity | Condition |
|-------|----------|-----------|
| HAProxyBackendDown | critical | Backend unreachable > 30s |
| HAProxyHighErrorRate | warning | 5xx > 5% for 2m |
| HAProxyHighLatency | warning | P99 > 2s for 5m |
| HAProxyConnectionSaturation | warning | > 80% maxconn for 2m |
| HAProxyRateLimited | warning | 429s > 50/s for 1m |
| NginxHighErrorRate | warning | 5xx > 5% for 2m |
| NginxUpstreamDown | critical | Upstream down > 1m |

### Dashboard

Grafana dashboard `06_loadbalancers.json` — 11 panels covering edge, internal, and per-sidecar views.

---

## Deployment: Docker Compose

The fastest path. All 8 containers orchestrated in one file.

```bash
# From repo root:
docker compose -f docker-compose.yml -f docker/docker-compose.loadbalancers.yml up -d

# Verify:
curl -s http://localhost:21404/stats     # HAProxy edge stats
curl -s http://localhost:21405/stats     # HAProxy internal stats
curl -s http://localhost:21404/metrics   # Prometheus metrics
```

**Config files:**
- `docker/haproxy/edge/haproxy.cfg`
- `docker/haproxy/internal/haproxy.cfg`
- `docker/nginx/nginx-{wotan,monad,sophia,dashboard,kanban,gateway}.conf`
- `docker/docker-compose.loadbalancers.yml`

**Networks:** `lb-edge` (HAProxy ↔ internal), `lb-internal` (internal ↔ Nginx sidecars). Both bridged to `unheaded-telemetry` for metrics scraping.

---

## Deployment: Bare Metal

For direct installation on Debian/Ubuntu hosts (WEST/EAST).

### Prerequisites

```bash
# HAProxy 2.8+ (with PROMEX module)
apt-get update && apt-get install -y haproxy=2.8*

# Nginx 1.25+ (with stub_status module — included by default)
apt-get install -y nginx

# Verify PROMEX support
haproxy -vv | grep -i prometheus
```

### HAProxy Edge (run on WEST)

```bash
# Copy config
cp docker/haproxy/edge/haproxy.cfg /etc/haproxy/haproxy.cfg

# Create cert directory
mkdir -p /etc/haproxy/certs /etc/haproxy/errors

# Edit bind addresses for bare metal IPs:
#   bind *:21443  →  bind [fd00:dead:beef::1]:21443
#   bind *:21080  →  bind [fd00:dead:beef::1]:21080
#   bind *:21404  →  bind 127.0.0.1:21404  (stats local only)
sed -i 's/bind \*:21443/bind [fd00:dead:beef::1]:21443/' /etc/haproxy/haproxy.cfg
sed -i 's/bind \*:21080/bind [fd00:dead:beef::1]:21080/' /etc/haproxy/haproxy.cfg
sed -i 's/bind \*:21404/bind 127.0.0.1:21404/' /etc/haproxy/haproxy.cfg

# Update backend server addresses (Docker hostnames → bare metal IPs)
sed -i 's/haproxy-internal:21081/[fd00:dead:beef::1]:21081/' /etc/haproxy/haproxy.cfg

# Validate and start
haproxy -c -f /etc/haproxy/haproxy.cfg
systemctl enable --now haproxy

# Verify
curl -s http://127.0.0.1:21404/healthz   # Should return 200
curl -s http://127.0.0.1:21404/metrics    # Should return Prometheus text
```

### HAProxy Internal (run on WEST, optionally also EAST)

```bash
cp docker/haproxy/internal/haproxy.cfg /etc/haproxy/haproxy-internal.cfg

# Update backend addresses (Docker → bare metal service IPs)
# Example for wotan:
sed -i 's/nginx-wotan:18080/[fd00:dead:beef:1::101]:18080/' /etc/haproxy/haproxy-internal.cfg
# Repeat for all backends...

# Run as separate systemd unit
cat > /etc/systemd/system/haproxy-internal.service << 'UNIT'
[Unit]
Description=HAProxy Internal Load Balancer
After=network.target

[Service]
Type=notify
ExecStartPre=/usr/sbin/haproxy -c -f /etc/haproxy/haproxy-internal.cfg
ExecStart=/usr/sbin/haproxy -Ws -f /etc/haproxy/haproxy-internal.cfg -p /run/haproxy-internal.pid
ExecReload=/bin/kill -USR2 $MAINPID
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now haproxy-internal
```

### Nginx Sidecars (run co-located with each service)

```bash
# Per-service sidecar — example for wotan:
cp docker/nginx/nginx-wotan.conf /etc/nginx/sites-available/wotan
ln -sf /etc/nginx/sites-available/wotan /etc/nginx/sites-enabled/wotan

# Update upstream address (Docker hostname → localhost or service IP)
sed -i 's/wotan:18000/127.0.0.1:18000/' /etc/nginx/sites-available/wotan

# Repeat for each service...
nginx -t && systemctl reload nginx
```

### Prometheus Scrape (bare metal)

Add to `/etc/prometheus/prometheus.yml`:

```yaml
  - job_name: 'haproxy-edge'
    static_configs:
      - targets: ['[fd00:dead:beef::1]:21404']
        labels:
          tier: edge
          component: haproxy

  - job_name: 'haproxy-internal'
    static_configs:
      - targets: ['[fd00:dead:beef::1]:21405']
        labels:
          tier: internal
          component: haproxy

  - job_name: 'nginx-sidecars'
    static_configs:
      - targets:
          - '[fd00:dead:beef:1::101]:18080'
          - '[fd00:dead:beef:1::102]:19080'
          - '[fd00:dead:beef:1::103]:19081'
          - '[fd00:dead:beef:1::201]:20080'
          - '[fd00:dead:beef:1::200]:20081'
          - '[fd00:dead:beef::1]:21090'
        labels:
          tier: sidecar
          component: nginx
    metrics_path: /stub_status
```

### Promtail (bare metal)

Add scrape config for HAProxy/Nginx log files:

```yaml
  - job_name: haproxy-logs
    static_configs:
      - targets: [localhost]
        labels:
          job: haproxy
          __path__: /var/log/haproxy*.log

  - job_name: nginx-logs
    static_configs:
      - targets: [localhost]
        labels:
          job: nginx
          __path__: /var/log/nginx/access.log
    pipeline_stages:
      - json:
          expressions:
            level: level
            service: service
            trace_id: trace_id
            status: status
```

---

## Deployment: LXD

HAProxy and Nginx run inside LXD system containers on the host.

### HAProxy Edge Container

```bash
# Create container
lxc launch ubuntu:22.04 haproxy-edge \
  --config limits.cpu=2 \
  --config limits.memory=512MB

# Attach to networks
lxc network attach unheaded-edge haproxy-edge eth0
lxc config device add haproxy-edge eth0 nic \
  nictype=bridged parent=br-edge \
  ipv4.address=10.10.10.2 \
  ipv6.address=fd00:dead:beef::2

# Proxy ports to host
lxc config device add haproxy-edge http proxy \
  listen=tcp:0.0.0.0:21080 connect=tcp:127.0.0.1:21080
lxc config device add haproxy-edge https proxy \
  listen=tcp:0.0.0.0:21443 connect=tcp:127.0.0.1:21443
lxc config device add haproxy-edge stats proxy \
  listen=tcp:0.0.0.0:21404 connect=tcp:127.0.0.1:21404

# Push config and install
lxc file push docker/haproxy/edge/haproxy.cfg haproxy-edge/etc/haproxy/haproxy.cfg
lxc exec haproxy-edge -- bash -c "apt-get update && apt-get install -y haproxy"
lxc exec haproxy-edge -- systemctl enable --now haproxy

# Verify
lxc exec haproxy-edge -- curl -s http://127.0.0.1:21404/healthz
```

### HAProxy Internal Container

```bash
lxc launch ubuntu:22.04 haproxy-internal \
  --config limits.cpu=2 \
  --config limits.memory=512MB

lxc network attach unheaded-internal haproxy-internal eth0
lxc config device add haproxy-internal eth0 nic \
  nictype=bridged parent=br-internal \
  ipv4.address=10.10.10.3

lxc config device add haproxy-internal service proxy \
  listen=tcp:0.0.0.0:21081 connect=tcp:127.0.0.1:21081
lxc config device add haproxy-internal stats proxy \
  listen=tcp:0.0.0.0:21405 connect=tcp:127.0.0.1:21405

lxc file push docker/haproxy/internal/haproxy.cfg haproxy-internal/etc/haproxy/haproxy.cfg
lxc exec haproxy-internal -- bash -c "apt-get update && apt-get install -y haproxy"
lxc exec haproxy-internal -- systemctl enable --now haproxy
```

### Nginx Sidecar Containers

```bash
# Template: create one per service
for svc in wotan monad sophia dashboard kanban gateway; do
  lxc launch ubuntu:22.04 nginx-${svc} \
    --config limits.cpu=1 \
    --config limits.memory=256MB

  lxc network attach unheaded-internal nginx-${svc} eth0

  lxc file push docker/nginx/nginx-${svc}.conf nginx-${svc}/etc/nginx/nginx.conf
  lxc exec nginx-${svc} -- bash -c "apt-get update && apt-get install -y nginx"
  lxc exec nginx-${svc} -- systemctl enable --now nginx
done
```

### LXD Profile (reusable)

```yaml
# lxd/profiles/loadbalancer.yaml
config:
  limits.cpu: "2"
  limits.memory: 512MB
  security.nesting: "false"
  boot.autostart: "true"
  boot.autostart.priority: "5"
description: Load balancer container profile
devices:
  root:
    path: /
    pool: default
    type: disk
    size: 2GB
```

```bash
lxc profile create loadbalancer
lxc profile edit loadbalancer < lxd/profiles/loadbalancer.yaml
# Then: lxc launch ubuntu:22.04 haproxy-edge --profile loadbalancer
```

---

## Deployment: containerd

For containerd-managed OCI containers (no Docker daemon).

### Pull Images

```bash
ctr image pull docker.io/library/haproxy:2.8-alpine
ctr image pull docker.io/library/nginx:1.25-alpine
```

### HAProxy Edge

```bash
# Create container
ctr run -d \
  --net-host \
  --mount type=bind,src=$(pwd)/docker/haproxy/edge/haproxy.cfg,dst=/usr/local/etc/haproxy/haproxy.cfg,options=ro \
  --mount type=bind,src=/etc/haproxy/certs,dst=/etc/haproxy/certs,options=ro \
  docker.io/library/haproxy:2.8-alpine \
  haproxy-edge

# Verify
ctr task exec --exec-id verify haproxy-edge \
  wget -qO- http://127.0.0.1:21404/healthz
```

### HAProxy Internal

```bash
ctr run -d \
  --net-host \
  --mount type=bind,src=$(pwd)/docker/haproxy/internal/haproxy.cfg,dst=/usr/local/etc/haproxy/haproxy.cfg,options=ro \
  docker.io/library/haproxy:2.8-alpine \
  haproxy-internal
```

### Nginx Sidecars

```bash
for svc in wotan monad sophia dashboard kanban gateway; do
  ctr run -d \
    --net-host \
    --mount type=bind,src=$(pwd)/docker/nginx/nginx-${svc}.conf,dst=/etc/nginx/nginx.conf,options=ro \
    docker.io/library/nginx:1.25-alpine \
    nginx-${svc}
done
```

### Lifecycle with systemd

```bash
# /etc/systemd/system/containerd-haproxy-edge.service
cat > /etc/systemd/system/containerd-haproxy-edge.service << 'UNIT'
[Unit]
Description=HAProxy Edge (containerd)
After=containerd.service
Requires=containerd.service

[Service]
Type=forking
ExecStartPre=/usr/bin/ctr image pull docker.io/library/haproxy:2.8-alpine
ExecStart=/usr/bin/ctr run -d \
  --net-host \
  --mount type=bind,src=/etc/haproxy/edge/haproxy.cfg,dst=/usr/local/etc/haproxy/haproxy.cfg,options=ro \
  docker.io/library/haproxy:2.8-alpine \
  haproxy-edge
ExecStop=/usr/bin/ctr task kill haproxy-edge
ExecStopPost=/usr/bin/ctr container rm haproxy-edge
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable containerd-haproxy-edge
```

---

## Deployment: NixOS

Declarative, reproducible, rollback-capable.

### HAProxy Edge Module

```nix
# nix/modules/haproxy-edge.nix
{ config, pkgs, lib, ... }:

{
  services.haproxy = {
    enable = true;
    config = builtins.readFile ../../docker/haproxy/edge/haproxy.cfg;
  };

  # Override for bare metal bind addresses
  environment.etc."haproxy/haproxy.cfg".text = lib.mkForce (
    builtins.replaceStrings
      [ "bind *:21443" "bind *:21080" "bind *:21404" "haproxy-internal:21081" ]
      [
        "bind [fd00:dead:beef::1]:21443 ssl crt /etc/haproxy/certs/ alpn h2,http/1.1"
        "bind [fd00:dead:beef::1]:21080"
        "bind 127.0.0.1:21404"
        "[fd00:dead:beef::1]:21081"
      ]
      (builtins.readFile ../../docker/haproxy/edge/haproxy.cfg)
  );

  networking.firewall.allowedTCPPorts = [ 21080 21443 ];

  # Prometheus scrape label
  services.prometheus.exporters.haproxy = {
    enable = false;  # Using native PROMEX instead
  };

  systemd.services.haproxy = {
    serviceConfig = {
      LimitNOFILE = 65536;
      MemoryMax = "512M";
      CPUQuota = "200%";
    };
  };
}
```

### HAProxy Internal Module

```nix
# nix/modules/haproxy-internal.nix
{ config, pkgs, lib, ... }:

let
  # Map Docker hostnames to NixOS container IPs
  backendMap = {
    "nginx-wotan:18080"        = "[fd00:dead:beef:1::101]:18080";
    "nginx-monad:19080"        = "[fd00:dead:beef:1::102]:19080";
    "nginx-sophia:19081"       = "[fd00:dead:beef:1::103]:19081";
    "nginx-dashboard:20080"    = "[fd00:dead:beef:1::201]:20080";
    "nginx-kanban:20081"       = "[fd00:dead:beef:1::200]:20081";
    "nginx-gateway:21090"      = "[fd00:dead:beef::1]:21090";
    "nginx-anamnesis:19082"    = "[fd00:dead:beef:1::104]:19082";
    "nginx-timeguru:19090"     = "[fd00:dead:beef:1::120]:19090";
    "nginx-captain:19091"      = "[fd00:dead:beef:1::121]:19091";
    "nginx-micromanager:19092" = "[fd00:dead:beef:1::122]:19092";
    "nginx-architect:19093"    = "[fd00:dead:beef:1::123]:19093";
    "nginx-cuirass:17080"      = "[fd00:dead:beef:1::105]:17080";
  };

  rawConfig = builtins.readFile ../../docker/haproxy/internal/haproxy.cfg;
  resolvedConfig = builtins.foldl'
    (cfg: pair: builtins.replaceStrings [ (builtins.elemAt pair 0) ] [ (builtins.elemAt pair 1) ] cfg)
    rawConfig
    (lib.mapAttrsToList (name: value: [ name value ]) backendMap);
in
{
  systemd.services.haproxy-internal = {
    description = "HAProxy Internal Load Balancer";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];

    serviceConfig = {
      Type = "notify";
      ExecStartPre = "${pkgs.haproxy}/bin/haproxy -c -f /etc/haproxy/haproxy-internal.cfg";
      ExecStart = "${pkgs.haproxy}/bin/haproxy -Ws -f /etc/haproxy/haproxy-internal.cfg";
      ExecReload = "${pkgs.coreutils}/bin/kill -USR2 $MAINPID";
      Restart = "on-failure";
      RestartSec = "5s";
      LimitNOFILE = 65536;
      MemoryMax = "512M";
    };
  };

  environment.etc."haproxy/haproxy-internal.cfg".text = resolvedConfig;
}
```

### Nginx Sidecar Module (Generic)

```nix
# nix/modules/nginx-sidecar.nix
{ config, pkgs, lib, ... }:

let
  cfg = config.services.unheaded.nginx-sidecars;

  mkSidecar = name: opts: {
    "nginx-${name}" = {
      description = "Nginx sidecar for ${name}";
      after = [ "network.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "forking";
        ExecStartPre = "${pkgs.nginx}/bin/nginx -t -c /etc/nginx/sidecar-${name}.conf";
        ExecStart = "${pkgs.nginx}/bin/nginx -c /etc/nginx/sidecar-${name}.conf";
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        PIDFile = "/run/nginx-${name}.pid";
        Restart = "on-failure";
        MemoryMax = "256M";
      };
    };
  };

  mkConfig = name: opts: {
    "nginx/sidecar-${name}.conf".text = ''
      worker_processes auto;
      error_log /var/log/nginx/error-${name}.log warn;
      pid /run/nginx-${name}.pid;

      events { worker_connections 1024; use epoll; }

      http {
        log_format json_combined escape=json
          '{"timestamp":"$time_iso8601","remote_addr":"$remote_addr",'
          '"request_method":"$request_method","request_uri":"$request_uri",'
          '"status":$status,"body_bytes_sent":$body_bytes_sent,'
          '"request_time":$request_time,"upstream_response_time":"$upstream_response_time",'
          '"trace_id":"$http_x_trace_id","service":"${name}"}';
        access_log /var/log/nginx/access-${name}.log json_combined;

        upstream ${name}_backend {
          least_conn;
          keepalive 32;
          server ${opts.upstream} max_fails=3 fail_timeout=10s;
        }

        server {
          listen ${toString opts.port};
          location /stub_status { stub_status; allow 127.0.0.1; allow fd00::/8; deny all; }
          location /health { proxy_pass http://${name}_backend/health; access_log off; }
          location /metrics { proxy_pass http://${name}_backend/metrics; }
          location / {
            proxy_pass http://${name}_backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Trace-Id $http_x_trace_id;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_next_upstream error timeout http_502 http_503;
          }
        }

        map $http_upgrade $connection_upgrade {
          default upgrade;
          "" close;
        }
      }
    '';
  };

in
{
  options.services.unheaded.nginx-sidecars = lib.mkOption {
    type = lib.types.attrsOf (lib.types.submodule {
      options = {
        port = lib.mkOption { type = lib.types.int; };
        upstream = lib.mkOption { type = lib.types.str; };
      };
    });
    default = {};
  };

  config = lib.mkIf (cfg != {}) {
    systemd.services = lib.mkMerge (lib.mapAttrsToList mkSidecar cfg);
    environment.etc = lib.mkMerge (lib.mapAttrsToList mkConfig cfg);
  };
}
```

### NixOS Host Configuration (usage)

```nix
# nix/hosts/west.nix (excerpt)
{
  imports = [
    ../modules/haproxy-edge.nix
    ../modules/haproxy-internal.nix
    ../modules/nginx-sidecar.nix
  ];

  services.unheaded.nginx-sidecars = {
    wotan       = { port = 18080; upstream = "[fd00:dead:beef:1::101]:18000"; };
    monad       = { port = 19080; upstream = "[fd00:dead:beef:1::102]:19004"; };
    sophia      = { port = 19081; upstream = "[fd00:dead:beef:1::103]:19005"; };
    anamnesis   = { port = 19082; upstream = "[fd00:dead:beef:1::104]:5002";  };
    timeguru    = { port = 19090; upstream = "[fd00:dead:beef:1::120]:19000"; };
    captain     = { port = 19091; upstream = "[fd00:dead:beef:1::121]:19002"; };
    micromanager= { port = 19092; upstream = "[fd00:dead:beef:1::122]:19003"; };
    architect   = { port = 19093; upstream = "[fd00:dead:beef:1::123]:19001"; };
    dashboard   = { port = 20080; upstream = "[fd00:dead:beef:1::201]:20000"; };
    kanban      = { port = 20081; upstream = "[fd00:dead:beef:1::200]:20001"; };
    gateway     = { port = 21090; upstream = "[fd00:dead:beef::1]:21000";     };
    cuirass     = { port = 17080; upstream = "[fd00:dead:beef:1::105]:17000"; };
  };
}
```

### Deploy

```bash
# Build and switch (WEST)
nixos-rebuild switch --flake .#west

# Verify
systemctl status haproxy haproxy-internal nginx-wotan nginx-monad
curl -s http://127.0.0.1:21404/healthz
curl -s http://127.0.0.1:21405/healthz
```

---

## Verification Checklist

```bash
# 1. HAProxy edge responding
curl -s http://localhost:21404/healthz     # 200 OK
curl -s http://localhost:21404/metrics     # Prometheus text format

# 2. HAProxy internal responding
curl -s http://localhost:21405/healthz     # 200 OK
curl -s http://localhost:21405/metrics     # Prometheus text format

# 3. Nginx sidecars responding (example: wotan)
curl -s http://localhost:18080/stub_status # Active connections: N ...
curl -s http://localhost:18080/health      # proxied from wotan

# 4. End-to-end through all 3 layers
curl -sk https://localhost:21443/api/wotan/health  # Edge → Internal → Nginx → Wotan

# 5. Prometheus scraping
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.component=="haproxy")'

# 6. Loki receiving logs
curl -s http://localhost:3100/loki/api/v1/query?query={component="haproxy"}&limit=5

# 7. Grafana dashboard
# Open http://localhost:3000 → Dashboards → Load Balancers
```

---

> **Source:** [haproxy/edge/haproxy.cfg](../../docker/haproxy/edge/haproxy.cfg) · [haproxy/internal/haproxy.cfg](../../docker/haproxy/internal/haproxy.cfg) · [nginx/templates/](../../docker/nginx/templates/) · [docker-compose.loadbalancers.yml](../../docker/docker-compose.loadbalancers.yml)

*The Pauldrons protect every shoulder of the Kingdom. Three layers deep. Every packet traced.*
