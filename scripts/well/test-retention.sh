#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks the retention job (db/migrations 042/043, cmd/well-retention,
# compliance/retention/classes.yaml) on a throwaway PostgreSQL server
# bootstrapped by db/init.sh from the manifest, never the real Well:
#   1. the bootstrap records 042 in postgres and unheaded_app and 043 in
#      unheaded_ops; retention_runner logs in, connection limit 4; both new
#      tables belong to well_ops_owner;
#   2. retention_runner's grants are scoped: it cannot delete from
#      compliance_findings_history, audit_events, llm_requests,
#      zhen_agent_runs, zhen_memories or zhen_captures, read a payload
#      column, update a card, or edit its own run records;
#   3. seeded: 3 old + 2 new conversations; a soft-deleted card past 90
#      days (an email and a key in its text), one past 90 days that an
#      agent run names, a recent one, a live one, an archived one; an
#      expired and a fresh capture; a 9-day-old health transition;
#   4. -dry-run with the repo's classes file counts (3 conversations, 1
#      card, 1 held, 1 capture), deletes nothing, writes one dry_run row
#      per class;
#   5. a live run with the repo's file (ratified: false) is refused, exit 2;
#   6. a live run with a ratified copy: old conversations gone and new
#      kept; the card is in retention_archive with the email and key
#      redacted, then deleted; the card a run names, the recent, live and
#      archived cards, 034's Genesis cards and the run itself remain; the
#      expired capture is gone; service-health-transitions is due for
#      partitions; metrics written;
#   7. a second live run deletes nothing;
#   8. a class on hold (legal hold) deletes nothing and records `held`;
#   9. retention_runs is append-only for the owner; ops_reader reads the
#      runs, not the archive.
#
#   scripts/well/test-retention.sh [image]   # default postgres:16.15-alpine
#
# WELL_MIGRATIONS_DIR (default db/migrations) points the bootstrap at
# another copy of the migrations, so a provocation can be tested.
# Needs docker (sudo if not in the group) and Go.
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="qa-retention-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
RR_PW="rr-$RANDOM$RANDOM"
RD_PW="rd-$RANDOM$RANDOM"
cleanup() {
	timeout 60 "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

(cd "$R" && go build -o "$TMP/well-retention" ./cmd/well-retention)

mkdir -p "$TMP/db" "$TMP/metrics"
cp -a "$SRC" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
timeout 120 "${DOCKER[@]}" run -d --rm --name "$N" -p 127.0.0.1::5432 \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-e RETENTION_RUNNER_PASSWORD="$RR_PW" \
	-v "$TMP/db/migrations:/migrations:ro" -v "$TMP/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 120); do
	if timeout 10 "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { timeout 20 "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready"; exit 1; }
PORT="$(timeout 20 "${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

# sq DB SQL: superuser. login ROLE PASSWORD DB SQL: a real login over TCP.
sq() { timeout 60 "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2"; }
login() { timeout 60 "${DOCKER[@]}" exec -i -e PGPASSWORD="$2" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$1" -d "$3" -c "$4"; }
rr() { login retention_runner "$RR_PW" "$@"; }
# Passwords exist only inside this throwaway container.
sq postgres "ALTER ROLE ops_reader PASSWORD '$RD_PW';" >/dev/null

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
job() { # args...: the binary against this server, as retention_runner
	WELL_HOST=127.0.0.1 WELL_PORT="$PORT" WELL_SSLMODE=disable WELL_PASSWORD="$RR_PW" \
		WELL_PASSWORD_FILE='' KINGDOM_NODE=qa "$TMP/well-retention" "$@"
}
eq() { # label got want
	[[ "$2" == "$3" ]] && pass "$1" || fail "$1: got [$2] want [$3]"
}

# 1. Ledger, role, owners.
eq "042 in the postgres ledger" "$(sq postgres "SELECT count(*) FROM well_schema_migrations WHERE filename = '042_retention_runner.sql'")" 1
eq "042 in the unheaded_app ledger" "$(sq unheaded_app "SELECT count(*) FROM well_schema_migrations WHERE filename = '042_retention_runner.sql'")" 1
eq "043 in the unheaded_ops ledger" "$(sq unheaded_ops "SELECT count(*) FROM well_schema_migrations WHERE filename = '043_ops_retention.sql'")" 1
eq "retention_runner: login, limit 4, no superuser/createrole" \
	"$(sq postgres "SELECT rolcanlogin, rolconnlimit, rolsuper, rolcreaterole FROM pg_roles WHERE rolname = 'retention_runner'")" "t|4|f|f"
for t in retention_runs retention_archive; do
	eq "$t belongs to well_ops_owner" "$(sq unheaded_ops "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = '$t'::regclass")" well_ops_owner
done
ok "retention_runner logs in to unheaded_ops over TCP" rr unheaded_ops "SELECT 1"
refused "retention_runner CONNECT unheaded_config" rr unheaded_config "SELECT 1"

# 2. Grants are scoped to the classes' tables.
refused "DELETE compliance_findings_history" rr unheaded_ops "DELETE FROM compliance_findings_history"
refused "DELETE audit_events" rr unheaded_ops "DELETE FROM audit_events"
refused "DELETE llm_requests" rr unheaded_ops "DELETE FROM llm_requests"
refused "DELETE service_health_transitions" rr unheaded_ops "DELETE FROM service_health_transitions"
refused "SELECT llm_requests.model_requested (payload column)" rr unheaded_ops "SELECT model_requested FROM llm_requests"
refused "SELECT audit_events.details (payload column)" rr unheaded_ops "SELECT details FROM audit_events"
refused "DELETE zhen_agent_runs" rr unheaded_app "DELETE FROM zhen_agent_runs"
refused "DELETE zhen_memories" rr unheaded_app "DELETE FROM zhen_memories"
refused "DELETE zhen_actions" rr unheaded_app "DELETE FROM zhen_actions"
refused "DELETE zhen_captures outside its purge function" rr unheaded_app "DELETE FROM zhen_captures"
refused "SELECT zhen_conversations.content" rr unheaded_app "SELECT content FROM zhen_conversations"
refused "UPDATE kanban_tasks" rr unheaded_app "UPDATE kanban_tasks SET title = 'x'"
refused "UPDATE retention_runs" rr unheaded_ops "UPDATE retention_runs SET error = 'x'"
refused "DELETE retention_runs" rr unheaded_ops "DELETE FROM retention_runs"
refused "TRUNCATE retention_archive" rr unheaded_ops "TRUNCATE retention_archive"

# 3. Seed (as the superuser; the job sees it through its grants).
KEY="sk-ant-$(printf 'q%.0s' {1..24})"
CAP_OLD="$(printf '%064x' 1)"
CAP_NEW="$(printf '%064x' 2)"
RUN_ID="$(printf '%016x' 7)"
BASE="$(printf '%040x' 9)"
sq unheaded_app "
INSERT INTO zhen_conversations (session_id, role, content, created_at) VALUES
  ('s-old', 'user', 'old 1', now() - interval '200 days'),
  ('s-old', 'assistant', 'old 2', now() - interval '120 days'),
  ('s-old', 'user', 'old 3', now() - interval '91 days'),
  ('s-new', 'user', 'new 1', now() - interval '10 days'),
  ('s-new', 'assistant', 'new 2', now());
INSERT INTO kanban_tasks (id, title, description, deleted_at, archived_at) VALUES
  ('card-old', 'mail jane@example.org', 'pasted $KEY by mistake', now() - interval '100 days', NULL),
  ('card-held', 'run history', '', now() - interval '100 days', NULL),
  ('card-recent', 'recent', '', now() - interval '5 days', NULL),
  ('card-live', 'live', '', NULL, NULL),
  ('card-archived', 'archived', '', NULL, now() - interval '400 days');
INSERT INTO zhen_agent_runs (id, card_id, lease_until, mode, family, branch, base_commit)
  VALUES ('$RUN_ID', 'card-held', now() + interval '1 hour', 'overnight', 'code', 'zhen/card-held', '$BASE');
INSERT INTO zhen_captures (id, captured_at, source, backend, model, author_kind, task_family, conversation_id,
                           record_id, schema, redaction_version, prompt, response, truncated) VALUES
  ('$CAP_OLD', now() - interval '200 days', 'zhen-claude', 'claude', 'm', 'claude', 'code', 'c1', 'r1', 'zhen-capture/v1', 1, 'p', 'r', false),
  ('$CAP_NEW', now(), 'zhen-claude', 'claude', 'm', 'claude', 'code', 'c1', 'r2', 'zhen-capture/v1', 1, 'p', 'r', false);
" >/dev/null
sq unheaded_ops "INSERT INTO service_health_transitions (service_name, host, old_status, new_status, transition_at)
  VALUES ('wotan', 'qa', 'ok', 'down', now() - interval '9 days')" >/dev/null
counts() {
	echo "$(sq unheaded_app "SELECT count(*) FROM zhen_conversations")/$(sq unheaded_app "SELECT count(*) FROM kanban_tasks WHERE id LIKE 'card-%'")/$(sq unheaded_app "SELECT count(*) FROM zhen_captures")"
}
# 034's Genesis seed: live cards the job must never touch.
GENESIS="$(sq unheaded_app "SELECT count(*) FROM kanban_tasks WHERE id NOT LIKE 'card-%'")"
eq "seeded conversations/cards/captures" "$(counts)" "5/5/2"

# 4. Dry run with the repo's file.
CLASSES="$R/compliance/retention/classes.yaml"
NCLASSES="$(grep -c '^  - name: ' "$CLASSES")"
if job -classes "$CLASSES" -dry-run >"$TMP/dry" 2>&1; then pass "dry run exits 0"; else fail "dry run"; sed 's/^/    /' "$TMP/dry" | tail -8; fi
grep -q 'class=zhen-conversations .* eligible=3 deleted=0' "$TMP/dry" && pass "dry run: 3 conversations eligible" || fail "dry conversations: $(grep zhen-conversations "$TMP/dry")"
grep -q 'class=kanban-soft-deleted .* eligible=1 deleted=0 archived=0 held=1' "$TMP/dry" && pass "dry run: 1 card eligible, 1 held by its run" ||
	fail "dry kanban: $(grep kanban "$TMP/dry")"
grep -q 'class=zhen-captures .* eligible=1 deleted=0' "$TMP/dry" && pass "dry run: 1 capture eligible" || fail "dry captures: $(grep captures "$TMP/dry")"
eq "dry run deleted nothing" "$(counts)" "5/5/2"
eq "one dry_run row per class" "$(sq unheaded_ops "SELECT count(*) FROM retention_runs WHERE dry_run AND host = 'qa' AND error = ''")" "$NCLASSES"
if grep -qE 'pasted|jane@|sk-ant' "$TMP/dry"; then fail "dry-run output carries row contents"; else pass "job output carries counts, never contents"; fi

# 5. A live run of an unratified file is refused.
set +e; job -classes "$CLASSES" >"$TMP/out" 2>&1; rc=$?; set -e
((rc == 2)) && grep -q 'ratified' "$TMP/out" && pass "live run of the unratified repo file refused (exit 2)" || fail "unratified live run: rc $rc $(tail -1 "$TMP/out")"
eq "refused run changed nothing" "$(counts)" "5/5/2"

# 6. A live run with a ratified copy.
sed 's/^ratified: false$/ratified: true/' "$CLASSES" >"$TMP/ratified.yaml"
if job -classes "$TMP/ratified.yaml" -metrics-dir "$TMP/metrics" -batch 2 >"$TMP/live" 2>&1; then pass "live run exits 0"; else fail "live run"; sed 's/^/    /' "$TMP/live" | tail -8; fi
eq "conversations left: the two new ones" "$(sq unheaded_app "SELECT string_agg(content, ',' ORDER BY content) FROM zhen_conversations")" "new 1,new 2"
eq "cards left" "$(sq unheaded_app "SELECT string_agg(id, ',' ORDER BY id) FROM kanban_tasks WHERE id LIKE 'card-%'")" "card-archived,card-held,card-live,card-recent"
eq "the Genesis cards are untouched" "$(sq unheaded_app "SELECT count(*) FROM kanban_tasks WHERE id NOT LIKE 'card-%'")" "$GENESIS"
eq "the agent run is untouched" "$(sq unheaded_app "SELECT card_id FROM zhen_agent_runs WHERE id = '$RUN_ID'")" card-held
eq "captures left: the fresh one" "$(sq unheaded_app "SELECT id FROM zhen_captures")" "$CAP_NEW"
eq "card-old archived once, from unheaded_app.kanban_tasks" \
	"$(sq unheaded_ops "SELECT count(*), min(source_db), min(source_table), min(class) FROM retention_archive WHERE source_key = 'card-old'")" \
	"1|unheaded_app|kanban_tasks|kanban-soft-deleted"
arch="$(sq unheaded_ops "SELECT row_data::text FROM retention_archive WHERE source_key = 'card-old'")"
if grep -qE 'jane@example.org|sk-ant-q' <<<"$arch"; then fail "archive kept PII or a key: $arch"; else pass "archive row is redacted"; fi
grep -q 'REDACTED:email' <<<"$arch" && grep -q 'REDACTED:anthropic_key' <<<"$arch" && pass "archive row carries the redaction tags" || fail "archive tags: $arch"
eq "archive row redaction count" "$(sq unheaded_ops "SELECT redactions FROM retention_archive WHERE source_key = 'card-old'")" 2
eq "live run rows: deleted per class" \
	"$(sq unheaded_ops "SELECT string_agg(class || '=' || deleted || '/' || archived || '/' || held, ',' ORDER BY class) FROM retention_runs WHERE NOT dry_run AND class IN ('zhen-conversations', 'kanban-soft-deleted', 'zhen-captures')")" \
	"kanban-soft-deleted=1/1/1,zhen-captures=1/0/0,zhen-conversations=3/0/0"
eq "service-health-transitions is due for partitions" \
	"$(sq unheaded_ops "SELECT detail->'partition'->>'due' FROM retention_runs WHERE NOT dry_run AND class = 'service-health-transitions'")" true
eq "audit-events (empty) is not due" \
	"$(sq unheaded_ops "SELECT detail->'partition'->>'due' FROM retention_runs WHERE NOT dry_run AND class = 'audit-events'")" false
M="$TMP/metrics/well_retention.prom"
grep -q '^unheaded_well_retention_last_success_timestamp_seconds ' "$M" && grep -q 'rows_deleted{class="zhen-conversations"} 3' "$M" &&
	grep -q 'partition_due{class="service-health-transitions",table="service_health_transitions"} 1' "$M" &&
	pass "metrics textfile written" || fail "metrics: $(cat "$M" 2>/dev/null | head -5)"

# 7. Idempotent.
if job -classes "$TMP/ratified.yaml" >"$TMP/again" 2>&1; then pass "second live run exits 0"; else fail "second run"; fi
if grep -E 'action=(purge|purge-function|archive-purge) ' "$TMP/again" | grep -qv ' deleted=0 '; then
	fail "second run deleted rows: $(grep deleted= "$TMP/again")"
else
	pass "second run deletes nothing"
fi

# 8. Legal hold.
sq unheaded_app "INSERT INTO zhen_conversations (session_id, role, content, created_at) VALUES ('s-hold', 'user', 'on hold', now() - interval '300 days')" >/dev/null
awk '{print} /^  - name: zhen-conversations$/ {print "    hold: true"}' "$TMP/ratified.yaml" >"$TMP/hold.yaml"
if job -classes "$TMP/hold.yaml" -class zhen-conversations >"$TMP/hold" 2>&1; then pass "held run exits 0"; else fail "held run"; sed 's/^/    /' "$TMP/hold" | tail -4; fi
eq "a held class deletes nothing" "$(sq unheaded_app "SELECT count(*) FROM zhen_conversations WHERE content = 'on hold'")" 1
eq "the held run is recorded as held" \
	"$(sq unheaded_ops "SELECT action, held, deleted FROM retention_runs WHERE class = 'zhen-conversations' ORDER BY id DESC LIMIT 1")" "held|1|0"

# 9. Append-only runs; the reader.
refused "owner UPDATE retention_runs" sq unheaded_ops "SET ROLE well_ops_owner; UPDATE retention_runs SET error = 'x'"
refused "owner DELETE retention_runs" sq unheaded_ops "SET ROLE well_ops_owner; DELETE FROM retention_runs"
refused "owner UPDATE retention_archive" sq unheaded_ops "SET ROLE well_ops_owner; UPDATE retention_archive SET redactions = 0"
ok "ops_reader reads retention_runs" login ops_reader "$RD_PW" unheaded_ops "SELECT count(*) FROM retention_runs"
refused "ops_reader reads retention_archive" login ops_reader "$RD_PW" unheaded_ops "SELECT count(*) FROM retention_archive"

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
