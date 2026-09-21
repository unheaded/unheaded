<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-091 — The Well: one orchestrator in `initdb.d`, migrations mounted elsewhere

**Status:** Accepted
**Date:** 2026-08-04
**Supersedes:** nothing. **Related:** ADR-089 (branch promotion)

## Context

The Well is three PostgreSQL databases — `unheaded_app`, `unheaded_ops`,
`unheaded_config` — plus a maintenance database `unheaded` and seven
service-scoped users. The split is an isolation boundary, not a convenience: each
service connects as a user that can reach exactly one database.

`db/init.sh` exists to apply each migration to the database it belongs to,
because the routing cannot be expressed by file naming alone.

`docker-compose.yml` mounted **both** into the initdb directory:

```yaml
- ./db/migrations:/docker-entrypoint-initdb.d
- ./db/init.sh:/docker-entrypoint-initdb.d/init.sh
```

This does not work, and had not worked for some time. Nothing caught it because
`/docker-entrypoint-initdb.d` only runs on a **fresh data volume**, and the
long-lived `postgres-data` volume on this machine was initialised before the
three-database split. The broken path is only reachable by a new contributor, a
clean checkout, or a disaster-recovery restore — the three cases where it matters
most.

## What actually happened

Reproduced against `postgres:16-alpine` with the exact compose mounts:

1. The entrypoint runs **every** `*.sql` in `/docker-entrypoint-initdb.d` itself,
   in alphabetical order, against `POSTGRES_DB` (`unheaded`). It does not know
   about the routing.
2. `001_initial_schema.sql` creates `kanban_tasks` in `unheaded`.
3. `003_app_schema.sql` — which belongs to `unheaded_app` — runs against
   `unheaded` and dies: `ERROR: relation "kanban_tasks" already exists`.
4. The entrypoint treats a failed init script as fatal. **Container exits 3.**
5. `init.sh` sorts after every `NNN_*.sql`, so it **never ran at all**.

Two further defects were sitting behind that, unreachable and therefore unnoticed:

- `init.sh` connected as `-U postgres`. With `POSTGRES_USER=unheaded` the image
  creates that role and **no `postgres` role exists**, so every line would have
  failed the moment the script was reached.
- `init.sh` applied only 002–007. Migrations **008–012 were never applied by it** —
  the zhen tables (`zhen_memories`, `zhen_actions`, `zhen_conversations`) and the
  `huginn_reader` user. Zhen connects to `unheaded_app`
  (`raft/zhen_app.py:167`), but the entrypoint had been putting those tables in
  `unheaded`.

Separately, nesting the second mount inside the first meant Docker had to create
the mountpoint `/docker-entrypoint-initdb.d/init.sh` — and since that directory
*is* the host's `db/migrations`, it created the file **on the host**. A
root-owned empty `db/migrations/init.sh` appeared in the working tree on every
`docker compose up`, which a developer cannot delete without `sudo`.

## Decision

**`/docker-entrypoint-initdb.d` contains exactly one file: the orchestrator.**
Migrations are mounted read-only at `/migrations` and applied *by* the
orchestrator, never *by* the entrypoint.

```yaml
- ./db/migrations:/migrations:ro
- ./db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro
```

This matches what the Kubernetes path already did — `configmap-initdb.yaml`
mounts only `00-init.sh` — so the two deployment targets now agree. The `00-`
prefix is kept for that consistency, though with a single file ordering is moot.

Corollaries:

- **Never mount a volume inside another volume's mountpoint.** Docker creates the
  inner mountpoint through to the host filesystem.
- **Connect as `$POSTGRES_USER`**, never a hardcoded `postgres`.
- `001_initial_schema.sql` is **not applied**. It is the pre-split single-database
  schema, superseded by 003/004/005. It is annotated in place rather than deleted:
  the migration ledger is append-only, and it is the record of what the schema was
  before the split.

## Verification

Same image, same mounts, clean volume:

| | before | after |
|---|---|---|
| container | **exit 3** | running, exit 0 |
| `init.sh` ran | **never** | start to finish |
| `unheaded` (maintenance) | `kanban_tasks`, … | **zero tables** |
| `unheaded_app` | — | kanban, timeline, 4 zhen tables |
| `unheaded_ops` | — | audit_events, 3 service_health tables |
| `unheaded_config` | — | kingdom_config ×2, service_registry |
| service users | — | all 8, incl. `huginn_reader` |
| stray root-owned file | written every up | none |

The maintenance database holding **zero** tables is the load-bearing assertion:
that is the isolation boundary, and before this change it held a copy of the
application schema.

## Consequences

- The compose path can initialise from a clean volume. It could not before.
- No behaviour change for the existing `postgres-data` volume — `initdb.d` does
  not run against a populated data directory. **Nothing is migrated by this
  change**; a running Well keeps whatever layout it already has. Reconciling an
  existing volume that was initialised before the split is a separate, attended
  job, and this ADR does not attempt it.
- `git status` stops showing an undeletable root-owned file.

## Open

`002_create_databases.sql` and `012_huginn_reader.sql` carry hardcoded `*_dev`
passwords, while the Kubernetes ConfigMap takes them from a secret
(`${APP_KANBAN_PASSWORD}` etc). That is the known baselined lab-credential
posture recorded in CLAUDE.md, not a new finding — but the two targets differ,
and the compose side is the one with credentials in the tree. Raised here, not
decided here; it must be closed before any public exposure.
