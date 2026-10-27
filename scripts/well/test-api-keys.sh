#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_app.api_keys and the app_auth role (db/migrations/035,
# 036; S-IRON-GATE Phase 3, ADR-103 §5) on a throwaway PostgreSQL server
# bootstrapped by db/init.sh from the manifest, never the real Well:
#   1. the bootstrap records 035 (postgres) and 036 (unheaded_app); the
#      table belongs to well_app_owner; app_kanban and app_zhen cannot read
#      it;
#   2. app_auth (a real TCP login) inserts a key record;
#   3. CHECKs: a hash that is not 64 hex, a prefix without uhk_, an unknown
#      role (viewer), an owner with a space, expiry before creation or more
#      than 366 days after it; a duplicate hash;
#   4. only revoked_at and last_used_at change: app_auth has no grant on
#      key_hash/role/expires_at, the owner is refused by the trigger;
#      revoked_at is set once, never cleared or moved;
#   5. DELETE (app_auth, owner) and TRUNCATE are refused;
#   6. --with-go: pkg/auth's Well store suite (build tag wellint) against
#      the same server as app_auth (WELL_TEST_APP_DSN).
#
#   scripts/well/test-api-keys.sh [--with-go] [image]   # default postgres:16.15-alpine
#
# Needs docker (sudo if not in the group).
set -euo pipefail

GO=0
if [[ "${1:-}" == --with-go ]]; then GO=1; shift; fi
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(timeout 120 docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(timeout 120 sudo -n docker)
N="qa-api-keys-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
APW="auth-$RANDOM$RANDOM"
KPW="kanban-$RANDOM$RANDOM"
ZPW="zhen-$RANDOM$RANDOM"
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

sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "${DB:-unheaded_app}" -c "$1"; }
au() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$APW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_auth -d unheaded_app -c "$1"; }
kb() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$KPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_kanban -d unheaded_app -c "$1"; }
zh() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$1"; }
own() { sq "SET ROLE well_app_owner; $1"; }
# The passwords exist only inside this throwaway container.
sq "ALTER ROLE app_auth PASSWORD '$APW'; ALTER ROLE app_kanban PASSWORD '$KPW'; ALTER ROLE app_zhen PASSWORD '$ZPW';" >/dev/null

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
ok() { # label cmd...
	local label=$1; shift
	if "$@" >"$TMP/out" 2>&1; then pass "$label"; else fail "$label"; sed 's/^/    /' "$TMP/out" | tail -4; fi
}
refused() { # label cmd...
	local label=$1; shift
	if "$@" >"$TMP/out" 2>&1; then fail "$label: was allowed"; else pass "$label refused"; fi
}
is() { # label want got
	if [[ "$2" == "$3" ]]; then pass "$1"; else fail "$1: want [$2] got [$3]"; fi
}

hex() { printf '%064x' "$1"; }
uuid() { printf '00000000-0000-4000-8000-%012d' "$1"; }
key() { # n [role] [owner] [expires] [prefix] [hash]
	echo "INSERT INTO api_keys (id, key_hash, prefix, owner, role, expires_at)
	      VALUES ('$(uuid "$1")', '${6:-$(hex "$1")}', '${5:-uhk_aaaa000$(($1 % 10))}', '${3:-ci}', '${2:-service}',
	              ${4:-now() + interval '90 days'})"
}

# 1. Ledger, owner, readers.
is "bootstrap recorded 035 in postgres" 1 \
	"$(DB=postgres sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '035_app_auth_role.sql'")"
is "bootstrap recorded 036 in unheaded_app" 1 \
	"$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '036_app_api_keys.sql'")"
is "api_keys owned by well_app_owner" well_app_owner \
	"$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE relname = 'api_keys'")"
refused "app_kanban SELECT api_keys" kb "SELECT count(*) FROM api_keys"
refused "app_zhen SELECT api_keys" zh "SELECT count(*) FROM api_keys"

# 2. app_auth writes and reads.
ok "app_auth INSERT a key" au "$(key 1)"
is "app_auth reads it back" 1 "$(au "SELECT count(*) FROM api_keys WHERE key_hash = '$(hex 1)'")"

# 3. CHECKs and uniqueness.
refused "hash not 64 hex" au "$(key 2 service ci "now() + interval '1 day'" uhk_aaaa0002 XYZ)"
refused "prefix without uhk_" au "$(key 3 service ci "now() + interval '1 day'" abc_aaaa0003)"
refused "role viewer" au "$(key 4 viewer)"
refused "owner with a space" au "$(key 5 service 'c i')"
refused "expiry before creation" au "$(key 6 service ci "now() - interval '1 day'")"
refused "expiry 400 days out" au "$(key 7 service ci "now() + interval '400 days'")"
ok "expiry 365 days out" au "$(key 8 service ci "now() + interval '365 days'")"
refused "duplicate key_hash" au "$(key 9 service ci "now() + interval '1 day'" uhk_aaaa0009 "$(hex 1)")"
ok "each role" au "$(key 10 admin); $(key 11 operator); $(key 12 observer)"

# 4. What may change.
refused "app_auth UPDATE of role" au "UPDATE api_keys SET role = 'admin' WHERE id = '$(uuid 1)'"
refused "app_auth UPDATE of expires_at" au "UPDATE api_keys SET expires_at = now() + interval '300 days' WHERE id = '$(uuid 1)'"
refused "owner UPDATE of key_hash" own "UPDATE api_keys SET key_hash = '$(hex 99)' WHERE id = '$(uuid 1)'"
refused "owner UPDATE of role" own "UPDATE api_keys SET role = 'admin' WHERE id = '$(uuid 1)'"
ok "app_auth records a use" au "UPDATE api_keys SET last_used_at = now() WHERE id = '$(uuid 1)'"
ok "app_auth revokes" au "UPDATE api_keys SET revoked_at = now() WHERE id = '$(uuid 1)'"
refused "un-revoke" au "UPDATE api_keys SET revoked_at = NULL WHERE id = '$(uuid 1)'"
refused "move a revocation" au "UPDATE api_keys SET revoked_at = now() + interval '1 day' WHERE id = '$(uuid 1)'"
refused "owner un-revoke" own "UPDATE api_keys SET revoked_at = NULL WHERE id = '$(uuid 1)'"
ok "a revoked key still records a use" au "UPDATE api_keys SET last_used_at = now() WHERE id = '$(uuid 1)'"

# 5. Keys are never deleted.
refused "app_auth DELETE" au "DELETE FROM api_keys WHERE id = '$(uuid 8)'"
refused "owner DELETE" own "DELETE FROM api_keys WHERE id = '$(uuid 8)'"
refused "owner TRUNCATE" own "TRUNCATE api_keys"

# 6. The Go Well store against this server.
if ((GO)); then
	export WELL_TEST_APP_DSN="postgres://app_auth:$APW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	if (cd "$R" && go test -count=1 -tags wellint ./pkg/auth/ -run WellStore -v >"$TMP/go.out" 2>&1); then
		pass "go test -tags wellint ./pkg/auth -run WellStore ($(grep -c '^    --- PASS' "$TMP/go.out") subtests)"
	else
		fail "go Well-store test"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	unset WELL_TEST_APP_DSN
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
