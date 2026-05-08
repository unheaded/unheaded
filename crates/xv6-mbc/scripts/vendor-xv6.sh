#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# vendor-xv6.sh — Phase 1.1 of ASCEND-LINUX
# Vendor xv6-riscv from MIT-PDOS upstream into crates/xv6-mbc/upstream/.
#
# License compatibility: xv6-riscv is MIT (kingdom is GPL-3.0; MIT is GPL-compat).
# After vendoring, add an entry to /home/govan/tmp/unheaded/THIRD_PARTY.md
# per ADR-052 source-of-truth policy.
#
# Usage:
#   bash crates/xv6-mbc/scripts/vendor-xv6.sh
#
# Idempotent: safe to re-run; updates the pinned commit if upstream advances.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
UPSTREAM_DIR="${REPO_ROOT}/crates/xv6-mbc/upstream"
PINNED_COMMIT="riscv"  # MIT-PDOS xv6-riscv main branch is named 'riscv'
# Update PINNED_COMMIT to a specific SHA once we've validated a working build.

UPSTREAM_URL="https://github.com/mit-pdos/xv6-riscv.git"

echo "vendor-xv6.sh: vendoring ${UPSTREAM_URL} into ${UPSTREAM_DIR}"

if [ -d "${UPSTREAM_DIR}/.git" ]; then
    echo "  upstream/ already exists; fetching latest..."
    cd "${UPSTREAM_DIR}"
    git fetch origin
    git checkout "${PINNED_COMMIT}"
    git reset --hard "origin/${PINNED_COMMIT}"
else
    echo "  fresh clone of ${UPSTREAM_URL}..."
    rm -rf "${UPSTREAM_DIR}"
    git clone "${UPSTREAM_URL}" "${UPSTREAM_DIR}"
    cd "${UPSTREAM_DIR}"
    git checkout "${PINNED_COMMIT}"
fi

# Capture the actual commit hash for THIRD_PARTY.md attestation.
COMMIT_HASH=$(git rev-parse HEAD)
echo ""
echo "✓ xv6-riscv vendored at ${COMMIT_HASH}"
echo "✓ upstream MIT license preserved at ${UPSTREAM_DIR}/LICENSE"
echo ""
echo "Next steps (Phase 1.1 continued):"
echo "  1. Verify license: cat ${UPSTREAM_DIR}/LICENSE"
echo "  2. Add to THIRD_PARTY.md:"
echo ""
echo "     | xv6-riscv | ${COMMIT_HASH:0:12} | MIT | crates/xv6-mbc/upstream/ |"
echo ""
echo "  3. Continue Phase 1.1: port kernel/start.c → adapters/start_mbc.c"
echo "  4. Port kernel/uart.c → adapters/console-mmio.c (writes to MMIO 0xC001)"
echo "  5. Port kernel/virtio_disk.c → adapters/blk-ramdisk.c (SYS_READ_BLOCK)"
echo "  6. Adapt Makefile to use riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32"
echo "  7. cargo run --bin doom-runner -- --kernel xv6-mbc.mbc"
echo "     Expected: 'xv6 booting...' on stdout, then HALT."
