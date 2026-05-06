# K8s Substrate Threat Model — Post-WAVE17

**Date:** 2026-05-06
**Author:** Marshal overnight unattended run (NORTH-STAR Appendix A, D-Tier-6 Step D1)
**Owners (handoff):** BlackMage (offensive), MoatGhost (compliance/policy)
**Substrate scoped:** the 3-node kind cluster brought up by `deploy/k8s/kind/bring-up.sh`, parameterized by `helm/unheaded/`. Forward-applicable to non-kind production K8s once `helm/unheaded/values-prod.yaml` activates.
**Status:** *initial draft* — doc-only; the Marshal authors the surface, BlackMage will pen-test, MoatGhost will map controls.

---

## 1. Scope and assumptions

In scope:
- **Control plane** of the kind cluster: `kube-apiserver`, `etcd`, `kube-scheduler`, `kube-controller-manager` (all run as pods on the control-plane node in kind).
- **Data plane**: `kindnet` CNI; Unheaded service pods on 2 worker nodes.
- **Helm chart `helm/unheaded/`**: namespace, services, network policies, RBAC, secrets surface.
- **Ingress**: NodePort exposure mapped on the kind container (`30080`, `30081`), and the optional `gateway` resource (per the chart's NetworkPolicy template).
- **RBAC**: ClusterRole `unheaded-armory` (read/write on pods/services/configmaps/deployments).

Out of scope:
- The host kernel below kind (handled by Mímir + heimdall-daemon — Mímir's Law).
- Anything Wotan-internal (handled by Wotan's own threat model — `pkg/wotan/`'s topic-signing + ML-DSA-65 work).
- Production cloud K8s control-plane operator security (GKE/EKS/AKS) — separate ADR territory.
- The eBPF kernel surface (handled by ADR-007 hardening + LICH campaigns).

Assumptions:
- Operators run kind locally during development; production will eventually move to a self-managed K8s (per ADR-040 Kubernetes Ecosystem Strategy) but that is **not the substrate WAVE17 proved** — WAVE17 proved kind only.
- Default `kindnet` CNI is in use. The cluster config (`deploy/k8s/kind/cluster-config.yaml`) explicitly notes Calico/Cilium are a future ADR; **NetworkPolicy enforcement guarantees are aspirational on kind today**.
- The cluster is intended to be torn down between sessions (`kind delete cluster --name unheaded`); persistent state survives only when the operator chooses to keep it.

---

## 2. Trust zones

| Zone | Tenants | Egress allowed? | Ingress allowed? |
|------|---------|-----------------|-------------------|
| Z0: Host | Operator's macOS / Linux dev box | yes (everything) | restricted to kind port mappings |
| Z1: kind container (Docker) | The 3 K8s nodes | yes (Docker bridge) | NodePort mapped 30080/30081 |
| Z2: kube-apiserver / etcd | Kubernetes control-plane components | inside kind container only | only via apiserver TLS, internal cluster IP |
| Z3: Unheaded namespace | 9 service pods | constrained by NetworkPolicy `allow-internal` (intra-namespace + DNS) | constrained by NetworkPolicy default-deny-all + per-service allowlists |
| Z4: External (the Internet) | n/a | n/a | n/a (kind cluster has no public exposure) |

Trust flows downward: Z0 trusts Z1, Z1 trusts Z2, Z2 issues credentials to Z3.

---

## 3. STRIDE-by-component

### 3.1 kube-apiserver

| Threat | Severity (with current controls) | Notes |
|--------|----------------------------------|-------|
| **S** — Spoofing of kubelet/operator identity | Medium | TLS by default in kind; client cert auth. **Gap:** no audit logging configured; an unauthenticated apiserver request is invisible. |
| **T** — Tampering with admission decisions | Medium | No PSA/PSS labels on `helm/unheaded/templates/namespace.yaml`. Pod admission would accept privileged/host-path pods today. |
| **R** — Repudiation | High | No apiserver audit log shipped. **Gap:** add `--audit-log-path` + `--audit-policy-file` for kind; pipe to Wotan in production. |
| **I** — Info disclosure (e.g. leaked tokens) | Medium | ServiceAccount tokens auto-mounted by default (`automountServiceAccountToken: true`). Some service pods do not need API access; should opt out per-deployment. |
| **D** — DoS via API floods | Medium | No `--max-mutating-requests-inflight` or rate limit configured for kind; relies on kindnet/CRI defaults. |
| **E** — Escalation via misconfigured RBAC | **High** | `unheaded-armory` ClusterRole grants `update`/`patch` on Deployments cluster-wide. If any service pod's SA bound to it is compromised, the attacker can swap any image cluster-wide. **See §5 recommendation 1.** |

### 3.2 etcd

| Threat | Severity | Notes |
|--------|----------|-------|
| **S/T** | Medium | etcd in kind is on the control-plane pod; client/peer TLS enabled by default. |
| **R** | High | No etcd watch-event logging, no append-only retention. Compromise of etcd would not leave a forensic trail. |
| **I** | High | etcd holds **all** cluster secrets (Secret resources). At rest encryption: **not configured by kind default**. **Gap:** add `--encryption-provider-config` for any production-bound deployment; for kind, accept the gap and document it. |
| **D** | Low | kind cluster is single-replica etcd; outage = whole-cluster outage but no attacker leverage in dev. Production must run 3-node etcd quorum. |
| **E** | Medium | Direct etcd access bypasses RBAC and admission. Anyone with `kubectl exec` on the control-plane pod has root over the cluster. |

### 3.3 NetworkPolicy + kindnet CNI

| Threat | Severity | Notes |
|--------|----------|-------|
| **Effective enforcement** | **High concern** | The chart ships a sound `default-deny-all` + `allow-internal` template, but the kind config explicitly notes: *"Calico/Cilium are future ADR if we need NetworkPolicy enforcement guarantees."* Translation: **NetworkPolicy may be advisory under default kindnet**. Pen-testing should confirm whether kindnet enforces or no-ops the policies. |
| **Cross-pod lateral movement** | Medium | If kindnet does enforce, intra-namespace allow-internal is permissive — any compromised pod can reach all 8 siblings. Mitigated by Wotan ML-DSA-65 topic signing on `config.*` topics; gap on `drift.*` topics (per heimdall TODO #2 parked tonight). |
| **Egress exfiltration** | Medium | `allow-internal` egress allows any-to-any inside the namespace + DNS only. External egress is blocked. **Good.** Verify with a `nc` from a pod to `1.1.1.1:80` — should fail. |
| **Ingress from outside** | Low (kind) / High (prod) | NodePorts 30080/30081 are exposed on the kind container only (host-local). For production, the gateway/ingress NetworkPolicy template must be tightened. |

### 3.4 Ingress / NodePort surface

| Threat | Severity | Notes |
|--------|----------|-------|
| **S** — TLS termination misconfiguration | Medium | NodePorts 30080/30081 are bare HTTP today. Production should add `cert-manager` or HAProxy edge with TLS 1.3 (per `pkg/transport/`'s gRPC-first cascade). |
| **D** — DDoS via NodePort | Medium (kind: low) | No rate limiting on NodePort. HAProxy edge for production already specs rate limiting (per CLAUDE.md). |
| **I** — Service info via NodePort enumeration | Low | NodePorts 30080/30081 are documented; not a finding. |

### 3.5 RBAC

| Threat | Severity | Notes |
|--------|----------|-------|
| `unheaded-armory` ClusterRole over-grants | **High** | Cluster-wide `update`/`patch` on Deployments is excessive for the intent (per the file comment, it serves the armory operator). **See §5 recommendation 1.** |
| ServiceAccount auto-mount | Medium | Most pods don't need API access. Add `automountServiceAccountToken: false` per Deployment that doesn't call kube-apiserver. |
| Default Service Account in `unheaded` namespace | Medium | The chart should ship a per-service SA + per-service Role binding model rather than letting pods inherit `default`. |

### 3.6 Secrets

| Threat | Severity | Notes |
|--------|----------|-------|
| Plaintext etcd storage | High (prod) / Accepted (kind) | Per §3.2; document as known dev posture. |
| Secrets mounted as env vars | Medium | `cuirass-deployment-with-secrets.yaml` exists — verify it mounts as files (preferred per CLAUDE.md), not env. |
| SOPS/age workflow | n/a | Per CLAUDE.md: "Use SOPS + age for encrypted secrets". Not yet wired into the helm chart values. **Gap:** chart-level secrets sourcing decision is open. |

### 3.7 Image supply chain

| Threat | Severity | Notes |
|--------|----------|-------|
| Image tampering | Medium | The kind cluster pulls images from the local docker daemon (built locally during WAVE17). For production, Sealed Cask (ADR-010) provides SHA-256 manifest verification. **Gap:** no `imagePullPolicy: Never` enforcement in the chart, so a compromised registry could inject. |
| Latest-tag drift | Medium | values-dev.yaml may use `:latest` for convenience; production must pin digests. **Verify** `helm/unheaded/values-prod.yaml`. |

---

## 4. Threat-vector summary

The five highest-severity items, in priority order for pen-test attention:

1. **`unheaded-armory` ClusterRole over-grants `update`/`patch` on Deployments cluster-wide** — single SA compromise = cluster-wide image-swap attack.
2. **NetworkPolicy enforcement is unproven on kindnet** — the chart's `default-deny-all` may be advisory; an attacker exploiting any pod can lateral-move freely if so.
3. **No apiserver audit log + no etcd encryption-at-rest** — forensic + secret-leak gaps that compound on each other.
4. **PodSecurityAdmission / PodSecurityStandards labels missing on `unheaded` namespace** — privileged-pod creation would be admitted, providing a host-escape path.
5. **No image-digest pinning convention enforced by the chart** — supply-chain attack surface.

---

## 5. Recommendations (prioritised)

1. **Tighten `unheaded-armory` ClusterRole.** Convert cluster-wide `update`/`patch` on Deployments to a namespaced Role bound only inside `unheaded`, OR drop write verbs entirely and let the operator service mediate writes via its own admission-friendlier path. **Owner:** Architect + BlackMage.
2. **Confirm kindnet NetworkPolicy enforcement.** Smoke-test from inside a service pod: `nc <other-namespace-pod-IP> <port>` should fail. If it succeeds, switch to Calico/Cilium *before* relying on the chart's NetworkPolicies in production. **Owner:** BlackMage.
3. **Add PodSecurityAdmission labels.** The `helm/unheaded/templates/namespace.yaml` should set `pod-security.kubernetes.io/enforce: restricted` (or at least `baseline` for kind). **Owner:** Architect.
4. **Wire apiserver audit logs to Wotan in non-kind environments.** `--audit-log-path=/var/log/audit.log --audit-policy-file=/etc/k8s/audit-policy.yaml`. Define the audit policy as part of ADR-064's K8s-native deployment. **Owner:** MoatGhost.
5. **Configure etcd encryption-at-rest for any production-bound K8s.** `--encryption-provider-config` with AES-CBC or AES-GCM and key rotation. **Owner:** Architect + MoatGhost.
6. **Pin `imagePullPolicy: IfNotPresent` + image digest** in `values-prod.yaml`. Cross-reference Sealed Cask SHA-256 manifest. **Owner:** Developer.
7. **Add per-service ServiceAccount + per-service Role** rather than relying on `default` SA. Auto-disable token mounting on every Deployment that doesn't call kube-apiserver. **Owner:** Developer.
8. **Document the kind-vs-prod posture explicitly.** Today's `cluster-config.yaml` comment says Calico/Cilium are future. Make that a TODO with an owner + ADR pointer (or inline ADR-064-style decision record). **Owner:** Architect.

---

## 6. Hand-off into D2 + D3

This document defines the surface; D2 (CIS k8s-bench scope) and D3 (RBAC review) refine specific axes:

- **D2** runs CIS Kubernetes Benchmark against the kind cluster. The benchmark covers: control-plane hardening (1.x), etcd (2.x), control-plane configuration (3.x), worker node (4.x), policies (5.x). Tonight's threat-model identifies the *gaps*; D2 measures coverage against industry baseline.
- **D3** narrows specifically into RBAC. Recommendation #1 above is the primary target; D3 should walk every `kind: Role|ClusterRole|RoleBinding|ClusterRoleBinding` shipped by the helm chart and rate each on least-privilege.

---

## 7. Provenance

Read-only audit; no cluster bring-up, no kubectl execution, no remote calls. Sources:
- `deploy/k8s/kind/cluster-config.yaml`
- `deploy/k8s/kind/bring-up.sh`
- `deploy/k8s/kind/values-local.yaml`
- `helm/unheaded/Chart.yaml`
- `helm/unheaded/templates/{namespace,services,wotan,networkpolicy}.yaml`
- `deploy/k8s/rbac/armory-role.yaml`
- `deploy/k8s/armory/*.yaml`, `deploy/k8s/gnostic/*.yaml`
- `docs/battle-plans/WAVE17-WAKEUP-SUMMARY.md`
- CLAUDE.md (security baseline + Sealed Cask + transport policy)

No upstream CIS k8s-bench output, no live cluster scan. Those land in D2.
