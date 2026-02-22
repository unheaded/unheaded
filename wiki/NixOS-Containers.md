# NixOS Containers

Immutable, declarative container definitions for every Unheaded service. Each container is a hardened NixOS system with:

- Minimum capabilities (CAP_NET_BIND_SERVICE only)
- NoNewPrivileges
- Seccomp syscall filtering
- Read-only filesystem (except designated paths)
- Private /tmp, protected /home

---

> **Source:** [nix/](../nix/)
