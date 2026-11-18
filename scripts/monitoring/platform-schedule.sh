#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# The clock for platform checks (ADR-149, ADR-147 decision 15 (a)). A systemd
# timer runs this; it runs one runbook through run-runbook.py and leaves three
# series in node_exporter's textfile directory, which Huginn forwards:
#
#   unheaded_schedule_last_run_timestamp_seconds{schedule="<name>"}
#   unheaded_schedule_last_exit_code{schedule="<name>"}
#   unheaded_schedule_last_success_timestamp_seconds{schedule="<name>"}
#
# The success time survives a failed run (read back from the previous file).
# A first run that fails starts the clock at that run, so a "not succeeding"
# alert counts from the install, not from 1970.
#
# Usage: platform-schedule.sh <name> <runbook.yaml> <textfile-dir>
#   name: [a-z0-9-]{1,64}. Exit: the runner's code; 2 on a usage error.
#   PLATFORM_SCHEDULE_RUNNER overrides the runner (tests use a stub).
#
# It runs as its own nologin account (unheaded-health-sweep,
# unheaded-ioc-audit), never as root.
set -uo pipefail

if [ "$#" -ne 3 ]; then
    echo "usage: platform-schedule.sh <name> <runbook.yaml> <textfile-dir>" >&2
    exit 2
fi
name="$1" runbook="$2" dir="$3"

if ! [[ "$name" =~ ^[a-z0-9-]{1,64}$ ]]; then
    echo "platform-schedule: bad schedule name: '$name' (want [a-z0-9-]{1,64})" >&2
    exit 2
fi
if [ ! -f "$runbook" ] || [ ! -r "$runbook" ]; then
    echo "platform-schedule: runbook not readable: $runbook" >&2
    exit 2
fi
if [ ! -d "$dir" ] || [ ! -w "$dir" ]; then
    echo "platform-schedule: textfile directory not writable: $dir" >&2
    exit 2
fi

runner="${PLATFORM_SCHEDULE_RUNNER:-/usr/local/lib/unheaded/schedules/run-runbook.py}"
if [[ "$runner" == *.py ]]; then
    python3 -B "$runner" "$runbook"
    rc=$?
else
    "$runner" "$runbook"
    rc=$?
fi

now="$(date +%s)"
out="${dir}/schedule_${name}.prom"
success="$now"
if [ "$rc" -ne 0 ]; then
    old=""
    if [ -r "$out" ]; then
        old="$(awk -v n="$name" '$1 == "unheaded_schedule_last_success_timestamp_seconds{schedule=\"" n "\"}" {print $2}' "$out" 2>/dev/null | tail -1)"
    fi
    if [[ "$old" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
        success="$old"
    fi
fi

tmp="${dir}/.schedule_${name}.tmp"
trap 'rm -f "$tmp"' EXIT
{
    echo "# HELP unheaded_schedule_last_run_timestamp_seconds When the platform schedule last ran (ADR-149)."
    echo "# TYPE unheaded_schedule_last_run_timestamp_seconds gauge"
    echo "unheaded_schedule_last_run_timestamp_seconds{schedule=\"${name}\"} ${now}"
    echo "# HELP unheaded_schedule_last_exit_code Exit code of the platform schedule's last run."
    echo "# TYPE unheaded_schedule_last_exit_code gauge"
    echo "unheaded_schedule_last_exit_code{schedule=\"${name}\"} ${rc}"
    echo "# HELP unheaded_schedule_last_success_timestamp_seconds When the platform schedule last succeeded."
    echo "# TYPE unheaded_schedule_last_success_timestamp_seconds gauge"
    echo "unheaded_schedule_last_success_timestamp_seconds{schedule=\"${name}\"} ${success}"
} > "$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "$out"
exit "$rc"
