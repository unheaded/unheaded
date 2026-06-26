<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Kubernetes Setup Summary

> This document replaces the old `DOCKER_SETUP_SUMMARY.md` that previously lived here
> (it was a verbatim copy of the Docker docs). It explains how the Docker stack maps
> onto Kubernetes so you can reason about the translation, not just apply it.

## What was created

A complete kustomize tree under `kubernetes/manifests/` that runs the entire Unheaded
Kingdom on Kubernetes, mirroring:

- `docker-compose.yml` — the registry-aligned dev stack (Doom-Range ports).
- `docker/docker-compose.loadbalancers.yml` — HAProxy edge/internal + nginx sidecars.
- `docker/telemetry/docker-compose.telemetry.yml` — Prometheus/VM/Loki/Grafana/exporters.
- `docker/vllm-rocm/docker-compose.vllm.yml` — vLLM on ROCm (optional overlay).
- `docker/wireguard/docker-compose.wireguard.yml` — east-west overlay (optional overlay).
- `docker/suricata/` — IDS (DaemonSet).

## Service inventory & ports (unchanged from the registry)

| Service | Port(s) | Kubernetes object |
|---------|---------|-------------------|
| wotan | 18000 HTTP / 18001 gRPC | StatefulSet + Service (+headless) |
| the-well (PostgreSQL) | 5432 | StatefulSet + headless Service + init ConfigMap |
| timeguru | 19000 | Deployment + Service |
| architect | 19001 | Deployment + Service |
| captain | 19002 | Deployment + Service |
| micromanager | 19003 | Deployment + Service |
| monad | 19004 | Deployment + Service |
| sophia | 19005 | Deployment + Service |
| cuirass | 19006 | Deployment + Service |
| dashboard-backend | 20000 | Deployment + Service |
| kanban-app | 20001 | Deployment + Service |
| gateway | 21000 / 21443 | Deployment + Service |
| haproxy-edge | 21080 / 21443 / 21404 | Deployment + Service |
| haproxy-internal | 21081 / 21405 | Deployment + Service |
| prometheus / victoriametrics / loki / grafana | 9090 / 8428 / 3100 / 3000 | Deployments (+PVCs) |
| node-exporter / promtail | 9100 / 9080 | DaemonSets |
| clickhouse / vector | 8123,9000 / — | StatefulSet / Deployment |
| suricata | hostNetwork | DaemonSet |
| vllm-deepseek *(overlay)* | 20100 | Deployment + Service |
| wireguard *(overlay)* | 51820/udp | DaemonSet |

## Concept mapping

| Docker concept | Kubernetes concept |
|----------------|--------------------|
| Compose service | Deployment (or StatefulSet for stateful) + Service |
| `image:` / `build:` | container image from a registry (`images:` overlay overrides) |
| `ports: "H:C"` host map | NodePort (dev) / LoadBalancer (prod) — only where a host port existed |
| `depends_on: service_healthy` | `initContainers` (wait-for-*) + readiness/liveness probes |
| `healthcheck:` | `readinessProbe` / `livenessProbe` on `/ready` and `/health` |
| `.env.shared` | `ConfigMap` (non-secret) + `Secret` (placeholders) |
| `read_only: true` + `tmpfs:` | `readOnlyRootFilesystem: true` + `emptyDir` mounts |
| `cap_drop: [ALL]` / `cap_add:` | `securityContext.capabilities.drop/add` |
| `no-new-privileges` | `allowPrivilegeEscalation: false` |
| bridge networks (control/data/observe) | NetworkPolicy default-deny + explicit allows |
| named volumes | PersistentVolumeClaims / volumeClaimTemplates |
| `docker compose up -d` | `kubectl apply -k manifests/overlays/<env>` |
| `docker/Makefile` build/push | image build loop + overlay `images:` block |
| Docker `coredns` service | the cluster's own CoreDNS (do not duplicate) |
| nginx per-app sidecars | native ClusterIP Services + optional Ingress |

## Deliberate divergences (and why)

- **nginx per-service sidecars → ClusterIP Services.** In Kubernetes a Service already
  L4-load-balances across pod replicas, so six nginx proxies that each fronted one
  backend would be an anti-pattern. The JSON-access-log / circuit-breaker behavior is
  recovered at the edge (HAProxy) or via an Ingress controller. An `Ingress` manifest
  is provided as the idiomatic edge.
- **HAProxy edge kept as a Deployment** for fidelity, with `Ingress` offered as the
  idiomatic alternative. Choose one — both binding 21443 would collide.
- **Docker CoreDNS dropped** — the cluster provides DNS; a second copy is redundant.
- **Single multi-stage `Dockerfile` targets** (`docker-compose.yml`) are the build
  source, not the per-service Dockerfiles in `docker/services/` — that's what the
  registry-aligned dev stack uses, and its ports already match the Doom Range.

## Hardening posture

Mirrors the NixOS baseline in `CLAUDE.md`: nonroot, no privilege escalation,
read-only root FS where feasible, all caps dropped, `seccomp: RuntimeDefault`, and a
namespace-wide default-deny network stance with explicit, label-driven allows. Because
the Doom Range uses ports > 1024, the services need no `NET_BIND_SERVICE`. Five
workloads carry documented, minimal exceptions (Suricata, node-exporter, promtail, and
the optional vLLM/WireGuard overlays).

## Next steps

See [`BUILD-PLAN.md`](BUILD-PLAN.md) §9 for the six decisions you must make (registry,
StorageClass, CNI, GPU nodes, edge type, secret backend) and
[`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md) for day-2 operations.
