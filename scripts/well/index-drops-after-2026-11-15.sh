#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ERDA 5.3 (M-076): list The Well's indexes that no query has used, and
# print DROP INDEX CONCURRENTLY statements for them. It never drops anything.
#
#   scripts/well/index-drops-after-2026-11-15.sh > draft.sql
#
# Refuses before 2026-11-15 (UTC): the 2026-10-15 baseline found 22
# indexes at idx_scan 0 on 2.5 hours of statistics, which proves nothing.
# Also refuses when a database's statistics were reset less than 28 days
# ago (a crash resets them; a clean restart keeps them, PG 15+).
#
# Candidates: idx_scan = 0 in pg_stat_user_indexes, and not primary, not
# unique, not an exclusion or other constraint's index, valid. Each is
# printed as a comment (database, table, index, size, definition) followed
# by its DROP statement. The output is a draft for review, not a migration:
# drop only what no planned query needs (ERDA 5.3), then commit the list as
# the next numbered migration, one per database, routed `notx` in the
# MANIFEST (DROP INDEX CONCURRENTLY cannot run in a transaction), and apply
# it through scripts/well/batch-apply.sh in a window.
#
# Reads only: as WELL_INDEX_ROLE (default well_backup, read-only, 023) over
# the container's local socket, in read-only transactions.
#
# Environment: WELL_CONTAINER (default unheaded-postgres), WELL_INDEX_ROLE,
# WELL_INDEX_DBS (default "unheaded_app unheaded_ops unheaded_config"),
# WELL_INDEX_DROPS_NOW (epoch seconds; a rehearsal on a throwaway server
# only: it stands in for the clock in both gates).
# Exit: 0 report printed, 1 failed, 2 refused (too early, stats too young).
set -euo pipefail

NOT_BEFORE="2026-11-15T00:00:00Z"
MIN_STATS_DAYS=28
now="${WELL_INDEX_DROPS_NOW:-$(date -u +%s)}"
[[ "$now" =~ ^[0-9]+$ ]] || { echo "[index-drops] WELL_INDEX_DROPS_NOW must be epoch seconds" >&2; exit 2; }
gate="$(date -u -d "$NOT_BEFORE" +%s)"
if ((now < gate)); then
	echo "[index-drops] refusing: not before $NOT_BEFORE (ERDA 5.3 waits for a month of index statistics); $(((gate - now + 86399) / 86400)) day(s) to go" >&2
	exit 2
fi

C="${WELL_CONTAINER:-unheaded-postgres}"
ROLE="${WELL_INDEX_ROLE:-well_backup}"
[[ "$ROLE" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[index-drops] bad role name '$ROLE'" >&2; exit 2; }
read -r -a DBS <<<"${WELL_INDEX_DBS:-unheaded_app unheaded_ops unheaded_config}"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
q() { # db sql
	PGOPTIONS='-c default_transaction_read_only=on' \
		timeout 60 "${DOCKER[@]}" exec -i -e PGOPTIONS "$C" psql -X -q -At -F $'\t' -v ON_ERROR_STOP=1 -U "$ROLE" -d "$1" -c "$2" </dev/null
}

# Gate 2: every database's statistics are old enough to mean something.
for db in "${DBS[@]}"; do
	[[ "$db" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[index-drops] bad database name '$db'" >&2; exit 2; }
	reset="$(q "$db" "SELECT coalesce(extract(epoch FROM stats_reset)::bigint, 0) FROM pg_stat_database WHERE datname = current_database()")"
	[[ "$reset" =~ ^[0-9]+$ ]] || { echo "[index-drops] $db: cannot read stats_reset ('$reset')" >&2; exit 1; }
	# stats_reset is NULL until the first reset: then statistics run from
	# the cluster's start, which the postmaster start time bounds.
	if ((reset == 0)); then
		reset="$(q "$db" "SELECT extract(epoch FROM pg_postmaster_start_time())::bigint")"
	fi
	age=$(((now - reset) / 86400))
	if ((age < MIN_STATS_DAYS)); then
		echo "[index-drops] refusing: $db statistics are $age day(s) old, need $MIN_STATS_DAYS (reset or restart at $(date -u -d "@$reset" +%FT%TZ))" >&2
		exit 2
	fi
done

CANDIDATES="
SELECT s.schemaname, s.relname, s.indexrelname,
       pg_size_pretty(pg_relation_size(s.indexrelid)),
       pg_get_indexdef(s.indexrelid)
  FROM pg_stat_user_indexes s
  JOIN pg_index i ON i.indexrelid = s.indexrelid
 WHERE s.idx_scan = 0
   AND NOT i.indisprimary AND NOT i.indisunique AND NOT i.indisexclusion
   AND i.indisvalid
   AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = s.indexrelid)
 ORDER BY pg_relation_size(s.indexrelid) DESC, s.relname, s.indexrelname"

echo "-- ERDA 5.3 index-drop candidates, $(date -u -d "@$now" +%FT%TZ): idx_scan = 0, not a key or constraint."
echo "-- A DRAFT for review: keep any index a planned query needs; split per database; route notx."
total=0
for db in "${DBS[@]}"; do
	echo
	echo "-- ═══ $db ═══"
	n=0
	while IFS=$'\t' read -r schema table idx size def; do
		[[ -n "$idx" ]] || continue
		[[ "$schema" =~ ^[a-z_][a-z0-9_]*$ && "$idx" =~ ^[a-z_][a-z0-9_]*$ ]] ||
			{ echo "[index-drops] $db: unexpected name $schema.$idx, skipped" >&2; continue; }
		echo "-- $table.$idx ($size): $def"
		echo "DROP INDEX CONCURRENTLY IF EXISTS $schema.$idx;"
		n=$((n + 1))
	done < <(q "$db" "$CANDIDATES")
	((n > 0)) || echo "-- (none)"
	total=$((total + n))
done
echo
echo "-- $total candidate(s). Nothing was dropped."
