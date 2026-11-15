#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-unit-start-limits.sh: every unit that restarts itself can also give
# up (BlackMage F16; ADR-147 SS3, S-SEE-AND-SURVIVE Phase 6). systemd's
# default start limit is 5 starts in 10 s; with RestartSec of 5 s or more
# that limit can never be reached, so a unit that fails at every start
# restarts for ever and nobody is told. A unit passes when
#
#   StartLimitIntervalSec > StartLimitBurst x RestartSec
#
# so the limit trips, the unit stays failed, and UnitFailed mails.
# StartLimitIntervalSec=0 or infinity means "never trips" and fails.
#
# It must not trip too soon either. A unit failing at every start gives up
# after about StartLimitBurst x RestartSec, so a pass also needs
#
#   StartLimitBurst x RestartSec >= BOOT_WINDOW (900 s)
#
# the 15 minutes the plan gave a dependency to come up after boot (dockerd
# took 3 minutes on 2026-11-12; the Well, wg0 and the compose bridge come
# after it). 10 starts at 5 s gave up after 50 s (review 2026-11-15, REV-3).
# Sizing: StartLimitBurst = 900 / RestartSec, StartLimitIntervalSec=30min.
# Named exceptions below carry a ruling.
#
# Usage: scripts/check-unit-start-limits.sh [UNIT_FILE...]
#   default: systemd/*.service deploy/systemd/*.service
# Units with no Restart= or Restart=no are not checked.
# Exit 0 all pass, 1 a unit cannot trip, 2 a value cannot be parsed.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ $# -gt 0 ]]; then
    files=("$@")
else
    shopt -s nullglob
    files=(systemd/*.service deploy/systemd/*.service)
fi

# secs <value>: a systemd time span (bare seconds, ms, s, min, h; one or
# more parts) as seconds with three decimals; "inf" for 0 or infinity.
secs() {
    local v="$1" total=0 part n u
    [[ "$v" == infinity || "$v" == 0 ]] && { echo inf; return 0; }
    for part in $v; do
        [[ "$part" =~ ^([0-9]+(\.[0-9]+)?)(ms|s|sec|min|m|h)?$ ]] || return 1
        n="${BASH_REMATCH[1]}" u="${BASH_REMATCH[3]:-s}"
        case "$u" in
            ms) total="$(awk -v t="$total" -v n="$n" 'BEGIN{printf "%.3f", t + n/1000}')" ;;
            s|sec) total="$(awk -v t="$total" -v n="$n" 'BEGIN{printf "%.3f", t + n}')" ;;
            min|m) total="$(awk -v t="$total" -v n="$n" 'BEGIN{printf "%.3f", t + n*60}')" ;;
            h) total="$(awk -v t="$total" -v n="$n" 'BEGIN{printf "%.3f", t + n*3600}')" ;;
        esac
    done
    echo "$total"
}

# last <key> <file>: the value of the last Key= line (later lines win in systemd).
last() { grep -E "^$1=" "$2" | tail -1 | cut -d= -f2- | sed 's/[[:space:]]*$//' || true; }

BOOT_WINDOW=900
# Units that may give up sooner than BOOT_WINDOW, each with its ruling.
declare -A SHORT_RIDE=(
    [unheaded-trace-collector.service]="Stevie 2026-11-09: 60 tries in ten minutes, every 5 s (its header)"
)
checked=0
failed=0
for f in "${files[@]}"; do
    [[ -f "$f" ]] || { echo "[FAIL] $f: no such file" >&2; exit 2; }
    restart="$(last Restart "$f")"
    [[ -z "$restart" || "$restart" == no ]] && continue
    interval="$(last StartLimitIntervalSec "$f")"
    [[ -z "$interval" ]] && interval="$(last StartLimitInterval "$f")"
    burst="$(last StartLimitBurst "$f")"
    rsec="$(last RestartSec "$f")"
    i="$(secs "${interval:-10s}")" || { echo "[ERROR] $f: StartLimitIntervalSec '$interval' not understood" >&2; exit 2; }
    r="$(secs "${rsec:-100ms}")" || { echo "[ERROR] $f: RestartSec '$rsec' not understood" >&2; exit 2; }
    b="${burst:-5}"
    [[ "$b" =~ ^[0-9]+$ ]] || { echo "[ERROR] $f: StartLimitBurst '$b' not a number" >&2; exit 2; }
    [[ "$r" == inf ]] && r=0
    checked=$((checked + 1))
    if [[ "$i" == inf ]]; then
        echo "[FAIL] $f: StartLimitIntervalSec=${interval}: the start limit never trips"
        failed=$((failed + 1))
        continue
    fi
    if ! awk -v i="$i" -v b="$b" -v r="$r" 'BEGIN{exit !(i > b*r)}'; then
        echo "[FAIL] $f: interval ${i}s <= burst ${b} x RestartSec ${r}s"
        failed=$((failed + 1))
        continue
    fi
    if [[ -z "${SHORT_RIDE[$(basename "$f")]:-}" ]] && ! awk -v b="$b" -v r="$r" -v w="$BOOT_WINDOW" 'BEGIN{exit !(b*r >= w)}'; then
        echo "[FAIL] $f: gives up after burst ${b} x RestartSec ${r}s, under the ${BOOT_WINDOW}s boot window"
        failed=$((failed + 1))
    fi
done

if [[ "$failed" -gt 0 ]]; then
    echo "[FAIL] ${failed} of ${checked} restarting units either never trip their start limit or trip inside the boot window (in [Unit]: StartLimitBurst = ${BOOT_WINDOW} / RestartSec, StartLimitIntervalSec=30min; ADR-147 SS3)"
    exit 1
fi
echo "[PASS] ${checked} restarting units can trip their start limit, none before ${BOOT_WINDOW}s"
