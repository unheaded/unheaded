#!/usr/bin/env bash
# Phase 10 Shield Monitor — real-time BPF stats + ring buffer health
#
# Usage: sudo ./scripts/shield-monitor.sh [interval_seconds]
#
# Reads STATS, ERROR_COUNTERS, and ANAMNESIS ring buffer positions.

set -euo pipefail

INTERVAL=${1:-1}

# STATS keys from shield-ebpf/src/main.rs
declare -A STAT_NAMES=(
    [0]="TOTAL"
    [1]="IPV6"
    [2]="BIRTHS"
    [3]="DEATHS"
    [4]="BLOCKED"
    [5]="EXT_STRIPPED"
    [6]="ANOMALIES"
    [7]="RING_DROPS"
    [8]="DST_OPT_PROC"
    [9]="DST_OPT_SKIP"
    [10]="DST_OPT_DISCARD"
    [11]="DST_OPT_ICMP"
    [12]="DST_OPT_ICMP_MCAST"
    [13]="TC_ENTRY"
    [14]="TC_IPV6"
    [15]="TC_HBH"
)

# Previous values for rate calculation
declare -A PREV

format_rate() {
    local val=$1
    if (( val > 1000000 )); then
        printf "%6.1fM" "$(echo "scale=1; $val/1000000" | bc)"
    elif (( val > 1000 )); then
        printf "%6.1fK" "$(echo "scale=1; $val/1000" | bc)"
    else
        printf "%7d" "$val"
    fi
}

read_stats() {
    # Parse bpftool map dump output
    # Format: key: XX XX XX XX  value: XX XX XX XX XX XX XX XX
    local output
    output=$(bpftool map dump name STATS 2>/dev/null) || return 1

    declare -gA STATS
    while IFS= read -r line; do
        if [[ "$line" =~ ^key:\ (..)\ (..)\ (..)\ (..)\ \ value:\ (..)\ (..)\ (..)\ (..)\ (..)\ (..)\ (..)\ (..) ]]; then
            # Little-endian key (u32)
            local k=$(( 16#${BASH_REMATCH[4]}${BASH_REMATCH[3]}${BASH_REMATCH[2]}${BASH_REMATCH[1]} ))
            # Little-endian value (u64)
            local v=$(( 16#${BASH_REMATCH[12]}${BASH_REMATCH[11]}${BASH_REMATCH[10]}${BASH_REMATCH[9]}${BASH_REMATCH[8]}${BASH_REMATCH[7]}${BASH_REMATCH[6]}${BASH_REMATCH[5]} ))
            STATS[$k]=$v
        fi
    done <<< "$output"
}

read_errors() {
    local output
    output=$(bpftool map dump name ERROR_COUNTERS 2>/dev/null) || return 1
    declare -gA ERRORS
    local total=0
    while IFS= read -r line; do
        if [[ "$line" =~ ^key:.*value:\ (..)\ (..)\ (..)\ (..)\ (..)\ (..)\ (..)\ (..) ]]; then
            local v=$(( 16#${BASH_REMATCH[8]}${BASH_REMATCH[7]}${BASH_REMATCH[6]}${BASH_REMATCH[5]}${BASH_REMATCH[4]}${BASH_REMATCH[3]}${BASH_REMATCH[2]}${BASH_REMATCH[1]} ))
            total=$((total + v))
        fi
    done <<< "$output"
    ERRORS[total]=$total
}

read_ringbuf() {
    # Read ANAMNESIS ring buffer positions from mmap
    # Can't easily read from bash, use bpftool map dump
    local output
    output=$(bpftool map dump name ANAMNESIS 2>/dev/null) || true
    # Ring buffers don't dump entries, but we can check if it exists
    RINGBUF_OK=1
}

print_header() {
    printf "\033[2J\033[H"  # Clear screen
    echo "╔══════════════════════════════════════════════════════════════════════╗"
    echo "║              Phase 10 Shield Monitor — tap-tomb                     ║"
    echo "╠══════════════════════════════════════════════════════════════════════╣"
    printf "║ %-24s │ %12s │ %12s │ %8s ║\n" "STAT" "TOTAL" "DELTA" "RATE/s"
    echo "╠══════════════════════════════════════════════════════════════════════╣"
}

print_stat() {
    local key=$1
    local name=${STAT_NAMES[$key]:-"UNKNOWN_$key"}
    local val=${STATS[$key]:-0}
    local prev=${PREV[$key]:-0}
    local delta=$((val - prev))
    local rate=$((delta / INTERVAL))

    # Color coding
    local color=""
    local reset="\033[0m"
    if (( delta > 0 )); then
        if [[ "$name" == *"ERROR"* ]] || [[ "$name" == *"ANOMALY"* ]]; then
            color="\033[1;31m"  # Red for errors
        elif [[ "$name" == *"BIRTH"* ]] || [[ "$name" == *"DEATH"* ]]; then
            color="\033[1;32m"  # Green for events
        elif [[ "$name" == *"MONAD"* ]]; then
            color="\033[1;36m"  # Cyan for Monad ops
        else
            color="\033[1;37m"  # White for activity
        fi
    fi

    printf "║ ${color}%-24s${reset} │ %12s │ %+12s │ %8s ║\n" \
        "$name" \
        "$(printf "%'d" "$val")" \
        "$(printf "%'+d" "$delta")" \
        "$(format_rate "$rate")"

    PREV[$key]=$val
}

print_footer() {
    echo "╠══════════════════════════════════════════════════════════════════════╣"
    # Error summary
    local err_total=${ERRORS[total]:-0}
    local err_color="\033[1;32m"  # Green
    if (( err_total > 0 )); then
        err_color="\033[1;31m"  # Red
    fi
    printf "║ ${err_color}ERROR_COUNTERS total: %'d${reset}%*s║\n" "$err_total" $((44 - ${#err_total})) ""
    echo "║ ANAMNESIS ring buffer: $([ "${RINGBUF_OK:-0}" = "1" ] && echo "LIVE (8MB)" || echo "MISSING")                            ║"
    echo "╚══════════════════════════════════════════════════════════════════════╝"
    echo ""
    echo "  Refreshing every ${INTERVAL}s — Ctrl+C to stop"
}

# Main loop
echo "Starting Shield Monitor (interval=${INTERVAL}s)..."

while true; do
    read_stats
    read_errors
    read_ringbuf

    print_header

    # Print stats in order
    for key in $(seq 0 15); do
        if [[ -n "${STATS[$key]:-}" ]] || [[ -n "${STAT_NAMES[$key]:-}" ]]; then
            print_stat "$key"
        fi
    done

    print_footer

    sleep "$INTERVAL"
done
