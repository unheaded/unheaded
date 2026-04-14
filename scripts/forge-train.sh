#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Wave 10D training launcher — runs zhenai-forge train under a systemd-run
# user scope with hard memory + wall-clock caps and an external log.
#
# Usage:
#   scripts/forge-train.sh [train args...]
#
# Environment overrides:
#   FORGE_LOG        path to log file (default /tmp/v6-run.log)
#   FORGE_BIN        path to zhenai-forge binary (default crates/zhenai-forge/target/release/zhenai-forge)
#   FORGE_MEM_MAX    MemoryMax for the scope (default 10G)
#   FORGE_MEM_HIGH   MemoryHigh for the scope (default 8G)
#   FORGE_MAX_SEC    RuntimeMaxSec for the scope (default 7200 = 2h)
#   FORGE_MIN_FREE_G host MemAvailable floor in GB (default 10)
#
# Emits to stdout:
#   [forge-train] unit=<scope name> log=<path>
# so the watchdog can attach.
#
# Exit codes:
#   0  — training process exited cleanly
#   2  — preflight gate failed (H6 RAM floor)
#   3  — binary missing
#   >0 — training exited non-zero (forwarded)
set -euo pipefail

FORGE_LOG="${FORGE_LOG:-/tmp/v6-run.log}"
FORGE_BIN="${FORGE_BIN:-/home/govan/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge}"
FORGE_MEM_MAX="${FORGE_MEM_MAX:-10G}"
FORGE_MEM_HIGH="${FORGE_MEM_HIGH:-8G}"
FORGE_MAX_SEC="${FORGE_MAX_SEC:-7200}"
FORGE_MIN_FREE_G="${FORGE_MIN_FREE_G:-10}"

log() { echo "[forge-train] $*" >&2; }

# Binary existence gate
if [[ ! -x "$FORGE_BIN" ]]; then
  log "ERROR: binary not executable: $FORGE_BIN"
  log "hint: cd crates/zhenai-forge && cargo build --release"
  exit 3
fi

# H6 host RAM floor gate (mirrors the in-process BW_ATTN budget check)
avail_kb=$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)
avail_g=$(( avail_kb / 1024 / 1024 ))
if (( avail_g < FORGE_MIN_FREE_G )); then
  log "ERROR: MemAvailable=${avail_g}G < required ${FORGE_MIN_FREE_G}G"
  log "hint: stop docker/containerd/fail2ban and re-check free -h"
  exit 2
fi
log "preflight: MemAvailable=${avail_g}G >= ${FORGE_MIN_FREE_G}G floor — OK"

# Fresh log
: > "$FORGE_LOG"

# Unit name with timestamp so repeated runs don't collide
UNIT="forge-train-$(date +%Y%m%d-%H%M%S)"

# Emit discovery line so the watchdog can attach
echo "[forge-train] unit=${UNIT}.scope log=${FORGE_LOG}"

log "launching scope=${UNIT}.scope mem_max=${FORGE_MEM_MAX} mem_high=${FORGE_MEM_HIGH} max_sec=${FORGE_MAX_SEC}"
log "command: $FORGE_BIN train $*"

# stdbuf -oL forces line-buffered stdout so the log gets per-step lines
# in real time (critical for the watchdog's K1/K2/K4 pattern matchers).
exec systemd-run --user --scope \
  --unit="$UNIT" \
  -p "MemoryMax=${FORGE_MEM_MAX}" \
  -p "MemoryHigh=${FORGE_MEM_HIGH}" \
  -p "TasksMax=256" \
  -p "RuntimeMaxSec=${FORGE_MAX_SEC}" \
  -- bash -c "stdbuf -oL '$FORGE_BIN' train $(printf '%q ' "$@") 2>&1 | tee -a '$FORGE_LOG'"
