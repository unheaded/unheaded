#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# planned-stop.sh: mark a stop as planned, so it spends no error budget
# (ADR-147 decision 4, S-SEE-AND-SURVIVE Phase 8).
#
#   scripts/monitoring/planned-stop.sh all 30m          # the whole stack, 30 minutes
#   scripts/monitoring/planned-stop.sh wotan 2h         # one SLO, or one container by name
#   scripts/monitoring/planned-stop.sh --list           # active marks: scope and UTC end
#   scripts/monitoring/planned-stop.sh --end wotan      # end one mark now
#   scripts/monitoring/planned-stop.sh --end            # end every mark now
#
# It writes unheaded_planned_stop_until_timestamp_seconds{scope} into
# planned_stop.prom in huginn's textfile directory (PLANNED_STOP_DIR,
# default /var/lib/unheaded/textfile); huginn forwards it. Every SLO's
# slo:good counts a minute under a mark for its name or "all" as good
# (cmd/alert-policy), and ContainerStopped stays quiet for a container
# under a mark for its name or "all" (rules/containers.yml). A silence
# (maintenance.sh) only stops mail; a mark stops the budget burning.
# maintenance.sh --planned and kingdom-shutdown.sh / kingdom-startup.sh call
# this.
#
# The directory is root's: as a normal user the write goes through
# PLANNED_STOP_SUDO (default "sudo -n"; empty writes directly, for tests).
# Scope: [a-z0-9][a-z0-9_.-]{0,63}. Duration: <n>m, <n>h or <n>d, at most 30d.
# A mark past its time is dropped on the next write.
set -euo pipefail

dir="${PLANNED_STOP_DIR:-/var/lib/unheaded/textfile}"
file="${dir}/planned_stop.prom"
metric="unheaded_planned_stop_until_timestamp_seconds"
scope_re='^[a-z0-9][a-z0-9_.-]{0,63}$'
usage() { sed -n '5,13p' "$0"; }
bad() { echo "planned-stop: $*" >&2; exit 2; }

# marks: "scope until" per line, from the current file, past ones dropped.
marks() {
    local now
    now="$(date +%s)"
    [[ -f "$file" ]] || return 0
    sed -n "s/^${metric}{scope=\"\\([a-z0-9][a-z0-9_.-]*\\)\"} \\([0-9][0-9]*\\)\$/\\1 \\2/p" "$file" |
        awk -v now="$now" '$2 > now'
}

# write <marks text>: the whole file, atomically, through PLANNED_STOP_SUDO.
write() {
    local tmp sudo
    tmp="$(mktemp)"
    trap 'rm -f "$tmp"' RETURN
    {
        echo "# HELP ${metric} Until when a stop of this scope (all, an SLO or a container) is planned (scripts/monitoring/planned-stop.sh)"
        echo "# TYPE ${metric} gauge"
        [[ -n "$1" ]] && awk -v m="$metric" '{ printf "%s{scope=\"%s\"} %s\n", m, $1, $2 }' <<<"$1"
    } >"$tmp"
    if [[ "$(id -u)" -eq 0 ]]; then
        sudo=""
    else
        sudo="${PLANNED_STOP_SUDO-sudo -n}"
    fi
    # shellcheck disable=SC2086 # sudo is "" or a command and its flag
    if ! $sudo install -m 0644 "$tmp" "${dir}/.planned_stop.tmp" || ! $sudo mv -f "${dir}/.planned_stop.tmp" "$file"; then
        echo "planned-stop: cannot write ${file}: run with sudo" >&2
        exit 1
    fi
}

case "${1:-}" in
"" | -h | --help)
    usage
    ;;
--list)
    marks | while read -r s u; do
        echo "$s $(date -u -d "@$u" +%Y-%m-%dT%H:%M:%SZ)"
    done
    ;;
--end)
    if [[ -n "${2:-}" ]]; then
        [[ "$2" =~ $scope_re ]] || bad "scope '$2': want [a-z0-9][a-z0-9_.-]{0,63}"
        write "$(marks | awk -v s="$2" '$1 != s')"
        echo "[ok] ended the planned stop of $2"
    else
        write ""
        echo "[ok] ended every planned stop"
    fi
    ;;
*)
    scope="$1"
    dur="${2:-}"
    [[ "$scope" =~ $scope_re ]] || bad "scope '$scope': want all, an SLO or a container name ([a-z0-9][a-z0-9_.-]{0,63})"
    [[ "$dur" =~ ^([0-9]+)(m|h|d)$ ]] || bad "duration '$dur': want <n>m, <n>h or <n>d"
    n="${BASH_REMATCH[1]}"
    case "${BASH_REMATCH[2]}" in
        m) secs=$((10#$n * 60)) ;;
        h) secs=$((10#$n * 3600)) ;;
        d) secs=$((10#$n * 86400)) ;;
    esac
    ((secs > 0 && secs <= 30 * 86400)) || bad "duration '$dur': at most 30d"
    until=$(($(date +%s) + secs))
    write "$( { marks | awk -v s="$scope" '$1 != s'; echo "$scope $until"; } | sed '/^$/d')"
    echo "[ok] planned stop of ${scope} until $(date -u -d "@$until" +%Y-%m-%dT%H:%M:%SZ)"
    ;;
esac
