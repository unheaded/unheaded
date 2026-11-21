#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Checks unheaded_app.bff_sessions, the view bff_sessions_own and the three
# front roles (db/migrations/055, 056; ADR-114 N5) on a throwaway PostgreSQL
# server bootstrapped by db/init.sh from the manifest, never the real Well:
#   1. the bootstrap records 055 (postgres) and 056 (unheaded_app); table,
#      view and trigger function belong to well_app_owner; each front role
#      exists with a connection limit of 3;
#   2. a front (a real TCP login) writes and reads its own session through
#      the view; it cannot read or write the table; another front sees
#      nothing of it; app_kanban and app_zhen see neither table nor view;
#   3. the view holds a front to its own name: an insert naming another
#      front is refused (check option), and updates, revocations and deletes
#      aimed at another front's row change nothing;
#   4. CHECKs: a hash that is not 64 hex, a front with an underscore, a
#      subject with a space or a newline, an upper-case role, an expiry past the maximum
#      age, a maximum age over 31 days, an expiry at creation;
#   5. only last_used_at, expires_at and revoked_at with revoked_by change:
#      a front has no grant on the rest and the trigger refuses the owner; a
#      revocation is set once, with what did it, and a revoked row never
#      changes; last_used_at does not go back; a front cannot revoke in the
#      command line's name;
#   6. app_auth reads the table and sets revoked_at with revoked_by, and
#      nothing else: no insert, no delete, no change of an expiry;
#   7. a front deletes its own ended rows (the sweep);
#   8. --with-go: pkg/bff/sessions' store suite (build tag wellint) against
#      the same server, each front as its own role and the Admin as
#      app_auth.
#
#   scripts/well/test-bff-sessions.sh [--with-go] [image]   # default postgres:16.15-alpine
#
# Needs docker (sudo if not in the group).
set -euo pipefail

GO=0
if [[ "${1:-}" == --with-go ]]; then GO=1; shift; fi
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(timeout 120 docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(timeout 120 sudo -n docker)
N="qa-bff-sessions-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
DPW="dash-$RANDOM$RANDOM"
BPW="board-$RANDOM$RANDOM"
WPW="wiki-$RANDOM$RANDOM"
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
as() { # role password sql
	"${DOCKER[@]}" exec -i -e PGPASSWORD="$2" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$1" -d unheaded_app -c "$3"
}
dash() { as bff_dashboard_backend "$DPW" "$1"; }
board() { as bff_kanban_app "$BPW" "$1"; }
wiki() { as bff_wiki_server "$WPW" "$1"; }
au() { as app_auth "$APW" "$1"; }
kb() { as app_kanban "$KPW" "$1"; }
zh() { as app_zhen "$ZPW" "$1"; }
own() { sq "SET ROLE well_app_owner; $1"; }
# The passwords exist only inside this throwaway container.
sq "ALTER ROLE bff_dashboard_backend PASSWORD '$DPW'; ALTER ROLE bff_kanban_app PASSWORD '$BPW'; ALTER ROLE bff_wiki_server PASSWORD '$WPW';
    ALTER ROLE app_auth PASSWORD '$APW'; ALTER ROLE app_kanban PASSWORD '$KPW'; ALTER ROLE app_zhen PASSWORD '$ZPW';" >/dev/null

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
# row n [front] [subject] [roles] [expires] [absolute] [hash] [target]; an
# empty argument takes its default, and roles "none" is the empty list.
row() {
	local roles="${4:-operator,viewer}"
	[[ "$roles" == none ]] && roles=""
	echo "INSERT INTO ${8:-bff_sessions_own} (id_hash, front, subject, roles, created_at, last_used_at, expires_at, absolute_expires_at)
	      VALUES ('${7:-$(hex "$1")}', '${2:-dashboard-backend}', '${3:-dev}', '$roles', now(), now(),
	              ${5:-now() + interval '30 minutes'}, ${6:-now() + interval '12 hours'})"
}

# 1. Ledger, owners, roles.
is "bootstrap recorded 055 in postgres" 1 \
	"$(DB=postgres sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '055_bff_session_roles.sql'")"
is "bootstrap recorded 056 in unheaded_app" 1 \
	"$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '056_app_bff_sessions.sql'")"
is "bff_sessions owned by well_app_owner" well_app_owner \
	"$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE relname = 'bff_sessions'")"
is "bff_sessions_own owned by well_app_owner" well_app_owner \
	"$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE relname = 'bff_sessions_own'")"
is "the view is a security barrier with a cascaded check option" "check_option=cascaded,security_barrier=true" \
	"$(sq "SELECT string_agg(o, ',' ORDER BY o) FROM pg_class, unnest(reloptions) o WHERE relname = 'bff_sessions_own'")"
is "three front roles, each limited to 3 connections" "bff_dashboard_backend:3,bff_kanban_app:3,bff_wiki_server:3" \
	"$(DB=postgres sq "SELECT string_agg(rolname || ':' || rolconnlimit, ',' ORDER BY rolname) FROM pg_roles WHERE rolname LIKE 'bff\_%'")"
is "no front role is a member of anything or a superuser" 0 \
	"$(DB=postgres sq "SELECT count(*) FROM pg_roles r WHERE rolname LIKE 'bff\_%' AND (rolsuper OR rolcreaterole OR rolbypassrls OR EXISTS (SELECT 1 FROM pg_auth_members m WHERE m.member = r.oid))")"

# 2. A front writes and reads its own; nobody else does.
ok "dashboard INSERT its own session" dash "$(row 1)"
is "dashboard reads it back" 1 "$(dash "SELECT count(*) FROM bff_sessions_own WHERE id_hash = '$(hex 1)'")"
refused "dashboard SELECT the table" dash "SELECT count(*) FROM bff_sessions"
refused "dashboard INSERT into the table" dash "$(row 2 dashboard-backend dev operator '' '' '' bff_sessions)"
is "the board sees none of the dashboard's" 0 "$(board "SELECT count(*) FROM bff_sessions_own")"
is "the wiki sees none of the dashboard's" 0 "$(wiki "SELECT count(*) FROM bff_sessions_own")"
refused "app_kanban SELECT the table" kb "SELECT count(*) FROM bff_sessions"
refused "app_kanban SELECT the view" kb "SELECT count(*) FROM bff_sessions_own"
refused "app_zhen SELECT the view" zh "SELECT count(*) FROM bff_sessions_own"
is "the owner sees nothing through the view (it is no front)" 0 "$(own "SELECT count(*) FROM bff_sessions_own" | tail -1)"

# 3. The view holds a front to its own name.
refused "the board INSERT a session naming the dashboard" board "$(row 3 dashboard-backend admin admin)"
is "nothing was written by it" 0 "$(sq "SELECT count(*) FROM bff_sessions WHERE id_hash = '$(hex 3)'")"
ok "the board INSERT its own session" board "$(row 4 kanban-app)"
board "UPDATE bff_sessions_own SET revoked_at = now(), revoked_by = 'logout' WHERE id_hash = '$(hex 1)'" >/dev/null
board "UPDATE bff_sessions_own SET expires_at = now() + interval '1 hour' WHERE id_hash = '$(hex 1)'" >/dev/null
board "DELETE FROM bff_sessions_own WHERE id_hash = '$(hex 1)'" >/dev/null
is "the board's update, revocation and delete of the dashboard's row changed nothing" "1 t" \
	"$(sq "SELECT count(*), bool_and(revoked_at IS NULL AND expires_at < now() + interval '31 minutes') FROM bff_sessions WHERE id_hash = '$(hex 1)'" | tr '|' ' ')"
refused "a front name with an underscore cannot be smuggled past the role match" own \
	"$(row 5 dashboard_backend dev operator '' '' '' bff_sessions)"

# 4. CHECKs.
refused "hash not 64 hex" dash "$(row 10 dashboard-backend dev operator '' '' XYZ)"
refused "upper-case hash" dash "$(row 10 dashboard-backend dev operator '' '' "$(printf 'A%063x' 10)")"
refused "subject with a space" dash "$(row 11 dashboard-backend 'de v')"
refused "subject with a newline" dash "$(row 12 dashboard-backend 'dev
admin')"
refused "upper-case role" dash "$(row 13 dashboard-backend dev Admin)"
refused "role list with a space" dash "$(row 14 dashboard-backend dev 'operator, viewer')"
ok "no roles at all" dash "$(row 15 dashboard-backend dev none)"
refused "expiry past the maximum age" dash "$(row 16 dashboard-backend dev operator "now() + interval '13 hours'")"
refused "maximum age over 31 days" dash "$(row 17 dashboard-backend dev operator '' "now() + interval '32 days'")"
ok "maximum age of 31 days" dash "$(row 18 dashboard-backend dev operator '' "now() + interval '31 days'")"
refused "expiry at creation" dash "$(row 19 dashboard-backend dev operator 'now()')"
refused "duplicate hash" dash "$(row 1)"

# 5. What may change.
refused "front UPDATE of subject (no grant)" dash "UPDATE bff_sessions_own SET subject = 'admin' WHERE id_hash = '$(hex 1)'"
refused "front UPDATE of roles (no grant)" dash "UPDATE bff_sessions_own SET roles = 'admin' WHERE id_hash = '$(hex 1)'"
refused "front UPDATE of the maximum age (no grant)" dash "UPDATE bff_sessions_own SET absolute_expires_at = now() + interval '30 days' WHERE id_hash = '$(hex 1)'"
refused "owner UPDATE of roles (trigger)" own "UPDATE bff_sessions SET roles = 'admin' WHERE id_hash = '$(hex 1)'"
refused "owner UPDATE of front (trigger)" own "UPDATE bff_sessions SET front = 'kanban-app' WHERE id_hash = '$(hex 1)'"
refused "owner UPDATE of the maximum age (trigger)" own "UPDATE bff_sessions SET absolute_expires_at = now() + interval '30 days' WHERE id_hash = '$(hex 1)'"
ok "front records a use" dash "UPDATE bff_sessions_own SET last_used_at = now(), expires_at = LEAST(now() + interval '30 minutes', absolute_expires_at) WHERE id_hash = '$(hex 1)'"
refused "front extends past the maximum age" dash "UPDATE bff_sessions_own SET expires_at = absolute_expires_at + interval '1 second' WHERE id_hash = '$(hex 1)'"
refused "last_used_at moves backwards" dash "UPDATE bff_sessions_own SET last_used_at = created_at WHERE id_hash = '$(hex 1)' AND last_used_at > created_at"
refused "a revocation without what did it" dash "UPDATE bff_sessions_own SET revoked_at = now() WHERE id_hash = '$(hex 1)'"
refused "an actor without a revocation" dash "UPDATE bff_sessions_own SET revoked_by = 'logout' WHERE id_hash = '$(hex 1)'"
refused "an actor with a space" dash "UPDATE bff_sessions_own SET revoked_at = now(), revoked_by = 'log out' WHERE id_hash = '$(hex 1)'"
refused "a front revoking in the command line's name" dash "UPDATE bff_sessions_own SET revoked_at = now(), revoked_by = 'cli:root@west' WHERE id_hash = '$(hex 1)'"
ok "front revokes (logout)" dash "UPDATE bff_sessions_own SET revoked_at = now(), revoked_by = 'logout' WHERE id_hash = '$(hex 1)'"
is "the row says what revoked it" logout "$(sq "SELECT revoked_by FROM bff_sessions WHERE id_hash = '$(hex 1)'")"
refused "un-revoke" dash "UPDATE bff_sessions_own SET revoked_at = NULL, revoked_by = NULL WHERE id_hash = '$(hex 1)'"
refused "change what revoked it" dash "UPDATE bff_sessions_own SET revoked_by = 'limit' WHERE id_hash = '$(hex 1)'"
refused "move a revocation" dash "UPDATE bff_sessions_own SET revoked_at = now() + interval '1 hour' WHERE id_hash = '$(hex 1)'"
refused "a revoked session records no use" dash "UPDATE bff_sessions_own SET last_used_at = now() + interval '1 second' WHERE id_hash = '$(hex 1)'"
refused "owner un-revoke" own "UPDATE bff_sessions SET revoked_at = NULL, revoked_by = NULL WHERE id_hash = '$(hex 1)'"

# 6. app_auth: read and revoke.
is "app_auth lists every front's sessions" t "$(au "SELECT count(DISTINCT front) >= 2 FROM bff_sessions")"
refused "app_auth revokes without saying who" au "UPDATE bff_sessions SET revoked_at = GREATEST(now(), created_at) WHERE id_hash = '$(hex 4)' AND revoked_at IS NULL"
ok "app_auth revokes the board's session" au "UPDATE bff_sessions SET revoked_at = GREATEST(now(), created_at), revoked_by = 'cli:tester@host' WHERE id_hash = '$(hex 4)' AND revoked_at IS NULL"
is "the row names the command line's user" cli:tester@host "$(sq "SELECT revoked_by FROM bff_sessions WHERE id_hash = '$(hex 4)'")"
is "the board sees its session revoked" t "$(board "SELECT revoked_at IS NOT NULL FROM bff_sessions_own WHERE id_hash = '$(hex 4)'")"
refused "app_auth INSERT a session" au "$(row 20 dashboard-backend admin admin '' '' '' bff_sessions)"
refused "app_auth DELETE" au "DELETE FROM bff_sessions WHERE id_hash = '$(hex 15)'"
refused "app_auth UPDATE of an expiry" au "UPDATE bff_sessions SET expires_at = now() + interval '1 hour' WHERE id_hash = '$(hex 15)'"
refused "app_auth UPDATE of roles" au "UPDATE bff_sessions SET roles = 'admin' WHERE id_hash = '$(hex 15)'"
refused "app_auth through the view" au "SELECT count(*) FROM bff_sessions_own"

# 7. The sweep: a front deletes its own rows.
before="$(sq "SELECT count(*) FROM bff_sessions WHERE front = 'dashboard-backend'")"
ok "front DELETE its revoked session" dash "DELETE FROM bff_sessions_own WHERE id_hash IN (SELECT id_hash FROM bff_sessions_own WHERE revoked_at < now() + interval '1 second' ORDER BY expires_at LIMIT 10)"
is "one row went" "$((before - 1))" "$(sq "SELECT count(*) FROM bff_sessions WHERE front = 'dashboard-backend'")"
is "the board's rows are untouched" 1 "$(sq "SELECT count(*) FROM bff_sessions WHERE front = 'kanban-app'")"

# 8. The Go Well store against this server.
if ((GO)); then
	export WELL_TEST_SESSION_DSN_A="postgres://bff_dashboard_backend:$DPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	export WELL_TEST_SESSION_DSN_B="postgres://bff_kanban_app:$BPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	export WELL_TEST_SESSION_ADMIN_DSN="postgres://app_auth:$APW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	if (cd "$R" && go test -count=1 -tags wellint ./pkg/bff/sessions/ -run 'Well' -v >"$TMP/go.out" 2>&1); then
		pass "go test -tags wellint ./pkg/bff/sessions -run Well ($(grep -c '^    --- PASS' "$TMP/go.out") subtests)"
	else
		fail "go Well-store test"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	unset WELL_TEST_SESSION_DSN_A WELL_TEST_SESSION_DSN_B WELL_TEST_SESSION_ADMIN_DSN
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
