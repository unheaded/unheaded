#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# clickhouse-drill.sh [set-name]: restore one ClickHouse backup set into a
# throwaway container and prove it (ADR-147 decision 6, S-SEE-AND-SURVIVE
# Phase 9). Default set: the one LATEST names.
#
#   1. Every file matches <name>.SHA256SUMS ("set damaged" otherwise).
#   2. <name>.meta.env is read through a parser that admits only
#      KEY=[A-Za-z0-9_:./@-]* lines (lib-disks.sh read_meta).
#   3. A container of the image that made the set starts with no network,
#      read-only, 2 GiB, its data in an empty scratch directory and the set
#      root read-only at /backups.
#   4. RESTORE DATABASE ... FROM File('<name>') must answer RESTORED, and
#      count() FINAL between FROM and CUTOFF must equal ROWS.
# A trap removes the container and the scratch directory on every exit. On
# success, store_drill_clickhouse.prom in STORE_METRICS_DIR.
#
# Environment: CH_BACKUP_DIR (default /srv/backups/unheaded-clickhouse),
# CH_DRILL_SCRATCH (default /srv/backups/.drill-clickhouse; must be absent or
# empty), CH_UID (default 16707), CH_BACKUPS_XML (default
# /usr/local/lib/unheaded/backup/clickhouse-backups.xml), CH_DRILL_WAIT
# (seconds for the server to answer, default 90), STORE_METRICS_DIR.
set -euo pipefail
umask 077

ROOT="${CH_BACKUP_DIR:-/srv/backups/unheaded-clickhouse}"
SCRATCH="${CH_DRILL_SCRATCH:-/srv/backups/.drill-clickhouse}"
CH_UID="${CH_UID:-16707}"
XML="${CH_BACKUPS_XML:-/usr/local/lib/unheaded/backup/clickhouse-backups.xml}"
WAIT="${CH_DRILL_WAIT:-90}"
TAG=clickhouse-drill
# shellcheck source=scripts/backup/lib-disks.sh
. "$(dirname "$0")/lib-disks.sh"

[[ "$CH_UID" =~ ^[0-9]+$ && "$WAIT" =~ ^[0-9]+$ ]] || { echo "[$TAG] CH_UID and CH_DRILL_WAIT are numbers" >&2; exit 2; }
[[ -f "$XML" ]] || { echo "[$TAG] $XML not found (config/clickhouse/backups.xml)" >&2; exit 1; }
NAME="${1:-$(cat "$ROOT/LATEST" 2>/dev/null || true)}"
[[ "$NAME" =~ ^ch-[0-9]{8}T[0-9]{6}Z$ ]] || { echo "[$TAG] no set to drill (name '$NAME')" >&2; exit 1; }
[[ -d "$ROOT/$NAME" && -f "$ROOT/$NAME.SHA256SUMS" && -f "$ROOT/$NAME.meta.env" ]] || { echo "[$TAG] $NAME is incomplete" >&2; exit 1; }
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
START=$SECONDS

(cd "$ROOT/$NAME" && sha256sum -c --quiet --strict "../$NAME.SHA256SUMS" >/dev/null 2>&1) ||
	{ echo "[$TAG] set damaged: $NAME does not match its SHA256SUMS" >&2; exit 1; }
read_meta "$ROOT/$NAME.meta.env"
for k in NAME DB TABLE FROM CUTOFF ROWS IMAGE; do
	v="META_$k"
	[[ -n "${!v:-}" ]] || { echo "[$TAG] $NAME.meta.env has no $k" >&2; exit 1; }
done
[[ "$META_NAME" == "$NAME" ]] || { echo "[$TAG] meta names $META_NAME, not $NAME" >&2; exit 1; }
[[ "$META_DB" =~ ^[a-z_][a-z0-9_]*$ && "$META_TABLE" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "[$TAG] bad database or table in meta" >&2; exit 1; }
[[ "$META_FROM" =~ ^[0-9]+$ && "$META_CUTOFF" =~ ^[0-9]+$ && "$META_ROWS" =~ ^[0-9]+$ ]] || { echo "[$TAG] bad numbers in meta" >&2; exit 1; }

if [[ -e "$SCRATCH" ]]; then
	[[ -d "$SCRATCH" && -z "$(ls -A "$SCRATCH")" ]] || { echo "[$TAG] $SCRATCH exists and is not empty: a drill left it (Appendix A 5)" >&2; exit 1; }
else
	mkdir -m 0700 "$SCRATCH"
fi
[[ "$(id -u)" -eq 0 ]] && chown "$CH_UID:$CH_UID" "$SCRATCH"
N="ch-drill-$$"
cleanup() {
	"${DOCKER[@]}" rm -f "$N" >/dev/null 2>&1 || true
	rm -rf -- "$SCRATCH"
}
trap cleanup EXIT

"${DOCKER[@]}" run -d --name "$N" --network none --user "$CH_UID:$CH_UID" --read-only --memory 2g \
	--tmpfs /tmp:size=64m \
	--tmpfs "/var/log/clickhouse-server:uid=$CH_UID,gid=$CH_UID,size=16m" \
	--tmpfs "/etc/clickhouse-server/users.d:uid=$CH_UID,gid=$CH_UID,size=1m" \
	-v "$SCRATCH:/var/lib/clickhouse" -v "$ROOT:/backups:ro" \
	-v "$XML:/etc/clickhouse-server/config.d/backups.xml:ro" \
	"$META_IMAGE" >/dev/null
q() { "${DOCKER[@]}" exec -i "$N" clickhouse-client --query "$1"; }
up=0
for _ in $(seq 1 "$WAIT"); do
	if q "SELECT 1" >/dev/null 2>&1; then up=1; break; fi
	sleep 1
done
((up)) || { "${DOCKER[@]}" logs --tail 20 "$N" >&2 || true; echo "[$TAG] the drill server did not answer within ${WAIT}s" >&2; exit 1; }

ANSWER="$(q "RESTORE DATABASE $META_DB FROM File('$NAME')")" || { echo "[$TAG] RESTORE failed" >&2; exit 1; }
[[ "$(awk '{print $2}' <<<"$ANSWER")" == RESTORED ]] || { echo "[$TAG] RESTORE answered: $ANSWER" >&2; exit 1; }
GOT="$(q "SELECT count() FROM $META_DB.$META_TABLE FINAL WHERE timestamp >= toDateTime64($META_FROM, 3, 'UTC') AND timestamp < toDateTime64($META_CUTOFF, 3, 'UTC')")"
[[ "$GOT" == "$META_ROWS" ]] || { echo "[$TAG] restored $GOT rows, the set says $META_ROWS" >&2; exit 1; }

if [[ -n "${STORE_METRICS_DIR:-}" ]]; then
	write_prom "$STORE_METRICS_DIR" store_drill_clickhouse.prom \
		"# HELP unheaded_store_drill_last_success_timestamp_seconds Last restore drill of the store that passed (scripts/backup)." \
		"# TYPE unheaded_store_drill_last_success_timestamp_seconds gauge" \
		"unheaded_store_drill_last_success_timestamp_seconds{store=\"clickhouse\"} $(date +%s)" \
		"unheaded_store_drill_duration_seconds{store=\"clickhouse\"} $((SECONDS - START))"
fi
echo "[$TAG] drill passed: $NAME restored, $GOT rows match"
