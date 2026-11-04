#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_app.zhen_conversations.client (db/migrations/045) on a
# throwaway PostgreSQL server bootstrapped by db/init.sh from the manifest,
# never the real Well:
#   1. the bootstrap records 045;
#   2. a row without client reads web; app_zhen inserts a cli row;
#   3. an unknown client is refused.
#
#   scripts/well/test-zhen-conversations.sh [image]   # default postgres:16.15-alpine
#
# WELL_MIGRATIONS_DIR (default db/migrations) points the bootstrap at
# another copy of the migrations, so a provocation can be tested.
# Needs docker as the normal user; it never uses sudo.
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(docker)
N="qa-zhen-conversations-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
ZPW="zhen-$RANDOM$RANDOM"
KPW="kanban-$RANDOM$RANDOM"
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

sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app -c "$1"; }
zh() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$1"; }
kb() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$KPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_kanban -d unheaded_app -c "$1"; }
own() { sq "SET ROLE well_app_owner; $1"; }
# The passwords exist only inside this throwaway container.
sq "ALTER ROLE app_zhen PASSWORD '$ZPW'; ALTER ROLE app_kanban PASSWORD '$KPW';" >/dev/null

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

[[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '045_app_zhen_conversations_client.sql'")" == 1 ]] &&
	pass "ledger records 045" || fail "ledger records 045"
zh "INSERT INTO zhen_conversations (session_id, role, content) VALUES ('s1', 'user', 'hello')" &&
	[[ "$(sq "SELECT client FROM zhen_conversations WHERE session_id = 's1'")" == web ]] &&
	pass "default client is web" || fail "default client"
zh "INSERT INTO zhen_conversations (session_id, role, content, client) VALUES ('s2', 'user', 'hi', 'cli')" &&
	[[ "$(sq "SELECT client FROM zhen_conversations WHERE session_id = 's2'")" == cli ]] &&
	pass "app_zhen writes a cli row" || fail "cli row"
if zh "INSERT INTO zhen_conversations (session_id, role, content, client) VALUES ('s3', 'user', 'x', 'gui')" 2>/dev/null; then
	fail "unknown client was allowed"
else
	pass "unknown client refused"
fi
((fails == 0)) || { echo "$fails check(s) failed"; exit 1; }
echo "all checks passed"
