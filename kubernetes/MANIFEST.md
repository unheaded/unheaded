<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Manifest Inventory

Every manifest in `kubernetes/manifests/`, the workload it defines, and the ports it
uses. All ports trace back to `pkg/ports/ports.go` (the Doom Range) or to a well-known
upstream image port. Nothing here invents a port.

## Base — foundation

| Path | Kind | Notes |
|------|------|-------|
| `base/namespace/` | Namespace | `unheaded`, PSA `enforce: restricted` |
| `base/config/` | ConfigMap, Secret | `unheaded-shared` (mirrors `.env.shared`), `unheaded-secrets` + `the-well-users` (placeholders) |
| `base/network-policies/` | 11 NetworkPolicies | default-deny + allow DNS / Wotan / The Well / metrics / edge |

## Base — stateful spine

| Path | Kind | Ports | Registry name |
|------|------|-------|---------------|
| `base/wotan/` | StatefulSet + Service (+headless) | 18000 HTTP, 18001 gRPC | `WotanHTTP`, `WotanGRPC` |
| `base/the-well/` | StatefulSet + headless Service + init ConfigMap | 5432 | PostgreSQL (3 DBs, 7 users) |

## Base — core services (Deployment + ClusterIP Service)

| Path | Port | Registry constant |
|------|------|-------------------|
| `base/timeguru/` | 19000 | `Timeguru` |
| `base/architect/` | 19001 | `Architect` |
| `base/captain/` | 19002 | `Captain` |
| `base/micromanager/` | 19003 | `Micromanager` |
| `base/monad/` | 19004 | `Monad` |
| `base/sophia/` | 19005 | `Sophia` |
| `base/cuirass/` | 19006 | `Cuirass` |
| `base/dashboard-backend/` | 20000 | `DashboardBackend` |
| `base/kanban-app/` | 20001 | `KanbanApp` |
| `base/gateway/` | 21000 HTTP, 21443 HTTPS | `GatewayHTTP`, `GatewayHTTPS` |

Every core service: `runAsNonRoot`, `readOnlyRootFilesystem`, `drop: [ALL]`,
`seccompProfile: RuntimeDefault`, `/health`+`/ready` probes, `wait-for-wotan` init
(kanban also `wait-for-the-well`), `kingdom.bus-client` label.

## Base — edge / load balancing

| Path | Kind | Ports |
|------|------|-------|
| `base/loadbalancers/haproxy-edge.yaml` (+`-config`) | Deployment + Service + ConfigMap | 21080, 21443, 21404 |
| `base/loadbalancers/haproxy-internal.yaml` (+`-config`) | Deployment + Service + ConfigMap | 21081, 21405 |
| `base/loadbalancers/ingress.yaml` | Ingress | idiomatic alternative to HAProxy edge |

> The Docker nginx per-app sidecars (`nginx-wotan/-monad/-sophia/-dashboard/-kanban/
> -gateway`) are replaced by the native per-service ClusterIP Services + this Ingress.
> Rationale is in [`BUILD-PLAN.md`](BUILD-PLAN.md) and `KUBERNETES_SETUP_SUMMARY.md`.

## Base — observability

| Path | Kind | Ports |
|------|------|-------|
| `base/telemetry/prometheus.yaml` (+rbac, +config) | Deployment + PVC + SA/ClusterRole | 9090 |
| `base/telemetry/victoriametrics.yaml` | Deployment + PVC | 8428 |
| `base/telemetry/loki.yaml` (+config) | Deployment + PVC | 3100 |
| `base/telemetry/grafana.yaml` | Deployment | 3000 |
| `base/telemetry/node-exporter-daemonset.yaml` | DaemonSet (hostPID/hostNetwork) | 9100 |
| `base/telemetry/promtail-daemonset.yaml` (+config) | DaemonSet | 9080 |
| `base/logging/clickhouse.yaml` | StatefulSet + PVC | 8123, 9000 |
| `base/logging/vector.yaml` (+config) | Deployment | — |

## Base — security

| Path | Kind | Notes |
|------|------|-------|
| `base/suricata/daemonset.yaml` (+configmap) | DaemonSet | hostNetwork IDS; adds `NET_ADMIN`,`NET_RAW`,`SYS_NICE` |

## Overlays

| Path | Purpose |
|------|---------|
| `overlays/dev/` | NodePort on 21000/21443/20000/20001/3000, `LOG_LEVEL=debug`, `:dev` images |
| `overlays/prod/` | `haproxy-edge` LoadBalancer, `replicas: 2` for stateless, pinned tags |
| `overlays/gpu-vllm/` | vLLM/ROCm Deployment + Service, port 20100, `amd.com/gpu` (optional) |
| `overlays/wireguard/` | host-net DaemonSet, 51820/udp, `fd00:dead:beef::/48` (optional) |

## Documented hardening exceptions

| Workload | Exception | Why |
|----------|-----------|-----|
| `suricata` | `hostNetwork`, root, `NET_ADMIN`/`NET_RAW` | raw packet capture (IDS) |
| `node-exporter` | `hostPID`/`hostNetwork`, host `/proc`,`/sys` (ro) | node-level metrics |
| `promtail` | host log dirs (ro) | tails container stdout |
| `gpu-vllm` | `SYS_PTRACE`, `/dev/kfd`,`/dev/dri`, GPU node | ROCm GPU passthrough |
| `wireguard` | `hostNetwork`, `NET_ADMIN`/`SYS_MODULE` | kernel tunnel interface |

Each keeps every other restriction (`drop: [ALL]` then add-back, `seccomp:
RuntimeDefault`, no privilege escalation).

## Counts

- Base components: 18 directories.
- Manifest files: see `find manifests -name '*.yaml' | wc -l` (84 YAML files at
  authoring time).
- Services mirrored: 10 Kingdom services + gateway + Wotan + The Well + 2 HAProxy LBs
  + 6 observability components + Suricata, plus 2 optional overlays.
