#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# K8s Mock-Prod Lab — provision 8 LXD containers across WEST + EAST.
# Companion to docs/labs/k8s-kingdom-lab.md (Phase 0).
#
# Idempotent: skips containers that already exist.
# Spike-safe: does NOT touch anything outside the "k8s-" prefix.
#
# Usage:
#   scripts/k8s-lab-up.sh              # provision all 8
#   scripts/k8s-lab-up.sh --west-only  # only the 5 WEST containers
#   scripts/k8s-lab-up.sh --east-only  # only the 3 EAST containers
#
# Environment:
#   K8S_LAB_IMAGE     LXD image (default ubuntu:22.04)
#   K8S_LAB_EAST_HOST ssh target for east (default govan@east)
#
# Exit codes:
#   0 success / already-present
#   2 preflight failure (lxc missing / east unreachable)
#   3 launch error
set -euo pipefail

IMAGE="${K8S_LAB_IMAGE:-ubuntu:22.04}"
EAST_HOST="${K8S_LAB_EAST_HOST:-govan@east}"
SCOPE="all"
[[ "${1:-}" == "--west-only" ]] && SCOPE="west"
[[ "${1:-}" == "--east-only" ]] && SCOPE="east"

# WEST: name memory-cap
WEST_CONTAINERS=(
  "k8s-jumpbox  512MB"
  "k8s-cp-1     2GB"
  "k8s-cp-2     2GB"
  "k8s-worker-1 2GB"
  "k8s-worker-2 2GB"
)
# EAST: name memory-cap
EAST_CONTAINERS=(
  "k8s-cp-3     2GB"
  "k8s-worker-3 1500MB"
  "k8s-worker-4 1500MB"
)

log() { printf '[lab-up] %s\n' "$*" >&2; }
die() { log "ERROR: $*"; exit "${2:-3}"; }

preflight_local() {
  command -v lxc >/dev/null || die "lxc not found locally (install LXD snap, add user to lxd group)" 2
  sudo lxc list >/dev/null 2>&1 || die "sudo lxc list failed — LXD daemon not initialized? run 'sudo lxd init'" 2
}

preflight_east() {
  ssh -o ConnectTimeout=5 -o BatchMode=yes "$EAST_HOST" 'sudo lxc list' >/dev/null 2>&1 \
    || die "east unreachable or LXD broken: $EAST_HOST. fix: ensure govan in lxd group on east, 'sudo lxd init' if needed" 2
}

container_exists_local() {
  sudo lxc info "$1" >/dev/null 2>&1
}

container_exists_east() {
  ssh -o BatchMode=yes "$EAST_HOST" "sudo lxc info '$1'" >/dev/null 2>&1
}

launch_local() {
  local name="$1" mem="$2"
  if container_exists_local "$name"; then
    log "WEST  $name — exists, skipping"
    return 0
  fi
  log "WEST  $name — launching ($mem) image=$IMAGE"
  sudo lxc launch "$IMAGE" "$name" >/dev/null \
    || die "launch failed: $name"
  sudo lxc config set "$name" limits.memory="$mem" \
    || die "memory cap failed: $name $mem"
}

launch_east() {
  local name="$1" mem="$2"
  if container_exists_east "$name"; then
    log "EAST  $name — exists, skipping"
    return 0
  fi
  log "EAST  $name — launching ($mem) image=$IMAGE"
  ssh -o BatchMode=yes "$EAST_HOST" "sudo lxc launch '$IMAGE' '$name'" >/dev/null \
    || die "launch failed on east: $name"
  ssh -o BatchMode=yes "$EAST_HOST" "sudo lxc config set '$name' limits.memory='$mem'" \
    || die "memory cap failed on east: $name $mem"
}

if [[ "$SCOPE" == "all" || "$SCOPE" == "west" ]]; then
  preflight_local
  log "WEST preflight OK — provisioning ${#WEST_CONTAINERS[@]} containers"
  for entry in "${WEST_CONTAINERS[@]}"; do
    read -r name mem <<<"$entry"
    launch_local "$name" "$mem"
  done
fi

if [[ "$SCOPE" == "all" || "$SCOPE" == "east" ]]; then
  preflight_east
  log "EAST preflight OK ($EAST_HOST) — provisioning ${#EAST_CONTAINERS[@]} containers"
  for entry in "${EAST_CONTAINERS[@]}"; do
    read -r name mem <<<"$entry"
    launch_east "$name" "$mem"
  done
fi

log "verification:"
if [[ "$SCOPE" == "all" || "$SCOPE" == "west" ]]; then
  echo "--- WEST ---"
  sudo lxc list "k8s-" --format=table -c ns4
fi
if [[ "$SCOPE" == "all" || "$SCOPE" == "east" ]]; then
  echo "--- EAST ($EAST_HOST) ---"
  ssh -o BatchMode=yes "$EAST_HOST" 'sudo lxc list "k8s-" --format=table -c ns4'
fi

log "done. next: docs/labs/k8s-kingdom-lab-plan.md Phase 1 (KTHW walkthrough)"
