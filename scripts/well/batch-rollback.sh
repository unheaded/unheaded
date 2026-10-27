#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Undo a migration batch: put back, from the backup batch-apply.sh took
# seconds before, every database the batch touched (Appendix A1).
#
#   scripts/well/batch-rollback.sh <pre-batch-dir>         # check the set, print the plan
#   scripts/well/batch-rollback.sh --yes <pre-batch-dir>   # do it
#
# <pre-batch-dir> is batch-apply.sh's <root>/pre-batch-<UTC>/: pending.tsv
# names the databases the batch touched, and the one backup set under it
# (scripts/db-backup.sh layout) holds their dumps.
#
# Why drop and re-create rather than pg_restore --clean: --clean drops only
# what the dump holds, so a table the batch created would survive, and the
# ledger is append-only, so the batch's rows in it cannot be deleted. Each
# touched database is therefore dropped (WITH (FORCE): its sessions are
# ended; services reconnect) and restored with pg_restore --create as the
# bootstrap superuser, as scripts/well/restore-drill.sh restores: the ledger
# belongs to the superuser, so no owner role could restore it. `postgres`
# itself, when touched (cluster-level files), is dropped and restored from
# template1. Then a role the batch created (a CREATE ROLE in a pending file,
# absent from the set's globals.sql) loses its remaining grants (DROP OWNED
# in every database) and is dropped. Writes made to a touched database after
# the backup are lost; untouched databases are left alone.
#
# Refuses, changing nothing, when: pending.tsv is missing or empty; there is
# not exactly one set; SHA256SUMS does not verify; a touched database has no
# dump. After a restore it checks that no pending file is left in any
# touched ledger and prints each ledger's tail.
#
# Environment: WELL_CONTAINER (default unheaded-postgres), WELL_MIGRATIONS_DIR
# (default db/migrations, for the pending files' CREATE ROLEs),
# WELL_ROLLBACK_TIMEOUT (seconds per docker call, default 1800).
# Exit: 0 rolled back (or plan printed), 1 failed, 2 refused or usage.
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
YES=0
if [[ "${1:-}" == --yes ]]; then YES=1; shift; fi
(($# == 1)) || { echo "usage: $0 [--yes] <pre-batch-dir>" >&2; exit 2; }
PRE="$1"
[[ -d "$PRE" ]] || { echo "[rollback] not a directory: $PRE" >&2; exit 2; }
C="${WELL_CONTAINER:-unheaded-postgres}"
DIR="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
T="${WELL_ROLLBACK_TIMEOUT:-1800}"
[[ "$T" =~ ^[1-9][0-9]*$ ]] || { echo "[rollback] WELL_ROLLBACK_TIMEOUT must be seconds" >&2; exit 2; }
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
x() { timeout "$T" "${DOCKER[@]}" exec -i "$C" "$@"; }

# The batch's record and its backup set.
[[ -s "$PRE/pending.tsv" ]] || { echo "[rollback] $PRE has no pending.tsv: not a batch-apply.sh directory" >&2; exit 2; }
mapfile -t SETS < <(find "$PRE" -mindepth 2 -maxdepth 2 -type d -name 'well-[0-9]*Z')
((${#SETS[@]} == 1)) || { echo "[rollback] expected one backup set under $PRE, found ${#SETS[@]}" >&2; exit 2; }
SET="${SETS[0]}"
for f in SHA256SUMS cluster.env globals.sql; do
	[[ -f "$SET/$f" ]] || { echo "[rollback] $SET lacks $f" >&2; exit 2; }
done
(cd "$SET" && sha256sum --quiet --strict -c SHA256SUMS) || { echo "[rollback] $SET: checksum mismatch, refusing" >&2; exit 2; }
SU="$(sed -n 's/^superuser=//p' "$SET/cluster.env")"
[[ "$SU" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[rollback] bad superuser in cluster.env: '$SU'" >&2; exit 2; }

mapfile -t DBS < <(cut -f1 "$PRE/pending.tsv" | sort -u)
mapfile -t FILES < <(cut -f2 "$PRE/pending.tsv")
for db in "${DBS[@]}"; do
	[[ "$db" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[rollback] bad database name in pending.tsv: '$db'" >&2; exit 2; }
	[[ -f "$SET/$db.dump" ]] || { echo "[rollback] the set has no dump of $db" >&2; exit 2; }
	grep -q "  $db.dump\$" "$SET/SHA256SUMS" || { echo "[rollback] $db.dump is not in SHA256SUMS" >&2; exit 2; }
done
for f in "${FILES[@]}"; do
	[[ "$f" =~ ^[0-9]{3}_[a-z0-9_]+\.sql$ ]] || { echo "[rollback] bad file name in pending.tsv: '$f'" >&2; exit 2; }
done
# `postgres` last: the other restores do not need it, and it is the one
# whose drop ends the most sessions.
ORDER=()
for db in "${DBS[@]}"; do [[ "$db" == postgres ]] || ORDER+=("$db"); done
for db in "${DBS[@]}"; do [[ "$db" == postgres ]] && ORDER+=("$db"); done

# Roles the batch created: named by a CREATE ROLE in a pending file and
# absent from the backup's globals.
ROLES=()
for f in "${FILES[@]}"; do
	[[ -f "$DIR/$f" ]] || continue
	while read -r role; do
		[[ "$role" =~ ^[a-z_][a-z0-9_]*$ ]] || continue
		grep -qE "^CREATE ROLE \"?$role\"?;" "$SET/globals.sql" && continue
		ROLES+=("$role")
	done < <(grep -oiE 'CREATE ROLE +[a-z_][a-z0-9_]*' "$DIR/$f" | awk '{print tolower($3)}' | sort -u)
done

echo "[rollback] set $SET (superuser $SU), container $C"
echo "[rollback] batch files: ${FILES[*]}"
echo "[rollback] databases to drop and restore: ${ORDER[*]}"
echo "[rollback] roles to drop: ${ROLES[*]:-(none)}"
if ((!YES)); then
	echo "[rollback] plan only: add --yes to do it (writes made since the backup to those databases are lost)"
	exit 0
fi

# </dev/null: inside a `while read` loop, exec -i would eat the loop's input.
sqlq() { x psql -X -q -At -v ON_ERROR_STOP=1 -U "$SU" -d "$1" -c "$2" </dev/null; }
for db in "${ORDER[@]}"; do
	echo "[rollback] $db: drop and restore"
	sqlq template1 "DROP DATABASE IF EXISTS \"$db\" WITH (FORCE)"
	x pg_restore -U "$SU" --exit-on-error --create -d template1 <"$SET/$db.dump"
done
if ((${#ROLES[@]})); then
	mapfile -t ALL < <(sqlq template1 "SELECT datname FROM pg_database WHERE datallowconn ORDER BY 1")
	for role in "${ROLES[@]}"; do
		[[ "$(sqlq template1 "SELECT count(*) FROM pg_roles WHERE rolname = '$role'")" == 1 ]] || continue
		for db in "${ALL[@]}"; do
			[[ "$db" =~ ^[A-Za-z0-9_]+$ ]] || { echo "[rollback] unexpected database name '$db'" >&2; exit 1; }
			sqlq "$db" "DROP OWNED BY \"$role\""
		done
		sqlq template1 "DROP ROLE \"$role\""
		echo "[rollback] dropped role $role"
	done
fi

# Check: no batch file left in a touched ledger.
bad=0
for db in "${ORDER[@]}"; do
	left="$(awk -F'\t' -v db="$db" '$1 == db {print $2}' "$PRE/pending.tsv" | while read -r f; do
		sqlq "$db" "SELECT filename FROM well_schema_migrations WHERE filename = '$f'"
	done)"
	if [[ -n "$left" ]]; then echo "[rollback] $db ledger still has: $left" >&2; bad=1; fi
	echo "[rollback] $db ledger tail:"
	sqlq "$db" "SELECT filename || '  ' || mode || '  ' || to_char(applied_at AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM well_schema_migrations ORDER BY filename DESC LIMIT 3" | sed 's/^/  /'
done
((bad == 0)) || exit 1
echo "[rollback] done: ${ORDER[*]} are as backed up in $SET. Role passwords are untouched (no role was restored); restart the services that use these databases."
