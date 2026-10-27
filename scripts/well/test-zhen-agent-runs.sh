#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_app.zhen_agent_runs (db/migrations/032) on a throwaway
# PostgreSQL server bootstrapped by db/init.sh from the manifest, never
# the real Well:
#   1. the bootstrap records 032; the table belongs to well_app_owner;
#   2. a claim race: eight app_zhen sessions INSERT a claim for one card
#      at once, exactly one wins; a second card is unaffected;
#   3. claimed -> running -> succeeded -> accepted works, and so do
#      claimed -> abandoned, running -> failed; every other move is
#      refused (claimed -> succeeded, failed -> running, accepted ->
#      rejected, succeeded -> running, a run born running);
#   4. identity columns are immutable (owner included); a finished run's
#      outcome is fixed; a review is recorded only with accept/reject;
#      DELETE and TRUNCATE are refused;
#   5. after a run ends, the card can be claimed again;
#   6. CHECKs: branch zhen/../x, branch main, bad id, bad family;
#   7. app_zhen moves a card (status, commits) and takes FOR UPDATE SKIP
#      LOCKED on it, but cannot change its title or delete it;
#   8. pkg/zhenagent's SQL store (build tag wellint) against the same
#      server as app_zhen (WELL_TEST_APP_DSN; cards created as app_kanban,
#      WELL_TEST_KANBAN_DSN); a skipped Go test counts as a failure;
#   9. scheduled runs (037): born without a branch or base commit, finish
#      succeeded or failed, never take a head commit or a review; every
#      other mode still needs both; an unknown mode is refused;
#  10. app_kanban (041) reads the columns the card modal shows, not the
#      error text or the cost, and cannot write a run; kanban-app's
#      PgStore.LatestAgentRun (build tag wellint) as app_kanban.
#
#   scripts/well/test-zhen-agent-runs.sh [--sql-only] [--keep] [image]   # default postgres:16.15-alpine
#
# --keep leaves the server running for a demo (Step 216) and writes the
# throwaway DSNs (app_zhen, app_kanban, ops_writer; passwords that exist
# only in this container) to $KEEP_ENV (default ./qa-zhen-agent-runs.env,
# 0600). Stop it with `docker stop <name>` (printed).
#
# WELL_MIGRATIONS_DIR (default db/migrations) points the bootstrap at
# another copy of the migrations, so a provocation can be tested.
# Needs docker (sudo if not in the group).
set -euo pipefail

GO=1 KEEP=0
while (($#)); do
	case "$1" in
	--sql-only) GO=0; shift ;;
	--keep) KEEP=1; shift ;;
	*) break ;;
	esac
done
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="qa-zhen-agent-runs-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
ZPW="zhen-$RANDOM$RANDOM"
KPW="kanban-$RANDOM$RANDOM"
OPW="ops-$RANDOM$RANDOM"
cleanup() {
	((KEEP)) || "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/db"
cp -a "$SRC" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
chmod -R a+rX "$TMP/db"
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
# init.sh runs before the final server start; wait for the ledger.
for _ in $(seq 1 60); do
	"${DOCKER[@]}" exec "$N" psql -X -q -At -h 127.0.0.1 -U unheaded -d unheaded_app -c "SELECT 1 FROM well_schema_migrations LIMIT 1" >/dev/null 2>&1 && break
	sleep 1
done
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app -c "$1"; }
zh() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$1"; }
kb() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$KPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_kanban -d unheaded_app -c "$1"; }
own() { sq "SET ROLE well_app_owner; $1"; }
# The passwords exist only inside this throwaway container.
sq "ALTER ROLE app_zhen PASSWORD '$ZPW'; ALTER ROLE app_kanban PASSWORD '$KPW';" >/dev/null
"${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_ops -c "ALTER ROLE ops_writer PASSWORD '$OPW';" >/dev/null

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

SHA=$(printf 'a%.0s' $(seq 1 40))
card() { sq "INSERT INTO kanban_tasks (id, title, status, owner) VALUES ('$1', 'card $1', 'todo', 'zhen')" >/dev/null; }
# shellcheck disable=SC2016  # the SQL's quotes, not the shell's
claim() { # id card [branch] [status] [family]
	echo "INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family, branch, base_commit${4:+, status})
	      VALUES ('$1', '$2', now() + interval '30 minutes', 'interactive', '${5:-repo-task}', '${3:-zhen/$2}', '$SHA'${4:+, '$4'})"
}
setst() { echo "UPDATE zhen_agent_runs SET status = '$2' WHERE id = '$1'"; }

# 1. Ledger and owner.
if [[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '032_app_zhen_agent_runs.sql'")" == 1 ]]; then
	pass "bootstrap recorded 032 in unheaded_app"; else fail "032 not in the ledger"; fi
if [[ "$(sq "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'zhen_agent_runs'::regclass")" == well_app_owner ]]; then
	pass "owned by well_app_owner"; else fail "owner"; fi

# 2. The claim race: eight sessions, one card.
card race-card
card other-card
for i in $(seq 1 8); do
	zh "BEGIN; $(claim "$(printf 'e%015x' "$i")" race-card); SELECT pg_sleep(0.3); COMMIT;" >"$TMP/race.$i" 2>&1 &
done
wait || true
winners=$(sq "SELECT count(*) FROM zhen_agent_runs WHERE card_id = 'race-card'")
if [[ "$winners" == 1 ]]; then pass "claim race: exactly one of 8 won"; else fail "claim race: $winners winners"; sed 's/^/    /' "$TMP/race.1" | tail -3; fi
dups=$(grep -l 'zhen_agent_runs_one_active' "$TMP"/race.* 2>/dev/null | wc -l || true)
if [[ "$dups" == 7 ]]; then pass "the 7 losers hit the one-active index"; else fail "losers on the index: $dups"; fi
ok "another card is claimable meanwhile" zh "$(claim 0000000000000002 other-card)"

# 3. Transitions.
W=$(sq "SELECT id FROM zhen_agent_runs WHERE card_id = 'race-card'")
refused "claimed -> succeeded" zh "$(setst "$W" succeeded)"
ok "claimed -> running" zh "$(setst "$W" running)"
ok "lease renewal while running" zh "UPDATE zhen_agent_runs SET lease_until = now() + interval '35 minutes' WHERE id = '$W'"
ok "running -> succeeded with the outcome" zh "UPDATE zhen_agent_runs SET status = 'succeeded', head_commit = '$(printf 'b%.0s' $(seq 1 40))', route_summary = '{\"check\":\"pass\"}' WHERE id = '$W'"
refused "succeeded -> running" zh "$(setst "$W" running)"
refused "review fields without accept/reject" zh "UPDATE zhen_agent_runs SET review_reason = 'nice' WHERE id = '$W'"
ok "succeeded -> accepted with a review" zh "UPDATE zhen_agent_runs SET status = 'accepted', reviewed_at = now(), review_reason = 'merged' WHERE id = '$W'"
refused "accepted -> rejected" zh "UPDATE zhen_agent_runs SET status = 'rejected' WHERE id = '$W'"
ok "claimed -> abandoned" zh "$(setst 0000000000000002 abandoned)"
ok "claim again after the run ended" zh "$(claim 0000000000000003 other-card zhen/other-card-2)"
ok "claimed -> running (3)" zh "$(setst 0000000000000003 running)"
ok "running -> failed" zh "UPDATE zhen_agent_runs SET status = 'failed', error = 'check failed' WHERE id = '0000000000000003'"
refused "failed -> running" zh "$(setst 0000000000000003 running)"
refused "a run born running" zh "$(claim 0000000000000004 other-card zhen/other-card-3 running)"

# 4. Identity, finished outcomes, deletes.
card id-card
ok "claim (5)" zh "$(claim 0000000000000005 id-card)"
refused "app_zhen UPDATE of branch" zh "UPDATE zhen_agent_runs SET branch = 'zhen/evil' WHERE id = '0000000000000005'"
refused "owner UPDATE of branch" own "UPDATE zhen_agent_runs SET branch = 'zhen/evil' WHERE id = '0000000000000005'"
refused "owner UPDATE of card_id" own "UPDATE zhen_agent_runs SET card_id = 'race-card' WHERE id = '0000000000000005'"
refused "owner UPDATE of base_commit" own "UPDATE zhen_agent_runs SET base_commit = '$(printf 'c%.0s' $(seq 1 40))' WHERE id = '0000000000000005'"
refused "owner rewrites a failed run's error" own "UPDATE zhen_agent_runs SET error = '' WHERE id = '0000000000000003'"
refused "app_zhen DELETE" zh "DELETE FROM zhen_agent_runs"
refused "owner DELETE" own "DELETE FROM zhen_agent_runs"
refused "owner TRUNCATE" own "TRUNCATE zhen_agent_runs"

# 6. CHECKs.
card chk-card
refused "branch zhen/../x" zh "$(claim 0000000000000006 chk-card 'zhen/../x')"
refused "branch main" zh "$(claim 0000000000000007 chk-card main)"
refused "id not 16 hex" zh "$(claim XYZ chk-card)"
refused "family with a space" zh "$(claim 0000000000000008 chk-card zhen/chk-card '' 'repo task')"
refused "unknown card" zh "$(claim 0000000000000009 no-such-card)"

# 7. The card columns app_zhen may move.
ok "app_zhen moves a card" zh "UPDATE kanban_tasks SET status = 'in-progress', commits = '[{\"branch\":\"zhen/id-card\"}]' WHERE id = 'id-card'"
ok "app_zhen FOR UPDATE SKIP LOCKED" zh "BEGIN; SELECT id FROM kanban_tasks WHERE owner = 'zhen' AND status = 'todo' ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED; COMMIT;"
refused "app_zhen UPDATE of a card title" zh "UPDATE kanban_tasks SET title = 'pwned' WHERE id = 'id-card'"
refused "app_zhen UPDATE of a card owner" zh "UPDATE kanban_tasks SET owner = 'zhen' WHERE id = 'race-card'"
refused "app_zhen DELETE of a card" zh "DELETE FROM kanban_tasks WHERE id = 'id-card'"
refused "hard delete of a card with runs" sq "DELETE FROM kanban_tasks WHERE id = 'id-card'"

# 9. Scheduled runs (037): no branch, no base commit, never reviewed.
if [[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '037_app_zhen_agent_runs_scheduled.sql'")" == 1 ]]; then
	pass "bootstrap recorded 037 in unheaded_app"; else fail "037 not in the ledger"; fi
sq "INSERT INTO kanban_tasks (id, title, status, type, owner) VALUES ('sched-card', 'Scheduled: sweep', 'in-progress', 'scheduled-run', 'zhen-scheduler')" >/dev/null
sched() { # id [extra-cols] [extra-vals]
	echo "INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family${2:-}) VALUES ('$1', 'sched-card', now() + interval '15 minutes', 'scheduled', 'service-health-sweep'${3:-})"
}
ok "a scheduled run without a branch" zh "$(sched 000000000000000a)"
ok "scheduled claimed -> running" zh "$(setst 000000000000000a running)"
refused "a head commit on a scheduled run" zh "UPDATE zhen_agent_runs SET status = 'succeeded', head_commit = '$SHA' WHERE id = '000000000000000a'"
ok "scheduled running -> succeeded" zh "UPDATE zhen_agent_runs SET status = 'succeeded', route_summary = '{\"route\":\"runbook\",\"check\":\"pass\"}' WHERE id = '000000000000000a'"
refused "a scheduled run reviewed" zh "UPDATE zhen_agent_runs SET status = 'accepted', reviewed_at = now() WHERE id = '000000000000000a'"
refused "a scheduled run with a branch" zh "$(sched 000000000000000b ', branch' ", 'zhen/sched-card'")"
refused "a scheduled run with a base commit" zh "$(sched 000000000000000c ', base_commit' ", '$SHA'")"
refused "an interactive run without a branch" zh "INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family, base_commit) VALUES ('000000000000000d', 'chk-card', now() + interval '30 minutes', 'interactive', 'repo-task', '$SHA')"
refused "an overnight run without a base commit" zh "INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family, branch) VALUES ('000000000000000e', 'chk-card', now() + interval '30 minutes', 'overnight', 'repo-task', 'zhen/chk-card')"
refused "mode cron" zh "INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family) VALUES ('000000000000000f', 'sched-card', now() + interval '15 minutes', 'cron', 'x')"

# 10. The Kanban board reads runs (041): the columns the card modal shows,
#     nothing else, and never writes.
if [[ "$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '041_app_kanban_reads_agent_runs.sql'")" == 1 ]]; then
	pass "bootstrap recorded 041 in unheaded_app"; else fail "041 not in the ledger"; fi
ok "app_kanban reads a card's newest run" kb "SELECT id, status, mode, family, branch, head_commit, route_summary->>'review_note', review_reason, claimed_at, reviewed_at FROM zhen_agent_runs WHERE card_id = 'race-card' ORDER BY claimed_at DESC LIMIT 1"
refused "app_kanban SELECT error" kb "SELECT error FROM zhen_agent_runs LIMIT 1"
refused "app_kanban SELECT claude_nano_usd" kb "SELECT claude_nano_usd FROM zhen_agent_runs LIMIT 1"
refused "app_kanban SELECT *" kb "SELECT * FROM zhen_agent_runs LIMIT 1"
refused "app_kanban UPDATE of a run" kb "UPDATE zhen_agent_runs SET status = 'cancelled' WHERE id = '0000000000000005'"
refused "app_kanban INSERT of a run" kb "INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family) VALUES ('00000000000000ff', 'sched-card', now() + interval '15 minutes', 'scheduled', 'x')"

# 8. The Go stores against this server: pkg/zhenagent as app_zhen, and
#    kanban-app's agent-run read (041) as app_kanban.
if ((GO)); then
	export WELL_TEST_APP_DSN="postgres://app_zhen:$ZPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	export WELL_TEST_KANBAN_DSN="postgres://app_kanban:$KPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	if (cd "$R" && go test -count=1 -race -tags wellint ./pkg/zhenagent/ -run Well -v >"$TMP/go.out" 2>&1) && grep -q '^--- PASS: TestWellAgentRuns' "$TMP/go.out"; then
		pass "go test -tags wellint ./pkg/zhenagent -run Well"
	else
		fail "go store test"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	if (cd "$R" && go test -count=1 -race -tags wellint ./cmd/kanban-app/ -run TestWell -v >"$TMP/go.out" 2>&1) && grep -q '^--- PASS: TestWellLatestAgentRun' "$TMP/go.out"; then
		pass "go test -tags wellint ./cmd/kanban-app -run TestWell"
	else
		fail "kanban-app agent-run read"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	unset WELL_TEST_APP_DSN WELL_TEST_KANBAN_DSN
fi

if ((KEEP)); then
	ENVF="${KEEP_ENV:-$PWD/qa-zhen-agent-runs.env}"
	umask 077
	cat >"$ENVF" <<EOF
# throwaway Well $N on 127.0.0.1:$PORT; stop with: docker stop $N
export WELL_APP_ZHEN_DSN='postgres://app_zhen:$ZPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable'
export WELL_OPS_DSN='postgres://ops_writer:$OPW@127.0.0.1:$PORT/unheaded_ops?sslmode=disable'
export WELL_HOST=127.0.0.1 WELL_PORT=$PORT WELL_USER=app_kanban WELL_PASSWORD='$KPW' WELL_DB=unheaded_app WELL_SSLMODE=disable
export QA_WELL_CONTAINER=$N
EOF
	echo "kept $N on 127.0.0.1:$PORT; DSNs in $ENVF"
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
