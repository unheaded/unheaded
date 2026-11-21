#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# boot-event.sh: each boot of this host as its own event (decision pack
# 2026-11-18 row 22, ADR-150 choice 15; ADR-147's finding "an unplanned boot
# is its own event"). The stack does not start at boot, so Prometheus,
# Alertmanager and the mailer are down when the host comes up and nothing can
# alert then. This script writes the boot down on the host; alert-mailer reads
# the record when it next runs and puts one line in the daily digest
# (cmd/alert-mailer/boot.go, wiki/Alerting.md).
#
#   boot-event.sh record <state dir> <textfile dir>   at boot (ExecStart)
#   boot-event.sh shutdown <state dir>                at stop (ExecStop)
#
# record appends one JSON line to <state dir>/boots.jsonl (the newest
# BOOT_EVENT_KEEP are kept) and writes <textfile dir>/host_boot.prom for
# huginn. Run twice in one boot it adds nothing. One line holds:
#   boot_id        the kernel's boot id
#   booted_at      btime of /proc/stat, Unix seconds
#   recorded_at    when this ran
#   host           BOOT_EVENT_HOST
#   previous       how the boot before this one ended:
#                    planned  an orderly shutdown with a planned-stop mark for
#                             "all" open at that time (planned-stop.sh)
#                    clean    an orderly shutdown, no such mark
#                    unclean  no orderly shutdown was recorded: power loss,
#                             a hang, a long press of the power key
#                    unknown  no earlier boot on record
#   shutdown_at    when the orderly shutdown was stamped (0: none)
#   stack          up (every container of the stack's projects running),
#                  down (none), partial, or unknown (Docker did not answer),
#                  when the boot was recorded
#   stack_running, stack_total, stack_projects
#
# shutdown writes <state dir>/clean-shutdown ("<boot id> <Unix time>") when
# systemd says the system is stopping or does not answer, and nothing when
# systemd says the system is up (the unit stopped by hand). record reads the
# stamp and removes it: a boot that finds no stamp for the last recorded
# boot followed an unclean end. The stamp is written when this unit is
# stopped, early in the shutdown: a shutdown that hangs after that point
# still reads clean.
#
# Settings (environment; the unit reads /etc/unheaded/host-settings.env,
# wiki/Host-Settings.md):
#   BOOT_EVENT_HOST            host name in the record (default: hostname -s)
#   BOOT_EVENT_STACK_PROJECTS  compose projects that are "the stack"
#                              (default "unheaded-dev": the monitoring project
#                              is left out, so the mailer started alone to get
#                              the digest out does not read as "stack up")
#   BOOT_EVENT_ONESHOT         compose services skipped: they exit 0 by design
#                              (default "volume-init clickhouse-schema", as
#                              HOST_STATE_ONESHOT of host-state-textfile.sh)
#   BOOT_EVENT_KEEP            boots kept in the history (default 32, 1..500)
#   BOOT_EVENT_DOCKER_WAIT     seconds Docker gets to answer, per project
#                              (default 20, 1..50; the unit's start timeout
#                              is 60 s); past it the stack is "unknown"
#   BOOT_EVENT_PROC            where /proc is (default /proc; the test's fake)
#
# Writes (host is added by huginn):
#   unheaded_host_boot_timestamp_seconds
#   unheaded_host_boot_previous_shutdown{state}   1 on the state that applies
#   unheaded_host_boot_stack_up                   1 if the stack was up when
#                                                 the boot was recorded (no
#                                                 line when Docker did not answer)
#
# Both files are replaced atomically (temp file in the same directory, then
# mv), mode 0644: the mailer's container reads boots.jsonl through a
# read-only mount. Exit 2 on a bad setting or usage, 1 when /proc or the
# directories cannot be read; Docker not answering is not a failure.
set -euo pipefail

usage() { sed -n '13,14p' "$0" >&2; exit 2; }
bad() { echo "boot-event: $*" >&2; exit 2; }

proc="${BOOT_EVENT_PROC:-/proc}"
boot_id_re='^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'

# this_boot_id: the kernel's boot id, checked.
this_boot_id() {
    local id
    id="$(cat "${proc}/sys/kernel/random/boot_id")"
    [[ "$id" =~ $boot_id_re ]] || { echo "boot-event: ${proc}/sys/kernel/random/boot_id is not a boot id" >&2; return 1; }
    printf '%s\n' "$id"
}

# write_textfile <dir> <record json>: host_boot.prom from one record.
write_textfile() {
    local dir="$1" rec="$2" tmp booted previous stack s
    booted="$(jq -r '.booted_at' <<<"$rec")"
    previous="$(jq -r '.previous' <<<"$rec")"
    stack="$(jq -r '.stack' <<<"$rec")"
    tmp="$(mktemp "${dir}/.host_boot.XXXXXX")"
    # shellcheck disable=SC2064 # the name is fixed now, on purpose
    trap "rm -f '$tmp'" EXIT
    {
        echo "# HELP unheaded_host_boot_timestamp_seconds When this host booted (scripts/monitoring/boot-event.sh)"
        echo "# TYPE unheaded_host_boot_timestamp_seconds gauge"
        echo "unheaded_host_boot_timestamp_seconds ${booted}"
        echo "# HELP unheaded_host_boot_previous_shutdown How the boot before this one ended: 1 on planned, clean, unclean or unknown"
        echo "# TYPE unheaded_host_boot_previous_shutdown gauge"
        for s in planned clean unclean unknown; do
            echo "unheaded_host_boot_previous_shutdown{state=\"${s}\"} $([[ "$s" == "$previous" ]] && echo 1 || echo 0)"
        done
        if [[ "$stack" != unknown ]]; then
            echo "# HELP unheaded_host_boot_stack_up 1 if every container of the stack's compose projects was running when the boot was recorded"
            echo "# TYPE unheaded_host_boot_stack_up gauge"
            echo "unheaded_host_boot_stack_up $([[ "$stack" == up ]] && echo 1 || echo 0)"
        fi
    } >"$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "${dir}/host_boot.prom"
    trap - EXIT
}

# stack_state: prints "<state> <running> <total>" for the stack's projects.
stack_state() {
    local projects="$1" oneshot="$2" wait="$3" p out running=0 total=0 svc state
    command -v docker >/dev/null || { echo "unknown 0 0"; return 0; }
    for p in $projects; do
        # Bounded: a Docker that hangs must not cost the record of the boot.
        out="$(timeout "$wait" docker ps -a --filter "label=com.docker.compose.project=${p}" \
            --format '{{.Label "com.docker.compose.service"}} {{.State}}' 2>/dev/null)" || { echo "unknown 0 0"; return 0; }
        while read -r svc state; do
            [[ -n "$svc" ]] || continue
            [[ " $oneshot " == *" $svc "* ]] && continue
            total=$((total + 1))
            [[ "$state" == running ]] && running=$((running + 1))
        done <<<"$out"
    done
    if ((running == 0)); then
        echo "down 0 ${total}"
    elif ((running == total)); then
        echo "up ${running} ${total}"
    else
        echo "partial ${running} ${total}"
    fi
}

record() {
    local state="$1" textfile="$2"
    local host projects oneshot keep wait
    host="${BOOT_EVENT_HOST:-$(hostname -s)}"
    projects="${BOOT_EVENT_STACK_PROJECTS:-unheaded-dev}"
    oneshot="${BOOT_EVENT_ONESHOT:-volume-init clickhouse-schema}"
    keep="${BOOT_EVENT_KEEP:-32}"
    wait="${BOOT_EVENT_DOCKER_WAIT:-20}"
    [[ "$wait" =~ ^[0-9]{1,2}$ ]] && ((10#$wait >= 1 && 10#$wait <= 50)) || bad "BOOT_EVENT_DOCKER_WAIT '$wait': want 1..50 (seconds)"
    [[ "$host" =~ ^[A-Za-z0-9][A-Za-z0-9.-]{0,62}$ ]] || bad "BOOT_EVENT_HOST '$host': want a host name"
    [[ "$projects" =~ ^[a-z0-9][a-z0-9\ _.-]*$ ]] || bad "BOOT_EVENT_STACK_PROJECTS '$projects' has characters outside [a-z0-9 _.-]"
    [[ "$oneshot" =~ ^[a-z0-9\ _.-]*$ ]] || bad "BOOT_EVENT_ONESHOT '$oneshot' has characters outside [a-z0-9 _.-]"
    [[ "$keep" =~ ^[0-9]{1,3}$ ]] && ((10#$keep >= 1 && 10#$keep <= 500)) || bad "BOOT_EVENT_KEEP '$keep': want 1..500"
    keep=$((10#$keep))
    command -v jq >/dev/null || bad "jq is required"
    [[ -d "$state" ]] || { echo "boot-event: $state is not a directory" >&2; exit 1; }
    [[ -d "$textfile" ]] || { echo "boot-event: $textfile is not a directory" >&2; exit 1; }

    local history="${state}/boots.jsonl" stamp="${state}/clean-shutdown"
    local boot_id booted now
    boot_id="$(this_boot_id)"
    booted="$(awk '$1 == "btime" { print $2 }' "${proc}/stat")"
    [[ "$booted" =~ ^[0-9]{1,12}$ ]] || { echo "boot-event: no btime in ${proc}/stat" >&2; exit 1; }
    now="$(date +%s)"

    # The last boot on record; a line that is not a record counts as none.
    local last="" last_id=""
    if [[ -s "$history" ]]; then
        last="$(tail -n 1 "$history")"
        last_id="$(jq -r '.boot_id // empty' <<<"$last" 2>/dev/null)" || last_id=""
    fi
    if [[ "$last_id" == "$boot_id" ]]; then
        # A stamp found now was a stop of the unit, not a shutdown.
        rm -f "$stamp"
        write_textfile "$textfile" "$last"
        echo "boot-event: boot ${boot_id} is already recorded"
        return 0
    fi

    # How the boot before this one ended.
    local previous=unknown shutdown_at=0 s_id="" s_at=""
    if [[ -n "$last_id" ]]; then
        previous=unclean
        if [[ -f "$stamp" ]]; then
            read -r s_id s_at <"$stamp" || true
            if [[ "$s_id" == "$last_id" && "$s_at" =~ ^[0-9]{1,12}$ ]]; then
                previous=clean
                shutdown_at="$s_at"
                # A planned-stop mark for "all" that was open then.
                if [[ -f "${textfile}/planned_stop.prom" ]] &&
                    sed -n 's/^unheaded_planned_stop_until_timestamp_seconds{scope="all"} \([0-9][0-9]*\)$/\1/p' "${textfile}/planned_stop.prom" |
                    awk -v t="$shutdown_at" '$1 >= t { f = 1 } END { exit !f }'; then
                    previous=planned
                fi
            fi
        fi
    fi
    rm -f "$stamp"

    local stack running total
    read -r stack running total <<<"$(stack_state "$projects" "$oneshot" "$wait")"

    local rec tmp
    rec="$(jq -c -n --arg boot_id "$boot_id" --argjson booted_at "$booted" --argjson recorded_at "$now" \
        --arg host "$host" --arg previous "$previous" --argjson shutdown_at "$shutdown_at" \
        --arg stack "$stack" --argjson stack_running "$running" --argjson stack_total "$total" --arg projects "$projects" \
        '{boot_id: $boot_id, booted_at: $booted_at, recorded_at: $recorded_at, host: $host,
          previous: $previous, shutdown_at: $shutdown_at, stack: $stack,
          stack_running: $stack_running, stack_total: $stack_total,
          stack_projects: ($projects | split(" ") | map(select(. != "")))}')"
    tmp="$(mktemp "${state}/.boots.XXXXXX")"
    # shellcheck disable=SC2064 # the name is fixed now, on purpose
    trap "rm -f '$tmp'" EXIT
    {
        [[ -s "$history" ]] && ((keep > 1)) && tail -n "$((keep - 1))" "$history"
        printf '%s\n' "$rec"
    } >"$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "$history"
    trap - EXIT

    write_textfile "$textfile" "$rec"
    echo "boot-event: ${host} booted $(date -u -d "@${booted}" +%Y-%m-%dT%H:%M:%SZ) (boot ${boot_id}); previous shutdown ${previous}; stack ${stack} (${running} of ${total} running)"
}

shutdown() {
    local state="$1" sys boot_id tmp
    [[ -d "$state" ]] || { echo "boot-event: $state is not a directory" >&2; exit 1; }
    # "stopping" while systemd shuts the system down (it goes before
    # "degraded"); is-system-running exits non-zero for every state but
    # running. A state that says the system is up means the unit was stopped
    # by hand: no stamp. No answer (systemd busy in a shutdown) is taken as a
    # shutdown, because a stop by hand is rare and a false "unclean" after
    # every reboot would be the worse error.
    sys="$(timeout 5 systemctl is-system-running 2>/dev/null || true)"
    case "$sys" in
    running | degraded | starting | initializing | maintenance)
        echo "boot-event: the system is not shutting down (${sys}): no stamp"
        return 0
        ;;
    esac
    boot_id="$(this_boot_id)"
    tmp="$(mktemp "${state}/.clean-shutdown.XXXXXX")"
    printf '%s %s\n' "$boot_id" "$(date +%s)" >"$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "${state}/clean-shutdown"
    echo "boot-event: orderly shutdown of boot ${boot_id} stamped"
}

case "${1:-}" in
record)
    [[ $# -eq 3 ]] || usage
    record "$2" "$3"
    ;;
shutdown)
    [[ $# -eq 2 ]] || usage
    shutdown "$2"
    ;;
*)
    usage
    ;;
esac
