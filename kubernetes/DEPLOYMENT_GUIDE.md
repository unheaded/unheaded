<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Unheaded Kingdom — Kubernetes Deployment Guide

Operating Unheaded on Kubernetes: structure, overlays, secrets, storage, networking,
scaling, and troubleshooting. For first-cluster bring-up see
[`BUILD-PLAN.md`](BUILD-PLAN.md); for the fast path see [`QUICKSTART.md`](QUICKSTART.md).

## 1. How the manifests are organized

`manifests/base/` holds one self-contained kustomization per component. The top-level
`manifests/base/kustomization.yaml` composes them in dependency order and stamps
`app.kubernetes.io/part-of: unheaded` on everything. `manifests/overlays/` layers
environment-specific changes on top of the base — never edit base for environment
differences.

```
overlays/dev   → NodePort exposure on Doom-Range ports, LOG_LEVEL=debug, :dev images
overlays/prod  → LoadBalancer edge, replicas=2 for stateless services, pinned tags
overlays/gpu-vllm  → optional vLLM/ROCm (amd.com/gpu)
overlays/wireguard → optional east-west DaemonSet
```

Render before applying to see exactly what you'll get:

```bash
kubectl kustomize manifests/overlays/dev | less
```

## 2. Images

The base references `ghcr.io/stevenrbellis/unheaded/<service>:dev`. Override registry
and tag per overlay:

```yaml
# overlays/<env>/kustomization.yaml
images:
  - name: ghcr.io/stevenrbellis/unheaded/wotan
    newName: registry.example.com/unheaded/wotan
    newTag: v1.0.0
```

Third-party images (postgres, prometheus, grafana, loki, victoria-metrics,
clickhouse, vector, haproxy, busybox, node-exporter, promtail, suricata) are pinned to
the same versions the Docker stack uses.

## 3. Secrets

`base/config/secret-shared.yaml` and the `the-well-users` Secret are **placeholders**.
Production options:

- **SOPS + age** — matches the repo's existing secret convention.
- **Sealed Secrets** — commit encrypted, controller decrypts in-cluster.
- **External Secrets Operator** — sync from Vault / cloud secret managers.

Keys to set: `POSTGRES_PASSWORD`, `APP_KANBAN_PASSWORD`, `APP_TIMEGURU_PASSWORD`,
`APP_ZHEN_PASSWORD`, `OPS_WRITER_PASSWORD`, `OPS_READER_PASSWORD`,
`CONFIG_ADMIN_PASSWORD`, `CONFIG_READER_PASSWORD`, `GRAFANA_ADMIN_PASSWORD`,
`GRAFANA_SECRET_KEY`, `CLICKHOUSE_PASSWORD`. The 7 DB passwords feed The Well's init
script, which creates the 3 databases (`unheaded_app` / `unheaded_ops` /
`unheaded_config`) and 7 least-privilege users exactly as `db/migrations/` does.

## 4. Storage

These components request PVCs (`ReadWriteOnce`): `the-well` (5Gi), `wotan` (1Gi),
`prometheus` (10Gi), `loki` (10Gi), `victoriametrics` (10Gi), `clickhouse` (10Gi).
Ensure a default StorageClass exists:

```bash
kubectl get storageclass            # one should be (default)
```

Bump sizes per environment with a `patches` entry targeting the PVC / volumeClaimTemplate.

## 5. Networking & policies

The namespace runs **default-deny ingress + egress**. Explicit allows live in
`base/network-policies/` and key off pod labels:

| Label | Grants |
|-------|--------|
| `kingdom.bus-client: "true"` | egress to `wotan` 18000/18001 (and Wotan accepts it) |
| `kingdom.well-client: "true"` | egress to `the-well` 5432 (and The Well accepts it) |
| `kingdom.tier: observability` | scrape ingress to every pod; egress from observability |
| `kingdom.role: edge` / `gateway` | reach published surfaces / fan out to services |
| `kingdom.published: "true"` | accept ingress from edge + gateway |

All pods get DNS egress to `kube-system` CoreDNS. **Your CNI must enforce
NetworkPolicy** (Calico/Cilium) — Flannel alone will not.

Pod Security Admission is `restricted` on the namespace. New workloads must satisfy
it or they will be rejected.

## 6. Exposure

ClusterIP is the default. Host ports are published **only** where Docker mapped one:

- **dev** — `gateway` (21000/21443), `dashboard-backend` (20000), `kanban-app`
  (20001), `grafana` (3000) become NodePorts on those exact ports (requires the
  widened `--service-node-port-range`).
- **prod / on-prem (recommended)** — the **HAProxy Kubernetes Ingress Controller**
  (`base/haproxy-ingress/`, applied as a standalone kustomization in its own
  `haproxy-controller` namespace) is the edge. It serves `loadbalancers/ingress.yaml`
  via `ingressClassName: haproxy` and exposes a NodePort on 21080/21443/21404, so the
  external entry is `https://<node-ip>:21443` with no cloud LoadBalancer. This
  supersedes the standalone `haproxy-edge` and removes the old 21443 collision.

## 7. Scaling & availability

Stateless services scale horizontally — the `prod` overlay sets `replicas: 2` for
`monad`, `sophia`, `gateway`, `dashboard-backend`. Scale others with a `replicas:`
entry. `wotan` and `the-well` are single-writer StatefulSets; treat replication of
those as a separate design exercise (Postgres HA, Wotan clustering) rather than a bare
`replicas` bump. Add `PodDisruptionBudget`s and anti-affinity for production HA.

## 8. Observability

Prometheus discovers pods via the `prometheus.io/scrape` annotation pattern and has a
ServiceAccount + ClusterRole for in-cluster SD. Grafana, Loki, VictoriaMetrics round
out the stack; `node-exporter` and `promtail` run as DaemonSets. Every service already
exposes `/health`, `/ready`, and `/metrics` per the platform standard.

## 9. Upgrades

```bash
# bump the image tag in the overlay, then:
kubectl apply -k manifests/overlays/prod
kubectl -n unheaded rollout status deploy/<service>
kubectl -n unheaded rollout undo deploy/<service>    # if needed
```

Rolling updates are the default Deployment strategy. StatefulSets roll one pod at a
time, honoring ordering.

## 10. Troubleshooting

| Symptom | Likely cause / fix |
|---------|--------------------|
| Pods `Pending` | No default StorageClass, or insufficient node resources. |
| `ImagePullBackOff` | Registry/tag mismatch — fix overlay `images:`; check pull secret. |
| Service A can't reach B | CNI not enforcing policy, or missing `kingdom.*` label. |
| `wait-for-wotan` init never finishes | Wotan not Ready; `kubectl -n unheaded logs sts/wotan`. |
| Pod rejected on admission | Violates PSA `restricted` — fix its securityContext. |
| Suricata/node-exporter `CrashLoop` | Host networking/caps unavailable on that node. |

Useful commands:

```bash
kubectl -n unheaded get pods,svc,netpol,pvc
kubectl -n unheaded describe pod <pod>
kubectl -n unheaded logs deploy/<service> -c <service>
kubectl -n unheaded exec -it deploy/dashboard-backend -- wget -qO- http://wotan:18000/health
```

## 11. Decisions checklist

- [ ] Image registry set (overlay `images:`)
- [ ] Real secrets in place (SOPS/Sealed/External/Vault)
- [ ] Default StorageClass present
- [ ] NetworkPolicy-enforcing CNI installed **before** applying policies
- [x] Edge: **HAProxy Kubernetes Ingress Controller** (`base/haproxy-ingress/`) — apply standalone; provide a TLS Secret for a real cert (built-in self-signed otherwise)
- [ ] (optional) GPU node labelled + AMD device plugin for `gpu-vllm`
- [ ] (optional) WireGuard keys provided as `wireguard-config` Secret
