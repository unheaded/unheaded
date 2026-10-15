#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# The Well: one backup set of the whole cluster.
#
#   scripts/db-backup.sh [--keep N]
#
# A set is a directory <root>/<YYYY-MM-DD>/well-<HHMMSS>Z/ holding
#   globals.sql      roles and tablespaces (pg_dumpall --globals-only
#                    --no-role-passwords: no password hashes; role passwords
#                    come from the environment, W-07, and are set again after
#                    a restore with scripts/well/set-role-passwords.sh;
#                    holds password hashes, hence 0600)
#   <db>.dump        pg_dump -Fc of every database that accepts connections
#                    (enumerated from pg_database, never a fixed list)
#   cluster.env      superuser=<name> and server_version=<num>: a restore
#                    needs a server bootstrapped with the same superuser
#                    (globals.sql grants GRANTED BY it)
#   rowcounts.tsv    db, table, exact row count, taken in the same snapshot
#                    as that database's dump (pg_export_snapshot), so a
#                    restore drill can compare like with like
#   SHA256SUMS       over every file above
# It is written under a .partial name and renamed only when every step has
# passed, including pg_restore -l over each dump. Any failure exits non-zero
# and leaves no set behind. Retention deletes only complete well-* sets.
#
# Environment:
#   WELL_BACKUP_DIR  root (default ~/backups/unheaded; refused inside the repo)
#   WELL_CONTAINER   container running The Well (default unheaded-postgres)
#   WELL_BACKUP_KEEP sets to keep (default 14; --keep overrides)
#   WELL_BACKUP_ROLE role to dump as (default well_backup: pg_read_all_data,
#                    db/migrations/023; read-only, never the superuser)
#
# After each set, <root>/LATEST (0644) names it: recency is visible to a
# probe that cannot read the 0700 sets themselves.
#
# Restore and verify a set: scripts/well/restore-drill.sh <set-dir>.
set -euo pipefail
umask 077

KEEP="${WELL_BACKUP_KEEP:-14}"
if [[ "${1:-}" == --keep ]]; then KEEP="${2:?--keep needs a number}"; shift 2; fi
(($# == 0)) || { echo "usage: $0 [--keep N]" >&2; exit 2; }
[[ "$KEEP" =~ ^[1-9][0-9]*$ ]] || { echo "[well] keep must be a positive integer, got '$KEEP'" >&2; exit 2; }

ROOT="${WELL_BACKUP_DIR:-$HOME/backups/unheaded}"
C="${WELL_CONTAINER:-unheaded-postgres}"
R="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$ROOT"
ROOT="$(cd "$ROOT" && pwd -P)"
case "$ROOT/" in "$(cd "$R" && pwd -P)"/*)
	echo "[well] refusing to write backups inside the repo: $ROOT" >&2; exit 2 ;;
esac

DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
ROLE="${WELL_BACKUP_ROLE:-well_backup}"
[[ "$ROLE" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[well] bad role name '$ROLE'" >&2; exit 2; }
"${DOCKER[@]}" exec "$C" pg_isready -q -U "$ROLE" -d postgres || { echo "[well] $C is not running or not accepting connections" >&2; exit 1; }
# The bootstrap superuser (oid 10): globals grant GRANTED BY it, so a restore
# must bootstrap with the same name (scripts/well/restore-drill.sh).
SU="$("${DOCKER[@]}" exec "$C" psql -X -At -v ON_ERROR_STOP=1 -U "$ROLE" -d postgres -c 'SELECT rolname FROM pg_roles WHERE oid = 10')" ||
	{ echo "[well] cannot connect as $ROLE (db/migrations/023 not applied?)" >&2; exit 1; }

DAY="$(date -u +%F)" T="$(date -u +%H%M%S)"
FINAL="$ROOT/$DAY/well-${T}Z"
PART="$ROOT/$DAY/.partial-well-${T}Z"
[[ ! -e "$FINAL" ]] || { echo "[well] $FINAL already exists" >&2; exit 1; }
mkdir -p "$PART"
cleanup() { [[ -d "$PART" ]] && rm -rf -- "$PART"; }
trap cleanup EXIT

exec_su() { "${DOCKER[@]}" exec -i "$C" "$@"; }

echo "[well] backing up $C into $FINAL"
exec_su pg_dumpall -U "$ROLE" --globals-only --no-role-passwords >"$PART/globals.sql"
! grep -qiE "PASSWORD '(SCRAM|md5)" "$PART/globals.sql" || { echo "[well] globals dump holds password hashes" >&2; exit 1; }
grep -q '^CREATE ROLE' "$PART/globals.sql" || { echo "[well] globals dump has no roles" >&2; exit 1; }
[[ "$SU" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[well] unexpected superuser name '$SU'" >&2; exit 1; }
printf 'superuser=%s\nserver_version=%s\n' "$SU" \
	"$(exec_su psql -X -At -U "$ROLE" -d postgres -c 'SHOW server_version_num')" >"$PART/cluster.env"

mapfile -t DBS < <(exec_su psql -X -At -v ON_ERROR_STOP=1 -U "$ROLE" -d postgres \
	-c "SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate ORDER BY 1")
((${#DBS[@]} > 0)) || { echo "[well] no databases found" >&2; exit 1; }

printf 'db\ttable\trows\n' >"$PART/rowcounts.tsv"
# One row per ordinary or partitioned table outside the system schemas, with
# an exact count. query_to_xml lets one statement count every table.
COUNTS="SELECT format('%I.%I', n.nspname, c.relname),
       (xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM %I.%I', n.nspname, c.relname), false, true, '')))[1]::text
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE c.relkind IN ('r', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema')
   AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp%'
 ORDER BY 1;"

for db in "${DBS[@]}"; do
	[[ "$db" =~ ^[A-Za-z0-9_]+$ ]] || { echo "[well] unexpected database name '$db'" >&2; exit 1; }
	# Hold a REPEATABLE READ transaction open, export its snapshot, dump in
	# that snapshot, then count in the same transaction.
	coproc PSQL { exec_su psql -X -q -At -v ON_ERROR_STOP=1 -U "$ROLE" -d "$db" 2>&1; }
	echo "BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SELECT pg_export_snapshot();" >&"${PSQL[1]}"
	IFS= read -r -t 30 snap <&"${PSQL[0]}" || { echo "[well] $db: no snapshot" >&2; exit 1; }
	[[ "$snap" =~ ^[0-9A-F]+-[0-9A-F]+-[0-9]+$ ]] || { echo "[well] $db: bad snapshot '$snap'" >&2; exit 1; }
	exec_su pg_dump -U "$ROLE" -Fc --snapshot="$snap" -d "$db" >"$PART/$db.dump"
	printf '%s\n' "$COUNTS" "SELECT '__end__';" >&"${PSQL[1]}"
	while IFS= read -r -t 300 line <&"${PSQL[0]}"; do
		[[ "$line" == __end__ ]] && break
		[[ "$line" == *'|'* ]] || { echo "[well] $db: count failed: $line" >&2; exit 1; }
		printf '%s\t%s\t%s\n' "$db" "${line%%|*}" "${line#*|}" >>"$PART/rowcounts.tsv"
	done
	[[ "${line:-}" == __end__ ]] || { echo "[well] $db: counts did not finish" >&2; exit 1; }
	echo "COMMIT;" >&"${PSQL[1]}"
	fd=${PSQL[1]}
	exec {fd}>&-
	wait "$PSQL_PID" || { echo "[well] $db: snapshot session failed" >&2; exit 1; }
	exec_su pg_restore -l <"$PART/$db.dump" >/dev/null || { echo "[well] $db: dump does not list" >&2; exit 1; }
	echo "[well]   $db: $(du -h "$PART/$db.dump" | cut -f1), $(awk -F'\t' -v d="$db" '$1==d' "$PART/rowcounts.tsv" | wc -l) tables"
done

(cd "$PART" && sha256sum cluster.env globals.sql ./*.dump rowcounts.tsv | sed 's# \./# #' >SHA256SUMS)
chmod 0600 "$PART"/*
mv -- "$PART" "$FINAL"
chmod 0700 "$FINAL"
trap - EXIT
printf '%s\n' "${FINAL#"$ROOT"/}" >"$ROOT/.LATEST.tmp" && chmod 0644 "$ROOT/.LATEST.tmp" && mv -f "$ROOT/.LATEST.tmp" "$ROOT/LATEST"

# Retention: newest $KEEP complete sets survive. Only well-<time>Z
# directories directly under a date directory are candidates.
mapfile -t SETS < <(find "$ROOT" -mindepth 2 -maxdepth 2 -type d -name 'well-[0-9][0-9][0-9][0-9][0-9][0-9]Z' \
	-path "$ROOT/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/*" | sort -r)
for old in "${SETS[@]:KEEP}"; do
	rm -rf -- "$old"
	rmdir --ignore-fail-on-non-empty -- "$(dirname "$old")"
	echo "[well] retention removed $old"
done
# ERDA 1.4: on success only, so a failed backup shows as a stale timestamp.
if [[ -n "${WELL_METRICS_DIR:-}" ]]; then
	m="$WELL_METRICS_DIR/.well_backup.prom.tmp"
	{
		echo "# HELP unheaded_well_backup_last_success_timestamp_seconds Last complete backup set of The Well."
		echo "# TYPE unheaded_well_backup_last_success_timestamp_seconds gauge"
		echo "unheaded_well_backup_last_success_timestamp_seconds $(date +%s)"
		echo "unheaded_well_backup_duration_seconds $SECONDS"
		echo "unheaded_well_backup_size_bytes $(du -sb "$FINAL" | cut -f1)"
		echo "unheaded_well_backup_databases ${#DBS[@]}"
		echo "unheaded_well_backup_rows $(awk -F'\t' 'NR > 1 {n += $3} END {print n + 0}' "$FINAL/rowcounts.tsv")"
	} >"$m" && chmod 0644 "$m" && mv -f "$m" "$WELL_METRICS_DIR/well_backup.prom"
fi
echo "[well] backup complete: $FINAL ($(du -sh "$FINAL" | cut -f1)); sets retained: $((${#SETS[@]} < KEEP ? ${#SETS[@]} : KEEP))"
