# CIS Kubernetes Benchmark — Scope and Recipe (Kind Cluster)

**Date:** 2026-05-06
**Author:** Marshal overnight unattended run (NORTH-STAR Appendix A, D-Tier-6 Step D2)
**Status:** *scope only* — no live kind cluster ran during this Marshal session, so no benchmark output is included. This document defines what to run, where to look, and how to interpret results when D-Tier-6 lands a live-cluster session.

---

## Why scope-only tonight

The kind cluster is brought up by `deploy/k8s/kind/bring-up.sh` and explicitly torn down at end-of-session per WAVE17 wake-up summary ("Kind cluster torn down at end. No live infra disturbed."). The Marshal's overnight unattended run does not bring up infrastructure that touches Docker / kernel-net interfaces; doing so without a human on standby would leak state.

When D-Tier-6 actually executes, the recipe below is the authoritative procedure.

---

## Recipe

### Pre-requisites

- `kind` v0.23+ on the host (already required by `bring-up.sh`)
- `kubectl` matching the kind cluster's kube-apiserver
- `kube-bench` (https://github.com/aquasecurity/kube-bench) — installable via `brew install kube-bench` on macOS or via the `aquasec/kube-bench` Docker image
- Shell session with the kubeconfig pointing at the unheaded kind cluster (`kind export kubeconfig --name unheaded`)

### Step 1 — Bring up the cluster

```bash
./deploy/k8s/kind/bring-up.sh
kubectl cluster-info --context kind-unheaded
kubectl get nodes
# expect: 1 control-plane + 2 worker nodes Ready
```

### Step 2 — Run kube-bench (controllers only — kind doesn't expose worker components the same way)

```bash
# kube-bench understands the kind layout via its --benchmark flag and node target
kube-bench --benchmark cis-1.7 \
           --targets master,etcd,policies \
           --json | tee sbom-results/kube-bench-2026-05-06-master.json

kube-bench --benchmark cis-1.7 \
           --targets node,policies \
           --json | tee sbom-results/kube-bench-2026-05-06-worker.json
```

(Use `cis-1.7` if the running k8s version is 1.27+; choose the closest benchmark to the actual k8s release. `kind` typically tracks one minor release behind upstream.)

### Step 3 — Aggregate findings

```bash
# Pull failed checks across both reports
jq -r '.Controls[] | .tests[] | .results[] | select(.status=="FAIL") | "\(.id): \(.test_desc)"' \
   sbom-results/kube-bench-*.json | sort -u
```

### Step 4 — File issues per failed control with severity from the benchmark

CIS controls are pre-rated (Level 1 / Level 2). Treat failed Level 1 controls as P1 for production deployment; Level 2 as P2 hardening backlog.

---

## Expected coverage map

The kind cluster's running components define which CIS sections will return meaningful results:

| CIS Section | What it checks | Will it run on kind? |
|-------------|----------------|----------------------|
| 1.x — Control-plane components | apiserver flags, scheduler/controller-manager TLS, audit logs | Yes (control-plane is a pod inside the kind container) |
| 2.x — etcd | client/peer TLS, encryption-at-rest, auto-tls | Yes |
| 3.x — Control-plane configuration | RBAC defaults, service-account-token-volume defaults | Yes |
| 4.x — Worker node | kubelet flags, kube-proxy, CNI | Partial — kindnet is non-standard; expect Section 4 to flag the CNI as "informational" |
| 5.x — Policies | NetworkPolicy presence, RBAC, PSA labels, secrets management, image policy | Yes |

The threat model in `k8s-threat-model-2026-05-06.md` predicts the failures: 2.1.x (etcd encryption), 1.2.x (apiserver audit), 5.x (NetworkPolicy enforcement, PSA labels, image-pull policy).

---

## Predicted kube-bench failures

Based on the manifests audited tonight, expect these to FAIL when live-run:

- **1.2.21** — `--audit-log-path` argument not set
- **1.2.22** — `--audit-log-maxage` not set
- **1.2.31** — `--encryption-provider-config` not set
- **2.1** — Multiple etcd encryption-at-rest checks
- **5.1.x** — RBAC over-grants (the `unheaded-armory` ClusterRole — see `cis-k8s-rbac-review-2026-05-06.md`)
- **5.2.x** — PodSecurity admission labels not set on `unheaded` namespace
- **5.3.2** — `automountServiceAccountToken: true` (default) — applicability per Deployment
- **5.7.x** — Secrets management (etcd encryption + sourcing)

These are the ones to watch for. If the live run produces *more* failures than predicted, that's signal of a finding the threat model missed.

---

## Outputs

`kube-bench --json` writes to `sbom-results/`:

- `kube-bench-2026-05-06-master.json`
- `kube-bench-2026-05-06-worker.json`

These should be uploaded as CI artefacts (the `.github/workflows/security.yml` daily 06:00 UTC run is the natural home — extend it to cron-bring-up + bench + tear-down on kind).

---

## When NOT to run kube-bench

- During an active dev session — bringing up the cluster, running the bench, and tearing down adds ~5 minutes; not worth the operator's flow.
- Without a paired threat-model entry — the bench output without context is just a list of control IDs.

The right cadence: weekly, alongside `sbom-audit.yml`'s weekly run, OR on every Helm-chart-touching PR that modifies `deploy/k8s/` or `helm/`.

---

## Hand-off

Pair this with `k8s-threat-model-2026-05-06.md` and `cis-k8s-rbac-review-2026-05-06.md` (D3). The trio is the doc artefact that lets BlackMage + MoatGhost work the cluster surface without rediscovering it.

**Owner for live run:** BlackMage (with MoatGhost reading the failure deltas for compliance mapping).
