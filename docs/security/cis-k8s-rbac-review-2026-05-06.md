# Kind Cluster RBAC Review

**Date:** 2026-05-06
**Author:** Marshal overnight unattended run (NORTH-STAR Appendix A, D-Tier-6 Step D3)
**Method:** read-only audit of every `Role | RoleBinding | ClusterRole | ClusterRoleBinding | ServiceAccount | serviceAccountName | automountServiceAccountToken` reference under `helm/unheaded/` and `deploy/k8s/`. No live cluster query.
**Status:** *findings only* — no manifests modified.

---

## Inventory

| Type | Object | Source | Notes |
|------|--------|--------|-------|
| ClusterRole | `unheaded-armory` | `deploy/k8s/rbac/armory-role.yaml` | The only ClusterRole shipped. |
| RoleBinding / ClusterRoleBinding | **none in repo** | — | The ClusterRole has no Binding shipped — it grants nothing today unless a Binding lands at apply-time. |
| ServiceAccount | **none declared in YAML** | — | The named SAs `pleroma`, `anamnesis`, `kenoma`, `cuirass` are *referenced* (`serviceAccountName: <name>`) but their `kind: ServiceAccount` resources are not in the tree. |
| `automountServiceAccountToken: false` | 4 Deployments | `deploy/k8s/gnostic/{pleroma,anamnesis,kenoma}-deployment.yaml`, `deploy/k8s/armory/cuirass-deployment.yaml` | Good — these opt out of token auto-mount. |
| `automountServiceAccountToken` set | helm Deployments | **none** | Bad — helm-deployed services (`helm/unheaded/templates/{services,wotan}.yaml`) don't set this; they inherit the namespace default (auto-mount = ON). |

---

## Findings

### F1 — `unheaded-armory` ClusterRole over-grants `update`/`patch` on Deployments cluster-wide [HIGH]

```
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

The role is intended for the armory operator (per file's location), but the verbs are unrestricted by namespace. A SA bound to this ClusterRole could:
- Patch any Deployment image in any namespace → cluster-wide image-swap attack.
- Patch `spec.template.spec.serviceAccountName` to escalate to a more-privileged SA.
- Set `replicas: 0` to disable any cluster service.

**Mitigation:** convert to `kind: Role` (namespaced to `unheaded`), and create a `RoleBinding` only inside that namespace. If cross-namespace operator scope is genuinely required, restrict via `resourceNames` to specific Deployments OR via an admission webhook.

### F2 — No RoleBinding / ClusterRoleBinding for `unheaded-armory` exists in the repo [INFO + risk]

The ClusterRole is shipped without a Binding. **Today it grants nothing.** But this is fragile:
- If an operator manually `kubectl apply`s a ClusterRoleBinding for testing, the broad permissions in F1 take effect immediately.
- If the chart eventually ships a Binding, F1's mitigation must be in place first.

**Recommendation:** add a *deliberately-narrow* RoleBinding to the chart now — bound to a specific SA in the `unheaded` namespace — so the *intent* is documented in YAML rather than applied ad hoc.

### F3 — Named ServiceAccounts (`pleroma`, `anamnesis`, `kenoma`, `cuirass`) are referenced but not declared [MEDIUM]

`pleroma-deployment.yaml` references `serviceAccountName: pleroma`. There is no `kind: ServiceAccount` resource in the tree creating that account. Behaviour at apply-time:
- If the SA exists (created out of band) → fine.
- If it doesn't → the pod fails with "serviceAccount X not found" (kubectl), or worse, silently inherits `default` (depending on cluster admission).

**Recommendation:** create a per-service `kind: ServiceAccount` YAML for each named SA. Today's WAVE17 kind smoke-test must be implicitly creating them via Helm post-render or relying on Kind defaults — verify and document the actual flow.

### F4 — Helm-deployed services (`helm/unheaded/templates/{services,wotan}.yaml`) don't set `automountServiceAccountToken: false` [MEDIUM]

The `deploy/k8s/gnostic` and one `deploy/k8s/armory` deployment opt out of token auto-mount. The helm-deployed services do not. If those services don't actually call kube-apiserver (most don't), they're carrying a credential they never use — every pod compromise leaks an SA token to the attacker.

**Recommendation:** in `helm/unheaded/templates/services.yaml` and `wotan.yaml`, set `automountServiceAccountToken: false` at the pod-spec level. If a service genuinely needs API access (e.g. the armory operator), set explicitly to `true` with a comment explaining why.

### F5 — No PodSecurityAdmission labels on `unheaded` namespace [HIGH]

(Cross-reference from `k8s-threat-model-2026-05-06.md` §3.5.) The `helm/unheaded/templates/namespace.yaml` does not set `pod-security.kubernetes.io/enforce`. Without it, privileged or hostPath pods can be admitted, bypassing the rest of the RBAC posture.

**Recommendation:** add `pod-security.kubernetes.io/enforce: restricted` (or `baseline` for a less-strict initial rollout) to the namespace metadata.

### F6 — `unheaded-armory` ClusterRole verbs include `pods/log` get cluster-wide [LOW]

```
- apiGroups: [""]
  resources: ["pods/log"]
  verbs: ["get"]
```

Cluster-wide log read = leaks sensitive info from any namespace. If the armory operator only needs logs from `unheaded` services, narrow this with `resourceNames` or namespace-scope this rule.

---

## Least-privilege rewrite (sketch — for daytime review)

```yaml
# Replacement for deploy/k8s/rbac/armory-role.yaml
# Namespaced; one verb-set for reads, one for the specific writes the
# operator actually performs.
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: unheaded-armory
  namespace: unheaded
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "configmaps", "pods/log"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments"]
  resourceNames: ["cuirass", "shield", "greaves"]   # explicit list
  verbs: ["get", "list", "watch", "patch"]          # patch only, drop update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: unheaded-armory
  namespace: unheaded
subjects:
- kind: ServiceAccount
  name: armory-operator
  namespace: unheaded
roleRef:
  kind: Role
  name: unheaded-armory
  apiGroup: rbac.authorization.k8s.io
```

This sketch is for review, **not** the Marshal's recommendation to apply directly. Architect / BlackMage own the final shape; the sketch makes the deltas obvious.

---

## Compliance mapping

For MoatGhost — this RBAC posture maps to:

| Control | Status today | Post-recommendations |
|---------|--------------|----------------------|
| **CIS K8s 5.1.3** — Minimize wildcards in Roles and ClusterRoles | partial fail (cluster-wide write) | PASS |
| **CIS K8s 5.1.6** — Ensure that Service Account Tokens are only mounted where necessary | partial pass (4 of N deployments opt out) | PASS |
| **NIST SP 800-53 AC-6** — Least Privilege | partial | PASS |
| **SOC2 CC6.1** — Logical access controls | partial | PASS |

---

## Provenance

Read-only audit. No `kubectl auth can-i`, no live cluster verification. Sources:
- `deploy/k8s/rbac/armory-role.yaml`
- `deploy/k8s/gnostic/{pleroma,anamnesis,kenoma}-deployment.yaml`
- `deploy/k8s/armory/cuirass-deployment.yaml`
- `helm/unheaded/templates/{services,wotan,namespace}.yaml`

Hand-off: this is the input D2 (CIS k8s-bench live run) will use as the predicted-failure baseline for Section 5.x controls.
