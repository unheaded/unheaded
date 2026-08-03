#!/bin/bash
# smoke-boot.sh — Boot the built Yggdrasil qcow2 in qemu and confirm
# yggdrasil-doctor upc passes. Called by Jenkinsfile "Smoke boot" stage.
#
# Status: SCAFFOLD. Real implementation requires the packer-built qcow2 to
# exist + qemu-system-x86_64 + SSH client. This file is the contract for
# what the smoke harness will do.
#
# Free to use. Free to share.
set -euo pipefail

IMAGE="${IMAGE:-../packer/build/yggdrasil-amd64/*.qcow2}"
TIMEOUT_BOOT="${TIMEOUT_BOOT:-180}"   # seconds to wait for ssh available
SSH_PORT="${SSH_PORT:-2222}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/yggdrasil-smoke-key}"

# Locate the image
# shellcheck disable=SC2086  # IMAGE defaults to a *.qcow2 glob; quoting would
# stop it expanding and ls would look for a file literally named '*.qcow2'.
IMG_PATH=$(ls $IMAGE 2>/dev/null | head -1 || true)
if [ -z "$IMG_PATH" ]; then
    echo "FAIL: no qcow2 image found matching $IMAGE"
    exit 1
fi

echo "=== Yggdrasil smoke boot — image: $IMG_PATH ==="

# Boot in snapshot mode (don't mutate the artifact)
qemu-system-x86_64 \
    -m 2G -smp 2 \
    -drive "file=$IMG_PATH,format=qcow2,if=virtio,snapshot=on" \
    -netdev "user,id=net0,hostfwd=tcp::${SSH_PORT}-:22" \
    -device virtio-net,netdev=net0 \
    -display none \
    -serial mon:stdio \
    -daemonize \
    -pidfile /tmp/yggdrasil-smoke-qemu.pid

QEMU_PID=$(cat /tmp/yggdrasil-smoke-qemu.pid)
trap "kill $QEMU_PID 2>/dev/null; rm -f /tmp/yggdrasil-smoke-qemu.pid" EXIT

# Wait for SSH
echo "Waiting up to ${TIMEOUT_BOOT}s for SSH..."
START=$(date +%s)
while true; do
    if ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -o ConnectTimeout=5 \
           -p "$SSH_PORT" "yggdrasil@localhost" true 2>/dev/null; then
        echo "SSH up after $(($(date +%s) - START))s"
        break
    fi
    if [ "$(( $(date +%s) - START ))" -ge "$TIMEOUT_BOOT" ]; then
        echo "FAIL: SSH did not come up within ${TIMEOUT_BOOT}s"
        exit 1
    fi
    sleep 5
done

# Run yggdrasil-doctor upc — must exit 0
echo "=== Running yggdrasil-doctor upc ==="
DOCTOR_OUT=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -p "$SSH_PORT" \
    "yggdrasil@localhost" "sudo yggdrasil-doctor upc")
DOCTOR_EXIT=$?

echo "$DOCTOR_OUT"

if [ "$DOCTOR_EXIT" -ne 0 ]; then
    echo "FAIL: yggdrasil-doctor upc exited $DOCTOR_EXIT"
    exit 1
fi

# Additional smoke checks
echo "=== Additional smoke checks ==="
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no -p "$SSH_PORT" "yggdrasil@localhost" '
    set -euo pipefail
    echo "kernel: $(uname -r)"
    echo "upc-bootctl: $(command -v upc-bootctl)"
    echo "upc-tty-bridge service: $(systemctl is-enabled upc-tty-bridge 2>&1)"
    echo "tty-bridge healthz: $(curl -fsS -m 2 http://127.0.0.1:26100/healthz || echo unreachable)"
    echo "monad-cpu-ebpf.o: $(test -f /opt/unheaded/share/monad-cpu-ebpf.o && echo present || echo MISSING)"
'

echo "=== Smoke boot PASSED ==="
