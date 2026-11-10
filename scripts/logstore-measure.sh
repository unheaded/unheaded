#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# shellcheck disable=SC2016 # $VARS inside sh -c strings are for the container shell
#
# logstore-measure.sh — one reading of the measured week (S-E1-LIVE Evening
# 16, Steps 278 and 282; ADR-117): the same journal rows in ClickHouse and in
# The Well (unheaded_ops.log_events), side by side.
#
#   scripts/logstore-measure.sh [FILE]
#
# Appends one block of `reading_utc<TAB>metric<TAB>value` lines to FILE
# (default $LOGSTORE_TSV, else ~/.local/state/unheaded/S-E1-LIVE-logstore.tsv:
# collected readings live on the host, not in the repository). Numbers only:
# no row, message or unit name is ever written, and every query's result goes
# to /dev/null. Read-only on both stores.
#
# What it reads, on each side, for west's own rows (host = 'west'; rows the
# syslog relay files for another device never enter the Well copy):
#   rows_yesterday, rows_7d        the row set being compared
#   bytes                          on disk: ClickHouse active parts; the Well
#                                  copy's partitions with their indexes
#   pg_bytes_yesterday             yesterday's partition alone
#   q1/q2/q3 _p50_ms and _p95_ms   last hour by host; a text match over 24 h;
#                                  a 7-day count by unit. Each runs RUNS times
#                                  (default 20); p95 is the 19th of 20.
#                                  ClickHouse: server time (--time). Postgres:
#                                  psql \timing over the container's socket.
#   well_probe_*                   the Well's own latency: one read the Kanban
#                                  makes (unheaded_app.kanban_tasks), RUNS times
#   well_calls_total, well_exec_ms_total
#                                  pg_stat_statements, every statement that
#                                  does not touch log_events (cumulative: the
#                                  mean a day is the difference of two readings)
#   backup_seconds, backup_bytes   the last nightly backup set
#   vector_*                       the record sink's buffer depth and its sends
#                                  over 1 s (a stall), and rows dropped at the
#                                  Well sink (cumulative)
#
# A value that cannot be read is written as `NA` and the script exits 1 after
# finishing the block: a broken reading is visible, never a silent zero.
#
# Needs sudo for docker and for the backup set's size.
set -uo pipefail

OUT="${1:-${LOGSTORE_TSV:-$HOME/.local/state/unheaded/S-E1-LIVE-logstore.tsv}}"
RUNS="${RUNS:-20}"
CH="${CLICKHOUSE_CONTAINER:-unheaded-clickhouse}"
PG="${WELL_CONTAINER:-unheaded-postgres}"
VECTOR_METRICS="${VECTOR_METRICS_URL:-http://172.17.0.1:9598/metrics}"
BACKUP_DIR="${WELL_BACKUP_DIR:-/srv/backups/unheaded-well}"
[[ "$RUNS" =~ ^[0-9]+$ && "$RUNS" -ge 5 ]] || { echo "RUNS must be a number, 5 or more" >&2; exit 2; }
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

ch() { # one query, value on stdout
	timeout 120 sudo docker exec "$CH" sh -c 'clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" -q "$1"' _ "$1" 2>/dev/null
}
ch_times() { # one query RUNS times; server seconds, one a line
	timeout 600 sudo docker exec "$CH" sh -c 'i=0; while [ "$i" -lt "$2" ]; do clickhouse-client --user unheaded --password "$CLICKHOUSE_PASSWORD" --time --format Null -q "$1" 2>&1 >/dev/null; i=$((i+1)); done' _ "$1" "$RUNS" 2>/dev/null
}
pg() { # db, one query, value on stdout
	printf 'SET default_transaction_read_only = on;\n%s\n' "$2" |
		timeout 120 sudo docker exec -i "$PG" sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1"' _ "$1" 2>/dev/null
}
pg_times() { # db, one query RUNS times; milliseconds, one a line
	{
		printf 'SET default_transaction_read_only = on;\n\\o /dev/null\n\\timing on\n'
		for _ in $(seq 1 "$RUNS"); do printf '%s\n' "$2"; done
	} | timeout 600 sudo docker exec -i "$PG" sh -c 'psql -X -q -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$1"' _ "$1" 2>/dev/null |
		sed -n 's/^Time: \([0-9.]*\) ms.*/\1/p'
}
# Median and the value at the 95th percentile rank (nearest rank), from one
# number a line; `scale` turns seconds into milliseconds. Prints "p50 p95",
# or nothing when fewer than RUNS values arrived.
quantiles() {
	sort -n | awk -v scale="$1" -v want="$RUNS" '
		{ v[NR] = $1 * scale }
		END {
			if (NR < want) exit 1
			m = int((NR + 1) / 2); p = int(NR * 0.95 + 0.999999); if (p > NR) p = NR
			printf "%.3f %.3f\n", v[m], v[p]
		}'
}
timed() { # name, clickhouse query, postgres query
	local r
	r="$(ch_times "$2" | quantiles 1000)" || r=""
	put "ch_$1_p50_ms" "${r%% *}"
	put "ch_$1_p95_ms" "${r##* }"
	r="$(pg_times unheaded_ops "$3" | quantiles 1)" || r=""
	put "pg_$1_p50_ms" "${r%% *}"
	put "pg_$1_p95_ms" "${r##* }"
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

# The Well's own latency.
r="$(pg_times unheaded_app "SELECT * FROM kanban_tasks;" | quantiles 1)" || r=""
put well_probe_p50_ms "${r%% *}"
put well_probe_p95_ms "${r##* }"
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
	# No series yet means nothing was dropped since Vector started.
	dropped="$(series vector_component_discarded_events_total well_logs "")"
	put vector_well_dropped_total "${dropped:-0}"
else
	for m in vector_record_buffer_events vector_record_buffer_bytes vector_record_sends_over_1s_total vector_well_dropped_total; do put "$m" NA; done
fi

if [[ $bad -ne 0 ]]; then
	echo "[FAIL] reading $NOW has NA values in $OUT" >&2
	exit 1
fi
echo "[PASS] reading $NOW appended to $OUT"
