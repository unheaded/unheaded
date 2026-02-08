#!/usr/bin/env bash
# unload-ebpf.sh — Unload Unheaded eBPF programs from the kernel
#
# Usage: sudo ./scripts/unload-ebpf.sh

set -euo pipefail

BPFFS_ROOT="/sys/fs/bpf/unheaded"
INTERFACE="${UNHEADED_INTERFACE:-eth0}"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

if [[ $EUID -ne 0 ]]; then
    log_error "Must be run as root (sudo)"
    exit 1
fi

# Detach XDP from interface
if ip link show "$INTERFACE" &>/dev/null; then
    log_info "Detaching XDP from ${INTERFACE}..."
    ip link set dev "$INTERFACE" xdp off 2>/dev/null || true

    log_info "Removing TC filters from ${INTERFACE}..."
    tc filter del dev "$INTERFACE" ingress 2>/dev/null || true
    tc qdisc del dev "$INTERFACE" clsact 2>/dev/null || true
fi

# Remove pinned programs and maps
if [[ -d "$BPFFS_ROOT" ]]; then
    log_info "Removing pinned BPF objects from ${BPFFS_ROOT}..."
    rm -rf "${BPFFS_ROOT}"
    log_info "Pinned objects removed"
else
    log_info "No pinned objects found at ${BPFFS_ROOT}"
fi

log_info "eBPF programs unloaded"
