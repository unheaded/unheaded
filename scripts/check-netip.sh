#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-netip.sh — shrink-only count of net.IP / net.ParseIP in non-test Go
# (S-LOW-FRUIT Phase 8). New code uses net/netip (comparable, allocation
# free, no IPv4-in-IPv6 ambiguity); this doesn't rewrite the old uses, it
# stops new ones.
#
#   scripts/check-netip.sh            fail unless the count equals the baseline
#   scripts/check-netip.sh --update   lower the baseline to today's count (never raises it)
#
# A count below the baseline fails too. Slack is room for new uses: S-WIRE-HONEST
# took the count from 74 to 68 and left the baseline at 74, so S-G9-FOLD's new
# net.ParseIP in cmd/compliance-evidence passed unnoticed.
set -euo pipefail
cd "$(dirname "$0")/.."
baseline=docs/policy/netip-baseline.txt
dirs=()
for d in cmd pkg services internal; do [[ -d $d ]] && dirs+=("$d"); done

count=$(grep -rnE --include='*.go' 'net\.ParseIP|net\.IP\b' "${dirs[@]}" | grep -vc '_test\.go:' || true)
base=$(grep -vE '^\s*(#|$)' "$baseline" | head -1)
[[ $base =~ ^[0-9]+$ ]] || { echo "FAIL: no number in ${baseline}"; exit 1; }

if [[ ${1:-} == --update ]]; then
	if (( count < base )); then
		sed -i "s/^${base}\$/${count}/" "$baseline"
		echo "baseline lowered ${base} -> ${count}"
	else
		echo "baseline stays ${base} (count ${count})"
	fi
	exit 0
fi
if (( count > base )); then
	echo "FAIL: ${count} net.IP/net.ParseIP uses, baseline ${base}. Use net/netip in new code:"
	echo "  netip.ParseAddr, netip.AddrPort, netip.Prefix"
	exit 1
fi
if (( count < base )); then
	echo "FAIL: ${count} net.IP/net.ParseIP uses, baseline ${base}. Lock the shrink in: scripts/check-netip.sh --update"
	exit 1
fi
echo "OK: ${count} uses (baseline ${base})"
