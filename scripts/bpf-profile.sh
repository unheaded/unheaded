#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# bpf-profile.sh — per-program hardware counters for a loaded BPF program
# with the kernel's own bpftool (no outside tool). Default program:
# monad_cpu, the UPC tick (what the EVOLUTION-1 ladder spends its verifier
# budget on).
#
#   sudo scripts/bpf-profile.sh [PROG_NAME] [SECONDS]
#
# Needs the program loaded (e.g. inside the UPC boot recipe in CLAUDE.md)
# and perf counters available (VMs often lack llc_misses). A bpftool built
# without clang (west's) has no `prog profile`; then the script falls back to
# the kernel's run statistics (kernel.bpf_stats_enabled for the window): runs,
# run time and ns per run, no hardware counters.
set -euo pipefail
prog=${1:-monad_cpu}
secs=${2:-10}
command -v bpftool >/dev/null || { echo "bpf-profile.sh: bpftool not found" >&2; exit 2; }
ids=$(bpftool prog show | awk -v p="$prog" '$0 ~ ("name " p "( |$)") {sub(":", "", $1); print $1}')
[[ -n $ids ]] || { echo "bpf-profile.sh: no loaded BPF program named ${prog}" >&2; exit 1; }
stats() { bpftool prog show id "$1" | awk '{for (i = 1; i < NF; i++) if ($i == "run_time_ns" || $i == "run_cnt") v[$i] = $(i + 1)} END {print v["run_time_ns"] + 0, v["run_cnt"] + 0}'; }
if bpftool prog profile help 2>&1 | grep -q 'not supported'; then
	echo "bpf-profile.sh: this bpftool has no 'prog profile'; kernel run statistics instead" >&2
	was=$(sysctl -n kernel.bpf_stats_enabled)
	trap 'sysctl -qw kernel.bpf_stats_enabled="$was"' EXIT
	sysctl -qw kernel.bpf_stats_enabled=1
	before=$(for id in $ids; do echo "$id $(stats "$id")"; done)
	sleep "$secs"
	for id in $ids; do
		read -r t n < <(stats "$id")
		read -r _ t0 n0 < <(grep "^${id} " <<<"$before")
		dt=$((t - t0)) dn=$((n - n0))
		echo "== prog id ${id} (${prog}), ${secs}s"
		awk -v dt="$dt" -v dn="$dn" 'BEGIN {printf "  runs %d  run_time %.1f ms  %.0f ns/run\n", dn, dt / 1e6, dn ? dt / dn : 0}'
	done
	exit 0
fi
for id in $ids; do
	echo "== prog id ${id} (${prog}), ${secs}s"
	bpftool prog profile id "$id" duration "$secs" cycles instructions l1d_loads llc_misses
done
