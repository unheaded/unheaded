# ADR-088 — Kubernetes as Kingdom Service Orchestration Layer

**Status:** Planned
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

The Kingdom currently runs services two ways: Docker Compose on WEST (the full
stack) and native systemd on EAST (bare-metal agents). As the lab grows — more
hosts, more services, Muninn HA clusters, Kvasir, Huginn on every node — this
becomes hard to manage by hand. Each new host needs manual unit file installs,
config copies, and service wiring.

Kubernetes is the industry standard for this class of problem. A CKA-certified
operator can manage a fleet this size in their sleep. It also directly aligns
with the Yggdrasil golden image work (ADR-081) and the plumbing-first strategy
(multiple deployment targets from one protocol).

The Kingdom's own IaC layer already has a Kubernetes output backend (manifests,
Helm charts, operators) — the platform can generate K8s artifacts for itself.
That is the meta moment for this layer.

---

## Decision

Adopt Kubernetes as the long-term orchestration substrate for multi-host Kingdom
services. The current Compose + systemd setup stays in place for the dev lab;
K8s is the target for when the lab grows past what one person can manage by hand.

---

## Rationale

**Why Kubernetes and not just more Compose/systemd:**

- Compose is per-host. Adding a third host means repeating the same copy-paste
  install dance. K8s makes the fleet a single logical unit.
- Systemd is great for always-on primitives (huginn, victoria) but has no
  concept of scheduling, replica sets, or rolling updates across nodes.
- Helm + GitOps (ArgoCD / Flux) give the same declarative, version-controlled
  config management we already apply to network devices (ADR-087) and
  infrastructure code — consistent discipline across all layers.
- Every major cloud provider's managed offering is K8s. Learning it in the
  home lab transfers directly to production work.

**Why not rush it:**

- The lab is currently two nodes. K8s control plane overhead isn't justified yet.
- There are more urgent blockers (ASCEND-LINUX EVOLUTION-1, Muninn, CI/CD).
- CKA is a legitimate certification goal and studying on the live platform is
  the right way to do it — deploy real workloads, break things, fix them.

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

## Migration Path

**Phase 1 — Lab learning (no production impact)**
- Stand up a single-node k3s cluster on a spare machine or VM.
- Port huginn + victoria manifests. Validate DaemonSet behaviour.
- Port Muninn to a Deployment with 2 replicas behind a ClusterIP.
- Study for CKA using this cluster as the lab environment.

**Phase 2 — Two-node cluster (WEST + EAST)**
- Promote WEST to control plane + worker.
- Join EAST as a worker node.
- Migrate huginn from systemd → DaemonSet (systemd unit stays as fallback
  during the cut-over; remove after validation).
- Migrate victoria from Compose → StatefulSet with persistent volume.
- `unheaded-victoria.service` and `huginn.service` systemd units are
  deprecated but kept in `deploy/systemd/` for reference.

**Phase 3 — Full fleet**
- All Kingdom services running in K8s.
- ArgoCD or Flux for GitOps (auto-sync from `k8s/` in main branch).
- Compose and systemd units archived in `deploy/legacy/`.

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

- New hosts join the fleet by running `kubeadm join` — huginn DaemonSet lands
  automatically, zero manual config.
- Muninn HA is free — K8s Deployment replicas replace the haproxy/nginx proxy
  planned in ADR-086. ClusterIP is the stable endpoint.
- The Kingdom IaC layer (ADR-002) generates K8s manifests; the platform manages
  itself via its own output. Meta moment for this layer.
- CKA study happens on real workloads, not synthetic exam prep. Knowledge sticks.

---

## Related

- ADR-002 — IaC backend strategy (K8s is one of the interchangeable backends)
- ADR-084 — Huginn (DaemonSet target)
- ADR-085 — CI/CD artifact layout (K8s manifests alongside .deb packages)
- ADR-086 — Muninn HA (Deployment + ClusterIP replaces haproxy/nginx plan)
- ADR-087 — NOC (Kvasir as a Deployment; network device manifests in k8s/)
- `docs/adr/ADR-69420` — Yggdrasil golden image (K8s worker node OS baseline)
