#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# zhen-copy-rows.sh: copy Zhen's rows from unheaded_app into unheaded_zhen
# (ADR-149; S-ZHEN-SWITCH Phase 8B, Step 276). Attended only: Zhen stopped
# (Step 271, no app_zhen session), 051 and 052 applied (Step 273), 053 not.
# Runs psql as the Well's superuser inside its container.
#
#   1. refuses unless unheaded_app's zhen_* tables are exactly the eight
#      below, unheaded_zhen's eight exist and are empty, and every non-empty
#      zhen_actions.trace_id is a uuid (052 makes the column uuid; see
#      --null-bad-trace-id below); then forgets the record of an earlier
#      copy (step 5): the target is empty, so that record is no longer true;
#   2. counts each source table;
#   3. streams every table through the column map below (COPY (SELECT ...)
#      TO STDOUT) into ONE transaction in unheaded_zhen, triggers off
#      (session_replication_role = replica, as pg_dump --disable-triggers),
#      sets each id sequence to the source sequence's state, and checks every
#      target count against step 2; COMMIT is the stream's last line, so a
#      failed or cut stream ends the session without it and all eight roll
#      back;
#   4. counts the source again: a row written during the copy stops here;
#   5. records the counts for 053's guard, as the comment on a table 053
#      drops (so the record goes with the tables; 053's header says why it
#      is not a database setting):
#        COMMENT ON TABLE public.zhen_conversations IS
#          'copied <UTC time>: zhen_action_snapshots=<n>,...'
#      053 drops nothing unless each table still holds its recorded count.
#      To take the record back by hand (plan Appendix A procedure 4), in
#      unheaded_app: COMMENT ON TABLE public.zhen_conversations IS NULL
#
# The map is where 052's tightened columns get their values: a NULL becomes
# the column's default (sources '[]', tokens and elapsed_ms 0, model and
# feedback '', source 'user', parameters/result/metadata '{}', revertable
# false, triggered_by 'user', confidence 0, created_at/snapshot_at the copy's
# time, planned_at the action's started_at or completed_at, else the copy's
# time); a '' trace_id becomes NULL. Nothing else changes.
# tests/scripts/test_zhen_copy_rows.py holds the map to 052's columns.
#
#   sudo scripts/well/zhen-copy-rows.sh [--null-bad-trace-id FILE]
#   WELL_CONTAINER=unheaded-postgres WELL_SUPERUSER=unheaded (defaults)
#
# --null-bad-trace-id FILE (DECISION-PACK-2026-11-18 row 15: "Null it, record
# it, continue"). Without it, a zhen_actions row whose trace_id is neither
# NULL, '' nor a uuid stops the copy in step 1. With it, such a row is copied
# with trace_id NULL, and FILE (made 0600 when new; never a symlink) gains
# one line per row, appended once step 4 has passed and before step 5:
#   zhen_actions <id> <the text, escaped as inside a JSON string>
# so a tab, a newline, a quote or a backslash stays on the row's one line,
# and the text is only ever data: psql writes it, no shell reads it. Before
# COMMIT the transaction checks that the recorded rows, and with them only
# the rows whose trace_id was NULL or '', hold NULL. The count is printed.
# A zhen_* table this script does not know stops the copy either way.
set -euo pipefail

usage() { echo "usage: $0 [--null-bad-trace-id FILE]" >&2; exit 2; }
NULL_BAD=0 BADFILE=""
while (($#)); do
	case "$1" in
	--null-bad-trace-id)
		(($# >= 2)) && [[ -n "$2" ]] || { echo "--null-bad-trace-id needs the file to record the rows in" >&2; usage; }
		NULL_BAD=1 BADFILE="$2"; shift 2 ;;
	*) usage ;;
	esac
done
if ((NULL_BAD)); then
	[[ -d "$(dirname -- "$BADFILE")" ]] || { echo "--null-bad-trace-id: no directory for $BADFILE" >&2; exit 2; }
	if [[ -L "$BADFILE" ]] || { [[ -e "$BADFILE" ]] && [[ ! -f "$BADFILE" ]]; }; then
		echo "--null-bad-trace-id: $BADFILE is not a regular file" >&2; exit 2
	fi
fi

CONTAINER="${WELL_CONTAINER:-unheaded-postgres}"
SUPERUSER="${WELL_SUPERUSER:-unheaded}"
[[ "$CONTAINER" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || { echo "WELL_CONTAINER: not a container name" >&2; exit 2; }
[[ "$SUPERUSER" =~ ^[a-z_][a-z0-9_]{0,62}$ ]] || { echo "WELL_SUPERUSER: not a role name" >&2; exit 2; }
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)

# table|target columns|source expressions, parents first.
MAP=(
	"zhen_conversations|id, session_id, role, content, sources, model, tokens_input, tokens_output, elapsed_ms, feedback, created_at, client|id, session_id, role, content, coalesce(sources, '[]'::jsonb), coalesce(model, ''), coalesce(tokens_input, 0), coalesce(tokens_output, 0), coalesce(elapsed_ms, 0), coalesce(feedback, ''), created_at, client"
	"zhen_memories|id, question, answer, embedding, source, model, created_at|id, question, answer, embedding, coalesce(source, 'user'), model, coalesce(created_at, now())"
	"zhen_actions|id, session_id, action_type, status, intent, parameters, result, result_summary, error, revertable, reverted_by, reverts_action, triggered_by, model, confidence, planned_at, started_at, completed_at, elapsed_ms, trace_id, parent_action|id, session_id, action_type, status, intent, coalesce(parameters, '{}'::jsonb), coalesce(result, '{}'::jsonb), coalesce(result_summary, ''), coalesce(error, ''), coalesce(revertable, false), reverted_by, reverts_action, coalesce(triggered_by, 'user'), coalesce(model, ''), coalesce(confidence, 0), coalesce(planned_at, started_at, completed_at, now()), started_at, completed_at, coalesce(elapsed_ms, 0), nullif(trace_id, '')::uuid, parent_action"
	"zhen_action_snapshots|id, action_id, resource_type, resource_id, content_before, content_after, metadata, snapshot_at|id, action_id, resource_type, resource_id, content_before, content_after, coalesce(metadata, '{}'::jsonb), coalesce(snapshot_at, now())"
	"zhen_pending_confirmations|id, created_at, expires_at, source_service, emitted_by, project_root, tool, call, call_sha256, summary, card_id, status, decided_at, decided_by|id, created_at, expires_at, source_service, emitted_by, project_root, tool, call, call_sha256, summary, card_id, status, decided_at, decided_by"
	"zhen_agent_runs|id, card_id, claimed_at, lease_until, status, mode, family, branch, base_commit, head_commit, route_summary, claude_nano_usd, review_reason, reviewed_at, error|id, card_id, claimed_at, lease_until, status, mode, family, branch, base_commit, head_commit, route_summary, claude_nano_usd, review_reason, reviewed_at, error"
	"zhen_captures|id, captured_at, ingested_at, source, backend, model, author_kind, task_family, conversation_id, run_id, record_id, schema, redaction_version, prompt, response, tool_trace, truncated, purge_after, promoted, quarantined|id, captured_at, ingested_at, source, backend, model, author_kind, task_family, conversation_id, run_id, record_id, schema, redaction_version, prompt, response, tool_trace, truncated, purge_after, promoted, quarantined"
	"zhen_capture_labels|id, capture_id, kind, value, source_ref, created_at|id, capture_id, kind, value, source_ref, created_at"
)
# Tables whose id comes from a sequence (serial in unheaded_app, identity in unheaded_zhen).
SEQ_TABLES=(zhen_conversations zhen_memories zhen_actions zhen_action_snapshots zhen_capture_labels)
# What 052's uuid column takes from the old VARCHAR(64): 32 hex digits, or
# the 8-4-4-4-12 form. One pattern decides the refusal, the record and the NULL.
UUID_RE='^([0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$'
BAD_TRACE="trace_id <> '' AND trace_id !~* '$UUID_RE'"
# The map's trace_id expression, and what stands in for it under
# --null-bad-trace-id (a text that is not a uuid, '' among them, gives NULL).
NULLIF_TRACE="nullif(trace_id, '')::uuid"
NULLING_TRACE="CASE WHEN trace_id ~* '$UUID_RE' THEN trace_id::uuid END"

psql_in() { # db [psql args...]: statements on stdin
	local db="$1"; shift
	"${DOCKER[@]}" exec -i "$CONTAINER" psql -X -q -At -v ON_ERROR_STOP=1 -U "$SUPERUSER" -d "$db" "$@"
}
q() { psql_in "$1" -c "$2"; }

TABLES=()
for m in "${MAP[@]}"; do TABLES+=("${m%%|*}"); done
want="$(printf '%s\n' "${TABLES[@]}" | sort)"
list_sql="SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relname LIKE 'zhen\_%' ORDER BY 1"

# 1. Preconditions.
have="$(q unheaded_app "$list_sql")"
if [[ "$have" != "$want" ]]; then
	echo "[REFUSE] unheaded_app's zhen_* tables are not the eight this script copies:" >&2
	diff <(echo "$want") <(echo "$have") | sed -n 's/^[<>] /    /p' >&2
	exit 1
fi
if [[ "$(q unheaded_zhen "$list_sql")" != "$want" ]]; then
	echo "[REFUSE] unheaded_zhen lacks 052's eight tables (apply 051 and 052 first, plan Step 273)" >&2
	exit 1
fi
for t in "${TABLES[@]}"; do
	if [[ "$(q unheaded_zhen "SELECT EXISTS (SELECT 1 FROM public.$t)")" != f ]]; then
		echo "[REFUSE] unheaded_zhen.$t already holds rows; empty it first (plan Step 278)" >&2
		exit 1
	fi
done
bad="$(q unheaded_app "SELECT count(*) FROM public.zhen_actions WHERE $BAD_TRACE")"
[[ "$bad" =~ ^[0-9]+$ ]] || { echo "[FAIL] count of trace_ids that are not uuids: $bad" >&2; exit 1; }
if ((bad > 0 && ! NULL_BAD)); then
	echo "[REFUSE] $bad zhen_actions row(s) carry a trace_id that is not a uuid; 052's column is uuid (ruled 2026-11-18: --null-bad-trace-id FILE stores them NULL and records each id and text in FILE)" >&2
	exit 1
fi
BADTMP="" bad_ids="" nulls_want=""
if ((NULL_BAD)); then
	for m in "${MAP[@]}"; do
		[[ "${m%%|*}" != zhen_actions || "$m" == *"$NULLIF_TRACE"* ]] || { echo "[FAIL] the map's trace_id expression is no longer $NULLIF_TRACE" >&2; exit 1; }
	done
	BADTMP="$(mktemp)"
	trap 'rm -f -- "$BADTMP"' EXIT
	# One line per row. to_json escapes the text (quote, backslash, every
	# control character); its enclosing quotes are cut off.
	q unheaded_app "SELECT 'zhen_actions ' || id || ' ' || substr(j, 2, length(j) - 2) FROM (SELECT id, to_json(trace_id::text)::text AS j FROM public.zhen_actions WHERE $BAD_TRACE) b ORDER BY id" >"$BADTMP"
	if [[ "$(wc -l <"$BADTMP")" != "$bad" ]] || grep -qvE '^zhen_actions [0-9]+ .+$' "$BADTMP"; then
		echo "[FAIL] listed $(wc -l <"$BADTMP") row(s) with a trace_id that is not a uuid, counted $bad: something writes (plan Step 278)" >&2
		exit 1
	fi
	bad_ids="$(cut -d' ' -f2 "$BADTMP" | paste -sd, -)"
	nulls_want="$(q unheaded_app "SELECT count(*) FROM public.zhen_actions WHERE trace_id IS NULL OR trace_id = '' OR ($BAD_TRACE)")"
	[[ "$nulls_want" =~ ^[0-9]+$ ]] || { echo "[FAIL] count of trace_ids that become NULL: $nulls_want" >&2; exit 1; }
	# The record can be written: found out before anything is copied.
	(umask 077; : >>"$BADFILE") || { echo "[REFUSE] cannot write $BADFILE" >&2; exit 1; }
fi
# The target is empty, so a record left by an earlier copy is false from
# here on: without this, a second copy that failed would leave 053 a record
# whose counts still hold.
q unheaded_app "COMMENT ON TABLE public.zhen_conversations IS NULL" >/dev/null

# 2. Source counts.
declare -A before
for t in "${TABLES[@]}"; do before[$t]="$(q unheaded_app "SELECT count(*) FROM public.$t")"; done

# 3. One stream, one transaction.
{
	echo "BEGIN;"
	echo "SET LOCAL session_replication_role = replica;"
	for m in "${MAP[@]}"; do
		t="${m%%|*}"; rest="${m#*|}"; cols="${rest%%|*}"; exprs="${rest#*|}"
		if ((NULL_BAD)) && [[ "$t" == zhen_actions ]]; then exprs="${exprs/"$NULLIF_TRACE"/$NULLING_TRACE}"; fi
		echo "COPY public.$t ($cols) FROM STDIN;"
		q unheaded_app "COPY (SELECT $exprs FROM public.$t ORDER BY 1) TO STDOUT"
		printf '%s\n' '\.'
	done
	for t in "${SEQ_TABLES[@]}"; do
		state="$(q unheaded_app "SELECT last_value || ',' || is_called FROM $(q unheaded_app "SELECT pg_get_serial_sequence('public.$t', 'id')")")"
		[[ "$state" =~ ^[0-9]+,(true|false)$ ]] || { echo "[FAIL] sequence state of $t: $state" >&2; exit 1; }
		echo "SELECT setval(pg_get_serial_sequence('public.$t', 'id'), ${state%,*}, ${state#*,});"
	done
	echo "DO \$\$ BEGIN"
	for t in "${TABLES[@]}"; do
		echo "  IF (SELECT count(*) FROM public.$t) <> ${before[$t]} THEN RAISE EXCEPTION '$t: copied % row(s), expected ${before[$t]}', (SELECT count(*) FROM public.$t); END IF;"
	done
	if ((NULL_BAD)); then
		# Exactly the recorded rows, and those whose trace_id was NULL or '', hold NULL.
		echo "  IF (SELECT count(*) FROM public.zhen_actions WHERE trace_id IS NULL) <> $nulls_want THEN RAISE EXCEPTION 'zhen_actions: % row(s) hold a NULL trace_id, expected $nulls_want', (SELECT count(*) FROM public.zhen_actions WHERE trace_id IS NULL); END IF;"
		if [[ -n "$bad_ids" ]]; then
			echo "  IF (SELECT count(*) FROM public.zhen_actions WHERE id IN ($bad_ids) AND trace_id IS NULL) <> $bad THEN RAISE EXCEPTION 'zhen_actions: a row recorded for --null-bad-trace-id does not hold a NULL trace_id'; END IF;"
		fi
	fi
	echo "END \$\$;"
	echo "COMMIT;"
} | psql_in unheaded_zhen >/dev/null
echo "[OK] copied into unheaded_zhen in one transaction; target counts checked before COMMIT"

# 4. Nothing wrote to the source meanwhile.
record=""
for t in "${TABLES[@]}"; do
	n="$(q unheaded_app "SELECT count(*) FROM public.$t")"
	if [[ "$n" != "${before[$t]}" ]]; then
		echo "[FAIL] unheaded_app.$t went from ${before[$t]} to $n rows during the copy: something writes (plan Step 278)" >&2
		exit 1
	fi
	record+="${record:+,}$t=$n"
	printf '%s\t%s\n' "$t" "$n"
done

# The rows stored with a NULL trace_id, on record before 053 may run: after
# it, unheaded_app no longer holds their text.
if ((NULL_BAD)); then
	cat -- "$BADTMP" >>"$BADFILE"
	if ((bad > 0)); then
		echo "[OK] $bad zhen_actions row(s) carried a trace_id that is not a uuid: stored NULL, each id and text appended to $BADFILE"
	else
		echo "[OK] 0 zhen_actions rows carry a trace_id that is not a uuid: $BADFILE gains no line"
	fi
fi

# 5. What 053 checks before it drops anything.
q unheaded_app "COMMENT ON TABLE public.zhen_conversations IS 'copied $(date -u +%FT%TZ): $record'" >/dev/null
echo "[OK] counts recorded as the comment on unheaded_app's zhen_conversations; 053 may run while they hold"
