#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_ops' deploy history and chaos revert journal
# (db/migrations/039, S-WELL-SPRING Phase 2) on a throwaway PostgreSQL
# server bootstrapped by db/init.sh from the manifest, never the real Well:
#   1. the bootstrap records 039 in unheaded_ops' ledger; the four tables
#      belong to well_ops_owner;
#   2. ops_writer, logged in over TCP, records a deployment with two
#      transitions and journals a fault; deploy_history_current shows the
#      newest status;
#   3. nobody, owner included, can UPDATE, DELETE or TRUNCATE them: a
#      reverted fault is a new transitions row;
#   4. the CHECKs refuse an unknown strategy, status and journal kind;
#   5. ops_reader can read and cannot write;
#   6. well_lint.py has nothing to say about 039;
#   7. the Go side against this server as ops_writer (WELL_TEST_DSN):
#      pkg/database's DeployStore/ChaosStore, Sword's history contract
#      (file and Well) and a rolling deploy recorded in the Well,
#      Yaldabaoth's Well journal, and `chaos-controller revert-all` and
#      `packet-chaos` reverting journaled faults (3 open rows -> 3
#      'reverted' rows, the journal rows untouched).
#
#   scripts/well/test-deploy-journal.sh [--sql-only] [image]   # default postgres:16.15-alpine
#
# --sql-only skips 7 (no Go toolchain needed). WELL_MIGRATIONS_DIR (default
# db/migrations) points the bootstrap at another copy of the migrations.
# Needs docker (sudo if not in the group). Every docker call is capped.
set -euo pipefail

GO=1
if [[ "${1:-}" == --sql-only ]]; then GO=0; shift; fi
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(timeout 60 docker)
timeout 20 docker info >/dev/null 2>&1 || DOCKER=(timeout 60 sudo -n docker)
N="qa-deploy-journal-$$"
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
	# pg_isready answers during init too; the ledger row for 039 is the
	# last thing the bootstrap writes to unheaded_ops.
	if "${DOCKER[@]}" exec "$N" psql -X -q -At -h 127.0.0.1 -U unheaded -d unheaded_ops \
		-c "SELECT 1 FROM well_schema_migrations WHERE filename = '039_ops_deploy_chaos.sql'" 2>/dev/null | grep -q 1; then
		ready=1
		break
	fi
	sleep 1
done
((ready)) || { "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready with 039 applied"; exit 1; }
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

TABLES=(deploy_history deploy_history_transitions chaos_journal chaos_journal_transitions)
declare -A COL=([deploy_history]=host [deploy_history_transitions]=note [chaos_journal]=host [chaos_journal_transitions]=note)

# 1. Ledger and owner.
pass "bootstrap recorded 039 in unheaded_ops"
for t in "${TABLES[@]}"; do
	[[ "$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = '$t'::regclass")" == well_ops_owner ]] &&
		pass "$t belongs to well_ops_owner" || fail "$t owner"
done

# 2. The writer.
DEP="INSERT INTO deploy_history (deployment_id, service, version, strategy, replicas, spec, host, created_at)"
TR="INSERT INTO deploy_history_transitions (deployment_id, from_status, to_status, state)"
CJ="INSERT INTO chaos_journal (entry_id, host, kind, fault_type, target, recipe, created_at, expires_at)"
ok "ops_writer records a deployment" login ops_writer "$OPS_PW" \
	"$DEP VALUES ('d1-1', 'steel-echo', 'v2', 'rolling', 3, '{\"strategy\":{\"type\":\"rolling\"}}', 'west', now())"
ok "ops_writer appends 3 transitions" login ops_writer "$OPS_PW" \
	"$TR VALUES ('d1-1', '', 'pending', '{}'), ('d1-1', 'pending', 'in_progress', '{}'), ('d1-1', 'in_progress', 'succeeded', '{}')"
ok "ops_writer journals a fault" login ops_writer "$OPS_PW" \
	"$CJ VALUES ('chaos-1-1', 'west', 'experiment', 'network_delay', '{\"kind\":\"lxd\"}', '{\"iface\":\"veth1\"}', now(), now() + interval '3 minutes')"
[[ "$(login ops_writer "$OPS_PW" "SELECT status FROM deploy_history_current WHERE deployment_id = 'd1-1'")" == succeeded ]] &&
	pass "deploy_history_current reads the newest transition" || fail "deploy_history_current"
[[ "$(login ops_writer "$OPS_PW" "SELECT count(*) FROM chaos_journal_open")" == 1 ]] &&
	pass "chaos_journal_open lists the fault" || fail "chaos_journal_open before revert"
ok "ops_writer marks the fault reverted" login ops_writer "$OPS_PW" \
	"INSERT INTO chaos_journal_transitions (entry_id, to_status, note) VALUES ('chaos-1-1', 'reverted', 'test')"
[[ "$(login ops_writer "$OPS_PW" "SELECT count(*) FROM chaos_journal_open")" == 0 ]] &&
	pass "a reverted fault leaves chaos_journal_open" || fail "chaos_journal_open after revert"
[[ "$(sq "SELECT count(*) FROM chaos_journal WHERE entry_id = 'chaos-1-1'")" == 1 ]] &&
	pass "the journal row is still there" || fail "journal row gone"

# 3. Append-only, for the writer (no grant) and the owner (triggers).
for t in "${TABLES[@]}"; do
	refused "ops_writer UPDATE $t" login ops_writer "$OPS_PW" "UPDATE $t SET ${COL[$t]} = 'x'"
	refused "ops_writer DELETE $t" login ops_writer "$OPS_PW" "DELETE FROM $t"
	refused "owner UPDATE $t" sq "SET ROLE well_ops_owner; UPDATE $t SET ${COL[$t]} = 'x'"
	refused "owner DELETE $t" sq "SET ROLE well_ops_owner; DELETE FROM $t"
	refused "owner TRUNCATE $t" sq "SET ROLE well_ops_owner; TRUNCATE $t CASCADE"
done

# 4. CHECKs and keys.
refused "strategy 'yolo'" login ops_writer "$OPS_PW" "$DEP VALUES ('d1-2', 'steel-echo', 'v2', 'yolo', 1, '{}', 'west', now())"
refused "replicas 0" login ops_writer "$OPS_PW" "$DEP VALUES ('d1-3', 'steel-echo', 'v2', 'rolling', 0, '{}', 'west', now())"
refused "service 'Bad_Name'" login ops_writer "$OPS_PW" "$DEP VALUES ('d1-4', 'Bad_Name', 'v2', 'rolling', 1, '{}', 'west', now())"
refused "spec not an object" login ops_writer "$OPS_PW" "$DEP VALUES ('d1-5', 'steel-echo', 'v2', 'rolling', 1, '[]', 'west', now())"
refused "duplicate deployment id" login ops_writer "$OPS_PW" "$DEP VALUES ('d1-1', 'steel-echo', 'v3', 'rolling', 1, '{}', 'west', now())"
refused "status 'exploded'" login ops_writer "$OPS_PW" "$TR VALUES ('d1-1', 'succeeded', 'exploded', '{}')"
refused "transition of an unknown deployment" login ops_writer "$OPS_PW" "$TR VALUES ('d-none', '', 'pending', '{}')"
refused "kind 'boom'" login ops_writer "$OPS_PW" "$CJ VALUES ('chaos-1-2', 'west', 'boom', 'x', '{}', '{}', now(), now())"
refused "expires before created" login ops_writer "$OPS_PW" "$CJ VALUES ('chaos-1-3', 'west', 'packet', 'x', '{}', '{}', now(), now() - interval '1 second')"
refused "a second 'reverted' row" login ops_writer "$OPS_PW" "INSERT INTO chaos_journal_transitions (entry_id, to_status) VALUES ('chaos-1-1', 'reverted')"
refused "to_status 'unreverted'" login ops_writer "$OPS_PW" "INSERT INTO chaos_journal_transitions (entry_id, to_status) VALUES ('chaos-1-1', 'unreverted')"

# 5. The reader.
ok "ops_reader SELECT" login ops_reader "$RD_PW" "SELECT count(*) FROM deploy_history_current; SELECT count(*) FROM chaos_journal_open"
refused "ops_reader INSERT" login ops_reader "$RD_PW" "$CJ VALUES ('chaos-1-4', 'west', 'packet', 'x', '{}', '{}', now(), now())"

# 6. The Well lint on the new file.
# The whole directory, as scripts/check-well-lint.sh reads it (some rules are
# cross-file: default privileges live in earlier migrations).
if python3 "$R/scripts/well/well_lint.py" --format json --fail-on NONE "$SRC"/*.sql >"$TMP/lint.json" 2>&1 &&
	python3 -c 'import json,sys; f=[x for x in json.load(open(sys.argv[1]))["findings"] if x["file"].endswith("039_ops_deploy_chaos.sql") and x["severity"] != "INFO"]; [print("   ", x["rule"], x["message"]) for x in f]; sys.exit(1 if f else 0)' "$TMP/lint.json"; then
	pass "well_lint: nothing above INFO on 039"
else
	fail "well_lint findings on 039"
fi

# 7. Go against this server.
if ((GO)); then
	export WELL_TEST_DSN="postgres://ops_writer:$OPS_PW@127.0.0.1:$PORT/unheaded_ops?sslmode=disable"
	gotest() { # label pkg run-regexp
		if (cd "$R" && go test -count=1 -race "$2" -run "$3" -v >"$TMP/go.out" 2>&1); then
			if grep -q -- '--- SKIP' "$TMP/go.out"; then
				fail "$1: skipped"; grep -- '--- SKIP' "$TMP/go.out" | sed 's/^/    /'
			elif ! grep -q -- '--- PASS' "$TMP/go.out"; then
				fail "$1: no test matched $3"
			else
				pass "$1 ($(grep -c -- '--- PASS' "$TMP/go.out") passed)"
			fi
		else
			fail "$1"; tail -30 "$TMP/go.out" | sed 's/^/    /'
		fi
	}
	gotest "pkg/database DeployStore, ChaosStore" ./pkg/database/ 'TestDeployStore|TestChaosStore'
	gotest "sword history contract (file + well)" ./services/sword/ 'TestHistoryContract'
	gotest "sword rolling deploy and file import in the Well" ./services/sword/ 'TestDeploy.*Well|TestWellHistory'
	gotest "yaldabaoth journal in the Well" ./services/yaldabaoth/ 'TestJournal.*Well|TestWellJournal'
	gotest "chaos-controller revert-all and packet-chaos on the Well" ./cmd/chaos-controller/ 'TestRevertAllWell|TestPacketChaosWell'
	unset WELL_TEST_DSN
	rows=$(sq "SELECT count(*) FROM chaos_journal_transitions WHERE note = 'revert-all'")
	[[ "$rows" -ge 3 ]] && pass "revert-all left $rows 'reverted' rows" || fail "revert-all rows: $rows"
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
