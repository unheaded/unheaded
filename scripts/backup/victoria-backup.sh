#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# victoria-backup.sh: one backup set of VictoriaMetrics (ADR-147 decision 6,
# S-SEE-AND-SURVIVE Phase 9). VictoriaMetrics holds the SLO window and every
# host series and had no copy (Erda F15).
#
# VictoriaMetrics makes an instant snapshot (hard links) through its
# /snapshot/create API; the snapshot is copied, links followed, into
# <root>/vm-<UTC>/, then deleted. Beside each set:
#   <name>.SHA256SUMS   over every file of the set
#   <name>.meta.env     NAME, CUTOFF, SERIES, SNAPSHOT, IMAGE, BYTES
#   LATEST              the newest set's name (0644)
# SERIES is count({__name__!=""}) at CUTOFF (two minutes before the
# snapshot); victoria-drill.sh serves the set from a throwaway container and
# must answer the same. The newest VM_BACKUP_KEEP sets are kept.
#
# Root: the data directory and the admin key are in the container's volumes.
# The key (volume victoria-auth, docker-compose.yml) goes to curl on stdin
# (--config -), never into argv, and is never printed.
#
# Environment:
#   VM_CONTAINER          default unheaded-victoria
#   VM_URL                default http://127.0.0.1:8428
#   VM_BACKUP_DIR         set root (default /srv/backups/unheaded-victoria)
#   VM_BACKUP_KEEP        sets kept (default 14)
#   VM_BACKUP_OTHER_DISK  1: refuse when the sets share a disk with the data (the unit sets it)
#   STORE_METRICS_DIR     write store_backup_victoria.prom there on success
set -euo pipefail
umask 077

C="${VM_CONTAINER:-unheaded-victoria}"
URL="${VM_URL:-http://127.0.0.1:8428}"
ROOT="${VM_BACKUP_DIR:-/srv/backups/unheaded-victoria}"
KEEP="${VM_BACKUP_KEEP:-14}"
TAG=victoria-backup
# shellcheck source=scripts/backup/lib-disks.sh
. "$(dirname "$0")/lib-disks.sh"

[[ "$KEEP" =~ ^[1-9][0-9]*$ ]] || { echo "[$TAG] VM_BACKUP_KEEP must be a positive integer" >&2; exit 2; }
[[ "$C" =~ ^[A-Za-z0-9_.-]+$ ]] || { echo "[$TAG] bad container name" >&2; exit 2; }
[[ "$URL" =~ ^http://(127\.0\.0\.1|localhost|\[::1\]):[0-9]+$ ]] || { echo "[$TAG] VM_URL must be loopback http (the key goes over it)" >&2; exit 2; }
command -v jq >/dev/null || { echo "[$TAG] jq is required" >&2; exit 2; }
[[ -d "$ROOT" ]] || { echo "[$TAG] $ROOT does not exist (install -d -m 0700 it)" >&2; exit 1; }
ROOT="$(cd "$ROOT" && pwd -P)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
START=$SECONDS

mount_src() { "${DOCKER[@]}" inspect "$C" --format "{{range .Mounts}}{{if eq .Destination \"$1\"}}{{.Source}}{{end}}{{end}}"; }
DATA="$(mount_src /victoria-data)" || { echo "[$TAG] cannot inspect $C" >&2; exit 1; }
AUTH="$(mount_src /victoria-auth)"
[[ -n "$DATA" && -d "$DATA" ]] || { echo "[$TAG] cannot find $C's data directory on the host" >&2; exit 1; }
KEY="$AUTH/admin.key"
[[ -n "$AUTH" && -r "$KEY" ]] || { echo "[$TAG] cannot read the admin key in $C's /victoria-auth volume" >&2; exit 1; }
[[ "$(<"$KEY")" =~ ^[A-Za-z0-9._-]+$ ]] || { echo "[$TAG] the admin key holds characters a curl config cannot carry" >&2; exit 1; }
if [[ "${VM_BACKUP_OTHER_DISK:-0}" == 1 ]]; then
	refuse_shared_disk "$DATA" "$ROOT" "$TAG"
fi

# vmcurl <curl args>: the admin key as authKey, from stdin, never in argv.
vmcurl() { printf 'data-urlencode = "authKey=%s"\n' "$(<"$KEY")" | curl -fsS -m 300 --config - "$@"; }

CUTOFF=$(($(date +%s) - 120))
SERIES="$(curl -fsS -m 60 -G "$URL/api/v1/query" --data-urlencode 'query=count({__name__!=""})' --data-urlencode "time=$CUTOFF" |
	jq -r '.data.result[0].value[1] // empty')" || { echo "[$TAG] cannot count series" >&2; exit 1; }
[[ "$SERIES" =~ ^[0-9]+$ ]] || { echo "[$TAG] unexpected series count: '$SERIES'" >&2; exit 1; }

SNAP="$(vmcurl -G "$URL/snapshot/create" | jq -r 'select(.status == "ok") | .snapshot // empty')" || true
[[ "$SNAP" =~ ^[A-Za-z0-9_-]+$ ]] || { echo "[$TAG] snapshot/create did not answer ok" >&2; exit 1; }
NAME="vm-$(date -u +%Y%m%dT%H%M%SZ)"
cleanup() {
	vmcurl -G "$URL/snapshot/delete" --data-urlencode "snapshot=$SNAP" >/dev/null 2>&1 ||
		echo "[$TAG] snapshot $SNAP not deleted: Appendix A 5 of the plan deletes it by hand" >&2
	[[ -n "${PART:-}" && -d "$PART" ]] && rm -rf -- "$PART"
	return 0
}
trap cleanup EXIT
[[ -d "$DATA/snapshots/$SNAP" ]] || { echo "[$TAG] snapshot $SNAP is not under $DATA/snapshots" >&2; exit 1; }
[[ ! -e "$ROOT/$NAME" ]] || { echo "[$TAG] $ROOT/$NAME already exists" >&2; exit 1; }

# Copied under a .partial name, renamed when complete: a killed run leaves
# nothing LATEST or the drill could take for a set.
PART="$ROOT/.partial-$NAME"
mkdir -m 0700 "$PART"
cp -rL -- "$DATA/snapshots/$SNAP/." "$PART/"
mv -- "$PART" "$ROOT/$NAME"
PART=""

(cd "$ROOT/$NAME" && find . -type f -print0 | sort -z | xargs -0 sha256sum) >"$ROOT/$NAME.SHA256SUMS"
IMAGE="$("${DOCKER[@]}" inspect -f '{{.Config.Image}}' "$C")"
BYTES="$(du -sb "$ROOT/$NAME" | cut -f1)"
printf 'NAME=%s\nCUTOFF=%s\nSERIES=%s\nSNAPSHOT=%s\nIMAGE=%s\nBYTES=%s\n' \
	"$NAME" "$CUTOFF" "$SERIES" "$SNAP" "$IMAGE" "$BYTES" >"$ROOT/$NAME.meta.env"
printf '%s\n' "$NAME" >"$ROOT/.LATEST.tmp" && chmod 0644 "$ROOT/.LATEST.tmp" && mv -f "$ROOT/.LATEST.tmp" "$ROOT/LATEST"

mapfile -t SETS < <(find "$ROOT" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | grep -E '^vm-[0-9]{8}T[0-9]{6}Z$' | sort -r)
for old in "${SETS[@]:KEEP}"; do
	rm -rf -- "${ROOT:?}/$old" "$ROOT/$old.SHA256SUMS" "$ROOT/$old.meta.env"
	echo "[$TAG] retention removed $old"
done

if [[ -n "${STORE_METRICS_DIR:-}" ]]; then
	write_prom "$STORE_METRICS_DIR" store_backup_victoria.prom \
		"# HELP unheaded_store_backup_last_success_timestamp_seconds Last complete backup set of the store (scripts/backup)." \
		"# TYPE unheaded_store_backup_last_success_timestamp_seconds gauge" \
		"unheaded_store_backup_last_success_timestamp_seconds{store=\"victoria\"} $(date +%s)" \
		"unheaded_store_backup_bytes{store=\"victoria\"} $BYTES" \
		"unheaded_store_backup_duration_seconds{store=\"victoria\"} $((SECONDS - START))"
fi
echo "[$TAG] backup complete: $ROOT/$NAME ($BYTES bytes, $SERIES series at $CUTOFF); sets retained: $(( ${#SETS[@]} < KEEP ? ${#SETS[@]} : KEEP ))"
