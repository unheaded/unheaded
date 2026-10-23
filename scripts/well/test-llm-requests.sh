#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_ops.llm_requests (db/migrations/030) on a throwaway
# PostgreSQL server bootstrapped by db/init.sh from the manifest, never the
# real Well:
#   1. the bootstrap records 030 in unheaded_ops' ledger; the table belongs
#      to well_ops_owner;
#   2. ops_writer, logged in over TCP, can INSERT and SELECT;
#   3. ops_writer cannot UPDATE, DELETE or TRUNCATE (no grant), and neither
#      can the owner (the append-only triggers);
#   4. the CHECKs refuse an unknown backend, a negative cost and an unknown
#      route_reason;
#   5. ops_reader can read and cannot write.
#   6. pkg/claudebridge's Well integration test (build tag wellint) runs
#      one bridge turn against the fake Claude API with the Well auditor
#      writing to this server as ops_writer (WELL_TEST_DSN): one row per
#      request, one audit event per tool decision, a valid hash chain;
#   7. a pg_dump of unheaded_ops holds no trace of the prompt marker.
#
#   scripts/well/test-llm-requests.sh [--sql-only] [image]   # default postgres:16.15-alpine
#
# --sql-only skips 6 and 7 (no Go toolchain needed).
#
# WELL_MIGRATIONS_DIR (default db/migrations) points the bootstrap at
# another copy of the migrations, so a provocation can be tested.
# Needs docker (sudo if not in the group).
set -euo pipefail

GO=1
if [[ "${1:-}" == --sql-only ]]; then GO=0; shift; fi
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="qa-llm-requests-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
OPS_PW="ops-$RANDOM$RANDOM"
RD_PW="rd-$RANDOM$RANDOM"
cleanup() {
	"${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/db"
cp -a "$SRC" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
"${DOCKER[@]}" run -d --rm --name "$N" -p 127.0.0.1::5432 \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-v "$TMP/db/migrations:/migrations:ro" -v "$TMP/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 120); do
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready"; exit 1; }
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_ops -c "$1"; }
# login ROLE PASSWORD SQL: a real login over TCP, not SET ROLE.
login() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$2" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$1" -d unheaded_ops -c "$3"; }
# Passwords exist only inside this throwaway container.
sq "ALTER ROLE ops_writer PASSWORD '$OPS_PW'; ALTER ROLE ops_reader PASSWORD '$RD_PW';" >/dev/null

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

ROW="INSERT INTO llm_requests (occurred_at, source_service, backend, model_requested, conversation_id, iteration, duration_ms, sent, cost_nano_usd)"

# 1. Ledger and owner.
[[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '030_ops_llm_requests.sql'")" == 1 ]] &&
	pass "bootstrap recorded 030 in unheaded_ops" || fail "030 not in the unheaded_ops ledger"
[[ "$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'llm_requests'::regclass")" == well_ops_owner ]] &&
	pass "llm_requests belongs to well_ops_owner" || fail "llm_requests owner"

# 2. The writer.
ok "ops_writer INSERT" login ops_writer "$OPS_PW" "$ROW VALUES (now(), 'zhen-claude', 'claude', 'claude-opus-5-5', 'c1', 1, 10, true, 1004000)"
ok "ops_writer INSERT (local)" login ops_writer "$OPS_PW" "$ROW VALUES (now(), 'zhen-agentd', 'local', 'qwen-7b', 'c2', 0, 10, true, 0)"
[[ "$(login ops_writer "$OPS_PW" "SELECT count(*) FROM llm_requests")" == 2 ]] && pass "ops_writer SELECT" || fail "ops_writer SELECT"

# 3. Append-only.
refused "ops_writer UPDATE" login ops_writer "$OPS_PW" "UPDATE llm_requests SET cost_nano_usd = 0"
refused "ops_writer DELETE" login ops_writer "$OPS_PW" "DELETE FROM llm_requests"
refused "ops_writer TRUNCATE" login ops_writer "$OPS_PW" "TRUNCATE llm_requests"
refused "owner UPDATE" sq "SET ROLE well_ops_owner; UPDATE llm_requests SET cost_nano_usd = 0"
refused "owner DELETE" sq "SET ROLE well_ops_owner; DELETE FROM llm_requests"
refused "owner TRUNCATE" sq "SET ROLE well_ops_owner; TRUNCATE llm_requests"
[[ "$(sq "SELECT sum(cost_nano_usd) FROM llm_requests")" == 1004000 ]] && pass "rows unchanged" || fail "rows changed"

# 4. CHECKs.
refused "backend 'gpt'" login ops_writer "$OPS_PW" "$ROW VALUES (now(), 'zhen-claude', 'gpt', 'm', 'c3', 1, 1, true, 0)"
refused "negative cost" login ops_writer "$OPS_PW" "$ROW VALUES (now(), 'zhen-claude', 'claude', 'm', 'c3', 1, 1, true, -1)"
refused "unknown source_service" login ops_writer "$OPS_PW" "$ROW VALUES (now(), 'mallory', 'claude', 'm', 'c3', 1, 1, true, 0)"
refused "unknown route_reason" login ops_writer "$OPS_PW" "INSERT INTO llm_requests (occurred_at, source_service, backend, model_requested, conversation_id, iteration, duration_ms, sent, route_reason) VALUES (now(), 'zhen-claude', 'claude', 'm', 'c3', 1, 1, true, 'because')"
refused "empty model" login ops_writer "$OPS_PW" "$ROW VALUES (now(), 'zhen-claude', 'claude', '', 'c3', 1, 1, true, 0)"

# 5. The reader.
ok "ops_reader SELECT" login ops_reader "$RD_PW" "SELECT count(*) FROM llm_requests"
refused "ops_reader INSERT" login ops_reader "$RD_PW" "$ROW VALUES (now(), 'zhen-claude', 'claude', 'm', 'c4', 1, 1, true, 0)"

# 6. Go integration test against this server.
if ((GO)); then
	export WELL_TEST_DSN="postgres://ops_writer:$OPS_PW@127.0.0.1:$PORT/unheaded_ops?sslmode=disable"
	if (cd "$R" && go test -count=1 -tags wellint ./pkg/claudebridge/ -run Well -v >"$TMP/go.out" 2>&1); then
		pass "go test -tags wellint ./pkg/claudebridge -run Well"
	else
		fail "go integration test"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	unset WELL_TEST_DSN
	# 7. The marker was in the prompt and in a tool input; neither is stored.
	"${DOCKER[@]}" exec "$N" pg_dump -U unheaded -d unheaded_ops >"$TMP/dump.sql"
	n=$(grep -c 'S2B-SECRET-MARKER' "$TMP/dump.sql" || true)
	rows=$(sq "SELECT count(*) FROM llm_requests")
	[[ "$n" == 0 && "$rows" -gt 2 ]] && pass "pg_dump: $rows llm_requests rows, 0 hits for the prompt marker" ||
		fail "pg_dump: $n hit(s) for the marker, $rows rows"
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
