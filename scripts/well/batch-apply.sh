#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Apply every pending migration to The Well as one batch, behind a backup
# taken seconds before (ADR-101, ADR-106 Q1; S-WELL-SPRING Phase 1, M-070).
#
#   scripts/well/batch-apply.sh --dry-run [--from NNN]   # list what would run, change nothing
#   scripts/well/batch-apply.sh [--from NNN]             # back up, apply, show the ledger after
#
# Pending = db/migrations/MANIFEST minus each database's
# well_schema_migrations ledger, as `well-migrate status` computes it. An
# applied file that changed (drift) or an orphan ledger row stops the batch
# before anything runs. --from NNN refuses when any pending file is numbered
# below NNN: the live ledger disagreeing with the repo about older files is a
# reason to stop and look, never to apply (Step 46).
#
# Apply, in order:
#   1. writes <root>/pre-batch-<UTC>/: pending.tsv (database and file of every
#      pending entry), status-before.txt;
#   2. takes a full backup set into that directory with scripts/db-backup.sh
#      (globals, every database, row counts, SHA256SUMS: the layout
#      scripts/well/restore-drill.sh and batch-rollback.sh restore), as the
#      read-only well_backup role inside the Well's container;
#   3. runs well-migrate apply -yes (lock_timeout 5s, statement_timeout 5m per
#      migration; it stops at the first failure, which records nothing);
#   4. prints the ledger rows the batch added and checks status is clean.
# On a failure after step 2 it prints the rollback command (Appendix A1:
# do not re-run; roll back, fix the file, rehearse again).
#
# Environment:
#   WELL_MIGRATE_PASSWORD  required (the superuser's; .env, never the repo)
#   WELL_HOST, WELL_PORT, WELL_SSLMODE, WELL_MIGRATE_USER  as well-migrate
#   WELL_CONTAINER         container running The Well (default unheaded-postgres)
#   WELL_BATCH_BACKUP_ROOT where pre-batch-* directories go
#                          (default /srv/backups/unheaded-well)
#   WELL_MIGRATIONS_DIR    migrations directory (default db/migrations)
#   WELL_MIGRATE_BIN       a built well-migrate (default: go build into a temp dir)
#
# Exit: 0 done (or nothing pending), 1 refused or failed, 2 usage.
set -euo pipefail
umask 077

R="$(cd "$(dirname "$0")/../.." && pwd)"
DRY=0 FROM=""
while (($#)); do
	case "$1" in
	--dry-run) DRY=1; shift ;;
	--from) FROM="${2:?--from needs a migration number}"; shift 2 ;;
	*) echo "usage: $0 [--dry-run] [--from NNN]" >&2; exit 2 ;;
	esac
done
[[ -z "$FROM" || "$FROM" =~ ^[0-9]{3}$ ]] || { echo "[batch] --from takes three digits, got '$FROM'" >&2; exit 2; }
: "${WELL_MIGRATE_PASSWORD:?set WELL_MIGRATE_PASSWORD (from .env): credentials are not stored in this repo}"

DIR="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
[[ -f "$DIR/MANIFEST" ]] || { echo "[batch] no MANIFEST in $DIR" >&2; exit 2; }
TMP="$(mktemp -d)"
trap 'rm -rf -- "$TMP"' EXIT
WM="${WELL_MIGRATE_BIN:-}"
if [[ -z "$WM" ]]; then
	(cd "$R" && go build -o "$TMP/well-migrate" ./cmd/well-migrate)
	WM="$TMP/well-migrate"
fi
[[ -x "$WM" ]] || { echo "[batch] well-migrate not executable: $WM" >&2; exit 2; }
wm() { "$WM" -dir "$DIR" -lock-timeout 5s -statement-timeout 5m "$@"; }

# Pending, and drift, before anything changes.
rc=0
wm status >"$TMP/before.txt" 2>&1 || rc=$?
case $rc in
0) echo "[batch] nothing pending: the ledger matches the manifest"; exit 0 ;;
3) ;;
*) cat "$TMP/before.txt" >&2; echo "[batch] status failed (rc $rc): drift, an orphan or no connection; nothing applied" >&2; exit 1 ;;
esac
awk '$1 == "pending" {print $2 "\t" $3}' "$TMP/before.txt" >"$TMP/pending.tsv"
[[ -s "$TMP/pending.tsv" ]] || { cat "$TMP/before.txt" >&2; echo "[batch] status said pending but listed nothing" >&2; exit 1; }
echo "[batch] pending ($(wc -l <"$TMP/pending.tsv")):"
sed 's/^/  /' "$TMP/pending.tsv"
if [[ -n "$FROM" ]]; then
	older="$(awk -F'\t' -v from="$FROM" 'substr($2, 1, 3) + 0 < from + 0' "$TMP/pending.tsv")"
	if [[ -n "$older" ]]; then
		echo "[batch] refusing: pending file(s) numbered below $FROM, so the ledger disagrees with the repo:" >&2
		sed 's/^/  /' <<<"$older" >&2
		exit 1
	fi
fi
if ((DRY)); then
	echo "[batch] dry run: nothing backed up, nothing applied"
	exit 0
fi

# 1-2. Pre-batch directory and backup.
ROOT="${WELL_BATCH_BACKUP_ROOT:-/srv/backups/unheaded-well}"
PRE="$ROOT/pre-batch-$(date -u +%Y%m%dT%H%M%SZ)"
[[ ! -e "$PRE" ]] || { echo "[batch] $PRE already exists" >&2; exit 1; }
mkdir -p "$PRE"
chmod 0700 "$PRE"
cp "$TMP/pending.tsv" "$PRE/pending.tsv"
cp "$TMP/before.txt" "$PRE/status-before.txt"
echo "[batch] backing up into $PRE"
# Its own set, kept apart from the nightly ones; no metrics file, so the
# nightly backup's freshness metric stays the nightly's.
env -u WELL_METRICS_DIR WELL_BACKUP_DIR="$PRE" bash "$R/scripts/db-backup.sh" --keep 1 >"$TMP/backup.log" 2>&1 ||
	{ cat "$TMP/backup.log" >&2; echo "[batch] backup failed: nothing applied" >&2; exit 1; }
tail -1 "$TMP/backup.log"
mapfile -t SETS < <(find "$PRE" -mindepth 2 -maxdepth 2 -type d -name 'well-[0-9]*Z')
((${#SETS[@]} == 1)) || { echo "[batch] expected one backup set under $PRE, found ${#SETS[@]}: nothing applied" >&2; exit 1; }
cut -f1 "$PRE/pending.tsv" | sort -u | while read -r db; do
	[[ -f "${SETS[0]}/$db.dump" ]] || { echo "[batch] the set has no dump of $db: nothing applied" >&2; exit 1; }
done

# 3. Apply.
echo "[batch] applying"
if ! wm apply -yes; then
	echo "[batch] FAILED part-way. Do not re-run. Roll back (Appendix A1):" >&2
	echo "  bash scripts/well/batch-rollback.sh --yes $PRE" >&2
	exit 1
fi

# 4. What the batch recorded.
rc=0
wm status >"$TMP/after.txt" 2>&1 || rc=$?
cp "$TMP/after.txt" "$PRE/status-after.txt"
echo "[batch] ledger rows added by this batch:"
while IFS=$'\t' read -r db f; do
	awk -v db="$db" -v f="$f" '$2 == db && $3 == f {print "  " $0}' "$TMP/after.txt"
done <"$PRE/pending.tsv"
if ((rc != 0)); then
	cat "$TMP/after.txt" >&2
	echo "[batch] status after the batch is not clean (rc $rc). Roll back: bash scripts/well/batch-rollback.sh --yes $PRE" >&2
	exit 1
fi
echo "[batch] done; status clean. Pre-batch backup: $PRE"
