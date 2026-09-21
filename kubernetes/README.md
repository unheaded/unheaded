<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Unheaded Kingdom — Kubernetes

Kubernetes manifests that run the **entire** Unheaded Kingdom, mirroring the Docker
stack one-for-one but expressed as hardened, declarative kustomize resources. Pair
this with [`BUILD-PLAN.md`](BUILD-PLAN.md) — a *Kubernetes The Hard Way* runbook that
brings up the control plane by hand and then deploys everything here.

> Configuration management automation platform. Free to use. Free to share. GPL-3.0.

## This is not the only Kubernetes tree

`deploy/k8s/` is a second, older one (2026-03-05, the "SK8 Convergence"), and it is
still maintained — the most recent commit touching Kubernetes at all touched both.
They do not conflict and are not duplicates of each other:

| | `kubernetes/` (here) | `deploy/k8s/` |
|---|---|---|
| Shape | kustomize `base/` + `overlays/` | raw manifests, grouped by Kingdom tier |
| Namespace | `unheaded` (+ `haproxy-controller`) | `unheaded-armory`, `-gnostic`, `-presentation`, `-system`, `-ebpf` |
| Emphasis | mirror the Docker stack service-for-service | policy and governance — Gatekeeper `ConstraintTemplate`s, `CiliumNetworkPolicy`, PDBs, `ServiceMonitor`s |
| Docs | self-contained (this file + 6 more) | cited by ADR-064, the runbooks, the compliance control matrices and the K8s threat model |

Checked 2026-08-04: **zero overlapping `(kind, namespace, name)` tuples** across the
two trees, so applying both to one cluster does not collide. That is a property of
the current contents, not a guarantee anyone enforces — if you add resources here in
a `unheaded-*` namespace, check `deploy/k8s/` first.

Neither has been declared canonical. Consolidating them is a real decision with real
losses either way — this tree has the kustomize structure and the documentation,
that one has the policy layer and every external reference — so it is recorded as
**D12** in `docs/battle-plans/STAGING-LADDER-DECISIONS.md` rather than settled here.

## What runs here

Everything in `docker-compose.yml` plus the four `docker/**/docker-compose.*.yml`
stacks, translated to Kubernetes:

- **Stateful spine** — `wotan` (message bus, gRPC 18001 / HTTP 18000) and `the-well`
  (PostgreSQL: 3 databases, 7 service-scoped users), both as `StatefulSet`s.
- **Core services** — `monad` `sophia` `timeguru` `architect` `captain`
  `micromanager` `cuirass` `dashboard-backend` `kanban-app` `gateway`, each a
  `Deployment` + ClusterIP `Service`, on their canonical **Doom Range** ports.
- **Edge / load balancing** — HAProxy edge + internal (`Deployment`+`Service`), plus
  an idiomatic `Ingress` alternative.
- **Observability** — Prometheus, VictoriaMetrics, Loki, Grafana, plus
  `node-exporter` and `promtail` `DaemonSet`s; ClickHouse + Vector for log storage.
- **Security** — Suricata IDS `DaemonSet`.
- **Optional overlays** — `gpu-vllm` (vLLM on ROCm, `amd.com/gpu`) and `wireguard`
  (east-west `fd00:dead:beef::/48` overlay).

Ports never deviate from `pkg/ports/ports.go`. See
[`MANIFEST.md`](MANIFEST.md) for the full file and port inventory.

## Layout

```
kubernetes/
├── BUILD-PLAN.md            # Kubernetes The Hard Way, adapted to Unheaded
├── README.md   QUICKSTART.md   DEPLOYMENT_GUIDE.md
├── KUBERNETES_SETUP_SUMMARY.md   INDEX.md   MANIFEST.md
└── manifests/
    ├── base/                # one folder per component, each its own kustomization
    │   ├── namespace/  config/  network-policies/
    │   ├── the-well/  wotan/    # StatefulSets
    │   ├── monad/ sophia/ timeguru/ architect/ captain/ micromanager/
    │   ├── cuirass/ dashboard-backend/ kanban-app/ gateway/
    │   ├── loadbalancers/   telemetry/   logging/   suricata/
    │   └── kustomization.yaml
    └── overlays/
        ├── dev/             # NodePort exposure, LOG_LEVEL=debug
        ├── prod/            # LoadBalancer edge, replicas, pinned tags
        ├── gpu-vllm/        # optional: vLLM/ROCm
        └── wireguard/       # optional: east-west overlay
```

> `KUBERNETES_SETUP_SUMMARY.md` replaces the old `DOCKER_SETUP_SUMMARY.md`.

## Quick start

```bash
# 1. Build & push the 11 service images (see BUILD-PLAN.md §1)
# 2. Have a cluster with a NetworkPolicy-enforcing CNI + a default StorageClass
kubectl apply -k manifests/overlays/dev
kubectl -n unheaded get pods
curl http://<node-ip>:20001/health     # the Kanban "Meta Moment"
```

Full walkthrough in [`QUICKSTART.md`](QUICKSTART.md); production detail in
[`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md).

## Hardening baseline

Every workload mirrors the NixOS hardening baseline from the root `CLAUDE.md`:
`runAsNonRoot`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem` (where
feasible), `capabilities.drop: [ALL]`, `seccompProfile: RuntimeDefault`. The Doom
Range deliberately uses ports > 1024, so **no `NET_BIND_SERVICE` is needed** for the
services. Network posture is **default-deny** ingress+egress with explicit allows —
the Kubernetes expression of the Docker firewall/default-deny stance. Documented
exceptions (Suricata, host-metric DaemonSets, GPU/WireGuard overlays) keep every
restriction they can and add back only what they must.

## License

GPL-3.0-or-later. See the repository `LICENSE`.
