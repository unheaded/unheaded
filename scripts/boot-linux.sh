#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Boot MBC Linux on the UPC
#
# Builds the init process, shell, root filesystem, and boots
# the kernel with the ramdisk.
#
# Usage: ./scripts/boot-linux.sh
#        (run from project root: ~/tmp/unheaded/)

set -euo pipefail

PROJ_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJ_ROOT"

echo "================================================================"
echo "  MBC Linux Boot — Unheaded Protocol Computer"
echo "================================================================"
echo ""

# 1. Build init
echo "[1/4] Building /init (PID 1)..."
(cd arch/mbc/boot/init && go run build.go)
echo "      -> arch/mbc/boot/init/init.upcf"
echo ""

# 2. Build shell
echo "[2/4] Building /bin/sh..."
(cd demos/mbc/shell && go run build.go)
echo "      -> demos/mbc/shell/shell.upcf"
echo ""

# 3. Create root filesystem
echo "[3/4] Creating UNFS root filesystem..."
go run arch/mbc/boot/mkrootfs.go
echo "      -> arch/mbc/boot/rootfs.img"
echo ""

# 4. Boot
echo "[4/4] Booting MBC Linux..."
echo ""
echo "  Kernel:   arch/mbc/boot/init/init.upcf"
echo "  Ramdisk:  arch/mbc/boot/rootfs.img"
echo "  Console:  ttyMBC0"
echo "  Root:     /dev/ram0 (UNFS ramdisk)"
echo ""

if command -v wotan-ctl &>/dev/null; then
    wotan-ctl boot \
        --kernel arch/mbc/boot/init/init.upcf \
        --ramdisk arch/mbc/boot/rootfs.img \
        --args "console=ttyMBC0 root=/dev/ram0"
else
    echo "  [NOTE] wotan-ctl not found — skipping actual boot."
    echo "  To boot manually:"
    echo "    wotan-ctl boot \\"
    echo "      --kernel arch/mbc/boot/init/init.upcf \\"
    echo "      --ramdisk arch/mbc/boot/rootfs.img \\"
    echo "      --args \"console=ttyMBC0 root=/dev/ram0\""
fi

echo ""
echo "================================================================"
echo "  MBC Linux booted."
echo "================================================================"
