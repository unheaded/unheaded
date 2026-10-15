#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Boots a throwaway Well with the deployed configuration (ERDA Phase 4):
# db/postgresql.conf, db/pg_hba.conf, TLS from scripts/well/make-tls.sh,
# data checksums, db/init.sh. Checks that it starts within its settings
# and that pg_hba lets in exactly what it says:
#   - the superuser over TLS from the gateway (the host), nowhere else;
#   - service roles over TLS from the network, never without TLS;
#   - verify-full works with ca.crt for the names in the certificate.
# Its own docker network stands in for `data` (pg_hba's subnet substituted),
# so it never collides with the running stack. Never touches the real Well.
#
#   scripts/well/test-config.sh [image]   # default postgres:16.13-alpine
set -euo pipefail

IMAGE="${1:-postgres:16.13-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
SUDO=(); ((EUID == 0)) || SUDO=(sudo -n)
N="well-config-test-$$" NET="well-config-test-$$"
SUBNET_PREFIX="172.31.$((RANDOM % 200 + 20))"
TMP="$(mktemp -d)"
PW="su-$RANDOM$RANDOM" APW="app-$RANDOM$RANDOM"
cleanup() {
	"${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	"${DOCKER[@]}" network rm "$NET" >/dev/null 2>&1 || true
	"${SUDO[@]}" rm -rf -- "$TMP"
}
trap cleanup EXIT

"${DOCKER[@]}" network create --subnet "$SUBNET_PREFIX.0/24" --gateway "$SUBNET_PREFIX.1" "$NET" >/dev/null
mkdir -p "$TMP/conf"
cp "$R/db/postgresql.conf" "$TMP/conf/postgresql.conf"
sed "s/172\.28\.1\./$SUBNET_PREFIX./g" "$R/db/pg_hba.conf" >"$TMP/conf/pg_hba.conf"
"${SUDO[@]}" "$R/scripts/well/make-tls.sh" "$TMP/tls" >/dev/null

"${DOCKER[@]}" run -d --rm --name "$N" --network "$NET" -p 127.0.0.1::5432 \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-e POSTGRES_INITDB_ARGS=--data-checksums -e APP_KANBAN_PASSWORD="$APW" \
	-v "$R/db/migrations:/migrations:ro" -v "$R/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	-v "$TMP/conf/postgresql.conf:/etc/postgresql/postgresql.conf:ro" \
	-v "$TMP/conf/pg_hba.conf:/etc/postgresql/pg_hba.conf:ro" \
	-v "$TMP/tls:/etc/postgresql/tls:ro" \
	"$IMAGE" postgres -c config_file=/etc/postgresql/postgresql.conf >/dev/null
for _ in $(seq 1 90); do
	# Ready once the entrypoint's temporary server is gone and the real one listens on TCP.
	if "${DOCKER[@]}" exec "$N" pg_isready -h "$("${DOCKER[@]}" inspect -f "{{(index .NetworkSettings.Networks \"$NET\").IPAddress}}" "$N")" -U unheaded -d postgres -q 2>/dev/null; then break; fi
	sleep 1
done
IP="$("${DOCKER[@]}" inspect -f "{{(index .NetworkSettings.Networks \"$NET\").IPAddress}}" "$N")"
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2"; }
# from-network ROLE PASSWORD DB SSLMODE: a client container on the network.
from_net() { "${DOCKER[@]}" run --rm --network "$NET" -e PGPASSWORD="$2" -e PGCONNECT_TIMEOUT=5 "$IMAGE" \
	psql -X -At "host=$IP user=$1 dbname=$3 sslmode=$4" -c "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()" 2>&1; }
# from-host: through the published port, so the server sees the gateway.
from_host() { "${DOCKER[@]}" run --rm --network host -e PGPASSWORD="$2" -e PGCONNECT_TIMEOUT=5 -v "$TMP/tls/ca.crt:/ca.crt:ro" "$IMAGE" \
	psql -X -At "host=${5:-127.0.0.1} port=$PORT user=$1 dbname=$3 sslmode=$4 sslrootcert=/ca.crt" -c "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()" 2>&1; }
ok() { [[ "$2" == t ]] && pass "$1" || fail "$1: $2"; }
refused() { [[ "$2" == *"pg_hba.conf rejects"* || "$2" == *"no pg_hba.conf entry"* ]] && pass "$1 (refused by pg_hba)" || fail "$1: $2"; }

# Settings in effect.
[[ "$(sq postgres "SELECT string_agg(name || '=' || setting, ' ' ORDER BY name) FROM pg_settings WHERE name IN ('ssl', 'data_checksums', 'shared_preload_libraries', 'shared_buffers', 'max_connections', 'listen_addresses', 'statement_timeout')")" == \
	"data_checksums=on listen_addresses=* max_connections=50 shared_buffers=8192 shared_preload_libraries=pg_stat_statements ssl=on statement_timeout=0" ]] &&
	pass "settings: ssl, checksums, preload, 64MB buffers, 50 connections, no global statement_timeout" ||
	fail "settings: $(sq postgres "SELECT string_agg(name || '=' || setting, ' ' ORDER BY name) FROM pg_settings WHERE name IN ('ssl', 'data_checksums', 'shared_preload_libraries', 'shared_buffers', 'max_connections', 'listen_addresses', 'statement_timeout')")"
[[ "$(sq postgres "SELECT count(*) >= 0 FROM pg_stat_statements")" == t ]] && pass "pg_stat_statements readable" || fail "pg_stat_statements"
[[ "$(sq postgres "SELECT array_to_string(setconfig, ',') FROM pg_db_role_setting WHERE setrole = 'well_backup'::regrole")" == "idle_in_transaction_session_timeout=0" ]] &&
	pass "well_backup exempt from the idle-transaction timeout" || fail "well_backup role setting"
[[ "$(sq postgres "SELECT count(*) FROM pg_hba_file_rules WHERE error IS NOT NULL")" == 0 ]] && pass "pg_hba.conf parses" || fail "pg_hba.conf errors"

# Who gets in, and how.
ok "superuser over TLS from the host" "$(from_host unheaded "$PW" postgres require)"
refused "superuser without TLS from the host" "$(from_host unheaded "$PW" postgres disable)"
refused "superuser over TLS from the network" "$(from_net unheaded "$PW" postgres require)"
ok "app_kanban over TLS from the network" "$(from_net app_kanban "$APW" unheaded_app require)"
refused "app_kanban without TLS from the network" "$(from_net app_kanban "$APW" unheaded_app disable)"
ok "app_kanban over TLS from the host" "$(from_host app_kanban "$APW" unheaded_app require)"
ok "verify-full against ca.crt (localhost)" "$(from_host app_kanban "$APW" unheaded_app verify-full localhost)"
out="$(from_net app_kanban "$APW" unheaded_ops require || true)"
[[ "$out" == *"permission denied for database"* ]] && pass "app_kanban cannot connect to unheaded_ops (CONNECT grant)" || fail "app_kanban to ops: $out"
proto="$("${DOCKER[@]}" run --rm --network "$NET" -e PGPASSWORD="$APW" "$IMAGE" psql -X -At "host=$IP user=app_kanban dbname=unheaded_app sslmode=require" -c "SELECT version FROM pg_stat_ssl WHERE pid = pg_backend_pid()" 2>&1)"
[[ "$proto" == TLSv1.3 ]] && pass "TLS 1.3" || fail "protocol: $proto"

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
