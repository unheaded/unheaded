#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# host-state-textfile.sh: what every watched container and long-running unit
# is doing, for huginn's textfile collector (ADR-147 decisions 1 and 2,
# S-SEE-AND-SURVIVE Phase 5). Restart policy on west stays "no", so a
# container that stops stays stopped: ContainerStopped and UnitFailed
# (monitoring/prometheus/rules/{containers,resources}.yml) read this file.
#
# Root, from unheaded-host-state.timer every 30 s: the Docker socket is
# root-equivalent anyway (as the Well backup's unit says), and the script
# only reads (docker ps, docker inspect, systemctl show).
#
# Usage: host-state-textfile.sh <dir>     writes <dir>/host_state.prom
#   HOST_STATE_PROJECTS  compose projects watched (default "unheaded-dev monitoring")
#   HOST_STATE_ONESHOT   compose services skipped: they exit 0 by design
#                        (default "volume-init clickhouse-schema")
#   HOST_STATE_UNITS     unit names or patterns for systemctl show
#                        (default below); Type=oneshot and not-found are skipped
#
# Writes (host is added by huginn):
#   unheaded_container_running{name,project,service}   1 running, 0 not
#   unheaded_container_exit_code{name}                 last exit code
#   unheaded_container_oom_killed{name}                1 if Docker says OOMKilled
#   unheaded_container_healthy{name}                   1 healthy (only with a healthcheck)
#   unheaded_container_started_timestamp_seconds{name}
#   unheaded_container_info{name,container_id}         1 (12-hex id)
#   unheaded_unit_active{unit}, unheaded_unit_failed{unit}, unheaded_unit_restarts{unit}
#   unheaded_host_state_last_run_timestamp_seconds, unheaded_host_state_docker_ok
#
# Docker unreachable: units are still written, docker_ok is 0, no container
# lines (HostStateStale fires on docker_ok 0). The file is replaced
# atomically (temp file in the same directory, then mv), mode 0644.
set -euo pipefail

dir="${1:?usage: host-state-textfile.sh <textfile dir>}"
projects="${HOST_STATE_PROJECTS:-unheaded-dev monitoring}"
oneshot="${HOST_STATE_ONESHOT:-volume-init clickhouse-schema}"
units="${HOST_STATE_UNITS:-unheaded-*.service huginn.service zhen-*.service docker.service containerd.service nginx.service}"
for v in "$projects" "$oneshot" "$units"; do
    [[ "$v" =~ ^[a-z0-9\ _.*-]+$ ]] || { echo "host-state: setting '$v' has characters outside [a-z0-9 _.*-]" >&2; exit 2; }
done
command -v jq >/dev/null || { echo "host-state: jq is required" >&2; exit 2; }
[[ -d "$dir" ]] || { echo "host-state: $dir is not a directory" >&2; exit 2; }

# A label value: backslash, quote and newline escaped (jq side: esc below).
docker_ok=1
containers=""
if ids="$(docker ps -aq --filter label=com.docker.compose.project 2>/dev/null)"; then
    if [[ -n "$ids" ]]; then
        # shellcheck disable=SC2086 # one id per word
        if json="$(docker inspect $ids 2>/dev/null)"; then
            containers="$(jq -r --arg projects "$projects" --arg oneshot "$oneshot" '
              def esc: tostring | gsub("\\\\"; "\\\\") | gsub("\""; "\\\"") | gsub("\n"; "\\n");
              ($projects | split(" ") | map(select(. != ""))) as $p
              | ($oneshot | split(" ") | map(select(. != ""))) as $o
              | .[]
              | (.Config.Labels["com.docker.compose.project"] // "") as $proj
              | (.Config.Labels["com.docker.compose.service"] // "") as $svc
              | select(($p | index($proj)) != null and ($o | index($svc)) == null)
              | (.Name | ltrimstr("/") | esc) as $n
              | "unheaded_container_running{name=\"\($n)\",project=\"\($proj | esc)\",service=\"\($svc | esc)\"} \(if .State.Running then 1 else 0 end)",
                "unheaded_container_exit_code{name=\"\($n)\"} \(.State.ExitCode // 0)",
                "unheaded_container_oom_killed{name=\"\($n)\"} \(if .State.OOMKilled then 1 else 0 end)",
                (if .State.Health then "unheaded_container_healthy{name=\"\($n)\"} \(if .State.Health.Status == "healthy" then 1 else 0 end)" else empty end),
                ((.State.StartedAt // "" | sub("\\.[0-9]+"; "") | (try fromdateiso8601 catch 0)) as $s
                  | if $s > 0 then "unheaded_container_started_timestamp_seconds{name=\"\($n)\"} \($s)" else empty end),
                "unheaded_container_info{name=\"\($n)\",container_id=\"\(.Id[0:12] | esc)\"} 1"
            ' <<<"$json")" || docker_ok=0
        else
            docker_ok=0
        fi
    fi
else
    docker_ok=0
fi
[[ "$docker_ok" == 1 ]] || containers=""

# systemctl show: blocks of Key=Value separated by blank lines.
# shellcheck disable=SC2086 # the patterns are words
unit_lines="$(systemctl show --property=Id,Type,LoadState,ActiveState,NRestarts $units 2>/dev/null | awk '
  function flush() {
    if (id != "" && type != "oneshot" && load != "not-found" && id ~ /^[A-Za-z0-9@:._-]+$/) {
      printf "unheaded_unit_active{unit=\"%s\"} %d\n", id, (active == "active")
      printf "unheaded_unit_failed{unit=\"%s\"} %d\n", id, (active == "failed")
      printf "unheaded_unit_restarts{unit=\"%s\"} %d\n", id, restarts + 0
    }
    id = type = load = active = ""; restarts = 0
  }
  /^$/ { flush(); next }
  { k = $0; sub(/=.*/, "", k); v = substr($0, length(k) + 2) }
  k == "Id" { id = v } k == "Type" { type = v } k == "LoadState" { load = v }
  k == "ActiveState" { active = v } k == "NRestarts" { restarts = v }
  END { flush() }
')" || unit_lines=""

tmp="$(mktemp "${dir}/.host_state.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
{
    echo "# HELP unheaded_container_running 1 if the compose container is running (host-state-textfile.sh)"
    echo "# TYPE unheaded_container_running gauge"
    [[ -n "$containers" ]] && printf '%s\n' "$containers"
    echo "# HELP unheaded_unit_failed 1 if the unit is in the failed state"
    echo "# TYPE unheaded_unit_failed gauge"
    [[ -n "$unit_lines" ]] && printf '%s\n' "$unit_lines"
    echo "# HELP unheaded_host_state_last_run_timestamp_seconds When host-state-textfile.sh last ran"
    echo "# TYPE unheaded_host_state_last_run_timestamp_seconds gauge"
    echo "unheaded_host_state_last_run_timestamp_seconds $(date +%s)"
    echo "# HELP unheaded_host_state_docker_ok 1 if Docker answered"
    echo "# TYPE unheaded_host_state_docker_ok gauge"
    echo "unheaded_host_state_docker_ok $docker_ok"
} >"$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "${dir}/host_state.prom"
trap - EXIT
