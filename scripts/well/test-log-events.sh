#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks the log store table (db/migrations/048_ops_log_events.sql) on a
# throwaway PostgreSQL server bootstrapped by db/init.sh from the manifest,
# never the real Well:
#   1. the bootstrap records 048 in postgres and unheaded_ops; log_writer
#      logs in, connection limit 4, no superuser; the table and the function
#      belong to well_ops_owner; the bootstrap leaves no partition;
#   2. log_writer's grants: INSERT and log_events_maintain() only. It cannot
#      SELECT, UPDATE, DELETE or TRUNCATE the rows, create or drop a
#      partition by hand, read another ops table, or connect to another
#      database; PUBLIC cannot run the function;
#   3. log_events_maintain() as log_writer creates 16 daily partitions
#      (13 days back to 2 ahead, UTC), returns a byte count, and a second
#      call changes nothing;
#   4. a row inserts; the same row again is a no-op (ON CONFLICT DO NOTHING
#      without SELECT); a batch through unnest() inserts; a row outside the
#      window, a retention value outside the two, and an empty entry are
#      refused;
#   5. a partition older than 13 days is dropped with its rows by the next
#      call, a table with another name is left alone, and the returned
#      bytes grow with rows;
#   6. ops_reader reads the rows and cannot write them.
#
#   scripts/well/test-log-events.sh [--go] [image]   # default postgres:16.15-alpine
#
# --go also runs cmd/logstore-writer's integration test against the same
# throwaway server as log_writer, then reads back what it stored; the server
# is then published on a random loopback port (127.0.0.1 only) instead of
# having no network.
# WELL_MIGRATIONS_DIR (default db/migrations) points the bootstrap at
# another copy of the migrations, so a provocation can be tested.
# Needs docker (sudo if not in the group).
set -euo pipefail

GO=0
if [[ "${1:-}" == --go ]]; then GO=1; shift; fi
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="qa-log-events-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
LW_PW="lw-$RANDOM$RANDOM"
RD_PW="rd-$RANDOM$RANDOM"
cleanup() {
	timeout 60 "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

NET=(--network none)
if ((GO)); then NET=(-p 127.0.0.1::5432); fi
mkdir -p "$TMP/db"
cp -a "$SRC" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
timeout 120 "${DOCKER[@]}" run -d --rm --name "$N" "${NET[@]}" \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-e LOG_WRITER_PASSWORD="$LW_PW" \
	-v "$TMP/db/migrations:/migrations:ro" -v "$TMP/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 120); do
	if timeout 10 "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { timeout 20 "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready"; exit 1; }

# sq DB SQL: superuser. login ROLE PASSWORD DB SQL: a real login over TCP.
sq() { timeout 60 "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2"; }
login() { timeout 60 "${DOCKER[@]}" exec -i -e PGPASSWORD="$2" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$1" -d "$3" -c "$4"; }
lw() { login log_writer "$LW_PW" unheaded_ops "$1"; }
# Passwords exist only inside this throwaway container.
sq postgres "ALTER ROLE ops_reader PASSWORD '$RD_PW';" >/dev/null

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
ok() { # label cmd... : must succeed
	local label=$1; shift
	if "$@" >"$TMP/out" 2>&1; then pass "$label"; else fail "$label"; sed 's/^/    /' "$TMP/out" | tail -4; fi
}
refused() { # label cmd... : must fail
	local label=$1; shift
	if "$@" >"$TMP/out" 2>&1; then fail "$label: was allowed"; else pass "$label refused"; fi
}
eq() { # label got want
	[[ "$2" == "$3" ]] && pass "$1" || fail "$1: got [$2] want [$3]"
}
parts() { sq unheaded_ops "SELECT count(*) FROM pg_inherits WHERE inhparent = 'log_events'::regclass"; }
# row TS ENTRY [RETENTION]: one INSERT statement, as the writer sends it.
row() {
	echo "INSERT INTO log_events (ts, host, source, unit, level, retention, message, entry, fields)
	      VALUES ($1, 'qa', 'journald', 'sshd.service', 'info', '${3:-operational}', 'm', '$2', '{}')
	      ON CONFLICT DO NOTHING"
}

# 1. Ledger, role, owners.
eq "048 in the postgres ledger" "$(sq postgres "SELECT count(*) FROM well_schema_migrations WHERE filename = '048_ops_log_events.sql'")" 1
eq "048 in the unheaded_ops ledger" "$(sq unheaded_ops "SELECT count(*) FROM well_schema_migrations WHERE filename = '048_ops_log_events.sql'")" 1
eq "log_writer: login, limit 4, no superuser/createrole" \
	"$(sq postgres "SELECT rolcanlogin, rolconnlimit, rolsuper, rolcreaterole FROM pg_roles WHERE rolname = 'log_writer'")" "t|4|f|f"
eq "log_events belongs to well_ops_owner" "$(sq unheaded_ops "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'log_events'::regclass")" well_ops_owner
eq "log_events_maintain() belongs to well_ops_owner, SECURITY DEFINER, pinned search_path" \
	"$(sq unheaded_ops "SELECT pg_get_userbyid(proowner), prosecdef, proconfig::text LIKE '%search_path=pg_catalog, public%' FROM pg_proc WHERE proname = 'log_events_maintain'")" "well_ops_owner|t|t"
eq "the bootstrap leaves no partition" "$(parts)" 0
ok "log_writer logs in to unheaded_ops over TCP" lw "SELECT 1"
for db in unheaded_app unheaded_config postgres; do
	refused "log_writer CONNECT $db" login log_writer "$LW_PW" "$db" "SELECT 1"
done

# 2. Grants.
refused "log_writer SELECT log_events" lw "SELECT count(*) FROM log_events"
refused "log_writer UPDATE log_events" lw "UPDATE log_events SET message = 'x'"
refused "log_writer DELETE log_events" lw "DELETE FROM log_events"
refused "log_writer TRUNCATE log_events" lw "TRUNCATE log_events"
refused "log_writer CREATE a partition by hand" lw "CREATE TABLE log_events_p19990101 PARTITION OF log_events FOR VALUES FROM ('1999-01-01') TO ('1999-01-02')"
refused "log_writer SELECT audit_events" lw "SELECT count(*) FROM audit_events"
refused "log_writer INSERT audit_events" lw "INSERT INTO audit_events DEFAULT VALUES"
eq "PUBLIC cannot run log_events_maintain()" \
	"$(sq unheaded_ops "SELECT has_function_privilege('ops_reader', 'log_events_maintain()', 'EXECUTE')")" f

# 3. Partitions.
ok "log_writer runs log_events_maintain()" lw "SELECT log_events_maintain()"
eq "16 daily partitions" "$(parts)" 16
eq "from 13 days back to 2 ahead (UTC)" \
	"$(sq unheaded_ops "SELECT min(relname) || ' ' || max(relname) FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'log_events'::regclass")" \
	"$(sq unheaded_ops "SELECT 'log_events_p' || to_char((now() AT TIME ZONE 'UTC')::date - 13, 'YYYYMMDD') || ' log_events_p' || to_char((now() AT TIME ZONE 'UTC')::date + 2, 'YYYYMMDD')")"
eq "partitions belong to well_ops_owner" \
	"$(sq unheaded_ops "SELECT DISTINCT pg_get_userbyid(c.relowner) FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'log_events'::regclass")" well_ops_owner
empty="$(lw "SELECT log_events_maintain()")"
[[ "$empty" =~ ^[0-9]+$ ]] && pass "returns a byte count ($empty, empty)" || fail "return: [$empty]"
eq "a second call adds nothing" "$(parts)" 16
refused "log_writer DROP a partition by hand" lw "DROP TABLE log_events_p$(date -u +%Y%m%d)"
refused "log_writer SELECT a partition directly" lw "SELECT count(*) FROM log_events_p$(date -u +%Y%m%d)"

# 4. Inserts.
ok "a row inserts" lw "$(row "now()" e1)"
ok "an audit row inserts" lw "$(row "now() - interval '1 hour'" e2 audit)"
T="$(sq unheaded_ops "SELECT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"')")"
ok "a fixed-time row inserts" lw "$(row "'$T'" e3)"
ok "the same row again is accepted" lw "$(row "'$T'" e3)"
eq "and stored once" "$(sq unheaded_ops "SELECT count(*) FROM log_events WHERE entry = 'e3'")" 1
ok "a batch through unnest() inserts" lw "
	INSERT INTO log_events (ts, host, source, unit, level, retention, message, entry, fields)
	SELECT * FROM unnest(
		ARRAY['$T', '$T']::timestamptz[], ARRAY['qa', 'qa'], ARRAY['journald', 'journald'],
		ARRAY['a.service', 'b.service'], ARRAY['info', 'err'], ARRAY['operational', 'audit'],
		ARRAY['one', 'two'], ARRAY['e3', 'e4'], ARRAY['{}', '{}'])
	ON CONFLICT DO NOTHING"
eq "the batch added its one new row" "$(sq unheaded_ops "SELECT count(*) FROM log_events")" 4
refused "a row 20 days old (no partition)" lw "$(row "now() - interval '20 days'" old)"
refused "a row 5 days ahead (no partition)" lw "$(row "now() + interval '5 days'" future)"
refused "retention 'forever'" lw "$(row "now()" bad forever)"
refused "an empty entry" lw "$(row "now()" '')"
refused "an empty host" lw "INSERT INTO log_events VALUES (now(), '', 'journald', 'u', 'info', 'audit', 'm', 'e9', '{}')"

# 5. Retention: an old partition goes, a stranger stays, bytes are counted.
sq unheaded_ops "
	SET ROLE well_ops_owner;
	CREATE TABLE log_events_p20200101 PARTITION OF log_events FOR VALUES FROM ('2020-01-01 00:00:00+00') TO ('2020-01-02 00:00:00+00');
	INSERT INTO log_events VALUES ('2020-01-01 12:00:00+00', 'qa', 'journald', 'u', 'info', 'audit', 'old', 'e-old', '{}');
	CREATE TABLE log_events_keep PARTITION OF log_events FOR VALUES FROM ('2019-01-01 00:00:00+00') TO ('2019-01-02 00:00:00+00');" >/dev/null
eq "seeded: 18 partitions, 5 rows" "$(parts) $(sq unheaded_ops "SELECT count(*) FROM log_events")" "18 5"
sq unheaded_ops "
	INSERT INTO log_events
	SELECT now() - (g || ' ms')::interval, 'qa', 'journald', 'bulk.service', 'info', 'operational', repeat('x', 1000), 'b' || g, '{}'
	FROM generate_series(1, 2000) AS g" >/dev/null
full="$(lw "SELECT log_events_maintain()")"
eq "the old partition is dropped" "$(sq unheaded_ops "SELECT to_regclass('public.log_events_p20200101') IS NULL")" t
eq "its row went with it" "$(sq unheaded_ops "SELECT count(*) FROM log_events WHERE entry = 'e-old'")" 0
eq "a partition with another name is left alone" "$(sq unheaded_ops "SELECT to_regclass('public.log_events_keep') IS NOT NULL")" t
eq "17 partitions remain" "$(parts)" 17
if [[ "$full" =~ ^[0-9]+$ ]] && ((full > empty + 1000000)); then
	pass "the byte count grew with 2,000 rows ($empty to $full)"
else
	fail "byte count: empty [$empty] full [$full]"
fi
eq "the count is the partitions' total size" "$full" \
	"$(sq unheaded_ops "SELECT sum(pg_total_relation_size(inhrelid))::bigint FROM pg_inherits WHERE inhparent = 'log_events'::regclass")"

# 6. The reader.
eq "ops_reader reads log_events" "$(login ops_reader "$RD_PW" unheaded_ops "SELECT count(*) FROM log_events WHERE unit = 'sshd.service'")" 3
refused "ops_reader INSERT log_events" login ops_reader "$RD_PW" unheaded_ops "$(row "now()" r1)"
refused "ops_reader runs log_events_maintain()" login ops_reader "$RD_PW" unheaded_ops "SELECT log_events_maintain()"

# 7. The writer's own statement, as log_writer, through the Go driver.
if ((GO)); then
	PORT="$(timeout 20 "${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"
	if (cd "$R" && LOGSTORE_TEST_DSN="host=127.0.0.1 port=$PORT user=log_writer password=$LW_PW dbname=unheaded_ops sslmode=disable" \
		go test ./cmd/logstore-writer/ -run Integration -count=1 -v >"$TMP/go" 2>&1) && grep -q -- '--- PASS: TestIntegrationWellStore' "$TMP/go"; then
		pass "cmd/logstore-writer integration test as log_writer"
	else
		fail "cmd/logstore-writer integration test"; tail -15 "$TMP/go" | sed 's/^/    /'
	fi
	eq "the writer stored its 9 rows once each" "$(sq unheaded_ops "SELECT count(*) FROM log_events WHERE host = 'qa-go'")" 9
	eq "text came through the array intact" \
		"$(sq unheaded_ops "SELECT (SELECT message FROM log_events WHERE entry = 'go-2') = '\"quoted\", {a,b} \\ back'
			AND (SELECT message FROM log_events WHERE entry = 'go-3') = E'two\\nlines'
			AND (SELECT message FROM log_events WHERE entry = 'go-4') = 'NULL'
			AND (SELECT length(message) FROM log_events WHERE entry = 'go-6') = 200000
			AND (SELECT message FROM log_events WHERE entry = 'go-7') = ''
			AND (SELECT fields FROM log_events WHERE entry = 'go-1') = '{\"k\":\"v, \\\"quoted\\\" {braces} \\\\ back\"}'")" t
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
