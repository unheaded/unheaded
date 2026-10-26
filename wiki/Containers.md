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

The runtime is a deployment-time choice. Definitions for each live side by side: `lxd/` and `containers/lxd/`, `containers/containerd/`, `nix/` + `nixos/`, `docker/`, `containers/docker/` and `docker-compose.yml`.

The control plane (Cuirass, `cmd/unheaded-daemon`) drives one runtime today: LXD, through `LXD_BACKEND` (`mock`, `unix`, `https`; below). There is no containerd, NixOS or Docker backend in the daemon. West runs its services in `docker compose`; east runs them as systemd units ([[Systemd Units|Systemd-Units]]).

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

**Live on west since 2026-10-26** (S-REAL-STEEL Phase 5). `scripts/cuirass-lxd-trust.sh --apply` made the LXD side: project `unheaded` (`restricted=true`, its own profiles because LXD 5.21 refuses to restrict a project that shares them; its default profile seeded with the default project's root disk and `lxdbr0` NIC; `restricted.devices.proxy=allow` because Cuirass publishes service ports with a proxy device; images shared with `default`), a P-384 client certificate in `/etc/unheaded/cuirass` owned by uid 16740, the trust `cuirass` restricted to that project, and the pinned server certificate. The compose override `docker/compose.cuirass-lxd.yml` points Cuirass at `https://host.docker.internal:8443` with those files mounted read-only and `AUTH_ENABLED=true` (key in `/etc/unheaded/cuirass/cuirass.env`, 0640 root:16740, not in the repo).

Window recipe (E2 window model, stack stopped at the end of a window):

```bash
# observe (enforce/prune/chaos unset)
sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d
# enforce + prune (prune only sees project unheaded, where every instance is ours)
sudo CUIRASS_ENFORCE=true CUIRASS_PRUNE=true docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d cuirass
# chaos on, game-day windows only
sudo CUIRASS_ENFORCE=true CUIRASS_PRUNE=true CUIRASS_CHAOS=true docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d cuirass
# window close
sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml stop
```

Proofs recorded in the plan (Steps 144-156): actual state `{}` with pihole running in `default`; observe mode logged a planned `create` and created nothing; enforce created `steel-smoke` with `user.unheaded.managed=true`, a managed `DELETE` pruned it one sync later; pihole's last start (boot) predates the trust apply; `cuirass_lxd_up 1`. Back to the mock backend: bring the stack up without the override file (the desired state in the `cuirass-state` volume is kept). Undo the LXD side: `lxc config trust remove <fingerprint>`, `lxc project delete unheaded` (when empty), `rm -r /etc/unheaded/cuirass`.

### Health, proxy and chaos (S-REAL-STEEL Phase 4)

A desired container (`PUT /api/v1/containers/{id}`) may also carry:

| Field | Shape | What Cuirass does |
|---|---|---|
| `health` | `{"port": 8080, "path": "/health"}` | Each sync, `GET http://<IP LXD reports>:<port><path>`: 2 s timeout, redirects not followed, no proxy from the environment, body dropped after 4 KiB. One pass = `healthy`, three failures in a row = `unhealthy` (enforce mode then restarts it); without `health` it stays `unknown`. The path is a plain path (letters, digits, `._~/-`, no `..`, query or `//`); an instance reporting a loopback, link-local, unspecified or multicast address is not probed and counts as failing. Gauge `cuirass_container_health{id}` (1, 0, -1 unknown). |
| `proxy` | `{"listen": "tcp:127.0.0.1:26601", "connect": "tcp:127.0.0.1:8080"}` | Keeps one LXD proxy device, `unheaded-proxy`, on the instance: the service's stable endpoint (blue-green moves it). `listen` is `127.0.0.1` or an address in `CUIRASS_PROXY_LISTEN_ADDRS` (comma-separated IPs), port 26000-26666; `connect` is `127.0.0.1` inside the container. Two containers cannot claim one `listen` (409). Drift plans `update-proxy`: a read-modify-write with the instance ETag in `If-Match` (412 retried once) that sends every other device and field back unchanged. |
| `labels.chaos` | `"allowed"` | Opts the instance into the chaos endpoint below. |

Actual state (`/api/v1/state/actual`) reports each instance's `proxy` (ours only) and `labels`.

**Chaos stop/kill**: `POST /api/v1/containers/{id}/chaos` with `{"action":"stop"|"kill","reason":"..."}` (stop = clean, kill = forced). 403 unless `AUTH_ENABLED=true`, `CUIRASS_CHAOS=true` and `CUIRASS_ENFORCE=true` (without enforce nothing starts the instance again); 409 unless the instance is managed and labelled `chaos=allowed`. Those two are checked twice, independently: by the handler against the reconciler's view, and by the LXD client against a fresh read. The reconciler is not told, so its next sync (30 s) starts the instance again: that is the revert. Every request, refused or not, is logged once and published on `cuirass.chaos` (action, reason, caller, result).

A game-day window turns chaos on with `CUIRASS_CHAOS=true` in the Cuirass environment and off again by removing it (restart Cuirass both times); outside a window the endpoint always answers 403.

`/api/v1/info` reports `lxd_backend`, `lxd_project`, `enforce`, `prune` and `chaos`; `cuirass_reconcile_actions_total{action,result}` counts actions as `done`, `failed`, `planned` (observe mode) or `refused`.

## NixOS Container Definitions

For NixOS-based deployments, declarative container definitions live in `nix/containers/`. These are the reference implementations — other runtimes map to equivalent security postures.

---

> **Source:** [nix/](../nix/) · [nix/DEPLOYMENT.md](../nix/DEPLOYMENT.md)
