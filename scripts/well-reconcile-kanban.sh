#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# The Well — move kanban_tasks from the maintenance database to unheaded_app.
#
# Why this exists: until 2026-09-21 kanban-app pointed at WELL_DB=unheaded, the
# maintenance database, and the ADR-091 initdb bug had misfiled kanban_tasks
# there too — so the board worked only because the two errors cancelled. The
# pointer now says unheaded_app. On a CLEAN volume that is the end of it:
# init.sh creates the table there. On an EXISTING volume nothing moves the rows,
# and kanban-app would seed a demo board into an empty unheaded_app while the
# real tasks sat untouched next door. This script moves them.
#
# It is one-shot and refuses to guess:
#   - source table absent            -> nothing to do, exit 0
#   - destination already has rows   -> refuse, exit 2 (never merges two boards)
#   - destination table absent/empty -> copy schema + rows, then re-apply the
#                                       003_app_schema.sql grants
# The source table is left in place. Drop it by hand once the board has been
# verified from the browser; the maintenance database is not read by anything
# on the kanban path after this.
#
# Usage: scripts/well-reconcile-kanban.sh [--dry-run]
#   PG_CONTAINER (default unheaded-postgres), PG_USER (default unheaded)
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-unheaded-postgres}"
PG_USER="${PG_USER:-unheaded}"
SRC_DB="${SRC_DB:-unheaded}"
DST_DB="${DST_DB:-unheaded_app}"
TABLE=kanban_tasks
DRY_RUN=false
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=true

psql_in() { docker exec -i "$PG_CONTAINER" psql -v ON_ERROR_STOP=1 -qAt -U "$PG_USER" -d "$1"; }
sql()     { printf '%s\n' "$2" | psql_in "$1"; }

if ! docker exec "$PG_CONTAINER" pg_isready -U "$PG_USER" >/dev/null 2>&1; then
    echo "ERROR: $PG_CONTAINER is not accepting connections" >&2
    exit 1
fi

table_exists() { sql "$1" "SELECT to_regclass('public.$TABLE') IS NOT NULL"; }
row_count()    { sql "$1" "SELECT count(*) FROM $TABLE"; }

if [[ "$(table_exists "$SRC_DB")" != "t" ]]; then
    echo "$SRC_DB.$TABLE does not exist — nothing to reconcile"
    exit 0
fi
src_rows=$(row_count "$SRC_DB")

dst_rows=0
if [[ "$(table_exists "$DST_DB")" == "t" ]]; then
    dst_rows=$(row_count "$DST_DB")
fi

echo "source      $SRC_DB.$TABLE: $src_rows rows"
echo "destination $DST_DB.$TABLE: $dst_rows rows"

if [[ "$dst_rows" -gt 0 ]]; then
    echo "REFUSED: $DST_DB.$TABLE already has rows; merging two boards is a human decision" >&2
    exit 2
fi
if [[ "$src_rows" -eq 0 ]]; then
    echo "source is empty — nothing to copy"
    exit 0
fi
if $DRY_RUN; then
    echo "dry-run: would copy $src_rows rows into $DST_DB"
    exit 0
fi

# An empty destination table may exist with a different shape (kanban-app's
# EnsureSchema, or a pre-2026-09-21 003_app_schema.sql). Replace it wholesale
# so the copy is the live table, indexes and all, not a column-by-column guess.
sql "$DST_DB" "DROP TABLE IF EXISTS $TABLE"
docker exec "$PG_CONTAINER" pg_dump -U "$PG_USER" -d "$SRC_DB" -t "$TABLE" \
    --no-owner --no-privileges | psql_in "$DST_DB" >/dev/null

# Grants exactly as 003_app_schema.sql issues them; the dump dropped them.
psql_in "$DST_DB" <<'SQL'
GRANT SELECT ON kanban_tasks TO app_kanban, app_timeguru, app_zhen;
GRANT INSERT, UPDATE, DELETE ON kanban_tasks TO app_kanban;
SQL

copied=$(row_count "$DST_DB")
if [[ "$copied" -ne "$src_rows" ]]; then
    echo "ERROR: copied $copied rows, expected $src_rows" >&2
    exit 1
fi
echo "copied $copied rows into $DST_DB.$TABLE"
echo "restart kanban-app, verify the board in the browser, then drop $SRC_DB.$TABLE by hand"
