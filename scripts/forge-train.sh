#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# zhenai-forge launcher — runs the binary under a systemd-run user scope
# with hard memory + wall-clock caps and an external log.
#
# Caller passes the full subcommand line (e.g. `train`, `train-gemma4`,
# `eval-gemma4`, `generate-gemma4`).
#
# Usage:
#   scripts/forge-train.sh <subcommand> [args...]
# Examples:
#   scripts/forge-train.sh train --model …            # Mistral path
#   scripts/forge-train.sh train-gemma4 --model …     # Gemma-4 path
#   scripts/forge-train.sh eval-gemma4 --model … --lora …
#
# Environment overrides:
#   FORGE_LOG        path to log file (default: scripts/lib/logpath.sh forge forge-train.log)
#   FORGE_BIN        path to zhenai-forge binary (default crates/zhenai-forge/target/release/zhenai-forge)
#   FORGE_MEM_MAX    MemoryMax for the scope (default 10G)
#   FORGE_MEM_HIGH   MemoryHigh for the scope (default 8G)
#   FORGE_MAX_SEC    RuntimeMaxSec for the scope (default 7200 = 2h)
#   FORGE_MIN_FREE_G host MemAvailable floor in GB (default 10)
#   FORGE_GPU_HANDOFF 1 (default): stop llama-server first, restart
#                    FORGE_RESTART_MODEL (default qwen-7b) after; 0: leave it
#   GPU_LEASE_TIMEOUT seconds to wait for the GPU lease (default 600)
#
# GPU lease (S-TWO-BRAINS Step 233): the whole run holds the exclusive
# lease of scripts/lib/gpu-lease.sh. llama-server and Gemma-4 training
# cannot share the 12 GB card, so inside the lease the served model is
# stopped and, after training, the default model is loaded again through
# scripts/switch-model.sh (its fit check included). zhen-agentd's worker
# defers cards while the lease is held.
#
# Emits to stdout:
#   [forge-train] unit=<scope name> log=<path>
# so the watchdog can attach.
#
# Exit codes:
#   0  — training process exited cleanly
#   2  — preflight gate failed (H6 RAM floor)
#   3  — binary missing
#   75 — the GPU lease was not granted in time
#   >0 — training exited non-zero (forwarded)
set -euo pipefail

if [[ "${GPU_LEASE_HELD:-}" != exclusive ]]; then
  exec "$(dirname "${BASH_SOURCE[0]}")/lib/gpu-lease.sh" exclusive "${GPU_LEASE_TIMEOUT:-600}" -- "${BASH_SOURCE[0]}" "$@"
fi
SCRIPTS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

FORGE_LOG="${FORGE_LOG:-$("$(dirname "${BASH_SOURCE[0]}")/lib/logpath.sh" forge forge-train.log)}"
FORGE_BIN="${FORGE_BIN:-/home/govan/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge}"
FORGE_MEM_MAX="${FORGE_MEM_MAX:-10G}"
FORGE_MEM_HIGH="${FORGE_MEM_HIGH:-8G}"
FORGE_MAX_SEC="${FORGE_MAX_SEC:-7200}"
FORGE_MIN_FREE_G="${FORGE_MIN_FREE_G:-10}"
FORGE_GPU_HANDOFF="${FORGE_GPU_HANDOFF:-1}"
FORGE_RESTART_MODEL="${FORGE_RESTART_MODEL:-qwen-7b}"
LLAMA_PORT="${LLAMA_PORT:-8081}"

log() { echo "[forge-train] $*" >&2; }

# Binary existence gate
if [[ ! -x "$FORGE_BIN" ]]; then
  log "ERROR: binary not executable: $FORGE_BIN"
  log "hint: cd crates/zhenai-forge && cargo build --release"
  exit 3
fi

# Hand the GPU over: stop the served model (we hold the exclusive lease).
STOPPED_LLAMA=0
restart_llama() {
  if (( STOPPED_LLAMA )); then
    log "restarting llama-server with ${FORGE_RESTART_MODEL}"
    "$SCRIPTS/switch-model.sh" "$FORGE_RESTART_MODEL" >&2 || log "WARNING: restarting ${FORGE_RESTART_MODEL} failed; the GPU is free, load a model by hand"
    STOPPED_LLAMA=0
  fi
}
if [[ "$FORGE_GPU_HANDOFF" == 1 ]] && ss -lnt 2>/dev/null | grep -q ":${LLAMA_PORT} "; then
  log "GPU handoff: stopping llama-server on :${LLAMA_PORT} for the training run"
  "$SCRIPTS/switch-model.sh" --stop >&2
  STOPPED_LLAMA=1
fi

# H6 host RAM floor gate (mirrors the in-process BW_ATTN budget check)
avail_kb=$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)
avail_g=$(( avail_kb / 1024 / 1024 ))
if (( avail_g < FORGE_MIN_FREE_G )); then
  log "ERROR: MemAvailable=${avail_g}G < required ${FORGE_MIN_FREE_G}G"
  log "hint: stop docker/containerd/fail2ban and re-check free -h"
  restart_llama
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
log "command: $FORGE_BIN $*"

# stdbuf -oL forces line-buffered stdout so the log gets per-step lines
# in real time (critical for the watchdog's K1/K2/K4 pattern matchers).
# Not exec'd: the served model comes back after the run.
rc=0
systemd-run --user --scope \
  --unit="$UNIT" \
  -p "MemoryMax=${FORGE_MEM_MAX}" \
  -p "MemoryHigh=${FORGE_MEM_HIGH}" \
  -p "TasksMax=256" \
  -p "RuntimeMaxSec=${FORGE_MAX_SEC}" \
  -- bash -c "stdbuf -oL '$FORGE_BIN' $(printf '%q ' "$@") 2>&1 | tee -a '$FORGE_LOG'" || rc=$?
restart_llama
exit "$rc"
