# ADR-085 — CI/CD Pipeline Polish: Canonical Artifact Layout

**Status:** Planned
**Date:** 2026-07-24
**Author:** Stevie Bellis
**Deciders:** Stevie Bellis

---

## Context

Kingdom services currently land wherever they are built. Go binaries accumulate
in `~/tmp/unheaded/bin/` (ad-hoc `go build -o bin/` invocations). Systemd unit
files reference `/usr/local/bin/<name>` but nothing guarantees those binaries
actually exist there. Docker images are managed by the daemon but there is no
registry or versioning discipline. Config files appear in `/etc/unheaded/` for
some services and inline in unit files for others.

The session pattern that surfaced this: `pkill -f <process>` was used to stop
services, which is a symptom of no ownership model. Proper systemd units (ADR-084)
fix the stop/start story, but a canonical layout for artifacts is the missing
prerequisite for reliable installation, upgrade, and rollback.

The `.deb` packaging path exists (`scripts/build-all-debs.sh`, `scripts/build-sealed-cask.sh`,
APT repo server runbook at `runbooks/infra/apt-repo-server.yaml`) but is not
wired into a full CI/CD pipeline. This ADR defines the target layout so
implementation can proceed incrementally without re-architecting later.

---

## Decision

Adopt the following canonical filesystem layout for all Kingdom artifacts:

### Binaries

```
/usr/local/bin/<service>        # installed production binaries (apt-managed)
```

This is already the convention in `deploy/systemd/` unit files. It is the
standard FHS location for locally-installed executables and integrates cleanly
with `.deb` packaging (`/usr/local/bin` is the correct DESTDIR target for
non-distro packages on Debian/Ubuntu).

No `/opt/` split. No `~/tmp/` binaries in production. Source-tree `bin/` is
build output only — never referenced by unit files or runbooks.

### Runtime data and state

```
/var/lib/unheaded/<service>/    # persistent runtime data (databases, state files)
```

Already referenced in all ADR-084 unit files. The `unheaded` system user owns
these directories. Created by the `.deb` postinst script.

### Configuration

```
/etc/unheaded/<service>.env     # per-service environment (secrets, overrides)
/etc/unheaded/<service>.yaml    # structured config where needed
```

Already used by `zhen-agentd.service` and all ADR-084 units via `EnvironmentFile`.
The `.env` files stay out of git; `.yaml` skeletons ship with the package.

### Build cache and intermediate artifacts

```
/var/cache/unheaded/            # build cache (Go module cache, Rust registry, pip)
/var/cache/unheaded/debs/       # locally-built .deb packages pending publication
```

This moves the build cache off the developer's home directory and onto a path
that survives user account changes and can be shared across CI agents.

### Container images

```
/var/lib/docker/                # Docker daemon storage (unchanged — daemon-managed)
```

The West host runs services via Docker Compose. Images are pulled from the
registry or built locally. No change to Docker's storage path. The gap to close
is a **private container registry** (see Phase 2 below).

### APT repository

```
/var/lib/unheaded/apt-repo/     # local APT repo served to EAST
```

Already referenced in `runbooks/infra/apt-repo-server.yaml`. This is the
distribution mechanism for binaries to EAST: WEST builds `.deb` packages,
signs them, publishes to the local repo, and EAST installs via `apt`.

---

## Implementation Phases

This ADR is intentionally PLANNED — implementation is deferred until the UPC
kernel work reaches a stable point. Phases are ordered by dependency.

### Phase 1 — Binary install discipline (prerequisite for EAST deploys)

- Wire `make install` target: `go build` → `DESTDIR=/usr/local/bin`
- Add `make install-east` that `scp`s binaries to EAST and restarts units via SSH
- Rename `cmd/host-agent` → `cmd/huginn` (per ADR-084)
- Ensure all binaries in `deploy/systemd/*.service` `ExecStart` lines exist after
  `make install`

### Phase 2 — `.deb` packaging per service

- One `.deb` per Kingdom service (timeguru, captain, architect, etc. + huginn)
- Postinst: create `unheaded` system user, create `/var/lib/unheaded/<svc>/`,
  copy unit file to `/etc/systemd/system/`, run `systemctl daemon-reload`
- Prerm: `systemctl stop`, `systemctl disable` if enabled
- Signed with the Kingdom GPG key
- Published to local APT repo at `/var/lib/unheaded/apt-repo/`

### Phase 3 — CI pipeline (Jenkins or GHA)

- On merge to `main`: `go build ./...`, `cargo build --release` (non-ascend +
  ascend variants), run tests, build `.deb` packages, publish to local APT repo
- EAST auto-upgrade hook: `apt update && apt upgrade unheaded-*` after repo publish
- Build artifacts (`.deb` files) versioned by git commit SHA
- `/var/cache/unheaded/` used for Go/Rust/pip caches across runs

### Phase 4 — Container registry (optional, future)

- Private OCI registry (Gitea, Zot, or similar) at a Kingdom-internal address
- Docker Compose on WEST pulls from registry instead of building locally
- Images tagged by git SHA + semver; rollback = tag switch + `docker compose up`

---

## Consequences

- `pkill -f <process>` is never the right stop mechanism — `systemctl stop` always
- Source-tree `bin/` stays as build output; CI cleans it; never referenced at runtime
- EAST upgrades become: `apt update && apt install unheaded-<svc>` (Phase 2+)
- `/var/lib/unheaded/` is the single persistent-state root for all Kingdom services
- The `unheaded` system user is the owner of all runtime paths — consistent with
  the systemd unit `User=unheaded` directive

---

## Related

- ADR-084 — Huginn host metrics agent (systemd units, binary naming)
- `runbooks/infra/apt-repo-server.yaml` — APT repo setup runbook
- `scripts/build-all-debs.sh` — existing .deb build script (needs wiring into CI)
- `scripts/build-sealed-cask.sh` — deterministic image builder
- `deploy/systemd/README.md` — install instructions (current manual process)
