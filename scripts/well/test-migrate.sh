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
#   5. a failing transactional migration records nothing;
#   6. the ledger refuses edits;
#   7. a Well bootstrapped without a ledger (the live one today) can be
#      baselined, then sees only what is new.
#
#   scripts/well/test-migrate.sh [image]   # default postgres:16-alpine
set -euo pipefail

IMAGE="${1:-postgres:16-alpine}"
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

start() { # name migrations-dir
	"${DOCKER[@]}" run -d --rm --name "$1" -p 127.0.0.1::5432 \
		-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=unheaded \
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
start "$N" "$M"
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

(cd "$R" && GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.2}" go build -o "$TMP/well-migrate" ./cmd/well-migrate)
wm() { WELL_HOST=127.0.0.1 WELL_PORT="${WP:-$PORT}" WELL_MIGRATE_PASSWORD="$PW" "$TMP/well-migrate" -dir "$M" "$@"; }

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
[[ "$(sq unheaded "SELECT to_regclass('public.well_schema_migrations') IS NULL")" == t ]] &&
	pass "no ledger in the legacy database" || fail "legacy database has a ledger"
expect 0 "status clean after bootstrap" wm status

# 2. A new migration.
cat >"$M/017_test_widgets.sql" <<'SQL'
CREATE TABLE test_widgets (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, name text NOT NULL);
SQL
expect 1 "check refuses an unrouted file" wm check
echo "unheaded_app 017_test_widgets.sql" >>"$M/MANIFEST"
expect 3 "status reports pending" wm status
expect 0 "apply without -yes" wm apply
[[ "$(sq unheaded_app "SELECT to_regclass('test_widgets') IS NULL")" == t ]] && pass "apply without -yes changed nothing" || fail "apply without -yes created the table"
expect 0 "apply -yes" wm apply -yes
[[ "$(sq unheaded_app "SELECT mode FROM well_schema_migrations WHERE filename = '017_test_widgets.sql'")" == apply ]] &&
	pass "017 recorded as apply" || fail "017 not recorded"
expect 0 "status clean after apply" wm status
expect 0 "second apply is a no-op" wm apply -yes

# 3. Drift.
cp "$M/017_test_widgets.sql" "$TMP/017.orig"
echo "-- edited" >>"$M/017_test_widgets.sql"
expect 1 "status refuses an edited applied file" wm status
echo "unheaded_app 018_after_edit.sql" >>"$M/MANIFEST"; echo "SELECT 1;" >"$M/018_after_edit.sql"
expect 1 "apply refuses to run past drift" wm apply -yes
[[ "$(sq unheaded_app "SELECT count(*) FROM well_schema_migrations WHERE filename = '018_after_edit.sql'")" == 0 ]] &&
	pass "nothing ran past drift" || fail "018 ran despite drift"
cp "$TMP/017.orig" "$M/017_test_widgets.sql"
expect 0 "018 applies once the file is restored" wm apply -yes

# 4. lock_timeout: another session holds ACCESS EXCLUSIVE on test_widgets.
"${DOCKER[@]}" exec -i "$N" psql -X -q -U unheaded -d unheaded_app \
	-c "BEGIN; LOCK TABLE test_widgets IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(60);" >/dev/null 2>&1 &
HOLDER=$!
for _ in $(seq 1 20); do
	[[ "$(sq unheaded_app "SELECT count(*) FROM pg_locks l JOIN pg_class c ON c.oid = l.relation WHERE c.relname = 'test_widgets' AND l.mode = 'AccessExclusiveLock' AND l.granted")" == 1 ]] && break
	sleep 0.5
done
echo "unheaded_app 019_widgets_color.sql" >>"$M/MANIFEST"
echo "ALTER TABLE test_widgets ADD COLUMN color text;" >"$M/019_widgets_color.sql"
t0=$(date +%s)
expect 1 "migration blocked by a lock fails" wm -lock-timeout 2s apply -yes
took=$(($(date +%s) - t0))
((took < 15)) && pass "it failed within the lock timeout (${took}s)" || fail "took ${took}s"
grep -q "lock timeout" "$TMP/out" && pass "the error names the lock timeout" || fail "error: $(tail -1 "$TMP/out")"
kill "$HOLDER" 2>/dev/null || true; wait "$HOLDER" 2>/dev/null || true; HOLDER=""
"${DOCKER[@]}" exec "$N" psql -X -q -U unheaded -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE query LIKE '%pg_sleep(60)%' AND pid <> pg_backend_pid()" >/dev/null
[[ "$(sq unheaded_app "SELECT count(*) FROM well_schema_migrations WHERE filename = '019_widgets_color.sql'")" == 0 ]] &&
	pass "blocked migration not recorded" || fail "blocked migration recorded"
expect 0 "it applies once the lock is gone" wm apply -yes

# 5. A failing transactional migration leaves nothing behind.
echo "unheaded_app 020_half.sql" >>"$M/MANIFEST"
printf 'CREATE TABLE half_done (id int);\nSELECT * FROM no_such_table;\n' >"$M/020_half.sql"
expect 1 "failing migration" wm apply -yes
[[ "$(sq unheaded_app "SELECT to_regclass('half_done') IS NULL AND NOT EXISTS (SELECT 1 FROM well_schema_migrations WHERE filename = '020_half.sql')")" == t ]] &&
	pass "failed migration rolled back and unrecorded" || fail "failed migration left state"
sed -i '/020_half.sql/d' "$M/MANIFEST"; rm "$M/020_half.sql"

# 6. The ledger is append-only.
if sq unheaded_app "UPDATE well_schema_migrations SET sha256 = repeat('0', 64)" >/dev/null 2>&1; then fail "ledger row edited"; else pass "ledger refuses UPDATE"; fi
if sq unheaded_app "TRUNCATE well_schema_migrations" >/dev/null 2>&1; then fail "ledger truncated"; else pass "ledger refuses TRUNCATE"; fi

# 7. Baseline: an old-style Well (bootstrapped without a ledger) is recorded
# without running anything, then sees only new files.
mkdir -p "$TMP/old/db"
cp -a "$R/db/migrations" "$TMP/old/db/migrations"
(cd "$R" && git show HEAD:db/init.sh) >"$TMP/old/db/init.sh"
if grep -q MANIFEST "$TMP/old/db/init.sh"; then
	echo "[SKIP] baseline: HEAD's init.sh already uses the manifest"
else
	# HEAD's init.sh applies the same files but keeps no ledger: the live
	# Well's situation.
	start "$N-old" "$TMP/old/db/migrations"
	WP="$("${DOCKER[@]}" port "$N-old" 5432/tcp | head -1 | sed 's/.*://')"
	M_SAVED=$M; M="$R/db/migrations"
	expect 3 "old Well: everything pending" wm status
	expect 2 "baseline without -yes is refused" wm baseline
	expect 0 "old Well: baseline -yes" wm baseline -yes
	expect 0 "old Well: status clean after baseline" wm status
	n=$("${DOCKER[@]}" exec "$N-old" psql -X -At -U unheaded -d unheaded_app -c "SELECT count(*) FROM well_schema_migrations WHERE mode = 'baseline'")
	[[ "$n" == "$(grep -cE '^unheaded_app +[0-9]{3}_' "$M/MANIFEST")" ]] && pass "baseline recorded every unheaded_app entry ($n)" || fail "baseline recorded $n"
	unset WP
	M=$M_SAVED
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
