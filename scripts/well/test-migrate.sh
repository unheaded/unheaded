#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# End-to-end check of cmd/well-migrate and the manifest-driven db/init.sh
# (ADR-101) on throwaway servers, never the real Well:
#   1. a fresh bootstrap records every manifest entry in the right ledger
#      (mode initdb) and `status` is clean;
#   2. a new migration is pending, `apply` without -yes changes nothing,
#      `apply -yes` runs and records it;
#   3. editing an applied file is drift: status and apply refuse;
#   4. a migration that cannot get its lock fails within -lock-timeout,
#      records nothing and leaves no change (W-10);
#   5. a failing transactional migration records nothing; a migration
#      that creates a database leaves that database's entries pending,
#      and apply creates it, then migrates it;
#   6. the ledger refuses edits;
#   7. a Well bootstrapped by the pre-ledger db/ (the live one today) is
#      baselined through 016 without running anything, then apply runs
#      017 onwards;
#   9. kingdom_config refuses a change without a reason (028);
#   8. owners (025/026): every object but the ledger belongs to its
#      database's owner role, init.sh and apply create new objects as that
#      owner unless the entry is flagged superuser, PUBLIC cannot connect
#      and each service role reaches only its databases.
#
#   scripts/well/test-migrate.sh [image]   # default postgres:16.15-alpine
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="well-migrate-test-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
cleanup() {
	"${DOCKER[@]}" stop "$N" "$N-old" >/dev/null 2>&1 || true
	[[ -n "${HOLDER:-}" ]] && kill "$HOLDER" 2>/dev/null || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

start() { # name migrations-dir [POSTGRES_DB]
	"${DOCKER[@]}" run -d --rm --name "$1" -p 127.0.0.1::5432 \
		-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB="${3:-postgres}" \
		-v "$2:/migrations:ro" -v "$2/../init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
		"$IMAGE" >/dev/null
	for _ in $(seq 1 90); do
		if "${DOCKER[@]}" exec "$1" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then return 0; fi
		sleep 1
	done
	"${DOCKER[@]}" logs "$1" | tail -20
	echo "$1 did not become ready"; exit 1
}
sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2"; }

# A private copy of db/ so test migrations never touch the repo.
mkdir -p "$TMP/db"
cp -a "$R/db/migrations" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
M="$TMP/db/migrations"
# Two extra entries after 026, run by init.sh: one as the owner, one flagged.
echo "CREATE TABLE test_init_owner (id int PRIMARY KEY);" >"$M/898_test_init_owner.sql"
echo "CREATE TABLE test_init_su (id int PRIMARY KEY);" >"$M/899_test_init_su.sql"
printf '%s\n' "unheaded_ops 898_test_init_owner.sql" "unheaded_ops 899_test_init_su.sql superuser" >>"$M/MANIFEST"
start "$N" "$M"
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

(cd "$R" && GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.3}" go build -o "$TMP/well-migrate" ./cmd/well-migrate)
wm() { WELL_SSLMODE=disable WELL_HOST=127.0.0.1 WELL_PORT="${WP:-$PORT}" WELL_MIGRATE_PASSWORD="$PW" "$TMP/well-migrate" -dir "$M" "$@"; }

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
expect() { # want-rc label cmd...
	local want=$1 label=$2 rc=0; shift 2
	"$@" >"$TMP/out" 2>&1 || rc=$?
	if ((rc == want)); then pass "$label (rc $rc)"; else fail "$label: rc $rc, want $want"; sed 's/^/    /' "$TMP/out" | tail -8; fi
}

# 1. Fresh bootstrap.
entries=$(grep -E '^[a-z_]+ +[0-9]{3}_' "$M/MANIFEST" | grep -vc '^skip ')
recorded=0
for db in postgres unheaded_app unheaded_ops unheaded_config; do
	recorded=$((recorded + $(sq "$db" "SELECT count(*) FROM well_schema_migrations WHERE mode = 'initdb'")))
done
((recorded == entries)) && pass "init.sh recorded all $entries manifest entries" || fail "init.sh recorded $recorded of $entries"
[[ "$(sq unheaded_ops "SELECT count(*) FROM well_schema_migrations WHERE filename = '007_grants.sql'")" == 1 ]] &&
	pass "007 recorded once per database" || fail "007 in unheaded_ops ledger"
[[ "$(sq postgres "SELECT string_agg(datname, ' ' ORDER BY datname) FROM pg_database WHERE NOT datistemplate")" == "postgres unheaded_app unheaded_config unheaded_ops" ]] &&
	pass "only the manifest's databases exist (no legacy unheaded)" || fail "databases: $(sq postgres "SELECT string_agg(datname, ' ') FROM pg_database WHERE NOT datistemplate")"
expect 0 "status clean after bootstrap" wm status
# 017/018 on a fresh bootstrap: no duplicate index, the FK is indexed.
dups=$(sq unheaded_app "SELECT count(*) FROM pg_index a JOIN pg_index b ON a.indrelid = b.indrelid AND a.indexrelid < b.indexrelid AND a.indkey::text = b.indkey::text AND a.indclass::text = b.indclass::text AND coalesce(pg_get_expr(a.indexprs, a.indrelid), '') = coalesce(pg_get_expr(b.indexprs, b.indrelid), '') AND coalesce(pg_get_expr(a.indpred, a.indrelid), '') = coalesce(pg_get_expr(b.indpred, b.indrelid), '')")
[[ "$dups" == 0 ]] && pass "no duplicate indexes in unheaded_app" || fail "$dups duplicate index pair(s) in unheaded_app"
[[ "$(sq unheaded_app "SELECT to_regclass('idx_zhen_created') IS NOT NULL AND to_regclass('idx_zhen_search') IS NOT NULL")" == t ]] &&
	pass "003's zhen indexes kept" || fail "003's zhen indexes missing"
[[ "$(sq unheaded_config "SELECT to_regclass('idx_compliance_host_bindings_profile') IS NOT NULL")" == t ]] &&
	pass "bindings(profile) indexed" || fail "bindings(profile) not indexed"

# 8. Owners (ERDA 2.2).
owner_of() { sq "$1" "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = '$2'::regclass"; }
for pair in unheaded_app:well_app_owner unheaded_ops:well_ops_owner unheaded_config:well_config_owner; do
	db=${pair%%:*} want=${pair#*:}
	[[ "$(sq postgres "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = '$db'")" == "$want" ]] &&
		pass "$db owned by $want" || fail "$db owner"
	stray=$(sq "$db" "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m','S') AND c.relname NOT IN ('well_schema_migrations', 'test_init_su') AND pg_get_userbyid(c.relowner) <> '$want'")
	strayf=$(sq "$db" "SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public' AND p.proname <> 'well_append_only' AND pg_get_userbyid(p.proowner) <> '$want'")
	[[ "$stray$strayf" == 00 ]] && pass "$db: every relation and function is $want's" || fail "$db: $stray relation(s), $strayf function(s) not $want's"
	[[ "$(owner_of "$db" well_schema_migrations)" == unheaded ]] && pass "$db: ledger stays the superuser's" || fail "$db: ledger owner"
	[[ "$(sq postgres "SELECT has_database_privilege('public', '$db', 'CONNECT') OR has_database_privilege('public', '$db', 'TEMPORARY')")" == f ]] &&
		pass "$db: PUBLIC has no CONNECT/TEMPORARY" || fail "$db: PUBLIC can connect"
done
[[ "$(owner_of unheaded_ops test_init_owner)" == well_ops_owner ]] && pass "init.sh ran a later entry as the owner" || fail "init.sh owner: $(owner_of unheaded_ops test_init_owner)"
[[ "$(owner_of unheaded_ops test_init_su)" == unheaded ]] && pass "init.sh kept the superuser for a superuser entry" || fail "superuser entry owner: $(owner_of unheaded_ops test_init_su)"
reach=$(sq postgres "SELECT string_agg(r || '>' || d, ' ' ORDER BY r, d) FROM unnest(ARRAY['app_kanban','ops_writer','config_admin','huginn_reader','well_backup']) r, unnest(ARRAY['postgres','unheaded_app','unheaded_ops','unheaded_config']) d WHERE has_database_privilege(r, d, 'CONNECT')")
want_reach="app_kanban>unheaded_app app_kanban>unheaded_config config_admin>unheaded_config huginn_reader>unheaded_app huginn_reader>unheaded_config huginn_reader>unheaded_ops ops_writer>unheaded_ops well_backup>postgres well_backup>unheaded_app well_backup>unheaded_config well_backup>unheaded_ops"
[[ "$reach" == "$want_reach" ]] && pass "service roles connect only where granted" || fail "CONNECT map: $reach"

# 028: a kingdom_config change without a reason is refused; with one, history keeps it.
if sq unheaded_config "UPDATE kingdom_config SET value = '\"x\"' WHERE key = 'kingdom.mode'" >/dev/null 2>&1; then
	fail "kingdom_config changed without a reason"
else pass "kingdom_config change without a reason refused"; fi
sq unheaded_config "BEGIN; SET LOCAL well.change_reason = 'test: why'; UPDATE kingdom_config SET value = '\"x\"' WHERE key = 'kingdom.mode'; COMMIT;" >/dev/null
[[ "$(sq unheaded_config "SELECT change_reason FROM kingdom_config_history WHERE key = 'kingdom.mode' ORDER BY changed_at DESC LIMIT 1")" == "test: why" ]] &&
	pass "kingdom_config change with a reason recorded in history" || fail "history reason"

# 2. A new migration.
cat >"$M/901_test_widgets.sql" <<'SQL'
CREATE TABLE test_widgets (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name text NOT NULL);
SQL
expect 1 "check refuses an unrouted file" wm check
echo "unheaded_app 901_test_widgets.sql" >>"$M/MANIFEST"
expect 3 "status reports pending" wm status
expect 0 "apply without -yes" wm apply
[[ "$(sq unheaded_app "SELECT to_regclass('test_widgets') IS NULL")" == t ]] && pass "apply without -yes changed nothing" || fail "apply without -yes created the table"
expect 0 "apply -yes" wm apply -yes
[[ "$(sq unheaded_app "SELECT mode FROM well_schema_migrations WHERE filename = '901_test_widgets.sql'")" == apply ]] &&
	pass "901 recorded as apply" || fail "901 not recorded"
[[ "$(owner_of unheaded_app test_widgets)" == well_app_owner ]] && pass "apply created 901's table as the owner" || fail "901 owner: $(owner_of unheaded_app test_widgets)"
expect 0 "status clean after apply" wm status
expect 0 "second apply is a no-op" wm apply -yes

# 3. Drift.
cp "$M/901_test_widgets.sql" "$TMP/901.orig"
echo "-- edited" >>"$M/901_test_widgets.sql"
expect 1 "status refuses an edited applied file" wm status
echo "unheaded_app 902_after_edit.sql" >>"$M/MANIFEST"; echo "SELECT 1;" >"$M/902_after_edit.sql"
expect 1 "apply refuses to run past drift" wm apply -yes
[[ "$(sq unheaded_app "SELECT count(*) FROM well_schema_migrations WHERE filename = '902_after_edit.sql'")" == 0 ]] &&
	pass "nothing ran past drift" || fail "902 ran despite drift"
cp "$TMP/901.orig" "$M/901_test_widgets.sql"
expect 0 "902 applies once the file is restored" wm apply -yes

# 4. lock_timeout: another session holds ACCESS EXCLUSIVE on test_widgets.
"${DOCKER[@]}" exec -i "$N" psql -X -q -U unheaded -d unheaded_app \
	-c "BEGIN; LOCK TABLE test_widgets IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(60);" >/dev/null 2>&1 &
HOLDER=$!
for _ in $(seq 1 20); do
	[[ "$(sq unheaded_app "SELECT count(*) FROM pg_locks l JOIN pg_class c ON c.oid = l.relation WHERE c.relname = 'test_widgets' AND l.mode = 'AccessExclusiveLock' AND l.granted")" == 1 ]] && break
	sleep 0.5
done
echo "unheaded_app 903_widgets_color.sql" >>"$M/MANIFEST"
echo "ALTER TABLE test_widgets ADD COLUMN color text;" >"$M/903_widgets_color.sql"
t0=$(date +%s)
expect 1 "migration blocked by a lock fails" wm -lock-timeout 2s apply -yes
took=$(($(date +%s) - t0))
((took < 15)) && pass "it failed within the lock timeout (${took}s)" || fail "took ${took}s"
grep -q "lock timeout" "$TMP/out" && pass "the error names the lock timeout" || fail "error: $(tail -1 "$TMP/out")"
kill "$HOLDER" 2>/dev/null || true; wait "$HOLDER" 2>/dev/null || true; HOLDER=""
"${DOCKER[@]}" exec "$N" psql -X -q -U unheaded -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE query LIKE '%pg_sleep(60)%' AND pid <> pg_backend_pid()" >/dev/null
[[ "$(sq unheaded_app "SELECT count(*) FROM well_schema_migrations WHERE filename = '903_widgets_color.sql'")" == 0 ]] &&
	pass "blocked migration not recorded" || fail "blocked migration recorded"
expect 0 "it applies once the lock is gone" wm apply -yes

# 5. A failing transactional migration leaves nothing behind.
echo "unheaded_app 904_half.sql" >>"$M/MANIFEST"
printf 'CREATE TABLE half_done (id int);\nSELECT * FROM no_such_table;\n' >"$M/904_half.sql"
expect 1 "failing migration" wm apply -yes
[[ "$(sq unheaded_app "SELECT to_regclass('half_done') IS NULL AND NOT EXISTS (SELECT 1 FROM well_schema_migrations WHERE filename = '904_half.sql')")" == t ]] &&
	pass "failed migration rolled back and unrecorded" || fail "failed migration left state"
sed -i '/904_half.sql/d' "$M/MANIFEST"; rm "$M/904_half.sql"

# 5b. A migration that adds a database: until it runs, the new database
# has no ledger to read (it does not exist), so its entries are pending.
echo "CREATE DATABASE test_newdb;" >"$M/905_test_newdb.sql"
echo "CREATE TABLE test_new (id int PRIMARY KEY);" >"$M/906_test_newdb_table.sql"
printf '%s\n' "postgres 905_test_newdb.sql notx" "test_newdb 906_test_newdb_table.sql" >>"$M/MANIFEST"
expect 3 "status: a database the manifest adds is pending" wm status
expect 0 "apply creates the database, then migrates it" wm apply -yes
[[ "$(sq test_newdb "SELECT count(*) FROM well_schema_migrations WHERE filename = '906_test_newdb_table.sql'") $(sq test_newdb "SELECT to_regclass('test_new') IS NOT NULL")" == "1 t" ]] &&
	pass "the new database has its table and ledger row" || fail "new database not migrated"
expect 0 "status clean with the new database" wm status

# 6. The ledger is append-only.
if sq unheaded_app "UPDATE well_schema_migrations SET sha256 = repeat('0', 64)" >/dev/null 2>&1; then fail "ledger row edited"; else pass "ledger refuses UPDATE"; fi
if sq unheaded_app "TRUNCATE well_schema_migrations" >/dev/null 2>&1; then fail "ledger truncated"; else pass "ledger refuses TRUNCATE"; fi

# 7. Baseline: a Well built before the ledger (db/ as of the commit before
# MANIFEST was added: the live Well's situation) is baselined through the
# last file that init.sh applied (016), then apply runs only what is new.
PRE="$(cd "$R" && git log --format=%H --diff-filter=A -1 -- db/migrations/MANIFEST)^"
mkdir -p "$TMP/old"
(cd "$R" && git archive "$PRE" db) | tar -x -C "$TMP/old"
start "$N-old" "$TMP/old/db/migrations" unheaded   # the pre-ledger Well had POSTGRES_DB=unheaded
WP="$("${DOCKER[@]}" port "$N-old" 5432/tcp | head -1 | sed 's/.*://')"
M="$R/db/migrations"
osq() { "${DOCKER[@]}" exec -i "$N-old" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2"; }
expect 3 "old Well: everything pending" wm status
expect 1 "-db naming a database the manifest does not use" wm status -db nope
expect 0 "old Well: baseline postgres alone (-db)" wm baseline -db postgres -through 012_huginn_reader.sql -yes
[[ "$(osq unheaded_app "SELECT to_regclass('well_schema_migrations') IS NULL")" == t ]] &&
	pass "-db postgres left unheaded_app untouched" || fail "-db postgres wrote elsewhere"
expect 2 "baseline without -through is refused" wm baseline -yes
expect 2 "baseline without -yes is refused" wm baseline -through 016_config_history_append_only.sql
expect 1 "baseline through an unknown file is refused" wm baseline -through 099_nope.sql -yes
expect 0 "old Well: baseline through 016" wm baseline -through 016_config_history_append_only.sql -yes
expect 3 "old Well: 017-019 still pending" wm status
[[ "$(osq unheaded_config "SELECT count(*) FROM information_schema.columns WHERE table_name = 'kingdom_config_history' AND column_name = 'op'")" == 0 ]] &&
	pass "baseline ran nothing (019's column absent)" || fail "baseline changed the schema"
expect 0 "old Well: apply the rest" wm apply -yes
expect 0 "old Well: status clean" wm status
[[ "$(osq unheaded_config "SELECT count(*) FROM information_schema.columns WHERE table_name = 'kingdom_config_history' AND column_name = 'op'")" == 1 ]] &&
	pass "apply ran 019 on the old Well" || fail "019 not applied"
[[ "$(osq unheaded_app "SELECT string_agg(mode, ',' ORDER BY applied_at, filename) FROM (SELECT DISTINCT mode, min(applied_at) AS applied_at, min(filename) AS filename FROM well_schema_migrations GROUP BY mode) m")" == baseline,apply ]] &&
	pass "unheaded_app ledger: baseline rows, then apply" || fail "unheaded_app ledger modes"
[[ "$(osq unheaded_config "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'kingdom_config'::regclass")" == well_config_owner ]] &&
	pass "old Well: 026 handed kingdom_config to well_config_owner" || fail "old Well: kingdom_config owner"
unset WP

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
