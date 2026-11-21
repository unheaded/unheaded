#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# shellcheck disable=SC2016 # $VARS inside sh -c strings are for the container shell
#
# logstore-measure.sh — one reading of a measured log store week (S-E1-LIVE
# Evening 16, Steps 278 and 282; S-LOG-WEEK-2; ADR-117, ADR-147 decision 5):
# the same journal rows in ClickHouse and in The Well
# (unheaded_ops.log_events), side by side.
#
#   scripts/logstore-measure.sh [FILE]
#
# Appends one block of `reading_utc<TAB>metric<TAB>value` lines to FILE
# (default $LOGSTORE_TSV, else ~/.local/state/unheaded/S-E1-LIVE-logstore.tsv:
# collected readings live on the host, not in the repository). Numbers only:
# no row, message or unit name is ever written, and every query's result goes
# to /dev/null. Read-only on both stores. scripts/logstore-reckon.py turns a
# file of readings into the table against the bar.
#
# What it reads, on each side, for west's own rows (host = 'west'; rows the
# syslog relay files for another device never enter the Well copy), in the
# order it reads them:
#   rows_yesterday, rows_7d        the row set being compared
#   bytes                          on disk: ClickHouse active parts; the Well
#                                  copy's partitions with their indexes
#   pg_bytes_yesterday             yesterday's partition alone
#   well_probe_first_*             the Well's own latency, before the queries
#                                  below load its cache: one read the Kanban
#                                  makes (unheaded_app.kanban_tasks)
#   q1/q2/q3 _p50_ms, _p95_ms, _runs
#                                  last hour by host; a text match over 24 h;
#                                  a 7-day count by unit. Each runs up to RUNS
#                                  times (default 20; p95 is the 19th of 20)
#                                  inside the time budget below; _runs is how
#                                  many finished. ClickHouse: server time
#                                  (--time). Postgres: psql \timing over the
#                                  container's socket, one session a query.
#   well_probe_*                   the same probe again, where week 1 had it:
#                                  after the queries
#   well_calls_total, well_exec_ms_total
#                                  pg_stat_statements, every statement that
#                                  does not touch log_events (cumulative: the
#                                  mean a day is the difference of two readings)
#   backup_seconds, backup_bytes   the last nightly backup set
#   vector_record_buffer_*, vector_record_sends_over_1s_total,
#   vector_well_dropped_total, vector_well_buffer_dropped_total
#                                  from Vector's own page: the record sink's
#                                  buffer depth and its sends over 1 s (a
#                                  stall); rows the Well sink gave up on
#                                  (component_discarded, week 1's count) and
#                                  rows its full buffer turned away
#                                  (buffer_discarded: what a stopped writer
#                                  causes). Cumulative since Vector's last
#                                  start.
#   vector_record_sends_over_1s_24h, vector_well_dropped_24h,
#   vector_well_buffer_dropped_24h the same three as increase(...[24h]) from
#                                  VictoriaMetrics, which a Vector restart
#                                  does not reset
#   ch_started_unix, pg_started_unix, vector_started_unix, writer_started_unix
#                                  when each container last started
#   host_boot_unix                 btime of /proc/stat
#   io_some_avg300                 /proc/pressure/io, `some`, 5-minute average
#
# The time budget (QUERY_BUDGET_S, default 900, at least QUERY_BUDGET_MIN_S):
# each query on each side, and each probe, gets that many seconds for its RUNS
# runs. When it is spent the run in flight is cut (Postgres ends the statement
# itself: statement_timeout is set to what is left before every run; the
# ClickHouse client is stopped) and the quantiles are taken from the runs that
# finished. Five or more: a number, with _runs beside it. Fewer than five: NA.
# A slow query is a number, not a broken reading (the 2026-11-16 reading held
# NA for a query that took 30 s a run). The budget costs the Well two small
# statements a run (what is left; SET statement_timeout), which
# well_calls_total counts: at most 2 x RUNS for each of the five Postgres
# sessions of a reading, about 200 against some 400,000 calls a day.
#
# A value that cannot be read is written as `NA` and the script exits 1 after
# finishing the block: a broken reading is visible, never a silent zero. One
# exception: writer_started_unix is NA while the writer is stopped (the
# baseline of S-LOG-WEEK-2 stops it on purpose), and that alone does not fail
# the reading.
#
# Settings (environment): RUNS, QUERY_BUDGET_S, QUERY_BUDGET_MIN_S (default
# 60), LOGSTORE_TSV, CLICKHOUSE_CONTAINER, WELL_CONTAINER, VECTOR_CONTAINER,
# LOGSTORE_WRITER_CONTAINER, VECTOR_METRICS_URL, VICTORIA_URL, WELL_BACKUP_DIR,
# LOGSTORE_PROC (where /proc is; the test's own). wiki/Log-Aggregation.md
# lists them with their defaults.
#
# Needs sudo for docker and for the backup set's size. Test:
# scripts/test-logstore-measure.sh (stubs, no container).
set -uo pipefail

# The fewest finished runs a p50 and a p95 are taken from. Not a setting: it is
# part of what a reading means, and two weeks are compared on it.
MIN_RUNS=5

OUT="${1:-${LOGSTORE_TSV:-$HOME/.local/state/unheaded/S-E1-LIVE-logstore.tsv}}"
RUNS="${RUNS:-20}"
QUERY_BUDGET_S="${QUERY_BUDGET_S:-900}"
QUERY_BUDGET_MIN_S="${QUERY_BUDGET_MIN_S:-60}"
CH="${CLICKHOUSE_CONTAINER:-unheaded-clickhouse}"
PG="${WELL_CONTAINER:-unheaded-postgres}"
VECTOR="${VECTOR_CONTAINER:-unheaded-vector}"
WRITER="${LOGSTORE_WRITER_CONTAINER:-unheaded-logstore-writer}"
VECTOR_METRICS="${VECTOR_METRICS_URL:-http://172.17.0.1:9598/metrics}"
VICTORIA="${VICTORIA_URL:-http://127.0.0.1:8428}"
BACKUP_DIR="${WELL_BACKUP_DIR:-/srv/backups/unheaded-well}"
PROC="${LOGSTORE_PROC:-/proc}"
[[ "$RUNS" =~ ^[0-9]+$ && "$RUNS" -ge "$MIN_RUNS" ]] || { echo "RUNS must be a number, $MIN_RUNS or more" >&2; exit 2; }
[[ "$QUERY_BUDGET_MIN_S" =~ ^[0-9]+$ && "$QUERY_BUDGET_MIN_S" -ge 1 ]] || { echo "QUERY_BUDGET_MIN_S must be a number, 1 or more" >&2; exit 2; }
[[ "$QUERY_BUDGET_S" =~ ^[0-9]+$ && "$QUERY_BUDGET_S" -ge "$QUERY_BUDGET_MIN_S" ]] || { echo "QUERY_BUDGET_S must be a number of seconds, $QUERY_BUDGET_MIN_S or more" >&2; exit 2; }
case "$OUT" in
-*) echo "usage: scripts/logstore-measure.sh [FILE]" >&2; exit 2 ;;
esac

NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
mkdir -p "$(dirname "$OUT")" || exit 1
[[ -s "$OUT" ]] || printf 'reading_utc\tmetric\tvalue\n' >"$OUT"
bad=0
put() { # metric value
	local v="$2"
	[[ "$v" =~ ^-?[0-9]+(\.[0-9]+)?$ ]] || { v=NA; bad=1; }
	printf '%s\t%s\t%s\n' "$NOW" "$1" "$v" >>"$OUT"
}
put_soft() { # metric value: as put, but an NA here does not fail the reading
	local was="$bad"
	put "$1" "$2"
	bad="$was"
}

ch() { # one query, value on stdout
	timeout 120 sudo docker exec "$CH" sh -c 'clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" -q "$1"' _ "$1" 2>/dev/null
}
# One query, a run at a time, up to RUNS runs inside the budget; server
# seconds, one a line. A run that fails, or that the budget cuts, gives no
# line: only a client that exited 0 and ended on a number counts.
ch_times() { # query
	local start="$SECONDS" left n=0 t
	while ((n < RUNS)); do
		left=$((QUERY_BUDGET_S - (SECONDS - start)))
		((left > 0)) || break
		if t="$(timeout "$left" sudo docker exec "$CH" sh -c 'clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" --time --format Null -q "$1" 2>&1 >/dev/null' _ "$1" 2>/dev/null)"; then
			t="${t##*$'\n'}"
			[[ "$t" =~ ^[0-9]+(\.[0-9]+)?$ ]] && printf '%s\n' "$t"
		fi
		n=$((n + 1))
	done
}
pg() { # db, one query, value on stdout
	printf 'SET default_transaction_read_only = on;\n%s\n' "$2" |
		timeout 120 sudo docker exec -i "$PG" sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1"' _ "$1" 2>/dev/null
}
# One query, a run at a time in one psql session (as week 1 ran it: one warm
# backend), up to RUNS runs inside the budget; milliseconds, one a line.
#
# The budget is kept inside the session, on the server's clock: a deadline is
# set once; before each run psql asks how much is left, skips the run when
# nothing is (\if), and otherwise sets statement_timeout to it, so the server
# itself ends a run that would pass the deadline and no scan outlives the
# measurement. psql stops at that error (ON_ERROR_STOP) and may still print a
# time for the run it cut: a time counts only when the marker after its
# statement follows it, and the marker is printed only after a run that
# finished. The outer timeout is for a session that hangs.
RUN_OK=logstore-run-ok
pg_times() { # db, query
	{
		printf 'SET default_transaction_read_only = on;\n\\o /dev/null\n'
		printf 'SELECT extract(epoch FROM clock_timestamp()) + %d AS deadline \\gset\n' "$QUERY_BUDGET_S"
		for _ in $(seq 1 "$RUNS"); do
			printf '%s\n' \
				'\timing off' \
				'SELECT :deadline > extract(epoch FROM clock_timestamp()) AS go, greatest(1, ceil((:deadline - extract(epoch FROM clock_timestamp())) * 1000))::bigint AS left_ms \gset' \
				'\if :go' \
				'SET statement_timeout = :left_ms;' \
				'\timing on' \
				"$2" \
				"\\echo $RUN_OK" \
				'\endif'
		done
	} | timeout "$((QUERY_BUDGET_S + 120))" sudo docker exec -i "$PG" sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1"' _ "$1" 2>/dev/null |
		awk -v ok="$RUN_OK" '
			/^Time: [0-9]+(\.[0-9]+)? ms/ { t = $2; next }
			$0 == ok { if (t != "") print t; t = ""; next }
			{ t = "" }'
}
# The count of values, then the median and the value at the 95th percentile
# rank (nearest rank), from one number a line; `scale` turns seconds into
# milliseconds. Prints "n p50 p95", or "n" alone when fewer than MIN_RUNS
# values arrived.
quantiles() {
	sort -n | awk -v scale="$1" -v floor="$MIN_RUNS" '
		{ v[NR] = $1 * scale }
		END {
			if (NR < floor || NR == 0) { printf "%d\n", NR; exit }
			m = int((NR + 1) / 2); p = int(NR * 0.95 + 0.999999); if (p > NR) p = NR
			printf "%d %.3f %.3f\n", NR, v[m], v[p]
		}'
}
timed() { # name, clickhouse query, postgres query
	local n p50 p95
	read -r n p50 p95 <<<"$(ch_times "$2" | quantiles 1000)"
	put "ch_$1_p50_ms" "${p50:-}"
	put "ch_$1_p95_ms" "${p95:-}"
	put "ch_$1_runs" "${n:-}"
	read -r n p50 p95 <<<"$(pg_times unheaded_ops "$3" | quantiles 1)"
	put "pg_$1_p50_ms" "${p50:-}"
	put "pg_$1_p95_ms" "${p95:-}"
	put "pg_$1_runs" "${n:-}"
}
well_probe() { # metric prefix: one read the Kanban makes, RUNS times
	local n p50 p95
	read -r n p50 p95 <<<"$(pg_times unheaded_app "SELECT * FROM kanban_tasks;" | quantiles 1)"
	put "$1_p50_ms" "${p50:-}"
	put "$1_p95_ms" "${p95:-}"
}

# The row set, and its size on disk.
put ch_rows_yesterday "$(ch "SELECT count() FROM unheaded_logs.logs WHERE host = 'west' AND toDate(timestamp) = yesterday()")"
put pg_rows_yesterday "$(pg unheaded_ops "SELECT count(*) FROM log_events WHERE host = 'west' AND ts >= (now() AT TIME ZONE 'UTC')::date - 1 AND ts < (now() AT TIME ZONE 'UTC')::date")"
put ch_rows_7d "$(ch "SELECT count() FROM unheaded_logs.logs WHERE host = 'west' AND timestamp > now() - INTERVAL 7 DAY")"
put pg_rows_7d "$(pg unheaded_ops "SELECT count(*) FROM log_events WHERE host = 'west' AND ts > now() - interval '7 days'")"
put ch_bytes "$(ch "SELECT sum(bytes_on_disk) FROM system.parts WHERE database = 'unheaded_logs' AND table = 'logs' AND active")"
put ch_rows "$(ch "SELECT sum(rows) FROM system.parts WHERE database = 'unheaded_logs' AND table = 'logs' AND active")"
put pg_bytes "$(pg unheaded_ops "SELECT coalesce(sum(pg_total_relation_size(inhrelid)), 0)::bigint FROM pg_inherits WHERE inhparent = 'public.log_events'::regclass")"
put pg_rows "$(pg unheaded_ops "SELECT count(*) FROM log_events")"
put pg_bytes_yesterday "$(pg unheaded_ops "SELECT coalesce(pg_total_relation_size(to_regclass('public.log_events_p' || to_char((now() AT TIME ZONE 'UTC')::date - 1, 'YYYYMMDD'))), 0)")"

# The Well's own latency, before the queries load the Well's cache;
# `well_probe_*` below stays where week 1 had it.
well_probe well_probe_first

# The three queries, the same on each side.
timed q1_last_hour \
	"SELECT timestamp, unit, level, message FROM unheaded_logs.logs WHERE host = 'west' AND timestamp > now() - INTERVAL 1 HOUR ORDER BY timestamp DESC LIMIT 200" \
	"SELECT ts, unit, level, message FROM log_events WHERE host = 'west' AND ts > now() - interval '1 hour' ORDER BY ts DESC LIMIT 200;"
timed q2_text_24h \
	"SELECT count() FROM unheaded_logs.logs WHERE host = 'west' AND timestamp > now() - INTERVAL 24 HOUR AND message LIKE '%session opened%'" \
	"SELECT count(*) FROM log_events WHERE host = 'west' AND ts > now() - interval '24 hours' AND message LIKE '%session opened%';"
timed q3_units_7d \
	"SELECT unit, count() FROM unheaded_logs.logs WHERE host = 'west' AND timestamp > now() - INTERVAL 7 DAY GROUP BY unit" \
	"SELECT unit, count(*) FROM log_events WHERE host = 'west' AND ts > now() - interval '7 days' GROUP BY unit;"

# The Well's own latency, after the queries (week 1's place).
well_probe well_probe
r="$(pg postgres "SELECT coalesce(sum(calls), 0)::bigint || ' ' || coalesce(round(sum(total_exec_time)), 0)::bigint FROM pg_stat_statements WHERE query NOT ILIKE '%log_events%'")"
put well_calls_total "${r%% *}"
put well_exec_ms_total "${r##* }"

# The last nightly backup: how long, how big.
start="$(systemctl show unheaded-well-backup.service -p ExecMainStartTimestampMonotonic --value 2>/dev/null)"
stop="$(systemctl show unheaded-well-backup.service -p ExecMainExitTimestampMonotonic --value 2>/dev/null)"
if [[ "$start" =~ ^[0-9]+$ && "$stop" =~ ^[0-9]+$ && "$stop" -ge "$start" && "$start" -gt 0 ]]; then
	put backup_seconds "$(awk -v a="$start" -v b="$stop" 'BEGIN { printf "%.3f", (b - a) / 1e6 }')"
else
	put backup_seconds NA
fi
latest="$(sudo cat "$BACKUP_DIR/LATEST" 2>/dev/null)"
if [[ "$latest" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}/well-[0-9]{6}Z$ ]]; then
	put backup_bytes "$(sudo du -sb "$BACKUP_DIR/$latest" 2>/dev/null | cut -f1)"
else
	put backup_bytes NA
fi

# Vector: the record sink must never stall; the Well sink may drop.
metrics="$(timeout 20 curl -fsS "$VECTOR_METRICS" 2>/dev/null)"
series() { # metric name, component, extra label match (may be empty)
	awk -v m="$1" -v c="component_id=\"$2\"" -v x="$3" '
		index($0, m "{") == 1 && index($0, c) && (x == "" || index($0, x)) { s += $2; n++ }
		END { if (n) printf "%.0f\n", s; else print "" }' <<<"$metrics"
}
if [[ -n "$metrics" ]]; then
	put vector_record_buffer_events "$(series vector_buffer_events clickhouse "")"
	put vector_record_buffer_bytes "$(series vector_buffer_byte_size clickhouse "")"
	all="$(series vector_buffer_send_duration_seconds_count clickhouse "")"
	fast="$(series vector_buffer_send_duration_seconds_bucket clickhouse 'le="1"')"
	if [[ "$all" =~ ^[0-9]+$ && "$fast" =~ ^[0-9]+$ ]]; then put vector_record_sends_over_1s_total "$((all - fast))"; else put vector_record_sends_over_1s_total NA; fi
	# No series yet means nothing was dropped since Vector started. Two
	# counters: rows the sink itself gave up on (a request the writer
	# refused for good), and rows that never entered its buffer because it
	# was full (when_full: drop_newest; what a stopped writer causes).
	dropped="$(series vector_component_discarded_events_total well_logs "")"
	put vector_well_dropped_total "${dropped:-0}"
	dropped="$(series vector_buffer_discarded_events_total well_logs "")"
	put vector_well_buffer_dropped_total "${dropped:-0}"
else
	for m in vector_record_buffer_events vector_record_buffer_bytes vector_record_sends_over_1s_total vector_well_dropped_total vector_well_buffer_dropped_total; do put "$m" NA; done
fi

# The same counts over 24 hours, from VictoriaMetrics (it scrapes Vector
# every 15 s, config/victoria/scrape.yml): Vector's own counters start again
# at every Vector start, these do not.
vm() { # one instant query: the first value, rounded; nothing for an empty answer; status 1 when there was no answer
	local body
	body="$(curl -fsS -m 10 -G "$VICTORIA/api/v1/query" --data-urlencode "query=$1" 2>/dev/null)" || return 1
	python3 -c '
import json, sys
try:
    answer = json.load(sys.stdin)
    if answer.get("status") != "success":
        sys.exit(1)
    result = answer["data"]["result"]
    if result:
        print(round(float(result[0]["value"][1])))
except (ValueError, KeyError, IndexError, TypeError, OverflowError):
    sys.exit(1)
' <<<"$body"
}
sends="$(vm 'sum(increase(vector_buffer_send_duration_seconds_count{component_id="clickhouse"}[24h])) - sum(increase(vector_buffer_send_duration_seconds_bucket{component_id="clickhouse",le="1"}[24h]))')" || sends=""
put vector_record_sends_over_1s_24h "$sends"
# A count Vector makes only on the first drop. An empty answer means no drop
# in 24 h, but only while VictoriaMetrics holds Vector's series at all: the
# sends above answered. Otherwise the series is missing, not zero.
vm_drops() { # query
	local v
	v="$(vm "$1")" || return 0
	[[ -n "$v" || -z "$sends" ]] || v=0
	printf '%s\n' "$v"
}
put vector_well_dropped_24h "$(vm_drops 'sum(increase(vector_component_discarded_events_total{component_id="well_logs"}[24h]))')"
put vector_well_buffer_dropped_24h "$(vm_drops 'sum(increase(vector_buffer_discarded_events_total{component_id="well_logs"}[24h]))')"

# What restarted: a start time that differs between two readings is a restart
# between them.
started() { # container: Unix seconds of its last start; nothing when it is not running
	local s
	s="$(timeout 20 sudo docker inspect -f '{{.State.Running}} {{.State.StartedAt}}' "$1" 2>/dev/null)" || return 0
	[[ "$s" == "true "* ]] || return 0
	date -u -d "${s#true }" +%s 2>/dev/null
}
put ch_started_unix "$(started "$CH")"
put pg_started_unix "$(started "$PG")"
put vector_started_unix "$(started "$VECTOR")"
put_soft writer_started_unix "$(started "$WRITER")"
put host_boot_unix "$(awk '$1 == "btime" { print $2 }' "$PROC/stat" 2>/dev/null)"
put io_some_avg300 "$(awk '$1 == "some" { for (i = 2; i <= NF; i++) if ($i ~ /^avg300=/) { sub(/^avg300=/, "", $i); print $i } }' "$PROC/pressure/io" 2>/dev/null)"

if [[ $bad -ne 0 ]]; then
	echo "[FAIL] reading $NOW has NA values in $OUT" >&2
	exit 1
fi
echo "[PASS] reading $NOW appended to $OUT"
