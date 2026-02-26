#!/usr/bin/env bash
# IPFire QEMU entrypoint
# Expects /images/ipfire.img (decompressed) or /images/ipfire.img.xz

set -euo pipefail

IMG="/images/ipfire.img"
IMG_XZ="/images/ipfire.img.xz"

# Decompress if needed
if [[ ! -f "$IMG" ]] && [[ -f "$IMG_XZ" ]]; then
    echo "[ipfire] Decompressing IPFire image (one-time)..."
    xz -dk "$IMG_XZ"
    echo "[ipfire] Decompression complete: $IMG"
fi

if [[ ! -f "$IMG" ]]; then
    echo "[ipfire] ERROR: No image found at $IMG or $IMG_XZ"
    exit 1
fi

# Set up bridge between WAN (eth0/macvlan) and QEMU TAP
ip tuntap add dev tap0 mode tap || true
ip link set tap0 up

ip tuntap add dev tap1 mode tap || true
ip link set tap1 up

echo "[ipfire] Starting IPFire 2.29 QEMU/KVM..."
exec qemu-system-x86_64 \
    -enable-kvm \
    -M q35 \
    -cpu host \
    -smp "${IPFIRE_CPUS}" \
    -m "${IPFIRE_MEM}M" \
    -drive file="${IMG}",format=raw,if=virtio,cache=writeback \
    -netdev tap,id=wan,ifname=tap0,script=no,downscript=no \
    -device virtio-net-pci,netdev=wan,mac=52:54:00:bb:00:01 \
    -netdev tap,id=lan,ifname=tap1,script=no,downscript=no \
    -device virtio-net-pci,netdev=lan,mac=52:54:00:bb:00:02 \
    -serial mon:stdio \
    -nographic \
    -no-reboot \
    -boot c
