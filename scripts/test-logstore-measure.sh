#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-logstore-measure.sh: logstore-measure.sh against stub sudo, docker,
# curl, systemctl and du on PATH and a made-up /proc (S-LOG-WEEK-2 Phase 1,
# ADR-147 decision 5). No container, no store, no sudo; runs as any user in
# about half a minute.
#
# The stub docker answers `docker exec` by the statement it is given: a count
# for a value query, one time a run for a timed one. Every timed answer also
# carries a row with the word CANARY, as if the result had leaked into the
# output: none of it may reach the readings file. $STUB_MODE names a file of
# key=value lines that changes one answer for one case.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT="$REPO_ROOT/scripts/logstore-measure.sh"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/proc/pressure" "$T/backups/2026-11-21/well-033700Z" "$T/home"
pass=0
fail=0
check() { # check <name> <command...>
	local name="$1"
	shift
	if "$@"; then
		echo "[PASS] $name"
		pass=$((pass + 1))
	else
		echo "[FAIL] $name"
		fail=$((fail + 1))
	fi
}

# --- the made-up host ---------------------------------------------------------
printf 'cpu  1 2 3 4\nbtime 1795225392\nprocesses 9\n' >"$T/proc/stat"
printf 'some avg10=3.02 avg60=3.10 avg300=6.07 total=933734810\nfull avg10=2.97 avg60=3.03 avg300=5.74 total=901761098\n' >"$T/proc/pressure/io"
printf '2026-11-21/well-033700Z\n' >"$T/backups/LATEST"
cat >"$T/vector.prom" <<'PROM'
# HELP vector_buffer_events buffered events
vector_buffer_events{component_id="clickhouse",component_kind="sink"} 3
vector_buffer_events{component_id="well_logs",component_kind="sink"} 4000
vector_buffer_byte_size{component_id="clickhouse",component_kind="sink"} 1024
vector_buffer_send_duration_seconds_bucket{component_id="clickhouse",le="0.5"} 400
vector_buffer_send_duration_seconds_bucket{component_id="clickhouse",le="1"} 498
vector_buffer_send_duration_seconds_bucket{component_id="clickhouse",le="10"} 500
vector_buffer_send_duration_seconds_count{component_id="clickhouse"} 500
vector_component_discarded_events_total{component_id="well_logs",intentional="false"} 7
PROM

# --- the stubs ----------------------------------------------------------------
cat >"$T/bin/sudo" <<'STUB'
#!/usr/bin/env bash
[[ "${1:-}" == -n ]] && shift
exec "$@"
STUB

cat >"$T/bin/du" <<'STUB'
#!/usr/bin/env bash
printf '101860940\t%s\n' "${*: -1}"
STUB

cat >"$T/bin/systemctl" <<'STUB'
#!/usr/bin/env bash
case "$*" in
*ExecMainStartTimestampMonotonic*) echo 1000000 ;;
*ExecMainExitTimestampMonotonic*) echo 58500000 ;;
*) exit 1 ;;
esac
STUB

cat >"$T/bin/curl" <<'STUB'
#!/usr/bin/env bash
set -u
mode() { # key default
	local v
	v="$(sed -n "s/^$1=//p" "$STUB_MODE" 2>/dev/null | tail -n 1)"
	printf '%s' "${v:-$2}"
}
url=""
query=""
for a in "$@"; do
	case "$a" in
	http*) url="$a" ;;
	query=*) query="${a#query=}" ;;
	esac
done
case "$url" in
*:9598/*) cat "$STUB_DIR/vector.prom" ;;
*/api/v1/query)
	printf 'vm %s\n' "$query" >>"$STUB_DIR/calls"
	[[ "$(mode VICTORIA up)" == up ]] || { echo "curl: (7) connection refused" >&2; exit 7; }
	case "$query" in
	*vector_buffer_send_duration_seconds_count*) v="$(mode VM_SENDS 2.4)" ;;
	*vector_component_discarded_events_total*) v="$(mode VM_DROPPED 263)" ;;
	*) v=empty ;;
	esac
	if [[ "$v" == empty ]]; then
		printf '{"status":"success","data":{"resultType":"vector","result":[]}}\n'
	else
		printf '{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1795200000,"%s"]}]}}\n' "$v"
	fi
	;;
*) exit 6 ;;
esac
STUB

cat >"$T/bin/docker" <<'STUB'
#!/usr/bin/env bash
set -u
mode() { # key default
	local v
	v="$(sed -n "s/^$1=//p" "$STUB_MODE" 2>/dev/null | tail -n 1)"
	printf '%s' "${v:-$2}"
}
bump() { # a counter kept in a file; prints the new count
	local f="$STUB_DIR/count.$1" n=0
	[[ -f "$f" ]] && n="$(<"$f")"
	n=$((n + 1))
	printf '%s\n' "$n" >"$f"
	printf '%s' "$n"
}
qname() { # which statement this is
	case "$1" in
	*kanban_tasks*) echo probe ;;
	*"LIKE '%session opened%'"*) echo q2 ;;
	*"GROUP BY unit"*) echo q3 ;;
	*"LIMIT 200"*) echo q1 ;;
	*) echo other ;;
	esac
}
case "${1:-}" in
inspect)
	case "${*: -1}" in
	unheaded-clickhouse) echo "true 2026-11-20T17:50:01.123456789Z" ;;
	unheaded-postgres) echo "true 2026-11-20T17:50:02.5Z" ;;
	unheaded-vector) echo "true 2026-11-20T17:50:03Z" ;;
	unheaded-logstore-writer)
		if [[ "$(mode WRITER_RUNNING true)" == true ]]; then echo "true 2026-11-20T17:50:04Z"; else echo "false 2026-11-19T00:00:00Z"; fi
		;;
	*) exit 1 ;;
	esac
	;;
exec)
	shift
	[[ "$1" == -i ]] && shift
	# <container> sh -c <script> _ <first argument>
	script="$4"
	arg="${6:-}"
	case "$script" in
	*clickhouse-client*)
		if [[ "$script" == *--time* ]]; then
			q="$(qname "$arg")"
			printf 'ch %s\n' "$q" >>"$STUB_DIR/calls"
			n="$(bump "ch.$q")"
			[[ "$n" -le "$(mode "CH_${q}_FAST_RUNS" 999)" ]] || exec sleep 30
			echo "CANARY west sshd[1]: session opened for user canary"
			printf '0.%03d\n' "$n"
		else
			case "$arg" in
			*bytes_on_disk*) echo 70000 ;;
			*) echo 1000 ;;
			esac
		fi
		;;
	*psql*)
		# A timed session, as psql answers it: nothing for a meta-command or
		# a \gset, a row and a time for a statement, the marker for \echo.
		# The stub runs every block (it does not evaluate \if). A statement
		# past <query>_FAST_RUNS is one the server cuts at the budget: it
		# takes SLEEP_S, psql prints its time all the same, then stops with
		# status 3 and no marker.
		timed=0
		i=0
		while IFS= read -r line; do
			case "$line" in
			"SET default_transaction_read_only"* | "SET statement_timeout"* | '\timing'* | '\if'* | '\endif'* | *'\gset') continue ;;
			'\o'*) timed=1; continue ;;
			'\echo '*) echo "${line#'\echo '}"; continue ;;
			esac
			if [[ "$timed" -eq 1 ]]; then
				q="$(qname "$line")"
				printf 'pg %s\n' "$q" >>"$STUB_DIR/calls"
				i=$((i + 1))
				echo "CANARY|canary.service|info|session opened for user canary"
				if [[ "$i" -gt "$(mode "PG_${q}_FAST_RUNS" 999)" ]]; then
					s="$(mode SLEEP_S 3)"
					sleep "$s"
					printf 'Time: %d.000 ms (00:0%d.000)\n' "$((s * 1000))" "$s"
					echo "ERROR:  canceling statement due to statement timeout" >&2
					exit 3
				fi
				printf 'Time: %d.500 ms (00:00.%03d)\n' "$i" "$i"
			else
				case "$line" in
				*pg_stat_statements*) echo "2909674 952131" ;;
				*inhrelid*) echo 1400000 ;;
				*to_regclass*) echo 400000 ;;
				"SELECT count(*) FROM log_events") mode PG_ROWS 1000; echo ;;
				*) echo 1000 ;;
				esac
			fi
		done
		;;
	*) exit 1 ;;
	esac
	;;
*) exit 1 ;;
esac
STUB
chmod +x "$T/bin/"*

# --- one run ------------------------------------------------------------------
TSV=""
RC=0
OUT=""
run() { # run <settings as VAR=value...> -- <mode lines...>
	local -a settings=()
	while [[ $# -gt 0 && "$1" != -- ]]; do
		settings+=("$1")
		shift
	done
	[[ $# -gt 0 ]] && shift
	: >"$T/mode"
	[[ $# -eq 0 ]] || printf '%s\n' "$@" >"$T/mode"
	rm -f "$T"/count.* "$T/calls"
	TSV="$T/readings.tsv"
	rm -f "$TSV"
	RC=0
	OUT="$(env -u RUNS -u QUERY_BUDGET_S -u LOGSTORE_TSV -u VICTORIA_URL -u VECTOR_METRICS_URL \
		PATH="$T/bin:$PATH" HOME="$T/home" STUB_DIR="$T" STUB_MODE="$T/mode" \
		LOGSTORE_PROC="$T/proc" WELL_BACKUP_DIR="$T/backups" LOGSTORE_TSV="$TSV" \
		"${settings[@]}" bash "$SCRIPT" 2>&1)" || RC=$?
}
val() { awk -F'\t' -v m="$1" '$2 == m { print $3 }' "$TSV"; }
line_of() { awk -F'\t' -v m="$1" '$2 == m { print NR }' "$TSV"; }
is_number() { [[ "$1" =~ ^-?[0-9]+(\.[0-9]+)?$ ]]; }
all_numbers() { # every named metric is in the block once, as a number
	local m v
	for m in "$@"; do
		v="$(val "$m")"
		is_number "$v" || { echo "    $m is '${v}'"; return 1; }
	done
}
unix_of() { date -u -d "$1" +%s; }
says() { [[ "$OUT" == *"$1"* ]]; } # the script's own output holds this text
asked() { grep -qF -- "$1" "$T/calls"; } # VictoriaMetrics was asked for this
only_numbers() { ! tail -n +2 "$TSV" | cut -f3 | grep -qvE '^-?[0-9]+(\.[0-9]+)?$'; }
refused_unwritten() { [[ "$RC" -eq 2 && ! -e "$TSV" ]]; }

WEEK1=(ch_rows_yesterday pg_rows_yesterday ch_rows_7d pg_rows_7d ch_bytes ch_rows pg_bytes pg_rows pg_bytes_yesterday
	ch_q1_last_hour_p50_ms ch_q1_last_hour_p95_ms pg_q1_last_hour_p50_ms pg_q1_last_hour_p95_ms
	ch_q2_text_24h_p50_ms ch_q2_text_24h_p95_ms pg_q2_text_24h_p50_ms pg_q2_text_24h_p95_ms
	ch_q3_units_7d_p50_ms ch_q3_units_7d_p95_ms pg_q3_units_7d_p50_ms pg_q3_units_7d_p95_ms
	well_probe_p50_ms well_probe_p95_ms well_calls_total well_exec_ms_total backup_seconds backup_bytes
	vector_record_buffer_events vector_record_buffer_bytes vector_record_sends_over_1s_total vector_well_dropped_total)
NEW=(ch_q1_last_hour_runs pg_q1_last_hour_runs ch_q2_text_24h_runs pg_q2_text_24h_runs ch_q3_units_7d_runs pg_q3_units_7d_runs
	well_probe_first_p50_ms well_probe_first_p95_ms vector_record_sends_over_1s_24h vector_well_dropped_24h
	ch_started_unix pg_started_unix vector_started_unix writer_started_unix host_boot_unix io_some_avg300)

# --- 1. all good --------------------------------------------------------------
run
check "1 all good: exit 0" test "$RC" -eq 0
check "1 all good: [PASS]" says "[PASS] reading "
check "1 every metric of week 1 is a number" all_numbers "${WEEK1[@]}"
check "1 every new metric is a number" all_numbers "${NEW[@]}"
check "1 one line a metric, under one header" test "$(wc -l <"$TSV")" -eq $((1 + ${#WEEK1[@]} + ${#NEW[@]}))
check "1 twenty runs: p50 is the 10th, p95 the 19th (Postgres, ms)" test "$(val pg_q1_last_hour_p50_ms) $(val pg_q1_last_hour_p95_ms)" = "10.500 19.500"
check "1 twenty runs: p50 is the 10th, p95 the 19th (ClickHouse, s to ms)" test "$(val ch_q3_units_7d_p50_ms) $(val ch_q3_units_7d_p95_ms)" = "10.000 19.000"
check "1 the run counts are RUNS" test "$(val ch_q2_text_24h_runs) $(val pg_q3_units_7d_runs)" = "20 20"
check "1 sends over 1 s since Vector's start" test "$(val vector_record_sends_over_1s_total)" = 2
check "1 sends over 1 s in 24 h, rounded" test "$(val vector_record_sends_over_1s_24h)" = 2
check "1 rows dropped in 24 h" test "$(val vector_well_dropped_24h)" = 263
check "1 the 24 h sends query names the record sink and the window" asked 'increase(vector_buffer_send_duration_seconds_count{component_id="clickhouse"}[24h])'
check "1 the 24 h drops query names the Well sink and the window" asked 'increase(vector_component_discarded_events_total{component_id="well_logs"}[24h])'
check "1 start times as Unix seconds" test "$(val ch_started_unix) $(val pg_started_unix) $(val vector_started_unix) $(val writer_started_unix)" = \
	"$(unix_of 2026-11-20T17:50:01Z) $(unix_of 2026-11-20T17:50:02Z) $(unix_of 2026-11-20T17:50:03Z) $(unix_of 2026-11-20T17:50:04Z)"
check "1 the boot time and the I/O pressure" test "$(val host_boot_unix) $(val io_some_avg300)" = "1795225392 6.07"
check "1 the backup: seconds and bytes" test "$(val backup_seconds) $(val backup_bytes)" = "57.500 101860940"

# --- 4. the Well is probed before it is loaded (same run as case 1) -----------
earlier() { # earlier <line> <line>: both present, the first above the second
	[[ "$1" =~ ^[0-9]+$ && "$2" =~ ^[0-9]+$ && "$1" -lt "$2" ]]
}
check "4 well_probe_first_p95_ms is written before ch_q1_last_hour_p50_ms" earlier "$(line_of well_probe_first_p95_ms)" "$(line_of ch_q1_last_hour_p50_ms)"
check "4 well_probe_p95_ms is written after pg_q3_units_7d_p95_ms" earlier "$(line_of pg_q3_units_7d_p95_ms)" "$(line_of well_probe_p95_ms)"
first_probe="$(grep -n '^pg probe$' "$T/calls" | head -n 1 | cut -d: -f1 || true)"
first_query="$(grep -n -E '^(ch|pg) q[123]$' "$T/calls" | head -n 1 | cut -d: -f1 || true)"
last_probe="$(grep -n '^pg probe$' "$T/calls" | tail -n 1 | cut -d: -f1 || true)"
last_query="$(grep -n -E '^(ch|pg) q[123]$' "$T/calls" | tail -n 1 | cut -d: -f1 || true)"
check "4 the first probe statement reaches the Well before any of the three queries" earlier "$first_probe" "$first_query"
check "4 the second probe statement reaches it after the last of them" earlier "$last_query" "$last_probe"

# --- 6. nothing but numbers reaches the file (same run, then a leaking count) --
check "6 no row text in the readings file" test "$(grep -c CANARY "$TSV" || true)" -eq 0
check "6 every value is a number" only_numbers
run RUNS=5 -- "PG_ROWS=CANARY row text where a count belongs"
check "6 a count that is text: NA, exit 1" test "$RC $(val pg_rows)" = "1 NA"
check "6 and still no row text" test "$(grep -c CANARY "$TSV" || true)" -eq 0

# --- 2. a slow query is a number ----------------------------------------------
run RUNS=8 QUERY_BUDGET_S=2 QUERY_BUDGET_MIN_S=1 -- PG_q3_FAST_RUNS=6
check "2 six runs inside the budget: pg_q3_units_7d_runs is 6" test "$(val pg_q3_units_7d_runs)" = 6
check "2 p50 and p95 are numbers" all_numbers pg_q3_units_7d_p50_ms pg_q3_units_7d_p95_ms
check "2 from the six that finished" test "$(val pg_q3_units_7d_p50_ms) $(val pg_q3_units_7d_p95_ms)" = "3.500 6.500"
check "2 exit 0" test "$RC" -eq 0
check "2 the other queries ran all eight" test "$(val pg_q2_text_24h_runs) $(val ch_q3_units_7d_runs)" = "8 8"

# --- 3. fewer than five runs is NA ---------------------------------------------
run RUNS=8 QUERY_BUDGET_S=2 QUERY_BUDGET_MIN_S=1 -- PG_q3_FAST_RUNS=2
check "3 two runs: pg_q3_units_7d_runs is 2" test "$(val pg_q3_units_7d_runs)" = 2
check "3 p50 and p95 are NA" test "$(val pg_q3_units_7d_p50_ms) $(val pg_q3_units_7d_p95_ms)" = "NA NA"
check "3 exit 1" test "$RC" -eq 1
check "3 [FAIL] is said" says "[FAIL] reading "
check "3 the second probe still ran" all_numbers well_probe_p50_ms well_probe_p95_ms
run RUNS=8 QUERY_BUDGET_S=2 QUERY_BUDGET_MIN_S=1 -- CH_q2_FAST_RUNS=3
check "3 the same on the ClickHouse side" test "$(val ch_q2_text_24h_runs) $(val ch_q2_text_24h_p95_ms) $RC" = "3 NA 1"

# --- 5. VictoriaMetrics -------------------------------------------------------
run RUNS=5 -- VICTORIA=down
check "5 unreachable: both 24 h metrics are NA" test "$(val vector_record_sends_over_1s_24h) $(val vector_well_dropped_24h)" = "NA NA"
check "5 the raw counters are numbers" all_numbers vector_record_buffer_events vector_record_buffer_bytes vector_record_sends_over_1s_total vector_well_dropped_total
check "5 exit 1" test "$RC" -eq 1
run RUNS=5 -- VM_DROPPED=empty
check "5 no drop series while Vector's others answer: 0, exit 0" test "$(val vector_well_dropped_24h) $(val vector_record_sends_over_1s_24h) $RC" = "0 2 0"
run RUNS=5 -- VM_DROPPED=empty VM_SENDS=empty
check "5 no Vector series at all: NA, never a zero" test "$(val vector_well_dropped_24h) $(val vector_record_sends_over_1s_24h) $RC" = "NA NA 1"

# --- 7. the writer stopped (the baseline) --------------------------------------
run RUNS=5 -- WRITER_RUNNING=false
check "7 writer stopped: writer_started_unix is NA and the reading passes" test "$(val writer_started_unix) $RC" = "NA 0"

# --- 8. settings ---------------------------------------------------------------
run QUERY_BUDGET_S=10
check "8 a budget under the floor exits 2 and writes nothing" refused_unwritten
run QUERY_BUDGET_S=abc
check "8 a budget that is not a number exits 2" test "$RC" -eq 2
run RUNS=3
check "8 fewer than five runs asked for exits 2" test "$RC" -eq 2

# --- 9. where the readings go by default ---------------------------------------
: >"$T/mode"
rm -f "$T"/count.* "$T/calls"
env -u LOGSTORE_TSV -u RUNS -u QUERY_BUDGET_S PATH="$T/bin:$PATH" HOME="$T/home" STUB_DIR="$T" STUB_MODE="$T/mode" \
	LOGSTORE_PROC="$T/proc" WELL_BACKUP_DIR="$T/backups" RUNS=5 bash "$SCRIPT" >/dev/null 2>&1 || true
check "9 the default file is week 1's" test -s "$T/home/.local/state/unheaded/S-E1-LIVE-logstore.tsv"

echo "test-logstore-measure: ${pass} passed, ${fail} failed"
[[ "$fail" -eq 0 ]]
