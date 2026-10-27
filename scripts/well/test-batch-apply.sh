#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Rehearsal of scripts/well/batch-apply.sh and batch-rollback.sh on a
# throwaway server, never the real Well (S-WELL-SPRING Phase 1, M-070):
#   1. a Well bootstrapped through 029 (the live ledger's last file);
#   2. batch-apply refuses without WELL_MIGRATE_PASSWORD; --dry-run lists
#      exactly the manifest's entries after 029 and changes nothing;
#      --from refuses when an older file is pending;
#   3. Appendix A1: a batch that fails part-way (a broken file after the
#      real ones) leaves files applied; batch-rollback without --yes changes
#      nothing, with --yes puts every touched database (postgres included)
#      back at 029, drops the role the batch created and keeps the rows
#      written before the backup; a corrupt set or a stray directory is
#      refused;
#   4. the real batch applies, status is clean, every file is in its ledger;
#   5. rollback of that batch returns every ledger to 029; apply again
#      works; a third run finds nothing pending.
#
#   timeout 900 scripts/well/test-batch-apply.sh [image]   # default postgres:16.15-alpine
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(timeout 120 docker); timeout 10 docker info >/dev/null 2>&1 || DOCKER=(timeout 120 sudo -n docker)
N="well-batch-test-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
cleanup() { "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true; rm -rf -- "$TMP"; }
trap cleanup EXIT

# 1. A Well at 029: the init copy routes nothing after 029.
mkdir -p "$TMP/init"
cp -a "$R/db/migrations" "$TMP/init/migrations"
cp "$R/db/init.sh" "$TMP/init/init.sh"
awk '{ if ($2 ~ /^[0-9][0-9][0-9]_/ && substr($2, 1, 3) + 0 > 29) next; print }' \
	"$R/db/migrations/MANIFEST" >"$TMP/init/migrations/MANIFEST"
for f in "$TMP"/init/migrations/[0-9][0-9][0-9]_*.sql; do
	b="$(basename "$f")"
	((10#${b:0:3} > 29)) && rm -- "$f"
done
"${DOCKER[@]}" run -d --rm --name "$N" -p 127.0.0.1::5432 \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-v "$TMP/init/migrations:/migrations:ro" -v "$TMP/init/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 90); do
	# pg_isready answers during init's temporary server too: wait for the
	# real one (init.sh's last step creates unheaded_config's ledger).
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null &&
		"${DOCKER[@]}" exec "$N" psql -X -qAt -h 127.0.0.1 -U unheaded -d unheaded_config -c "SELECT count(*) FROM well_schema_migrations" >/dev/null 2>&1; then
		ready=1; break
	fi
	sleep 1
done
((ready)) || { "${DOCKER[@]}" logs "$N" | tail -20; echo "throwaway Well did not become ready"; exit 1; }
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"
(cd "$R" && go build -o "$TMP/well-migrate" ./cmd/well-migrate)

export WELL_CONTAINER="$N" WELL_HOST=127.0.0.1 WELL_PORT="$PORT" WELL_SSLMODE=disable \
	WELL_BATCH_BACKUP_ROOT="$TMP/backups" WELL_MIGRATE_BIN="$TMP/well-migrate"
sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2" </dev/null; }
DBS=(postgres unheaded_app unheaded_ops unheaded_config)
ledger_max() { local db out=""; for db in "${DBS[@]}"; do out+="$db:$(sq "$db" "SELECT max(filename) FROM well_schema_migrations") "; done; echo "$out"; }
AT029="$(ledger_max)"

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
run() { # want-rc label cmd... (output in $TMP/out)
	local want=$1 label=$2 rc=0; shift 2
	"$@" >"$TMP/out" 2>&1 || rc=$?
	if ((rc == want)); then pass "$label (rc $rc)"; else fail "$label: rc $rc, want $want"; sed 's/^/    /' "$TMP/out" | tail -12; fi
}
batch() { WELL_MIGRATE_PASSWORD="$PW" timeout 600 bash "$R/scripts/well/batch-apply.sh" "$@"; }
rollback() { timeout 600 bash "$R/scripts/well/batch-rollback.sh" "$@"; }

[[ "$AT029" == *"postgres:029_"* && "$AT029" == *"unheaded_app:0"* && "$AT029" != *":03"* ]] &&
	pass "throwaway Well stops at 029 ($AT029)" || fail "bootstrap ledger: $AT029"
EXPECT="$(awk '$1 != "skip" && $2 ~ /^[0-9][0-9][0-9]_/ && substr($2, 1, 3) + 0 > 29 {print $1 "\t" $2}' "$R/db/migrations/MANIFEST")"
[[ -n "$EXPECT" ]] || { echo "nothing after 029 in the MANIFEST: nothing to rehearse"; exit 1; }

# 2. Guards and the dry run.
rc=0; bash "$R/scripts/well/batch-apply.sh" --dry-run >"$TMP/out" 2>&1 || rc=$?
((rc != 0)) && grep -q WELL_MIGRATE_PASSWORD "$TMP/out" && pass "refuses without WELL_MIGRATE_PASSWORD" || fail "ran without a password (rc $rc)"
run 0 "dry run" batch --dry-run
got="$(sed -n 's/^  \([a-z_]*\)\t\(.*\)$/\1\t\2/p' "$TMP/out")"
[[ "$got" == "$EXPECT" ]] && pass "dry run lists exactly the files after 029 ($(wc -l <<<"$EXPECT"))" ||
	fail "dry-run list differs: got [$got] want [$EXPECT]"
[[ ! -e "$TMP/backups" && "$(ledger_max)" == "$AT029" ]] && pass "dry run backed up nothing and changed nothing" || fail "dry run left a trace"
run 1 "--from 031 refuses while 030 is pending" batch --dry-run --from 031
run 0 "--from 030 accepts" batch --dry-run --from 030

# Rows written before the batch: a rollback must keep them.
sq unheaded_app "INSERT INTO kanban_tasks (id, title) VALUES ('pre-batch-card', 'written before the batch')" >/dev/null

# 3. Appendix A1: a batch that fails part-way, then the rollback.
mkdir -p "$TMP/broken"
cp -a "$R/db/migrations" "$TMP/broken/migrations"
printf 'CREATE TABLE test_batch_half (id int);\nSELECT * FROM no_such_table;\n' >"$TMP/broken/migrations/999_test_batch_fails.sql"
echo "unheaded_app 999_test_batch_fails.sql" >>"$TMP/broken/migrations/MANIFEST"
WELL_MIGRATIONS_DIR="$TMP/broken/migrations" run 1 "A1: a batch with a broken last file fails" batch
grep -q "Do not re-run" "$TMP/out" && grep -q "batch-rollback.sh --yes" "$TMP/out" &&
	pass "A1: the failure prints the rollback command" || fail "A1: no rollback hint"
[[ "$(sq unheaded_ops "SELECT to_regclass('llm_requests') IS NOT NULL")" == t ]] &&
	pass "A1: the files before the broken one were applied" || fail "A1: nothing applied before the failure"
[[ "$(sq unheaded_app "SELECT to_regclass('test_batch_half') IS NULL")" == t ]] && pass "A1: the broken file left nothing" || fail "A1: broken file left a table"
mapfile -t PRES < <(find "$TMP/backups" -mindepth 1 -maxdepth 1 -type d -name 'pre-batch-*' | sort)
((${#PRES[@]} == 1)) && pass "one pre-batch directory" || fail "${#PRES[@]} pre-batch directories"
PRE1="${PRES[0]}"
grep -q $'^unheaded_app\t999_test_batch_fails.sql$' "$PRE1/pending.tsv" && pass "pending.tsv records the batch" || fail "pending.tsv: $(cat "$PRE1/pending.tsv")"

run 2 "rollback refuses a directory without pending.tsv" rollback --yes "$TMP"
cp -a "$PRE1" "$TMP/corrupt"
d="$(find "$TMP/corrupt" -name unheaded_app.dump | head -1)"
printf 'x' | dd of="$d" bs=1 seek=200 conv=notrunc status=none
run 2 "rollback refuses a set whose checksums fail" rollback --yes "$TMP/corrupt"
rm -rf -- "$TMP/corrupt"
before="$(ledger_max)"
WELL_MIGRATIONS_DIR="$TMP/broken/migrations" run 0 "rollback without --yes prints the plan" rollback "$PRE1"
grep -q "roles to drop: app_auth" "$TMP/out" && pass "the plan names the role the batch created" || fail "plan: $(grep roles "$TMP/out")"
[[ "$(ledger_max)" == "$before" ]] && pass "the plan changed nothing" || fail "plan-only run changed a ledger"
WELL_MIGRATIONS_DIR="$TMP/broken/migrations" run 0 "A1: rollback --yes" rollback --yes "$PRE1"
[[ "$(ledger_max)" == "$AT029" ]] && pass "A1: every ledger back at 029" || fail "A1: ledgers $(ledger_max), want $AT029"
[[ "$(sq unheaded_ops "SELECT to_regclass('llm_requests') IS NULL")$(sq unheaded_app "SELECT to_regclass('api_keys') IS NULL")$(sq unheaded_ops "SELECT to_regclass('slo_definitions') IS NULL")" == ttt ]] &&
	pass "A1: the batch's tables are gone" || fail "A1: batch tables survived"
[[ "$(sq postgres "SELECT count(*) FROM pg_roles WHERE rolname = 'app_auth'")" == 0 ]] && pass "A1: the batch's role is gone" || fail "A1: app_auth survived"
[[ "$(sq unheaded_app "SELECT count(*) FROM kanban_tasks WHERE id = 'pre-batch-card'")" == 1 ]] && pass "A1: rows written before the batch kept" || fail "A1: pre-batch row lost"
[[ "$(sq unheaded_app "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'kanban_tasks'::regclass")" == well_app_owner ]] &&
	pass "A1: owners restored (kanban_tasks is well_app_owner's)" || fail "A1: kanban_tasks owner"
[[ "$(sq postgres "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'unheaded_app'")" == well_app_owner ]] &&
	pass "A1: unheaded_app re-created with its owner" || fail "A1: database owner"
rc=0; WELL_MIGRATE_PASSWORD="$PW" "$TMP/well-migrate" -dir "$R/db/migrations" status >"$TMP/out" 2>&1 || rc=$?
((rc == 3)) && [[ "$(awk '$1 == "pending" {print $2 "\t" $3}' "$TMP/out")" == "$EXPECT" ]] &&
	pass "A1: status after rollback: the same files pending, no drift" || fail "A1: status rc $rc"

# 4. The real batch.
sleep 1 # pre-batch directories are named to the second
run 0 "batch applies" batch --from 030
grep -q "ledger rows added by this batch" "$TMP/out" && grep -q "status clean" "$TMP/out" && pass "it prints the ledger rows it added" || fail "no ledger report"
rc=0; WELL_MIGRATE_PASSWORD="$PW" "$TMP/well-migrate" -dir "$R/db/migrations" status >/dev/null 2>&1 || rc=$?
((rc == 0)) && pass "status clean after the batch" || fail "status rc $rc after the batch"
missing=0
while IFS=$'\t' read -r db f; do
	[[ "$(sq "$db" "SELECT mode FROM well_schema_migrations WHERE filename = '$f'")" == apply ]] || { missing=1; echo "    $db $f not recorded"; }
done <<<"$EXPECT"
((missing == 0)) && pass "every batch file recorded (mode apply) in its database's ledger" || fail "batch files missing from ledgers"
[[ "$(sq unheaded_app "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'api_keys'::regclass")" == well_app_owner ]] &&
	pass "batch created api_keys as the owner role" || fail "api_keys owner"
[[ "$(sq unheaded_app "SELECT count(*) FROM kanban_tasks WHERE id = 'pre-batch-card'")" == 1 ]] && pass "the board kept its card through the batch" || fail "card lost"

# 5. Roll the real batch back, apply again, then nothing is pending.
PRE2="$(find "$TMP/backups" -mindepth 1 -maxdepth 1 -type d -name 'pre-batch-*' | sort | tail -1)"
[[ "$PRE2" != "$PRE1" ]] || fail "no new pre-batch directory"
run 0 "rollback of the real batch" rollback --yes "$PRE2"
[[ "$(ledger_max)" == "$AT029" ]] && pass "every ledger back at 029" || fail "ledgers $(ledger_max), want $AT029"
sleep 1
run 0 "apply again after the rollback" batch
rc=0; WELL_MIGRATE_PASSWORD="$PW" "$TMP/well-migrate" -dir "$R/db/migrations" status >/dev/null 2>&1 || rc=$?
((rc == 0)) && pass "status clean after the second apply" || fail "status rc $rc"
run 0 "a third run" batch
grep -q "nothing pending" "$TMP/out" && pass "a third run finds nothing pending" || fail "third run: $(tail -1 "$TMP/out")"

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
