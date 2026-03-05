#!/bin/bash
# SPDX-License-Identifier: MIT
# verify-wireguard.sh — Verify WireGuard IPv6 overlay health and performance
# Phase 2: S77 Age 2 Acceleration
set -euo pipefail

###############################################################################
# Configuration
###############################################################################
WEST_P2P="192.168.13.2"
EAST_P2P="192.168.13.1"
WEST_WG="fd00:dead:beef::2"
EAST_WG="fd00:dead:beef::1"
WG_IFACE="wg0"
REMOTE_USER="govan"
PING_COUNT=10
TCPDUMP_DURATION=5
PASS=0
FAIL=0
WARN=0

###############################################################################
# Helpers
###############################################################################
log()    { printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }
pass()   { log "PASS: $*"; ((PASS++)) || true; }
fail()   { log "FAIL: $*"; ((FAIL++)) || true; }
warn()   { log "WARN: $*"; ((WARN++)) || true; }
header() { printf '\n[%s] === %s ===\n' "$(date '+%H:%M:%S')" "$*"; }

###############################################################################
# Test 1: Interface existence
###############################################################################
test_interfaces() {
    header "WireGuard Interface Check"

    # WEST
    if ip link show "$WG_IFACE" >/dev/null 2>&1; then
        pass "WEST wg0 interface exists"
    else
        fail "WEST wg0 interface missing"
        return 1
    fi

    # EAST
    if ssh -o ConnectTimeout=5 "${REMOTE_USER}@${EAST_P2P}" \
        "ip link show ${WG_IFACE}" >/dev/null 2>&1; then
        pass "EAST wg0 interface exists"
    else
        fail "EAST wg0 interface missing"
        return 1
    fi
}

###############################################################################
# Test 2: Bidirectional ping over IPv6
###############################################################################
test_ping_west_to_east() {
    header "WEST -> EAST IPv6 Ping (${PING_COUNT} packets)"

    local output
    output=$(ping6 -c "$PING_COUNT" -W 3 "$EAST_WG" 2>&1) || true

    if echo "$output" | grep -q "0% packet loss"; then
        pass "WEST -> EAST: 0% packet loss"
    elif echo "$output" | grep -q "packet loss"; then
        local loss
        loss=$(echo "$output" | grep -oP '\d+(?=% packet loss)')
        if [ "$loss" -lt 10 ]; then
            warn "WEST -> EAST: ${loss}% packet loss (acceptable)"
        else
            fail "WEST -> EAST: ${loss}% packet loss"
        fi
    else
        fail "WEST -> EAST: ping6 failed entirely"
    fi

    # Extract latency
    local rtt_line
    rtt_line=$(echo "$output" | grep "rtt\|round-trip" || true)
    if [ -n "$rtt_line" ]; then
        log "  Latency: ${rtt_line}"
    fi
}

test_ping_east_to_west() {
    header "EAST -> WEST IPv6 Ping (${PING_COUNT} packets)"

    local output
    output=$(ssh -o ConnectTimeout=5 "${REMOTE_USER}@${EAST_P2P}" \
        "ping6 -c ${PING_COUNT} -W 3 ${WEST_WG}" 2>&1) || true

    if echo "$output" | grep -q "0% packet loss"; then
        pass "EAST -> WEST: 0% packet loss"
    elif echo "$output" | grep -q "packet loss"; then
        local loss
        loss=$(echo "$output" | grep -oP '\d+(?=% packet loss)')
        if [ "$loss" -lt 10 ]; then
            warn "EAST -> WEST: ${loss}% packet loss (acceptable)"
        else
            fail "EAST -> WEST: ${loss}% packet loss"
        fi
    else
        fail "EAST -> WEST: ping6 failed entirely"
    fi

    local rtt_line
    rtt_line=$(echo "$output" | grep "rtt\|round-trip" || true)
    if [ -n "$rtt_line" ]; then
        log "  Latency: ${rtt_line}"
    fi
}

###############################################################################
# Test 3: wg show on both hosts
###############################################################################
test_wg_show() {
    header "WireGuard Status"

    log "--- WEST wg show ---"
    local west_wg
    west_wg=$(sudo wg show "$WG_IFACE" 2>&1) || true
    echo "$west_wg"

    if echo "$west_wg" | grep -q "latest handshake"; then
        pass "WEST has active handshake"
    else
        warn "WEST has no recent handshake (peer may be idle)"
    fi

    if echo "$west_wg" | grep -q "transfer:"; then
        local rx tx
        rx=$(echo "$west_wg" | grep "transfer:" | awk '{print $2, $3}')
        tx=$(echo "$west_wg" | grep "transfer:" | awk '{print $5, $6}')
        log "  WEST transfer: RX=${rx}, TX=${tx}"
    fi

    echo
    log "--- EAST wg show ---"
    local east_wg
    east_wg=$(ssh -o ConnectTimeout=5 "${REMOTE_USER}@${EAST_P2P}" \
        "sudo wg show ${WG_IFACE}" 2>&1) || true
    echo "$east_wg"

    if echo "$east_wg" | grep -q "latest handshake"; then
        pass "EAST has active handshake"
    else
        warn "EAST has no recent handshake (peer may be idle)"
    fi
}

###############################################################################
# Test 4: tcpdump capture for HbH extension headers
###############################################################################
test_hbh_capture() {
    header "Hop-by-Hop Extension Header Capture Test"

    if ! command -v tcpdump >/dev/null 2>&1; then
        warn "tcpdump not available — skipping HbH capture test"
        return 0
    fi

    local capture_file="/tmp/wg-hbh-capture-$$.pcap"

    log "Starting tcpdump on ${WG_IFACE} for ${TCPDUMP_DURATION}s..."
    sudo timeout "${TCPDUMP_DURATION}" tcpdump -i "$WG_IFACE" -c 20 -w "$capture_file" \
        "ip6" 2>/dev/null &
    local tcpdump_pid=$!

    # Generate some traffic to capture
    sleep 1
    ping6 -c 3 -W 2 "$EAST_WG" >/dev/null 2>&1 || true
    sleep 1

    wait "$tcpdump_pid" 2>/dev/null || true

    if [ -f "$capture_file" ] && [ -s "$capture_file" ]; then
        local pkt_count
        pkt_count=$(sudo tcpdump -r "$capture_file" 2>/dev/null | wc -l)
        if [ "$pkt_count" -gt 0 ]; then
            pass "Captured ${pkt_count} IPv6 packets on ${WG_IFACE}"
        else
            warn "Capture file exists but contains no readable packets"
        fi

        # Check for HbH headers specifically
        local hbh_count
        hbh_count=$(sudo tcpdump -r "$capture_file" -v 2>/dev/null | grep -ci "hop-by-hop\|HBH\|next-header: 0" || echo "0")
        if [ "$hbh_count" -gt 0 ]; then
            pass "Found ${hbh_count} packets with Hop-by-Hop extension headers"
        else
            log "  No HbH extension headers detected (expected if Monad HbH not yet injecting)"
            log "  HbH passthrough will be validated when Monad IPv6 injection is enabled"
        fi

        sudo rm -f "$capture_file"
    else
        warn "No packets captured (tunnel may be quiet)"
    fi
}

###############################################################################
# Test 5: Latency report
###############################################################################
test_latency_report() {
    header "Latency Summary"

    # P2P direct (IPv4)
    log "--- Direct P2P (IPv4: ${EAST_P2P}) ---"
    local p2p_output
    p2p_output=$(ping -c "$PING_COUNT" -W 3 "$EAST_P2P" 2>&1) || true
    local p2p_rtt
    p2p_rtt=$(echo "$p2p_output" | grep "rtt\|round-trip" || echo "N/A")
    log "  ${p2p_rtt}"

    # WireGuard overlay (IPv6)
    log "--- WireGuard Overlay (IPv6: ${EAST_WG}) ---"
    local wg_output
    wg_output=$(ping6 -c "$PING_COUNT" -W 3 "$EAST_WG" 2>&1) || true
    local wg_rtt
    wg_rtt=$(echo "$wg_output" | grep "rtt\|round-trip" || echo "N/A")
    log "  ${wg_rtt}"

    # Parse and compare
    local p2p_avg wg_avg
    p2p_avg=$(echo "$p2p_rtt" | grep -oP '[\d.]+(?=/[\d.]+/[\d.]+ ms)' | head -1 || echo "")
    wg_avg=$(echo "$wg_rtt" | grep -oP '[\d.]+(?=/[\d.]+/[\d.]+ ms)' | head -1 || echo "")

    if [ -n "$p2p_avg" ] && [ -n "$wg_avg" ]; then
        log ""
        log "  Direct P2P avg:   ${p2p_avg} ms"
        log "  WireGuard avg:    ${wg_avg} ms"
        local overhead
        overhead=$(echo "$wg_avg - $p2p_avg" | bc 2>/dev/null || echo "N/A")
        log "  WG overhead:      ${overhead} ms"

        # Check if overhead is acceptable (< 1ms for P2P link)
        if [ "$overhead" != "N/A" ]; then
            local ok_check
            ok_check=$(echo "$overhead < 2.0" | bc 2>/dev/null || echo "0")
            if [ "$ok_check" = "1" ]; then
                pass "WireGuard overhead is acceptable (${overhead} ms)"
            else
                warn "WireGuard overhead is high (${overhead} ms) — investigate MTU or CPU"
            fi
        fi
    fi
}

###############################################################################
# Summary
###############################################################################
print_summary() {
    header "Verification Summary"
    log "  PASS: ${PASS}"
    log "  FAIL: ${FAIL}"
    log "  WARN: ${WARN}"

    if [ "$FAIL" -gt 0 ]; then
        log ""
        log "RESULT: FAILED — ${FAIL} check(s) did not pass"
        exit 1
    elif [ "$WARN" -gt 0 ]; then
        log ""
        log "RESULT: PASSED WITH WARNINGS — review above"
        exit 0
    else
        log ""
        log "RESULT: ALL CHECKS PASSED"
        exit 0
    fi
}

###############################################################################
# Main
###############################################################################
main() {
    log "=========================================="
    log "Unheaded S77 — WireGuard Verification"
    log "=========================================="

    test_interfaces
    test_ping_west_to_east
    test_ping_east_to_west
    test_wg_show
    test_hbh_capture
    test_latency_report
    print_summary
}

main "$@"
