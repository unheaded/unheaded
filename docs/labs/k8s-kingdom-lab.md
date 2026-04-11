# K8s Mock-Prod Lab — Kingdom-Mapped, Real-Topology

**Purpose**: Ramp Stevie's K8s knowledge fast via Unheaded mental models AND
build a topology that resembles a real production cluster (HA control plane,
multiple workers, real ingress, real CNI, real observability, real GitOps).
Hardware: WEST + EAST bare metal + LXD containers. Reference:
`~/tmp/kubernetes-the-hard-way/`. Per ADR-047.

**Scope**: Not "kindergarten K8s" — actual mock-prod. HA etcd quorum, multi-AZ
analogue (WEST + EAST as two failure domains), Cilium CNI, cert-manager,
Prometheus stack, Argo CD, sample 3-tier app.

---

## Topology — 8 Nodes Across 2 Bare Metal Hosts

8 LXD containers + 2 bare metal hosts = real distributed cluster.

```
┌──────────────── WEST (14GB / 12-core / GPU) ────────────────┐
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │ jumpbox  │  │  cp-1    │  │ worker-1 │  │ worker-2 │    │
│  │ 512MB    │  │ 2GB etcd │  │ 2GB      │  │ 2GB      │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
│  ┌──────────┐                                              │
│  │  cp-2    │   (Spike daemons coexist on bare metal —    │
│  │ 2GB etcd │    Mímir's Law spike untouched)             │
│  └──────────┘                                              │
└──────────────────────────┬─────────────────────────────────┘
                           │ WireGuard fd00:dead:beef::/48
┌──────────────────────────┴─────────────────────────────────┐
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │  cp-3    │  │ worker-3 │  │ worker-4 │                  │
│  │ 2GB etcd │  │ 1.5GB    │  │ 1.5GB    │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
└──────────────── EAST (8GB / 4-core) ───────────────────────┘
```

**RAM budget**: WEST containers ~9GB, EAST containers ~5GB. Within budget.

**HA story**: 3-node etcd quorum across both hosts. Lose either host → cluster
limps but doesn't die.

**Failure domain story**: WEST failure ≠ EAST failure (different network,
different power if you cared). Realistic for an interview answer.

---

## The Concept Rosetta Stone

Print this. Pin it. Use it as a translation dictionary.

| K8s Concept | Unheaded Equivalent | One-Line "I Already Know This" |
|---|---|---|
| **etcd** | Wotan ring buffer + protocol RAM | Distributed key-value store with watch + revisions |
| **kube-apiserver** | unheaded-daemon HTTP/gRPC | Single REST/gRPC endpoint, all state mutations through it |
| **kube-controller-manager** | Heimdall daemon | Reconciliation loop: observe → compare desired → act |
| **kube-scheduler** | (no equivalent) | Decides which node runs which pod |
| **kubelet** | heimdall-daemon on EAST | Per-node agent: runs containers, reports status |
| **kube-proxy** | iptables / eBPF | Service IP → pod IP NAT/load balance |
| **CRI (containerd)** | LXD / containerd / Docker | Container runtime — already use this |
| **CNI (Cilium/Calico)** | eBPF + WireGuard | Pod networking |
| **CSI** | Phylactery | Storage abstraction |
| **Pod** | An LXD container or 2-3 grouped | Smallest deployable unit |
| **Service** | nginx sidecar / haproxy entry | Stable endpoint for pod set |
| **Ingress** | gateway nginx | External entry, TLS termination |
| **ConfigMap** | `references/*.yaml` | Non-secret config blob |
| **Secret** | SOPS/age / Wotan signed payload | Secret blob |
| **Deployment** | systemd unit + reconciliation | Desired-state spec for replicated pods |
| **StatefulSet** | (no equivalent) | Ordered, named, persistent pods |
| **DaemonSet** | All-node systemd unit | One pod per node (e.g., heimdall-daemon) |
| **Namespace** | Linux namespace + naming convention | Logical scope inside a cluster |
| **Label / Selector** | Wotan topic pattern | Tags + filtering |
| **CRD** | Mjölnir manifest schema | User-defined object type |
| **Operator** | Heimdall daemon for a CRD | Reconciliation loop for a CRD |
| **Helm chart** | (none — we ship Go binaries) | Templated YAML package |
| **GitOps (Argo/Flux)** | `references/baseline/mjolnir.yaml` + Heimdall scan | Git is desired state |
| **Admission webhook** | `services/wotan/internal/signing/` config.* | Validate/mutate before apply |
| **NetworkPolicy** | NixOS firewall + iptables `default deny` | Cluster-wide network rules |
| **Cilium** | Our eBPF + Aya stack | eBPF as the data plane |
| **Hubble** | trace-collector + dashboard | eBPF flow visualization |
| **Prometheus** | logagg + dashboard backend | Metrics scrape + alert rules |
| **Loki** | Wotan ring buffer + log topics | Log aggregation |
| **Grafana** | Unheaded dashboard | Time-series visualization |
| **ArgoCD** | (manual mjolnir.yaml updates today) | Git-driven reconciliation |
| **cert-manager** | `pkg/gungnir` (different crypto) | Cert lifecycle automation |
| **Velero** | Sealed Cask + snapshots | Backup/restore |

---

## Phases (Ordered, Time-Boxed)

### Phase 0 — LXD Topology Provision (~1h)

Stand up 8 containers across both hosts.

**WEST containers** (5):
```bash
for n in jumpbox cp-1 cp-2 worker-1 worker-2; do
  sudo lxc launch ubuntu:22.04 k8s-$n
done
# RAM caps
sudo lxc config set k8s-jumpbox  limits.memory=512MB
sudo lxc config set k8s-cp-1     limits.memory=2GB
sudo lxc config set k8s-cp-2     limits.memory=2GB
sudo lxc config set k8s-worker-1 limits.memory=2GB
sudo lxc config set k8s-worker-2 limits.memory=2GB
```

**EAST containers** (3):
```bash
ssh govan@east 'for n in cp-3 worker-3 worker-4; do sudo lxc launch ubuntu:22.04 k8s-$n; done'
ssh govan@east 'sudo lxc config set k8s-cp-3     limits.memory=2GB'
ssh govan@east 'sudo lxc config set k8s-worker-3 limits.memory=1500MB'
ssh govan@east 'sudo lxc config set k8s-worker-4 limits.memory=1500MB'
```

**Cross-host LXD networking**: each container gets an IP on its host's lxdbr0.
Cross-host reachability via WireGuard `fd00:dead:beef::/48` + iptables FORWARD
or LXD network forwards.

**Verify**:
```bash
sudo lxc list k8s-
ssh govan@east 'sudo lxc list k8s-'
```

**Spike coexistence**: do NOT touch `/opt/spike-mimirs/` on EAST. K8s containers
sit alongside; bare-metal heimdall-daemon keeps running.

### Phase 1 — KTHW Walkthrough (~4h, weekend session)

Walk `~/tmp/kubernetes-the-hard-way/docs/01-13` step by step but rebadge:
- jumpbox = `k8s-jumpbox` (LXD on WEST)
- server = `k8s-cp-1` (we'll add cp-2/cp-3 in Phase 2)
- node-0 = `k8s-worker-1`
- node-1 = `k8s-worker-3` (cross-host worker, exercises WG path)

**This phase produces a working but NON-HA cluster.** Single control plane.
HA comes in Phase 2.

**Skip in Phase 1**: KTHW's "single CA + manual cert generation" is fine for
learning but in Phase 2 we'll move to cert-manager. Don't get attached.

**Goals at end of Phase 1**:
- `kubectl get nodes` shows worker-1 + worker-3 Ready
- `kubectl run nginx --image=nginx && kubectl expose pod nginx --port=80` works
- Cross-host pod scheduling verified (nginx sometimes lands on worker-3 across WG)

### Phase 2 — HA Control Plane (~2h)

Add cp-2 and cp-3 as additional control plane members.

- etcd: join cp-2 + cp-3 to the etcd cluster (quorum = 3)
- apiserver: run on all 3 control plane nodes
- controller-manager + scheduler: leader election active
- haproxy on jumpbox loadbalancing the 3 apiservers (single VIP for kubectl)

**Failure test**: stop kube-apiserver on cp-1. Verify kubectl still works
(haproxy fails over to cp-2 or cp-3). This is the moment HA clicks.

### Phase 3 — Cilium CNI (~1h)

Replace kube-proxy with Cilium. This is the "K8s plugin to eBPF" angle and
the closest analogue to what Unheaded does natively.

```bash
# On jumpbox
helm repo add cilium https://helm.cilium.io/
helm install cilium cilium/cilium --namespace kube-system \
  --set kubeProxyReplacement=true --set k8sServiceHost=$VIP --set k8sServicePort=6443
```

**Compare and contrast (write notes!)**: Cilium's flow visibility (Hubble) vs
your trace-collector dashboard. Cilium's eBPF programs vs your `crates/heimdall-bpf/`.

### Phase 4 — Storage (~1h)

Pick ONE — local-path-provisioner is the "just works" answer, longhorn is the
"real distributed storage" answer.

**Recommendation**: longhorn. It actually behaves like cloud block storage
and exercises cross-host replication.

```bash
helm install longhorn longhorn/longhorn -n longhorn-system --create-namespace
```

**Verify**: create a PVC, attach to a pod, write data, delete pod, recreate
pod, verify data persists.

### Phase 5 — Ingress + cert-manager (~1.5h)

```bash
# nginx-ingress
helm install ingress-nginx ingress-nginx/ingress-nginx -n ingress-nginx --create-namespace \
  --set controller.service.type=NodePort

# cert-manager
helm install cert-manager jetstack/cert-manager -n cert-manager --create-namespace --set installCRDs=true
```

**Compare**: cert-manager + Let's Encrypt vs `pkg/gungnir` + ML-DSA-65. Note
x509-vs-PQ angle for ADR-047 extraction notes.

### Phase 6 — Observability Stack (~2h)

The "kube-prometheus-stack" Helm chart is the standard mock-prod choice.

```bash
helm install monitoring prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace
```

This installs Prometheus + Grafana + AlertManager + node-exporter +
kube-state-metrics + service monitors.

**Add Loki** for logs:
```bash
helm install loki grafana/loki-stack -n monitoring \
  --set grafana.enabled=false
```

**Compare**: this is most of what `dashboard-backend` + `pkg/logagg/` do, but
opinionated and battle-tested. Note for ADR-047: this is the layer where
"YOLO write your own" loses to ecosystem.

### Phase 7 — GitOps (Argo CD) (~1h)

```bash
helm install argocd argo/argo-cd -n argocd --create-namespace
```

Push your `mjolnir.yaml`-equivalent (a YAML directory) to a git repo, point
Argo at it, verify reconciliation.

**This is your THE DECLARATIVE EVERYTHING moment**. Argo CD is what Unheaded's
Heimdall daemon would be if it had a UI and didn't reinvent the wheel.

### Phase 8 — Sample 3-Tier App (~1h)

Deploy a realistic web app: Postgres (StatefulSet) + API (Deployment) +
nginx static frontend (Deployment) + Ingress.

Use one of:
- **Sock Shop** (microservices.io demo) — heavyweight, very prod-like
- **Online Boutique** (Google) — multi-language microservices
- **Guestbook + Postgres** — minimal, faster

**Recommendation**: Online Boutique for interview talking points; Guestbook if
you're tired and want it shipped.

### Phase 9 — Network Policies + RBAC (~1h)

Apply default-deny network policy. Watch your sample app break. Add explicit
allow rules. Watch it heal. **This is the exact same exercise as Unheaded's
NixOS hardening modules — same idea, different syntax.**

Then create a ServiceAccount + RoleBinding for a CI-style user with namespace
scope. RBAC clicks once you do it once.

### Phase 10 — Day 2 Ops Skills (~2h)

The interview-gold list. Practice each:

| Drill | Command |
|---|---|
| Drain a node | `kubectl drain k8s-worker-1 --ignore-daemonsets` |
| Cordon/uncordon | `kubectl cordon` / `kubectl uncordon` |
| Rolling update | `kubectl set image deploy/api api=...:v2` |
| Rollback | `kubectl rollout undo deploy/api` |
| Scale | `kubectl scale deploy/api --replicas=5` |
| Logs across pods | `kubectl logs -l app=api --tail=50 -f` |
| Exec into pod | `kubectl exec -it $POD -- sh` |
| Port-forward | `kubectl port-forward svc/api 8080:80` |
| etcd snapshot | `etcdctl snapshot save backup.db` |
| etcd restore | `etcdctl snapshot restore backup.db` |
| Describe failing pod | `kubectl describe pod $POD` |

### Phase 11 — Disaster Drills (~1h)

- Kill a control plane member. Cluster survives?
- Kill a worker node. Pods reschedule?
- Corrupt etcd on one node. Quorum holds?
- Network-partition WEST and EAST. Each side's behavior?
- Restore from etcd snapshot.

### Phase 12 — Honest Comparison Notes (~1h)

Write `docs/labs/k8s-vs-unheaded-notes.md`:

1. What did K8s do better? (Probably: scheduler, ecosystem, admission control,
   declarative API maturity, RBAC depth)
2. What did Unheaded do better? (Probably: 14GB footprint, eBPF as substrate,
   PQ crypto, programmable wire format)
3. Which Mímir's Law concepts translate cleanly to a K8s operator?
4. Which would be awkward?
5. Which Unheaded components could ship as **K8s plugins/operators** instead
   of standalone? (extraction candidates for ADR-047)

### Phase 13 — Custom Operator (~3h, optional but high-value)

Pick ONE Mímir's Law concept (baseline drift detection is the obvious one).

- Define CRD: `BaselineManifest`
- Write operator in Go using `kubebuilder` or `operator-sdk`
- Reconciliation loop: hash files in pods, compare to manifest, emit events
- Deploy your operator into the cluster
- Test against a sample workload

**This phase is the moment Unheaded's spike work and K8s converge in your
head.** You will VISCERALLY understand the overlap.

---

## What to Skip

| Area | Why |
|---|---|
| Cloud-provider integrations (CCM) | We're bare metal |
| Multi-cluster federation | Beyond mock-prod scope |
| Service mesh (Istio/Linkerd) | Optional Phase 14 if curious |
| Pod Security Standards detail | Note + return |
| Audit log streaming | Note + return |
| Custom scheduler plugins | Way beyond mock-prod |

---

## What to Pay Extra Attention To (Interview Gold)

| Topic | Why |
|---|---|
| Pod lifecycle (Pending → Running → Succeeded/Failed) | Asked every interview |
| Service types (ClusterIP/NodePort/LoadBalancer/ExternalName) | Asked every interview |
| Liveness vs Readiness vs Startup probes | Common confusion |
| Resource requests vs limits + OOM | The pager-duty question |
| `kubectl describe` anatomy | The debugging question |
| Rolling update strategy | Deployment config question |
| NetworkPolicy default-deny | Security question |
| etcd backup/restore | DR question |
| RBAC subject vs role vs binding | Auth question |
| StorageClass + PV vs PVC | Storage question |
| HPA / VPA | Scaling question |
| Affinity / anti-affinity / taints / tolerations | Scheduling question |

---

## Spike Coexistence Rules

| Component | Status | Rule |
|---|---|---|
| `/opt/spike-mimirs/heimdall-daemon` on EAST | Manual launches only | Stop before lab work, restart after |
| `/opt/spike-mimirs/gjallarhorn-listener` | Manual | Same |
| Ports 16666-26666 (Doom Range) | Reserved for Unheaded | K8s doesn't use these |
| Ports 6443/10250-10259/2379-2380 | Reserved for K8s | Spike doesn't use these |
| WireGuard `fd00:dead:beef::/48` | Shared | Both lab and spike use it |

---

## Choice: Implementation Path

| | KTHW | k3s | kubeadm |
|---|---|---|---|
| Vanilla K8s | ✓✓ | ✗ | ✓ |
| Single binary | ✗ | ✓ | ✗ |
| Fits 8GB EAST containers | tight | ✓ | tight |
| Interview-relevant | ✓✓ | ✓ | ✓✓ |
| Time to first cluster | 4h | 10m | 30m |
| Learning depth | very deep | shallow | medium |
| HA support | manual | needs separate setup | first-class |

**Recommendation order**:
1. **k3s** (10 min) — get a feel, run `kubectl get nodes`, see it work
2. **KTHW walkthrough** (Phases 1-2 above, ~6h) — actually understand each layer
3. **Optionally kubeadm** if you want to feel "real K8s install"

---

## After the Lab — Update

| File | Update |
|---|---|
| `docs/adr/ADR-047-...md` | Append "Lab Complete" with key findings |
| `docs/labs/k8s-vs-unheaded-notes.md` | Honest comparison output (Phase 12) |
| `wiki/K8s-Lab.md` | Public-facing summary |
| `~/.claude/projects/-home-govan-tmp/memory/` | Save findings as user/project memory |

---

## Quick Resume Prompt

If session resumes mid-lab:

> Resuming K8s Mock-Prod Lab (per `docs/labs/k8s-kingdom-lab.md`).
> Current phase: [N]. Last verified: [what works].
> Topology: 8 LXD containers (5 WEST + 3 EAST), HA control plane on cp-1/2/3.
> Spike daemons on EAST coexist (do not stop K8s containers, do not start spike).

---

*K8s Mock-Prod Lab — Forged 2026-04-11*
*"Use what you know to learn what you don't. Build it like prod."*
