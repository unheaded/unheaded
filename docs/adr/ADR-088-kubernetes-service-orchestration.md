# ADR-088 — Kingdom Deployment Substrate Ladder

**Status:** Planned
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

The Kingdom runs 15+ services across two bare-metal hosts: Wotan, Timeguru,
Captain, Architect, Micromanager, Monad, Sophia, Dashboard, Kanban,
unheaded-daemon, Akira, Huginn, VictoriaMetrics, Grafana, Zhenai, the DOOM
pipeline, and the UPC stack. This is production-equivalent scale for a small
startup — real traffic, real interdependencies, real failure modes.

Unheaded's core moat is the custom protocol stack (Wotan, Monad, Sophia, the
eBPF/UPC layer). That is not in scope here. This ADR is about **how services
are deployed and managed** on the hosts that run them.

The deployment surface is intentionally a learning ladder. K8s, Terraform,
Ansible, NixOS, and traditional package management (.deb/.rpm, apt repo) are
all industry-standard skills that are explicitly tested in SRE, systems
engineer, infra engineer, and network engineer hiring. Running all of these
against 15+ real services — breaking them, fixing them, writing runbooks — is
a more effective preparation environment than any certification lab or online
course, because the complexity and failure modes are real.

Current deployment methods:
- **Docker Compose** (WEST) — full stack, fast iteration
- **systemd units** (EAST) — bare-metal agents (huginn, etc.)
- **NixOS** — container definitions exist in `nix/`; not yet exercised live

---

## Decision

Treat the Kingdom's deployment surface as a **ladder of substrates**, each
additive and optional. Nothing is replaced; substrates are added when there
is a learning goal or operational need that justifies them. The custom protocol
stack is the constant; the deployment layer is the variable.

```
Substrate ladder (low → high complexity):
  .deb / .rpm packages   — traditional Linux packaging; apt/yum installs
  apt/rpm repository     — self-hosted; ADR-085 CI/CD artifact store
  systemd units          — bare-metal always-on (huginn, victoria) ← current
  Docker Compose         — full-stack local + WEST ← current
  NixOS                  — reproducible, declarative; nix/ already scaffolded
  Ansible                — push-based config management across hosts
  Terraform              — infrastructure provisioning (cloud or bare-metal)
  Kubernetes             — container scheduling, DaemonSets, HA Deployments
    └── GitOps (ArgoCD / Flux) — K8s declarative sync from repo
```

No substrate is "the answer." Each has a place:
- huginn: systemd (always-on, near-zero overhead) AND K8s DaemonSet (fleet scaling)
- victoria: Compose (single-host) AND K8s StatefulSet (when HA matters)
- new hosts: apt install unheaded-huginn OR kubeadm join — both valid
- OS baseline: Yggdrasil (ADR-69420) built from NixOS or Debian, both supported

---

## Substrate Details

### .deb / .rpm + Self-Hosted Repository (ADR-085)

The CI/CD artifact pipeline (ADR-085) produces `.deb` packages for every
Kingdom binary. A self-hosted apt repo on WEST means `apt install unheaded-huginn`
works on any Debian/Ubuntu host — no manual binary copy, no build tools required.

**What this exercises:** package lifecycle (pre/post-install scripts, conffiles,
version pinning), apt repo management (reprepro or aptly), dependency declaration,
upgrade paths. These are bread-and-butter SRE skills tested in every infra interview.

**Mapping to `deploy/systemd/`:** the `.deb` packages install the systemd units
from `deploy/systemd/` automatically. `apt install unheaded-huginn` = binary +
unit file + default config in one step.

### NixOS

NixOS container definitions live in `nix/` and cover all core services. The
value of NixOS at the Kingdom scale: reproductible builds, atomic rollback,
and a declarative OS config that can be version-controlled exactly like the
application code.

**What this exercises:** Nix expression language, NixOS module system, flake
pinning, cross-compilation (relevant for Yggdrasil ARM targets), declarative
OS config vs. imperative config management. Increasingly asked about in senior
SRE roles at companies using NixOS (e.g. Replit, Shopify infra teams).

**Current state:** scaffolded, not live. Activating means booting a host from
a NixOS flake and validating the Kingdom services start correctly from the
NixOS module definitions.

### Ansible

Ansible is the config management tool most frequently asked about in SRE/infra
interviews and is the standard tool for Junos network device management (ADR-087).
The Kingdom has enough hosts and enough config surface to write real roles, not
toy playbooks.

**Kingdom Ansible roles (planned):**
```
ansible/
  inventory/
    hosts.yaml            # WEST, EAST, future nodes
  group_vars/
    kingdom.yaml          # shared vars (huginn port, victoria URL, etc.)
  host_vars/
    west.yaml
    east.yaml
  roles/
    kingdom-base/         # packages, users, sysctl, firewall baseline
    huginn/               # install binary + unit + config
    victoria/             # docker + compose for victoria only
    network-device/       # NTP, DNS, syslog; Junos NETCONF (ADR-087)
  playbooks/
    site.yaml             # full desired state
    check.yaml            # --check --diff only (CI gate)
```

**What this exercises:** idempotent role design, Ansible facts, inventory
management, vault for secrets, handlers, templates (Jinja2), CI gate with
`ansible-lint`. These are pass/fail questions on SRE/infra job applications.

### Terraform

Terraform is the standard for provisioning cloud infrastructure and is tested
in every cloud-focused SRE and platform engineering role. The Kingdom's bare
metal is too small to justify Terraform for physical hosts, but it's the right
tool for:

- Provisioning cloud VMs (if/when the Kingdom gets a cloud node for WireGuard
  exit or Yggdrasil PXE relay)
- Declaring DNS records, firewall rules, or object storage as code
- Exercising the plan/apply/destroy lifecycle on real resources

**Planned:** a `terraform/` directory with modules for cloud provider resources
that complement the bare-metal core. Not scheduled — deferred until there's a
cloud resource to manage.

### Kubernetes

See Architecture Target and Rollout Path sections below.

---

## Kubernetes Architecture Target

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

**Key choices:**
- **huginn as DaemonSet** — lands automatically on every node. Zero manual install.
- **Muninn HA as Deployment (replicas: n+1)** — ClusterIP replaces the haproxy/nginx
  proxy plan from ADR-086.
- **VictoriaMetrics as StatefulSet** — persistent volume, stable pod identity.
- **NetworkPolicy** — deny-by-default, same discipline as the systemd units.
- **RBAC** — one ServiceAccount per service, minimum permissions.

```
k8s/
  namespaces/kingdom.yaml
  rbac/
  daemonsets/huginn.yaml
  statefulsets/victoria.yaml
  deployments/{muninn,dashboard,kanban,unheaded-daemon}.yaml
  services/{victoria-svc,muninn-svc}.yaml
  configmaps/huginn-config.yaml
  secrets/.gitkeep           # Sealed Secrets or SOPS — never plaintext
  helm/kingdom/              # umbrella Helm chart
```

CI gate: `kubectl apply --dry-run=server` on every PR touching `k8s/`.

---

## Rollout Path

**Phase 1 — k3s single-node (CKA study, no impact on WEST/EAST)**
- Single-node k3s on a spare machine or VM.
- Write and validate huginn DaemonSet, victoria StatefulSet, Muninn Deployment.
- Compose + systemd on WEST/EAST are completely unaffected.

**Phase 2 — Parallel operation on WEST + EAST**
- EAST joins as K8s worker. Run huginn both ways simultaneously; validate parity.
- Neither path removed until K8s has proven equal uptime.

**Phase 3 — K8s as preferred path for new services**
- New services get K8s manifests first. Existing services move only when there's
  a reason (HA, new-node rollout, scaling). No forced migration.
- ArgoCD or Flux for GitOps sync.
- `deploy/systemd/` stays permanently as the bare-metal fallback.

---

## Certification Alignment

| Cert / Skill | Kingdom workload that exercises it |
|---|---|
| **CKA** — cluster architecture | WEST control plane + EAST worker |
| **CKA** — DaemonSets | huginn on every node automatically |
| **CKA** — StatefulSets / PV | VictoriaMetrics with persistent storage |
| **CKA** — Services / NetworkPolicy | Muninn ClusterIP, deny-by-default policies |
| **CKA** — RBAC | Per-service ServiceAccounts |
| **CKA** — Troubleshooting | Breaking and recovering real services |
| **RHCSA / LPIC** — package mgmt | apt repo, .deb lifecycle, systemd units |
| **JNCIA / net eng** — Ansible | Junos NETCONF roles, network GitOps (ADR-087) |
| **Terraform associate** | Cloud node provisioning, DNS-as-code |
| **NixOS / platform eng** | Yggdrasil golden image, declarative OS config |

The K8s SecurityContext fields map 1:1 to the systemd hardening already in
place: `allowPrivilegeEscalation: false` = `NoNewPrivileges`,
`readOnlyRootFilesystem` = `ProtectSystem=strict`,
`capabilities.drop: [ALL]` = `CapabilityBoundingSet=`. No new discipline
required — apply what we already know.

---

## Consequences

- The custom Unheaded protocol stack (Wotan, Monad, eBPF, UPC) is unchanged.
- Every substrate is additive. Compose + systemd stay operational indefinitely.
- The Kingdom at 15+ services is a legitimate production-equivalent lab. Skills
  built here transfer directly to senior SRE / infra / network engineer roles —
  not as "I did the course" but as "I ran this in production and broke it."
- The Unheaded IaC layer generates artifacts for all of these substrates. The
  platform manages itself via its own output — that is the meta moment.

---

## Related

- ADR-002 — IaC backend strategy (Ansible, Terraform, K8s as interchangeable outputs)
- ADR-084 — Huginn (DaemonSet + systemd dual deployment)
- ADR-085 — CI/CD artifact layout (.deb packages, apt repo)
- ADR-086 — Muninn HA (Deployment + ClusterIP when K8s path is ready)
- ADR-087 — NOC (Ansible for Junos, Kvasir as K8s Deployment)
- ADR-69420 — Yggdrasil golden image (NixOS or Debian baseline)
