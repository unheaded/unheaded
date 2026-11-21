#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# The time budget of scripts/logstore-measure.sh against a real psql and a
# real server (S-LOG-WEEK-2 Phase 1, G5), on a throwaway PostgreSQL, never the
# real Well. scripts/test-logstore-measure.sh proves the script's own logic
# against stubs; what a stub cannot show is what psql and the server do with
# the session the script writes (\gset, \if, statement_timeout, the marker
# after a finished run). Here `log_events` is a view whose every read takes
# SLEEP seconds, so each of the three queries is "slow" by a known amount:
#
#   1. budget 5 s, runs of 0.9 s: five finish, the sixth is cut by the server
#      at the deadline and is not counted (a time psql prints for it would
#      make six); p50 and p95 are about one run; the three sessions together
#      end about when their budgets do; no statement is left running;
#   2. budget 2 s: two runs at most finish, so the query is NA with its run count
#      beside it;
#   3. the probe (unheaded_app.kanban_tasks, fast) runs all its RUNS, before
#      the queries and after them.
#
# ClickHouse, Vector, VictoriaMetrics and the backup are not part of this:
# the wrapper docker below answers for ClickHouse and the rest read NA.
#
#   scripts/well/test-logstore-budget.sh [image]   # default postgres:16.15-alpine
#
# Needs docker (sudo if not in the group). About a minute.
set -euo pipefail

IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$R/scripts/logstore-measure.sh"
DOCKER=(docker); timeout 20 docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
REAL_DOCKER="$(command -v docker)"
N="qa-logstore-budget-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
SLEEP=0.9
cleanup() {
	timeout 60 "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

timeout 120 "${DOCKER[@]}" run -d --rm --name "$N" --network none \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 120); do
	# The entrypoint restarts the server once after its init; ask twice.
	if timeout 10 "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null && sleep 1 &&
		timeout 10 "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then
		ready=1
		break
	fi
	sleep 1
done
((ready)) || { timeout 20 "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready"; exit 1; }

sq() { timeout 60 "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$1" -c "$2"; }
sq postgres "CREATE DATABASE unheaded_ops;" >/dev/null
sq postgres "CREATE DATABASE unheaded_app;" >/dev/null
sq unheaded_app "CREATE TABLE kanban_tasks (id int PRIMARY KEY, title text); INSERT INTO kanban_tasks SELECT g, 'card ' || g FROM generate_series(1, 50) g;" >/dev/null
# Every read of the "table" takes SLEEP seconds: the volatile call in WHERE is
# run once for the one row.
sq unheaded_ops "CREATE VIEW log_events AS SELECT now() AS ts, 'west'::text AS host, 'unit'::text AS unit, 'info'::text AS level, 'session opened'::text AS message FROM generate_series(1, 1) g WHERE pg_sleep($SLEEP) IS NOT NULL;" >/dev/null

# sudo runs its arguments; docker goes to the throwaway server for the Well's
# container and answers by itself for ClickHouse.
mkdir -p "$TMP/bin"
cat >"$TMP/bin/sudo" <<'STUB'
#!/usr/bin/env bash
[[ "${1:-}" == -n ]] && shift
exec "$@"
STUB
# The wrapper is first on PATH, so it names the real docker (and the real sudo,
# where docker needs it) by full path.
REAL=("$REAL_DOCKER")
[[ "${DOCKER[0]}" == sudo ]] && REAL=("$(command -v sudo)" -n "$REAL_DOCKER")
{
	printf '#!/usr/bin/env bash\n'
	printf 'REAL=(%s)\n' "$(printf '%q ' "${REAL[@]}")"
	printf 'WELL=%q\n' "$N"
	cat <<'STUB'
case "${1:-}" in
exec)
	for a in "$@"; do
		[[ "$a" == "$WELL" ]] && exec "${REAL[@]}" "$@"
	done
	case "$*" in
	*--time*) echo 0.010 ;;
	*) echo 1000 ;;
	esac
	;;
inspect) echo "true 2026-11-20T17:50:00Z" ;;
*) exit 1 ;;
esac
STUB
} >"$TMP/bin/docker"
chmod +x "$TMP/bin/sudo" "$TMP/bin/docker"

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
val() { awk -F'\t' -v m="$1" '$2 == m { print $3 }' "$TSV"; }
between() { awk -v v="$1" -v lo="$2" -v hi="$3" 'BEGIN { exit !(v ~ /^[0-9.]+$/ && v >= lo && v <= hi) }'; }
reading() { # reading <RUNS> <QUERY_BUDGET_S>; sets TSV, TOOK
	TSV="$TMP/readings-$1-$2.tsv"
	local t0="$SECONDS"
	env PATH="$TMP/bin:$PATH" HOME="$TMP" WELL_CONTAINER="$N" LOGSTORE_TSV="$TSV" RUNS="$1" QUERY_BUDGET_S="$2" QUERY_BUDGET_MIN_S=1 \
		VECTOR_METRICS_URL=http://127.0.0.1:9/metrics VICTORIA_URL=http://127.0.0.1:9 WELL_BACKUP_DIR="$TMP/none" \
		bash "$SCRIPT" >/dev/null 2>&1 || true
	TOOK=$((SECONDS - t0))
}

# 1. Five runs fit in 5 s; the sixth is cut and is not a run.
reading 8 5
for q in q1_last_hour q2_text_24h q3_units_7d; do
	if [[ "$(val "pg_${q}_runs")" == 5 ]]; then pass "1 $q: five runs finished inside 5 s"; else fail "1 $q: runs is '$(val "pg_${q}_runs")', want 5 (six: the run the server cut was counted, or it was not cut)"; fi
	if between "$(val "pg_${q}_p50_ms")" 880 1300 && between "$(val "pg_${q}_p95_ms")" 880 1300; then
		pass "1 $q: p50 and p95 are about one run"
	else
		fail "1 $q: p50 '$(val "pg_${q}_p50_ms")' p95 '$(val "pg_${q}_p95_ms")', want about 900 ms each"
	fi
done
# Three sessions of 5 s, four counts through the slow view, two fast probes.
if ((TOOK >= 15 && TOOK <= 40)); then pass "1 the reading took ${TOOK}s: each session ended at its budget"; else fail "1 the reading took ${TOOK}s, want 15 to 40"; fi
left="$(sq unheaded_ops "SELECT count(*) FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND state <> 'idle' AND query LIKE '%log_events%'")"
if [[ "$left" == 0 ]]; then pass "1 no statement outlives the reading"; else fail "1 $left statement(s) still running after the reading"; fi
# 3. The probe ran all its runs both times.
if between "$(val well_probe_first_p95_ms)" 0 500 && between "$(val well_probe_p95_ms)" 0 500; then
	pass "3 the probe has a p95 before the queries and after them"
else
	fail "3 the probes read '$(val well_probe_first_p95_ms)' and '$(val well_probe_p95_ms)'"
fi

# 2. Two runs at most fit in 2 s: NA, with the count.
reading 8 2
if [[ "$(val pg_q3_units_7d_runs)" =~ ^[0-2]$ && "$(val pg_q3_units_7d_p50_ms) $(val pg_q3_units_7d_p95_ms)" == "NA NA" ]]; then
	pass "2 a budget of 2 s: $(val pg_q3_units_7d_runs) run(s), p50 and p95 NA"
else
	fail "2 runs '$(val pg_q3_units_7d_runs)' p50 '$(val pg_q3_units_7d_p50_ms)' p95 '$(val pg_q3_units_7d_p95_ms)', want at most 2 runs and NA NA"
fi

if ((fails)); then
	echo "FAIL: $fails check(s)"
	exit 1
fi
echo "PASS: the log store reading's time budget holds against a real server"
