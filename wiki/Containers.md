# Containers — Immutable Container Definitions

Unheaded supports interchangeable, drop-in container runtimes. Bring your preferred architecture — the hardening baseline applies uniformly across all of them.

## Supported Runtimes

| Runtime | Use Case | Status |
|---------|----------|--------|
| **LXD** | System containers, NixOS guests, full OS isolation | Primary (Alpha) |
| **containerd** | OCI-native, Kubernetes-ready, lightweight | Supported |
| **NixOS** | Declarative, immutable, reproducible definitions | Supported |
| **Docker** | Development, CI/CD, Docker Compose stacks | Supported |

## Hardening Baseline (All Runtimes)

Every container, regardless of runtime, MUST enforce:

- Minimum capabilities (CAP_NET_BIND_SERVICE only)
- NoNewPrivileges
- Seccomp syscall filtering
- Read-only filesystem (except designated paths)
- Private /tmp, protected /home
- Default-deny network policy
- eBPF traceability from packet zero

## Runtime Selection

The container runtime is a deployment-time choice, not an architectural decision. Unheaded's control plane (unheaded-daemon) abstracts the runtime behind a common interface:

```
unheaded-daemon
  ├── runtime: lxd      → LXD REST API
  ├── runtime: containerd → CRI / containerd API
  ├── runtime: nixos     → NixOS container definitions
  └── runtime: docker    → Docker Engine API
```

Services don't know or care which runtime hosts them. The hardening, networking, and observability layers are identical.

## Reconciliation safe mode

Cuirass (unheaded-daemon) shares hosts with things it did not create: west's LXD runs `pihole`. As first written, the first reconcile against a real LXD would have force-deleted it as "orphaned". Since 2026-10-23:

- **Managed label.** Cuirass sets `user.unheaded.managed=true` on every instance it creates and never starts, restarts or deletes one without it. An unmanaged instance is not drift; one whose name collides with a desired container is refused and reported as `unmanaged` drift.
- **Observe by default.** Reconcile plans actions and logs them (`Observe mode: would act`, `planned: true` in the `cuirass.drift` events) and takes none, until `CUIRASS_ENFORCE=true` (or `enforce: true` in the config file).
- **No pruning by default.** A managed instance nobody desires is reported as orphaned and deleted only with `CUIRASS_PRUNE=true` (`prune: true`).
- **Writes need auth.** `PUT`/`DELETE /api/v1/containers/{id}` answer 403 unless `AUTH_ENABLED=true`; reads stay open.

The same label and prune rule apply to `pkg/state`'s reconciler. The daemon still runs the mock LXD client: no real LXD connection exists yet. LXD socket access is root-equivalent. A real client should use LXD's HTTPS API with a restricted client certificate, limited to an `unheaded` project, so pihole is not even visible to it.

## NixOS Container Definitions

For NixOS-based deployments, declarative container definitions live in `nix/containers/`. These are the reference implementations — other runtimes map to equivalent security postures.

---

> **Source:** [nix/](../nix/) · [nix/DEPLOYMENT.md](../nix/DEPLOYMENT.md)
