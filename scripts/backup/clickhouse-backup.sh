#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# clickhouse-backup.sh: one backup set of ClickHouse's log database (ADR-147
# decision 6, S-SEE-AND-SURVIVE Phase 9). ClickHouse holds the audit logs
# for 455 days and had no copy (Erda F15).
#
# ClickHouse writes the set itself: BACKUP DATABASE ... TO File('<name>')
# under /backups in the container, which compose binds to CH_BACKUP_DIR
# (config/clickhouse/backups.xml allows only that path). Beside each set:
#   <name>.SHA256SUMS   over every file of the set
#   <name>.meta.env     NAME, DB, FROM, CUTOFF, ROWS, IMAGE, BYTES
#   LATEST              the newest set's name (0644)
# ROWS is count() FINAL of the logs between FROM (CUTOFF - 89 days) and
# CUTOFF (5 minutes before the backup): inside every TTL, so neither a
# merge nor a TTL pass can change it after a restore. clickhouse-drill.sh
# restores the set into a throwaway container and must count the same.
# The newest CH_BACKUP_KEEP sets are kept.
#
# Root: it reads the container's mounts and writes beside sets ClickHouse
# owns (uid 16707). The password stays in the container's environment: the
# query runs through `docker exec ... sh -c`, never in the host's argv.
#
# Environment:
#   CH_CONTAINER          default unheaded-clickhouse
#   CH_BACKUP_DIR         set root on the host (default /srv/backups/unheaded-clickhouse)
#   CH_BACKUP_KEEP        sets kept (default 14)
#   CH_BACKUP_DB          database (default unheaded_logs)
#   CH_BACKUP_TABLE       table counted (default logs)
#   CH_BACKUP_OTHER_DISK  1: refuse when the sets share a disk with the data (the unit sets it)
#   STORE_METRICS_DIR     write store_backup_clickhouse.prom there on success
set -euo pipefail
umask 077

C="${CH_CONTAINER:-unheaded-clickhouse}"
ROOT="${CH_BACKUP_DIR:-/srv/backups/unheaded-clickhouse}"
KEEP="${CH_BACKUP_KEEP:-14}"
DB="${CH_BACKUP_DB:-unheaded_logs}"
TABLE="${CH_BACKUP_TABLE:-logs}"
TAG=clickhouse-backup
# shellcheck source=scripts/backup/lib-disks.sh
. "$(dirname "$0")/lib-disks.sh"

[[ "$KEEP" =~ ^[1-9][0-9]*$ ]] || { echo "[$TAG] CH_BACKUP_KEEP must be a positive integer" >&2; exit 2; }
[[ "$DB" =~ ^[a-z_][a-z0-9_]*$ && "$TABLE" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[$TAG] bad database or table name" >&2; exit 2; }
[[ "$C" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "[$TAG] bad container name" >&2; exit 2; }
[[ -d "$ROOT" ]] || { echo "[$TAG] $ROOT does not exist (install -d -m 0700 -o 16707 -g 16707 it)" >&2; exit 1; }
ROOT="$(cd "$ROOT" && pwd -P)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
START=$SECONDS

mount_src() { "${DOCKER[@]}" inspect "$C" --format "{{range .Mounts}}{{if eq .Destination \"$1\"}}{{.Source}}{{end}}{{end}}"; }
BACKUPS_SRC="$(mount_src /backups)" || { echo "[$TAG] cannot inspect $C" >&2; exit 1; }
[[ -n "$BACKUPS_SRC" && "$(cd "$BACKUPS_SRC" 2>/dev/null && pwd -P)" == "$ROOT" ]] ||
	{ echo "[$TAG] ClickHouse has no /backups mount of $ROOT: recreate it from docker-compose.yml" >&2; exit 1; }
if [[ "${CH_BACKUP_OTHER_DISK:-0}" == 1 ]]; then
	DATA="$(mount_src /var/lib/clickhouse)"
	[[ -n "$DATA" ]] || { echo "[$TAG] cannot find $C's data directory on the host" >&2; exit 1; }
	refuse_shared_disk "$DATA" "$ROOT" "$TAG"
fi

# ch <query>: one query, as the container's own user, password from its env.
ch() {
	# shellcheck disable=SC2016 # expanded inside the container, on purpose
	"${DOCKER[@]}" exec -i "$C" sh -c 'exec clickhouse-client --user "${CLICKHOUSE_USER:-default}" --password "${CLICKHOUSE_PASSWORD:-}" --query "$1"' sh "$1"
}

NAME="ch-$(date -u +%Y%m%dT%H%M%SZ)"
[[ ! -e "$ROOT/$NAME" ]] || { echo "[$TAG] $ROOT/$NAME already exists" >&2; exit 1; }
CUTOFF=$(($(date +%s) - 300))
FROM=$((CUTOFF - 89 * 86400))
ROWS="$(ch "SELECT count() FROM $DB.$TABLE FINAL WHERE timestamp >= toDateTime64($FROM, 3, 'UTC') AND timestamp < toDateTime64($CUTOFF, 3, 'UTC')")" ||
	{ echo "[$TAG] cannot count $DB.$TABLE" >&2; exit 1; }
[[ "$ROWS" =~ ^[0-9]+$ ]] || { echo "[$TAG] unexpected count: $ROWS" >&2; exit 1; }

echo "[$TAG] BACKUP DATABASE $DB TO File('$NAME') ($ROWS rows in the window)"
ANSWER="$(ch "BACKUP DATABASE $DB TO File('$NAME')")" || { echo "[$TAG] BACKUP failed" >&2; exit 1; }
STATUS="$(awk '{print $2}' <<<"$ANSWER")"
[[ "$STATUS" == BACKUP_CREATED ]] || { echo "[$TAG] BACKUP answered: $ANSWER" >&2; exit 1; }
[[ -d "$ROOT/$NAME" ]] || { echo "[$TAG] ClickHouse reported $STATUS but $ROOT/$NAME is not there" >&2; exit 1; }

(cd "$ROOT/$NAME" && find . -type f -print0 | sort -z | xargs -0 sha256sum) >"$ROOT/$NAME.SHA256SUMS"
IMAGE="$("${DOCKER[@]}" inspect -f '{{.Config.Image}}' "$C")"
BYTES="$(du -sb "$ROOT/$NAME" | cut -f1)"
printf 'NAME=%s\nDB=%s\nTABLE=%s\nFROM=%s\nCUTOFF=%s\nROWS=%s\nIMAGE=%s\nBYTES=%s\n' \
	"$NAME" "$DB" "$TABLE" "$FROM" "$CUTOFF" "$ROWS" "$IMAGE" "$BYTES" >"$ROOT/$NAME.meta.env"
printf '%s\n' "$NAME" >"$ROOT/.LATEST.tmp" && chmod 0644 "$ROOT/.LATEST.tmp" && mv -f "$ROOT/.LATEST.tmp" "$ROOT/LATEST"

# Retention: only ch-<UTC> sets directly under the root; each with its side files.
mapfile -t SETS < <(find "$ROOT" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | grep -E '^ch-[0-9]{8}T[0-9]{6}Z$' | sort -r)
for old in "${SETS[@]:KEEP}"; do
	rm -rf -- "${ROOT:?}/$old" "$ROOT/$old.SHA256SUMS" "$ROOT/$old.meta.env"
	echo "[$TAG] retention removed $old"
done

if [[ -n "${STORE_METRICS_DIR:-}" ]]; then
	write_prom "$STORE_METRICS_DIR" store_backup_clickhouse.prom \
		"# HELP unheaded_store_backup_last_success_timestamp_seconds Last complete backup set of the store (scripts/backup)." \
		"# TYPE unheaded_store_backup_last_success_timestamp_seconds gauge" \
		"unheaded_store_backup_last_success_timestamp_seconds{store=\"clickhouse\"} $(date +%s)" \
		"unheaded_store_backup_bytes{store=\"clickhouse\"} $BYTES" \
		"unheaded_store_backup_duration_seconds{store=\"clickhouse\"} $((SECONDS - START))"
fi
echo "[$TAG] backup complete: $ROOT/$NAME ($BYTES bytes, $ROWS rows); sets retained: $(( ${#SETS[@]} < KEEP ? ${#SETS[@]} : KEEP ))"
