<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Quick Start — Unheaded on Kubernetes

The fastest path from an existing cluster to a running Kingdom. If you need to build
the cluster itself first, start with [`BUILD-PLAN.md`](BUILD-PLAN.md).

## Prerequisites

- A Kubernetes cluster (v1.27+), `kubectl`, and `kustomize` (bundled with `kubectl`).
- A **CNI that enforces NetworkPolicy** (Calico or Cilium). Without it the
  default-deny posture is silently ignored.
- A **default StorageClass** (the stateful components request `ReadWriteOnce` PVCs).
- A container registry holding the service images (see step 1).
- For NodePort exposure on the real Doom-Range ports, the apiserver must run with
  `--service-node-port-range=3000-32767` (see [`BUILD-PLAN.md`](BUILD-PLAN.md) §4).

## 1. Build & push images

From the repository root (uses the existing multi-stage `Dockerfile` targets):

```bash
REG=ghcr.io/stevenrbellis/unheaded            # ← your registry
for t in wotan timeguru captain architect micromanager monad sophia \
         cuirass dashboard-backend kanban-app gateway; do
  docker build --target "$t" -t $REG/$t:dev . && docker push $REG/$t:dev
done
```

If your registry differs from the default, set it once per overlay via the
`images:` block in `manifests/overlays/dev/kustomization.yaml`.

## 2. Set real secrets

`manifests/base/config/secret-shared.yaml` ships **placeholders only**. Replace them
before exposing anything (`POSTGRES_PASSWORD`, the 7 `the-well-users` passwords,
`GRAFANA_ADMIN_PASSWORD`, `GRAFANA_SECRET_KEY`, `CLICKHOUSE_PASSWORD`). Use SOPS/age,
Sealed Secrets, External Secrets, or Vault — never commit real values.

## 3. Deploy

```bash
kubectl apply -k manifests/overlays/dev
```

This applies, in the order encoded by the kustomizations and enforced at runtime by
`initContainers` + probes:

1. namespace, ConfigMap/Secret, default-deny + allow NetworkPolicies
2. `the-well` (PostgreSQL) and `wotan` (bus) StatefulSets
3. core services (`monad` … `gateway`)
4. HAProxy edge/internal, the telemetry stack, ClickHouse/Vector, Suricata

## 4. Watch it come up

```bash
kubectl -n unheaded get pods -w
kubectl -n unheaded rollout status statefulset/wotan
kubectl -n unheaded rollout status deploy/gateway
```

## 5. Reach the published surfaces

With the `dev` overlay (NodePort on registry ports):

```bash
NODE=<any-node-ip>
curl http://$NODE:21000/health      # gateway
curl http://$NODE:20000/health      # dashboard-backend
curl http://$NODE:20001/health      # kanban — the Meta Moment
open  http://$NODE:3000             # grafana (admin / your secret)
```

## 6. Optional add-ons

```bash
kubectl apply -k manifests/overlays/gpu-vllm    # vLLM/ROCm — needs a GPU node
kubectl apply -k manifests/overlays/wireguard   # east-west fd00:dead:beef::/48
```

## Tear down

```bash
kubectl delete -k manifests/overlays/dev
# PVCs (the-well, wotan, prometheus, loki, victoriametrics, clickhouse) persist by
# design; delete them explicitly to wipe state:
kubectl -n unheaded delete pvc --all
```

## If something's stuck

- Pods `Pending` on volumes → no default StorageClass. See
  [`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md) → Storage.
- Services can't reach each other → CNI doesn't enforce policy, or a pod is missing
  the `kingdom.bus-client` / `kingdom.well-client` label.
- `ImagePullBackOff` → registry/tag mismatch; fix the overlay `images:` block.

More in [`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md).
