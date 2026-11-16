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
# environment (one with a quote in it) and leaves unset ones unable to log in,
# and that neither it nor scripts/well/set-role-passwords.sh leaves a
# password in the server log or pg_stat_statements: the source logs DDL and
# tracks statements as db/postgresql.conf does.
# Zhen's database (ADR-149) has its own set: the default set leaves
# unheaded_zhen out; WELL_BACKUP_DATABASES=unheaded_zhen WELL_BACKUP_SET=zhen
# makes a set of it alone (zhen_backup.prom) that the drill restores; a
# database that does not exist exits 1, a bad set name exits 2.
#
#   scripts/well/test-backup-restore.sh [image]   # default postgres:16.15-alpine
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="well-backup-test-$$"
TMP="$(mktemp -d)"
cleanup() { "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true; rm -rf -- "$TMP"; }
trap cleanup EXIT

KANBAN_PW="it's-$RANDOM\$x" HUGINN_PW="h-$RANDOM$RANDOM"
"${DOCKER[@]}" run -d --rm --name "$N" --network none \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="throwaway-$RANDOM" -e POSTGRES_DB=postgres \
	-e APP_KANBAN_PASSWORD="$KANBAN_PW" -e HUGINN_READER_PASSWORD="$HUGINN_PW" \
	-v "$R/db/migrations:/migrations:ro" -v "$R/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" postgres -c log_statement=ddl -c shared_preload_libraries=pg_stat_statements -c pg_stat_statements.track=top >/dev/null
ready=0
for _ in $(seq 1 90); do
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { "${DOCKER[@]}" logs "$N" | tail -20; echo "source did not become ready"; exit 1; }
"${DOCKER[@]}" exec -i "$N" psql -X -q -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app <<'SQL'
INSERT INTO kanban_tasks (id, title) SELECT 't' || g, 'task ' || g FROM generate_series(1, 25) g;
SQL
# The migrations seed kanban_tasks too (034, the Genesis cards), so the count
# the backup must record is whatever the table holds now, not just our 25.
KANBAN_ROWS="$("${DOCKER[@]}" exec "$N" psql -X -qAt -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app -c 'SELECT count(*) FROM kanban_tasks')"
[[ "$KANBAN_ROWS" =~ ^[0-9]+$ ]] && ((KANBAN_ROWS >= 25)) || { echo "kanban_tasks count unreadable: $KANBAN_ROWS"; exit 1; }
"${DOCKER[@]}" exec -i "$N" psql -X -q -v ON_ERROR_STOP=1 -U unheaded -d postgres <<'SQL'
CREATE TABLE "odd name" (id int); INSERT INTO "odd name" SELECT generate_series(1, 7);
SQL
# A row in Zhen's database, which the default set must leave out.
"${DOCKER[@]}" exec -i "$N" psql -X -q -v ON_ERROR_STOP=1 -U unheaded -d unheaded_zhen <<'SQL'
INSERT INTO zhen_memories (question, answer) VALUES ('backup?', 'its own set');
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
# A logged literal doubles the quote: look for what follows the last one,
# which holds the random part.
in_log() { "${DOCKER[@]}" logs "$N" 2>&1 | grep -qF -- "${1##*\'}"; }
if in_log "$KANBAN_PW"; then fail "a password appears in the init log"; else pass "no password in the init log"; fi
# Rotation (and a restore's password step) goes through set-role-passwords.sh.
NEW_PW="rot'-$RANDOM$RANDOM"
# A docker shim first on PATH records every argv the script hands docker:
# /proc/<pid>/cmdline is world-readable, so the password must never be in one.
SHIM="$TMP/argv-shim"
mkdir -p "$SHIM"
REAL_DOCKER="$(command -v docker)"
printf '#!/bin/sh\nprintf "%%s\\n" "$*" >>"%s/argv.log"\nexec "%s" "$@"\n' "$SHIM" "$REAL_DOCKER" >"$SHIM/docker"
chmod 755 "$SHIM/docker"
env -u POSTGRES_PASSWORD PATH="$SHIM:$PATH" WELL_CONTAINER="$N" APP_KANBAN_PASSWORD="$NEW_PW" "$R/scripts/well/set-role-passwords.sh" >/dev/null
if login app_kanban "$NEW_PW" unheaded_app && ! login app_kanban "$KANBAN_PW" unheaded_app; then
	pass "set-role-passwords.sh rotates app_kanban"
else fail "set-role-passwords.sh rotation"; fi
if [[ ! -s "$SHIM/argv.log" ]]; then
	echo "  [skip] argv check: docker ran through sudo (secure_path bypasses the shim)"
elif grep -qF -- "${NEW_PW##*\'}" "$SHIM/argv.log"; then
	fail "set-role-passwords.sh put the password in docker's argv"
else pass "the rotated password is in no argv ($(wc -l <"$SHIM/argv.log") docker calls)"; fi
if in_log "$NEW_PW"; then fail "a rotated password appears in the server log"; else pass "no rotated password in the server log"; fi
leaked="$("${DOCKER[@]}" exec "$N" psql -X -At -v ON_ERROR_STOP=1 -U unheaded -d postgres \
	-c "SELECT count(*) FROM pg_stat_statements WHERE query ILIKE '%PASSWORD%'")"
[[ "$leaked" == 0 ]] && pass "no password statement in pg_stat_statements" || fail "pg_stat_statements holds $leaked password statement(s)"
export WELL_CONTAINER="$N" WELL_BACKUP_DIR="$TMP/backups" WELL_METRICS_DIR="$TMP/metrics"
mkdir -p "$WELL_METRICS_DIR"

"$R/scripts/db-backup.sh" --keep 2
SET="$(find "$TMP/backups" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' | sort | tail -1)"
[[ -n "$SET" ]] && pass "backup produced a set" || { fail "no set"; exit 1; }
[[ "$(stat -c %a "$SET/globals.sql")" == 600 ]] && pass "globals.sql is 0600" || fail "globals.sql mode $(stat -c %a "$SET/globals.sql")"
if grep -qiE "PASSWORD '(SCRAM|md5)" "$SET/globals.sql"; then fail "globals.sql holds password hashes"; else pass "globals.sql holds no password hashes"; fi
grep -q '^superuser=unheaded$' "$SET/cluster.env" && pass "cluster.env names the bootstrap superuser" || fail "cluster.env: $(cat "$SET/cluster.env")"
[[ "$(cat "$TMP/backups/LATEST")" == "${SET#"$TMP/backups/"}" && "$(stat -c %a "$TMP/backups/LATEST")" == 644 ]] &&
	pass "LATEST names the newest set (0644)" || fail "LATEST: $(cat "$TMP/backups/LATEST" 2>&1)"
if "${DOCKER[@]}" exec "$N" psql -X -q -U well_backup -d unheaded_app -c "INSERT INTO kanban_tasks(id,title) VALUES ('wb','x')" >/dev/null 2>&1; then
	fail "well_backup can write"
else pass "well_backup cannot write"; fi
for db in unheaded_app unheaded_ops unheaded_config postgres; do
	[[ -f "$SET/$db.dump" ]] && pass "dumped $db" || fail "no dump of $db"
done
grep -q $'^unheaded_app\tpublic.kanban_tasks\t'"$KANBAN_ROWS"'$' "$SET/rowcounts.tsv" && pass "kanban_tasks counted $KANBAN_ROWS" ||
	fail "kanban_tasks count: want $KANBAN_ROWS, rowcounts.tsv has $(grep kanban_tasks "$SET/rowcounts.tsv")"
grep -q $'^postgres\tpublic."odd name"\t7$' "$SET/rowcounts.tsv" && pass "quoted table counted" || fail "quoted table count"
! find "$TMP/backups" -name '.partial-*' | grep -q . && pass "no partial set left" || fail "partial set left behind"
# (a) The default set leaves Zhen's database out and writes the platform's metrics.
[[ ! -e "$SET/unheaded_zhen.dump" ]] && pass "default set has no unheaded_zhen.dump" || fail "default set dumped unheaded_zhen"
! cut -f1 "$SET/rowcounts.tsv" | grep -qx unheaded_zhen && pass "default rowcounts name no unheaded_zhen table" || fail "default rowcounts name unheaded_zhen"
grep -q '^unheaded_well_backup_last_success_timestamp_seconds ' "$WELL_METRICS_DIR/well_backup.prom" 2>/dev/null &&
	[[ ! -e "$WELL_METRICS_DIR/zhen_backup.prom" ]] && pass "default set writes well_backup.prom only" || fail "default set metrics: $(ls "$WELL_METRICS_DIR")"

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
broken "a wrong row count" "row counts differ" sh -c "sed -i 's/\tpublic.kanban_tasks\t$KANBAN_ROWS\$/\tpublic.kanban_tasks\t$((KANBAN_ROWS - 1))/' rowcounts.tsv && sha256sum cluster.env globals.sql ./*.dump rowcounts.tsv | sed 's# \./# #' >SHA256SUMS"
broken "an unlisted dump" "not in SHA256SUMS" cp postgres.dump extra.dump

# Retention: two more sets with --keep 2 leave exactly two.
sleep 1; "$R/scripts/db-backup.sh" --keep 2 >/dev/null
sleep 1; "$R/scripts/db-backup.sh" --keep 2 >/dev/null
n=$(find "$TMP/backups" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' | wc -l)
((n == 2)) && pass "retention keeps 2" || fail "retention left $n sets"
[[ ! -d "$SET" ]] && pass "oldest set removed" || fail "oldest set still present"

# (b) Zhen's own set: unheaded_zhen alone, its own metrics, restorable.
ZDIR="$TMP/zhen-backups"
if WELL_BACKUP_DIR="$ZDIR" WELL_BACKUP_DATABASES=unheaded_zhen WELL_BACKUP_EXCLUDE='' WELL_BACKUP_SET=zhen \
	"$R/scripts/db-backup.sh" >"$TMP/zhen.out" 2>&1; then pass "zhen set exits 0"; else fail "zhen set"; tail -4 "$TMP/zhen.out" | sed 's/^/    /'; fi
ZSET="$(find "$ZDIR" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' 2>/dev/null | sort | tail -1)"
got="$(cd "$ZSET" 2>/dev/null && ls -A | sort | tr '\n' ' ')"
[[ "$got" == "SHA256SUMS cluster.env globals.sql rowcounts.tsv unheaded_zhen.dump " ]] &&
	pass "zhen set holds unheaded_zhen.dump and the four set files, nothing else" || fail "zhen set files: $got"
grep -q $'^unheaded_zhen\tpublic.zhen_memories\t1$' "$ZSET/rowcounts.tsv" 2>/dev/null && pass "zhen set counted zhen_memories" ||
	fail "zhen set rowcounts: $(grep zhen_memories "$ZSET/rowcounts.tsv" 2>&1)"
grep -q '^unheaded_zhen_backup_last_success_timestamp_seconds ' "$WELL_METRICS_DIR/zhen_backup.prom" 2>/dev/null &&
	pass "zhen set writes zhen_backup.prom" || fail "zhen_backup.prom: $(cat "$WELL_METRICS_DIR/zhen_backup.prom" 2>&1 | head -3)"
if "$R/scripts/well/restore-drill.sh" --image "$IMAGE" "$ZSET"; then pass "drill restores the zhen set"; else fail "drill on the zhen set"; fi
# (c) A database that does not exist: exit 1 naming it, no set left.
set +e; WELL_BACKUP_DIR="$TMP/nosuch-backups" WELL_BACKUP_DATABASES=nosuch "$R/scripts/db-backup.sh" >"$TMP/nosuch.out" 2>&1; rc=$?; set -e
((rc == 1)) && grep -q nosuch "$TMP/nosuch.out" && pass "WELL_BACKUP_DATABASES=nosuch exits 1 naming it" || fail "nosuch: rc $rc $(tail -1 "$TMP/nosuch.out")"
! find "$TMP/nosuch-backups" -mindepth 2 -maxdepth 2 -name 'well-*Z' 2>/dev/null | grep -q . && pass "no set written for nosuch" || fail "a set was written for nosuch"
# (d) A set name that is not [a-z]{1,16}: exit 2.
set +e; WELL_BACKUP_DIR="$TMP/badset-backups" WELL_BACKUP_SET=../x "$R/scripts/db-backup.sh" >"$TMP/badset.out" 2>&1; rc=$?; set -e
((rc == 2)) && pass "WELL_BACKUP_SET=../x exits 2" || fail "WELL_BACKUP_SET=../x: rc $rc $(tail -1 "$TMP/badset.out")"

if WELL_BACKUP_DIR="$R/tmp-backup-test" "$R/scripts/db-backup.sh" >/dev/null 2>&1; then
	fail "backup wrote inside the repo"
else pass "backup refuses a directory inside the repo"; fi
rmdir "$R/tmp-backup-test" 2>/dev/null || true

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
