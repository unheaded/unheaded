#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Restore drill: proves a backup set from scripts/db-backup.sh restores.
#
#   scripts/well/restore-drill.sh [--image IMAGE] [--into-volume VOLUME] [SET_DIR]
#
# SET_DIR defaults to the newest complete set under WELL_BACKUP_DIR
# (default ~/backups/unheaded). The drill:
#   1. checks SHA256SUMS (every listed file, and no unlisted dump);
#   2. starts a throwaway container (no network, no volume, removed on exit)
#      bootstrapped with the set's superuser (cluster.env), as The Well is:
#      globals.sql grants roles GRANTED BY that superuser, which only the
#      bootstrap superuser can do;
#   3. applies globals.sql (minus its CREATE ROLE for the bootstrap
#      superuser, which initdb already made), then pg_restore --create
#      --exit-on-error for each dump (into the existing `postgres`);
#   4. counts every table and compares with rowcounts.tsv, which the backup
#      took in the dump's own snapshot: any difference fails the drill;
#   5. prints the restore time (the RTO this set gives today).
# It never touches The Well. Exit 0 = the set restores exactly.
#
# IMAGE defaults to the image of WELL_CONTAINER (unheaded-postgres) when that
# container exists, else postgres:16.15-alpine: restore with the same server
# major the dump came from. A newer major works too: that is how a set moves
# to PostgreSQL 18 (runbooks/data/well-pg18-upgrade.yaml).
#
# --into-volume VOLUME restores into a NEW docker volume and keeps it (data
# checksums on, server stopped cleanly) instead of throwing the server away:
# the upgrade's data step. It refuses a volume that already exists, so it can
# never write into a live data volume, and deletes the volume it made when
# the restore or the row-count check fails. The volume is mounted at the
# image's one declared data volume (PG 16 /var/lib/postgresql/data, PG 18
# /var/lib/postgresql).
set -euo pipefail

IMAGE="" VOL=""
while [[ "${1:-}" == --* ]]; do
	case "$1" in
	--image) IMAGE="${2:?--image needs a value}"; shift 2 ;;
	--into-volume) VOL="${2:?--into-volume needs a value}"; shift 2 ;;
	*) echo "[drill] unknown option $1" >&2; exit 2 ;;
	esac
done
ROOT="${WELL_BACKUP_DIR:-$HOME/backups/unheaded}"
SET="${1:-}"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)

if [[ -z "$SET" ]]; then
	SET="$(find "$ROOT" -mindepth 2 -maxdepth 2 -type d -name 'well-[0-9][0-9][0-9][0-9][0-9][0-9]Z' \
		-path "$ROOT/[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/*" | sort | tail -1)"
	[[ -n "$SET" ]] || { echo "[drill] no backup set under $ROOT" >&2; exit 1; }
fi
[[ -d "$SET" ]] || { echo "[drill] not a directory: $SET" >&2; exit 2; }
if [[ -z "$IMAGE" ]]; then
	IMAGE="$("${DOCKER[@]}" inspect -f '{{.Config.Image}}' "${WELL_CONTAINER:-unheaded-postgres}" 2>/dev/null || true)"
	IMAGE="${IMAGE:-postgres:16.15-alpine}"
fi
echo "[drill] set $SET, image $IMAGE"

# 1. Integrity.
for f in SHA256SUMS cluster.env globals.sql rowcounts.tsv; do
	[[ -f "$SET/$f" ]] || { echo "[drill] missing $f" >&2; exit 1; }
done
(cd "$SET" && sha256sum --quiet --strict -c SHA256SUMS) || { echo "[drill] checksum mismatch" >&2; exit 1; }
mapfile -t DUMPS < <(find "$SET" -maxdepth 1 -type f -name '*.dump' -printf '%f\n' | sort)
((${#DUMPS[@]} > 0)) || { echo "[drill] no dumps in set" >&2; exit 1; }
for d in "${DUMPS[@]}"; do
	grep -q "  $d\$" "$SET/SHA256SUMS" || { echo "[drill] $d is not in SHA256SUMS" >&2; exit 1; }
	[[ "${d%.dump}" =~ ^[A-Za-z0-9_]+$ ]] || { echo "[drill] unexpected dump name $d" >&2; exit 1; }
done
SU="$(sed -n 's/^superuser=//p' "$SET/cluster.env")"
[[ "$SU" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[drill] bad superuser in cluster.env: '$SU'" >&2; exit 1; }

# 2. Throwaway server. WELL_DRILL_NAME lets a suite give it its own prefix
# (scripts/well/test-pq-groups.sh: qa-pg18-*), so its cleanup check can
# account for every container it caused.
N="${WELL_DRILL_NAME:-well-drill-$$}"
[[ "$N" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || { echo "[drill] bad WELL_DRILL_NAME: '$N'" >&2; exit 2; }
RUN=(--rm) KEEP=0
if [[ -n "$VOL" ]]; then
	[[ "$VOL" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{1,127}$ ]] || { echo "[drill] bad volume name: '$VOL'" >&2; exit 2; }
	if "${DOCKER[@]}" volume inspect "$VOL" >/dev/null 2>&1; then
		echo "[drill] volume $VOL already exists: --into-volume only fills a new one" >&2; exit 2
	fi
	"${DOCKER[@]}" image inspect "$IMAGE" >/dev/null 2>&1 || "${DOCKER[@]}" pull -q "$IMAGE" >/dev/null
	mapfile -t MP < <("${DOCKER[@]}" image inspect -f '{{range $k, $v := .Config.Volumes}}{{println $k}}{{end}}' "$IMAGE" | sed '/^$/d')
	((${#MP[@]} == 1)) || { echo "[drill] $IMAGE declares ${#MP[@]} volumes, expected one data volume" >&2; exit 2; }
	"${DOCKER[@]}" volume create "$VOL" >/dev/null
	# Not --rm: the container is removed explicitly, after a clean stop, so the
	# volume is never deleted while a server still has it.
	RUN=(-v "$VOL:${MP[0]}" -e POSTGRES_INITDB_ARGS=--data-checksums)
	echo "[drill] restoring into new volume $VOL at ${MP[0]}"
fi
cleanup() {
	"${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	if [[ -n "$VOL" ]]; then
		"${DOCKER[@]}" rm -f "$N" >/dev/null 2>&1 || true
		((KEEP)) || "${DOCKER[@]}" volume rm "$VOL" >/dev/null 2>&1 || echo "[drill] could not remove volume $VOL" >&2
	fi
}
trap cleanup EXIT
start=$(date +%s)
"${DOCKER[@]}" run -d "${RUN[@]}" --name "$N" --network none \
	-e POSTGRES_USER="$SU" -e POSTGRES_PASSWORD="throwaway-$RANDOM$RANDOM" -e POSTGRES_DB=postgres "$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 60); do
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U "$SU" -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { echo "[drill] throwaway server did not become ready" >&2; exit 1; }
x() { "${DOCKER[@]}" exec -i "$N" "$@"; }

# 3. Restore.
grep -vxF -e "CREATE ROLE $SU;" -e "CREATE ROLE \"$SU\";" "$SET/globals.sql" |
	x psql -X -q -v ON_ERROR_STOP=1 -U "$SU" -d postgres >/dev/null
for d in "${DUMPS[@]}"; do
	db="${d%.dump}"
	if [[ "$db" == postgres ]]; then
		x pg_restore -U "$SU" --exit-on-error -d postgres <"$SET/$d"
	else
		x pg_restore -U "$SU" --exit-on-error --create -d postgres <"$SET/$d"
	fi
done
restored=$(date +%s)

# 4. Row counts, same query shape as the backup.
COUNTS="SELECT format('%I.%I', n.nspname, c.relname),
       (xpath('/row/n/text()', query_to_xml(format('SELECT count(*) AS n FROM %I.%I', n.nspname, c.relname), false, true, '')))[1]::text
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE c.relkind IN ('r', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema')
   AND n.nspname NOT LIKE 'pg\_toast%' AND n.nspname NOT LIKE 'pg\_temp%'
 ORDER BY 1;"
got="$(mktemp)"; trap 'cleanup; rm -f "$got"' EXIT
printf 'db\ttable\trows\n' >"$got"
for d in "${DUMPS[@]}"; do
	db="${d%.dump}"
	x psql -X -At -F $'\t' -v ON_ERROR_STOP=1 -U "$SU" -d "$db" -c "$COUNTS" | sed "s/^/$db\t/" >>"$got"
done
if ! diff -u "$SET/rowcounts.tsv" "$got"; then
	echo "[drill] FAIL: restored row counts differ from the backup's (diff above: - backup, + restored)" >&2
	exit 1
fi
tables=$(($(wc -l <"$got") - 1)) rows=$(awk -F'\t' 'NR>1{s+=$3}END{print s+0}' "$got")
echo "[drill] PASS: ${#DUMPS[@]} databases, $tables tables, $rows rows match; restore took $((restored - start))s"
if [[ -n "$VOL" ]]; then
	KEEP=1
	echo "[drill] kept volume $VOL; its superuser password is a throwaway: set the real ones with scripts/well/set-role-passwords.sh"
fi
# ERDA 1.4: on a pass only, so a failing drill shows as a stale timestamp.
if [[ -n "${WELL_METRICS_DIR:-}" ]]; then
	m="$WELL_METRICS_DIR/.well_drill.prom.tmp"
	{
		echo "# HELP unheaded_well_drill_last_success_timestamp_seconds Last passing restore drill of The Well."
		echo "# TYPE unheaded_well_drill_last_success_timestamp_seconds gauge"
		echo "unheaded_well_drill_last_success_timestamp_seconds $(date +%s)"
		echo "unheaded_well_drill_restore_seconds $((restored - start))"
		echo "unheaded_well_drill_tables $tables"
		echo "unheaded_well_drill_rows $rows"
	} >"$m" && chmod 0644 "$m" && mv -f "$m" "$WELL_METRICS_DIR/well_drill.prom"
fi
