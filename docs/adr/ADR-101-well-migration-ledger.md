<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-101: The Well's migrations: one manifest, one ledger per database, one runner

**Status:** Proposed. Built and tested on throwaway servers 2026-10-14; nothing has run against the live Well (it is stopped; ERDA Phase 0 needs Stevie's yes).
**Date:** 2026-10-14
**Related:**

- ADR-016 (The Well)
- ADR-091 (initdb ordering: `db/init.sh` is the only initdb entry)
- ADR-004 / ADR-096 (dependencies: stdlib plus `lib/pq`, already in go.mod)
- `docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md` (W-03, W-09, W-10, W-16; Phase 3)

## Context

The ERDA audit (2026-10-13) found that The Well has no record of which
schema changes it has had:

- `db/init.sh` was the only schema path, and it runs once, on an empty
  volume. The compliance tables (013/014, 2026-10-01) reached a fresh Well
  but never the live one; that needed hand-run, unrecorded `psql` (W-09).
- Nothing says which version of a file ran. A file edited after it ran is
  invisible (W-03).
- No lock or statement timeouts: a migration that queues behind a lock
  queues every later query behind it (W-10).
- Four files carried psql meta-commands (`\connect`, `\c`, `\set`, `\echo`)
  for routing, tying them to psql and to hard-coded database names (W-16).

## Decision

1. **`db/migrations/MANIFEST` is the order and the routing.** Lines are
   `<database> <file> [notx]` or `skip <file>`. Cluster-level files (roles,
   databases) run against, and are recorded in, `postgres`. A file applied
   to several databases (007) is listed once per database. `notx` files run
   statement by statement outside a transaction (CREATE DATABASE refuses
   one). 001 is `skip`ped (pre-split schema, ADR-091).
2. **One ledger per database**, `well_schema_migrations` (`db/migrations/LEDGER.sql`):
   filename, sha256 of the file as it ran, mode (`initdb`, `apply`,
   `baseline`), when and by whom. Append-only by trigger, like the
   compliance history (013-016).
3. **`db/init.sh` reads the manifest** and records every file with mode
   `initdb`, so a fresh Well starts with a complete ledger.
4. **`cmd/well-migrate`** (stdlib + `lib/pq`) for an existing Well:
   - `check`: offline. Every numbered file is applied or skipped by the
     manifest, every named file exists, no psql meta-commands. CI runs it
     (`scripts/check-well-migrations.sh`, security.yml; meta-gate provoked).
   - `status`: ledger vs manifest. Exit 3 when something is pending, 1 on
     drift.
   - `apply` prints the plan; `apply -yes` runs pending files in manifest
     order and stops at the first failure.
   - `baseline -through FILE -yes` records pending files up to that
     manifest entry **without running them**: for a Well whose schema
     already matches up to there (the live one, after Phase 0's shape diff).
     `-through` is required; `apply -yes` then runs the rest.
5. **Drift refuses everything.** An applied file whose sha256 changed, a
   ledger row the manifest does not name, or a pending file before an
   applied one in the same database: status and apply refuse. Applied files
   are never edited; a change is a new file.
6. **Every migration runs with `lock_timeout` (5 s) and `statement_timeout`
   (5 min)**, set per transaction (`SET LOCAL`), under an advisory lock so
   two runners never race. A transactional migration and its ledger row
   commit together: a failure changes nothing and records nothing. A `notx`
   file that fails part way says how many statements ran.
7. **Migrations run as the database's owner** *(amended 2026-10-15, ERDA
   2.2)*. 025 gives each database a NOLOGIN owner role
   (`well_app_owner`, `well_ops_owner`, `well_config_owner`) and 026 hands
   it every object but the ledger and `well_append_only()`. From then on,
   init.sh and the runner `SET ROLE` to that owner for each file (in the
   transaction, or for the session of a `notx` file), so new objects belong
   to it on a fresh Well and a migrated one alike. A manifest line flagged
   `superuser` (`<database> <file> [notx] [superuser]`) keeps the
   superuser: for files that hand objects over or need cluster privileges.
   The ledger row is written after `RESET ROLE`. `scripts/well/test-migrate.sh`
   checks owners after init.sh and after apply, the flag, and that PUBLIC
   cannot connect.

## Consequences

- The four meta-command files (008-011) were edited to drop the
  meta-commands. The live Well ran the old versions; it has no ledger, so
  this costs nothing: Phase 0's baseline records the current files.
- Adding a migration is three steps: the file, its manifest line, and
  `scripts/well/render-k8s-initdb.py` (the Kubernetes initdb ConfigMap is
  rendered from `db/`, byte for byte, so the ledger checksums agree). CI
  fails on each missing step (`check-well-migrations`,
  `check-k8s-well-initdb`).
- Startup DDL in services (kanban store, champion pgstore, zhen) still
  exists (ERDA 3.3). Once the live Well is baselined, services can check the
  ledger instead of creating tables.
- `well-migrate` connects as the owner role (`WELL_MIGRATE_USER`, default
  `unheaded`) with `WELL_MIGRATE_PASSWORD`; there is no default password.
  Phase 2's per-database owner roles (ERDA 2.2) will replace the superuser
  here.

## Verification

- `go test ./pkg/database/migrate/`: manifest parsing (9 malformed cases),
  loader refusals (unrouted, missing, meta-commands), the SQL splitter
  (strings, E-strings, quoted identifiers, dollar quotes, nested comments),
  plan states (pending, mismatch, orphan, gap), and the real directory.
  Mutations of the gap rule, the dollar-quote lexer and the meta-command
  check each fail the tests.
- `scripts/well/test-migrate.sh` (throwaway servers): a fresh bootstrap
  records all 17 entries in the right ledgers and status is clean; a new
  file is pending, `apply` without `-yes` changes nothing, `apply -yes`
  runs and records it; an edited applied file stops status and apply; a
  migration blocked by an ACCESS EXCLUSIVE lock fails in 2 s with
  `-lock-timeout 2s` and records nothing; a failing migration rolls back;
  the ledger refuses UPDATE and TRUNCATE; a Well bootstrapped by the
  pre-ledger `db/` (like the live one) is baselined through 016 without
  any schema change, then `apply` runs 017-019.
