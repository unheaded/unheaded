#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# End-to-end check of scripts/db-backup.sh and scripts/well/restore-drill.sh,
# never touching the real Well: a throwaway source server is bootstrapped
# exactly as compose does (db/init.sh + db/migrations), given some rows, backed
# up into a temporary directory, and the set is restored by the drill. Then
# the drill must refuse three broken sets: a flipped byte in a dump, a row
# count that disagrees (with SHA256SUMS recomputed to match), and an
# unlisted dump. Retention is checked by taking more sets than --keep.
# The bootstrap also checks that db/init.sh sets role passwords from the
# environment (one with a quote in it) and leaves unset ones unable to log in.
#
#   scripts/well/test-backup-restore.sh [image]   # default postgres:16-alpine
set -euo pipefail

IMAGE="${1:-postgres:16-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="well-backup-test-$$"
TMP="$(mktemp -d)"
cleanup() { "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true; rm -rf -- "$TMP"; }
trap cleanup EXIT

KANBAN_PW="it's-$RANDOM\$x" HUGINN_PW="h-$RANDOM$RANDOM"
"${DOCKER[@]}" run -d --rm --name "$N" --network none \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="throwaway-$RANDOM" -e POSTGRES_DB=unheaded \
	-e APP_KANBAN_PASSWORD="$KANBAN_PW" -e HUGINN_READER_PASSWORD="$HUGINN_PW" \
	-v "$R/db/migrations:/migrations:ro" -v "$R/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 90); do
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { "${DOCKER[@]}" logs "$N" | tail -20; echo "source did not become ready"; exit 1; }
"${DOCKER[@]}" exec -i "$N" psql -X -q -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app <<'SQL'
INSERT INTO kanban_tasks (id, title) SELECT 't' || g, 'task ' || g FROM generate_series(1, 25) g;
SQL
"${DOCKER[@]}" exec -i "$N" psql -X -q -v ON_ERROR_STOP=1 -U unheaded -d unheaded <<'SQL'
CREATE TABLE "odd name" (id int); INSERT INTO "odd name" SELECT generate_series(1, 7);
SQL

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

# The image trusts 127.0.0.1 (initdb's default), which would make every
# login below pass. Require scram on loopback in this throwaway server.
"${DOCKER[@]}" exec "$N" sh -c 'sed -i -E "s/^(host[[:space:]].*)trust$/\1scram-sha-256/" "$PGDATA/pg_hba.conf"'
"${DOCKER[@]}" exec "$N" psql -X -q -U unheaded -d postgres -c 'SELECT pg_reload_conf()' >/dev/null
login() { # role password db -> exit status of a password login over TCP
	"${DOCKER[@]}" exec -e PGPASSWORD="$2" "$N" psql -X -q -h 127.0.0.1 -U "$1" -d "$3" -c 'SELECT 1' >/dev/null 2>&1
}
if login app_kanban "$KANBAN_PW" unheaded_app; then pass "app_kanban logs in with its env password"; else fail "app_kanban login"; fi
if login huginn_reader "$HUGINN_PW" unheaded_ops; then pass "huginn_reader logs in with its env password"; else fail "huginn_reader login"; fi
if login app_kanban kanban_dev unheaded_app; then fail "app_kanban accepts the old repo password"; else pass "old repo password refused"; fi
if login ops_reader ops_reader_dev unheaded_ops || login ops_reader "" unheaded_ops; then
	fail "ops_reader (variable unset) can log in"
else pass "ops_reader (variable unset) cannot log in"; fi
if "${DOCKER[@]}" logs "$N" 2>&1 | grep -qF "$KANBAN_PW"; then fail "a password appears in the init log"; else pass "no password in the init log"; fi
export WELL_CONTAINER="$N" WELL_BACKUP_DIR="$TMP/backups"

"$R/scripts/db-backup.sh" --keep 2
SET="$(find "$TMP/backups" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' | sort | tail -1)"
[[ -n "$SET" ]] && pass "backup produced a set" || { fail "no set"; exit 1; }
[[ "$(stat -c %a "$SET/globals.sql")" == 600 ]] && pass "globals.sql is 0600" || fail "globals.sql mode $(stat -c %a "$SET/globals.sql")"
for db in unheaded unheaded_app unheaded_ops unheaded_config postgres; do
	[[ -f "$SET/$db.dump" ]] && pass "dumped $db" || fail "no dump of $db"
done
grep -q $'^unheaded_app\tpublic.kanban_tasks\t25$' "$SET/rowcounts.tsv" && pass "kanban_tasks counted 25" || fail "kanban_tasks count"
grep -q $'^unheaded\tpublic."odd name"\t7$' "$SET/rowcounts.tsv" && pass "quoted table counted" || fail "quoted table count"
! find "$TMP/backups" -name '.partial-*' | grep -q . && pass "no partial set left" || fail "partial set left behind"

if "$R/scripts/well/restore-drill.sh" --image "$IMAGE" "$SET"; then pass "drill restores the set"; else fail "drill on a good set"; fi

broken() { # label, expected reason, then a command run inside a copy of the set
	local label=$1 want=$2 out; shift 2
	rm -rf "$TMP/bad"; cp -a "$SET" "$TMP/bad"
	(cd "$TMP/bad" && "$@")
	if out="$("$R/scripts/well/restore-drill.sh" --image "$IMAGE" "$TMP/bad" 2>&1)"; then
		fail "drill accepted $label"
	elif [[ "$out" == *"$want"* ]]; then pass "drill refused $label ($want)"
	else fail "drill refused $label, but not with '$want': $(tail -1 <<<"$out")"; fi
}
broken "a flipped byte in a dump" "checksum mismatch" sh -c 'printf "\377" | dd of=unheaded_app.dump bs=1 seek=200 conv=notrunc status=none'
broken "a wrong row count" "row counts differ" sh -c "sed -i 's/\tpublic.kanban_tasks\t25\$/\tpublic.kanban_tasks\t24/' rowcounts.tsv && sha256sum cluster.env globals.sql ./*.dump rowcounts.tsv | sed 's# \./# #' >SHA256SUMS"
broken "an unlisted dump" "not in SHA256SUMS" cp unheaded.dump extra.dump

# Retention: two more sets with --keep 2 leave exactly two.
sleep 1; "$R/scripts/db-backup.sh" --keep 2 >/dev/null
sleep 1; "$R/scripts/db-backup.sh" --keep 2 >/dev/null
n=$(find "$TMP/backups" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' | wc -l)
((n == 2)) && pass "retention keeps 2" || fail "retention left $n sets"
[[ ! -d "$SET" ]] && pass "oldest set removed" || fail "oldest set still present"

if WELL_BACKUP_DIR="$R/tmp-backup-test" "$R/scripts/db-backup.sh" >/dev/null 2>&1; then
	fail "backup wrote inside the repo"
else pass "backup refuses a directory inside the repo"; fi
rmdir "$R/tmp-backup-test" 2>/dev/null || true

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
