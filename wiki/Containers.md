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

The same label and prune rule apply to `pkg/state`'s reconciler.

## Cuirass LXD backend

`LXD_BACKEND` picks the client (`cmd/unheaded-daemon/lxd_rest.go`, standard library only):

| `LXD_BACKEND` | Connects with | Settings |
|---|---|---|
| `mock` (default) | in-memory, no LXD | — |
| `unix` | LXD's unix socket | `LXD_SOCKET` (default `/var/snap/lxd/common/lxd/unix.socket`) |
| `https` | LXD's HTTPS API, client certificate, pinned server certificate | `LXD_URL` (`https://host:8443`), `LXD_CLIENT_CERT`, `LXD_CLIENT_KEY`, `LXD_SERVER_CERT` |

`LXD_PROJECT` applies to both real backends; a project that does not exist is an error, never a fallback to `default`. The YAML config takes the same settings as `lxd_backend`, `lxd_socket`, `lxd_url`, `lxd_project`, `lxd_client_cert`, `lxd_client_key`, `lxd_server_cert`.

- **Use `https`.** The unix socket is root-equivalent and sees every project; the daemon logs a warning when it uses it. A certificate added with `lxc config trust add --restricted --projects unheaded` sees only that project, so pihole is not even visible to it.
- **The server certificate is pinned.** The connection fails unless LXD presents exactly the certificate in `LXD_SERVER_CERT` (`lxc info | sed -n '/BEGIN CERT/,/END CERT/p'`, or `/var/snap/lxd/common/lxd/server.crt`).
- **The client re-checks the label.** Start, stop, restart and delete each re-read the instance and refuse it without `user.unheaded.managed=true`, whatever the reconciler planned.
- **A failed list skips the cycle.** Actual state and drift stay as last seen; an unreachable LXD is not reported as every container missing. `cuirass_lxd_up` is 0 until a list succeeds.
- Created instances get `user.unheaded.managed=true`, `limits.cpu` (spec `cpu`, cores), `limits.memory` (spec `memory`, bytes) and `user.unheaded.label.<key>` per label. Images are a local alias or `images:<alias>` / `ubuntu:<alias>`. Instance names follow LXD's rule (letter first, letters, digits and hyphens, at most 63); `PUT /api/v1/containers/{id}` rejects others with 400.

**Desired state persists** in `desired.json` under `CUIRASS_STATE_DIR` (`state_dir`; default `/var/lib/unheaded/cuirass`, the compose `cuirass-state` volume; the deb's unit uses its StateDirectory `/var/lib/unheaded/daemon`). It was in memory only: a restart forgot every desired container, and with pruning the next reconcile would have deleted every managed one. Writes are atomic (temp file, fsync, rename, mode 0600); a write that cannot be saved returns 503 and changes nothing; an empty `state_dir` refuses all writes; a damaged file stops Cuirass at startup instead of starting empty.

Not deployed yet. `scripts/cuirass-lxd-trust.sh` (dry run unless `--apply`) does the LXD side: project `unheaded` (`restricted=true`, sharing the default project's images and profiles), a P-384 client certificate in `/etc/unheaded/cuirass` owned by Cuirass's uid, a trust restricted to that project, and the server certificate to pin; then it prints the daemon settings. West's LXD already listens on `[::]:8443` with no trusted certificates. Checked 2026-10-23 with an untrusted scratch certificate (opt-in `TestLXDREST_LiveHTTPS`): TLS 1.3 and the pin work, a wrong pin is refused, and LXD answers `untrusted`. After that: the daemon with `LXD_BACKEND=https` in observe mode, and read the planned actions before enforcing.

## NixOS Container Definitions

For NixOS-based deployments, declarative container definitions live in `nix/containers/`. These are the reference implementations — other runtimes map to equivalent security postures.

---

> **Source:** [nix/](../nix/) · [nix/DEPLOYMENT.md](../nix/DEPLOYMENT.md)
