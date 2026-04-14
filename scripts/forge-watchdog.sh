#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Wave 10D external watchdog — tails the training log and kills the
# systemd-run scope on K1/K2/K4 trip conditions (defined in
# docs/battle-plans/WAVE10D-GPU-BACKWARD.md).
#
# Rationale: in-process timers cannot kill a hipBLAS call wedged in kernel
# D-state. This watchdog runs in a separate tmux pane, tails the log file,
# pattern-matches failure signatures, and sends SIGTERM (then SIGKILL 10s
# later) to the scope unit. SIGTERM is survivable by userspace; a wedged
# D-state process needs SIGKILL + eventual RuntimeMaxSec + kernel hang-task
# recovery.
#
# Usage:
#   scripts/forge-watchdog.sh <scope-unit> [log-path]
#
# Example:
#   scripts/forge-watchdog.sh forge-train-20260414-091234.scope /tmp/v6-run.log
#
# Kill triggers:
#   K1  — "shape mismatch" or similar sgemm geometry failure
#   K2  — Loss: nan | inf | >=20  (divergence)
#   K4  — no "Loss:" line for STALL_S seconds (livelock / CPU-fallback)
#   TIP — the Apr 12 failure signature in kernel log: svm_range_restore_work
set -euo pipefail

UNIT="${1:?Usage: $0 <scope-unit> [log-path]}"
LOG="${2:-/tmp/v6-run.log}"
STALL_S="${STALL_S:-300}"
HEARTBEAT="${HEARTBEAT:-/tmp/forge.heartbeat}"

log() { echo "[watchdog] $(date -Is) $*" >&2; }

kill_unit() {
  local reason="$1"
  log "TRIP — $reason"
  log "SIGTERM → $UNIT"
  systemctl --user kill --signal=SIGTERM "$UNIT" 2>&1 || log "warn: SIGTERM send failed (unit gone?)"
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if ! systemctl --user is-active --quiet "$UNIT"; then
      log "unit stopped cleanly after SIGTERM"
      exit 0
    fi
    sleep 1
  done
  log "SIGTERM did not stop unit in 10s — SIGKILL → $UNIT"
  systemctl --user kill --signal=SIGKILL "$UNIT" 2>&1 || true
  exit 1
}

# Wait briefly for the log file to appear (train.sh creates it before execing)
for _ in 1 2 3 4 5 6 7 8 9 10; do
  [[ -f "$LOG" ]] && break
  sleep 0.5
done
if [[ ! -f "$LOG" ]]; then
  log "ERROR: log file never appeared: $LOG"
  exit 4
fi
log "attached unit=$UNIT log=$LOG stall_s=$STALL_S"
date +%s > "$HEARTBEAT"

# Stall detector — background loop that checks heartbeat age.
(
  while true; do
    sleep 30
    if [[ ! -f "$HEARTBEAT" ]]; then continue; fi
    last=$(cat "$HEARTBEAT" 2>/dev/null || echo 0)
    now=$(date +%s)
    age=$(( now - last ))
    if (( age > STALL_S )); then
      # Write a sentinel line into the log so the tailer sees and kills.
      echo "[watchdog] K4 STALL — no Loss line in ${age}s (threshold ${STALL_S}s)" >> "$LOG"
      exit 0
    fi
  done
) &
STALL_PID=$!

# Cleanup: kill stall detector + any surviving `tail -F` owned by this PID.
# `tail -F | while` leaves tail orphaned on exit; process substitution +
# explicit pkill on our own PGID is the cleanest path bash gives us.
trap 'kill $STALL_PID 2>/dev/null || true; pkill -P $$ 2>/dev/null || true' EXIT

# Main tailer via process substitution so `exit` in the loop exits the
# main shell (not just a pipeline subshell).
while IFS= read -r line; do
  # K1 — sgemm shape/geometry failures
  if [[ "$line" =~ (shape[[:space:]]mismatch|sgemm.*error|sgemm.*failed|HIPBLAS.*error) ]]; then
    kill_unit "K1 sgemm failure: $line"
  fi
  # K2 — loss divergence. Match `Loss:` or `loss=` followed by nan/inf/>=20.
  if [[ "$line" =~ (Loss|loss)[[:space:]=:]+(nan|NaN|inf|Inf) ]]; then
    kill_unit "K2 divergence (NaN/Inf): $line"
  fi
  if [[ "$line" =~ (Loss|loss)[[:space:]=:]+([0-9]+)\.[0-9]+ ]]; then
    int="${BASH_REMATCH[2]}"
    if (( int >= 20 )); then
      kill_unit "K2 divergence (>=20): $line"
    fi
    # Any loss line refreshes the heartbeat and clears stall risk.
    date +%s > "$HEARTBEAT"
  fi
  # K4 sentinel from the stall background loop
  if [[ "$line" =~ \[watchdog\][[:space:]]K4[[:space:]]STALL ]]; then
    kill_unit "K4 stall signaled"
  fi
  # Tip — AMD HMM/SVM thrash signature propagated from journal
  if [[ "$line" =~ svm_range_restore_work ]]; then
    kill_unit "TIP Apr-12-class ROCm SVM thrash detected in log"
  fi
done < <(tail -n0 -F "$LOG")
