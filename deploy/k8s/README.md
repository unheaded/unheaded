<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# `deploy/k8s/` — SK8 Convergence manifests

Raw Kubernetes manifests grouped by Kingdom tier, from the 2026-03-05 "Kubernetes
Convergence" work (`ec8108e2`, battle plan at
`docs/internal/battle-plans/SK8_Unheaded_Kubernetes_Convergence_Battle_Plan.md`).

Layout is by tier and by operational concern rather than by service:

| Path | Contents |
|---|---|
| `armory/` `gnostic/` `presentation/` `ebpf/` | workloads, one directory per Kingdom tier |
| `policies/` | Gatekeeper `ConstraintTemplate`s — see `policies/POLICY_SUMMARY.md` |
| `network-policies/` | `CiliumNetworkPolicy` |
| `rbac/` `secrets/` `node-config/` | cluster-level supporting resources |
| `monitoring/` | `ServiceMonitor`s and dashboards |
| `kind/` | local `kind` cluster definition |
| `templates/` | shared fragments |

Namespaces: `unheaded-armory`, `unheaded-gnostic`, `unheaded-presentation`,
`unheaded-system`, `unheaded-ebpf`.

## This is not the only Kubernetes tree

`kubernetes/` is a second, newer one (2026-06-26, "Kubernetes the hard way,
mirroring the Docker stack"), and it is also still maintained — the most recent
commit touching Kubernetes at all touched both. They are not duplicates:

- **`kubernetes/`** is kustomize (`base/` + `overlays/`), deploys into a single
  `unheaded` namespace, mirrors `docker-compose.yml` service-for-service, and is
  self-documented (`kubernetes/README.md` plus six more files).
- **This tree** carries the policy and governance layer that `kubernetes/` has no
  equivalent for, and is the one every external document points at — ADR-064, the
  runbooks, the compliance control matrices, and the K8s threat model.

Checked 2026-08-04: **zero overlapping `(kind, namespace, name)` tuples** across the
two trees, so applying both to one cluster does not collide. That is a property of
the current contents, not a guarantee anyone enforces — if you add a resource here
in the `unheaded` namespace, check `kubernetes/manifests/` first.

Neither has been declared canonical. Consolidating them would lose something either
way, so it is recorded as **D12** in
`docs/battle-plans/STAGING-LADDER-DECISIONS.md` rather than settled here.
