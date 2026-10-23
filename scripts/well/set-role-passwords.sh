#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Sets The Well's role passwords from the environment, as db/init.sh does
# on an empty volume (and the superuser's from POSTGRES_PASSWORD): after a restore (backups carry no password hashes,
# pg_dumpall --no-role-passwords) or to rotate one. Only roles whose
# variable is set are touched; the value goes to psql through the
# environment (\getenv), never a command line or SQL text, and the session
# keeps the statement out of the server log and pg_stat_statements.
#
#   APP_KANBAN_PASSWORD=... scripts/well/set-role-passwords.sh   # e.g. with `set -a; . ./.env`
set -euo pipefail
C="${WELL_CONTAINER:-unheaded-postgres}"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
SU="$("${DOCKER[@]}" exec "$C" printenv POSTGRES_USER)"
n=0
while read -r role var; do
	[[ -n "${!var:-}" ]] || continue
	"${DOCKER[@]}" exec -i -e "PW=${!var}" "$C" psql -X -q -v ON_ERROR_STOP=1 -v role="$role" -U "$SU" -d postgres <<'SQL'
\getenv pw PW
SET log_statement = 'none';
SET log_min_duration_statement = -1;
SET log_min_error_statement = 'panic';
SET pg_stat_statements.track_utility = off;
ALTER ROLE :"role" PASSWORD :'pw';
SQL
	echo "[well] $role: password set from $var"
	n=$((n + 1))
done <<ROLES
$SU POSTGRES_PASSWORD
app_kanban APP_KANBAN_PASSWORD
app_timeguru APP_TIMEGURU_PASSWORD
app_zhen APP_ZHEN_PASSWORD
ops_writer OPS_WRITER_PASSWORD
ops_reader OPS_READER_PASSWORD
config_admin CONFIG_ADMIN_PASSWORD
config_reader CONFIG_READER_PASSWORD
huginn_reader HUGINN_READER_PASSWORD
well_backup WELL_BACKUP_PASSWORD
ROLES
echo "[well] $n role password(s) set"
