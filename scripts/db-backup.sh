#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# The Well — Automated backup
set -euo pipefail

BACKUP_DIR="/mnt/hdd/backups/postgres"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/unheaded_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

echo "[well] Starting backup..."
sudo docker exec unheaded-postgres pg_dump -U unheaded unheaded | gzip > "$BACKUP_FILE"

# Keep last 30 backups
ls -t "${BACKUP_DIR}"/unheaded_*.sql.gz | tail -n +31 | xargs -r rm

echo "[well] Backup complete: $BACKUP_FILE ($(du -sh "$BACKUP_FILE" | cut -f1))"
echo "[well] Backups retained: $(ls "${BACKUP_DIR}"/unheaded_*.sql.gz | wc -l)"
