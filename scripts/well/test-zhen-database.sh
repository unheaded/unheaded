#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Checks unheaded_zhen (db/migrations/051-053, ADR-149) on throwaway
# PostgreSQL servers bootstrapped by db/init.sh, never the real Well:
#   1. a server from the repository's manifest: unheaded_zhen exists, owned
#      by well_zhen_owner; PUBLIC has no CONNECT on it; app_zhen's
#      connection limit is 20; unheaded_app holds no zhen_% table or
#      function; kanban_zhen_optin stays in unheaded_app, and app_zhen may
#      SELECT it and kanban_tasks there;
#   2. a second server from a copy of the migrations whose manifest stops
#      at 049: its unheaded_app fingerprint
#      (scripts/well/zhen-schema-fingerprint.sql) and the first server's
#      unheaded_zhen fingerprint differ by exactly the deliberate changes in
#      scripts/well/zhen-schema-changes.txt (scripts/well/zhen_schema_diff.py);
#   3. on the second server: 051 and 052 applied with well-migrate; rows in
#      unheaded_app, NULLs and a '' trace_id among them; 053 refused ("copy
#      the rows"); scripts/well/zhen-copy-rows.sh copies them (counts equal,
#      NULLs now the defaults, the '' trace_id NULL, the next id above the
#      copied ones); a row written after the copy makes 053 refuse ("the copy
#      recorded"); without it 053 runs, and the tables and the copy's record
#      (the comment on zhen_conversations) are gone afterwards;
#   4. pkg/zhenagent's split store (build tag wellint, -run Split) against
#      the first server: WELL_TEST_ZHEN_DSN (unheaded_zhen as app_zhen),
#      WELL_TEST_APP_DSN (unheaded_app as app_zhen), WELL_TEST_KANBAN_DSN
#      (unheaded_app as app_kanban); a skipped test counts as a failure.
#
#   scripts/well/test-zhen-database.sh [image]    # default postgres:16.15-alpine
#
# Needs docker (sudo if not in the group). Its servers keep their data in
# Docker's storage: never run it while an evidence window forbids load on it.
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
TMP="$(mktemp -d)"
NAMES=()
cleanup() {
	local n
	for n in "${NAMES[@]}"; do "${DOCKER[@]}" stop "$n" >/dev/null 2>&1 || true; done
	rm -rf -- "$TMP"
}
trap cleanup EXIT

PW="throwaway-$RANDOM$RANDOM"
ZPW="zhen-$RANDOM$RANDOM"
KPW="kanban-$RANDOM$RANDOM"

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

# serve NAME MIGRATIONS_DIR: start a server bootstrapped from that copy.
serve() {
	local n="$1" src="$2" d="$TMP/$1"
	mkdir -p "$d"
	cp -a "$src" "$d/migrations"
	cp "$R/db/init.sh" "$d/init.sh"
	chmod -R a+rX "$d"
	"${DOCKER[@]}" run -d --rm --name "$n" -p 127.0.0.1::5432 \
		-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
		-v "$d/migrations:/migrations:ro" -v "$d/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
		"$IMAGE" >/dev/null
	NAMES+=("$n")
	local ok=0
	for _ in $(seq 1 120); do
		if "${DOCKER[@]}" exec "$n" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ok=1; break; fi
		sleep 1
	done
	((ok)) || { "${DOCKER[@]}" logs "$n" 2>&1 | tail -20; echo "postgres $n did not become ready"; exit 1; }
	for _ in $(seq 1 60); do
		"${DOCKER[@]}" exec "$n" psql -X -q -At -h 127.0.0.1 -U unheaded -d unheaded_app -c "SELECT 1 FROM well_schema_migrations LIMIT 1" >/dev/null 2>&1 && break
		sleep 1
	done
}
su() { "${DOCKER[@]}" exec -i "$1" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$2" -c "$3"; }
fingerprint() { "${DOCKER[@]}" exec -i "$1" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$2" <"$R/scripts/well/zhen-schema-fingerprint.sql"; }

# 1. The repository's manifest.
A="qa-zhen-db-a-$$"
serve "$A" "$R/db/migrations"
if [[ "$(su "$A" postgres "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'unheaded_zhen'")" == well_zhen_owner ]]; then
	pass "unheaded_zhen exists, owner well_zhen_owner"; else fail "unheaded_zhen owner"; fi
if [[ "$(su "$A" postgres "SELECT has_database_privilege('public', 'unheaded_zhen', 'CONNECT')")" == f ]]; then
	pass "PUBLIC has no CONNECT on unheaded_zhen"; else fail "PUBLIC may connect"; fi
if [[ "$(su "$A" postgres "SELECT rolconnlimit FROM pg_roles WHERE rolname = 'app_zhen'")" == 20 ]]; then
	pass "app_zhen connection limit 20"; else fail "app_zhen connection limit"; fi
if [[ "$(su "$A" unheaded_app "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relname LIKE 'zhen\_%' AND c.relkind IN ('r','p')")" == 0 ]]; then
	pass "unheaded_app holds no zhen_% table"; else fail "zhen_% tables left in unheaded_app"; fi
if [[ "$(su "$A" unheaded_app "SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public' AND p.proname LIKE 'zhen\_%'")" == 0 ]]; then
	pass "unheaded_app holds no zhen_% function"; else fail "zhen_% functions left in unheaded_app"; fi
if [[ "$(su "$A" unheaded_app "SELECT to_regclass('public.kanban_zhen_optin') IS NOT NULL AND has_table_privilege('app_zhen', 'kanban_zhen_optin', 'SELECT') AND has_table_privilege('app_zhen', 'kanban_tasks', 'SELECT')")" == t ]]; then
	pass "kanban_zhen_optin stays in unheaded_app; app_zhen reads it and kanban_tasks"; else fail "kanban_zhen_optin or app_zhen's board reads"; fi
fingerprint "$A" unheaded_zhen >"$TMP/fp-zhen.txt"

# 2. The schema before the move, from a manifest that stops at 049.
B="qa-zhen-db-b-$$"
mkdir -p "$TMP/at049"
cp -a "$R/db/migrations" "$TMP/at049/migrations"
python3 -I - "$TMP/at049/migrations/MANIFEST" <<'PY'
import sys
p = sys.argv[1]
out = []
for line in open(p, encoding="utf-8"):
    parts = line.split()
    if len(parts) >= 2 and parts[0] != "skip" and parts[1][:3].isdigit() and int(parts[1][:3]) > 49:
        continue
    out.append(line)
open(p, "w", encoding="utf-8").write("".join(out))
PY
serve "$B" "$TMP/at049/migrations"
fingerprint "$B" unheaded_app >"$TMP/fp-app.txt"
if python3 -I "$R/scripts/well/zhen_schema_diff.py" "$TMP/fp-app.txt" "$TMP/fp-zhen.txt" >"$TMP/fp.diff"; then
	pass "fingerprints differ by the listed changes only ($(wc -l <"$TMP/fp-zhen.txt") lines)"
else
	fail "fingerprints differ by more or less than scripts/well/zhen-schema-changes.txt"; sed 's/^/    /' "$TMP/fp.diff" | head -20
fi

# 3. 051-053 on the 049 server, with a row in the way.
PORT_B="$("${DOCKER[@]}" port "$B" 5432/tcp | head -1 | sed 's/.*://')"
(cd "$R" && go build -o "$TMP/well-migrate" ./cmd/well-migrate)
migrate() { # args...
	WELL_SSLMODE=disable WELL_HOST=127.0.0.1 WELL_PORT="$PORT_B" WELL_MIGRATE_PASSWORD="$PW" "$TMP/well-migrate" -dir "$R/db/migrations" "$@"
}
if migrate apply -db postgres -yes >"$TMP/m1" 2>&1 && migrate apply -db unheaded_zhen -yes >>"$TMP/m1" 2>&1; then
	pass "051 and 052 applied with well-migrate"
else
	fail "051/052 with well-migrate"; tail -5 "$TMP/m1" | sed 's/^/    /'
fi
su "$B" unheaded_app "INSERT INTO zhen_memories (question, answer, source, created_at) VALUES ('q', 'a', NULL, NULL)" >/dev/null
su "$B" unheaded_app "INSERT INTO zhen_conversations (session_id, role, content, model, tokens_input) VALUES ('s', 'user', 'hi', NULL, NULL)" >/dev/null
su "$B" unheaded_app "INSERT INTO zhen_actions (session_id, action_type, intent, result_summary, error, triggered_by, trace_id) VALUES ('s', 'tool_call_attempt', 'qa', NULL, NULL, NULL, ''), ('s', 'tool_call_attempt', 'qa', 'ok', '', 'zhen', '0123456789abcdef0123456789abcdef')" >/dev/null
if migrate apply -db unheaded_app -yes >"$TMP/m2" 2>&1; then
	fail "053 ran over an uncopied row"
elif grep -q 'copy the rows' "$TMP/m2"; then
	pass "053 refused: copy the rows first"
else
	fail "053 failed for another reason"; tail -5 "$TMP/m2" | sed 's/^/    /'
fi
if WELL_CONTAINER="$B" WELL_SUPERUSER=unheaded "$R/scripts/well/zhen-copy-rows.sh" >"$TMP/copy" 2>&1; then
	pass "zhen-copy-rows.sh copied the rows"
else
	fail "zhen-copy-rows.sh"; tail -5 "$TMP/copy" | sed 's/^/    /'
fi
got="$(su "$B" unheaded_zhen "SELECT (SELECT count(*) FROM zhen_memories) || ' ' || (SELECT count(*) FROM zhen_conversations) || ' ' || (SELECT count(*) FROM zhen_actions)
  || ' ' || (SELECT count(*) FROM zhen_memories WHERE source = 'user' AND created_at IS NOT NULL)
  || ' ' || (SELECT count(*) FROM zhen_conversations WHERE model = '' AND tokens_input = 0)
  || ' ' || (SELECT count(*) FROM zhen_actions WHERE result_summary = '' AND error = '' AND triggered_by = 'user' AND trace_id IS NULL)
  || ' ' || (SELECT count(*) FROM zhen_actions WHERE trace_id = '0123456789abcdef0123456789abcdef'::uuid)")"
if [[ "$got" == "1 1 2 1 1 1 1" ]]; then
	pass "counts equal; NULLs took the defaults; '' trace_id NULL, a hex one kept"
else
	fail "copied rows: $got (want 1 1 2 1 1 1 1)"
fi
if [[ "$(su "$B" unheaded_zhen "INSERT INTO zhen_actions (session_id, action_type, intent) VALUES ('s', 'tool_call_attempt', 'next') RETURNING id > (SELECT max(id) FROM zhen_actions WHERE intent = 'qa')")" == t ]]; then
	pass "the next zhen_actions id follows the copied ones"
else
	fail "zhen_actions identity behind the copied ids"
fi
su "$B" unheaded_app "INSERT INTO zhen_memories (question, answer) VALUES ('late', 'row')" >/dev/null
if migrate apply -db unheaded_app -yes >"$TMP/m3" 2>&1; then
	fail "053 ran over a row written after the copy"
elif grep -q 'the copy recorded' "$TMP/m3"; then
	pass "053 refused: a row written after the copy"
else
	fail "053 (late row) failed for another reason"; tail -5 "$TMP/m3" | sed 's/^/    /'
fi
su "$B" unheaded_app "DELETE FROM zhen_memories WHERE question = 'late'" >/dev/null
if migrate apply -db unheaded_app -yes >"$TMP/m4" 2>&1; then
	pass "053 ran once the recorded counts held"
else
	fail "053 after the copy"; tail -5 "$TMP/m4" | sed 's/^/    /'
fi
# Changed 2026-11-18 with 053: the copy's record is the comment on
# zhen_conversations, no longer a database setting. The check here read
# pg_db_role_setting, which nothing writes now, so it could not fail; this
# one fails when 053 leaves a table or the record.
if [[ "$(su "$B" unheaded_app "SELECT (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relname LIKE 'zhen\_%' AND c.relkind IN ('r','p'))
  || ' ' || (SELECT count(*) FROM pg_description WHERE description LIKE 'copied %zhen\_%=%')")" == "0 0" ]]; then
	pass "053 dropped the eight tables and the copy's record with them"; else fail "053 left a zhen table or the copy's record"; fi

# 4. The split store against the first server.
PORT_A="$("${DOCKER[@]}" port "$A" 5432/tcp | head -1 | sed 's/.*://')"
su "$A" postgres "ALTER ROLE app_zhen PASSWORD '$ZPW'; ALTER ROLE app_kanban PASSWORD '$KPW';" >/dev/null
dsn() { echo "postgres://$1:$2@127.0.0.1:$PORT_A/$3?sslmode=disable"; }
if (cd "$R" && WELL_TEST_ZHEN_DSN="$(dsn app_zhen "$ZPW" unheaded_zhen)" WELL_TEST_APP_DSN="$(dsn app_zhen "$ZPW" unheaded_app)" \
	WELL_TEST_KANBAN_DSN="$(dsn app_kanban "$KPW" unheaded_app)" \
	go test -tags wellint -count=1 -v -run Split ./pkg/zhenagent/) >"$TMP/go" 2>&1 \
	&& ! grep -q -- '--- SKIP' "$TMP/go"; then
	pass "pkg/zhenagent split store (wellint) against unheaded_zhen + unheaded_app"
else
	fail "pkg/zhenagent split store"; grep -E -- '--- (FAIL|SKIP)|FAIL|panic' "$TMP/go" | head -10 | sed 's/^/    /'
fi

if ((fails)); then echo "test-zhen-database: $fails failure(s)"; exit 1; fi
echo "test-zhen-database: all checks pass"
