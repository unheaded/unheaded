#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Copies the newest backup set of The Well to east (ERDA 1.2), over SFTP
# as east's well-backup-copy account: nologin, key only, chrooted to
# /var/backups/unheaded-well-west, and unable to delete or rename anything
# there (deploy/east/sshd-well-offsite.conf). So a compromised west can add
# sets but never remove the copies; east prunes its own (offsite-check.sh).
#
# Run by unheaded-well-offsite.service after every successful backup. The
# set is verified against its SHA256SUMS before it leaves; on success the
# set name and time go to $WELL_BACKUP_DIR/OFFSITE and the timestamp to the
# metrics file (ERDA 1.4).
#
#   WELL_OFFSITE_DEST=well-backup-copy@192.168.13.1 scripts/well/offsite-copy.sh
set -euo pipefail
DIR="${WELL_BACKUP_DIR:-/srv/backups/unheaded-well}"
DEST="${WELL_OFFSITE_DEST:?set WELL_OFFSITE_DEST (user@host)}"
KEYDIR="${WELL_OFFSITE_KEYDIR:-/etc/unheaded/well-offsite}"
METRICS="${WELL_METRICS_DIR:-}"

SET="$(cat "$DIR/LATEST")"
[[ "$SET" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}/well-[0-9]{6}Z$ ]] || { echo "[offsite] LATEST names no set: $SET" >&2; exit 1; }
(cd "$DIR/$SET" && sha256sum --quiet --strict -c SHA256SUMS) || { echo "[offsite] $SET fails its SHA256SUMS; not copied" >&2; exit 1; }
day="${SET%%/*}"

# '-' before mkdir: an existing day directory is not an error.
sftp -q -b - -i "$KEYDIR/id_ed25519" \
	-o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
	-o UserKnownHostsFile="$KEYDIR/known_hosts" "$DEST" <<EOF
-mkdir /sets/$day
put -r $DIR/$SET /sets/$day/
put $DIR/LATEST /sets/LATEST
EOF

now="$(date +%s)"
printf 'set=%s\ntime=%s\n' "$SET" "$now" >"$DIR/.OFFSITE.tmp"
chmod 0644 "$DIR/.OFFSITE.tmp"
mv -f "$DIR/.OFFSITE.tmp" "$DIR/OFFSITE"
if [[ -n "$METRICS" ]]; then
	printf '# HELP unheaded_well_offsite_last_success_timestamp_seconds Last backup set copied to east.\n# TYPE unheaded_well_offsite_last_success_timestamp_seconds gauge\nunheaded_well_offsite_last_success_timestamp_seconds %s\n' "$now" >"$METRICS/.well_offsite.prom.tmp"
	chmod 0644 "$METRICS/.well_offsite.prom.tmp"
	mv -f "$METRICS/.well_offsite.prom.tmp" "$METRICS/well_offsite.prom"
fi
echo "[offsite] $SET -> $DEST"
