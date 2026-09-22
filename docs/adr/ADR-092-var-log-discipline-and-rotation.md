<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-092 — Every service logs to `/var/log/unheaded/<service>/`, with its own rotation budget

**Status:** Proposed
**Date:** 2026-09-22
**Supersedes:** nothing.
**Related:** ADR-084 (Huginn host metrics), ADR-086 (Muninn observability fan-out),
ADR-091 (The Well initdb ordering — same failure shape: a path nothing audits),
`docs/policy/retention-policy-v1-2026-05-06.md` (retention classes),
`runbooks/observe/log-rotation.yaml` (the runbook this ADR replaces).

## Context

There is no single answer today to "where does service X log, and what stops it
filling the disk?" There are four answers, and none of them is `/var/log`.

**1. Container log caps exist only on this one host, untracked.**
`docker-compose.yml` declares no `logging:` block for any of the 17 services, so
each inherits the daemon default. On this machine that default is set in
`/etc/docker/daemon.json` to `max-size: 100m, max-file: 5` — a 500 MB ceiling
per container, 8.5 GB across the fleet. **That file is not in the repository.**
A fresh host, a CI runner, a new contributor's laptop or a disaster-recovery
rebuild gets Docker's own default instead, which is `json-file` with no
`max-size` and no `max-file`: the log grows until the filesystem is full.

This is the ADR-091 shape exactly — a protection that holds only because of
state on one long-lived box, where the broken path is reachable only by the
people and situations that can least afford it. It is also not hypothetical
that a service can produce that volume: `c15c0e9d` (B8) fixed a
dashboard-backend log storm of **1.6M "broadcast channel full" lines** in under
an hour. The container was OOM-killed before the disk noticed, which is the only
reason it did not become a disk-full incident on a host that happened to be
capped anyway.

**2. Bare-metal and helper processes log to `/tmp`.** 31 distinct `/tmp/*.log`
paths appear across `scripts/`, `runbooks/`, `raft/` and `deploy/` — `zhen.log`,
`llama-server.log`, `akira.log`, `dashboard.log`, the doom harness set, the
injectors. `/tmp` is the wrong place on every axis: it is world-writable (any
local user can pre-create or truncate a log a privileged process later opens),
it is cleared on reboot so post-mortem evidence is destroyed exactly when it is
wanted, it is `tmpfs` on some hosts so logs consume RAM, and it is not covered
by any default rotation.

**3. Vector ships container stdout to ClickHouse** (`config/vector/vector.yaml`,
`docker_logs` source). Good for querying, but it is a *copy*. It does not bound
what the json-file driver keeps on disk, and it does not exist on a host where
Vector is not running.

**4. `logagg` publishes structured logs to Wotan** (`918808fd`). Also a copy,
also in-memory at the far end, and deliberately lossy — it drops on a full queue.

So: the durable, local, on-disk copy — the one you read when the network is
down, the message bus is the thing that is broken, or you are doing forensics on
a box that has already rebooted — is either uncapped in
`/var/lib/docker/containers/` or in a directory that gets erased.

The existing `runbooks/observe/log-rotation.yaml` acknowledges the problem and
then encodes the mistake: it installs a logrotate stanza **for `/tmp/*.log`**. It
also uses `copytruncate`, which races any writer that holds an offset and
silently loses the lines written between the copy and the truncate.

Separately, `feedback: service-nologin-users` requires every third-party service
to run as its own dedicated `nologin` system user. Those users need somewhere to
write that is not `/tmp` and not world-writable.

## Decision

**1. One canonical location.** Every Unheaded-authored service writes its durable
local log to `/var/log/unheaded/<service>/<service>.log`. One directory per
service, owned by that service's dedicated user, mode `0750`, group-readable by a
`unheaded-logreaders` group so an operator (and Huginn) can read without being
root and without any service being able to read another's log.

**2. Journald stays the front door for systemd units.** Units that already log to
stdout keep doing so; journald is the collector and `/var/log/unheaded/` is where
a unit puts anything it must own the file handle for (large structured dumps,
per-run artefacts, crash context). This ADR does not ask every service to stop
using stdout — it asks that wherever a service *does* open a file, the file is in
one predictable, owned, rotated place.

**3. Containers get an explicit, capped logging driver — in the repository.**
The cap must not depend on an untracked host file. Every service in
`docker-compose.yml` declares:

```yaml
logging:
  driver: json-file
  options:
    max-size: "10m"
    max-file: "3"
```

30 MB ceiling per container, enforced by the daemon, with no cron and no
logrotate involvement. A per-service override is allowed where justified, in the
compose file, with a comment saying why. This is deliberately far tighter than
the 500 MB this host currently allows: compose is read by everyone who runs the
stack, so the number in the tree is the one that should be defensible. Vector
already ships container stdout to ClickHouse, so the on-disk json file is a
local buffer, not the archive.

The host-level `daemon.json` should still set a sane default — it is the
backstop for anything started outside compose — but it belongs in the
provisioning tree rather than only in `/etc` on one machine.

**4. Per-service logrotate configs, not one shared stanza.** Each service ships
`/etc/logrotate.d/unheaded-<service>`. One file per service so a service can be
added or removed without editing a shared file, and so a malformed stanza takes
out one service's rotation rather than all of them. House style:

```
/var/log/unheaded/<service>/*.log {
    daily
    rotate 7
    maxsize 100M          # rotate early if it blows up before the daily tick
    compress
    delaycompress
    missingok
    notifempty
    create 0640 <service> unheaded-logreaders
    su <service> unheaded-logreaders
    sharedscripts
    postrotate
        # signal the writer to reopen; NEVER copytruncate
        systemctl kill -s HUP --kind=main unheaded-<service>.service 2>/dev/null || true
    endscript
}
```

**`copytruncate` is banned.** It loses whatever is written between the copy and
the truncate, and it is the reason a rotation "works" in testing and drops lines
under load. Services reopen on `SIGHUP` instead; a service that cannot reopen
gets `create` plus a restart, documented in its unit.

**5. A catastrophic-failure ceiling that does not depend on the daily tick.**
`maxsize 100M` makes logrotate act within its hourly run rather than waiting for
midnight. That still leaves a gap: a service panicking in a tight loop can write
gigabytes between hourly runs. So, in addition:

- `/var/log` is a **separate filesystem** (or a quota'd dataset) on any host the
  Kingdom provisions, so filling it cannot take down the root filesystem, the
  container runtime, or Postgres.
- `journald` gets `SystemMaxUse=` and `RuntimeMaxUse=` set explicitly rather than
  left at "10% of the filesystem", which on a large disk is a very large number.
- Huginn (ADR-084) exports `/var/log` free space and per-service log directory
  size as metrics, with an alert **before** the filesystem is full, not after.

**6. The panic case is a bug, not a capacity problem.** A service emitting
enough log volume to hit these ceilings is malfunctioning. The ceilings exist to
keep one malfunctioning service from taking the host down — not to make
unbounded logging survivable. Rate limiting belongs in the service (the
warn-once-per-5s pattern from `c15c0e9d` is the reference), and a service that
trips its `maxsize` should be treated as a defect to fix, with the alert routed
accordingly.

## Consequences

**Good.** One place to look. A service's logs survive reboot. No service can
read another's. Disk-full from logging requires simultaneously defeating a
per-container cap, a per-service rotation budget, a separate filesystem and an
alert. The `/tmp` class of bug (world-writable, cleared on reboot, RAM-backed)
is gone. Each `nologin` service user has a writable directory it owns, which the
least-privilege model needs anyway.

**Costs.** 31 `/tmp/*.log` call sites to migrate, each one a place a script or
unit hardcodes a path. Per-service logrotate files are more files to keep in sync
with the service list — a drift risk, and the mitigation is generation from the
same source of truth that generates the compose services, plus a CI check that
every service has exactly one stanza. Making `/var/log` a separate filesystem is
a provisioning change, so it lands with the NixOS/host work, not with the
application change.

**Migration is mechanical but not free.** Doing the `logrotate` configs before
the `/tmp` migration would produce configs pointing at paths nothing writes — the
ADR-091 shape, a mechanism that "works" because nothing exercises it. Order
matters:

1. compose `logging:` caps (independent, immediately useful, no code change)
2. `/var/log/unheaded/<service>/` directories + ownership in provisioning
3. migrate call sites off `/tmp`, service by service
4. per-service logrotate stanzas, each landing *after* its service's paths move
5. CI check: every service has a directory, a stanza, and a compose cap
6. retire `runbooks/observe/log-rotation.yaml`, or rewrite it to verify rather
   than to install
7. `/var/log` as its own filesystem + journald caps + Huginn alerting

**Explicitly deferred.** Whether ClickHouse retention (Vector's sink) changes at
all — that is a retention-policy question, covered by
`docs/policy/retention-policy-v1-2026-05-06.md`, not a disk-safety question.
Central syslog/rsyslog forwarding is Muninn's problem (ADR-086), not this one.

## Verification

A gate that cannot fail is not a gate (`check-timeline-freshness.sh`, three
batches). This one is checkable:

- every service in `docker-compose.yml` has a `logging:` block with `max-size` —
  a shell check over the compose file, failing loudly. Checking the *running*
  containers instead would pass on this host for the wrong reason, by reading
  the untracked daemon default rather than what the repository guarantees
- every service has exactly one `/etc/logrotate.d/unheaded-*` stanza, and
  `logrotate -d` parses all of them clean
- `grep -rn '/tmp/[a-z-]*\.log'` over tracked source returns **zero** outside
  tests and ephemeral scratch — this count is 31 today and is the migration's
  progress metric
- a provoked rotation loses no lines: write a known sequence, force
  `logrotate -f`, keep writing, verify the concatenation of the rotated set is
  gap-free. This is the check that would catch a `copytruncate` regression, and
  it must be provoked to red before it is trusted
