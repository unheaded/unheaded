#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# victoria-drill.sh [set-name]: serve one VictoriaMetrics backup set from a
# throwaway container and prove it (ADR-147 decision 6, S-SEE-AND-SURVIVE
# Phase 9). Default set: the one LATEST names.
#
#   1. Every file matches <name>.SHA256SUMS ("set damaged" otherwise).
#   2. <name>.meta.env is read through lib-disks.sh's strict parser.
#   3. The set is copied into an empty scratch directory, and a container of
#      the image that made it serves it on 127.0.0.1:VM_DRILL_PORT, read-only,
#      1 GiB, no scrape config (it fetches nothing), retention 100 years.
#   4. count({__name__!=""}) at CUTOFF must equal SERIES.
# A trap removes the container and the scratch directory on every exit. On
# success, store_drill_victoria.prom in STORE_METRICS_DIR.
#
# Environment: VM_BACKUP_DIR (default /srv/backups/unheaded-victoria),
# VM_DRILL_SCRATCH (default /srv/backups/.drill-victoria; absent or empty),
# VM_UID (default 16703), VM_DRILL_PORT (default 26428), VM_DRILL_WAIT
# (seconds, default 60), STORE_METRICS_DIR.
set -euo pipefail
umask 077

ROOT="${VM_BACKUP_DIR:-/srv/backups/unheaded-victoria}"
SCRATCH="${VM_DRILL_SCRATCH:-/srv/backups/.drill-victoria}"
VM_UID="${VM_UID:-16703}"
PORT="${VM_DRILL_PORT:-26428}"
WAIT="${VM_DRILL_WAIT:-60}"
TAG=victoria-drill
# shellcheck source=scripts/backup/lib-disks.sh
. "$(dirname "$0")/lib-disks.sh"

[[ "$VM_UID" =~ ^[0-9]+$ && "$PORT" =~ ^[0-9]+$ && "$WAIT" =~ ^[0-9]+$ ]] || { echo "[$TAG] VM_UID, VM_DRILL_PORT and VM_DRILL_WAIT are numbers" >&2; exit 2; }
command -v jq >/dev/null || { echo "[$TAG] jq is required" >&2; exit 2; }
NAME="${1:-$(cat "$ROOT/LATEST" 2>/dev/null || true)}"
[[ "$NAME" =~ ^vm-[0-9]{8}T[0-9]{6}Z$ ]] || { echo "[$TAG] no set to drill (name '$NAME')" >&2; exit 1; }
[[ -d "$ROOT/$NAME" && -f "$ROOT/$NAME.SHA256SUMS" && -f "$ROOT/$NAME.meta.env" ]] || { echo "[$TAG] $NAME is incomplete" >&2; exit 1; }
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
START=$SECONDS

(cd "$ROOT/$NAME" && sha256sum -c --quiet --strict "../$NAME.SHA256SUMS" >/dev/null 2>&1) ||
	{ echo "[$TAG] set damaged: $NAME does not match its SHA256SUMS" >&2; exit 1; }
read_meta "$ROOT/$NAME.meta.env"
for k in NAME CUTOFF SERIES IMAGE; do
	v="META_$k"
	[[ -n "${!v:-}" ]] || { echo "[$TAG] $NAME.meta.env has no $k" >&2; exit 1; }
done
[[ "$META_NAME" == "$NAME" ]] || { echo "[$TAG] meta names $META_NAME, not $NAME" >&2; exit 1; }
[[ "$META_CUTOFF" =~ ^[0-9]+$ && "$META_SERIES" =~ ^[0-9]+$ ]] || { echo "[$TAG] bad numbers in meta" >&2; exit 1; }

if [[ -e "$SCRATCH" ]]; then
	[[ -d "$SCRATCH" && -z "$(ls -A "$SCRATCH")" ]] || { echo "[$TAG] $SCRATCH exists and is not empty: a drill left it (Appendix A 5)" >&2; exit 1; }
else
	mkdir -m 0700 "$SCRATCH"
fi
N="vm-drill-$$"
cleanup() {
	"${DOCKER[@]}" rm -f "$N" >/dev/null 2>&1 || true
	rm -rf -- "$SCRATCH"
}
trap cleanup EXIT
cp -a -- "$ROOT/$NAME/." "$SCRATCH/"
[[ "$(id -u)" -eq 0 ]] && chown -R "$VM_UID:$VM_UID" "$SCRATCH"

"${DOCKER[@]}" run -d --name "$N" --user "$VM_UID:$VM_UID" --read-only --memory 1g --tmpfs /tmp:size=64m \
	-p "127.0.0.1:$PORT:8428" -v "$SCRATCH:/victoria-data" \
	"$META_IMAGE" -storageDataPath=/victoria-data -retentionPeriod=100y -httpListenAddr=:8428 >/dev/null
up=0
for _ in $(seq 1 "$WAIT"); do
	if curl -fsS -m 2 "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then up=1; break; fi
	sleep 1
done
((up)) || { "${DOCKER[@]}" logs --tail 20 "$N" >&2 || true; echo "[$TAG] the drill server did not answer within ${WAIT}s" >&2; exit 1; }

GOT="$(curl -fsS -m 60 -G "http://127.0.0.1:$PORT/api/v1/query" --data-urlencode 'query=count({__name__!=""})' --data-urlencode "time=$META_CUTOFF" |
	jq -r '.data.result[0].value[1] // empty')"
[[ "$GOT" == "$META_SERIES" ]] || { echo "[$TAG] the restored copy has ${GOT:-no} series at $META_CUTOFF, the set says $META_SERIES" >&2; exit 1; }

if [[ -n "${STORE_METRICS_DIR:-}" ]]; then
	write_prom "$STORE_METRICS_DIR" store_drill_victoria.prom \
		"# HELP unheaded_store_drill_last_success_timestamp_seconds Last restore drill of the store that passed (scripts/backup)." \
		"# TYPE unheaded_store_drill_last_success_timestamp_seconds gauge" \
		"unheaded_store_drill_last_success_timestamp_seconds{store=\"victoria\"} $(date +%s)" \
		"unheaded_store_drill_duration_seconds{store=\"victoria\"} $((SECONDS - START))"
fi
echo "[$TAG] drill passed: $NAME served, $GOT series at $META_CUTOFF match"
