# ADR-088 — Kubernetes as Kingdom Service Orchestration Layer

**Status:** Planned
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

The Kingdom currently runs 15+ services across two bare-metal hosts (WEST +
EAST): Wotan, Timeguru, Captain, Architect, Micromanager, Monad, Sophia,
Dashboard, Kanban, unheaded-daemon, Akira, Huginn, VictoriaMetrics, Grafana,
Zhenai, plus the DOOM pipeline and the UPC stack. This is closer to a real
small-startup production environment than a lab — the scale and service
interdependencies are genuine.

Unheaded's core technical moat is the custom protocol stack (Wotan, Monad,
Sophia, the eBPF/UPC execution layer). That does not change. Kubernetes is
one of the IaC output targets Unheaded already supports (ADR-002) — it is
an additional deployment substrate, not a replacement for anything.

The primary motivation for K8s integration here is **learning by doing on a
real system**. Toy labs and official exam demos have 2-3 services. The Kingdom
has 15+, real observability, real inter-service traffic, real failure modes.
That is a meaningfully different learning environment and a better CKA
preparation surface.

---

## Decision

Add Kubernetes as an **additional, optional** deployment target for Kingdom
services, running alongside the existing Compose + systemd setup. Neither is
replaced. The custom Unheaded stack (Wotan bus, Monad wire format, eBPF
pipeline, UPC) remains the core platform. K8s is one substrate it can deploy
to — the same relationship it has with Ansible, Terraform, and the other IaC
backends.

The Kingdom's production-scale service count makes it a legitimate K8s learning
environment. Manifests live in `k8s/` in the repo and are the primary study
material for CKA certification.

---

## Rationale

**Why K8s alongside, not instead of, the custom solution:**

- The Unheaded protocol (Wotan, Monad, eBPF) IS the moat. K8s doesn't touch
  that. It's a scheduling and deployment layer on top.
- Unheaded already outputs K8s manifests as one of its IaC backends. Exercising
  that backend on the Kingdom itself is the meta moment for this layer.
- Compose + systemd stay. They're simpler for fast iteration and DOOM/UPC work
  that doesn't benefit from container scheduling semantics.

**Why the Kingdom is the right lab for CKA prep:**

- 15+ services, real traffic, real inter-service dependencies, real observability
  data. This is what a 10-person startup's cluster looks like — not a toy.
- Huginn as a DaemonSet exercises the exact CKA topic of "deploy to every node
  automatically." The exam asks you to understand it; the Kingdom makes you live it.
- Breaking a real service (wotan goes down, huginn loses its push target) and
  recovering it under K8s is a better diagnostic exercise than staged exam scenarios.
- CKA prep on your own infrastructure also builds the muscle memory for production
  on-call work — not just exam pass/fail.

---

## Architecture Target

```
Kingdom K8s Cluster (future)
├── Control plane (WEST or dedicated node)
├── Worker: west
│     huginn DaemonSet, victoria StatefulSet
│     wotan Deployment, dashboard Deployment
│     kanban Deployment, unheaded-daemon Deployment
├── Worker: east
│     huginn DaemonSet
│     (stateless services scheduled as needed)
└── Worker: future nodes
      huginn DaemonSet (automatic, zero config)
```

**Key design choices:**

- **huginn as DaemonSet** — one pod per node, automatically. No manual unit
  installs on new hosts. This is the biggest operational win.
- **Muninn HA as Deployment (replicas: n+1)** — K8s handles pod restarts,
  rescheduling, and the fixed ClusterIP that huginn pushes to. Replaces the
  haproxy/nginx proxy planned in ADR-086.
- **VictoriaMetrics as StatefulSet** — persistent volume, stable pod identity.
- **Wotan as Deployment** — stateless enough for replicas once Wotan supports it.
- **Network policies** — enforce the same deny-by-default isolation we apply
  everywhere. Pods can only talk to explicitly allowed peers.
- **RBAC** — one ServiceAccount per service, minimum permissions. Mirrors the
  CapabilityBoundingSet discipline in systemd units.

---

## GitOps Layout

```
k8s/
  namespaces/
    kingdom.yaml          # namespace: kingdom
  rbac/
    service-accounts.yaml
    roles.yaml
  daemonsets/
    huginn.yaml
  statefulsets/
    victoria.yaml
    wotan.yaml            # when Wotan gains HA support
  deployments/
    muninn.yaml           # replicas: 2 (n+1)
    dashboard.yaml
    kanban.yaml
    unheaded-daemon.yaml
  services/
    victoria-svc.yaml     # ClusterIP — huginn push target
    muninn-svc.yaml       # ClusterIP — stable endpoint for huginn
  configmaps/
    huginn-config.yaml    # huginn.yaml baked in
  secrets/
    .gitkeep              # secrets via Sealed Secrets or SOPS, never plaintext
  helm/
    kingdom/              # umbrella chart wrapping the above
```

CI gate: `kubectl apply --dry-run=server` on every PR touching `k8s/`.

---

## Rollout Path

**Phase 1 — k3s single-node (CKA study starts here)**
- Stand up a single-node k3s cluster (spare machine or VM on WEST).
- Write and validate huginn DaemonSet, victoria StatefulSet, Muninn Deployment.
- Verify DaemonSet auto-scheduling and ClusterIP stability.
- This is the primary CKA study environment; break things freely.
- Compose + systemd on WEST/EAST are completely unaffected.

**Phase 2 — Two-node cluster alongside existing setup**
- Add EAST as a K8s worker node (WEST stays control plane + worker).
- Run huginn both ways simultaneously: systemd unit (always-on, battle-tested)
  AND DaemonSet (K8s path). Compare behaviour; validate the K8s path matches.
- victoria runs in both Compose (primary) and K8s (parallel experiment).
- Neither is removed until K8s path has proven equal uptime.

**Phase 3 — K8s as preferred path for new services**
- New services get K8s manifests first, Compose second.
- Existing services stay on Compose/systemd until they need a reason to move
  (scaling, HA, new-node rollout). No forced migration.
- ArgoCD or Flux for GitOps once the manifests are stable.
- `deploy/systemd/` stays in the repo permanently — it's the bare-metal
  fallback for hosts that aren't in the cluster.

---

## CKA Alignment

The CKA exam covers exactly what this lab will exercise:

| CKA Domain | Kingdom workload that exercises it |
|------------|-----------------------------------|
| Cluster architecture | WEST control plane + EAST worker setup |
| Workloads & scheduling | huginn DaemonSet, Muninn Deployment |
| Services & networking | ClusterIP for victoria/Muninn, NetworkPolicy |
| Storage | VictoriaMetrics PersistentVolume |
| Troubleshooting | Breaking and fixing real services |
| Security | RBAC, NetworkPolicy, SecurityContext (mirrors systemd hardening) |

The K8s SecurityContext fields map 1:1 to what we already write in systemd:
`allowPrivilegeEscalation: false` = `NoNewPrivileges`, `readOnlyRootFilesystem`
= `ProtectSystem=strict`, `capabilities.drop: [ALL]` = `CapabilityBoundingSet=`.
Existing systemd hardening discipline transfers directly.

---

## Consequences

- The custom Unheaded protocol stack is unchanged. K8s is a deployment surface,
  not a replacement for Wotan, Monad, or the eBPF layer.
- Compose + systemd stay in the repo and stay operational. They're the faster
  iteration path for UPC/DOOM/kernel work where container scheduling is overhead.
- New hosts can join via `kubeadm join` — huginn DaemonSet lands automatically.
  Or they get the systemd unit installed manually. Both are valid.
- Muninn HA simplifies: ClusterIP Service replaces the haproxy/nginx proxy plan
  from ADR-086 when the K8s path is ready.
- The Kingdom IaC layer generates K8s manifests; the platform can deploy itself
  to a K8s substrate it also manages. Meta moment for this layer.
- CKA study happens on 15+ real services with real interdependencies, not a
  staged 2-service exam demo. The knowledge transfers to production on-call work.

---

## Related

- ADR-002 — IaC backend strategy (K8s is one of the interchangeable backends)
- ADR-084 — Huginn (DaemonSet target)
- ADR-085 — CI/CD artifact layout (K8s manifests alongside .deb packages)
- ADR-086 — Muninn HA (Deployment + ClusterIP replaces haproxy/nginx plan)
- ADR-087 — NOC (Kvasir as a Deployment; network device manifests in k8s/)
- `docs/adr/ADR-69420` — Yggdrasil golden image (K8s worker node OS baseline)
