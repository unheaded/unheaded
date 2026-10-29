#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# flamegraph.sh — capture a CPU profile from a binary's loopback pprof
# listener (--pprof ADDR, or PPROF_ADDR for wotan/dashboard-backend) and
# open pprof's built-in flame graph (View -> Flame Graph). Go toolchain only.
#
#   scripts/flamegraph.sh ADDR BINARY [SECONDS] [--top]
#     ADDR     the --pprof address, e.g. 127.0.0.1:6060
#     BINARY   the running program's binary, for symbols (build it with
#              `make build-profile`: the release build strips them)
#     --top    print the top 25 functions instead of opening the web UI
#
# Raw profiles land in var/pprof/ (gitignored): heap and goroutine dumps can
# hold service tokens and keys, so they never get committed.
set -euo pipefail
cd "$(dirname "$0")/.."
addr=${1:?usage: flamegraph.sh ADDR BINARY [SECONDS] [--top]}
bin=${2:?usage: flamegraph.sh ADDR BINARY [SECONDS] [--top]}
secs=${3:-30}
top=0
[[ ${4:-} == --top || ${3:-} == --top ]] && top=1
[[ $secs == --top ]] && secs=30
[[ -x $bin ]] || { echo "flamegraph.sh: no executable at $bin" >&2; exit 2; }

mkdir -p var/pprof
out="var/pprof/$(basename "$bin")-$(date -u +%Y%m%dT%H%M%SZ).pb.gz"
echo "capturing ${secs}s CPU profile from ${addr} -> ${out}" >&2
curl -fsS --max-time $((secs + 15)) "http://${addr}/debug/pprof/profile?seconds=${secs}" -o "$out"
if (( top )); then
	go tool pprof -top -nodecount=25 "$bin" "$out"
else
	go tool pprof -http=127.0.0.1:0 "$bin" "$out"
fi
