# ADR-007: Container Hardening Strategy (NixOS + Seccomp + Capabilities)

## Status: Accepted

## Date: 2026-01-26

## Context

Unheaded manages customer application infrastructure. The platform's core security promise is **zero customer data access** -- an architectural guarantee, not just a policy. This means the containers hosting Unheaded's own services must be hardened to a level where even a compromised service cannot escape its sandbox to access customer data or other services' internal state.

The threat model includes:

1. **Compromised service**: An attacker exploits a vulnerability in a Go/Rust service and gains code execution inside a container.
2. **Supply chain attack**: A dependency (even in dev builds) contains malicious code that activates at runtime.
3. **Privilege escalation**: An attacker leverages kernel vulnerabilities or misconfigured capabilities to escape the container.
4. **Lateral movement**: An attacker moves from one compromised container to others on the same network.
5. **Data exfiltration**: An attacker attempts to read secrets, customer data, or platform state from a compromised container.

The hardening strategy must address all five threats while keeping containers functional and allowing legitimate operations (network communication via Busboy, file I/O for timeline/strategy documents, metrics exposition).

### Alternatives Considered

**Docker with default seccomp profile**: Docker's default seccomp profile blocks ~44 syscalls. However, it allows many dangerous operations (ptrace, mount, module loading) and the default capability set includes CAP_NET_RAW, CAP_SYS_CHROOT, and others unnecessary for application services.

**Kubernetes with Pod Security Standards**: K8s PSS provides "restricted" profiles, but requires a Kubernetes cluster. Unheaded uses LXD for lightweight system containers, not Kubernetes pods.

**gVisor/Kata Containers**: Provide strong isolation via a user-space kernel or lightweight VM. However, gVisor's syscall compatibility is incomplete (problematic for eBPF), and Kata's VM overhead conflicts with the sub-10-second container startup requirement.

**NixOS containers with layered hardening**: NixOS provides declarative, reproducible container definitions. Hardening is expressed as Nix configuration, version-controlled alongside application code. Seccomp filters, capability restrictions, filesystem protections, and kernel hardening parameters are all declared in `.nix` files.

## Decision

We adopt a **defense-in-depth container hardening strategy** using NixOS containers with four reinforcing security layers:

### Layer 1: NixOS Declarative Immutability

Every container is defined in `nix/containers/<service>.nix` as a declarative NixOS configuration. The key properties:

- **Immutable root filesystem**: The NixOS store (`/nix/store/`) is read-only by design. No runtime modification of system binaries or libraries is possible.
- **Reproducible builds**: Given the same `flake.lock`, the same container image is produced byte-for-byte. Drift between environments is eliminated.
- **Atomic upgrades**: Container updates are atomic Nix profile switches, not in-place mutations. Rollback is instant.

### Layer 2: Linux Capability Restrictions

Every service's systemd unit drops all capabilities except the minimum required:

```nix
serviceConfig = {
  CapabilityBoundingSet = [ "CAP_NET_BIND_SERVICE" ];
  AmbientCapabilities = [ "CAP_NET_BIND_SERVICE" ];
  NoNewPrivileges = true;
};
```

- `CAP_NET_BIND_SERVICE` is the only capability granted (for binding to ports < 1024 if needed).
- `NoNewPrivileges = true` prevents any child process from gaining capabilities not held by the parent, blocking SUID/SGID escalation.

### Layer 3: Seccomp System Call Filtering

Every service applies a strict seccomp filter that blocks dangerous syscall categories:

```nix
serviceConfig = {
  SystemCallFilter = [
    "@system-service"
    "~@privileged"
    "~@resources"
    "~@obsolete"
    "~@debug"
    "~@mount"
    "~@reboot"
    "~@swap"
    "~@module"
    "~@raw-io"
  ];
};
```

This allowlists only the syscalls needed for a standard network service (socket, read, write, open, close, etc.) while explicitly blocking: module loading, mount operations, reboot, swap management, raw I/O, debugging (ptrace), and privileged operations.

### Layer 4: Filesystem and Process Isolation

```nix
serviceConfig = {
  PrivateTmp = true;              # Isolated /tmp
  PrivateDevices = true;          # No access to physical devices
  ProtectSystem = "strict";       # Read-only /usr, /boot, /etc
  ProtectHome = true;             # No access to /home
  ProtectKernelTunables = true;   # No /proc/sys writes
  ProtectKernelModules = true;    # No module loading
  ProtectKernelLogs = true;       # No kernel log access
  ProtectControlGroups = true;    # No cgroup modification
  MemoryDenyWriteExecute = true;  # No W+X memory pages (prevents shellcode)
  RestrictNamespaces = true;      # No new namespace creation
  RestrictSUIDSGID = true;        # No SUID/SGID binaries
  RestrictRealtime = true;        # No realtime scheduling
  ReadOnlyPaths = [ "/etc" "/usr" ];
  ReadWritePaths = [ "/opt/unheaded/references" ];  # Only writable path
};
```

### Layer 5: Network Hardening

```nix
networking.firewall = {
  enable = true;
  allowedTCPPorts = [ <service-port> ];  # Only the service's own port
};
```

Default-deny firewall with explicit allow rules. Only the container network (10.10.10.0/24) and the service's own port are permitted. IP forwarding is disabled, FORWARD chain policy is DROP, and kernel hardening parameters (`dmesg_restrict=1`, `kptr_restrict=2`, `ptrace_scope=2`) are applied via `nix/modules/hardening.nix`.

### Docker Compose Hardening

For development and Docker-based deployments, equivalent hardening is applied:

```yaml
security_opt: [ "no-new-privileges:true" ]
cap_drop: [ "MKNOD", "AUDIT_WRITE" ]
privileged: false
read_only: true  # where possible
```

## Consequences

### Positive

- **Defense in depth**: Five independent security layers mean an attacker must defeat all five to achieve meaningful access. A seccomp bypass is useless if capabilities are dropped; a capability escalation is useless if the filesystem is read-only.
- **Declarative and auditable**: All hardening configuration is in version-controlled `.nix` files. Security auditors can review the complete security posture by reading `nix/modules/hardening.nix` and `nix/containers/base.nix` -- no runtime inspection needed.
- **Reproducible**: The same Nix flake produces the same hardened container on any machine. There is no configuration drift between development, staging, and production.
- **Verified by audit**: The February 6, 2026 security audit (`docs/SECURITY_AUDIT.md`, findings F-006 through F-009) confirms that all containers receive uniform hardening, firewalls enforce default-deny, and the isolation boundary holds.
- **Minimal attack surface**: `MemoryDenyWriteExecute` prevents shellcode injection. `ProtectKernelTunables` prevents sysctl manipulation. `RestrictNamespaces` prevents container breakout via namespace tricks. The combination blocks the most common container escape techniques.
- **Self-documenting**: The NixOS configuration *is* the security specification. There is no gap between "what security policy says" and "what is deployed."

### Negative

- **NixOS learning curve**: NixOS has a steep learning curve. Writing and debugging Nix expressions requires familiarity with functional programming concepts and the Nix language. The team must maintain NixOS expertise.
- **Development friction**: Strict seccomp filters can block legitimate syscalls during development. Debugging seccomp violations requires `strace` and careful filter adjustment. The development environment may need relaxed filters that differ from production.
- **eBPF exception**: The trace-collector container requires `CAP_BPF`, `CAP_NET_ADMIN`, and `CAP_SYS_ADMIN` for eBPF program loading. This is a necessary exception to the minimal-capabilities rule, documented in `nix/containers/trace-collector.nix`.
- **Startup overhead**: NixOS containers with full hardening take slightly longer to start than minimal Alpine containers. The 10-second startup target is achievable but requires optimization of the Nix evaluation and systemd boot sequence.
- **Complexity**: Five security layers means five places where a misconfiguration can break a service. Debugging "service won't start" requires checking capabilities, seccomp, filesystem permissions, firewall rules, and NixOS service configuration.

## References

- `nix/modules/hardening.nix` -- Shared hardening module applied to all containers
- `nix/containers/base.nix` -- Base container configuration with all security layers
- `nix/containers/` -- Per-service NixOS container definitions
- `docs/SECURITY_AUDIT.md` -- February 6, 2026 audit verifying hardening (F-006 through F-009)
- `docs/ARCHITECTURE.md` -- Security Architecture section
- `docker-compose.yml` -- Docker Compose hardening for development deployments
- `CLAUDE.md` -- Security Requirements and Container Hardening sections
