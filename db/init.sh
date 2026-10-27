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

# db/migrations/MANIFEST says which file runs against which database, in
# order; the well-migrate runner reads the same file for an existing Well (ADR-101).
# Each file is recorded in its database's well_schema_migrations ledger with
# its sha256, mode initdb, so a fresh Well starts with a complete ledger.
# Lines: `<database> <file> [notx] [superuser]`, `skip <file>`, blank, or #
# comments. (notx only matters to the runner: psql -f already sends each
# statement alone.) Like the runner, a file runs as its database's owner
# role once that owner is not a superuser (025), unless flagged superuser.
ledgers=" "
record() { # database file
  case "$ledgers" in *" $1 "*) ;; *)
    psql_su -d "$1" -q -f "$MIGRATIONS/LEDGER.sql"
    ledgers="$ledgers$1 " ;;
  esac
  sum="$(sha256sum "$MIGRATIONS/$2" | cut -d' ' -f1)"
  psql_su -d "$1" -q -v f="$2" -v h="$sum" <<'SQL'
INSERT INTO well_schema_migrations (filename, sha256, mode) VALUES (:'f', :'h', 'initdb');
SQL
}
# 012 (applied everywhere, so unchangeable) grants CONNECT on the pre-split
# database `unheaded`, retired 2026-10-15 (ERDA 3.6). It exists only while
# this script runs: made empty here, dropped after the manifest. This runs
# on an empty volume only, so nothing else can be in it.
psql_su -d postgres -q -c 'CREATE DATABASE unheaded'
exec 3<"$MIGRATIONS/MANIFEST"
while read -r db file flags <&3; do
  case "$db" in ''|'#'*|skip) continue ;; esac
  echo "The Well: $file -> $db"
  set -- -f "$MIGRATIONS/$file"
  case " $flags " in *" superuser "*) ;; *)
    owner="$(psql_su -d "$db" -Atq -c "SELECT r.rolname FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba WHERE d.datname = current_database() AND NOT r.rolsuper")"
    if [ -n "$owner" ]; then set -- -c "SET ROLE \"$owner\"" "$@"; fi ;;
  esac
  psql_su -d "$db" "$@"
  record "$db" "$file"
done
exec 3<&-
psql_su -d postgres -q -c 'DROP DATABASE unheaded'

# Role passwords come from the environment, never from the migrations (W-07).
# \getenv reads the variable inside psql, so the value is neither on a command
# line nor interpolated into SQL text; :'pw' quotes it as a literal. An unset
# or empty variable leaves the role without a password: it cannot log in
# until one is set (ALTER ROLE ... PASSWORD), and there is no default.
# The server still receives the statement with the password in it: this
# session neither logs it (log_statement = 'ddl' in db/postgresql.conf) nor
# keeps it in pg_stat_statements, which pg_monitor (huginn_reader) reads.
set_password() { # role env-var
  if [ -z "$(printenv "$2" || true)" ]; then
    echo "The Well: $2 unset, role $1 has no password (cannot log in yet)"
    return
  fi
  psql_su -d postgres -q <<SQL
\\getenv pw $2
SET log_statement = 'none';
SET log_min_duration_statement = -1;
SET log_min_error_statement = 'panic';
SET pg_stat_statements.track_utility = off;
ALTER ROLE $1 PASSWORD :'pw';
SQL
}
set_password app_kanban     APP_KANBAN_PASSWORD
set_password app_timeguru   APP_TIMEGURU_PASSWORD
set_password app_zhen       APP_ZHEN_PASSWORD
set_password app_auth       APP_AUTH_PASSWORD
set_password ops_writer     OPS_WRITER_PASSWORD
set_password ops_reader     OPS_READER_PASSWORD
set_password config_admin   CONFIG_ADMIN_PASSWORD
set_password config_reader  CONFIG_READER_PASSWORD
set_password huginn_reader  HUGINN_READER_PASSWORD
set_password well_backup    WELL_BACKUP_PASSWORD

echo "=== The Well — Ready ==="
