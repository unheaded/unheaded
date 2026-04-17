#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# K8s Mock-Prod Lab — tear down 8 LXD containers across WEST + EAST.
# Companion to docs/labs/k8s-kingdom-lab.md.
#
# HARD GUARD: only touches containers whose name starts with "k8s-".
# Spike daemons (/opt/spike-mimirs/*, heimdall-daemon) are NEVER touched.
#
# Usage:
#   scripts/k8s-lab-down.sh                   # interactive confirm
#   scripts/k8s-lab-down.sh --yes             # no prompt
#   scripts/k8s-lab-down.sh --west-only --yes # only WEST
#   scripts/k8s-lab-down.sh --east-only --yes # only EAST
#
# Environment:
#   K8S_LAB_EAST_HOST ssh target for east (default govan@east)
#
# Exit codes:
#   0 success
#   1 user declined
#   2 east unreachable
set -euo pipefail

EAST_HOST="${K8S_LAB_EAST_HOST:-govan@east}"
SCOPE="all"
ASSUME_YES="no"
for arg in "$@"; do
  case "$arg" in
    --west-only) SCOPE="west" ;;
    --east-only) SCOPE="east" ;;
    --yes|-y)    ASSUME_YES="yes" ;;
  esac
done

WEST_CONTAINERS=(k8s-jumpbox k8s-cp-1 k8s-cp-2 k8s-worker-1 k8s-worker-2)
EAST_CONTAINERS=(k8s-cp-3 k8s-worker-3 k8s-worker-4)

log() { printf '[lab-down] %s\n' "$*" >&2; }

# Hard guard — refuse anything not k8s-
guard_name() {
  case "$1" in
    k8s-*) return 0 ;;
    *) log "REFUSED to operate on non-k8s container: $1"; exit 99 ;;
  esac
}

if [[ "$ASSUME_YES" != "yes" ]]; then
  echo "About to STOP and DELETE these LXD containers:"
  [[ "$SCOPE" != "east" ]] && printf '  WEST: %s\n' "${WEST_CONTAINERS[*]}"
  [[ "$SCOPE" != "west" ]] && printf '  EAST: %s\n' "${EAST_CONTAINERS[*]}"
  echo ""
  echo "Spike daemons (/opt/spike-mimirs/*, heimdall-daemon) will NOT be touched."
  read -rp "Proceed? [y/N] " ans
  [[ "$ans" =~ ^[Yy]$ ]] || { log "aborted"; exit 1; }
fi

stop_delete_local() {
  local name="$1"
  guard_name "$name"
  if ! sudo lxc info "$name" >/dev/null 2>&1; then
    log "WEST  $name — absent, skipping"
    return 0
  fi
  log "WEST  $name — stop + delete"
  sudo lxc stop "$name" --force 2>/dev/null || true
  sudo lxc delete "$name" --force 2>/dev/null || log "WEST  $name — delete failed (ignored)"
}

stop_delete_east() {
  local name="$1"
  guard_name "$name"
  if ! ssh -o BatchMode=yes "$EAST_HOST" "sudo lxc info '$name'" >/dev/null 2>&1; then
    log "EAST  $name — absent, skipping"
    return 0
  fi
  log "EAST  $name — stop + delete"
  ssh -o BatchMode=yes "$EAST_HOST" "sudo lxc stop '$name' --force 2>/dev/null || true; sudo lxc delete '$name' --force 2>/dev/null || true"
}

if [[ "$SCOPE" == "all" || "$SCOPE" == "west" ]]; then
  for name in "${WEST_CONTAINERS[@]}"; do
    stop_delete_local "$name"
  done
fi

if [[ "$SCOPE" == "all" || "$SCOPE" == "east" ]]; then
  ssh -o ConnectTimeout=5 -o BatchMode=yes "$EAST_HOST" 'true' \
    || { log "ERROR: $EAST_HOST unreachable"; exit 2; }
  for name in "${EAST_CONTAINERS[@]}"; do
    stop_delete_east "$name"
  done
fi

log "verification (should show no k8s- containers):"
if [[ "$SCOPE" == "all" || "$SCOPE" == "west" ]]; then
  echo "--- WEST ---"
  sudo lxc list "k8s-" --format=table -c ns4 2>/dev/null || true
fi
if [[ "$SCOPE" == "all" || "$SCOPE" == "east" ]]; then
  echo "--- EAST ($EAST_HOST) ---"
  ssh -o BatchMode=yes "$EAST_HOST" 'sudo lxc list "k8s-" --format=table -c ns4' 2>/dev/null || true
fi

log "done. spike daemons untouched."
