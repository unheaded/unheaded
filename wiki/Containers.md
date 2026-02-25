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

## NixOS Container Definitions

For NixOS-based deployments, declarative container definitions live in `nix/containers/`. These are the reference implementations — other runtimes map to equivalent security postures.

---

> **Source:** [nix/](../nix/) · [nix/DEPLOYMENT.md](../nix/DEPLOYMENT.md)
