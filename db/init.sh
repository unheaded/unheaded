#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# The Well — ordered initialisation for the three-database split.
#
# This is the ONLY file mounted into /docker-entrypoint-initdb.d. The migrations
# live at /migrations and are applied from here, in order, each against the
# database it belongs to.
#
# The migrations directory MUST NOT be mounted into /docker-entrypoint-initdb.d.
# Postgres's entrypoint runs every *.sql it finds there itself, alphabetically,
# against POSTGRES_DB — which applies the per-database schemas to the wrong
# database and aborts on the first collision, before ever reaching this script.
# See docs/adr/ADR-091-the-well-initdb-ordering.md.
set -e

MIGRATIONS=/migrations

# The superuser is POSTGRES_USER, not `postgres` — with POSTGRES_USER set, the
# image creates that role and no `postgres` role exists to connect as.
psql_su() { psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$@"; }

echo "=== The Well — Initializing databases ==="

# Cluster-level: the three databases and the seven service-scoped users.
psql_su -f "$MIGRATIONS/002_create_databases.sql"

# Per-database schemas. This routing is the whole reason this script exists.
psql_su -d unheaded_app    -f "$MIGRATIONS/003_app_schema.sql"
psql_su -d unheaded_ops    -f "$MIGRATIONS/004_ops_schema.sql"
psql_su -d unheaded_config -f "$MIGRATIONS/005_config_schema.sql"
psql_su -d unheaded_config -f "$MIGRATIONS/006_seed_data.sql"

for db in unheaded_app unheaded_ops unheaded_config; do
  psql_su -d "$db" -f "$MIGRATIONS/007_grants.sql"
done

# 008-011 are the zhen tables. 008 and 009 self-route with `\connect
# unheaded_app`; 010 and 011 do not, so they are pointed there explicitly —
# unheaded_app is where zhen_app.py connects (ZHEN_DB_NAME default).
psql_su -d unheaded_app -f "$MIGRATIONS/008_zhen_memories.sql"
psql_su -d unheaded_app -f "$MIGRATIONS/009_zhen_actions.sql"
psql_su -d unheaded_app -f "$MIGRATIONS/010_zhen_conversations.sql"
psql_su -d unheaded_app -f "$MIGRATIONS/011_zhen_actions_relax_constraints.sql"

# 012 is entirely cluster-level (CREATE USER, GRANT pg_monitor, GRANT CONNECT),
# so it runs against the maintenance database.
psql_su -f "$MIGRATIONS/012_huginn_reader.sql"

# 001_initial_schema.sql is deliberately NOT applied — it is the pre-split
# single-database schema, superseded by 003/004/005. Applying it would put a
# fourth copy of kanban_tasks, timeline_milestones and zhen_conversations in the
# maintenance database, which is the isolation boundary the split exists to
# create. See the note at the top of that file.

echo "=== The Well — Ready ==="
