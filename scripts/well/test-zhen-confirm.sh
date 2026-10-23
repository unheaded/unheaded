#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_app.zhen_pending_confirmations (db/migrations/031) on a
# throwaway PostgreSQL server bootstrapped by db/init.sh from the
# manifest, never the real Well:
#   1. the bootstrap records 031; the table belongs to well_app_owner;
#   2. app_zhen (a real TCP login) inserts a pending row;
#      pending -> approved -> executed works, and so does pending -> denied;
#   3. every other transition is refused: approved -> pending,
#      executed -> approved, denied -> approved, a row born approved;
#   4. the call is immutable (owner included); app_zhen cannot even name
#      another column in an UPDATE; DELETE and TRUNCATE are refused;
#   5. CHECKs: an unconfirmable tool (runbook_execute), a bad id, a
#      relative project root;
#   6. an approval after expires_at is refused;
#   7. at most 20 pending rows per source_service.
#   8. cmd/zhen-claude's queue store (build tag wellint) against the same
#      server as app_zhen (WELL_TEST_APP_DSN).
#
#   scripts/well/test-zhen-confirm.sh [--sql-only] [image]   # default postgres:16.15-alpine
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
N="qa-zhen-confirm-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
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

sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app -c "$1"; }
zh() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$1"; }
own() { sq "SET ROLE well_app_owner; $1"; }
# The password exists only inside this throwaway container.
sq "ALTER ROLE app_zhen PASSWORD '$ZPW';" >/dev/null

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

H64=$(printf 'a%.0s' $(seq 1 64))
row() { # id [tool] [expires] [source] [status]
	echo "INSERT INTO zhen_pending_confirmations (id, expires_at, source_service, emitted_by, project_root, tool, call, call_sha256, summary${5:+, status})
	      VALUES ('$1', ${3:-now() + interval '1 hour'}, '${4:-zhen-claude}', 'claude-api/m', '/home/x/repo', '${2:-write_file}',
	              '{\"path\":\"/home/x/repo/a\",\"content\":\"hi\"}', '$H64', 'write a'${5:+, '$5'})"
}
setst() { echo "UPDATE zhen_pending_confirmations SET status = '$2', decided_at = now(), decided_by = 'stevie@west' WHERE id = '$1'"; }

# 1. Ledger and owner.
[[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '031_app_zhen_pending_confirmations.sql'")" == 1 ]] &&
	pass "bootstrap recorded 031 in unheaded_app" || fail "031 not in the ledger"
[[ "$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'zhen_pending_confirmations'::regclass")" == well_app_owner ]] &&
	pass "owned by well_app_owner" || fail "owner"

# 2. The happy paths, as app_zhen.
ok "app_zhen INSERT pending" zh "$(row 0000000000000001)"
ok "pending -> approved" zh "$(setst 0000000000000001 approved)"
ok "approved -> executed" zh "UPDATE zhen_pending_confirmations SET status = 'executed' WHERE id = '0000000000000001'"
ok "INSERT a second" zh "$(row 0000000000000002)"
ok "pending -> denied" zh "$(setst 0000000000000002 denied)"
ok "INSERT a third" zh "$(row 0000000000000003)"
ok "pending -> approved (third)" zh "$(setst 0000000000000003 approved)"

# 3. Every other transition.
refused "approved -> pending" zh "UPDATE zhen_pending_confirmations SET status = 'pending', decided_at = NULL, decided_by = NULL WHERE id = '0000000000000003'"
refused "executed -> approved" zh "UPDATE zhen_pending_confirmations SET status = 'approved' WHERE id = '0000000000000001'"
refused "denied -> approved" zh "UPDATE zhen_pending_confirmations SET status = 'approved' WHERE id = '0000000000000002'"
refused "approved -> approved (re-decide)" zh "UPDATE zhen_pending_confirmations SET decided_by = 'mallory' WHERE id = '0000000000000003'"
refused "a row born approved" zh "$(row 0000000000000004 write_file "now() + interval '1 hour'" zhen-claude approved)"

# 4. The call is immutable; no deletes.
refused "app_zhen UPDATE of call" zh "UPDATE zhen_pending_confirmations SET call = '{}' WHERE id = '0000000000000003'"
refused "owner UPDATE of call" own "UPDATE zhen_pending_confirmations SET status = 'executed', call = '{\"path\":\"/etc/passwd\"}' WHERE id = '0000000000000003'"
refused "owner UPDATE of project_root" own "UPDATE zhen_pending_confirmations SET status = 'executed', project_root = '/' WHERE id = '0000000000000003'"
refused "app_zhen DELETE" zh "DELETE FROM zhen_pending_confirmations"
refused "owner DELETE" own "DELETE FROM zhen_pending_confirmations"
refused "owner TRUNCATE" own "TRUNCATE zhen_pending_confirmations"

# 5. CHECKs.
refused "tool runbook_execute" zh "$(row 0000000000000005 runbook_execute)"
refused "id not 16 hex" zh "$(row 'XYZ')"
refused "relative project root" zh "$(row 0000000000000006 | sed "s|'/home/x/repo'|'repo'|")"

# 6. Expiry.
ok "INSERT one that expires in 1 s" zh "$(row 0000000000000007 write_file "now() + interval '1 second'")"
sleep 2
refused "approval after expiry" zh "$(setst 0000000000000007 approved)"
ok "pending -> expired" zh "$(setst 0000000000000007 expired)"

# 7. The cap: 20 pending per source_service.
for i in $(seq 1 20); do
	zh "$(row "$(printf 'b%015x' "$i")" write_file "now() + interval '1 hour'" zhen-agentd)" >/dev/null || fail "pending row $i refused below the cap"
done
refused "21st pending for zhen-agentd" zh "$(row b000000000000099 write_file "now() + interval '1 hour'" zhen-agentd)"
ok "zhen-claude has its own cap" zh "$(row c000000000000001)"

# 8. The Go queue store against this server.
if ((GO)); then
	export WELL_TEST_APP_DSN="postgres://app_zhen:$ZPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	if (cd "$R" && go test -count=1 -tags wellint ./cmd/zhen-claude/ -run Well -v >"$TMP/go.out" 2>&1); then
		pass "go test -tags wellint ./cmd/zhen-claude -run Well"
	else
		fail "go queue-store test"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	unset WELL_TEST_APP_DSN
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
