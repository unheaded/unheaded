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
# and perf counters available (VMs often lack llc_misses).
set -euo pipefail
prog=${1:-monad_cpu}
secs=${2:-10}
command -v bpftool >/dev/null || { echo "bpf-profile.sh: bpftool not found" >&2; exit 2; }
ids=$(bpftool prog show | awk -v p="$prog" '$0 ~ ("name " p "( |$)") {sub(":", "", $1); print $1}')
[[ -n $ids ]] || { echo "bpf-profile.sh: no loaded BPF program named ${prog}" >&2; exit 1; }
for id in $ids; do
	echo "== prog id ${id} (${prog}), ${secs}s"
	bpftool prog profile id "$id" duration "$secs" cycles instructions l1d_loads llc_misses
done
