#!/usr/bin/env bash
# OPNsense QEMU entrypoint
# Expects /images/opnsense.img (decompressed) or /images/opnsense.img.bz2

set -euo pipefail

IMG="/images/opnsense.img"
IMG_BZ2="/images/opnsense.img.bz2"

# Decompress if needed
if [[ ! -f "$IMG" ]] && [[ -f "$IMG_BZ2" ]]; then
    echo "[opnsense] Decompressing OPNsense image (one-time)..."
    pbzip2 -dk "$IMG_BZ2"
    echo "[opnsense] Decompression complete: $IMG"
fi

if [[ ! -f "$IMG" ]]; then
    echo "[opnsense] ERROR: No image found at $IMG or $IMG_BZ2"
    exit 1
fi

# Set up bridge between WAN (eth0/macvlan) and QEMU TAP
ip tuntap add dev tap0 mode tap || true
ip link set tap0 up

ip tuntap add dev tap1 mode tap || true
ip link set tap1 up

echo "[opnsense] Starting OPNsense 26.1.2 QEMU/KVM..."
exec qemu-system-x86_64 \
    -enable-kvm \
    -M q35 \
    -cpu host \
    -smp "${OPNSENSE_CPUS}" \
    -m "${OPNSENSE_MEM}M" \
    -drive file="${IMG}",format=raw,if=virtio,cache=writeback \
    -netdev tap,id=wan,ifname=tap0,script=no,downscript=no \
    -device virtio-net-pci,netdev=wan,mac=52:54:00:aa:00:01 \
    -netdev tap,id=lan,ifname=tap1,script=no,downscript=no \
    -device virtio-net-pci,netdev=lan,mac=52:54:00:aa:00:02 \
    -serial mon:stdio \
    -nographic \
    -no-reboot \
    -boot c
