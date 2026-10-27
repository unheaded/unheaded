#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_ops.slo_definitions (db/migrations/040) on a throwaway
# PostgreSQL server bootstrapped by db/init.sh from the manifest, never the
# real Well:
#   1. the bootstrap records 040 in unheaded_ops' ledger; both tables
#      belong to well_ops_owner;
#   2. ops_writer, logged in over TCP, inserts one row per SLO named in
#      observability/alerting/policy.yaml (the fixture), and each INSERT
#      writes one history row;
#   3. an UPDATE of an objective writes one history row with the old and
#      new value and the writer's change reason; an UPDATE that changes
#      nothing writes none; a DELETE writes one;
#   4. the CHECKs refuse objective 1.0 (and 0), an unknown urgency, a bad
#      name and a short sha256;
#   5. the history is append-only for ops_writer and the owner, and
#      slo_definitions cannot be TRUNCATEd (it would leave no history);
#   6. ops_reader can read both tables and write neither;
#   7. with Go: go test ./pkg/database -run TestSLOStore, then ./cmd/alert-policy -run TestWell,
#      against this server (WELL_TEST_DSN): -sync-well, then the drift leg
#      of -check passes, then fails after a row is changed behind its back;
#      afterwards every SLO row carries the policy file's sha256.
#
#   scripts/well/test-slo-definitions.sh [--sql-only] [image]   # default postgres:16.15-alpine
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
DOCKER=(docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="qa-slo-definitions-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
OPS_PW="ops-$RANDOM$RANDOM"
RD_PW="rd-$RANDOM$RANDOM"
cleanup() {
	timeout 60 "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/db"
cp -a "$SRC" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
timeout 120 "${DOCKER[@]}" run -d --rm --name "$N" -p 127.0.0.1::5432 \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-v "$TMP/db/migrations:/migrations:ro" -v "$TMP/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 120); do
	if timeout 10 "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { timeout 20 "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready"; exit 1; }
PORT="$(timeout 20 "${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

sq() { timeout 60 "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_ops -c "$1"; }
# login ROLE PASSWORD SQL: a real login over TCP, not SET ROLE.
login() { timeout 60 "${DOCKER[@]}" exec -i -e PGPASSWORD="$2" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$1" -d unheaded_ops -c "$3"; }
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

SHA="$(sha256sum "$R/observability/alerting/policy.yaml" | cut -c1-64)"
COLS="INSERT INTO slo_definitions (name, service, description, objective, slo_window, sli_good_expr, urgency, runbook_url, source_sha256)"
BOOK="https://github.com/unheaded/unheaded/blob/develop/runbooks/observe/slo-error-budget-burn.yaml"
# The fixture: one row per SLO the policy names (alert-policy -sync-well
# writes the real values in step 7).
mapfile -t NAMES < <(sed -n 's/^  - name: \([a-z0-9-]*\)$/\1/p' "$R/observability/alerting/policy.yaml")
((${#NAMES[@]} >= 7)) || { echo "fixture: only ${#NAMES[@]} SLO names in policy.yaml"; exit 1; }
LAST="${NAMES[${#NAMES[@]}-1]}"

# 1. Ledger and owner.
[[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '040_ops_slo_definitions.sql'")" == 1 ]] &&
	pass "bootstrap recorded 040 in unheaded_ops" || fail "040 not in the unheaded_ops ledger"
for t in slo_definitions slo_definitions_history; do
	[[ "$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = '$t'::regclass")" == well_ops_owner ]] &&
		pass "$t belongs to well_ops_owner" || fail "$t owner"
done

# 2. The writer inserts every SLO; each INSERT is one history row.
for n in "${NAMES[@]}"; do
	login ops_writer "$OPS_PW" "$COLS VALUES ('$n', 'svc-$n', 'fixture $n', 0.995, '28 days', 'avg(up{job=\"$n\"})', 'page', '$BOOK', '$SHA')" >"$TMP/out" 2>&1 ||
		{ fail "ops_writer INSERT $n"; sed 's/^/    /' "$TMP/out" | tail -3; }
done
[[ "$(login ops_writer "$OPS_PW" "SELECT count(*) FROM slo_definitions")" == "${#NAMES[@]}" ]] &&
	pass "ops_writer inserted ${#NAMES[@]} SLOs" || fail "row count"
[[ "$(sq "SELECT count(*) FROM slo_definitions_history WHERE op = 'INSERT' AND changed_by = 'ops_writer'")" == "${#NAMES[@]}" ]] &&
	pass "one INSERT history row per SLO, changed_by ops_writer" || fail "INSERT history: $(sq "SELECT count(*), min(changed_by) FROM slo_definitions_history")"

# 3. UPDATE, a no-op UPDATE, DELETE.
ok "ops_writer UPDATE objective with a reason" login ops_writer "$OPS_PW" \
	"BEGIN; SET LOCAL well.change_reason = 'tighten wotan'; UPDATE slo_definitions SET objective = 0.999, updated_at = now() WHERE name = 'wotan'; COMMIT;"
got="$(sq "SELECT old_row->>'objective', new_row->>'objective', change_reason FROM slo_definitions_history WHERE op = 'UPDATE' AND name = 'wotan'")"
[[ "$got" == "0.9950|0.9990|tighten wotan" ]] && pass "UPDATE wrote history: $got" || fail "UPDATE history: [$got]"
ok "ops_writer no-op UPDATE" login ops_writer "$OPS_PW" "UPDATE slo_definitions SET objective = objective WHERE name = 'wotan'"
[[ "$(sq "SELECT count(*) FROM slo_definitions_history WHERE op = 'UPDATE'")" == 1 ]] &&
	pass "an UPDATE that changes nothing writes no history" || fail "no-op UPDATE wrote history"
ok "ops_writer DELETE" login ops_writer "$OPS_PW" "DELETE FROM slo_definitions WHERE name = '$LAST'"
[[ "$(sq "SELECT count(*) FROM slo_definitions_history WHERE op = 'DELETE' AND new_row IS NULL AND old_row->>'name' = '$LAST'")" == 1 ]] &&
	pass "DELETE wrote history" || fail "DELETE history"

# 4. CHECKs.
refused "objective 1.0" login ops_writer "$OPS_PW" "UPDATE slo_definitions SET objective = 1.0 WHERE name = 'wotan'"
refused "objective 0" login ops_writer "$OPS_PW" "UPDATE slo_definitions SET objective = 0 WHERE name = 'wotan'"
refused "urgency 'sometimes'" login ops_writer "$OPS_PW" "UPDATE slo_definitions SET urgency = 'sometimes' WHERE name = 'wotan'"
refused "name 'Bad Name'" login ops_writer "$OPS_PW" "$COLS VALUES ('Bad Name', 's', 'd', 0.99, '28 days', 'x', 'page', '$BOOK', '$SHA')"
refused "short sha256" login ops_writer "$OPS_PW" "$COLS VALUES ('short-sha', 's', 'd', 0.99, '28 days', 'x', 'page', '$BOOK', 'abc')"
refused "empty sli_total_expr" login ops_writer "$OPS_PW" "UPDATE slo_definitions SET sli_total_expr = '' WHERE name = 'wotan'"
[[ "$(sq "SELECT objective FROM slo_definitions WHERE name = 'wotan'")" == 0.9990 ]] && pass "refused writes changed nothing" || fail "row changed"

# 5. Append-only history; no TRUNCATE.
refused "ops_writer UPDATE history" login ops_writer "$OPS_PW" "UPDATE slo_definitions_history SET change_reason = 'x'"
refused "ops_writer DELETE history" login ops_writer "$OPS_PW" "DELETE FROM slo_definitions_history"
refused "ops_writer INSERT history" login ops_writer "$OPS_PW" "INSERT INTO slo_definitions_history (name, op, changed_by) VALUES ('wotan', 'INSERT', 'mallory')"
refused "owner UPDATE history" sq "SET ROLE well_ops_owner; UPDATE slo_definitions_history SET change_reason = 'x'"
refused "owner DELETE history" sq "SET ROLE well_ops_owner; DELETE FROM slo_definitions_history"
refused "owner TRUNCATE history" sq "SET ROLE well_ops_owner; TRUNCATE slo_definitions_history"
refused "owner TRUNCATE slo_definitions" sq "SET ROLE well_ops_owner; TRUNCATE slo_definitions"
refused "ops_writer TRUNCATE slo_definitions" login ops_writer "$OPS_PW" "TRUNCATE slo_definitions"
refused "ops_writer calls the trigger function" login ops_writer "$OPS_PW" "SELECT slo_definitions_track()"

# 6. The reader.
ok "ops_reader SELECT" login ops_reader "$RD_PW" "SELECT count(*) FROM slo_definitions, slo_definitions_history"
refused "ops_reader UPDATE" login ops_reader "$RD_PW" "UPDATE slo_definitions SET objective = 0.99"
refused "ops_reader INSERT" login ops_reader "$RD_PW" "$COLS VALUES ('reader', 's', 'd', 0.99, '28 days', 'x', 'page', '$BOOK', '$SHA')"

# 7. Go against this server.
if ((GO)); then
	export WELL_TEST_DSN="postgres://ops_writer:$OPS_PW@127.0.0.1:$PORT/unheaded_ops?sslmode=disable"
	# One package at a time: both replace the whole table. alert-policy last,
	# so the rows left behind are the policy's.
	if (cd "$R" && go test -count=1 ./pkg/database/ -run 'TestSLOStore' -v >"$TMP/go.out" 2>&1 &&
		go test -count=1 ./cmd/alert-policy/ -run 'TestWell' -v >>"$TMP/go.out" 2>&1) &&
		! grep -q -- '--- SKIP' "$TMP/go.out"; then
		pass "go test ./pkg/database -run TestSLOStore; ./cmd/alert-policy -run TestWell (none skipped)"
	else
		fail "go integration tests"; grep -E -- '--- (FAIL|SKIP)|_test.go' "$TMP/go.out" | head -20 | sed 's/^/    /'
	fi
	unset WELL_TEST_DSN
	rows="$(sq "SELECT count(*) FROM slo_definitions WHERE source_sha256 = '$SHA'")"
	[[ "$rows" == "${#NAMES[@]}" ]] && pass "after -sync-well: ${#NAMES[@]} rows carry the policy's sha256" ||
		fail "after sync: $rows rows with the policy sha256, want ${#NAMES[@]}"
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
