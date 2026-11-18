#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Runs pkg/database's TestIntegrationHealthBridge (the health bridge with its
# heartbeat gate, ADR-147 Phase 14) as ops_writer against a throwaway
# PostgreSQL server bootstrapped by db/init.sh from the manifest, never the
# real Well. The same throwaway setup as scripts/well/test-api-keys.sh.
#
#   scripts/well/test-health-bridge.sh [image]   # default postgres:16.15-alpine
#
# Needs docker (sudo if not in the group). Not run before the S-WASTE-OUT
# E-GATE (it starts a container); its Step 321 runs it.
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(timeout 120 docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(timeout 120 sudo -n docker)
N="qa-health-bridge-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
OPW="ops-$RANDOM$RANDOM"
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

# The password exists only inside this throwaway container.
"${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d postgres \
	-c "ALTER ROLE ops_writer PASSWORD '$OPW'" >/dev/null

export WELL_TEST_OPS_DSN="postgres://ops_writer:$OPW@127.0.0.1:$PORT/unheaded_ops?sslmode=disable"
if (cd "$R" && go test -count=1 -run TestIntegrationHealthBridge -v ./pkg/database >"$TMP/go.out" 2>&1) &&
	grep -q '^--- PASS: TestIntegrationHealthBridge' "$TMP/go.out"; then
	echo "[PASS] TestIntegrationHealthBridge against a throwaway server"
else
	echo "[FAIL] TestIntegrationHealthBridge"
	tail -25 "$TMP/go.out" | sed 's/^/    /'
	exit 1
fi
